package stage

import (
	"strings"
	"testing"
)

func agentFormAt(t *testing.T) (*Model, *recorder, *agentForm) {
	t.Helper()
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
	r := &recorder{}
	return New(Options{OnAction: r.on}), r, newAgentForm()
}

func formToLoading(t *testing.T, m *Model, a *agentForm) {
	t.Helper()
	if got := m.formKey(a, "enter", ""); got != formNone || a.sub != SubKey {
		t.Fatalf("provider enter: result %v sub %v, want formNone/SubKey", got, a.sub)
	}
	m.formKey(a, "", "k")
	if got := m.formKey(a, "enter", ""); got != formNone || a.sub != SubLoading {
		t.Fatalf("key enter: result %v sub %v, want formNone/SubLoading", got, a.sub)
	}
}

func TestAgentFormKeyFlow(t *testing.T) {
	m, r, a := agentFormAt(t)
	if got := m.formKey(a, "enter", ""); got != formNone {
		t.Fatalf("provider enter = %v, want formNone", got)
	}
	if a.sub != SubKey {
		t.Fatalf("sub = %v, want SubKey", a.sub)
	}
	m.formKey(a, "", "k")
	if got := m.formKey(a, "enter", ""); got != formNone {
		t.Fatalf("key enter = %v, want formNone", got)
	}
	if a.sub != SubLoading {
		t.Fatalf("sub = %v, want SubLoading", a.sub)
	}
	loaded := recorded[LoadModels](r)
	want := LoadModels{Provider: "opencode-go", URL: "https://opencode.ai/inference/go/v1", Key: "k"}
	if len(loaded) != 1 || loaded[0] != want {
		t.Fatalf("LoadModels = %+v, want [%+v]", loaded, want)
	}
}

func TestAgentFormModelPick(t *testing.T) {
	m, _, a := agentFormAt(t)
	formToLoading(t, m, a)
	a.reply([]ModelInfo{{ID: "a/x"}}, 3, "")
	a.step(1.3)
	if a.sub != SubModel {
		t.Fatalf("sub = %v, want SubModel", a.sub)
	}
	if got := m.formKey(a, "enter", ""); got != formDone {
		t.Fatalf("model enter = %v, want formDone", got)
	}
	provider, url, key, model, effort := a.result()
	if provider != "opencode-go" || url != "https://opencode.ai/inference/go/v1" || key != "k" || model != "a/x" || effort != "" {
		t.Fatalf("result = %q %q %q %q %q", provider, url, key, model, effort)
	}
}

func TestAgentFormEffort(t *testing.T) {
	m, _, a := agentFormAt(t)
	formToLoading(t, m, a)
	a.reply([]ModelInfo{{ID: "r/y", Reasoning: true}}, 1, "")
	a.step(1.3)
	if got := m.formKey(a, "enter", ""); got != formNone {
		t.Fatalf("model enter = %v, want formNone", got)
	}
	if a.sub != SubEffort {
		t.Fatalf("sub = %v, want SubEffort", a.sub)
	}
	m.formKey(a, "right", "")
	if got := m.formKey(a, "enter", ""); got != formDone {
		t.Fatalf("effort enter = %v, want formDone", got)
	}
	if _, _, _, _, effort := a.result(); effort != "high" {
		t.Fatalf("effort = %q, want high", effort)
	}
}

func TestAgentFormEsc(t *testing.T) {
	m, _, a := agentFormAt(t)
	if got := m.formKey(a, "esc", ""); got != formBack {
		t.Fatalf("provider esc = %v, want formBack", got)
	}
	m.formKey(a, "enter", "")
	if a.sub != SubKey {
		t.Fatalf("sub = %v, want SubKey", a.sub)
	}
	if got := m.formKey(a, "esc", ""); got != formNone {
		t.Fatalf("key esc = %v, want formNone", got)
	}
	if a.sub != SubProvider {
		t.Fatalf("sub = %v, want SubProvider", a.sub)
	}
}

func TestAgentFormReplyError(t *testing.T) {
	m, _, a := agentFormAt(t)
	formToLoading(t, m, a)
	a.reply(nil, 0, "key rejected")
	if a.sub != SubKey {
		t.Fatalf("sub = %v, want SubKey", a.sub)
	}
	if a.errMsg != "key rejected" {
		t.Fatalf("errMsg = %q, want key rejected", a.errMsg)
	}
}

func TestAgentFormDrawWithoutOnboard(t *testing.T) {
	m, _, a := agentFormAt(t)
	a.list = []ModelInfo{{ID: "a/x", Reasoning: true}}
	a.picked = "a/x"
	for _, s := range []agentSub{SubProvider, SubURL, SubKey, SubLoading, SubModel, SubEffort} {
		a.setSub(s)
		f := newFB(70, 24)
		m.drawAgentForm(f, a, 1, 1, 60, 22, false, 1)
	}
}

func TestAgentFormDraw(t *testing.T) {
	m, _, a := agentFormAt(t)
	f := newFB(70, 24)
	m.drawAgentForm(f, a, 1, 1, 60, 22, false, 1)
	frame := stripANSI(f.String())
	for _, want := range []string{"OpenCode Go", "Command Code"} {
		if !strings.Contains(frame, want) {
			t.Errorf("no-intro frame lacks %q:\n%s", want, frame)
		}
	}
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, "heart of AUX") {
			t.Errorf("no-intro frame has the heart line:\n%s", frame)
		}
	}
	f = newFB(70, 24)
	m.drawAgentForm(f, a, 1, 1, 60, 22, true, 1)
	found := false
	for _, line := range strings.Split(stripANSI(f.String()), "\n") {
		if strings.Contains(line, "heart of AUX") {
			found = true
		}
	}
	if !found {
		t.Errorf("intro frame lacks the heart line:\n%s", stripANSI(f.String()))
	}
}

// formToProvider moves the provider cursor to id.
func formToProvider(t *testing.T, m *Model, a *agentForm, id string) {
	t.Helper()
	for range len(providers) {
		if providers[a.prov].id == id {
			return
		}
		m.formKey(a, "down", "")
	}
	t.Fatalf("the provider cursor is on %q, want %q", providers[a.prov].id, id)
}

// G1: the subscription provider signs in through the browser, not a key.
func TestAgentFormChatGPTFlow(t *testing.T) {
	m, r, a := agentFormAt(t)
	formToProvider(t, m, a, "chatgpt")
	f := newFB(70, 24)
	m.drawAgentForm(f, a, 1, 1, 60, 22, false, 1)
	if frame := stripANSI(f.String()); !strings.Contains(frame, "unofficial, via the Codex sign-in") {
		t.Errorf("the form lacks the Codex warning:\n%s", frame)
	}
	if got := m.formKey(a, "enter", ""); got != formNone || a.sub != SubLoading {
		t.Fatalf("provider enter: result %v sub %v, want formNone/SubLoading", got, a.sub)
	}
	loaded := recorded[LoadModels](r)
	want := LoadModels{Provider: "chatgpt", URL: "https://chatgpt.com/backend-api/codex"}
	if len(loaded) != 1 || loaded[0] != want {
		t.Fatalf("LoadModels = %+v, want [%+v]", loaded, want)
	}
	f = newFB(70, 24)
	m.drawAgentForm(f, a, 1, 1, 60, 22, false, 1)
	if frame := stripANSI(f.String()); !strings.Contains(frame, "sign in to ChatGPT in the browser") {
		t.Errorf("the loading frame lacks the browser sign-in:\n%s", frame)
	}
}

// G2: a failed sign-in returns to the provider list with the error under it.
func TestAgentFormChatGPTReplyError(t *testing.T) {
	m, _, a := agentFormAt(t)
	formToProvider(t, m, a, "chatgpt")
	m.formKey(a, "enter", "")
	a.reply(nil, 0, "sign-in failed")
	if a.sub != SubProvider {
		t.Fatalf("sub = %v, want SubProvider", a.sub)
	}
	f := newFB(70, 24)
	m.drawAgentForm(f, a, 1, 1, 60, 22, false, 1)
	if frame := stripANSI(f.String()); !strings.Contains(frame, "sign-in failed") {
		t.Errorf("the provider frame lacks the error:\n%s", frame)
	}
}

// G3: the subscription form settles with an empty key and the Codex URL.
func TestAgentFormChatGPTResult(t *testing.T) {
	m, _, a := agentFormAt(t)
	formToProvider(t, m, a, "chatgpt")
	m.formKey(a, "enter", "")
	a.reply([]ModelInfo{{ID: "gpt-6-luna", Reasoning: true, Rec: true}}, 1, "")
	a.step(1.4)
	if got := m.formKey(a, "enter", ""); got != formNone || a.sub != SubEffort {
		t.Fatalf("model enter: result %v sub %v, want formNone/SubEffort", got, a.sub)
	}
	if got := m.formKey(a, "enter", ""); got != formDone {
		t.Fatalf("effort enter = %v, want formDone", got)
	}
	provider, url, key, model, effort := a.result()
	if provider != "chatgpt" || url != "https://chatgpt.com/backend-api/codex" || key != "" || model != "gpt-6-luna" || effort != "low" {
		t.Fatalf("result = %q %q %q %q %q", provider, url, key, model, effort)
	}
}
