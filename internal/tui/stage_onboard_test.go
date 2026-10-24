package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/simone-vibes/vibez/internal/config"
	"github.com/simone-vibes/vibez/internal/stage"
	"github.com/simone-vibes/vibez/internal/vibeai/llm"
)

// ruLang pins the scene language for the Russian assertions and restores it after.
func ruLang(t *testing.T) {
	t.Helper()
	stage.SetLang("ru")
	t.Cleanup(func() { stage.SetLang("ru") })
}

// isolatedHome keeps the config Save calls away from the owner's real config.
func isolatedHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
}

// setListModels replaces the model-list fetch for the test and restores it.
func setListModels(t *testing.T, f func(context.Context, *http.Client, string, string) ([]llm.ModelInfo, error)) {
	t.Helper()
	orig := stageListModels
	stageListModels = f
	t.Cleanup(func() { stageListModels = orig })
}

// setLastfmAuth replaces the Last.fm flow for the test and restores it.
func setLastfmAuth(t *testing.T, f func(context.Context, string, string) (string, error)) {
	t.Helper()
	orig := stageLastfmAuth
	stageLastfmAuth = f
	t.Cleanup(func() { stageLastfmAuth = orig })
}

// obStepUntil advances the scene in 33 ms ticks while cond is false, up to seconds.
func obStepUntil(st *stage.Model, seconds float64, cond func() bool) {
	for i := 0; i < int(seconds/0.033) && !cond(); i++ {
		st.Step(0.033)
	}
}

func TestStageOptions(t *testing.T) {
	o := stageOptions(&config.Config{})
	if !o.Onboard || o.Connected != [3]bool{true, false, false} || o.NoEggs {
		t.Fatalf("empty config: onboard=%v connected=%v noEggs=%v, want true/[true false false]/false",
			o.Onboard, o.Connected, o.NoEggs)
	}
	off := false
	o = stageOptions(&config.Config{VibeLLMAPIKey: "k", LastfmSessionKey: "s", VibeEggs: &off})
	if o.Onboard || o.Connected != [3]bool{true, false, true} || !o.NoEggs {
		t.Fatalf("filled config: onboard=%v connected=%v noEggs=%v, want false/[true false true]/true",
			o.Onboard, o.Connected, o.NoEggs)
	}
}

func TestStageOnboardGate(t *testing.T) {
	isolatedHome(t)
	cfg := testCfg()
	cfg.VibeLLMAPIKey = ""
	m := New(cfg, &mockProvider{}, nil, Options{})
	if phase := m.vibeAIP.m.st.Phase(); phase != stage.PhOnboard {
		t.Fatalf("phase without an agent key = %v, want onboarding", phase)
	}
	if phase := newModel(nil).vibeAIP.m.st.Phase(); phase == stage.PhOnboard {
		t.Fatalf("phase with an agent key = %v, want not onboarding", phase)
	}
}

func TestStageLangFromConfig(t *testing.T) {
	ruLang(t)
	isolatedHome(t)
	t.Setenv("LANG", "ru_RU.UTF-8") // the config lang must win over the environment
	cfg := testCfg()
	cfg.VibeLang = "en"
	m := New(cfg, &mockProvider{}, nil, Options{})
	_ = m
	if got := stage.Tr("а", "b"); got != "b" {
		t.Fatalf("Tr after a config lang of en = %q, want b", got)
	}
}

func TestStageLoadModels(t *testing.T) {
	ruLang(t)
	isolatedHome(t)
	var gotURL, gotKey string
	setListModels(t, func(_ context.Context, _ *http.Client, baseURL, apiKey string) ([]llm.ModelInfo, error) {
		gotURL, gotKey = baseURL, apiKey
		return []llm.ModelInfo{
			{ID: "a/x", Tools: true, Reasoning: true, Known: true},
			{ID: "b/y", Known: true},
			{ID: "c/z"},
			{ID: "deepseek/deepseek-v4.1-flash", Tools: true, Known: true},
		}, nil
	})
	m := newModel(nil)
	s := m.vibeAIP.m
	s.act(stage.LoadModels{URL: "https://openrouter.ai/api/v1", Key: "k1"})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("LoadModels must ship as a command")
	}
	msg, ok := cmd().(stageModelsMsg)
	if !ok {
		t.Fatalf("flush returned %T, want stageModelsMsg", msg)
	}
	if gotURL != "https://openrouter.ai/api/v1" || gotKey != "k1" {
		t.Fatalf("list models got url=%q key=%q, want openrouter and k1", gotURL, gotKey)
	}
	want := stageModelsMsg{list: []stage.ModelInfo{
		{ID: "a/x", Reasoning: true},
		{ID: "c/z"},
		{ID: "deepseek/deepseek-v4.1-flash", Rec: true},
	}, total: 4}
	if !reflect.DeepEqual(msg, want) {
		t.Fatalf("models msg = %#v, want %#v", msg, want)
	}
}

func TestStageLoadModelsUnauthorized(t *testing.T) {
	isolatedHome(t)
	setListModels(t, func(context.Context, *http.Client, string, string) ([]llm.ModelInfo, error) {
		return nil, fmt.Errorf("x: %w", llm.ErrUnauthorized)
	})
	m := newModel(nil)
	ruLang(t)
	s := m.vibeAIP.m
	s.act(stage.LoadModels{URL: "https://openrouter.ai/api/v1", Key: "k1"})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("LoadModels must ship as a command")
	}
	msg, ok := cmd().(stageModelsMsg)
	if !ok {
		t.Fatalf("flush returned %T, want stageModelsMsg", msg)
	}
	if msg.err != "ключ не подошёл" {
		t.Fatalf("err = %q, want ключ не подошёл", msg.err)
	}
}

func TestStageLoadModelsNoTools(t *testing.T) {
	isolatedHome(t)
	setListModels(t, func(context.Context, *http.Client, string, string) ([]llm.ModelInfo, error) {
		return []llm.ModelInfo{{ID: "b/y", Known: true}}, nil
	})
	m := newModel(nil)
	ruLang(t)
	s := m.vibeAIP.m
	s.act(stage.LoadModels{URL: "https://openrouter.ai/api/v1", Key: "k1"})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("LoadModels must ship as a command")
	}
	msg, ok := cmd().(stageModelsMsg)
	if !ok {
		t.Fatalf("flush returned %T, want stageModelsMsg", msg)
	}
	if msg.err != "у провайдера нет моделей с инструментами" {
		t.Fatalf("err = %q, want у провайдера нет моделей с инструментами", msg.err)
	}
}

func TestStageModelsReachScene(t *testing.T) {
	isolatedHome(t)
	setListModels(t, func(context.Context, *http.Client, string, string) ([]llm.ModelInfo, error) {
		return []llm.ModelInfo{{ID: "a/x", Tools: true, Known: true}}, nil
	})
	cfg := testCfg()
	cfg.VibeLLMAPIKey = ""
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	st := s.st
	for i := 0; i < 182; i++ { // ≈6 s, past obReady
		st.Step(0.033)
	}
	if _, _, _, ok := st.Onboarding(); !ok {
		t.Fatal("onboarding is not on screen after the intro")
	}
	st.Key("enter", "") // the language step
	obStepUntil(st, 2, func() bool { step, _, _, ok := st.Onboarding(); return ok && step == 1 })
	step, _, _, ok := st.Onboarding()
	if !ok || step != 1 {
		t.Fatalf("step after the language enter = %d (ok %v), want 1", step, ok)
	}
	st.Key("enter", "") // the provider list, OpenRouter is first
	st.Key("", "k")     // the agent key
	st.Key("enter", "") // confirm → LoadModels
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("no command after the agent key was confirmed")
	}
	m.handleVibeAIMsg(cmd())
	for i := 0; i < 46; i++ { // 1.5 s: the loading substep gives way to the model list
		st.Step(0.033)
	}
	_, sub, _, ok := st.Onboarding()
	if !ok || sub != stage.SubModel {
		t.Fatalf("after the model list sub = %d (ok %v), want SubModel", sub, ok)
	}
}

func TestStageSetupSaves(t *testing.T) {
	isolatedHome(t)
	m := newModel(nil)
	s := m.vibeAIP.m
	s.act(stage.Setup{Lang: "en", URL: "https://openrouter.ai/api/v1", Key: "k1", Model: "a/x", Effort: "high"})
	if m.cfg.VibeLLMBaseURL != "https://openrouter.ai/api/v1" || m.cfg.VibeLLMAPIKey != "k1" ||
		m.cfg.VibeLLMModel != "a/x" || m.cfg.VibeLLMEffort != "high" || m.cfg.VibeLang != "en" {
		t.Fatalf("config after setup = %+v", m.cfg)
	}
	if got := vibeAIConfig(m.cfg).LLM.Effort; got != "high" {
		t.Fatalf("engine effort = %q, want high", got)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "vibez", "config.json"))
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	for _, want := range []string{`"vibe_llm_effort": "high"`, `"vibe_lang": "en"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("the saved config misses %s:\n%s", want, data)
		}
	}
}

func TestStageSetupKeepsEmpty(t *testing.T) {
	isolatedHome(t)
	m := newModel(nil)
	m.cfg.VibeLLMModel = "old"
	m.vibeAIP.m.act(stage.Setup{Lang: "ru"})
	if m.cfg.VibeLLMModel != "old" {
		t.Fatalf("VibeLLMModel = %q, want old", m.cfg.VibeLLMModel)
	}
	if m.cfg.VibeLang != "ru" {
		t.Fatalf("VibeLang = %q, want ru", m.cfg.VibeLang)
	}
}

func TestStageLastfmConnect(t *testing.T) {
	isolatedHome(t)
	setLastfmAuth(t, func(context.Context, string, string) (string, error) { return "sess", nil })
	cfg := testCfg()
	cfg.LastfmAPIKey, cfg.LastfmAPISecret = "k", "s"
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.act(stage.ConnectLastfm{})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("ConnectLastfm must ship as a command")
	}
	m.handleVibeAIMsg(cmd())
	if m.cfg.LastfmSessionKey != "sess" {
		t.Fatalf("session key = %q, want sess", m.cfg.LastfmSessionKey)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "vibez", "config.json"))
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	if !strings.Contains(string(data), `"lastfm_session_key": "sess"`) {
		t.Fatalf("the saved config misses the session key:\n%s", data)
	}
}

func TestStageLastfmNoKeys(t *testing.T) {
	isolatedHome(t)
	called := false
	setLastfmAuth(t, func(context.Context, string, string) (string, error) {
		called = true
		return "", nil
	})
	m := newModel(nil)
	ruLang(t)
	s := m.vibeAIP.m
	s.act(stage.ConnectLastfm{})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("ConnectLastfm must still ship a command without app keys")
	}
	msgs := stageMsgs(t, cmd)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one", msgs)
	}
	sm, ok := msgs[0].(stageStepMsg)
	if !ok || sm.step != 2 || sm.session != "" || sm.err != "нет ключа приложения Last.fm" {
		t.Fatalf("msg = %#v, want step 2 with the app-key error", msgs[0])
	}
	if called {
		t.Fatal("the auth flow must not run without app keys")
	}
}

type fakeAuther struct {
	token, url, session string
	results             []error // per GetSession call, nil → success
	failAll             bool
	calls               int
}

func (f *fakeAuther) GetToken() (string, error)        { return f.token, nil }
func (f *fakeAuther) AuthorizeURL(token string) string { return f.url + "?" + token }
func (f *fakeAuther) GetSession(token string) (string, error) {
	i := f.calls
	f.calls++
	if f.failAll || (i < len(f.results) && f.results[i] != nil) {
		return "", errors.New("lastfm: not yet")
	}
	return f.session, nil
}

func TestLastfmAuthPolls(t *testing.T) {
	f := &fakeAuther{token: "t", url: "u", session: "sess", results: []error{errors.New("a"), errors.New("b")}}
	var opens []string
	session, err := lastfmAuth(context.Background(), f, func(u string) error {
		opens = append(opens, u)
		return errors.New("no browser in tests")
	}, time.Millisecond)
	if err != nil || session != "sess" {
		t.Fatalf("lastfmAuth = %q, %v, want sess, nil", session, err)
	}
	if len(opens) != 1 || opens[0] != "u?t" {
		t.Fatalf("open calls = %v, want one with u?t", opens)
	}
	if f.calls != 3 {
		t.Fatalf("GetSession calls = %d, want 3", f.calls)
	}
}

func TestLastfmAuthTimeout(t *testing.T) {
	f := &fakeAuther{token: "t", url: "u", failAll: true}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := lastfmAuth(ctx, f, func(string) error { return nil }, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestStageAppleConnect(t *testing.T) {
	isolatedHome(t)
	m := newModel(nil)
	s := m.vibeAIP.m
	s.act(stage.ConnectApple{})
	cmd := s.flush()
	if cmd == nil {
		t.Fatal("ConnectApple must ship as a command")
	}
	sm, ok := cmd().(stageStepMsg)
	if !ok || sm.step != 0 || sm.session != "" || sm.err != "" {
		t.Fatalf("msg = %#v, want stageStepMsg{step: 0}", sm)
	}
}
