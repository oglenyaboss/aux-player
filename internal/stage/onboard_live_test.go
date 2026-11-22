package stage

import (
	"reflect"
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
	for l := range strings.SplitSeq(frame, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

// toAgent walks the Apple Music step with the host answering the sign-in, landing on
// the agent step with OpenCode Go selected.
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
		t.Fatalf("want one ConnectApple, recorded %v", r.acts)
	}
	steps(m, 3)
	step, _, _, ok := m.Onboarding()
	if !ok || step != 0 {
		t.Fatalf("without a host reply the step must not change: step=%d ok=%v", step, ok)
	}
}

func TestOnboardAppleDone(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "")
	steps(m, 2)
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("after StepDone step %d (ok=%v), want 1", step, ok)
	}
}

func TestOnboardAppleFail(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "отказано")
	steps(m, 0.5)
	if step, _, _, ok := m.Onboarding(); !ok || step != 0 {
		t.Fatalf("after the error step %d (ok=%v), want 0", step, ok)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "отказано") {
		t.Errorf("frame without the host error:\n%s", frame)
	}
	m.Key("x", "")
	if frame := stripANSI(m.View()); strings.Contains(frame, "отказано") {
		t.Errorf("the error stayed after a key:\n%s", frame)
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
	if got := recorded[LoadModels](r); len(got) != 1 || got[0] != (LoadModels{Provider: "opencode-go", URL: "https://opencode.ai/inference/go/v1", Key: "k1"}) {
		t.Fatalf("want LoadModels{https://opencode.ai/inference/go/v1, k1}, recorded %v", r.acts)
	}
	steps(m, 3)
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubLoading {
		t.Fatalf("without a reply substep %d (ok=%v), want loading", sub, ok)
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
		t.Errorf("frame without a/x:\n%s", frame)
	}
	cnt := frameRow(frame, "с инструментами")
	if cnt == "" {
		t.Fatalf("frame without the counter row:\n%s", frame)
	}
	if !strings.Contains(cnt, "2") || !strings.Contains(cnt, "50") {
		t.Errorf("the counter without two models of 50: %q", cnt)
	}
}

func TestOnboardModelsFail(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Models(nil, 0, "ключ не подошёл")
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubKey {
		t.Fatalf("after the error substep %d (ok=%v), want the key", sub, ok)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "ключ не подошёл") {
		t.Errorf("frame without the error:\n%s", frame)
	}
}

func TestOnboardCustomURL(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("enter", "")
	m.StepDone(0, "")
	steps(m, 2)
	for range 7 {
		m.Key("down", "")
	}
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubProvider {
		t.Fatalf("substep %d (ok=%v), want the provider", sub, ok)
	}
	m.Key("enter", "")
	m.Key("", "localhost:8080/v1")
	m.Key("enter", "")
	m.Key("enter", "")
	if got := recorded[LoadModels](r); len(got) != 1 || got[0] != (LoadModels{Provider: "custom", URL: "https://localhost:8080/v1"}) {
		t.Fatalf("want LoadModels{https://localhost:8080/v1}, recorded %v", r.acts)
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
		t.Fatalf("want exactly one Setup, recorded %v", r.acts)
	}
	want := Setup{Lang: "ru", Provider: "opencode-go", URL: "https://opencode.ai/inference/go/v1", Key: "k1", Model: "a/x", Effort: "high", Lastfm: false}
	if setups[0] != want {
		t.Fatalf("Setup %+v, want %+v", setups[0], want)
	}
	for i := 0; i < 600 && m.Phase() != PhIntro; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("after the needle phase %v, want PhIntro", m.Phase())
	}
}

func TestOnboardConnectedCascade(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{Connected: [3]bool{true, false, true}}, r)
	steps(m, 0.6)
	if got := recorded[ConnectApple](r); len(got) != 0 {
		t.Fatalf("no action is expected from a connected step, recorded %v", r.acts)
	}
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("current step %d (ok=%v), want 1", step, ok)
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
		t.Fatalf("the onboarding did not finish without keys: phase %v", m.Phase())
	}
	setups := recorded[Setup](r)
	if len(setups) != 1 {
		t.Fatalf("want one Setup, recorded %v", r.acts)
	}
	if setups[0].URL != "" || setups[0].Key != "" || setups[0].Model != "" {
		t.Fatalf("Setup %+v, want empty URL/Key/Model", setups[0])
	}
}

func TestNoEggs(t *testing.T) {
	sky := []Track{{Artist: "Playboi Carti", Title: "Sky", Album: "Whole Lotta Red", Dur: 190}}
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(sky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "†") {
		t.Errorf("egg frame without the † knob:\n%s", frame)
	}
	m = New(Options{NoEggs: true})
	m.SetSize(110, 32)
	m.JumpPlay(sky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); strings.Contains(frame, "†") {
		t.Errorf("frame without eggs with the † knob:\n%s", frame)
	}
}

func TestOnboardConnectedWaitsLang(t *testing.T) {
	m := New(Options{Onboard: true, Connected: [3]bool{true, false, false}})
	m.SetSize(110, 32)
	steps(m, obReady+2)
	if step, _, _, _ := m.Onboarding(); step != 0 {
		t.Fatalf("step %d before the language choice, want 0", step)
	}
	m.Key("enter", "")
	steps(m, 0.6)
	if step, _, _, _ := m.Onboarding(); step != 1 {
		t.Fatalf("step %d after the language choice, want 1", step)
	}
}

func TestOnboardLastfmSkipWhileWaiting(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{Connected: [3]bool{true, true, false}}, r)
	steps(m, 1.5)
	m.Key("enter", "")
	if got := recorded[ConnectLastfm](r); len(got) != 1 {
		t.Fatalf("want ConnectLastfm, recorded %v", r.acts)
	}
	m.Key("tab", "")
	m.StepDone(2, "")
	steps(m, 0.2)
	setups := recorded[Setup](r)
	if len(setups) != 1 || setups[0].Lastfm {
		t.Fatalf("want one Setup without Last.fm, recorded %v", r.acts)
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
		t.Fatalf("picked %q, want the recommended b/y", m.ob.picked)
	}
}

// R1: the URL and key substeps render on a narrow screen without panicking.
func TestOnboardNarrowNoPanic(t *testing.T) {
	sizes := [][2]int{{20, 5}, {24, 6}, {30, 8}, {34, 8}, {40, 10}}
	for _, sz := range sizes {
		m := liveOnboard(t, Options{}, &recorder{})
		toAgent(m)
		m.Key("enter", "")
		if _, sub, _, ok := m.Onboarding(); !ok || sub != SubKey {
			t.Fatalf("%dx%d: substep %d (ok=%v), want the key", sz[0], sz[1], sub, ok)
		}
		m.SetSize(sz[0], sz[1])
		m.View()

		m = liveOnboard(t, Options{}, &recorder{})
		toAgent(m)
		for i := 0; i < len(providers)-1; i++ {
			m.Key("down", "")
		}
		m.Key("enter", "")
		if _, sub, _, ok := m.Onboarding(); !ok || sub != SubURL {
			t.Fatalf("%dx%d: substep %d (ok=%v), want the address", sz[0], sz[1], sub, ok)
		}
		m.SetSize(sz[0], sz[1])
		m.View()
	}
}

// R2: the model filter matches the ID in any case.
func TestOnboardFilterCase(t *testing.T) {
	for _, q := range []string{"llama", "Llama-3.1"} {
		m := liveOnboard(t, Options{}, &recorder{})
		modelsStep(t, m)
		m.Models([]ModelInfo{{ID: "Meta-Llama/Llama-3.1-70B"}, {ID: "a/x"}}, 2, "")
		steps(m, 1.5)
		m.Key("", q)
		if frame := stripANSI(m.View()); !strings.Contains(frame, "Meta-Llama/Llama-3.1-70B") {
			t.Errorf("filter %q: frame without Meta-Llama/Llama-3.1-70B:\n%s", q, frame)
		}
	}
}

// R3: esc on the loading substep goes back to the key, enter asks for the list again.
func TestOnboardLoadingEsc(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	steps(m, 1)
	m.Key("esc", "")
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubKey {
		t.Fatalf("after esc substep %d (ok=%v), want the key", sub, ok)
	}
	m.Key("enter", "")
	if got := recorded[LoadModels](r); len(got) != 2 {
		t.Fatalf("want two LoadModels, recorded %v", r.acts)
	}
}

// R4: the answer to the abandoned load does not move past the key.
func TestOnboardLoadingEscLateReply(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	modelsStep(t, m)
	m.Key("esc", "")
	m.Models([]ModelInfo{{ID: "a/x"}}, 1, "")
	steps(m, 2)
	if _, sub, _, ok := m.Onboarding(); !ok || sub != SubKey {
		t.Fatalf("late reply: substep %d (ok=%v), want the key", sub, ok)
	}
}

// R1: tab on the intro sends Settings and leaves the input empty.
func TestIntroTabSettings(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	if !runTo(m, PhIntro, 300) {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	m.Key("tab", "")
	if got := recorded[Settings](r); len(got) != 1 {
		t.Fatalf("want one Settings, recorded %v", r.acts)
	}
	if m.Input() != "" {
		t.Fatalf("Input()=%q, want an empty field", m.Input())
	}
}

// R2: Onboard reopens the onboarding over the intro; the connected Apple step closes
// itself once the preferences are in — language, and the eggs in the settings — and the
// screen stops on the agent step.
func TestOnboardReopen(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	if !runTo(m, PhIntro, 300) {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	m.Onboard([3]bool{true, false, false})
	if m.Phase() != PhOnboard {
		t.Fatalf("phase %v, want PhOnboard", m.Phase())
	}
	for i := 0; i < 400 && m.ob.t < obReady; i++ {
		m.Step(eventDT)
	}
	m.Key("enter", "") // language
	m.Key("enter", "") // the eggs switch
	steps(m, 3)
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("after the settings step %d (ok=%v), want 1", step, ok)
	}
}

// R4: the intro shows the settings hint in the bottom right corner.
func TestIntroSettingsHint(t *testing.T) {
	langEN(t)
	m := New(Options{})
	m.SetSize(110, 32)
	steps(m, 3)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "tab settings") {
		t.Errorf("frame without the tab settings hint:\n%s", frame)
	}
}

func TestOnboardPasteKey(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	toAgent(m)
	m.Key("enter", "")
	m.Paste("  sk-or-v1-abc\n")
	m.Key("enter", "")
	if got := recorded[LoadModels](r); len(got) != 1 || got[0].Key != "sk-or-v1-abc" {
		t.Fatalf("want LoadModels with the key sk-or-v1-abc, recorded %v", r.acts)
	}
}

func TestIntroPaste(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	if !runTo(m, PhIntro, 300) {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	m.Paste("lofi\nна вечер\t")
	if m.Input() != "lofi на вечер" {
		t.Fatalf("Input()=%q, want «lofi на вечер»", m.Input())
	}
}

func TestOnboardPasteFilter(t *testing.T) {
	m := liveOnboard(t, Options{}, &recorder{})
	modelsStep(t, m)
	m.Models([]ModelInfo{{ID: "Meta-Llama/Llama-3.1-70B"}, {ID: "a/x"}}, 2, "")
	steps(m, 1.5)
	m.Paste("llama\n")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "Meta-Llama/Llama-3.1-70B") || strings.Contains(frame, "a/x") {
		t.Fatalf("the paste into the filter did not work:\n%s", frame)
	}
}

// lastfmKeys lands on the Last.fm step of an onboarding that asks for the app keys.
func lastfmKeys(t *testing.T, key string, r *recorder) *Model {
	t.Helper()
	m := liveOnboard(t, Options{Connected: [3]bool{true, true, false}, LastfmAskKeys: true, LastfmKey: key}, r)
	steps(m, 1.5)
	return m
}

func TestLastfmAskKeys(t *testing.T) {
	r := &recorder{}
	m := lastfmKeys(t, "k1", r)
	frame := stripANSI(m.View())
	for _, want := range []string{"API key", "secret", "k1"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("frame without %q:\n%s", want, frame)
		}
	}
	m.Key("enter", "")
	if got := recorded[ConnectLastfm](r); len(got) != 0 {
		t.Fatalf("enter on the key must not connect, recorded %v", r.acts)
	}
	m.Paste("s3cret\n")
	frame = stripANSI(m.View())
	if strings.Contains(frame, "s3cret") {
		t.Fatalf("the secret is visible in plain text:\n%s", frame)
	}
	if !strings.Contains(frame, "••••••") {
		t.Fatalf("the secret is not masked:\n%s", frame)
	}
	m.Key("enter", "")
	want := []ConnectLastfm{{Key: "k1", Secret: "s3cret"}}
	if got := recorded[ConnectLastfm](r); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
}

func TestLastfmAskKeysEmpty(t *testing.T) {
	r := &recorder{}
	m := lastfmKeys(t, "", r)
	m.Key("enter", "")
	if got := recorded[ConnectLastfm](r); len(got) != 0 {
		t.Fatalf("an empty key must not connect, recorded %v", r.acts)
	}
	m.Key("", "k")
	m.Key("enter", "")
	m.Key("enter", "")
	if got := recorded[ConnectLastfm](r); len(got) != 0 {
		t.Fatalf("an empty secret must not connect, recorded %v", r.acts)
	}
	m.Key("", "s")
	m.Key("enter", "")
	want := []ConnectLastfm{{Key: "k", Secret: "s"}}
	if got := recorded[ConnectLastfm](r); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
}

func TestLastfmAskKeysEsc(t *testing.T) {
	r := &recorder{}
	m := lastfmKeys(t, "k1", r)
	m.Key("enter", "")
	m.Key("esc", "")
	if got := recorded[Quit](r); len(got) != 0 {
		t.Fatalf("esc on the secret must not quit, recorded %v", r.acts)
	}
	m.Key("backspace", "")
	m.Key("", "2")
	m.Key("enter", "")
	m.Key("", "s")
	m.Key("enter", "")
	want := []ConnectLastfm{{Key: "k2", Secret: "s"}}
	if got := recorded[ConnectLastfm](r); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded %v, want %v", got, want)
	}
}

func TestLastfmAskKeysRetry(t *testing.T) {
	r := &recorder{}
	m := lastfmKeys(t, "k1", r)
	m.Key("enter", "")
	m.Paste("s3cret")
	m.Key("enter", "")
	first := recorded[ConnectLastfm](r)
	if len(first) != 1 {
		t.Fatalf("want one ConnectLastfm, recorded %v", r.acts)
	}
	m.StepDone(2, "bad key")
	steps(m, 0.2)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "bad key") {
		t.Fatalf("frame without the error:\n%s", frame)
	}
	m.Key("enter", "")
	got := recorded[ConnectLastfm](r)
	if len(got) != 2 || got[1] != got[0] {
		t.Fatalf("recorded %v, want a retry %v", got, first[0])
	}
}

func TestLastfmAskKeysSkip(t *testing.T) {
	r := &recorder{}
	m := lastfmKeys(t, "k1", r)
	m.Key("tab", "")
	steps(m, 1)
	setups := recorded[Setup](r)
	if len(setups) != 1 || setups[0].Lastfm {
		t.Fatalf("want one Setup without Last.fm, recorded %v", r.acts)
	}
}

// liveSettings reopens the onboarding the way the settings do: a model that reached the
// intro calls Onboard, the cinematic runs past obReady and the language is confirmed.
func liveSettings(t *testing.T, o Options, r *recorder) *Model {
	t.Helper()
	o.OnAction = r.on
	m := New(o)
	m.SetSize(110, 32)
	if !runTo(m, PhIntro, 300) {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	m.Onboard([3]bool{true, false, false})
	for i := 0; i < 400 && m.ob.t < obReady; i++ {
		m.Step(eventDT)
	}
	m.Key("enter", "")
	return m
}

var wlrSky = []Track{{Artist: "Playboi Carti", Title: "Sky", Album: "Whole Lotta Red", Dur: 193}}

// 1: the settings carry the eggs row after the language; the first run has none.
func TestSettingsEggsRow(t *testing.T) {
	r := &recorder{}
	m := liveSettings(t, Options{}, r)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "Пасхалки") {
		t.Errorf("settings frame without «Пасхалки»:\n%s", frame)
	}

	m = liveOnboard(t, Options{}, r)
	if frame := stripANSI(m.View()); strings.Contains(frame, "Пасхалки") || strings.Contains(frame, "Easter eggs") {
		t.Errorf("first-run frame with eggs:\n%s", frame)
	}
}

// 2: right + enter switch the eggs off, the egg is gone from the playing track.
func TestSettingsEggsOff(t *testing.T) {
	r := &recorder{}
	m := liveSettings(t, Options{}, r)
	m.Key("right", "")
	m.Key("enter", "")
	if eggs := recorded[Eggs](r); len(eggs) != 1 || eggs[0] != (Eggs{On: false}) {
		t.Fatalf("want Eggs{On: false}, recorded %v", r.acts)
	}
	m.JumpPlay(wlrSky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); strings.Contains(frame, "†") {
		t.Errorf("frame with an egg after switching off:\n%s", frame)
	}
}

// 3: with NoEggs the switch starts off; in a reopened settings it comes back on and the egg returns.
func TestSettingsEggsOn(t *testing.T) {
	r := &recorder{}
	m := liveSettings(t, Options{NoEggs: true}, r)
	m.Key("enter", "")
	if eggs := recorded[Eggs](r); len(eggs) != 1 || eggs[0] != (Eggs{On: false}) {
		t.Fatalf("want Eggs{On: false}, recorded %v", r.acts)
	}

	m.Onboard([3]bool{true, false, false})
	for i := 0; i < 400 && m.ob.t < obReady; i++ {
		m.Step(eventDT)
	}
	m.Key("enter", "")
	m.Key("left", "")
	m.Key("enter", "")
	if eggs := recorded[Eggs](r); len(eggs) != 2 || eggs[1] != (Eggs{On: true}) {
		t.Fatalf("want a second Eggs{On: true}, recorded %v", r.acts)
	}
	m.JumpPlay(wlrSky, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "†") {
		t.Errorf("frame without an egg after switching on:\n%s", frame)
	}
}

// 4: in the settings the connected Apple Music step waits for the eggs too.
func TestSettingsEggsGate(t *testing.T) {
	r := &recorder{}
	m := liveSettings(t, Options{}, r)
	steps(m, 1)
	if got := recorded[ConnectApple](r); len(got) != 0 {
		t.Fatalf("Apple Music connected by itself: %v", r.acts)
	}
	if step, _, _, ok := m.Onboarding(); !ok || step != 0 {
		t.Fatalf("step %d (ok=%v) before the eggs choice, want 0", step, ok)
	}
	if frame := stripANSI(m.View()); strings.Contains(frame, "подключено") {
		t.Fatalf("Apple Music closed by itself:\n%s", frame)
	}
	m.Key("enter", "")
	steps(m, 1)
	if step, _, _, ok := m.Onboarding(); !ok || step != 1 {
		t.Fatalf("step %d (ok=%v) after the eggs, want 1", step, ok)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "подключено") {
		t.Fatalf("frame without «подключено»:\n%s", frame)
	}
}

// 5: the confirmed eggs row shows the status next to it.
func TestSettingsEggsStatus(t *testing.T) {
	r := &recorder{}
	m := liveSettings(t, Options{}, r)
	m.Key("enter", "")
	frame := stripANSI(m.View())
	if got := recorded[ConnectApple](r); len(got) != 0 {
		t.Fatalf("enter on the eggs must not open Apple Music: %v", r.acts)
	}
	if !strings.Contains(frame, "Пасхалки") || !strings.Contains(frame, "вкл") {
		t.Fatalf("frame without «Пасхалки»/«вкл»:\n%s", frame)
	}
}

// 6: switching the eggs off on a playing track drops the cached egg theme.
func TestSettingsEggsTheme(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(wlrSky, 0, 0)
	steps(m, 2)
	if m.to.a1 != hex(0xe3242b) {
		t.Fatalf("a1=%v, want the egg colour #e3242b", m.to.a1)
	}
	m.Onboard([3]bool{true, false, false})
	for i := 0; i < 400 && m.ob.t < obReady; i++ {
		m.Step(eventDT)
	}
	m.Key("enter", "")
	m.Key("right", "")
	m.Key("enter", "")
	if got := recorded[Eggs](r); len(got) != 1 || got[0] != (Eggs{On: false}) {
		t.Fatalf("want Eggs{On: false}, recorded %v", r.acts)
	}
	m.JumpPlay(wlrSky, 0, 0)
	if m.to.a1 == hex(0xe3242b) {
		t.Fatalf("a1=%v, the egg theme came back from the cache", m.to.a1)
	}
}
