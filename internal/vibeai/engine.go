package vibeai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
)

type Config struct {
	AppleDeveloperToken string
	AppleUserToken      string
	Storefront          string
	LastfmKey           string // empty → similar_artists and tags answer with an error, the agent still works
	LLM                 llm.Config
	MemoryPath          string // memory file
	CacheDir            string // library and Favorites cache
	RunsDir             string // session logs; "" — do not write
	SignalsPath         string // signals log; "" — in memory only
	CacheMaxAge         time.Duration
	PortionSize         int
	Lang                string
}

type Track struct {
	ID, Artist, Title, Album, Year, ArtworkURL, Why string
	Released                                        string
	DurationMs                                      int
	Favorite                                        bool
}

type Portion struct {
	Tracks []Track
	Note   string
	Later  *Question
}

type Engine struct {
	agent      *Agent
	session    *Session
	sig        *Signals
	mem        *memory.Store
	log        *runLog
	api        *appleAPI
	cfg        Config
	query      string
	start      time.Time
	sessionID  string
	turns      int
	portions   [][]Track
	replies    []reply
	asked      []string
	skipped    []string
	laterAsked bool
}

type reply struct {
	after int
	text  string
}

func (c Config) Validate() error {
	if c.AppleDeveloperToken == "" || c.AppleUserToken == "" {
		return errors.New("vibe: no Apple tokens — sign in to Apple Music")
	}
	if c.LLM.APIKey == "" {
		return errors.New("vibe: no LLM key (vibe_llm_api_key in config.json)")
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
		cfg.PortionSize = 5
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
	e.agent = NewAgent(client, &liveBackend{api: api, lfm: lfm, sf: cfg.Storefront}, session, mem, ui, e.log, favs, cfg.Lang)
	return e, nil
}

func (e *Engine) Turn(ctx context.Context, text string) (Portion, error) {
	message := text
	if e.query == "" {
		e.query = text
		e.log.f("# vibe ask %s", text)
		e.log.f("time: %s", e.start.Format(time.RFC3339))
		e.log.f("memory at start: %d/%d", e.mem.Size(), e.mem.Limit)
		if block := globalBlock(e.sig.globalSkips()); block != "" {
			message += "\n\n" + block
		}
		if block := skippedBlock(e.sig.skippedElsewhere(e.sessionID)); block != "" {
			message += "\n\n" + block
		}
	} else {
		if text == "" {
			e.log.f("## listener: (stream)")
		} else {
			e.log.f("## listener: %s", text)
		}
		e.agent.Reset()
		e.session.ResetPortion()
		tail := "Continue the stream: the next portion. Hold the core of the request. ▲ is the main referent, look for something close in sound; played through is a weak plus; move away from ▼ and skipped."
		if text != "" {
			tail = "New reply from the listener: \"" + text + "\". Take it into account in the next portion."
			e.session.ClearReserve()
		}
		if text == "" && len(e.skipped) >= 2 {
			tail += "\n\nSkipped since the last question: " + strings.Join(e.skipped, "; ") + ". If you see a fork — one played through, the other skipped — call ask_later: options are played-through tracks from the \"Served\" list."
		}
		message = e.history() + "\n\n" + tail
		if text != "" {
			e.replies = append(e.replies, reply{after: len(e.portions), text: text})
		}
	}
	e.turns++
	history := make([]Track, 0)
	for _, p := range e.portions {
		history = append(history, p...)
	}
	e.agent.History = history
	entries, err := e.agent.Turn(ctx, message)
	if err != nil {
		return Portion{}, err
	}
	if e.agent.Later != nil {
		e.skipped = nil
	}
	for _, r := range e.agent.Warmup {
		e.replies = append(e.replies, reply{after: 0, text: "to the question \"" + r.Question + "\" — " + r.Answer})
	}
	e.agent.Warmup = nil
	e.laterAsked = e.agent.Later != nil
	if e.laterAsked {
		e.asked = append(e.asked, e.agent.Later.Question)
	}
	e.log.f("## portion %d", e.turns)
	for _, en := range entries {
		e.log.f("%s — %s", en.Song.Artist, en.Song.Title)
	}
	tracks := toTracks(entries)
	e.portions = append(e.portions, tracks)
	if e.agent.Note != "" {
		e.log.f("note: %s", e.agent.Note)
	}
	if e.agent.Later != nil {
		e.log.f("later question: %s %v", e.agent.Later.Question, e.agent.Later.Options)
	}
	return Portion{Tracks: tracks, Note: e.agent.Note, Later: e.agent.Later}, nil
}

func (e *Engine) DropLast() {
	if len(e.portions) == 0 {
		return
	}
	e.portions = e.portions[:len(e.portions)-1]
	if e.laterAsked {
		e.asked = e.asked[:len(e.asked)-1]
		e.laterAsked = false
	}
	e.session.DropLast()
	e.log.f("## portion %d dropped: listener reply", len(e.portions)+1)
}

func (e *Engine) Answer(q Question, raw string) {
	ans := resolveAnswer(q, raw)
	e.log.f("## listener answer: \"%s\" → %s", q.Question, ans)
	e.replies = append(e.replies, reply{after: len(e.portions), text: "to the question \"" + q.Question + "\" — " + ans})
}

func (e *Engine) history() string {
	seeds := make([]string, 0, len(e.session.plan.Seeds))
	for _, sd := range e.session.plan.Seeds {
		seeds = append(seeds, sd.Artist+" ("+sd.Mode+")")
	}
	if len(seeds) == 0 {
		seeds = append(seeds, "none")
	}
	excl := e.session.plan.Exclusions
	if len(excl) == 0 {
		excl = append(excl, "none")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Session: request \"%s\".\n", e.query)
	favMode := ""
	if e.session.plan.FromFavorites {
		favMode = "; favorites mode"
	}
	fmt.Fprintf(&b, "Plan is already set: referents: %s; exclusions: %s%s. set_plan — only if you change the direction.\n", strings.Join(seeds, ", "), strings.Join(excl, ", "), favMode)
	b.WriteString("Served (continuous numbering, as on the listener's screen):\n")
	n := 0
	for i, portion := range e.portions {
		fmt.Fprintf(&b, "Portion %d:\n", i+1)
		for _, t := range portion {
			n++
			fmt.Fprintf(&b, "%d. %s — %s — %s\n", n, t.Artist, t.Title, e.trackOutcome(t))
		}
	}
	if len(e.replies) > 0 {
		b.WriteString("Listener replies:\n")
		for _, r := range e.replies {
			fmt.Fprintf(&b, "- after portion %d: \"%s\"\n", r.after, r.text)
		}
	}
	if len(e.asked) > 0 {
		qs := make([]string, len(e.asked))
		for i, q := range e.asked {
			qs[i] = "\"" + q + "\""
		}
		fmt.Fprintf(&b, "Questions already asked (do not repeat): %s\n", strings.Join(qs, ", "))
	}
	if block := globalBlock(e.sig.globalSkips()); block != "" {
		b.WriteString("\n")
		b.WriteString(block)
	}
	if block := skippedBlock(e.sig.skippedElsewhere(e.sessionID)); block != "" {
		b.WriteString("\n")
		b.WriteString(block)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (e *Engine) trackOutcome(t Track) string {
	out := "not listened"
	if r, ok := e.sig.lastOutcome(e.sessionID, t.ID); ok {
		if r.Kind == "played" {
			out = "played through"
		} else {
			out = fmt.Sprintf("skipped at %d:%02d", r.PosSec/60, r.PosSec%60)
		}
	}
	switch e.sig.lastRating(e.sessionID, t.ID) {
	case "more":
		out += ", ▲ more of this"
	case "less":
		out += ", ▼ less of this"
	}
	return out
}

func globalBlock(stats []skipStat) string {
	if len(stats) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Lasting skips from past sessions (the code no longer lets these artists through):")
	for _, st := range stats {
		fmt.Fprintf(&b, "\n- %s: early skips %d in %d sessions, played through %d", st.Artist, st.Early, st.Sessions, st.Played)
		if len(st.Queries) > 0 {
			qs := make([]string, len(st.Queries))
			for i, q := range st.Queries {
				qs[i] = "\"" + q + "\""
			}
			fmt.Fprintf(&b, "; queries: %s", strings.Join(qs, ", "))
		}
	}
	b.WriteString("\nIf they share a sound or a direction — write it to memory as one entry (memory_add or memory_replace), evidence — these numbers. Do not write about single artists to memory.")
	return b.String()
}

func skippedBlock(songs []Song) string {
	if len(songs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("In past sessions the listener dropped these before the middle (the code will reject them, do not offer them):")
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
	for _, out := range outs {
		if out.Kind == "skip_early" || out.Kind == "skip_mid" {
			e.skipped = append(e.skipped, out.Artist+" — "+out.Title)
		}
	}
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
		e.log.f("signal: %s — %s: %s %d/%d s", out.Artist, out.Title, out.Kind, out.PosSec, out.DurSec)
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
	id, err := createPlaylist(ctx, e.api, name, "AUX · «"+e.query+"»", trackIDs(tracks))
	if err != nil {
		return "", err
	}
	e.log.f("playlist: %s (%d tracks)", name, len(tracks))
	return id, nil
}

func (e *Engine) AppendPlaylist(ctx context.Context, playlistID string, tracks []Track) error {
	if err := addToPlaylist(ctx, e.api, playlistID, trackIDs(tracks)); err != nil {
		return err
	}
	e.log.f("playlist +%d", len(tracks))
	return nil
}

func (e *Engine) Close() (logPath string, err error) {
	e.log.f("memory at end: %d/%d", e.mem.Size(), e.mem.Limit)
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
		Released:   en.Song.Released,
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
