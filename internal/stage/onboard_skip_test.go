package stage

import (
	"strings"
	"testing"
)

// agentFrame renders the onboarding over an agent-less host and walks the Apple row
// to the agent step.
func agentFrame(t *testing.T, r *recorder) *Model {
	t.Helper()
	m := liveOnboard(t, Options{Connected: [3]bool{true, false, false}}, r)
	steps(m, 0.6)
	return m
}

func TestOnboardAgentCore(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	frame := stripANSI(m.View())
	for _, s := range []string{"сердце AUX", "tab без агента"} {
		if !strings.Contains(frame, s) {
			t.Errorf("agent step frame without %q:\n%s", s, frame)
		}
	}
	if row := frameRow(frame, "Агент"); !strings.Contains(row, "главное") {
		t.Errorf("the «Агент» row without the main marker: %q", row)
	}
	if step, _, _, _ := m.Onboarding(); step != 1 {
		t.Fatalf("step %d, want 1", step)
	}
}

func TestOnboardAgentAsk(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	m.SetSize(160, 45)
	m.Key("tab", "")
	frame := stripANSI(m.View())
	for _, s := range []string{"без агента останется только ручной выбор музыки", "всё равно пропустить"} {
		if !strings.Contains(frame, s) {
			t.Errorf("confirmation frame without %q:\n%s", s, frame)
		}
	}
	if got := recorded[Setup](r); len(got) != 0 {
		t.Fatalf("the confirmation sent %v, want nothing", r.acts)
	}
}

func TestOnboardAgentAskCancel(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	m.Key("tab", "")
	m.SetSize(160, 45)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "всё равно пропустить") {
		t.Fatalf("there is no confirmation on the screen:\n%s", frame)
	}
	m.Key("enter", "")
	m.SetSize(160, 45)
	if frame := stripANSI(m.View()); strings.Contains(frame, "всё равно пропустить") {
		t.Errorf("the confirmation stayed after enter:\n%s", frame)
	}
	if step, sub, _, ok := m.Onboarding(); !ok || step != 1 || sub != SubProvider {
		t.Fatalf("after the cancel step %d substep %d (ok=%v), want 1/SubProvider", step, sub, ok)
	}
	if !m.ob.agentAsk == false || m.ob.agentSkip {
		t.Fatalf("after the cancel ask=%v skip=%v, want false/false", m.ob.agentAsk, m.ob.agentSkip)
	}
}

func TestOnboardAgentSkipSendsSetup(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	m.Key("tab", "")
	m.Key("tab", "")
	if step, _, _, ok := m.Onboarding(); !ok || step != 2 {
		t.Fatalf("after the skip step %d (ok=%v), want 2", step, ok)
	}
	m.SetSize(160, 45)
	row := frameRow(stripANSI(m.View()), "Агент")
	if !strings.Contains(row, "пропущено · только ручной выбор") || strings.Contains(row, "подключено") {
		t.Fatalf("the «Агент» row after the skip: %q", row)
	}
	m.Key("tab", "")
	for i := 0; i < 900 && m.ob != nil; i++ {
		m.Step(eventDT)
	}
	setups := recorded[Setup](r)
	want := Setup{Lang: "ru"}
	if len(setups) != 1 || setups[0] != want {
		t.Fatalf("Setup %+v, want one %+v", setups, want)
	}
}

func TestOnboardAgentAskKeepsFilter(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models([]ModelInfo{{ID: "a/x", Reasoning: true}, {ID: "b/y"}}, 50, "")
	steps(m, 1.5)
	m.Key("", "a")
	if m.ob.sub != SubModel {
		t.Fatalf("substep %d, want SubModel", m.ob.sub)
	}
	m.Key("tab", "")
	if !m.ob.agentAsk {
		t.Fatal("tab did not open the confirmation")
	}
	m.Key("esc", "")
	if m.ob.agentAsk {
		t.Fatal("esc did not close the confirmation")
	}
	if got := string(m.ob.filter); got != "a" {
		t.Errorf("filter %q after the confirmation, want a", got)
	}
	if m.ob.sub != SubModel {
		t.Errorf("substep %d after esc, want SubModel", m.ob.sub)
	}
}

func TestOnboardAgentLoadingIgnoresTab(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Key("tab", "")
	if m.ob.agentAsk {
		t.Error("tab during loading opened the confirmation")
	}
	if _, sub, _, _ := m.Onboarding(); sub != SubLoading {
		t.Errorf("substep %d, want SubLoading", sub)
	}
}

func TestIntroAgentGate(t *testing.T) {
	ruLangAt(t)
	r := &recorder{}
	m := New(Options{OnAction: r.on, NoAgent: true})
	m.SetSize(110, 32)
	steps(m, 2.5)
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "агент не подключён — enter откроет настройку") {
		t.Errorf("intro without the agent hint:\n%s", frame)
	}
	if strings.Contains(frame, "например:") {
		t.Errorf("the intro calls for an example without an agent:\n%s", frame)
	}
	m.Key("", "rain")
	m.Key("enter", "")
	if m.Phase() != PhOnboard {
		t.Fatalf("enter without an agent gave phase %v, want PhOnboard", m.Phase())
	}
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("step %d (ok=%v), want 1", step, ok)
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("without an agent %v went out, want nothing", r.acts)
	}
	steps(m, obReady+0.2)
	m.Key("tab", "")
	m.Key("tab", "")
	for i := 0; i < 900 && m.Phase() != PhIntro; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("after the needle phase %v, want PhIntro", m.Phase())
	}
	if got := m.Input(); got != "rain" {
		t.Errorf("input %q after the skip, want rain", got)
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("after the skip %v went out, want nothing", r.acts)
	}
	setups := recorded[Setup](r)
	if len(setups) != 1 || setups[0].Lastfm {
		t.Fatalf("Setup %+v, want one without Last.fm", setups)
	}
}

func TestIntroAgentGateLaunch(t *testing.T) {
	ruLangAt(t)
	r := &recorder{}
	m := New(Options{OnAction: r.on, NoAgent: true})
	m.SetSize(110, 32)
	steps(m, 2.5)
	m.Key("", "rain")
	m.Key("enter", "")
	steps(m, obReady+0.2)
	m.Key("enter", "")
	m.Key("", "k1")
	m.Key("enter", "")
	m.Models([]ModelInfo{{ID: "a/x", Reasoning: true}, {ID: "b/y"}}, 50, "")
	steps(m, 1.5)
	m.Key("enter", "")
	m.Key("right", "")
	m.Key("enter", "")
	for i := 0; i < 900 && m.Phase() != PhIntro; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("after the needle phase %v, want PhIntro", m.Phase())
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("before enter %v went out, want nothing", r.acts)
	}
	m.Key("enter", "")
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "rain" {
		t.Fatalf("after the setup %v went out, want one Query{rain}", r.acts)
	}
}

func TestIntroExampleWithAgent(t *testing.T) {
	ruLangAt(t)
	m := New(Options{Example: func() string { return "в духе HARDSTONE PSYCHO" }})
	m.SetSize(110, 32)
	steps(m, 2.5)
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "например:") {
		t.Errorf("the intro with an agent has no example:\n%s", frame)
	}
	if strings.Contains(frame, "агент не подключён") {
		t.Errorf("the intro with an agent calls for setup:\n%s", frame)
	}
}

func TestPaletteAgentGateKeepsTrack(t *testing.T) {
	ruLangAt(t)
	r := &recorder{}
	m := New(Options{OnAction: r.on, NoAgent: true, MusicStyle: "palette"})
	m.SetSize(160, 45)
	m.JumpPlay(threeTracks(), 1, 0)
	steps(m, 0.2)
	m.Key("/", "/")
	steps(m, 0.2)
	m.Key("", "x")
	m.Key("enter", "")
	if m.Phase() != PhOnboard {
		t.Fatalf("enter in the palette without an agent gave %v, want PhOnboard", m.Phase())
	}
	steps(m, obReady+0.2)
	m.Key("tab", "")
	m.Key("tab", "")
	for i := 0; i < 900 && m.Phase() != PhPlay; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhPlay {
		t.Fatalf("after the needle phase %v, want PhPlay", m.Phase())
	}
	if got, want := m.curTrack().Key(), threeTracks()[1].Key(); got != want {
		t.Errorf("track %q after the return, want %q", got, want)
	}
}

func TestPaletteAgentCaps(t *testing.T) {
	ruLangAt(t)
	m := New(Options{NoAgent: true, MusicStyle: "palette"})
	m.SetSize(160, 45)
	m.JumpPlay(threeTracks(), 1, 0)
	m.Key("/", "/")
	steps(m, 0.4)
	frame := stripANSI(m.View())
	for _, s := range []string{"подключить агента", "агент не подключён — enter откроет настройку"} {
		if !strings.Contains(frame, s) {
			t.Errorf("palette without an agent without %q:\n%s", s, frame)
		}
	}
}

// ruLangAt fixes the interface language to Russian for the test.
func ruLangAt(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetLang("ru") })
	SetLang("ru")
}
