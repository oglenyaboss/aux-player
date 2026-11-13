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
			t.Errorf("кадр шага агента без %q:\n%s", s, frame)
		}
	}
	if row := frameRow(frame, "Агент"); !strings.Contains(row, "главное") {
		t.Errorf("строка «Агент» без метки главного: %q", row)
	}
	if step, _, _, _ := m.Onboarding(); step != 1 {
		t.Fatalf("шаг %d, ждём 1", step)
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
			t.Errorf("кадр подтверждения без %q:\n%s", s, frame)
		}
	}
	if got := recorded[Setup](r); len(got) != 0 {
		t.Fatalf("подтверждение отправило %v, ждём ничего", r.acts)
	}
}

func TestOnboardAgentAskCancel(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	m.Key("tab", "")
	m.SetSize(160, 45)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "всё равно пропустить") {
		t.Fatalf("подтверждения нет на экране:\n%s", frame)
	}
	m.Key("enter", "")
	m.SetSize(160, 45)
	if frame := stripANSI(m.View()); strings.Contains(frame, "всё равно пропустить") {
		t.Errorf("подтверждение осталось после enter:\n%s", frame)
	}
	if step, sub, _, ok := m.Onboarding(); !ok || step != 1 || sub != SubProvider {
		t.Fatalf("после отмены шаг %d подшаг %d (ok=%v), ждём 1/SubProvider", step, sub, ok)
	}
	if !m.ob.agentAsk == false || m.ob.agentSkip {
		t.Fatalf("после отмены ask=%v skip=%v, ждём false/false", m.ob.agentAsk, m.ob.agentSkip)
	}
}

func TestOnboardAgentSkipSendsSetup(t *testing.T) {
	r := &recorder{}
	m := agentFrame(t, r)
	m.Key("tab", "")
	m.Key("tab", "")
	if step, _, _, ok := m.Onboarding(); !ok || step != 2 {
		t.Fatalf("после пропуска шаг %d (ok=%v), ждём 2", step, ok)
	}
	m.SetSize(160, 45)
	row := frameRow(stripANSI(m.View()), "Агент")
	if !strings.Contains(row, "пропущено · только ручной выбор") || strings.Contains(row, "подключено") {
		t.Fatalf("строка «Агент» после пропуска: %q", row)
	}
	m.Key("tab", "")
	for i := 0; i < 900 && m.ob != nil; i++ {
		m.Step(eventDT)
	}
	setups := recorded[Setup](r)
	want := Setup{Lang: "ru"}
	if len(setups) != 1 || setups[0] != want {
		t.Fatalf("Setup %+v, ждём один %+v", setups, want)
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
		t.Fatalf("подшаг %d, ждём SubModel", m.ob.sub)
	}
	m.Key("tab", "")
	if !m.ob.agentAsk {
		t.Fatal("tab не открыл подтверждение")
	}
	m.Key("esc", "")
	if m.ob.agentAsk {
		t.Fatal("esc не закрыл подтверждение")
	}
	if got := string(m.ob.filter); got != "a" {
		t.Errorf("фильтр %q после подтверждения, ждём a", got)
	}
	if m.ob.sub != SubModel {
		t.Errorf("подшаг %d после esc, ждём SubModel", m.ob.sub)
	}
}

func TestOnboardAgentLoadingIgnoresTab(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Key("tab", "")
	if m.ob.agentAsk {
		t.Error("tab в загрузке открыл подтверждение")
	}
	if _, sub, _, _ := m.Onboarding(); sub != SubLoading {
		t.Errorf("подшаг %d, ждём SubLoading", sub)
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
		t.Errorf("интро без подсказки об агенте:\n%s", frame)
	}
	if strings.Contains(frame, "например:") {
		t.Errorf("интро зовёт пример без агента:\n%s", frame)
	}
	m.Key("", "rain")
	m.Key("enter", "")
	if m.Phase() != PhOnboard {
		t.Fatalf("enter без агента дал фазу %v, ждём PhOnboard", m.Phase())
	}
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("шаг %d (ok=%v), ждём 1", step, ok)
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("без агента ушёл %v, ждём ничего", r.acts)
	}
	steps(m, obReady+0.2)
	m.Key("tab", "")
	m.Key("tab", "")
	for i := 0; i < 900 && m.Phase() != PhIntro; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("после иглы фаза %v, ждём PhIntro", m.Phase())
	}
	if got := m.Input(); got != "rain" {
		t.Errorf("ввод %q после пропуска, ждём rain", got)
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("после пропуска ушёл %v, ждём ничего", r.acts)
	}
	setups := recorded[Setup](r)
	if len(setups) != 1 || setups[0].Lastfm {
		t.Fatalf("Setup %+v, ждём один без Last.fm", setups)
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
		t.Fatalf("после иглы фаза %v, ждём PhIntro", m.Phase())
	}
	if got := recorded[Query](r); len(got) != 0 {
		t.Fatalf("до enter ушёл %v, ждём ничего", r.acts)
	}
	m.Key("enter", "")
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "rain" {
		t.Fatalf("после настройки ушёл %v, ждём один Query{rain}", r.acts)
	}
}

func TestIntroExampleWithAgent(t *testing.T) {
	ruLangAt(t)
	m := New(Options{Example: func() string { return "в духе HARDSTONE PSYCHO" }})
	m.SetSize(110, 32)
	steps(m, 2.5)
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "например:") {
		t.Errorf("интро с агентом без примера:\n%s", frame)
	}
	if strings.Contains(frame, "агент не подключён") {
		t.Errorf("интро с агентом зовёт настройку:\n%s", frame)
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
		t.Fatalf("enter в палитре без агента дал %v, ждём PhOnboard", m.Phase())
	}
	steps(m, obReady+0.2)
	m.Key("tab", "")
	m.Key("tab", "")
	for i := 0; i < 900 && m.Phase() != PhPlay; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhPlay {
		t.Fatalf("после иглы фаза %v, ждём PhPlay", m.Phase())
	}
	if got, want := m.curTrack().Key(), threeTracks()[1].Key(); got != want {
		t.Errorf("трек %q после возврата, ждём %q", got, want)
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
			t.Errorf("палитра без агента без %q:\n%s", s, frame)
		}
	}
}

// ruLangAt fixes the interface language to Russian for the test.
func ruLangAt(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetLang("ru") })
	SetLang("ru")
}
