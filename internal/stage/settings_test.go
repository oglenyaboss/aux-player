package stage

import (
	"strings"
	"testing"
)

// settingsSetup is a model in English, at 100x40, with a recorder for its actions.
func settingsSetup(t *testing.T) (*Model, *recorder) {
	t.Helper()
	prev := lang
	SetLang("en")
	t.Cleanup(func() { SetLang(prev) })
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(100, 40)
	return m, r
}

func settingsInfo() SettingsInfo {
	return SettingsInfo{
		Provider: "commandcode",
		URL:      "https://api.commandcode.ai/provider/v1",
		Model:    "deepseek/deepseek-v4.1-flash",
		Keys:     map[string]bool{"commandcode": true},
		Lang:     "en",
		Eggs:     true,
	}
}

func settingsFrame(m *Model) string { return stripANSI(m.View()) }

func settingsDown(m *Model, n int) {
	for range n {
		m.Key("down", "")
	}
}

// S1: the overlay opens over the intro with the active agent and the preferences.
func TestSettingsFrame(t *testing.T) {
	m, _ := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	if !m.SettingsOpen() {
		t.Fatal("OpenSettings did not open the overlay")
	}
	frame := settingsFrame(m)
	for _, want := range []string{"settings", "provider", "Command Code", "deepseek/deepseek-v4.1-flash", "language", "English", "remove key"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the settings frame lacks %q:\n%s", want, frame)
		}
	}
}

// S2: esc closes the overlay, the phase stays and nothing goes out.
func TestSettingsEscCloses(t *testing.T) {
	m, r := settingsSetup(t)
	ph := m.Phase()
	m.OpenSettings(settingsInfo())
	m.Key("esc", "")
	if m.SettingsOpen() {
		t.Fatal("esc did not close the settings")
	}
	if m.Phase() != ph {
		t.Fatalf("phase = %v, want %v", m.Phase(), ph)
	}
	if len(r.acts) != 0 {
		t.Fatalf("esc sent %v, want nothing", r.acts)
	}
}

// S3: enter on the intro belongs to the overlay, not to the intro input.
func TestSettingsEnterStays(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	m.Key("enter", "")
	if len(r.acts) != 0 {
		t.Fatalf("enter sent %v, want nothing", r.acts)
	}
	if m.Phase() != PhIntro {
		t.Fatalf("phase = %v, want the intro", m.Phase())
	}
}

// S4: a new provider asks for its key, loads the models and sets the agent.
func TestSettingsProviderFlow(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	m.Key("enter", "")
	m.Key("down", "")
	m.Key("enter", "")
	if m.settings.form == nil || m.settings.form.sub != SubKey {
		t.Fatalf("the form is not at the key: %+v", m.settings.form)
	}
	m.Key("", "k")
	m.Key("enter", "")
	loaded := recorded[LoadModels](r)
	wantLoad := LoadModels{Provider: "openrouter", URL: "https://openrouter.ai/api/v1", Key: "k"}
	if len(loaded) != 1 || loaded[0] != wantLoad {
		t.Fatalf("LoadModels = %+v, want [%+v]", loaded, wantLoad)
	}
	m.Models([]ModelInfo{{ID: "a/x"}}, 1, "")
	steps(m, 1.4)
	m.Key("enter", "")
	sets := recorded[AgentSet](r)
	wantSet := AgentSet{Provider: "openrouter", URL: "https://openrouter.ai/api/v1", Key: "k", Model: "a/x"}
	if len(sets) != 1 || sets[0] != wantSet {
		t.Fatalf("AgentSet = %+v, want [%+v]", sets, wantSet)
	}
	if m.settings.form != nil {
		t.Fatal("the list did not come back after the form")
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "OpenRouter") {
		t.Errorf("the frame does not show the new provider:\n%s", frame)
	}
}

// S5: a provider with a stored key skips the key and loads the models with it.
func TestSettingsSavedProvider(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	m.Key("enter", "")
	m.Key("enter", "")
	loaded := recorded[LoadModels](r)
	want := LoadModels{Provider: "commandcode", URL: "https://api.commandcode.ai/provider/v1"}
	if len(loaded) != 1 || loaded[0] != want {
		t.Fatalf("LoadModels = %+v, want [%+v]", loaded, want)
	}
	if m.settings.form == nil || m.settings.form.sub != SubLoading {
		t.Fatalf("the form is not loading: %+v", m.settings.form)
	}
}

// S6: the model row loads the stored-key models and puts the cursor on the current one.
func TestSettingsModelRow(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 2)
	m.Key("enter", "")
	loaded := recorded[LoadModels](r)
	wantLoad := LoadModels{Provider: "commandcode", URL: "https://api.commandcode.ai/provider/v1"}
	if len(loaded) != 1 || loaded[0] != wantLoad {
		t.Fatalf("LoadModels = %+v, want [%+v]", loaded, wantLoad)
	}
	m.Models([]ModelInfo{{ID: "x"}, {ID: "deepseek/deepseek-v4.1-flash"}}, 2, "")
	steps(m, 1.4)
	if m.settings.form == nil || m.settings.form.sub != SubModel {
		t.Fatalf("the form is not on the models: %+v", m.settings.form)
	}
	m.Key("enter", "")
	sets := recorded[AgentSet](r)
	if len(sets) != 1 || sets[0].Provider != "commandcode" || sets[0].Model != "deepseek/deepseek-v4.1-flash" || sets[0].Key != "" {
		t.Fatalf("AgentSet = %+v, want the current model of commandcode", sets)
	}
}

// S7: a rejected key offers to save it anyway; tab sets it without the model.
func TestSettingsSaveAnyway(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 1)
	m.Key("enter", "")
	m.Key("", "bad")
	m.Key("enter", "")
	m.Models(nil, 0, "key rejected")
	frame := settingsFrame(m)
	for _, want := range []string{"key rejected", "save anyway"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the frame lacks %q:\n%s", want, frame)
		}
	}
	m.Key("tab", "")
	sets := recorded[AgentSet](r)
	want := AgentSet{Provider: "commandcode", URL: "https://api.commandcode.ai/provider/v1", Key: "bad"}
	if len(sets) != 1 || sets[0] != want {
		t.Fatalf("AgentSet = %+v, want [%+v]", sets, want)
	}
	if m.settings.form != nil {
		t.Fatal("the list did not come back after the save")
	}
}

// S8: the reasoning row steps to the next level.
func TestSettingsEffortRow(t *testing.T) {
	m, r := settingsSetup(t)
	info := settingsInfo()
	info.Effort = "low"
	m.OpenSettings(info)
	settingsDown(m, 3)
	m.Key("enter", "")
	sets := recorded[AgentSet](r)
	want := AgentSet{Provider: "commandcode", Model: "deepseek/deepseek-v4.1-flash", Effort: "medium"}
	if len(sets) != 1 || sets[0] != want {
		t.Fatalf("AgentSet = %+v, want [%+v]", sets, want)
	}
}

// S9: removing the key is armed by the first enter and done by the second.
func TestSettingsForgetKey(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 3)
	m.Key("enter", "")
	if got := recorded[AgentForget](r); len(got) != 0 {
		t.Fatalf("the first enter forgot the key: %v", r.acts)
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "enter again to remove") {
		t.Fatalf("the frame does not ask to confirm:\n%s", frame)
	}
	m.Key("enter", "")
	forgets := recorded[AgentForget](r)
	if len(forgets) != 1 || forgets[0].Provider != "commandcode" {
		t.Fatalf("AgentForget = %+v, want commandcode", forgets)
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "none") {
		t.Errorf("the key row does not say none:\n%s", frame)
	}
	if !m.o.NoAgent {
		t.Error("the removed key did not switch the agent off")
	}
}

// S10: the language row switches the language and the overlay at once.
func TestSettingsLangRow(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 4)
	m.Key("enter", "")
	langs := recorded[LangSet](r)
	if len(langs) != 1 || langs[0].Lang != "ru" {
		t.Fatalf("LangSet = %+v, want ru", langs)
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "настройки") {
		t.Errorf("the frame did not switch to Russian:\n%s", frame)
	}
}

// S11: the eggs row flips the switch and mirrors it in the options.
func TestSettingsEggsSwitch(t *testing.T) {
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 5)
	m.Key("enter", "")
	eggs := recorded[Eggs](r)
	if len(eggs) != 1 || eggs[0].On {
		t.Fatalf("Eggs = %+v, want off", eggs)
	}
	if !m.o.NoEggs {
		t.Error("the options did not turn the eggs off")
	}
}

// S12: without an agent only the provider and the preferences are listed.
func TestSettingsWithoutAgent(t *testing.T) {
	m, _ := settingsSetup(t)
	m.OpenSettings(SettingsInfo{Lang: "en"})
	frame := settingsFrame(m)
	if !strings.Contains(frame, "not connected") {
		t.Errorf("the frame does not say not connected:\n%s", frame)
	}
	if strings.Contains(frame, "model") || strings.Contains(frame, "remove key") {
		t.Errorf("the frame has agent rows without an agent:\n%s", frame)
	}
}

// S13: s in the player and the palette actions menu both ask for the settings.
func TestSettingsEntries(t *testing.T) {
	m, r := settingsSetup(t)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("s", "s")
	if got := recorded[Settings](r); len(got) != 1 {
		t.Fatalf("s in the player sent %v, want Settings", r.acts)
	}

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	m2.OpenMusic()
	steps(m2, 0.2)
	m2.Key("ctrl+k", "")
	if !m2.mu.menu.open {
		t.Fatal("ctrl+k did not open the menu on the root")
	}
	items := m2.mlMenuItems()
	if len(items) == 0 || items[len(items)-1].kind != mlMenuSettings {
		t.Fatalf("menu = %+v, want the settings entry", items)
	}
	m2.Key("enter", "")
	if got := recorded[Settings](r2); len(got) != 1 {
		t.Fatalf("the menu sent %v, want Settings", r2.acts)
	}
	if m2.mu.open {
		t.Error("the palette stayed open after the settings entry")
	}
}

// S14: the onboarding owns the screen, the settings stay closed there.
func TestSettingsIgnoredOnOnboard(t *testing.T) {
	m, _ := settingsSetup(t)
	m.Onboard([3]bool{true, false, false})
	m.OpenSettings(settingsInfo())
	if m.SettingsOpen() {
		t.Fatal("OpenSettings opened over the onboarding")
	}
}
