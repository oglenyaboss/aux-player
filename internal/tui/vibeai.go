package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simone-vibes/vibez/internal/config"
	"github.com/simone-vibes/vibez/internal/player"
	"github.com/simone-vibes/vibez/internal/provider"
	"github.com/simone-vibes/vibez/internal/stage"
	"github.com/simone-vibes/vibez/internal/tui/views"
	"github.com/simone-vibes/vibez/internal/vibeai"
	"github.com/simone-vibes/vibez/internal/vibeai/llm"
	"github.com/simone-vibes/vibez/internal/vibeai/memory"
)

type vibeAIPanel struct{ m *stageScreen }

func (p *vibeAIPanel) NavKey() string   { return "v" }
func (p *vibeAIPanel) NavLabel() string { return "vibe" }
func (p *vibeAIPanel) SetSize(w, h int) { p.m.SetSize(w, h) }
func (p *vibeAIPanel) Update(msg tea.KeyPressMsg) tea.Cmd {
	if k := msg.String(); k == "left" || k == "right" {
		return p.m.seekCmd(k == "right")
	}
	return nil
}
func (p *vibeAIPanel) View() string { return p.m.View() }
func (p *vibeAIPanel) Back() bool   { return p.m.Back() }

type vibeAIProgressMsg struct {
	gen  int
	line string
}

type vibeAIAskMsg struct {
	gen int
	qs  []vibeai.Question
}

type vibeAIPickMsg struct {
	gen  int
	pick vibeai.Pick
}

type vibeAIDoneMsg struct {
	gen     int
	tracks  []vibeai.Track
	note    string
	later   *vibeai.Question
	err     error
	elapsed time.Duration
	refill  bool
}

type vibeAISavedMsg struct {
	id, name string
	n        int
	created  bool
	err      error
}

// vibeAIRun is the agent session behind the vibe screen. The engine runs in
// command goroutines; mu serialises them, everything else is owned by Update.
type vibeAIRun struct {
	mu      sync.Mutex
	engine  *vibeai.Engine
	turnCtx context.Context
	turnGen int

	events  chan tea.Msg
	answers chan []string
	cancel  context.CancelFunc
	gen     int
	busy    bool
	refill  bool

	tracks       []vibeai.Track
	feed         []vibeai.Track
	portionStart int
	served       map[string]vibeai.Track
	pending      []vibeai.Outcome
	pendingIDs   []string
	heardID      string
	heardPos     time.Duration
	portion      int
	playlistID   string
	playlistName string
	savedPortion int
	autoplay     bool
	autoplayID   string
	later        *vibeai.Question
	laterAnswers []vibeaiLaterAnswer
	stream       bool
	streamForID  string
}

type vibeaiLaterAnswer struct {
	q   vibeai.Question
	raw string
}

func newVibeAIRun() *vibeAIRun {
	return &vibeAIRun{events: make(chan tea.Msg, 64), answers: make(chan []string), served: make(map[string]vibeai.Track), stream: true}
}

type vibeAIBridge struct{ r *vibeAIRun }

func (b vibeAIBridge) Progress(line string) {
	b.r.events <- vibeAIProgressMsg{gen: b.r.turnGen, line: line}
}

func (b vibeAIBridge) Ask(qs []vibeai.Question) []string {
	b.r.events <- vibeAIAskMsg{gen: b.r.turnGen, qs: qs}
	select {
	case a := <-b.r.answers:
		return a
	case <-b.r.turnCtx.Done():
		return nil
	}
}

func (b vibeAIBridge) Picked(p vibeai.Pick) {
	b.r.events <- vibeAIPickMsg{gen: b.r.turnGen, pick: p}
}

func waitVibeAI(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func vibeAIConfig(cfg *config.Config) vibeai.Config {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	cacheDir = filepath.Join(cacheDir, "vibez", "vibe")
	memDir := cacheDir
	if p, err := config.ConfigPath(""); err == nil {
		memDir = filepath.Dir(p)
	}
	return vibeai.Config{
		AppleDeveloperToken: cfg.AppleDeveloperToken,
		AppleUserToken:      cfg.AppleUserToken,
		Storefront:          cfg.StoreFront,
		LastfmKey:           cfg.LastfmAPIKey,
		LLM:                 llm.Config{BaseURL: cfg.VibeLLMBaseURL, APIKey: cfg.VibeLLMAPIKey, Model: cfg.VibeLLMModel, Effort: cfg.VibeLLMEffort},
		MemoryPath:          filepath.Join(memDir, "vibe-memory.md"),
		SignalsPath:         filepath.Join(memDir, "vibe-signals.jsonl"),
		CacheDir:            cacheDir,
		RunsDir:             filepath.Join(cacheDir, "runs"),
	}
}

// vibeAITurnCmd runs one agent turn. fresh closes the previous session and
// starts a new one for a new request (and arms one-shot autoplay of the first
// accepted candidate); text "" continues the flow; refill marks a stream
// top-up whose tracks go to the end of the queue instead of the screen's play
// actions. Listening outcomes accumulated since the last turn and answers to
// a mid-stream question asked earlier in the same session are handed to the
// engine before it turns.
func (m *Model) vibeAITurnCmd(text string, fresh, refill bool) tea.Cmd {
	r := m.vibeAI
	if r.cancel != nil {
		r.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.gen++
	r.busy = true
	r.refill = refill
	gen := r.gen
	pending := r.pending
	r.pending = nil
	cfg := vibeAIConfig(m.cfg)
	if fresh {
		r.tracks, r.portion, r.feed = nil, 0, nil
		r.playlistID, r.playlistName, r.savedPortion = "", "", 0
		m.vibeAIP.m.SetPlaylist("")
		r.autoplay, r.autoplayID, r.later, r.laterAnswers = true, "", nil, nil
	} else {
		r.autoplay = false
	}
	r.portionStart = len(r.feed)
	answers := r.laterAnswers
	r.laterAnswers = nil
	m.appendLog(fmt.Sprintf("[vibe] turn %q (fresh=%v refill=%v)", text, fresh, refill))
	return func() tea.Msg {
		start := time.Now()
		r.mu.Lock()
		defer r.mu.Unlock()
		r.turnCtx, r.turnGen = ctx, gen
		if fresh && r.engine != nil {
			if len(pending) > 0 {
				_ = r.engine.Record(pending)
				pending = nil
			}
			_, _ = r.engine.Close()
			r.engine = nil
		}
		if r.engine == nil {
			e, err := vibeai.Start(ctx, cfg, vibeAIBridge{r})
			if err != nil {
				return vibeAIDoneMsg{gen: gen, err: err, refill: refill}
			}
			r.engine = e
		}
		if len(pending) > 0 {
			_ = r.engine.Record(pending)
		}
		for _, a := range answers {
			r.engine.Answer(a.q, a.raw)
		}
		p, err := r.engine.Turn(ctx, text)
		return vibeAIDoneMsg{gen: gen, tracks: p.Tracks, note: p.Note, later: p.Later, err: err, elapsed: time.Since(start), refill: refill}
	}
}

// close saves the session log on quit; a turn still in flight is cancelled
// and waited for briefly, its log dropped only if it does not unwind in time.
func (r *vibeAIRun) close() {
	if r.cancel != nil {
		r.cancel()
	}
	deadline := time.Now().Add(vibeAICloseWait)
	for !r.mu.TryLock() {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer r.mu.Unlock()
	if r.engine != nil {
		if len(r.pending) > 0 {
			_ = r.engine.Record(r.pending)
			r.pending = nil
		}
		_, _ = r.engine.Close()
		r.engine = nil
	}
}

func (m *Model) cancelVibeAI() {
	r := m.vibeAI
	if r.cancel != nil {
		r.cancel()
	}
	r.gen++
	r.busy = false
	if len(r.feed) > r.portionStart {
		r.feed = r.feed[:r.portionStart]
	}
	r.autoplay = false
	m.vibeAIP.m.Cancelled()
	m.appendLog("[vibe] cancelled")
}

func (m *Model) handleVibeAIMsg(msg tea.Msg) tea.Cmd {
	r := m.vibeAI
	v := m.vibeAIP.m
	switch msg := msg.(type) {
	case stageTickMsg:
		now := time.Time(msg)
		if !m.vibeAIActive() {
			v.ticking = false
			return nil
		}
		dt := now.Sub(v.last)
		if dt > 100*time.Millisecond {
			dt = 100 * time.Millisecond
		}
		v.last = now
		if e := v.initErr; e != "" {
			switch v.st.Phase() {
			case stage.PhIntro:
				v.st.Fail(e)
				v.initErr = ""
			case stage.PhPlay:
				v.st.Toast(e)
				v.initErr = ""
			}
		}
		if m.auxOnly && m.errMsg != v.lastErr {
			v.lastErr = m.errMsg
			if m.errMsg != "" && v.st.Phase() == stage.PhPlay {
				v.st.Toast(m.errMsg)
			}
		}
		if m.playerState.Track != nil {
			v.st.Now(views.PlaybackID(*m.playerState.Track), m.playerState.Position.Seconds(), m.playerState.Playing)
		}
		v.st.Step(dt.Seconds())
		return tea.Batch(v.flush(), v.coverCmds(m.artHTTP), stageTick())

	case stageCoverMsg:
		if msg.err == nil && msg.img != nil {
			v.st.SetCover(msg.key, msg.img)
		}
		return nil

	case stageStepMsg:
		if msg.session != "" {
			m.cfg.LastfmSessionKey = msg.session
			if err := m.cfg.Save(""); err != nil {
				m.appendLog(fmt.Sprintf("[vibe] config save error: %v", err))
			}
			if m.lastfmConnected != nil {
				select {
				case m.lastfmConnected <- struct{}{}:
				default:
				}
			}
		}
		v.st.StepDone(msg.step, msg.err)
		return nil

	case stageModelsMsg:
		v.st.Models(msg.list, msg.total, msg.err)
		return nil

	case AppleLoginMsg:
		errText := ""
		if msg.Err != nil {
			errText = msg.Err.Error()
		}
		v.st.StepDone(0, errText)
		return nil

	case vibeAIProgressMsg:
		if msg.gen == r.gen {
			v.AddProgress(msg.line)
		}
		return waitVibeAI(r.events)

	case vibeAIPickMsg:
		if msg.gen != r.gen || !r.busy {
			return waitVibeAI(r.events)
		}
		v.AddPick(vibeAIViewTrack(msg.pick.Track), msg.pick.Name, msg.pick.Accepted, msg.pick.Reason)
		if msg.pick.Accepted {
			r.feed = append(r.feed, msg.pick.Track)
			r.served[msg.pick.Track.ID] = msg.pick.Track
			if r.autoplay && !r.refill && r.autoplayID == "" {
				r.autoplayID = msg.pick.Track.ID
				return tea.Batch(waitVibeAI(r.events), m.vibeAIPlay([]vibeai.Track{msg.pick.Track}))
			}
		}
		return waitVibeAI(r.events)

	case vibeAIAskMsg:
		if msg.gen != r.gen || !r.busy {
			return waitVibeAI(r.events)
		}
		qs := make([]views.VibeAIQuestion, len(msg.qs))
		for i, q := range msg.qs {
			qs[i] = vibeAIViewQuestion(q)
		}
		v.Ask(qs)
		if !m.vibeAIActive() {
			m.errMsg = "✦ vibe спрашивает — нажми v"
			m.errExpiry = time.Now().Add(10 * time.Second)
		}
		return waitVibeAI(r.events)

	case views.VibeAIAnswersMsg:
		select {
		case r.answers <- msg.Answers:
		default:
		}

	case views.VibeAILaterMsg:
		if r.later != nil {
			r.laterAnswers = append(r.laterAnswers, vibeaiLaterAnswer{q: *r.later, raw: msg.Raw})
			r.later = nil
			m.appendLog("[vibe] later answer " + msg.Raw)
		}

	case views.VibeAISubmitMsg:
		m.vibeAIP.m.SetWorking("")
		return m.vibeAITurnCmd(msg.Text, msg.Fresh, false)

	case views.VibeAICancelMsg:
		m.cancelVibeAI()

	case vibeAIDoneMsg:
		if msg.gen != r.gen {
			return nil
		}
		r.busy = false
		if msg.err != nil {
			if len(r.feed) > r.portionStart {
				r.feed = r.feed[:r.portionStart]
			}
			r.autoplay = false
			m.appendLog(fmt.Sprintf("[vibe] error: %v", msg.err))
			v.SetError(msg.err)
			return nil
		}
		r.tracks = msg.tracks
		r.portion++
		for _, t := range msg.tracks {
			r.served[t.ID] = t
		}
		m.appendLog(fmt.Sprintf("[vibe] portion %d: %d tracks in %s", r.portion, len(msg.tracks), msg.elapsed.Round(time.Second)))
		r.feed = append(r.feed[:r.portionStart], msg.tracks...)
		v.PortionDone(vibeAIViewTracks(msg.tracks), msg.note)
		v.SetMemory(vibeAIMemoryLines(m.cfg))
		if msg.later != nil {
			r.later = msg.later
			v.AskLater(vibeAIViewQuestion(*msg.later))
		}
		if r.autoplay {
			r.autoplay = false
			if r.autoplayID == "" {
				return m.vibeAIPlay(msg.tracks)
			}
			rest := make([]vibeai.Track, 0, len(msg.tracks))
			for _, t := range msg.tracks {
				if t.ID != r.autoplayID {
					rest = append(rest, t)
				}
			}
			return m.vibeAIAppend(rest)
		}
		if msg.refill || r.stream {
			cmds := []tea.Cmd{m.vibeAIAppend(msg.tracks)}
			if msg.refill && r.playlistID != "" {
				cmds = append(cmds, m.vibeAISaveCmd())
			}
			return tea.Batch(cmds...)
		}

	case views.VibeAIActionMsg:
		if len(r.feed) == 0 {
			return nil
		}
		switch msg.Action {
		case views.VibeAIPlay:
			if msg.Index < 0 || msg.Index >= len(r.feed) {
				return nil
			}
			return m.vibeAIPlay(r.feed[msg.Index:])
		case views.VibeAIQueue:
			v.SetNotice(fmt.Sprintf(stage.Tr("+%d в очередь", "+%d queued"), len(r.tracks)))
			return m.vibeAIAppend(r.tracks)
		case views.VibeAISave:
			if r.savedPortion == r.portion {
				v.SetNotice(stage.Tr("эта порция уже в ♫ ", "this batch is already in ♫ ") + r.playlistName)
				return nil
			}
			v.SetNotice(stage.Tr("сохраняю в Apple Music…", "saving to Apple Music…"))
			return m.vibeAISaveCmd()
		case views.VibeAIStream:
			r.stream = !r.stream
			r.streamForID = ""
			v.SetStream(r.stream)
			if r.stream {
				v.SetNotice(stage.Tr("поток включён: новые порции сами встают в очередь", "stream on: new batches join the queue"))
			} else {
				v.SetNotice(stage.Tr("поток выключен", "stream off"))
			}
		}

	case views.VibeAIRateMsg:
		if msg.Index < 0 || msg.Index >= len(r.feed) {
			return nil
		}
		t := r.feed[msg.Index]
		kind := "rate_clear"
		switch msg.Rating {
		case 1:
			kind = "more"
		case -1:
			kind = "less"
		default:
			v.SetNotice(stage.Tr("оценка снята", "rating cleared"))
		}
		r.pending = append(r.pending, vibeai.Outcome{ID: t.ID, Artist: t.Artist, Title: t.Title, Kind: kind})
		m.appendLog(fmt.Sprintf("[vibe] %s — %s: %s", t.Artist, t.Title, kind))
		if msg.Rating == -1 && t.ID == r.heardID {
			r.heardID, r.heardPos = "", 0
			m.appendLog("[player] next")
			m.playerState.Loading = true
			return m.playerCmd(func(p player.Player) error { return p.Next() })
		}
		return nil

	case vibeAISavedMsg:
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[vibe] playlist error: %v", msg.err))
			v.SetNotice(stage.Tr("✗ плейлист: ", "✗ playlist: ") + msg.err.Error())
			return nil
		}
		r.playlistID, r.playlistName = msg.id, msg.name
		v.SetPlaylist(msg.name)
		if msg.created {
			v.SetNotice(fmt.Sprintf(stage.Tr("создан ♫ %s · %d треков", "created ♫ %s · %d tracks"), msg.name, msg.n))
		} else {
			v.SetNotice(fmt.Sprintf("+%d → ♫ %s", msg.n, msg.name))
		}
		m.appendLog(fmt.Sprintf("[vibe] playlist %q +%d", msg.name, msg.n))
	}
	return nil
}

// vibeAIStreamTick starts a top-up turn while stream mode is on and the
// currently playing vibe track has vibeAIStreamAhead or fewer tracks left
// behind it in the queue.
func (m *Model) vibeAIStreamTick(s player.State) tea.Cmd {
	r := m.vibeAI
	if !r.stream || r.busy || s.Track == nil || r.streamForID == s.Track.ID {
		return nil
	}
	if _, ok := r.served[views.PlaybackID(*s.Track)]; !ok {
		return nil
	}
	pid := views.PlaybackID(*s.Track)
	idx := -1
	for i := len(m.queueTracks) - 1; i >= 0; i-- {
		if views.PlaybackID(m.queueTracks[i]) == pid {
			idx = i
			break
		}
	}
	if idx < 0 || len(m.queueTracks)-1-idx > vibeAIStreamAhead {
		return nil
	}
	r.streamForID = s.Track.ID
	m.vibeAIP.m.SetWorking("")
	return m.vibeAITurnCmd("", false, true)
}

const vibeAIStreamAhead = 4

const vibeAICloseWait = 3 * time.Second

func vibeAIOutcome(t vibeai.Track, pos time.Duration) vibeai.Outcome {
	posSec := int(pos.Seconds())
	durSec := t.DurationMs / 1000
	kind := "skip_mid"
	switch {
	case (durSec > 0 && posSec*10 >= durSec*7) || (durSec == 0 && posSec >= 120):
		kind = "played"
	case posSec < 30:
		kind = "skip_early"
	}
	return vibeai.Outcome{ID: t.ID, Artist: t.Artist, Title: t.Title, Kind: kind, PosSec: posSec, DurSec: durSec}
}

// vibeAIOnState marks the playing track in the feed when it came from the
// vibe agent.
func (m *Model) vibeAIOnState(s player.State) {
	if s.Track == nil {
		return
	}
	r := m.vibeAI
	id := views.PlaybackID(*s.Track)
	changed := id != r.heardID
	if changed {
		m.vibeAINoteLeave()
		r.heardID, r.heardPos = id, 0
	}
	if s.Position > 0 {
		r.heardPos = s.Position
	}
	if _, ok := r.served[id]; ok {
		m.vibeAIP.m.SetPlaying(id)
		if changed {
			m.vibeAIP.m.FollowPlaying(id)
		}
	}
}

// vibeAINoteLeave writes the outcome for the heard track captured by vibeAIOnState, resetting it before any return.
func (m *Model) vibeAINoteLeave() {
	r := m.vibeAI
	id, pos := r.heardID, r.heardPos
	r.heardID, r.heardPos = "", 0
	if id == "" || pos < time.Second {
		return
	}
	t, ok := r.served[id]
	if !ok {
		return
	}
	out := vibeAIOutcome(t, pos)
	r.pending = append(r.pending, out)
	if out.Kind == "played" {
		m.vibeAIP.m.SetPlayed(t.ID)
	} else {
		m.vibeAIP.m.SetSkipped(t.ID, out.PosSec)
	}
	m.appendLog(fmt.Sprintf("[vibe] %s — %s: %s at %ds", t.Artist, t.Title, out.Kind, out.PosSec))
}

// vibeAIPlay replaces the player queue with ts and starts the first track.
func (m *Model) vibeAIPlay(ts []vibeai.Track) tea.Cmd {
	if len(ts) == 0 {
		return nil
	}
	tracks := vibeAIProviderTracks(ts)
	ids := make([]string, len(tracks))
	for i, t := range tracks {
		ids[i] = views.PlaybackID(t)
	}
	m.appendLog(fmt.Sprintf("[vibe] play %d tracks from %s — %s", len(tracks), tracks[0].Artist, tracks[0].Title))
	m.queueTracks = tracks
	m.queueIDs = ids
	first := tracks[0]
	m.playerState.Track = &first
	m.playerState.Loading = true
	m.playerState.Playing = false
	m.playerState.Position = 0
	m.queue.SetTracks(m.queueTracks)
	m.vibeAI.streamForID = ""
	if m.player == nil {
		m.vibeAI.pendingIDs = ids
		return nil
	}
	return m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
}

// vibeAIAppend puts ts at the end of the player queue.
func (m *Model) vibeAIAppend(ts []vibeai.Track) tea.Cmd {
	if len(ts) == 0 {
		return nil
	}
	tracks := vibeAIProviderTracks(ts)
	ids := make([]string, len(tracks))
	for i, t := range tracks {
		ids[i] = views.PlaybackID(t)
	}
	m.queueTracks = append(m.queueTracks, tracks...)
	m.queueIDs = append(m.queueIDs, ids...)
	m.queue.SetTracks(m.queueTracks)
	if m.player == nil {
		m.vibeAI.pendingIDs = append(m.vibeAI.pendingIDs, ids...)
		return nil
	}
	return m.playerCmd(func(p player.Player) error { return p.AppendQueue(ids) })
}

// vibeAIFlushPending hands the player the queue that arrived before it was ready.
func (m *Model) vibeAIFlushPending() tea.Cmd {
	ids := m.vibeAI.pendingIDs
	if len(ids) == 0 || m.player == nil {
		return nil
	}
	m.vibeAI.pendingIDs = nil
	return m.playerCmd(func(p player.Player) error { return p.SetQueue(ids) })
}

func (m *Model) vibeAISaveCmd() tea.Cmd {
	r := m.vibeAI
	tracks := r.tracks
	id, name := r.playlistID, r.playlistName
	if id == "" {
		name = vibeAIPlaylistName(m.vibeAIP.m.Query())
	}
	r.savedPortion = r.portion
	return func() tea.Msg {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.engine == nil {
			return vibeAISavedMsg{err: fmt.Errorf("no vibe session")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if id == "" {
			newID, err := r.engine.SavePlaylist(ctx, name, tracks)
			return vibeAISavedMsg{id: newID, name: name, n: len(tracks), created: true, err: err}
		}
		err := r.engine.AppendPlaylist(ctx, id, tracks)
		return vibeAISavedMsg{id: id, name: name, n: len(tracks), err: err}
	}
}

func vibeAIPlaylistName(query string) string {
	q := []rune(strings.TrimSpace(query))
	if len(q) > 40 {
		q = append(q[:39], '…')
	}
	return "Vibes AI · " + string(q)
}

func vibeAIViewTrack(t vibeai.Track) views.VibeAITrack {
	return views.VibeAITrack{ID: t.ID, Artist: t.Artist, Title: t.Title, Album: t.Album, Year: t.Year, Why: t.Why, Favorite: t.Favorite,
		ArtworkURL: t.ArtworkURL, Dur: time.Duration(t.DurationMs) * time.Millisecond}
}

func vibeAIViewTracks(ts []vibeai.Track) []views.VibeAITrack {
	out := make([]views.VibeAITrack, len(ts))
	for i, t := range ts {
		out[i] = vibeAIViewTrack(t)
	}
	return out
}

func vibeAIViewQuestion(q vibeai.Question) views.VibeAIQuestion {
	return views.VibeAIQuestion{Question: q.Question, Kind: q.Kind, Options: q.Options, Left: q.Left, Right: q.Right, Steps: q.Steps, Default: q.Default}
}

func vibeAIMemoryLines(cfg *config.Config) []string {
	store, err := memory.Open(vibeAIConfig(cfg).MemoryPath, memory.DefaultLimit)
	if err != nil {
		return nil
	}
	entries := store.Entries()
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.Text
	}
	return lines
}

func vibeAIProviderTracks(ts []vibeai.Track) []provider.Track {
	out := make([]provider.Track, len(ts))
	for i, t := range ts {
		art := strings.NewReplacer("{w}", "300", "{h}", "300").Replace(t.ArtworkURL)
		out[i] = provider.Track{
			ID:         t.ID,
			CatalogID:  t.ID,
			Title:      t.Title,
			Artist:     t.Artist,
			Album:      t.Album,
			Duration:   time.Duration(t.DurationMs) * time.Millisecond,
			ArtworkURL: art,
		}
	}
	return out
}

func (m *Model) vibeAIActive() bool {
	return m.activePanel >= 0 && m.panels[m.activePanel] == m.vibeAIP
}
