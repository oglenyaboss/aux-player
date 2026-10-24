package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simone-vibes/vibez/internal/vibeai/llm"
	"github.com/simone-vibes/vibez/internal/vibeai/memory"
)

type chatter interface {
	Chat(ctx context.Context, msgs []llm.Message, tools []llm.Tool, jsonMode bool) (llm.Response, error)
}

type backend interface {
	describe(ctx context.Context, kind, artist, name string) (any, error) // kind: artist | album | track
	similarArtists(ctx context.Context, artist string) ([]similar, error)
	findSong(ctx context.Context, sug Suggestion) (Song, bool, error)
}

type Question struct {
	Question string   `json:"question"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options"`
	Left     string   `json:"left"`
	Right    string   `json:"right"`
	Steps    int      `json:"steps"`
	Default  string   `json:"default"`
}

type UI interface {
	Ask(qs []Question) []string // по строке на вопрос: "" = по умолчанию, "1".."N" = вариант, иначе свой текст
	Progress(line string)
	Picked(p Pick)
}

type Pick struct {
	Track    Track
	Name     string
	Accepted bool
	Reason   string
}

type Agent struct {
	LLM      chatter
	Backend  backend
	Session  *Session
	Memory   *memory.Store
	UI       UI
	Log      *runLog
	MaxSteps int // по умолчанию 15
	Note     string

	msgs          []llm.Message
	askedThisTurn bool
	planSet       bool
	Later         *Question
	turns         int
	Warmup        []answerRow
}

const askUserErr = "ошибка: 1–4 вопроса; choice — 2–4 варианта; scale — steps 3–7 и подписи left/right"

const askUserSchema = `{
  "type": "object",
  "properties": {
    "questions": {
      "type": "array",
      "minItems": 1,
      "maxItems": 4,
      "items": {
        "type": "object",
        "properties": {
          "question": {"type": "string"},
          "kind": {"type": "string", "enum": ["choice", "scale"]},
          "options": {"type": "array", "items": {"type": "string"}},
          "left": {"type": "string"},
          "right": {"type": "string"},
          "steps": {"type": "integer", "minimum": 3, "maximum": 7},
          "default": {"type": "string", "description": "choice: текст варианта; scale: номер шага 1..steps"}
        },
        "required": ["question", "kind", "default"]
      }
    }
  },
  "required": ["questions"]
}`

const askLaterSchema = `{
  "type": "object",
  "properties": {
    "question": {"type": "string"},
    "options": {"type": "array", "items": {"type": "string"}},
    "default": {"type": "string"}
  },
  "required": ["question", "options", "default"]
}`

const setPlanSchema = `{
  "type": "object",
  "properties": {
    "seeds": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "artist": {"type": "string"},
          "mode": {"type": "string", "enum": ["not_it", "and_similar", "want_it"]}
        },
        "required": ["artist", "mode"]
      }
    },
    "exclusions": {"type": "array", "items": {"type": "string"}, "description": "Имена артистов, которых не должно быть в подборке. Только артисты, не альбомы и не треки."}
  },
  "required": ["seeds", "exclusions"]
}`

const describeSchema = `{
  "type": "object",
  "properties": {
    "kind": {"type": "string", "enum": ["artist", "album", "track"]},
    "artist": {"type": "string"},
    "name": {"type": "string"}
  },
  "required": ["kind", "artist"]
}`

const similarSchema = `{
  "type": "object",
  "properties": {
    "artist": {"type": "string"}
  },
  "required": ["artist"]
}`

const submitSchema = `{
  "type": "object",
  "properties": {
    "tracks": {
      "type": "array",
      "minItems": 1,
      "maxItems": 30,
      "items": {
        "type": "object",
        "properties": {
          "artist": {"type": "string"},
          "title": {"type": "string"},
          "why": {"type": "string"}
        },
        "required": ["artist", "title", "why"]
      }
    },
    "note": {"type": "string", "description": "Заметка владельцу над порцией, до 8 слов: что держишь и куда сдвигаешь"}
  },
  "required": ["tracks", "note"]
}`

const memoryAddSchema = `{
  "type": "object",
  "properties": {
    "text": {"type": "string"},
    "evidence": {"type": "string"}
  },
  "required": ["text", "evidence"]
}`

const memoryReplaceSchema = `{
  "type": "object",
  "properties": {
    "old": {"type": "string"},
    "text": {"type": "string"},
    "evidence": {"type": "string"}
  },
  "required": ["old", "text", "evidence"]
}`

const memoryRemoveSchema = `{
  "type": "object",
  "properties": {
    "old": {"type": "string"},
    "reason": {"type": "string"}
  },
  "required": ["old", "reason"]
}`

const memoryConsolidateSchema = `{
  "type": "object",
  "properties": {
    "entries": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "text": {"type": "string"},
          "evidence": {"type": "string"}
        },
        "required": ["text", "evidence"]
      }
    }
  },
  "required": ["entries"]
}`

func agentTools() []llm.Tool {
	return []llm.Tool{
		{Name: "ask_user", Description: "Задать владельцу уточняющие вопросы с вариантами. Один раз за ход.", Parameters: json.RawMessage(askUserSchema)},
		{Name: "ask_later", Description: "Задать владельцу один короткий вопрос посреди потока, не останавливая музыку. Ответ придёт репликой в следующем ходе. Не чаще раза за ход.", Parameters: json.RawMessage(askLaterSchema)},
		{Name: "set_plan", Description: "Зафиксировать ориентиры с режимами и исключения. Обязательно до submit_candidates.", Parameters: json.RawMessage(setPlanSchema)},
		{Name: "describe", Description: "Описание ориентира из Apple Music и Last.fm: жанры, аннотация, релизы с датами или треклист, теги.", Parameters: json.RawMessage(describeSchema)},
		{Name: "similar_artists", Description: "Похожие артисты по общей аудитории Last.fm (шумно, не по звучанию).", Parameters: json.RawMessage(similarSchema)},
		{Name: "submit_candidates", Description: "Предложить треки в порцию. Код проверит их в Apple Music и по правилам и вернёт принятые и отклонённые с причинами.", Parameters: json.RawMessage(submitSchema)},
		{Name: "memory_add", Description: "Добавить устойчивое предпочтение владельца в память.", Parameters: json.RawMessage(memoryAddSchema)},
		{Name: "memory_replace", Description: "Заменить запись памяти, которая содержит подстроку old.", Parameters: json.RawMessage(memoryReplaceSchema)},
		{Name: "memory_remove", Description: "Удалить запись памяти, которая содержит подстроку old.", Parameters: json.RawMessage(memoryRemoveSchema)},
		{Name: "memory_consolidate", Description: "Заменить всю память сжатым списком записей. Только когда память почти заполнена.", Parameters: json.RawMessage(memoryConsolidateSchema)},
	}
}

func NewAgent(c chatter, b backend, s *Session, m *memory.Store, ui UI, log *runLog, favorites []Song) *Agent {
	return &Agent{
		LLM:      c,
		Backend:  b,
		Session:  s,
		Memory:   m,
		UI:       ui,
		Log:      log,
		MaxSteps: 15,
		msgs:     []llm.Message{{Role: "system", Content: systemPrompt(m, favorites)}},
	}
}

func systemPrompt(m *memory.Store, favorites []Song) string {
	var b strings.Builder
	b.WriteString(promptRules)
	b.WriteString("\n\n")
	b.WriteString(m.Section())
	b.WriteString("\n\n## Любимые треки владельца (Favorites)\n")
	for _, s := range favorites {
		fmt.Fprintf(&b, "- %s — %s (%s)\n", s.Artist, s.Title, s.Album)
	}
	return b.String()
}

func (a *Agent) Turn(ctx context.Context, user string) ([]Entry, error) {
	a.turns++
	a.askedThisTurn = false
	a.Note = ""
	a.Later = nil
	if picked := a.drainReserve(); len(picked) > 0 {
		user += fmt.Sprintf("\n\nИз запаса прошлой порции уже приняты: %s. Порция %d/%d, нужно ещё %d.",
			strings.Join(picked, "; "), len(a.Session.Portion()), a.Session.Size, a.Session.Need())
	}
	a.msgs = append(a.msgs, llm.Message{Role: "user", Content: user})
	hints := 0
	start := time.Now()
	for step := 1; step <= a.MaxSteps; step++ {
		resp, err := a.LLM.Chat(ctx, a.msgs, agentTools(), false)
		if err != nil {
			return nil, err
		}
		a.logf("### шаг %d (%d с, токены in %d, out %d, reasoning %d)", step, int(time.Since(start).Seconds()),
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.ReasoningTokens)
		if resp.Message.Content != "" {
			a.logf("%s", resp.Message.Content)
		}
		a.msgs = append(a.msgs, resp.Message)
		if len(resp.Message.ToolCalls) == 0 {
			if a.Session.Done() {
				break
			}
			if hints < 2 {
				hints++
				a.msgs = append(a.msgs, llm.Message{
					Role:    "user",
					Content: fmt.Sprintf("Порция не заполнена: нужно ещё %d треков. Предлагай через submit_candidates.", a.Session.Need()),
				})
				continue
			}
			break
		}
		for _, call := range resp.Message.ToolCalls {
			a.logf("→ %s %s", call.Name, call.Arguments)
			res := a.dispatch(ctx, call)
			a.logf("← %s", truncateRunes(res, 1500))
			a.msgs = append(a.msgs, llm.Message{Role: "tool", ToolCallID: call.ID, Content: res})
		}
		if a.Session.Done() {
			break
		}
	}
	return a.Session.ClosePortion(), nil
}

func (a *Agent) drainReserve() []string {
	accepted := make([]string, 0)
	for _, v := range a.Session.DrainReserve() {
		if !v.Accepted {
			continue
		}
		name := v.Song.Artist + " — " + v.Song.Title
		accepted = append(accepted, name)
		a.logf("запас: %s", name)
		if a.UI != nil {
			portion := a.Session.Portion()
			a.UI.Picked(Pick{
				Track:    toTrack(portion[len(portion)-1]),
				Name:     name,
				Accepted: true,
			})
		}
	}
	return accepted
}

func (a *Agent) Reset() { a.msgs = a.msgs[:1] }

func (a *Agent) dispatch(ctx context.Context, call llm.ToolCall) string {
	switch call.Name {
	case "ask_user":
		var args struct {
			Questions []Question `json:"questions"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.askUser(args.Questions)
	case "ask_later":
		var later Question
		if err := toolArgs(call.Arguments, &later); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.askLater(later)
	case "set_plan":
		var args struct {
			Seeds      []planSeedArg `json:"seeds"`
			Exclusions []string      `json:"exclusions"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.setPlan(args.Seeds, args.Exclusions)
	case "describe":
		var args struct {
			Kind   string `json:"kind"`
			Artist string `json:"artist"`
			Name   string `json:"name"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.describe(ctx, args.Kind, args.Artist, args.Name)
	case "similar_artists":
		var args struct {
			Artist string `json:"artist"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.similarArtists(ctx, args.Artist)
	case "submit_candidates":
		var args struct {
			Tracks []trackArg `json:"tracks"`
			Note   string     `json:"note"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		if note := strings.TrimSpace(args.Note); note != "" {
			a.Note = note
		}
		return a.submitCandidates(ctx, args.Tracks)
	case "memory_add":
		var args struct {
			Text     string `json:"text"`
			Evidence string `json:"evidence"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.memoryAdd(args.Text, args.Evidence)
	case "memory_replace":
		var args struct {
			Old      string `json:"old"`
			Text     string `json:"text"`
			Evidence string `json:"evidence"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.memoryReplace(args.Old, args.Text, args.Evidence)
	case "memory_remove":
		var args struct {
			Old    string `json:"old"`
			Reason string `json:"reason"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.memoryRemove(args.Old, args.Reason)
	case "memory_consolidate":
		var args struct {
			Entries []memoryEntryArg `json:"entries"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "ошибка: " + err.Error()
		}
		return a.memoryConsolidate(args.Entries)
	default:
		return fmt.Sprintf("ошибка: нет инструмента %s", call.Name)
	}
}

func toolArgs(args string, v any) error {
	if !json.Valid([]byte(args)) {
		return errors.New("аргументы не JSON")
	}
	return json.Unmarshal([]byte(args), v)
}

type trackArg struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Why    string `json:"why"`
}

type planSeedArg struct {
	Artist string `json:"artist"`
	Mode   string `json:"mode"`
}

type memoryEntryArg struct {
	Text     string `json:"text"`
	Evidence string `json:"evidence"`
}

func (a *Agent) askUser(qs []Question) string {
	a.progress("ask_user", fmt.Sprintf("вопросов %d", len(qs)))
	if a.askedThisTurn {
		return "ошибка: вопросы уже заданы в этом ходе"
	}
	if a.turns > 1 {
		return "ошибка: ask_user — только в первом ходе сессии; развилку по скипам спроси через ask_later"
	}
	if len(qs) < 1 || len(qs) > 4 {
		return askUserErr
	}
	for _, q := range qs {
		switch q.Kind {
		case "", "choice":
			if len(q.Options) < 2 || len(q.Options) > 4 {
				return askUserErr
			}
		case "scale":
			if q.Steps < 3 || q.Steps > 7 || strings.TrimSpace(q.Left) == "" || strings.TrimSpace(q.Right) == "" {
				return askUserErr
			}
		default:
			return askUserErr
		}
	}
	a.askedThisTurn = true
	answers := a.UI.Ask(qs)
	rows := make([]answerRow, len(qs))
	for i, q := range qs {
		raw := ""
		if i < len(answers) {
			raw = answers[i]
		}
		rows[i] = answerRow{Question: q.Question, Answer: resolveAnswer(q, raw)}
	}
	a.Warmup = rows
	b, err := json.Marshal(rows)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	return string(b)
}

type answerRow struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func resolveAnswer(q Question, raw string) string {
	v := strings.TrimSpace(raw)
	if q.Kind == "scale" {
		empty := v == ""
		if empty {
			v = q.Default
		}
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= q.Steps {
			return fmt.Sprintf("%d из %d (1 — %s, %d — %s)", n, q.Steps, q.Left, q.Steps, q.Right)
		}
		if empty {
			return q.Default
		}
		return "свой вариант: " + v
	}
	if v == "" {
		return q.Default
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= len(q.Options) {
		return q.Options[n-1]
	}
	return "свой вариант: " + v
}

func (a *Agent) askLater(q Question) string {
	a.progress("ask_later", q.Question)
	if a.Later != nil {
		return "ошибка: вопрос уже задан в этом ходе"
	}
	if len(q.Options) < 2 || len(q.Options) > 3 || strings.TrimSpace(q.Question) == "" {
		return "ошибка: вопрос и 2–3 варианта"
	}
	q.Kind = "choice"
	a.Later = &q
	return "ok: вопрос покажу владельцу, ответ придёт репликой"
}

func (a *Agent) setPlan(seeds []planSeedArg, exclusions []string) string {
	a.progress("set_plan", fmt.Sprintf("ориентиров %d, исключений %d", len(seeds), len(exclusions)))
	for _, s := range seeds {
		switch s.Mode {
		case "not_it", "and_similar", "want_it":
		default:
			return "ошибка: mode " + s.Mode
		}
	}
	ss := make([]Seed, len(seeds))
	for i, s := range seeds {
		ss[i] = Seed{Artist: s.Artist, Mode: s.Mode}
	}
	a.Session.SetPlan(Plan{Seeds: ss, Exclusions: exclusions})
	a.planSet = true
	return fmt.Sprintf("план принят: ориентиров %d, исключений %d", len(seeds), len(exclusions))
}

func (a *Agent) describe(ctx context.Context, kind, artist, name string) string {
	switch kind {
	case "artist", "album", "track":
	default:
		return "ошибка: kind " + kind
	}
	if kind != "artist" && strings.TrimSpace(name) == "" {
		return "ошибка: нужен name"
	}
	short := kind + " " + artist
	if name != "" {
		short += " — " + name
	}
	a.progress("describe", short)
	res, err := a.Backend.describe(ctx, kind, artist, name)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	return string(b)
}

func (a *Agent) similarArtists(ctx context.Context, artist string) string {
	a.progress("similar_artists", artist)
	sims, err := a.Backend.similarArtists(ctx, artist)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	if len(sims) > 20 {
		sims = sims[:20]
	}
	out := make([]similarOut, len(sims))
	for i, s := range sims {
		out[i] = similarOut{Name: s.Name, Match: matchPct(s.Match)}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	return string(b)
}

type similarOut struct {
	Name  string `json:"name"`
	Match int    `json:"match"`
}

func (a *Agent) submitCandidates(ctx context.Context, tracks []trackArg) string {
	if !a.planSet {
		return "ошибка: сначала set_plan"
	}
	if len(tracks) < 1 || len(tracks) > 30 {
		return "ошибка: 1–30 треков"
	}
	type found struct {
		song Song
		ok   bool
	}
	results := make([]found, len(tracks))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, t := range tracks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			song, ok, err := a.Backend.findSong(ctx, Suggestion{Artist: t.Artist, Title: t.Title, Why: t.Why})
			if err != nil {
				ok = false
			}
			results[i] = found{song: song, ok: ok}
		}()
	}
	wg.Wait()

	accepted := make([]string, 0)
	type rejectedRow struct {
		Track  string `json:"track"`
		Reason string `json:"reason"`
	}
	rejected := make([]rejectedRow, 0)
	for i, t := range tracks {
		sug := Suggestion{Artist: t.Artist, Title: t.Title, Why: t.Why}
		v := a.Session.Offer(sug, results[i].song, results[i].ok)
		if a.UI != nil {
			if v.Accepted {
				portion := a.Session.Portion()
				a.UI.Picked(Pick{
					Track:    toTrack(portion[len(portion)-1]),
					Name:     v.Song.Artist + " — " + v.Song.Title,
					Accepted: true,
				})
			} else {
				a.UI.Picked(Pick{Name: t.Artist + " — " + t.Title, Reason: v.Reason})
			}
		}
		if v.Accepted {
			accepted = append(accepted, v.Song.Artist+" — "+v.Song.Title)
			continue
		}
		rejected = append(rejected, rejectedRow{Track: t.Artist + " — " + t.Title, Reason: v.Reason})
	}
	out := struct {
		Accepted  []string      `json:"accepted"`
		Rejected  []rejectedRow `json:"rejected"`
		Portion   string        `json:"portion"`
		Need      int           `json:"need"`
		KnownLeft int           `json:"known_left"`
	}{
		Accepted:  accepted,
		Rejected:  rejected,
		Portion:   fmt.Sprintf("%d/%d", len(a.Session.Portion()), a.Session.Size),
		Need:      a.Session.Need(),
		KnownLeft: a.Session.KnownLeft(),
	}
	if a.UI != nil {
		a.UI.Progress(fmt.Sprintf("· submit %d → принято %d, отклонено %d (порция %s)",
			len(tracks), len(accepted), len(rejected), out.Portion))
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "ошибка: " + err.Error()
	}
	return string(b)
}

func (a *Agent) memoryAdd(text, evidence string) string {
	a.progress("memory_add", truncateRunes(text, 80))
	if err := a.Memory.Add(text, evidence); err != nil {
		return "ошибка: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryReplace(old, text, evidence string) string {
	a.progress("memory_replace", truncateRunes(old, 80))
	if err := a.Memory.Replace(old, text, evidence); err != nil {
		return "ошибка: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryRemove(old, reason string) string {
	a.progress("memory_remove", truncateRunes(old, 80))
	if err := a.Memory.Remove(old, reason); err != nil {
		return "ошибка: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryConsolidate(entries []memoryEntryArg) string {
	a.progress("memory_consolidate", fmt.Sprintf("записей %d", len(entries)))
	es := make([]memory.Entry, len(entries))
	for i, e := range entries {
		es[i] = memory.Entry{Text: e.Text, Evidence: e.Evidence}
	}
	if err := a.Memory.Consolidate(es); err != nil {
		return "ошибка: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryOK() string {
	return fmt.Sprintf("ok: память %d/%d", a.Memory.Size(), a.Memory.Limit)
}

func (a *Agent) progress(name, short string) {
	if a.UI != nil {
		a.UI.Progress("· " + name + " " + short)
	}
}

func (a *Agent) logf(format string, args ...any) {
	if a.Log != nil {
		a.Log.f(format, args...)
	}
}

func matchPct(m float64) int {
	return int(math.Floor(m*100 + 1e-9))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
