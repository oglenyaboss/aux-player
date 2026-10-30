package tui

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/tui/art"
)

// ── Messages ──────────────────────────────────────────────────────────────

type playerStateMsg player.State
type tickMsg time.Time
type glowTickMsg time.Time
type errMsg struct{ err error }

type SessionExpiredMsg struct{}
type SessionRestoredMsg struct{}

// ── Model ─────────────────────────────────────────────────────────────────

// Model hosts the AUX scene: it owns the player bridge, the agent bridge and
// the screen, renders the scene full-screen and forwards keys to it.
type Model struct {
	cfg      *config.Config
	provider provider.Provider
	player   player.Player

	width, height int

	playerState player.State
	stateCh     <-chan player.State

	queueIDs    []string         // current playback queue IDs
	queueTracks []provider.Track // full track objects for the agent's stream top-up

	artHTTP          *http.Client
	supportsArtColor func() bool

	// Agent session and the scene that draws it.
	vibeAI  *vibeAIRun
	vibeAIP *vibeAIPanel

	// appleLogin carries the asks from the Apple Music onboarding step;
	// lastfmConnected signals a freshly connected Last.fm session.
	appleLogin      chan<- struct{}
	lastfmConnected chan<- struct{}

	// Errors / status messages
	errMsg    string
	errExpiry time.Time

	// Debug log
	debugLog []string
	logW     io.Writer
}

func New(cfg *config.Config, prov provider.Provider, plyr player.Player, opts Options) *Model {
	m := &Model{
		cfg:      cfg,
		provider: prov,
		player:   plyr,
		artHTTP:  &http.Client{Timeout: 5 * time.Second},
		// Album covers need at least a 256-colour terminal to look reasonable;
		// on 16-colour/ASCII terminals the scene skips the download entirely.
		supportsArtColor: art.SupportsColor,
		appleLogin:       opts.AppleLogin,
		lastfmConnected:  opts.LastfmConnected,
	}
	if plyr != nil {
		m.stateCh = plyr.Subscribe()
	}
	m.vibeAI = newVibeAIRun()
	m.vibeAIP = &vibeAIPanel{m: newStageScreen(m)}
	m.logW = opts.DebugLog
	for _, line := range opts.InitialLogs {
		m.appendLog(line)
	}
	if opts.Backend != "" {
		m.appendLog("[engine] backend: " + opts.Backend)
	}
	if opts.ScanNotice != "" {
		m.appendLog("[local] " + opts.ScanNotice)
		if len(opts.InitialTracks) == 0 {
			m.errMsg = opts.ScanNotice
			m.errExpiry = time.Now().Add(30 * time.Second)
		}
	}
	if len(opts.InitialTracks) > 0 {
		ids := make([]string, len(opts.InitialTracks))
		for i, t := range opts.InitialTracks {
			ids[i] = PlaybackID(t)
		}
		m.queueTracks = append([]provider.Track{}, opts.InitialTracks...)
		m.queueIDs = ids
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tick(),
		glowTick(),
		waitVibeAI(m.vibeAI.events),
	}
	if m.stateCh != nil {
		cmds = append(cmds, waitForState(m.stateCh))
	}
	if m.player != nil {
		cmds = append(cmds, m.restoreAudioSettings()...)
	}
	return tea.Batch(cmds...)
}

func (m *Model) restoreAudioSettings() []tea.Cmd {
	var cmds []tea.Cmd
	if m.cfg.Volume != nil {
		v := m.cfg.VolumeOrDefault()
		cmds = append(cmds, m.playerCmd(func(p player.Player) error {
			return p.SetVolume(v)
		}))
		m.appendLog(fmt.Sprintf("[vol] restored %.0f%% from config", v*100))
	}
	if len(m.cfg.EQBands) > 0 {
		bands := configEQBandsToPlayer(m.cfg.EQBands)
		cmds = append(cmds, m.playerCmd(func(p player.Player) error {
			return p.SetEqualizer(bands)
		}))
		m.appendLog("[eq] restored from config")
	}
	return cmds
}

// ── Timers ────────────────────────────────────────────────────────────────

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func glowTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return glowTickMsg(t) })
}

func waitForState(ch <-chan player.State) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-ch
		if !ok {
			return nil
		}
		// Drain any additional buffered states, keeping only the most recent.
		// During a track transition the player fires a rapid burst of events
		// (paused → buffering → playing). Processing each one floods the event
		// loop and starves glowTick, causing the animation to freeze. Collapsing
		// the burst into a single Update cycle keeps the UI responsive.
		for {
			select {
			case newer, ok := <-ch:
				if !ok {
					return playerStateMsg(s)
				}
				s = newer
			default:
				return playerStateMsg(s)
			}
		}
	}
}

// ── Update ────────────────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case vibeAIProgressMsg, vibeAIAskMsg, vibeAIPickMsg, vibeAIDoneMsg, vibeAISavedMsg, stageTickMsg, stageCoverMsg, stageStepMsg, stageModelsMsg, AppleLoginMsg,
		VibeAIAnswersMsg, VibeAISubmitMsg, VibeAICancelMsg, VibeAIActionMsg, VibeAILaterMsg, VibeAIRateMsg:
		cmds = append(cmds, m.handleVibeAIMsg(msg))

	case tickMsg:
		if m.errMsg != "" && time.Now().After(m.errExpiry) {
			m.errMsg = ""
		}
		cmds = append(cmds, tick())

	case glowTickMsg:
		cmds = append(cmds, glowTick())
		cmds = append(cmds, m.stageWake())

	case playerStateMsg:
		wasPlaying := m.playerState.Playing
		s := player.State(msg)
		for _, line := range s.Logs {
			m.appendLog(line)
		}
		s.Logs = nil
		if s.Error != "" {
			if strings.Contains(s.Error, "CONTENT_RESTRICTED") {
				// Track is region-locked or unavailable in this storefront.
				// Log it silently and skip to the next track — same behaviour
				// as any streaming app encountering a restricted title.
				title := "track"
				if s.Track != nil {
					title = s.Track.Artist + " — " + s.Track.Title
				}
				m.appendLog(fmt.Sprintf("[skip] restricted: %s", title))
				if m.player != nil {
					cmds = append(cmds, m.playerCmd(func(p player.Player) error { return p.Next() }))
				}
			} else {
				m.appendLog("[error] " + s.Error)
				m.errMsg = s.Error
				m.errExpiry = time.Now().Add(4 * time.Second)
			}
			s.Error = ""
		}
		s.SkippedID = ""
		if s.Track != nil && (m.playerState.Track == nil || m.playerState.Track.Title != s.Track.Title) {
			m.appendLog("[playing] " + s.Track.Artist + " — " + s.Track.Title)
			// Log playParams so we can confirm which ID path MusicKit will use.
			trackType := "catalog"
			if strings.HasPrefix(s.Track.ID, "i.") {
				trackType = "library"
			}
			pp := fmt.Sprintf("[playParams] id=%s type=%s", s.Track.ID, trackType)
			if s.Track.CatalogID != "" {
				pp += " catalogId=" + s.Track.CatalogID
			}
			m.appendLog(pp)
		}
		m.vibeAIOnState(s)
		cmds = append(cmds, m.vibeAIHandoff(s), m.vibeAIStreamTick(s))
		m.playerState = s
		if !wasPlaying && m.playerState.Playing {
			m.appendLog("[player] playing")
		} else if wasPlaying && !m.playerState.Playing && !m.playerState.Loading {
			m.appendLog("[player] paused")
		}
		if m.stateCh != nil {
			cmds = append(cmds, waitForState(m.stateCh))
		}

	case DebugLogMsg:
		m.appendLog(string(msg))

	case EngineReadyMsg:
		m.player = msg.Player
		m.provider = msg.Provider
		m.stateCh = msg.Player.Subscribe()
		m.appendLog("[engine] backend: " + msg.Backend)
		cmds = append(cmds, waitForState(m.stateCh))
		cmds = append(cmds, m.vibeAIFlushPending())
		cmds = append(cmds, m.restoreAudioSettings()...)

	case InitErrMsg:
		m.appendLog("[init error] " + msg.Err.Error())
		m.errMsg = msg.Err.Error()
		m.errExpiry = time.Now().Add(30 * time.Second)
		m.vibeAIP.m.initErr = msg.Err.Error()

	case errMsg:
		m.appendLog("[error] " + msg.err.Error())
		m.errMsg = msg.err.Error()
		m.errExpiry = time.Now().Add(3 * time.Second)

	case SessionExpiredMsg:
		m.errMsg = "Session expired — opening browser to re-authenticate…"
		m.errExpiry = time.Now().Add(365 * 24 * time.Hour) // persists until restored

	case SessionRestoredMsg:
		m.errMsg = "✓ Re-authenticated with Apple Music"
		m.errExpiry = time.Now().Add(5 * time.Second)

	case RestartMsg:
		return m, tea.Quit

	case tea.KeyPressMsg:
		cmds = append(cmds, m.handleKey(msg))

	case tea.PasteMsg:
		cmds = append(cmds, m.vibeAIP.m.Paste(msg.Content))
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()

	// ctrl+c always quits
	if k == "ctrl+c" {
		m.vibeAINoteLeave()
		m.vibeAI.close()
		if m.player != nil {
			_ = m.player.Close()
		}
		return tea.Quit
	}

	// The scene keys first; whatever it declines and is not a player gesture
	// stops here.
	cmd, handled := m.vibeAIP.m.HandleKey(msg)
	if handled {
		return cmd
	}
	switch k {
	case "space":
		return m.togglePlayPause()
	case "n":
		m.appendLog("[player] next")
		m.playerState.Loading = true
		return m.playerCmd(func(p player.Player) error { return p.Next() })
	case "p":
		m.appendLog("[player] previous")
		m.playerState.Loading = true
		return m.playerCmd(func(p player.Player) error { return p.Previous() })
	case "left", "right":
		return m.vibeAIP.m.seekCmd(k == "right")
	}
	return cmd
}

func (m *Model) togglePlayPause() tea.Cmd {
	return func() tea.Msg {
		if m.player == nil {
			return errMsg{fmt.Errorf("no player")}
		}
		var err error
		if m.playerState.Playing {
			err = m.player.Pause()
		} else {
			err = m.player.Play()
		}
		if err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m *Model) playerCmd(fn func(player.Player) error) tea.Cmd {
	p := m.player
	return func() tea.Msg {
		if p == nil {
			return errMsg{fmt.Errorf("no player")}
		}
		if err := fn(p); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// ── View ──────────────────────────────────────────────────────────────────

func (m *Model) View() tea.View {
	v := tea.View{AltScreen: true}
	if m.width == 0 || m.height == 0 {
		return v
	}
	v.SetContent(m.vibeAIP.m.Frame(m.width, m.height))
	return v
}

// appendLog adds a timestamped entry to the debug log, capped at 500 lines.
func (m *Model) appendLog(line string) {
	const maxLines = 500
	ts := time.Now().Format("15:04:05")
	m.debugLog = append(m.debugLog, ts+"  "+line)
	if m.logW != nil {
		_, _ = fmt.Fprintf(m.logW, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05.000"), line)
	}
	if len(m.debugLog) > maxLines {
		m.debugLog = m.debugLog[len(m.debugLog)-maxLines:]
	}
}

func configEQBandsToPlayer(bands []config.EQBand) []player.EQBand {
	if len(bands) == 0 {
		return nil
	}
	out := make([]player.EQBand, len(bands))
	for i, b := range bands {
		out[i] = player.EQBand{Frequency: b.Frequency, Q: b.Q, Gain: b.Gain}
	}
	return out
}
