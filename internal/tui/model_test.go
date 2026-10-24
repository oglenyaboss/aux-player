package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
)

func TestNew_NilPlayer(t *testing.T) {
	m := newModel(nil)
	if m == nil {
		t.Fatal("New(cfg, prov, nil) returned nil")
	}
}

func TestNew_WithPlayer(t *testing.T) {
	m := newModel(newMockPlayer())
	if m == nil {
		t.Fatal("New with player returned nil")
	}
	if m.stateCh == nil {
		t.Error("stateCh should be set when player is provided")
	}
}

func TestModel_Init(t *testing.T) {
	m := newModel(nil)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() should return non-nil cmd")
	}
}

// --- View ---

func TestModel_View_WidthZero(t *testing.T) {
	m := newModel(nil)
	got := m.View()
	// With width=0 the intro animation hasn't started yet — expect empty string.
	if got.Content != "" {
		t.Errorf("View() with width=0 should return empty string, got %q", got.Content)
	}
}

func TestModel_View_WithDimensions(t *testing.T) {
	m := newModel(nil)
	m.width = 80
	m.height = 24
	got := m.View()
	if got.Content == "" {
		t.Error("View() with dimensions should return non-empty string")
	}
}

// --- Update: WindowSizeMsg ---

func TestModel_Update_WindowSizeMsg(t *testing.T) {
	m := newModel(nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.width != 100 || m.height != 30 {
		t.Errorf("width=%d height=%d, want 100 30", m.width, m.height)
	}
}

// --- Update: tickMsg ---

func TestModel_Update_TickMsg(t *testing.T) {
	m := newModel(nil)
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Error("tickMsg should return a non-nil cmd (reschedule tick)")
	}
}

func TestModel_Update_TickMsg_ClearsExpiredErr(t *testing.T) {
	m := newModel(nil)
	m.errMsg = "old error"
	m.errExpiry = time.Now().Add(-1 * time.Second) // already expired
	m.Update(tickMsg(time.Now()))
	if m.errMsg != "" {
		t.Errorf("expired errMsg should be cleared, got %q", m.errMsg)
	}
}

// --- Update: errMsg ---

func TestModel_Update_ErrMsg(t *testing.T) {
	m := newModel(nil)
	m.Update(errMsg{err: errors.New("test error")})
	if m.errMsg != "test error" {
		t.Errorf("errMsg = %q, want %q", m.errMsg, "test error")
	}
}

func TestTogglePlayPause_NilPlayer(t *testing.T) {
	m := newModel(nil)
	cmd := m.togglePlayPause()
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("togglePlayPause with nil player should return errMsg, got %T", msg)
	}
}

func TestTogglePlayPause_Playing_CallsPause(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.playerState.Playing = true
	cmd := m.togglePlayPause()
	cmd() // execute
	if !mp.pauseCalled {
		t.Error("Pause() should be called when state is playing")
	}
}

func TestTogglePlayPause_NotPlaying_CallsPlay(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.playerState.Playing = false
	cmd := m.togglePlayPause()
	cmd() // execute
	if !mp.playCalled {
		t.Error("Play() should be called when state is not playing")
	}
}

func TestTogglePlayPause_PlayerError(t *testing.T) {
	mp := newMockPlayer()
	mp.err = errors.New("player error")
	m := newModel(mp)
	m.playerState.Playing = true
	cmd := m.togglePlayPause()
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("togglePlayPause with player error should return errMsg, got %T", msg)
	}
}

// --- playerCmd ---

func TestPlayerCmd_NilPlayer(t *testing.T) {
	m := newModel(nil)
	cmd := m.playerCmd(func(player.Player) error { return nil })
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("playerCmd with nil player should return errMsg, got %T", msg)
	}
}

func TestPlayerCmd_Next(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	cmd := m.playerCmd(func(p player.Player) error { return p.Next() })
	msg := cmd()
	if msg != nil {
		t.Errorf("playerCmd success should return nil msg, got %v", msg)
	}
	if !mp.nextCalled {
		t.Error("Next() should have been called")
	}
}

func TestPlayerCmd_Previous(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	cmd := m.playerCmd(func(p player.Player) error { return p.Previous() })
	cmd()
	if !mp.prevCalled {
		t.Error("Previous() should have been called")
	}
}

func TestPlayerCmdUsesCapturedPlayer(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	cmd := m.playerCmd(func(p player.Player) error { return p.Next() })
	m.player = nil
	msg := cmd()
	if msg != nil {
		t.Fatalf("playerCmd with captured player returned %v", msg)
	}
	if !mp.nextCalled {
		t.Fatal("captured player Next() was not called")
	}
}

// --- adjustVolume ---

func TestModel_KeySpace_NilPlayer(t *testing.T) {
	m := newModel(nil)
	stageIntoPlayer(m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space key should return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Errorf("space with nil player should produce errMsg, got %T", msg)
	}
}

func TestModel_KeyNext_WithPlayer(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	stageIntoPlayer(m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd != nil {
		cmd() // execute to trigger Next()
	}
	if !mp.nextCalled {
		t.Error("n key should call player.Next()")
	}
}

func TestModel_KeyPrevious_WithPlayer(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	stageIntoPlayer(m)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil {
		cmd() // execute to trigger Previous()
	}
	if !mp.prevCalled {
		t.Error("p key should call player.Previous()")
	}
}

func TestModel_Update_PlayerStateMsg(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	track := &provider.Track{Title: "Live Track", Artist: "Live Artist"}
	msg := playerStateMsg{
		Track:   track,
		Playing: true,
		Volume:  0.8,
	}
	m.Update(msg)
	if m.playerState.Track == nil {
		t.Error("playerState.Track should be set after playerStateMsg")
	}
	if m.playerState.Track.Title != "Live Track" {
		t.Errorf("Track.Title = %q, want %q", m.playerState.Track.Title, "Live Track")
	}
}

// --- contentHeight ---

func TestWaitForState_ReadsFromChannel(t *testing.T) {
	ch := make(chan player.State, 1)
	st := player.State{Playing: true, Volume: 0.9}
	ch <- st

	cmd := waitForState(ch)
	msg := cmd() // should not block since channel has a value

	ps, ok := msg.(playerStateMsg)
	if !ok {
		t.Fatalf("waitForState returned %T, want playerStateMsg", msg)
	}
	if !ps.Playing {
		t.Error("playerStateMsg.Playing should be true")
	}
	if ps.Volume != 0.9 {
		t.Errorf("playerStateMsg.Volume = %v, want 0.9", ps.Volume)
	}
}

func TestModel_Update_InitErrMsg(t *testing.T) {
	m := newModel(nil)
	_, cmd := m.Update(InitErrMsg{Err: errors.New("fatal error")})
	_ = cmd
	if m.errMsg == "" {
		t.Error("InitErrMsg should set errMsg")
	}
}

func TestModel_Update_ErrMsg_SetsErrField(t *testing.T) {
	m := newModel(nil)
	_, cmd := m.Update(errMsg{err: errors.New("some error")})
	_ = cmd
	if m.errMsg == "" {
		t.Error("errMsg should set m.errMsg")
	}
}

func TestModel_Update_SessionExpiredMsg(t *testing.T) {
	m := newModel(nil)
	_, cmd := m.Update(SessionExpiredMsg{})
	_ = cmd
	if m.errMsg == "" {
		t.Error("SessionExpiredMsg should set errMsg")
	}
}

func TestModel_Update_SessionRestoredMsg(t *testing.T) {
	m := newModel(nil)
	_, cmd := m.Update(SessionRestoredMsg{})
	_ = cmd
	if m.errMsg == "" {
		t.Error("SessionRestoredMsg should set success errMsg")
	}
}

func TestModel_Update_PlayerStateMsg_WithLog(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.stateCh = mp.stateCh
	st := playerStateMsg{Playing: true, Logs: []string{"something logged", "and this too"}}
	_, _ = m.Update(st)
	for _, want := range []string{"something logged", "and this too"} {
		found := false
		for _, entry := range m.debugLog {
			if strings.Contains(entry, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Logs entry %q should be appended to debugLog", want)
		}
	}
}

func TestModel_Update_PlayerStateMsg_ContentRestricted(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.stateCh = mp.stateCh
	track := &provider.Track{Title: "Restricted Song", Artist: "Artist", ID: "111"}
	st := playerStateMsg{
		Playing: false,
		Error:   "CONTENT_RESTRICTED: track unavailable",
		Track:   track,
	}
	_, cmd := m.Update(st)
	_ = cmd
	// Restricted content should not set errMsg but should log.
}

func TestModel_Update_PlayerStateMsg_GenericError(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(mp)
	m.stateCh = mp.stateCh
	st := playerStateMsg{Error: "something went wrong"}
	_, cmd := m.Update(st)
	_ = cmd
	if m.errMsg == "" {
		t.Error("generic player error should set errMsg")
	}
}

func TestModel_Update_EngineReadyMsg(t *testing.T) {
	mp := newMockPlayer()
	m := newModel(nil)

	// Simulate a window size arriving before the engine is ready (normal startup).
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	_, cmd := m.Update(EngineReadyMsg{
		Player:      mp,
		Provider:    &mockProvider{},
		HelperPaths: []string{"/usr/bin/helper"},
		Backend:     "cdp",
	})
	_ = cmd
	if m.player != mp {
		t.Error("EngineReadyMsg should set m.player")
	}
	if m.provider == nil {
		t.Error("EngineReadyMsg should set m.provider")
	}
	if m.stateCh == nil {
		t.Error("EngineReadyMsg must resubscribe to the new player state stream")
	}
}

// ─── handleNormalKey ────────────────────────────────────────────────────────

func logContains(m *Model, want string) bool {
	for _, entry := range m.debugLog {
		if strings.Contains(entry, want) {
			return true
		}
	}
	return false
}

func TestNew_ScanNoticeShownWhenTheLibraryIsEmpty(t *testing.T) {
	notice := "no playable tracks in /music: skipped 4 files (4 .ogg); macOS plays .flac, .m4a, .mp3"
	m := New(testCfg(), &mockProvider{}, newMockPlayer(), Options{ScanNotice: notice})

	if m.errMsg != notice {
		t.Errorf("errMsg = %q, want the notice", m.errMsg)
	}
	if !m.errExpiry.After(time.Now().Add(20 * time.Second)) {
		t.Errorf("errExpiry = %v, want long enough to read past the intro", m.errExpiry)
	}
	if !logContains(m, notice) {
		t.Errorf("debugLog = %v, want an entry carrying the notice", m.debugLog)
	}
}

func TestNew_ScanNoticeOnlyLoggedWhenTracksWereFound(t *testing.T) {
	notice := "indexed 1 of 5 files in /music: skipped 4 files (4 .ogg); macOS plays .flac, .m4a, .mp3"
	m := New(testCfg(), &mockProvider{}, newMockPlayer(), Options{
		ScanNotice:    notice,
		InitialTracks: []provider.Track{{ID: "local:/music/a.mp3", Title: "A"}},
	})

	if m.errMsg != "" {
		t.Errorf("errMsg = %q, want empty when the scan found tracks", m.errMsg)
	}
	if !logContains(m, notice) {
		t.Errorf("debugLog = %v, want an entry carrying the notice", m.debugLog)
	}
}
