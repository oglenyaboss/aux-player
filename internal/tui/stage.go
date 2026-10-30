package tui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net/http"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/lastfm"
	"github.com/oglenyaboss/aux-player/internal/openurl"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/tui/art"
	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

type stageScreen struct {
	st      *stage.Model
	host    *Model
	feed    []VibeAITrack // mirror of the bridge's r.feed: accepted picks, then the portion's final tracks
	open    int           // index in feed where the open portion starts
	out     []tea.Msg     // actions collected from OnAction, turned into commands
	cmds    []tea.Cmd     // commands for actions that need I/O, flushed beside the messages
	query   string
	ticking bool
	last    time.Time
	covers  map[string]bool // keys whose cover was requested
	want    []stageCover    // covers to fetch on the next tick

	initErr string // launch error the scene shows on the next tick
	lastErr string // host error already shown in the player, so a new one toasts once
	quit    bool   // the scene asked to leave the program; the tick chain stops
}

type stageCover struct{ key, url string }

func newStageScreen(host *Model) *stageScreen {
	s := &stageScreen{host: host, covers: map[string]bool{}}
	if lang := host.cfg.VibeLang; lang != "" {
		stage.SetLang(lang)
	} else {
		stage.SetLang(stage.DefaultLang())
	}
	opts := stageOptions(host.cfg, host.appleLogin != nil)
	opts.OnAction = s.act
	s.st = stage.New(opts)
	return s
}

// stageOptions reads the scene's opening state from the config: onboarding
// while the agent key is empty or the Apple sign-in waits for the ask,
// the steps already connected, the eggs opt-out.
func stageOptions(cfg *config.Config, appleLogin bool) stage.Options {
	apple := !appleLogin || cfg.AppleUserToken != ""
	return stage.Options{
		Onboard:       cfg.VibeLLMAPIKey == "" || !apple,
		Connected:     [3]bool{apple, cfg.VibeLLMAPIKey != "", cfg.LastfmSessionKey != ""},
		Slots:         vibeAIPortion,
		LastfmAskKeys: cfg.LastfmAPIKey == "" || cfg.LastfmAPISecret == "",
		LastfmKey:     cfg.LastfmAPIKey,
		NoEggs:        cfg.VibeEggs != nil && !*cfg.VibeEggs,
		Example:       func() string { return stage.Tr("в духе HARDSTONE PSYCHO", "like HARDSTONE PSYCHO") },
	}
}

// ── adapter surface: what the agent bridge calls ──────────────────────────

func (s *stageScreen) Query() string { return s.query }
func (s *stageScreen) View() string  { return s.st.View() }

// Frame renders the screen at full terminal size; Model.View() calls it while
// the screen is open.
func (s *stageScreen) Frame(w, h int) string {
	s.st.SetSize(w, h)
	return s.st.View()
}

func (s *stageScreen) AddProgress(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if rest, ok := strings.CutPrefix(line, "·"); ok {
		s.st.Progress(thinkingPhrase(strings.TrimSpace(rest)))
		return
	}
	s.st.Progress(line)
}

func (s *stageScreen) Ask(qs []VibeAIQuestion) {
	cards := make([]stage.Question, len(qs))
	for i, q := range qs {
		cards[i] = stageQuestion(q)
	}
	s.st.Ask(cards)
	for _, q := range qs {
		for _, t := range q.Tracks {
			s.wantCover(t)
		}
	}
}

func (s *stageScreen) AskLater(q VibeAIQuestion) {
	s.st.AskLater(stageQuestion(q))
	for _, t := range q.Tracks {
		s.wantCover(t)
	}
}

func (s *stageScreen) AddPick(t VibeAITrack, name string, accepted bool, reason string) {
	s.st.Candidate(stageTrack(t, name), accepted, reason)
	if accepted {
		s.feed = append(s.feed, t)
		s.wantCover(t)
	}
}

// PortionDone mirrors the bridge: the open part of the feed is replaced by the
// final tracks, the mirror index moves past them.
func (s *stageScreen) PortionDone(tracks []VibeAITrack, note string) {
	s.feed = append(s.feed[:s.open], tracks...)
	s.open = len(s.feed)
	cards := make([]stage.Track, len(tracks))
	for i, t := range tracks {
		cards[i] = stageTrack(t, "")
	}
	s.st.PortionDone(cards, note)
	for _, t := range tracks {
		s.wantCover(t)
	}
}

func (s *stageScreen) NextGathering(reply string) { s.st.NextGathering(reply) }

func (s *stageScreen) NextPick(t VibeAITrack) {
	s.st.NextPick(stageTrack(t, ""))
	s.wantCover(t)
}

func (s *stageScreen) NextReady(tracks []VibeAITrack) {
	cards := make([]stage.Track, len(tracks))
	for i, t := range tracks {
		cards[i] = stageTrack(t, "")
		s.wantCover(t)
	}
	s.st.NextReady(cards)
}

func (s *stageScreen) NextFailed() { s.st.NextFailed() }

func (s *stageScreen) NextClear() { s.st.NextClear() }

func (s *stageScreen) SetError(err error) {
	s.feed = s.feed[:s.open]
	s.st.Fail(err.Error())
}

func (s *stageScreen) SetNotice(text string) { s.st.Toast(text) }

// HandleKey hands the key to the scene first; keys the scene declines reach the
// AUX globals (pause, next, seek). Actions collected meanwhile ship as a cmd.
func (s *stageScreen) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	handled := s.st.Key(msg.String(), msg.Text)
	return s.flush(), handled
}

// Paste hands the scene a bracketed paste, which arrives apart from key presses.
func (s *stageScreen) Paste(text string) tea.Cmd {
	s.st.Paste(text)
	return s.flush()
}

// seekCmd seeks the AUX player: the scene declines left/right (they belong to
// the player, not the screen), and the host forwards them to the active panel.
func (s *stageScreen) seekCmd(right bool) tea.Cmd {
	h := s.host
	if h.playerState.Track == nil {
		return nil
	}
	step, dir := -10*time.Second, "←"
	if right {
		step, dir = 10*time.Second, "→"
	}
	pos := max(0, h.playerState.Position+step)
	if right {
		if dur := h.playerState.Track.Duration; dur > 0 && pos > dur {
			pos = dur
		}
	}
	h.appendLog(fmt.Sprintf("[player] seek %s %s", dir, FormatDuration(pos)))
	return h.playerCmd(func(p player.Player) error { return p.Seek(pos) })
}

// ── actions → messages ────────────────────────────────────────────────────

func (s *stageScreen) act(a stage.Action) {
	switch a := a.(type) {
	case stage.Query:
		s.query = a.Text
		s.feed, s.open = nil, 0
		s.out = append(s.out, VibeAISubmitMsg{Text: a.Text, Fresh: true})
	case stage.Answers:
		s.out = append(s.out, VibeAIAnswersMsg{Answers: a.Raw})
	case stage.Say:
		s.out = append(s.out, VibeAISubmitMsg{Text: a.Text})
	case stage.LaterPick:
		s.out = append(s.out, VibeAILaterMsg{Raw: strconv.Itoa(a.N)})
	case stage.Rate:
		if i, ok := s.rateTarget(); ok {
			rating := -1
			if a.Up {
				rating = 1
			}
			s.out = append(s.out, VibeAIRateMsg{Index: i, Rating: rating})
		}
	case stage.Save:
		s.out = append(s.out, VibeAIActionMsg{Action: VibeAISave})
	case stage.Restart:
		s.feed = s.feed[:s.open] // the bridge trims the same way on cancel
		s.out = append(s.out, VibeAICancelMsg{})
	case stage.Settings:
		c := stageOptions(s.host.cfg, s.host.appleLogin != nil).Connected
		c[1] = false
		s.st.Onboard(c)
	case stage.Eggs:
		on := a.On
		cfg := s.host.cfg
		cfg.VibeEggs = &on
		if err := cfg.Save(""); err != nil {
			s.host.appendLog(fmt.Sprintf("[vibe] config save error: %v", err))
		}
	case stage.ConnectApple:
		if s.host.appleLogin != nil {
			// The sign-in waits for the onboarding to ask; the answer lands
			// later through AppleLoginMsg, nothing ships right away. A buffer
			// of one keeps a repeated enter from queuing a second login.
			select {
			case s.host.appleLogin <- struct{}{}:
			default:
			}
			break
		}
		s.cmds = append(s.cmds, func() tea.Msg { return stageStepMsg{step: 0} })
	case stage.ConnectLastfm:
		cfg := s.host.cfg
		if a.Key != "" && a.Secret != "" {
			cfg.LastfmAPIKey, cfg.LastfmAPISecret = a.Key, a.Secret
			if err := cfg.Save(""); err != nil {
				s.host.appendLog(fmt.Sprintf("[vibe] config save error: %v", err))
			}
		}
		key, secret := cfg.LastfmAPIKey, cfg.LastfmAPISecret
		if key == "" || secret == "" {
			s.cmds = append(s.cmds, func() tea.Msg {
				return stageStepMsg{step: 2, err: stage.Tr("нет ключа приложения Last.fm", "no Last.fm app key")}
			})
			break
		}
		s.cmds = append(s.cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			session, err := stageLastfmAuth(ctx, key, secret)
			switch {
			case err == nil:
				return stageStepMsg{step: 2, session: session}
			case errors.Is(err, context.DeadlineExceeded):
				return stageStepMsg{step: 2, err: stage.Tr("не дождался ответа Last.fm", "Last.fm did not answer in time")}
			default:
				return stageStepMsg{step: 2, err: err.Error()}
			}
		})
	case stage.LoadModels:
		url, key := a.URL, a.Key
		s.cmds = append(s.cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			all, err := stageListModels(ctx, nil, url, key)
			return stageModelsReply(all, err)
		})
	case stage.Setup:
		cfg := s.host.cfg
		if a.URL != "" {
			cfg.VibeLLMBaseURL = a.URL
		}
		if a.Key != "" {
			cfg.VibeLLMAPIKey = a.Key
		}
		if a.Model != "" {
			cfg.VibeLLMModel = a.Model
		}
		cfg.VibeLLMEffort = a.Effort
		cfg.VibeLang = a.Lang
		if err := cfg.Save(""); err != nil {
			s.host.appendLog(fmt.Sprintf("[vibe] config save error: %v", err))
		}
	case stage.Quit:
		// The AUX is the whole app: leaving it leaves the program.
		s.quit = true
		s.ticking = false
		s.host.vibeAINoteLeave()
		s.host.vibeAI.close()
		if s.host.player != nil {
			_ = s.host.player.Close()
		}
		s.cmds = append(s.cmds, tea.Quit)
	}
}

// stageModelsReply maps the provider's answer onto the model step: only the
// models that can call tools, with deepseek-v4.1-flash marked as recommended.
func stageModelsReply(all []llm.ModelInfo, err error) stageModelsMsg {
	if err != nil {
		if errors.Is(err, llm.ErrUnauthorized) {
			return stageModelsMsg{err: stage.Tr("ключ не подошёл", "key rejected")}
		}
		return stageModelsMsg{err: stage.Tr("не достучался: ", "unreachable: ") + err.Error()}
	}
	var list []stage.ModelInfo
	for _, mi := range all {
		if mi.Tools || !mi.Known {
			list = append(list, stage.ModelInfo{ID: mi.ID, Reasoning: mi.Reasoning,
				Rec: strings.HasSuffix(mi.ID, "deepseek-v4.1-flash")})
		}
	}
	if len(list) == 0 {
		return stageModelsMsg{total: len(all),
			err: stage.Tr("у провайдера нет моделей с инструментами", "the provider has no models with tools")}
	}
	return stageModelsMsg{list: list, total: len(all)}
}

// rateTarget locates the playing vibe track in the mirror feed; without it the
// rating has nowhere to land.
func (s *stageScreen) rateTarget() (int, bool) {
	if s.host.playerState.Track == nil {
		return 0, false
	}
	id := PlaybackID(*s.host.playerState.Track)
	for i := len(s.feed) - 1; i >= 0; i-- {
		if s.feed[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

// flush drains the collected actions and I/O commands: a single item ships
// as itself, several as a batch; the tests unwrap both alike.
func (s *stageScreen) flush() tea.Cmd {
	msgs, cmds := s.out, s.cmds
	s.out, s.cmds = nil, nil
	if len(msgs)+len(cmds) == 0 {
		return nil
	}
	if len(msgs)+len(cmds) == 1 {
		if len(msgs) == 1 {
			return func() tea.Msg { return msgs[0] }
		}
		return cmds[0]
	}
	all := make([]tea.Cmd, 0, len(msgs)+len(cmds))
	for i := range msgs {
		all = append(all, func() tea.Msg { return msgs[i] })
	}
	return tea.Batch(append(all, cmds...)...)
}

// ── helpers moved from the old views screen ───────────────────────────────

func stageTrack(t VibeAITrack, name string) stage.Track {
	if t.ID == "" {
		artist, title, ok := strings.Cut(name, " — ")
		if !ok {
			return stage.Track{Title: name}
		}
		return stage.Track{Artist: artist, Title: title}
	}
	return stage.Track{ID: t.ID, Artist: t.Artist, Title: t.Title, Album: t.Album, Released: t.Released, Why: t.Why, Dur: t.Dur.Seconds()}
}

func stageQuestion(q VibeAIQuestion) stage.Question {
	tracks := make([]stage.Track, len(q.Tracks))
	for i, t := range q.Tracks {
		tracks[i] = stageTrack(t, "")
	}
	return stage.Question{Title: q.Question, Options: q.Options, Tracks: tracks, Def: questionDefault(q)}
}

func questionDefault(q VibeAIQuestion) int {
	if n, err := strconv.Atoi(q.Default); err == nil && n >= 1 && n <= len(q.Options) {
		return n - 1
	}
	return 0
}

func thinkingPhrase(rest string) string {
	name, args, _ := strings.Cut(rest, " ")
	switch name {
	case "describe":
		_, what, _ := strings.Cut(args, " ")
		if what == "" {
			what = args
		}
		return stage.Tr("читаю про ", "reading about ") + what
	case "set_plan":
		return stage.Tr("наметил план", "sketched a plan")
	case "similar_artists":
		return stage.Tr("смотрю соседей ", "checking neighbors of ") + args
	case "submit":
		return stage.Tr("проверяю в Apple Music…", "checking Apple Music…")
	case "ask_user":
		return stage.Tr("спросил тебя", "asked you")
	case "ask_later":
		return stage.Tr("придумал вопрос", "came up with a question")
	case "memory_add", "memory_replace":
		return stage.Tr("запомнил: ", "remembered: ") + args
	case "memory_remove":
		return stage.Tr("забыл: ", "forgot: ") + args
	case "memory_consolidate":
		return stage.Tr("навожу порядок в памяти", "tidying up memory")
	}
	return rest
}

// ── covers ────────────────────────────────────────────────────────────────

func (s *stageScreen) wantCover(t VibeAITrack) {
	if t.ArtworkURL == "" || s.covers[t.ID] {
		return
	}
	s.covers[t.ID] = true
	url := strings.NewReplacer("{w}", "300", "{h}", "300").Replace(t.ArtworkURL)
	s.want = append(s.want, stageCover{key: t.ID, url: url})
}

// coverCmds turns the queued cover keys into fetch commands and clears the queue;
// terminals without colour support skip the download entirely.
func (s *stageScreen) coverCmds(client *http.Client) tea.Cmd {
	if len(s.want) == 0 {
		return nil
	}
	wanted := s.want
	s.want = nil
	if g := s.host.supportsArtColor; g == nil || !g() {
		return nil
	}
	var cmds []tea.Cmd
	for _, c := range wanted {
		key, url := c.key, c.url
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			img, err := art.FetchAndDecode(ctx, client, url, 5<<20)
			return stageCoverMsg{key: key, img: img, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// ── ticks ─────────────────────────────────────────────────────────────────

type stageTickMsg time.Time

type stageCoverMsg struct {
	key string
	img image.Image
	err error
}

// stageStepMsg answers the Apple Music or Last.fm connect step: the session
// key on success, the failure text otherwise.
type stageStepMsg struct {
	step    int
	session string
	err     string
}

// stageModelsMsg answers LoadModels: the models that support tools, how many
// the provider has in all, or the failure for the key substep.
type stageModelsMsg struct {
	list  []stage.ModelInfo
	total int
	err   string
}

// stageListModels and stageLastfmAuth are the onboarding's I/O seams; the
// tests swap them so no test opens a browser or reaches the network.
var stageListModels = llm.ListModels

var stageLastfmAuth = func(ctx context.Context, key, secret string) (string, error) {
	return lastfmAuth(ctx, lastfm.NewClient(key, secret, ""), openurl.Open, 2*time.Second)
}

// lastfmAuther is the slice of the Last.fm client the onboarding needs.
type lastfmAuther interface {
	GetToken() (string, error)
	AuthorizeURL(token string) string
	GetSession(token string) (string, error)
}

// lastfmAuth opens the authorize page and polls for the session key until it
// shows up or the context runs out; an open error is ignored, the browser may
// not come up.
func lastfmAuth(ctx context.Context, c lastfmAuther, open func(string) error, every time.Duration) (string, error) {
	token, err := c.GetToken()
	if err != nil {
		return "", err
	}
	_ = open(c.AuthorizeURL(token))
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
			if session, err := c.GetSession(token); err == nil {
				return session, nil
			}
		}
	}
}

func stageTick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return stageTickMsg(t) })
}

// stageWake starts the 30 fps chain while the screen is open; glowTick calls it
// every 100 ms, the ticking flag keeps the chain from multiplying.
func (m *Model) stageWake() tea.Cmd {
	s := m.vibeAIP.m
	if s.ticking {
		return nil
	}
	s.ticking = true
	s.last = time.Now()
	return stageTick()
}
