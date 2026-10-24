package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
)

// auxKey makes the key a terminal sends: space carries its text.
func auxKey(k string) tea.KeyPressMsg {
	if k == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	return stageKeyMsg(k)
}

// auxPress sends a key through Update, so the AUX-only branch of handleKey sees it.
func auxPress(m *Model, k string) tea.Cmd {
	_, cmd := m.Update(auxKey(k))
	return cmd
}

// auxModel is what `aux` starts with: the scene owns the terminal and the
// Apple Music sign-in waits for the onboarding to ask.
func auxModel(t *testing.T, plyr player.Player, cfg *config.Config, opts Options) *Model {
	t.Helper()
	isolatedHome(t)
	if cfg == nil {
		cfg = testCfg()
	}
	return New(cfg, &mockProvider{}, plyr, opts)
}

func auxFrame(m *Model, w, h int) string {
	return ansi.Strip(m.vibeAIP.m.Frame(w, h))
}

func TestAUXView(t *testing.T) {
	m := auxModel(t, nil, nil, Options{})
	m.width, m.height = 100, 30
	content := m.View().Content
	if lines := strings.Split(content, "\n"); len(lines) != 30 {
		t.Fatalf("frame has %d lines, want 30", len(lines))
	}
	if plain := ansi.Strip(content); strings.Contains(plain, "NORMAL") {
		t.Errorf("the old status bar leaked into the AUX-only view:\n%s", plain)
	}
}

func TestAUXBlocksPanels(t *testing.T) {
	m := auxModel(t, nil, nil, Options{})
	stageIntoPlayer(m)
	phase := m.vibeAIP.m.st.Phase()
	cmd := auxPress(m, "l")
	if cmd != nil {
		t.Errorf("the library key must not produce a cmd, got %#v", cmd())
	}
	cmd = auxPress(m, ":")
	if cmd != nil {
		t.Errorf("':' must not open command mode, got %#v", cmd())
	}
	if got := m.vibeAIP.m.st.Phase(); got != phase {
		t.Errorf("phase = %d, want %d: panel keys must not leave the AUX screen", got, phase)
	}
}

func TestAUXPassesSpace(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "1", Title: "One"}
	m.playerState.Playing = false
	cmd := auxPress(m, "space")
	if mp.playCalled || mp.pauseCalled {
		t.Fatal("the player was touched without running the cmd: broken expectation")
	}
	if cmd == nil {
		t.Fatal("space with a track around must return the play/pause cmd")
	}
	cmd()
	if !mp.playCalled {
		t.Error("space on the AUX screen must toggle the player, play not called")
	}
}

func TestAUXTypingKeepsSpace(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	if phase := m.vibeAIP.m.st.Phase(); phase != stage.PhIntro {
		t.Fatalf("phase = %d, want intro", phase)
	}
	auxPress(m, "space")
	if mp.playCalled || mp.pauseCalled {
		t.Error("space in the intro input must not touch the player")
	}
	if in := m.vibeAIP.m.st.Input(); in != " " {
		t.Fatalf("Input() = %q, want one space on the intro input", in)
	}
}

func TestAUXQuit(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	m.vibeAIP.m.act(stage.Quit{})
	if !mp.closeCalled {
		t.Error("quitting the AUX must close the player")
	}
	cmd := m.vibeAIP.m.flush()
	if cmd == nil {
		t.Fatal("stage.Quit must ship as a command")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("quit msg = %#v, want tea.QuitMsg", msg)
	}
}

func TestStageOptionsApple(t *testing.T) {
	o := stageOptions(&config.Config{VibeLLMAPIKey: "k"}, true)
	if !o.Onboard || o.Connected != [3]bool{false, true, false} {
		t.Fatalf("waiting for the sign-in: onboard=%v connected=%v, want true/[false true false]",
			o.Onboard, o.Connected)
	}
	if o := stageOptions(&config.Config{VibeLLMAPIKey: "k", AppleUserToken: "t"}, true); o.Onboard {
		t.Error("a signed-in Apple step must not re-onboard")
	}
	if o := stageOptions(&config.Config{VibeLLMAPIKey: "k"}, false); o.Onboard {
		t.Error("without the sign-in wait a filled config must not onboard")
	}
}

func TestAUXConnectApple(t *testing.T) {
	ch := make(chan struct{}, 1)
	cfg := testCfg()
	cfg.VibeLLMAPIKey = ""
	m := auxModel(t, nil, cfg, Options{AppleLogin: ch})
	st := m.vibeAIP.m.st
	obStepUntil(st, 8, func() bool { _, _, tt, ok := st.Onboarding(); return ok && tt >= 5.6 })
	st.Key("enter", "") // the language
	st.Key("enter", "") // Apple Music
	if got := len(ch); got != 1 {
		t.Fatalf("after the enter the ask holds %d values, want the pending sign-in", got)
	}
	m.handleVibeAIMsg(AppleLoginMsg{})
	obStepUntil(st, 2, func() bool {
		step, _, _, ok := st.Onboarding()
		return ok && step == 1
	})
	if step, _, _, ok := st.Onboarding(); !ok || step != 1 {
		t.Fatalf("after the successful sign-in step = %d (ok %v), want 1: the Apple step completes", step, ok)
	}
}

func TestAUXConnectAppleFail(t *testing.T) {
	ch := make(chan struct{}, 1)
	cfg := testCfg()
	cfg.VibeLLMAPIKey = ""
	m := auxModel(t, nil, cfg, Options{AppleLogin: ch})
	st := m.vibeAIP.m.st
	obStepUntil(st, 8, func() bool { _, _, tt, ok := st.Onboarding(); return ok && tt >= 5.6 })
	st.Key("enter", "")
	st.Key("enter", "")
	m.handleVibeAIMsg(AppleLoginMsg{Err: errors.New("отказ")})
	obStepUntil(st, 0.5, func() bool { return false })
	if step, _, _, ok := st.Onboarding(); !ok || step != 0 {
		t.Fatalf("after the failed sign-in step = %d (ok %v), want 0: the step keeps asking", step, ok)
	}
	if frame := auxFrame(m, 110, 32); !strings.Contains(frame, "отказ") {
		t.Errorf("frame without the failure:\n%s", frame)
	}
}

func TestAUXInitErrInIntro(t *testing.T) {
	m := auxModel(t, nil, nil, Options{})
	m.width, m.height = 110, 32
	m.Update(InitErrMsg{Err: errors.New("browser setup: boom")})
	m.handleVibeAIMsg(stageTickMsg(time.Now()))
	if frame := auxFrame(m, 110, 32); !strings.Contains(frame, "boom") {
		t.Errorf("frame without the launch failure:\n%s", frame)
	}
}

func TestAUXErrToastInPlay(t *testing.T) {
	m := auxModel(t, newMockPlayer(), nil, Options{})
	stageIntoPlayer(m)
	m.errMsg = "playback failed"
	base := time.Now()
	found := false
	for i := 0; i < 40 && !found; i++ {
		m.handleVibeAIMsg(stageTickMsg(base.Add(time.Duration(i+1) * 100 * time.Millisecond)))
		found = strings.Contains(ansi.Strip(m.vibeAIP.m.Frame(110, 32)), "playback failed")
	}
	if !found {
		t.Errorf("frame without the playback error:\n%s", auxFrame(m, 110, 32))
	}
}

func TestAUXPassesNext(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "1", Title: "One"}
	m.playerState.Playing = true
	cmd := auxPress(m, "n")
	if cmd == nil {
		t.Fatal("n on the AUX player must return the next cmd")
	}
	runAll(cmd)
	if !mp.nextCalled {
		t.Error("n on the AUX screen must reach the player, Next not called")
	}
}

func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b {
			runAll(c)
		}
	}
}

func TestLastfmConnectedSignal(t *testing.T) {
	ch := make(chan struct{}, 1)
	cfg := testCfg()
	m := auxModel(t, nil, cfg, Options{LastfmConnected: ch})
	m.Update(stageStepMsg{step: 2, session: "s"})
	if got := len(ch); got != 1 {
		t.Fatalf("after the session the signal channel holds %d values, want 1", got)
	}
	if m.cfg.LastfmSessionKey != "s" {
		t.Fatalf("LastfmSessionKey = %q, want the onboarding session", m.cfg.LastfmSessionKey)
	}

	failed := make(chan struct{}, 1)
	mf := auxModel(t, nil, testCfg(), Options{LastfmConnected: failed})
	mf.Update(stageStepMsg{step: 2, err: "x"})
	if got := len(failed); got != 0 {
		t.Fatalf("a failed step signalled %d times, want 0", got)
	}
}

func TestAUXPaste(t *testing.T) {
	m := auxModel(t, nil, nil, Options{})
	st := m.vibeAIP.m.st
	for i := 0; i < 400 && st.Phase() != stage.PhIntro; i++ {
		st.Step(1.0 / 60)
	}
	m.Update(tea.PasteMsg{Content: "lofi"})
	if got := st.Input(); got != "lofi" {
		t.Fatalf("Input() = %q after paste, want lofi", got)
	}
}

func TestAUXSeekKeys(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "1", Title: "One", Duration: 3 * time.Minute}
	m.playerState.Position = time.Minute
	cmd := auxPress(m, "right")
	if cmd == nil {
		t.Fatal("right: the scene declines it, the host must keep seeking")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(errMsg); ok {
			t.Fatalf("seek right failed: %v", msg)
		}
	}
	if !mp.seekCalled || mp.seekPos != 70*time.Second {
		t.Errorf("seek right: called=%v pos=%v, want 10s forward", mp.seekCalled, mp.seekPos)
	}
	cmd = auxPress(m, "left")
	if cmd == nil {
		t.Fatal("left: the scene declines it, the host must keep seeking")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(errMsg); ok {
			t.Fatalf("seek left failed: %v", msg)
		}
	}
	if mp.seekPos != 50*time.Second {
		t.Errorf("seek left: pos=%v, want 10s back", mp.seekPos)
	}
}

func TestAUXOtherKeysInert(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "1", Title: "One"}
	m.width, m.height = 100, 30
	for _, k := range []string{"l", "/", ":"} {
		if cmd := auxPress(m, k); cmd != nil {
			t.Fatalf("%q must do nothing on the scene, got a cmd", k)
		}
	}
	if mp.playCalled || mp.pauseCalled || mp.nextCalled || mp.prevCalled || mp.seekCalled || mp.closeCalled {
		t.Errorf("the player was touched by inert keys: %+v", mp)
	}
	frame := ansi.Strip(m.View().Content)
	if strings.TrimSpace(frame) == "" {
		t.Fatal("View() must still draw the scene")
	}
	if strings.Contains(frame, "NORMAL") {
		t.Errorf("the old status bar leaked into the view:\n%s", frame)
	}
}

func TestAUXCtrlC(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	cmd := auxPress(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("ctrl+c must quit")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("ctrl+c msg = %#v, want tea.QuitMsg", msg)
	}
	if !mp.closeCalled {
		t.Error("ctrl+c must close the player")
	}
}

func TestAUXRestrictedSkips(t *testing.T) {
	mp := newMockPlayer()
	m := auxModel(t, mp, nil, Options{})
	m.stateCh = nil // no blocking waitForState in the batch under test
	_, cmd := m.Update(playerStateMsg{
		Error: "CONTENT_RESTRICTED: track unavailable",
		Track: &provider.Track{ID: "111", Title: "Restricted Song", Artist: "Artist"},
	})
	runAll(cmd)
	if !mp.nextCalled {
		t.Error("CONTENT_RESTRICTED must skip to the next track")
	}
	if m.errMsg != "" {
		t.Errorf("errMsg = %q, want none: a restricted track is skipped silently", m.errMsg)
	}
}
