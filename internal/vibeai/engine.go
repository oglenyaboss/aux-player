package vibeai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/simone-vibes/vibez/internal/vibeai/llm"
	"github.com/simone-vibes/vibez/internal/vibeai/memory"
)

type Config struct {
	AppleDeveloperToken string
	AppleUserToken      string
	Storefront          string
	LastfmKey           string // пусто → similar_artists и теги отвечают ошибкой, агент работает
	LLM                 llm.Config
	MemoryPath          string // файл памяти
	CacheDir            string // кэш библиотеки и favorites
	RunsDir             string // логи сессий; "" — не писать
	SignalsPath         string // журнал сигналов; "" — только в памяти
	CacheMaxAge         time.Duration
	PortionSize         int
}

type Track struct {
	ID, Artist, Title, Album, Year, ArtworkURL, Why string
	DurationMs                                      int
	Favorite                                        bool
}

type Portion struct {
	Tracks []Track
	Note   string
	Later  *Question
}

type Engine struct {
	agent     *Agent
	session   *Session
	sig       *Signals
	mem       *memory.Store
	log       *runLog
	api       *appleAPI
	cfg       Config
	query     string
	start     time.Time
	sessionID string
	turns     int
	portions  [][]Track
	replies   []reply
	asked     []string
}

type reply struct {
	after int
	text  string
}

func (c Config) Validate() error {
	if c.AppleDeveloperToken == "" || c.AppleUserToken == "" {
		return errors.New("vibe: нет Apple-токенов — войди в Apple Music")
	}
	if c.LLM.APIKey == "" {
		return errors.New("vibe: нет ключа LLM (vibe_llm_api_key в config.json)")
	}
	return nil
}

func Start(ctx context.Context, cfg Config, ui UI) (*Engine, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Storefront == "" {
		cfg.Storefront = "us"
	}
	if cfg.CacheMaxAge == 0 {
		cfg.CacheMaxAge = 24 * time.Hour
	}
	if cfg.PortionSize == 0 {
		cfg.PortionSize = 20
	}
	api := &appleAPI{http: &http.Client{Timeout: 30 * time.Second}, dev: cfg.AppleDeveloperToken, user: cfg.AppleUserToken}
	var progress func(string)
	if ui != nil {
		progress = ui.Progress
	}
	library, favs, err := loadCaches(ctx, api, cfg.CacheDir, cfg.CacheMaxAge, progress)
	if err != nil {
		return nil, err
	}
	lfm := &lastfmClient{http: api.http, key: cfg.LastfmKey}
	client := llm.New(cfg.LLM, &http.Client{Timeout: 120 * time.Second})
	mem, err := memory.Open(cfg.MemoryPath, memory.DefaultLimit)
	if err != nil {
		return nil, err
	}
	sig, err := OpenSignals(cfg.SignalsPath)
	if err != nil {
		return nil, err
	}
	session := NewSession(library, favs, cfg.PortionSize)
	start := time.Now()
	e := &Engine{
		session:   session,
		mem:       mem,
		sig:       sig,
		log:       &runLog{},
		api:       api,
		cfg:       cfg,
		start:     start,
		sessionID: start.Format(time.RFC3339Nano),
	}
	session.SetSignals(sig, e.sessionID)
	e.agent = NewAgent(client, &liveBackend{api: api, lfm: lfm, sf: cfg.Storefront}, session, mem, ui, e.log, favs)
	return e, nil
}

func (e *Engine) Turn(ctx context.Context, text string) (Portion, error) {
	message := text
	if e.query == "" {
		e.query = text
		e.log.f("# vibe ask %s", text)
		e.log.f("время: %s", e.start.Format(time.RFC3339))
		e.log.f("память на старте: %d/%d", e.mem.Size(), e.mem.Limit)
		if block := globalBlock(e.sig.globalSkips()); block != "" {
			message += "\n\n" + block
		}
		if block := servedBlock(e.sig.servedElsewhere(e.sessionID)); block != "" {
			message += "\n\n" + block
		}
	} else {
		if text == "" {
			e.log.f("## владелец: (поток)")
		} else {
			e.log.f("## владелец: %s", text)
		}
		e.agent.Reset()
		tail := "Продолжи поток: следующая порция. Держись ядра запроса. ▲ — главный ориентир, ищи близкое по звучанию; дослушанное — слабый плюс; от ▼ и скипнутого уходи."
		if text != "" {
			tail = "Новая реплика владельца: «" + text + "». Учти её в следующей порции."
			e.session.ClearReserve()
		}
		message = e.history() + "\n\n" + tail
		if text != "" {
			e.replies = append(e.replies, reply{after: len(e.portions), text: text})
		}
	}
	e.turns++
	entries, err := e.agent.Turn(ctx, message)
	if err != nil {
		return Portion{}, err
	}
	for _, r := range e.agent.Warmup {
		e.replies = append(e.replies, reply{after: 0, text: "на вопрос «" + r.Question + "» — " + r.Answer})
	}
	e.agent.Warmup = nil
	if e.agent.Later != nil {
		e.asked = append(e.asked, e.agent.Later.Question)
	}
	e.log.f("## порция %d", e.turns)
	for _, en := range entries {
		e.log.f("%s — %s", en.Song.Artist, en.Song.Title)
	}
	if e.sig != nil && len(entries) > 0 {
		now := time.Now()
		recs := make([]signalRec, 0, len(entries))
		for _, en := range entries {
			recs = append(recs, signalRec{
				Time:    now,
				Session: e.sessionID,
				Query:   e.query,
				Kind:    "served",
				ID:      en.Song.ID,
				Artist:  en.Song.Artist,
				Title:   en.Song.Title,
			})
		}
		if err := e.sig.append(recs...); err != nil {
			e.log.f("журнал сигналов: %v", err)
		}
	}
	tracks := toTracks(entries)
	e.portions = append(e.portions, tracks)
	if e.agent.Note != "" {
		e.log.f("заметка: %s", e.agent.Note)
	}
	if e.agent.Later != nil {
		e.log.f("вопрос на потом: %s %v", e.agent.Later.Question, e.agent.Later.Options)
	}
	return Portion{Tracks: tracks, Note: e.agent.Note, Later: e.agent.Later}, nil
}

func (e *Engine) Answer(q Question, raw string) {
	ans := resolveAnswer(q, raw)
	e.log.f("## ответ владельца: «%s» → %s", q.Question, ans)
	e.replies = append(e.replies, reply{after: len(e.portions), text: "на вопрос «" + q.Question + "» — " + ans})
}

func (e *Engine) history() string {
	seeds := make([]string, 0, len(e.session.plan.Seeds))
	for _, sd := range e.session.plan.Seeds {
		seeds = append(seeds, sd.Artist+" ("+sd.Mode+")")
	}
	if len(seeds) == 0 {
		seeds = append(seeds, "нет")
	}
	excl := e.session.plan.Exclusions
	if len(excl) == 0 {
		excl = append(excl, "нет")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Сессия: запрос «%s».\n", e.query)
	fmt.Fprintf(&b, "План уже задан: ориентиры: %s; исключения: %s. set_plan — только если меняешь направление.\n", strings.Join(seeds, ", "), strings.Join(excl, ", "))
	b.WriteString("Выдано (сквозная нумерация, как на экране владельца):\n")
	n := 0
	for i, portion := range e.portions {
		fmt.Fprintf(&b, "Порция %d:\n", i+1)
		for _, t := range portion {
			n++
			fmt.Fprintf(&b, "%d. %s — %s — %s\n", n, t.Artist, t.Title, e.trackOutcome(t))
		}
	}
	if len(e.replies) > 0 {
		b.WriteString("Реплики владельца:\n")
		for _, r := range e.replies {
			fmt.Fprintf(&b, "- после порции %d: «%s»\n", r.after, r.text)
		}
	}
	if len(e.asked) > 0 {
		qs := make([]string, len(e.asked))
		for i, q := range e.asked {
			qs[i] = "«" + q + "»"
		}
		fmt.Fprintf(&b, "Вопросы уже заданы (не повторяй): %s\n", strings.Join(qs, ", "))
	}
	if block := globalBlock(e.sig.globalSkips()); block != "" {
		b.WriteString("\n")
		b.WriteString(block)
	}
	if block := servedBlock(e.sig.servedElsewhere(e.sessionID)); block != "" {
		b.WriteString("\n")
		b.WriteString(block)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (e *Engine) trackOutcome(t Track) string {
	out := "не слушал"
	if r, ok := e.sig.lastOutcome(e.sessionID, t.ID); ok {
		if r.Kind == "played" {
			out = "дослушан"
		} else {
			out = fmt.Sprintf("скип на %d:%02d", r.PosSec/60, r.PosSec%60)
		}
	}
	switch e.sig.lastRating(e.sessionID, t.ID) {
	case "more":
		out += ", ▲ больше такого"
	case "less":
		out += ", ▼ меньше такого"
	}
	return out
}

func globalBlock(stats []skipStat) string {
	if len(stats) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Устойчивые скипы прошлых сессий (этих артистов код уже не пропускает):")
	for _, st := range stats {
		fmt.Fprintf(&b, "\n- %s: ранних скипов %d в %d сессиях, дослушано %d", st.Artist, st.Early, st.Sessions, st.Played)
		if len(st.Queries) > 0 {
			qs := make([]string, len(st.Queries))
			for i, q := range st.Queries {
				qs[i] = "«" + q + "»"
			}
			fmt.Fprintf(&b, "; запросы: %s", strings.Join(qs, ", "))
		}
	}
	b.WriteString("\nЕсли у них есть общее звучание или направление — запиши его в память одной записью (memory_add или memory_replace), evidence — эти цифры. Об отдельных артистах в память не пиши.")
	return b.String()
}

func servedBlock(songs []Song) string {
	if len(songs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Уже выдавали в прошлых сессиях (код их отклонит, не предлагай):")
	idx := make(map[string]int, len(songs))
	artists := make([]string, 0, len(songs))
	titles := make([][]string, 0, len(songs))
	for _, s := range songs {
		i, ok := idx[s.Artist]
		if !ok {
			i = len(artists)
			idx[s.Artist] = i
			artists = append(artists, s.Artist)
			titles = append(titles, nil)
		}
		titles[i] = append(titles[i], s.Title)
	}
	for i, artist := range artists {
		fmt.Fprintf(&b, "\n- %s: %s", artist, strings.Join(titles[i], ", "))
	}
	return b.String()
}

func (e *Engine) Record(outs []Outcome) error {
	if e.sig == nil {
		return nil
	}
	now := time.Now()
	recs := make([]signalRec, 0, len(outs))
	for _, out := range outs {
		switch out.Kind {
		case "played", "skip_early", "skip_mid", "more", "less", "rate_clear":
		default:
			continue
		}
		e.log.f("сигнал: %s — %s: %s %d/%d с", out.Artist, out.Title, out.Kind, out.PosSec, out.DurSec)
		recs = append(recs, signalRec{
			Time:    now,
			Session: e.sessionID,
			Query:   e.query,
			Kind:    out.Kind,
			ID:      out.ID,
			Artist:  out.Artist,
			Title:   out.Title,
			PosSec:  out.PosSec,
			DurSec:  out.DurSec,
		})
	}
	return e.sig.append(recs...)
}

func (e *Engine) MemorySize() (used, limit int) {
	return e.mem.Size(), e.mem.Limit
}

func trackIDs(tracks []Track) []string {
	ids := make([]string, 0, len(tracks))
	for _, t := range tracks {
		ids = append(ids, t.ID)
	}
	return ids
}

func (e *Engine) SavePlaylist(ctx context.Context, name string, tracks []Track) (playlistID string, err error) {
	id, err := createPlaylist(ctx, e.api, name, "Vibes AI · «"+e.query+"»", trackIDs(tracks))
	if err != nil {
		return "", err
	}
	e.log.f("плейлист: %s (%d треков)", name, len(tracks))
	return id, nil
}

func (e *Engine) AppendPlaylist(ctx context.Context, playlistID string, tracks []Track) error {
	if err := addToPlaylist(ctx, e.api, playlistID, trackIDs(tracks)); err != nil {
		return err
	}
	e.log.f("плейлист +%d", len(tracks))
	return nil
}

func (e *Engine) Close() (logPath string, err error) {
	e.log.f("память в конце: %d/%d", e.mem.Size(), e.mem.Limit)
	if e.cfg.RunsDir == "" {
		return "", nil
	}
	return e.log.save(e.cfg.RunsDir, e.query, e.start)
}

func toTrack(en Entry) Track {
	return Track{
		ID:         en.Song.ID,
		Artist:     en.Song.Artist,
		Title:      en.Song.Title,
		Album:      en.Song.Album,
		Year:       en.Song.Year,
		ArtworkURL: en.Song.ArtworkURL,
		DurationMs: en.Song.DurationMs,
		Why:        en.Why,
		Favorite:   en.Favorite,
	}
}

func toTracks(entries []Entry) []Track {
	out := make([]Track, 0, len(entries))
	for _, en := range entries {
		out = append(out, toTrack(en))
	}
	return out
}
