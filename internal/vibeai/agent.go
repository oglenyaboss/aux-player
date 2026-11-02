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

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
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
	Options  []string `json:"options"`
	Default  string   `json:"default"`
	Tracks   []Track  `json:"tracks,omitempty"`
}

type UI interface {
	Ask(qs []Question) []string // one line per question: "" = default, "1".."N" = option, otherwise the listener's own text
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
	MaxSteps int // 15 by default
	Note     string

	msgs          []llm.Message
	askedThisTurn bool
	askMisses     int
	planSet       bool
	Later         *Question
	History       []Track
	turns         int
	Warmup        []answerRow
}

const askUserErr = "error: 1–2 questions; each with 2–4 options {artist, title, label}; default — the option number"

const askLaterErr = "error: a question and 2–3 options {n, label}; n is a track number from the \"Served\" list; default is the option number"

const askUserSchema = `{
  "type": "object",
  "properties": {
    "questions": {
      "type": "array",
      "minItems": 1,
      "maxItems": 2,
      "items": {
        "type": "object",
        "properties": {
          "question": {"type": "string"},
          "options": {
            "type": "array",
            "minItems": 2,
            "maxItems": 4,
            "items": {
              "type": "object",
              "properties": {
                "artist": {"type": "string"},
                "title": {"type": "string"},
                "label": {"type": "string", "description": "Option label of up to 3 words: a direction, an album or an edge"}
              },
              "required": ["artist", "title", "label"]
            }
          },
          "default": {"type": "integer", "minimum": 1, "description": "Default option number"}
        },
        "required": ["question", "options", "default"]
      }
    }
  },
  "required": ["questions"]
}`

const askLaterSchema = `{
  "type": "object",
  "properties": {
    "question": {"type": "string"},
    "options": {
      "type": "array",
      "minItems": 2,
      "maxItems": 3,
      "items": {
        "type": "object",
        "properties": {
          "n": {"type": "integer", "minimum": 1, "description": "Track number from the \"Served\" list"},
          "label": {"type": "string", "description": "Label: where to lead next, up to 3 words"}
        },
        "required": ["n", "label"]
      }
    },
    "default": {"type": "integer", "minimum": 1}
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
    "exclusions": {"type": "array", "items": {"type": "string"}, "description": "Artist names that must not be in the selection. Artists only, not albums and not tracks."},
    "from_favorites": {"type": "boolean", "description": "Favorites mode: the listener asks for their own favorite, familiar music. Only tracks from Favorites and from the library of Favorites artists."}
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
    "note": {"type": "string", "description": "Note to the listener above the portion, up to 8 words: what you hold and where you shift"}
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
		{Name: "ask_user", Description: "Ask the listener with a card: every option is a real track with artwork. The chosen track goes straight into the portion and plays. Only in the first turn, once; again — only if the answer says so in the next field.", Parameters: json.RawMessage(askUserSchema)},
		{Name: "ask_later", Description: "Ask the listener one question mid-stream without stopping the music: options are tracks already served (numbers from the \"Served\" list), the label is the direction. The answer comes as a reply in the next turn. No more than once per turn.", Parameters: json.RawMessage(askLaterSchema)},
		{Name: "set_plan", Description: "Fix the referents with modes and exclusions. Always before submit_candidates.", Parameters: json.RawMessage(setPlanSchema)},
		{Name: "describe", Description: "Description of a referent from Apple Music and Last.fm: genres, annotation, releases with dates or a tracklist, tags.", Parameters: json.RawMessage(describeSchema)},
		{Name: "similar_artists", Description: "Similar artists by shared Last.fm audience (noisy, not by sound).", Parameters: json.RawMessage(similarSchema)},
		{Name: "submit_candidates", Description: "Offer tracks for the portion. The code checks them in Apple Music and against the rules and returns the accepted and the rejected ones with reasons.", Parameters: json.RawMessage(submitSchema)},
		{Name: "memory_add", Description: "Add a lasting preference of the listener to memory.", Parameters: json.RawMessage(memoryAddSchema)},
		{Name: "memory_replace", Description: "Replace the memory entry that contains the substring old.", Parameters: json.RawMessage(memoryReplaceSchema)},
		{Name: "memory_remove", Description: "Remove the memory entry that contains the substring old.", Parameters: json.RawMessage(memoryRemoveSchema)},
		{Name: "memory_consolidate", Description: "Replace the whole memory with a compressed list of entries. Only when memory is almost full.", Parameters: json.RawMessage(memoryConsolidateSchema)},
	}
}

func langLine(lang string) string {
	if lang == "ru" {
		return "Language: write why, note, card questions and labels, and ask_later questions in Russian."
	}
	return "Language: write why, note, card questions and labels, and ask_later questions in English."
}

func NewAgent(c chatter, b backend, s *Session, m *memory.Store, ui UI, log *runLog, favorites []Song, lang string) *Agent {
	return &Agent{
		LLM:      c,
		Backend:  b,
		Session:  s,
		Memory:   m,
		UI:       ui,
		Log:      log,
		MaxSteps: 15,
		msgs:     []llm.Message{{Role: "system", Content: systemPrompt(m, favorites, lang)}},
	}
}

func systemPrompt(m *memory.Store, favorites []Song, lang string) string {
	var b strings.Builder
	b.WriteString(promptRules)
	b.WriteString("\n\n")
	b.WriteString(langLine(lang))
	b.WriteString("\n\n")
	b.WriteString(m.Section())
	b.WriteString("\n\n## Favorite tracks of the listener (Favorites)\n")
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
	if names := a.Session.TakeReserve(); len(names) > 0 {
		user += "\n\nCandidates from the previous turn that did not make it into the portion: " + strings.Join(names, "; ") + ". Take them through submit_candidates only if they fit the fresh reactions."
		a.logf("reserve offered: %s", strings.Join(names, "; "))
	}
	a.msgs = append(a.msgs, llm.Message{Role: "user", Content: user})
	hints := 0
	start := time.Now()
	for step := 1; step <= a.MaxSteps; step++ {
		resp, err := a.LLM.Chat(ctx, a.msgs, agentTools(), false)
		if err != nil {
			return nil, err
		}
		a.logf("### step %d (%d s, tokens in %d, out %d, reasoning %d)", step, int(time.Since(start).Seconds()),
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
					Content: fmt.Sprintf("Portion is not full: %d more tracks needed. Offer them through submit_candidates.", a.Session.Need()),
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

func (a *Agent) Reset() { a.msgs = a.msgs[:1] }

func (a *Agent) dispatch(ctx context.Context, call llm.ToolCall) string {
	switch call.Name {
	case "ask_user":
		var args struct {
			Questions []askUserArg `json:"questions"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.askUser(ctx, args.Questions)
	case "ask_later":
		var args askLaterArg
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.askLater(args)
	case "set_plan":
		var args struct {
			Seeds         []planSeedArg `json:"seeds"`
			Exclusions    []string      `json:"exclusions"`
			FromFavorites bool          `json:"from_favorites"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.setPlan(args.Seeds, args.Exclusions, args.FromFavorites)
	case "describe":
		var args struct {
			Kind   string `json:"kind"`
			Artist string `json:"artist"`
			Name   string `json:"name"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.describe(ctx, args.Kind, args.Artist, args.Name)
	case "similar_artists":
		var args struct {
			Artist string `json:"artist"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.similarArtists(ctx, args.Artist)
	case "submit_candidates":
		var args struct {
			Tracks []trackArg `json:"tracks"`
			Note   string     `json:"note"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
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
			return "error: " + err.Error()
		}
		return a.memoryAdd(args.Text, args.Evidence)
	case "memory_replace":
		var args struct {
			Old      string `json:"old"`
			Text     string `json:"text"`
			Evidence string `json:"evidence"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.memoryReplace(args.Old, args.Text, args.Evidence)
	case "memory_remove":
		var args struct {
			Old    string `json:"old"`
			Reason string `json:"reason"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.memoryRemove(args.Old, args.Reason)
	case "memory_consolidate":
		var args struct {
			Entries []memoryEntryArg `json:"entries"`
		}
		if err := toolArgs(call.Arguments, &args); err != nil {
			return "error: " + err.Error()
		}
		return a.memoryConsolidate(args.Entries)
	default:
		return fmt.Sprintf("error: no such tool %s", call.Name)
	}
}

func toolArgs(args string, v any) error {
	if !json.Valid([]byte(args)) {
		return errors.New("arguments are not JSON")
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

type askUserArg struct {
	Question string         `json:"question"`
	Options  []askOptionArg `json:"options"`
	Default  int            `json:"default"`
}

type askOptionArg struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Label  string `json:"label"`
}

type askLaterArg struct {
	Question string           `json:"question"`
	Options  []laterOptionArg `json:"options"`
	Default  int              `json:"default"`
}

type laterOptionArg struct {
	N     int    `json:"n"`
	Label string `json:"label"`
}

func (a *Agent) askUser(ctx context.Context, qs []askUserArg) string {
	a.progress("ask_user", fmt.Sprintf("questions %d", len(qs)))
	if a.askedThisTurn {
		return "error: questions were already asked in this turn"
	}
	if a.turns > 1 {
		return "error: ask_user is allowed only in the first turn of the session; ask a fork in the skips through ask_later"
	}
	if len(qs) < 1 || len(qs) > 2 {
		return askUserErr
	}
	for _, q := range qs {
		if strings.TrimSpace(q.Question) == "" || len(q.Options) < 2 || len(q.Options) > 4 || q.Default < 1 || q.Default > len(q.Options) {
			return askUserErr
		}
		for _, o := range q.Options {
			if strings.TrimSpace(o.Artist) == "" || strings.TrimSpace(o.Title) == "" || strings.TrimSpace(o.Label) == "" {
				return askUserErr
			}
		}
	}
	if a.UI == nil {
		return "error: no listener display attached; build the portion without asking"
	}
	sugs := make([]Suggestion, 0, len(qs)*4)
	for _, q := range qs {
		for _, o := range q.Options {
			sugs = append(sugs, Suggestion{Artist: o.Artist, Title: o.Title, Why: o.Label})
		}
	}
	hits := a.findSongs(ctx, sugs)
	songs := make([][]Song, len(qs))
	missing := make([]string, 0)
	idx := 0
	for qi, q := range qs {
		songs[qi] = make([]Song, len(q.Options))
		for oi, o := range q.Options {
			if hits[idx].found {
				songs[qi][oi] = hits[idx].song
			} else {
				missing = append(missing, o.Artist+" — "+o.Title)
			}
			idx++
		}
	}
	if len(missing) > 0 {
		return "error: not found in Apple Music: " + strings.Join(missing, "; ") + ". Replace those options and call ask_user again."
	}
	questions := make([]Question, len(qs))
	for qi, q := range qs {
		labels := make([]string, len(q.Options))
		tracks := make([]Track, len(q.Options))
		for oi, o := range q.Options {
			labels[oi] = o.Label
			song := songs[qi][oi]
			tracks[oi] = Track{
				ID:         song.ID,
				Artist:     song.Artist,
				Title:      song.Title,
				Album:      song.Album,
				Year:       song.Year,
				ArtworkURL: song.ArtworkURL,
				DurationMs: song.DurationMs,
				Why:        o.Label,
			}
		}
		questions[qi] = Question{Question: q.Question, Options: labels, Tracks: tracks, Default: strconv.Itoa(q.Default)}
	}
	a.askedThisTurn = true
	answers := a.UI.Ask(questions)
	taken := make([]string, 0)
	rows := make([]answerRow, len(questions))
	for i, q := range questions {
		raw := ""
		if i < len(answers) {
			raw = answers[i]
		}
		for _, oi := range pickedOptions(q, raw) {
			song := songs[i][oi]
			v := a.Session.Take(Suggestion{Artist: song.Artist, Title: song.Title, Why: q.Options[oi]}, song)
			if !v.Accepted {
				continue
			}
			name := song.Artist + " — " + song.Title
			taken = append(taken, name)
			if a.UI != nil {
				portion := a.Session.Portion()
				a.UI.Picked(Pick{
					Track:    toTrack(portion[len(portion)-1]),
					Name:     name,
					Accepted: true,
				})
			}
			a.logf("picked: %s — %s", song.Artist, song.Title)
		}
		rows[i] = answerRow{Question: q.Question, Answer: resolveAnswer(q, raw)}
	}
	miss := len(answers) > 0
	for _, raw := range answers {
		if strings.TrimSpace(raw) != "none" {
			miss = false
		}
	}
	next := ""
	if miss {
		a.askMisses++
		if a.askMisses == 1 {
			a.askedThisTurn = false
			next = "The listener answered \"missed\": no option fit. Call ask_user again: other sides of the sound, without the shown tracks and their artists."
		} else {
			next = "The listener answered \"missed\" again. Do not ask any more: build the portion yourself and move away from the shown directions."
		}
	}
	a.Warmup = append(a.Warmup, rows...)
	out := struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
		Portion string      `json:"portion"`
		Need    int         `json:"need"`
		Next    string      `json:"next,omitempty"`
	}{
		Answers: rows,
		Taken:   taken,
		Portion: fmt.Sprintf("%d/%d", len(a.Session.Portion()), a.Session.Size),
		Need:    a.Session.Need(),
		Next:    next,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}

type answerRow struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func resolveAnswer(q Question, raw string) string {
	v := strings.TrimSpace(raw)
	if v == "none" {
		return "missed: no option fits"
	}
	if v == "" {
		v = q.Default
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= len(q.Options) {
		return q.Options[n-1] + " (" + q.Tracks[n-1].Artist + " — " + q.Tracks[n-1].Title + ")"
	}
	if v == "all" {
		parts := make([]string, 0, len(q.Tracks))
		for _, t := range q.Tracks {
			parts = append(parts, t.Artist+" — "+t.Title)
		}
		return "all together: " + strings.Join(parts, "; ")
	}
	return "my own pick: " + v
}

func pickedOptions(q Question, raw string) []int {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = q.Default
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= len(q.Tracks) {
		return []int{n - 1}
	}
	if v == "all" {
		all := make([]int, len(q.Tracks))
		for i := range all {
			all[i] = i
		}
		return all
	}
	return nil
}

func (a *Agent) askLater(args askLaterArg) string {
	a.progress("ask_later", args.Question)
	if a.Later != nil {
		return "error: the question was already asked in this turn"
	}
	if strings.TrimSpace(args.Question) == "" || len(args.Options) < 2 || len(args.Options) > 3 || args.Default < 1 || args.Default > len(args.Options) {
		return askLaterErr
	}
	labels := make([]string, len(args.Options))
	tracks := make([]Track, len(args.Options))
	for i, o := range args.Options {
		if o.N < 1 || o.N > len(a.History) || strings.TrimSpace(o.Label) == "" {
			return askLaterErr
		}
		labels[i] = o.Label
		tracks[i] = a.History[o.N-1]
	}
	a.Later = &Question{Question: args.Question, Options: labels, Tracks: tracks, Default: strconv.Itoa(args.Default)}
	return "ok: the question will be shown to the listener, the answer comes as a reply"
}

func (a *Agent) setPlan(seeds []planSeedArg, exclusions []string, fromFavorites bool) string {
	a.progress("set_plan", fmt.Sprintf("referents %d, exclusions %d", len(seeds), len(exclusions)))
	for _, s := range seeds {
		switch s.Mode {
		case "not_it", "and_similar", "want_it":
		default:
			return "error: unknown mode " + s.Mode
		}
	}
	ss := make([]Seed, len(seeds))
	for i, s := range seeds {
		ss[i] = Seed(s)
	}
	a.Session.SetPlan(Plan{Seeds: ss, Exclusions: exclusions, FromFavorites: fromFavorites})
	a.planSet = true
	res := fmt.Sprintf("plan accepted: referents %d, exclusions %d", len(seeds), len(exclusions))
	if fromFavorites {
		res += ", favorites mode"
	}
	return res
}

func (a *Agent) describe(ctx context.Context, kind, artist, name string) string {
	switch kind {
	case "artist", "album", "track":
	default:
		return "error: unknown kind " + kind
	}
	if kind != "artist" && strings.TrimSpace(name) == "" {
		return "error: name is required"
	}
	if kind == "artist" && strings.TrimSpace(artist) == "" {
		artist, name = name, ""
	}
	if kind == "artist" && strings.TrimSpace(artist) == "" {
		return "error: artist is required"
	}
	short := kind + " " + artist
	if name != "" {
		short += " — " + name
	}
	a.progress("describe", short)
	res, err := a.Backend.describe(ctx, kind, artist, name)
	if err != nil {
		return "error: " + err.Error()
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}

func (a *Agent) similarArtists(ctx context.Context, artist string) string {
	a.progress("similar_artists", artist)
	sims, err := a.Backend.similarArtists(ctx, artist)
	if err != nil {
		return "error: " + err.Error()
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
		return "error: " + err.Error()
	}
	return string(b)
}

type similarOut struct {
	Name  string `json:"name"`
	Match int    `json:"match"`
}

type songHit struct {
	song  Song
	found bool
}

func (a *Agent) findSongs(ctx context.Context, sugs []Suggestion) []songHit {
	hits := make([]songHit, len(sugs))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, sug := range sugs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			song, ok, err := a.Backend.findSong(ctx, sug)
			if err != nil {
				ok = false
			}
			hits[i] = songHit{song: song, found: ok}
		})
	}
	wg.Wait()
	return hits
}

func (a *Agent) submitCandidates(ctx context.Context, tracks []trackArg) string {
	if !a.planSet {
		return "error: call set_plan first"
	}
	if len(tracks) < 1 || len(tracks) > 30 {
		return "error: 1–30 tracks"
	}
	sugs := make([]Suggestion, len(tracks))
	for i, t := range tracks {
		sugs[i] = Suggestion(t)
	}
	hits := a.findSongs(ctx, sugs)

	accepted := make([]string, 0)
	type rejectedRow struct {
		Track  string `json:"track"`
		Reason string `json:"reason"`
	}
	rejected := make([]rejectedRow, 0)
	for i, t := range tracks {
		sug := Suggestion(t)
		v := a.Session.Offer(sug, hits[i].song, hits[i].found)
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
		a.UI.Progress(fmt.Sprintf("· submit %d → accepted %d, rejected %d (portion %s)",
			len(tracks), len(accepted), len(rejected), out.Portion))
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}

func (a *Agent) memoryAdd(text, evidence string) string {
	a.progress("memory_add", truncateRunes(text, 80))
	if err := a.Memory.Add(text, evidence); err != nil {
		return "error: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryReplace(old, text, evidence string) string {
	a.progress("memory_replace", truncateRunes(old, 80))
	if err := a.Memory.Replace(old, text, evidence); err != nil {
		return "error: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryRemove(old, reason string) string {
	a.progress("memory_remove", truncateRunes(old, 80))
	if err := a.Memory.Remove(old, reason); err != nil {
		return "error: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryConsolidate(entries []memoryEntryArg) string {
	a.progress("memory_consolidate", fmt.Sprintf("entries %d", len(entries)))
	es := make([]memory.Entry, len(entries))
	for i, e := range entries {
		es[i] = memory.Entry{Text: e.Text, Evidence: e.Evidence}
	}
	if err := a.Memory.Consolidate(es); err != nil {
		return "error: " + err.Error()
	}
	return a.memoryOK()
}

func (a *Agent) memoryOK() string {
	return fmt.Sprintf("ok: memory %d/%d", a.Memory.Size(), a.Memory.Limit)
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
