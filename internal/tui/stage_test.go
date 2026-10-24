package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simone-vibes/vibez/internal/provider"
	"github.com/simone-vibes/vibez/internal/stage"
	"github.com/simone-vibes/vibez/internal/tui/views"
	"github.com/simone-vibes/vibez/internal/vibeai"
)

func stageKeyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

// stageMsgs flattens the cmd the stage produced: single messages and BatchMsg
// alike; tick commands execute into a timeout msg and are skipped.
func stageMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if _, isTick := c().(stageTickMsg); isTick {
				continue
			}
			out = append(out, stageMsgs(t, c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// stageIntoPlayer drops the scene into its player phase on the fixed tracks.
func stageIntoPlayer(m *Model) {
	ts := vibeTracks()
	tracks := make([]stage.Track, len(ts))
	for i, t := range ts {
		tracks[i] = stageTrack(vibeAIViewTrack(t), t.Artist+" — "+t.Title)
	}
	m.vibeAIP.m.st.JumpPlay(tracks, 0, 0)
}

func TestStageFullScreen(t *testing.T) {
	m := newModel(nil)
	m.introStep = introDone // skip the startup animation
	m.width, m.height = 100, 30
	press(m, "v")
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != 30 {
		t.Fatalf("frame has %d lines, want 30", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width %d exceeds %d: %q", i, w, 100, ansi.Strip(line))
		}
	}
}

func TestStageEscStays(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	if !m.vibeAIActive() {
		t.Fatal("v must open the aux screen")
	}
	press(m, "esc")
	if !m.vibeAIActive() {
		t.Fatal("esc on the input must not close the aux screen")
	}
}

func TestStagePassesSpace(t *testing.T) {
	m := openVibeWithPortion(t)
	stageIntoPlayer(m)
	if _, handled := m.vibeAIP.m.HandleKey(tea.KeyPressMsg{Code: tea.KeySpace}); handled {
		t.Fatal("the scene must pass space through so vibez can pause")
	}
}

func TestStageRateIndex(t *testing.T) {
	m := openVibeWithPortion(t)
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "2"}
	cmd := press(m, "+")
	if cmd == nil {
		t.Fatal("+ must send the rating")
	}
	if got := cmd(); got != (views.VibeAIRateMsg{Index: 1, Rating: 1}) {
		t.Fatalf("got %#v, want rate index 1 (+1)", got)
	}
}

func TestStageRateNoTrack(t *testing.T) {
	m := openVibeWithPortion(t)
	stageIntoPlayer(m)
	m.playerState.Track = nil
	if cmd := press(m, "+"); cmd != nil {
		t.Fatalf("rate without a playing track must send nothing, got %#v", cmd())
	}
}

func TestStageProgressPhrase(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	press(m, "enter") // the example query launches the scene
	m.vibeAIP.m.st.Step(2.5)
	if m.vibeAIP.m.st.Phase() != stage.PhScan {
		t.Fatalf("phase = %d, want scan", m.vibeAIP.m.st.Phase())
	}
	m.vibeAIP.m.AddProgress("·describe album Blonde")
	frame := ansi.Strip(m.vibeAIP.m.Frame(110, 32))
	if !strings.Contains(frame, "читаю про Blonde") {
		t.Errorf("frame must show the thought:\n%s", frame)
	}
}

func TestStageCoverOnce(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	tr := vibeAIViewTrack(vibeai.Track{ID: "1", Artist: "A", Title: "One", ArtworkURL: "https://x/{w}x{h}.jpg"})
	m.vibeAIP.m.AddPick(tr, "A — One", true, "")
	m.vibeAIP.m.AddPick(tr, "A — One", true, "")
	if len(m.vibeAIP.m.want) != 1 {
		t.Fatalf("queued cover fetches = %d, want 1", len(m.vibeAIP.m.want))
	}
	c := m.vibeAIP.m.want[0]
	if c.key != "1" || c.url != "https://x/300x300.jpg" {
		t.Errorf("cover request = %+v, want key 1 at https://x/300x300.jpg", c)
	}
}

func TestStageTickStops(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	if m.stageWake() == nil {
		t.Fatal("opening the screen must start the tick chain")
	}
	if m.stageWake() != nil {
		t.Fatal("a second wake must not duplicate the chain")
	}
	m.vibeAIP.m.act(stage.Quit{})
	if m.vibeAIActive() {
		t.Fatal("quit must close the screen")
	}
	if cmd := m.handleVibeAIMsg(stageTickMsg(time.Now())); cmd != nil {
		t.Errorf("a tick with the screen closed must end the chain, got %v", cmd)
	}
	if m.vibeAIP.m.ticking {
		t.Error("ticking must be off after the screen closed")
	}
}

func TestStageTickFlushesAnswers(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	vibeAITypeText(m, "первый запрос")
	m.vibeAIP.m.st.Step(2.5)
	m.handleVibeAIMsg(vibeAIAskMsg{gen: m.vibeAI.gen, qs: []vibeai.Question{{Question: "что по духу?", Options: []string{"новое", "знакомое"}}}})
	if m.vibeAIP.m.st.Phase() != stage.PhAsk {
		t.Fatalf("phase = %d, want ask", m.vibeAIP.m.st.Phase())
	}
	m.vibeAIP.m.st.Step(0.8) // the card accepts an answer only after 0.7s
	if cmd := press(m, "enter"); cmd != nil {
		t.Fatalf("press enter must be silent: %v", stageMsgs(t, cmd))
	}
	base := time.Now()
	found := false
	for i := 0; i < 40 && !found; i++ {
		for _, msg := range stageMsgs(t, m.handleVibeAIMsg(stageTickMsg(base.Add(time.Duration(i)*33*time.Millisecond)))) {
			ans, ok := msg.(views.VibeAIAnswersMsg)
			if !ok {
				continue
			}
			want := views.VibeAIAnswersMsg{Answers: []string{"1"}}
			if !reflect.DeepEqual(ans, want) {
				t.Fatalf("answers = %#v, want %#v", ans, want)
			}
			found = true
		}
	}
	if !found {
		t.Error("the answer must reach the bridge via the tick flush")
	}
}

func TestStageSeekPassesThrough(t *testing.T) {
	plyr := newMockPlayer()
	m := newModel(plyr)
	press(m, "v")
	vibeAITypeText(m, "первый запрос")
	m.playerState.Track = &provider.Track{ID: "1"}
	m.playerState.Position = 30 * time.Second
	cmd := m.handleNormalKey(stageKeyMsg("left"), "left")
	if cmd == nil {
		t.Fatal("left: the scene declines it, the host must keep seeking")
	}
	_ = cmd()
	if !plyr.seekCalled || plyr.seekPos != 20*time.Second {
		t.Errorf("seek: called=%v pos=%v, want 20s back", plyr.seekCalled, plyr.seekPos)
	}
	cmd = m.handleNormalKey(stageKeyMsg("right"), "right")
	if cmd == nil {
		t.Fatal("right: the scene declines it, the host must keep seeking")
	}
	_ = cmd()
	if !plyr.seekCalled || plyr.seekPos != 40*time.Second {
		t.Errorf("seek: called=%v pos=%v, want 40s forward", plyr.seekCalled, plyr.seekPos)
	}
}

// TestStagePlayActions walks the player-phase gestures the bridge listens to.
func TestStagePlayActions(t *testing.T) {
	m := openVibeWithPortion(t)
	stageIntoPlayer(m)
	m.playerState.Track = &provider.Track{ID: "1", Title: "One"}
	_ = press(m, "/")
	for _, r := range "меньше русского" {
		_ = press(m, string(r))
	}
	msgs := stageMsgs(t, press(m, "enter"))
	if len(msgs) != 1 {
		t.Fatalf("/ reply msgs = %#v, want one", msgs)
	}
	say, ok := msgs[0].(views.VibeAISubmitMsg)
	if !ok || say.Text != "меньше русского" || say.Fresh {
		t.Fatalf("/ reply = %#v, want a non-fresh submit", say)
	}
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks(), later: &vibeai.Question{Question: "убрать хоррор?", Options: []string{"да", "нет"}}})
	msgs = stageMsgs(t, press(m, "1"))
	if len(msgs) != 1 {
		t.Fatalf("toast answer msgs = %#v, want one", msgs)
	}
	later, ok := msgs[0].(views.VibeAILaterMsg)
	if !ok || later.Raw != "1" {
		t.Fatalf("toast answer = %#v, want raw 1", later)
	}
	msgs = stageMsgs(t, press(m, "w"))
	if len(msgs) != 1 {
		t.Fatalf("save msgs = %#v, want one", msgs)
	}
	if msgs[0] != (views.VibeAIActionMsg{Action: views.VibeAISave}) {
		t.Fatalf("save = %#v", msgs[0])
	}
	msgs = stageMsgs(t, press(m, "r"))
	if len(msgs) != 1 || msgs[0] != (views.VibeAICancelMsg{}) {
		t.Fatalf("restart = %#v", msgs)
	}
	if m.vibeAIP.m.st.Phase() != stage.PhIntro {
		t.Errorf("phase = %d, want intro after restart", m.vibeAIP.m.st.Phase())
	}
}
