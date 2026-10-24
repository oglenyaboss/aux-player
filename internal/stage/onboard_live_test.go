package stage

import (
	"strings"
	"testing"
)

// liveOnboard builds the onboarding over a host that only records the actions, leaves
// Onboard on, sizes the screen and moves past the cinematic; the language is confirmed
// with enter, as a user would.
func liveOnboard(t *testing.T, o Options, r *recorder) *Model {
	t.Helper()
	o.Onboard = true
	o.OnAction = r.on
	m := New(o)
	m.SetSize(110, 32)
	for i := 0; i < 400 && m.ob.t < obReady; i++ {
		m.Step(eventDT)
	}
	m.Key("enter", "")
	return m
}

// frameRow is the first frame line carrying sub.
func frameRow(frame, sub string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// toAgent walks the Apple Music step with the host answering the sign-in, landing on
// the agent step with OpenRouter selected.
func toAgent(m *Model) {
	m.Key("enter", "")
	m.StepDone(0, "")
	steps(m, 2)
}

func TestOnboardAppleAction(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	if got := recorded[ConnectApple](r); len(got) != 1 {
		t.Fatalf("ждём один ConnectApple, записано %v", r.acts)
	}
	steps(m, 3)
	step, _, _, ok := m.Onboarding()
	if !ok || step != 0 {
		t.Fatalf("без ответа хоста шаг не должен менять: step=%d ok=%v", step, ok)
	}
}

func TestOnboardAppleDone(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "")
	steps(m, 2)
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("после StepDone шаг %d (ok=%v), ждём 1", step, ok)
	}
}

func TestOnboardAppleFail(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "отказано")
	steps(m, 0.5)
	if step, _, _, ok := m.Onboarding(); !ok || step != 0 {
		t.Fatalf("после ошибки шаг %d (ok=%v), ждём 0", step, ok)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "отказано") {
		t.Errorf("кадр без ошибки хоста:\n%s", frame)
	}
	m.Key("x", "")
	if frame := stripANSI(m.View()); strings.Contains(frame, "отказано") {
		t.Errorf("ошибка осталась после клавиши:\n%s", frame)
	}
}

// modelsStep fetches the agent key k1 and leaves the loading substep up.
func modelsStep(t *testing.T, m *Model) {
	t.Helper()
	toAgent(m)
	m.Key("enter", "")
	m.Key("", "k1")
	m.Key("enter", "")
}

func TestOnboardLoadModels(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	if got := recorded[LoadModels](r); len(got) != 1 || got[0] != (LoadModels{URL: "https://openrouter.ai/api/v1", Key: "k1"}) {
		t.Fatalf("ждём LoadModels{https://openrouter.ai/api/v1, k1}, записано %v", r.acts)
	}
	steps(m, 3)
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubLoading {
		t.Fatalf("без ответа подшаг %d (ok=%v), ждём загрузку", sub, ok)
	}
}

func TestOnboardModelsReply(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models([]ModelInfo{{ID: "a/x", Reasoning: true}, {ID: "b/y"}}, 50, "")
	steps(m, 1.5)
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "a/x") {
		t.Errorf("кадр без a/x:\n%s", frame)
	}
	cnt := frameRow(frame, "с инструментами")
	if cnt == "" {
		t.Fatalf("кадр без строки счётчика:\n%s", frame)
	}
	if !strings.Contains(cnt, "2") || !strings.Contains(cnt, "50") {
		t.Errorf("счётчик без двух моделей из 50: %q", cnt)
	}
}

func TestOnboardModelsFail(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models(nil, 0, "ключ не подошёл")
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubKey {
		t.Fatalf("после ошибки подшаг %d (ok=%v), ждём ключ", sub, ok)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "ключ не подошёл") {
		t.Errorf("кадр без ошибки:\n%s", frame)
	}
}

func TestOnboardCustomURL(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "")
	steps(m, 2)
	for i := 0; i < 5; i++ {
		m.Key("down", "")
	}
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubProvider {
		t.Fatalf("подшаг %d (ok=%v), ждём провайдера", sub, ok)
	}
	m.Key("enter", "")
	m.Key("", "localhost:8080/v1")
	m.Key("enter", "")
	m.Key("enter", "")
	if got := recorded[LoadModels](r); len(got) != 1 || got[0] != (LoadModels{URL: "https://localhost:8080/v1"}) {
		t.Fatalf("ждём LoadModels{https://localhost:8080/v1}, записано %v", r.acts)
	}
}

func TestOnboardSetup(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models([]ModelInfo{{ID: "a/x", Reasoning: true}, {ID: "b/y"}}, 50, "")
	steps(m, 1.5)
	m.Key("enter", "")
	m.Key("right", "")
	m.Key("enter", "")
	m.Key("tab", "")
	setups := recorded[Setup](r)
	if len(setups) != 1 {
		t.Fatalf("ждём ровно один Setup, записано %v", r.acts)
	}
	want := Setup{Lang: "ru", URL: "https://openrouter.ai/api/v1", Key: "k1", Model: "a/x", Effort: "high", Lastfm: false}
	if setups[0] != want {
		t.Fatalf("Setup %+v, ждём %+v", setups[0], want)
	}
	for i := 0; i < 600 && m.Phase() != PhIntro; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("после иглы фаза %v, ждём PhIntro", m.Phase())
	}
}

func TestOnboardConnectedCascade(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{Connected: [3]bool{true, false, true}}, r)
	steps(m, 0.6)
	if got := recorded[ConnectApple](r); len(got) != 0 {
		t.Fatalf("от подключённого шага действия не ждём, записано %v", r.acts)
	}
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("текущий шаг %d (ok=%v), ждём 1", step, ok)
	}
}

func TestOnboardConnectedAll(t *testing.T) {
	r := &recorder{}
	m := New(Options{Onboard: true, OnAction: r.on, Connected: [3]bool{true, true, true}})
	m.SetSize(110, 32)
	for i := 0; i < 700 && m.ob != nil; i++ {
		if m.ob.t >= obReady && !m.ob.langDone {
			m.Key("enter", "")
		}
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("без клавиш онбординг не вышел: фаза %v", m.Phase())
	}
	setups := recorded[Setup](r)
	if len(setups) != 1 {
		t.Fatalf("ждём один Setup, записано %v", r.acts)
	}
	if setups[0].URL != "" || setups[0].Key != "" || setups[0].Model != "" {
		t.Fatalf("Setup %+v, ждём пустые URL/Key/Model", setups[0])
	}
}

func TestNoEggs(t *testing.T) {
	sky := []Track{{Artist: "Playboi Carti", Title: "Sky", Dur: 190}}
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(sky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "†") {
		t.Errorf("кадр пасхалки без ползунка †:\n%s", frame)
	}
	m = New(Options{NoEggs: true})
	m.SetSize(110, 32)
	m.JumpPlay(sky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); strings.Contains(frame, "†") {
		t.Errorf("кадр без пасхалок с ползунком †:\n%s", frame)
	}
}

func TestOnboardConnectedWaitsLang(t *testing.T) {
	m := New(Options{Onboard: true, Connected: [3]bool{true, false, false}})
	m.SetSize(110, 32)
	steps(m, obReady+2)
	if step, _, _, _ := m.Onboarding(); step != 0 {
		t.Fatalf("шаг %d до выбора языка, ждём 0", step)
	}
	m.Key("enter", "")
	steps(m, 0.6)
	if step, _, _, _ := m.Onboarding(); step != 1 {
		t.Fatalf("шаг %d после выбора языка, ждём 1", step)
	}
}

func TestOnboardLastfmSkipWhileWaiting(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{Connected: [3]bool{true, true, false}}, r)
	steps(m, 1.5)
	m.Key("enter", "")
	if got := recorded[ConnectLastfm](r); len(got) != 1 {
		t.Fatalf("ждём ConnectLastfm, записано %v", r.acts)
	}
	m.Key("tab", "")
	m.StepDone(2, "")
	steps(m, 0.2)
	setups := recorded[Setup](r)
	if len(setups) != 1 || setups[0].Lastfm {
		t.Fatalf("ждём один Setup без Last.fm, записано %v", r.acts)
	}
}

func TestOnboardModelsCursorOnRec(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models([]ModelInfo{{ID: "a/x"}, {ID: "b/y", Rec: true}}, 2, "")
	steps(m, 1.5)
	m.Key("enter", "")
	if m.ob.picked != "b/y" {
		t.Fatalf("выбрана %q, ждём рекомендуемую b/y", m.ob.picked)
	}
}
