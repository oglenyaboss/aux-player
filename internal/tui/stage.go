package tui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simone-vibes/vibez/internal/config"
	"github.com/simone-vibes/vibez/internal/lastfm"
	"github.com/simone-vibes/vibez/internal/openurl"
	"github.com/simone-vibes/vibez/internal/player"
	"github.com/simone-vibes/vibez/internal/stage"
	"github.com/simone-vibes/vibez/internal/tui/art"
	"github.com/simone-vibes/vibez/internal/tui/views"
	"github.com/simone-vibes/vibez/internal/vibeai/llm"
)

type stageScreen struct {
	st      *stage.Model
	host    *Model
	feed    []views.VibeAITrack // mirror of the bridge's r.feed: accepted picks, then the portion's final tracks
	open    int                 // index in feed where the open portion starts
	out     []tea.Msg           // actions collected from OnAction, turned into commands
	cmds    []tea.Cmd           // commands for actions that need I/O, flushed beside the messages
	query   string
	ticking bool
	last    time.Time
	covers  map[string]bool // keys whose cover was requested
	want    []stageCover    // covers to fetch on the next tick
}

type stageCover struct{ key, url string }

func newStageScreen(host *Model) *stageScreen {
	s := &stageScreen{host: host, covers: map[string]bool{}}
	if lang := host.cfg.VibeLang; lang != "" {
		stage.SetLang(lang)
	} else {
		stage.SetLang(stage.DefaultLang())
	}
	opts := stageOptions(host.cfg)
	opts.OnAction = s.act
	s.st = stage.New(opts)
	return s
}

// stageOptions reads the scene's opening state from the config: onboarding
// while the agent key is empty, the steps already connected, the eggs opt-out.
func stageOptions(cfg *config.Config) stage.Options {
	return stage.Options{
		Onboard:   cfg.VibeLLMAPIKey == "",
		Connected: [3]bool{true, false, cfg.LastfmSessionKey != ""},
		NoEggs:    cfg.VibeEggs != nil && !*cfg.VibeEggs,
		Example:   func() string { return stage.Tr("в духе HARDSTONE PSYCHO", "like HARDSTONE PSYCHO") },
	}
}

// ── adapter surface: what the agent bridge calls ──────────────────────────

func (s *stageScreen) SetSize(w, h int)       {}
func (s *stageScreen) SetStep(int)            {}
func (s *stageScreen) SetPlaylist(string)     {}
func (s *stageScreen) SetStream(bool)         {}
func (s *stageScreen) SetMemory([]string)     {}
func (s *stageScreen) SetPlaying(string)      {}
func (s *stageScreen) FollowPlaying(string)   {}
func (s *stageScreen) SetPlayed(string)       {}
func (s *stageScreen) SetSkipped(string, int) {}
func (s *stageScreen) Cancelled()             {}
func (s *stageScreen) SetWorking(string)      {}
func (s *stageScreen) Back() bool             { return false }
func (s *stageScreen) Query() string          { return s.query }
func (s *stageScreen) View() string           { return s.st.View() }

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

func (s *stageScreen) Ask(qs []views.VibeAIQuestion) {
	cards := make([]stage.Question, len(qs))
	for i, q := range qs {
		cards[i] = stageQuestion(q)
	}
	s.st.Ask(cards)
}

func (s *stageScreen) AskLater(q views.VibeAIQuestion) {
	s.st.AskLater(stageQuestion(q))
}

func (s *stageScreen) AddPick(t views.VibeAITrack, name string, accepted bool, reason string) {
	s.st.Candidate(stageTrack(t, name), accepted, reason)
	if accepted {
		s.feed = append(s.feed, t)
		s.wantCover(t)
	}
}

// PortionDone mirrors the bridge: the open part of the feed is replaced by the
// final tracks, the mirror index moves past them.
func (s *stageScreen) PortionDone(tracks []views.VibeAITrack, note string) {
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

func (s *stageScreen) SetError(err error) {
	s.feed = s.feed[:s.open]
	s.st.Fail(err.Error())
}

func (s *stageScreen) SetNotice(text string) { s.st.Toast(text) }

// HandleKey hands the key to the scene first; keys the scene declines reach the
// vibez globals (pause, next, seek). Actions collected meanwhile ship as a cmd.
func (s *stageScreen) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	handled := s.st.Key(msg.String(), msg.Text)
	return s.flush(), handled
}

// seekCmd seeks the vibez player: the scene declines left/right (they belong to
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
	h.appendLog(fmt.Sprintf("[player] seek %s %s", dir, views.FormatDuration(pos)))
	return h.playerCmd(func(p player.Player) error { return p.Seek(pos) })
}

// ── actions → messages ────────────────────────────────────────────────────

func (s *stageScreen) act(a stage.Action) {
	switch a := a.(type) {
	case stage.Query:
		s.query = a.Text
		s.feed, s.open = nil, 0
		s.out = append(s.out, views.VibeAISubmitMsg{Text: a.Text, Fresh: true})
	case stage.Answers:
		s.out = append(s.out, views.VibeAIAnswersMsg{Answers: a.Raw})
	case stage.Say:
		s.out = append(s.out, views.VibeAISubmitMsg{Text: a.Text})
	case stage.LaterPick:
		s.out = append(s.out, views.VibeAILaterMsg{Raw: strconv.Itoa(a.N)})
	case stage.Rate:
		if i, ok := s.rateTarget(); ok {
			rating := -1
			if a.Up {
				rating = 1
			}
			s.out = append(s.out, views.VibeAIRateMsg{Index: i, Rating: rating})
		}
	case stage.Save:
		s.out = append(s.out, views.VibeAIActionMsg{Action: views.VibeAISave})
	case stage.Restart:
		s.feed = s.feed[:s.open] // the bridge trims the same way on cancel
		s.out = append(s.out, views.VibeAICancelMsg{})
	case stage.ConnectApple:
		s.cmds = append(s.cmds, func() tea.Msg { return stageStepMsg{step: 0} })
	case stage.ConnectLastfm:
		key, secret := s.host.cfg.LastfmAPIKey, s.host.cfg.LastfmAPISecret
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
		s.host.activePanel = -1
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
	id := views.PlaybackID(*s.host.playerState.Track)
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

func stageTrack(t views.VibeAITrack, name string) stage.Track {
	if t.ID == "" {
		artist, title, ok := strings.Cut(name, " — ")
		if !ok {
			return stage.Track{Title: name}
		}
		return stage.Track{Artist: artist, Title: title}
	}
	return stage.Track{ID: t.ID, Artist: t.Artist, Title: t.Title, Why: t.Why, Dur: t.Dur.Seconds()}
}

func stageQuestion(q views.VibeAIQuestion) stage.Question {
	def := questionDefault(q)
	if q.Kind == "scale" {
		def--
	}
	return stage.Question{Title: q.Question, Options: q.Options, Scale: q.Kind == "scale",
		Left: q.Left, Right: q.Right, Steps: q.Steps, Def: def}
}

func questionDefault(q views.VibeAIQuestion) int {
	if q.Kind == "scale" {
		n, err := strconv.Atoi(q.Default)
		if err != nil || n < 1 || n > q.Steps {
			return (q.Steps + 1) / 2
		}
		return n
	}
	if i := slices.Index(q.Options, q.Default); i >= 0 {
		return i
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
		return "читаю про " + what
	case "set_plan":
		return "наметил план"
	case "similar_artists":
		return "смотрю соседей " + args
	case "submit":
		return "проверяю в Apple Music…"
	case "ask_user":
		return "спросил тебя"
	case "ask_later":
		return "придумал вопрос"
	case "memory_add", "memory_replace":
		return "запомнил: " + args
	case "memory_remove":
		return "забыл: " + args
	case "memory_consolidate":
		return "навожу порядок в памяти"
	}
	return rest
}

// ── covers ────────────────────────────────────────────────────────────────

func (s *stageScreen) wantCover(t views.VibeAITrack) {
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
	if !m.vibeAIActive() || s.ticking {
		return nil
	}
	s.ticking = true
	s.last = time.Now()
	return stageTick()
}
