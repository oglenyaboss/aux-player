package tui

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/oglenyaboss/aux-player/internal/chatgpt"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

// setOpenChatGPT replaces the subscription store opener for the test.
func setOpenChatGPT(t *testing.T, store *chatgpt.Store) {
	t.Helper()
	orig := openChatGPT
	openChatGPT = func() (*chatgpt.Store, error) { return store, nil }
	t.Cleanup(func() { openChatGPT = orig })
}

// setChatGPTLogin replaces the browser sign-in for the test.
func setChatGPTLogin(t *testing.T, f func(context.Context, *chatgpt.Store) error) {
	t.Helper()
	orig := chatgptLogin
	chatgptLogin = f
	t.Cleanup(func() { chatgptLogin = orig })
}

// setListChatGPTModels replaces the subscription model fetch for the test.
func setListChatGPTModels(t *testing.T, f func(context.Context, *http.Client, string, llm.Tokens) ([]llm.ModelInfo, error)) {
	t.Helper()
	orig := stageListChatGPTModels
	stageListChatGPTModels = f
	t.Cleanup(func() { stageListChatGPTModels = orig })
}

// loadSaved reads back what the host wrote under the isolated home.
func loadSaved(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(savedConfigPath(t))
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	return cfg
}

// newEnglishModel is a host whose scene language is English, for the text assertions.
func newEnglishModel(t *testing.T) *Model {
	t.Helper()
	cfg := testCfg()
	cfg.VibeLang = "en"
	return New(cfg, &mockProvider{}, nil, Options{})
}

// H1: Settings opens the overlay and the info mirrors the active config.
func TestSettingsOpensAndReadsConfig(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider: "commandcode",
		VibeProviders: map[string]config.LLMProvider{
			"commandcode": {APIKey: "k", Model: "m"},
			"openrouter":  {APIKey: "or"},
		},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.act(stage.Settings{})
	if !s.st.SettingsOpen() {
		t.Fatal("Settings did not open the overlay")
	}
	info := m.settingsInfo()
	want := stage.SettingsInfo{
		Provider: "commandcode",
		URL:      "https://api.commandcode.ai/provider/v1",
		Model:    "m",
		Keys:     map[string]bool{"commandcode": true, "openrouter": true},
		Lang:     stage.DefaultLang(),
		Eggs:     true,
		Discord:  true,
	}
	if !reflect.DeepEqual(info, want) {
		t.Fatalf("settingsInfo = %+v, want %+v", info, want)
	}
}

// H2: a model change keeps the stored key of the provider and the other profiles.
func TestSettingsAgentSetKeepsKey(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider: "commandcode",
		VibeProviders: map[string]config.LLMProvider{
			"commandcode": {APIKey: "k", Model: "m"},
			"openrouter":  {APIKey: "or", Model: "old"},
		},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.AgentSet{Provider: "openrouter", Model: "x/y", Effort: "low"})
	saved := loadSaved(t)
	want := config.LLMProvider{APIKey: "or", Model: "x/y", Effort: "low"}
	if got := saved.VibeProviders["openrouter"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("openrouter profile = %+v, want %+v", got, want)
	}
	if saved.VibeProvider != "openrouter" {
		t.Fatalf("active provider = %q, want openrouter", saved.VibeProvider)
	}
	wantCommand := config.LLMProvider{APIKey: "k", Model: "m"}
	if got := saved.VibeProviders["commandcode"]; !reflect.DeepEqual(got, wantCommand) {
		t.Fatalf("commandcode profile = %+v, want %+v", got, wantCommand)
	}
}

// H3: a key-only change stores the key, leaves the model empty and activates the provider.
func TestSettingsAgentSetKeyOnly(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider:  "commandcode",
		VibeProviders: map[string]config.LLMProvider{"commandcode": {APIKey: "k", Model: "m"}},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.AgentSet{Provider: "deepseek", Key: "dk"})
	saved := loadSaved(t)
	want := config.LLMProvider{APIKey: "dk"}
	if got := saved.VibeProviders["deepseek"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("deepseek profile = %+v, want %+v", got, want)
	}
	if saved.VibeProvider != "deepseek" {
		t.Fatalf("active provider = %q, want deepseek", saved.VibeProvider)
	}
}

// H4: a forgotten key is cleared, the model stays and the intro sees no agent.
func TestSettingsAgentForget(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider:  "commandcode",
		VibeProviders: map[string]config.LLMProvider{"commandcode": {APIKey: "k", Model: "m"}},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.AgentForget{Provider: "commandcode"})
	saved := loadSaved(t)
	if got := saved.VibeProviders["commandcode"].APIKey; got != "" {
		t.Fatalf("commandcode key = %q, want empty", got)
	}
	if got := saved.VibeProviders["commandcode"].Model; got != "m" {
		t.Fatalf("commandcode model = %q, want m", got)
	}
	if o := stageOptions(saved, false); !o.NoAgent {
		t.Fatal("NoAgent is false after the key was forgotten")
	}
}

// A forgotten inactive provider is cleared without taking over the active one.
func TestSettingsAgentForgetInactive(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider: "commandcode",
		VibeProviders: map[string]config.LLMProvider{
			"commandcode": {APIKey: "k", Model: "m"},
			"openrouter":  {APIKey: "or", Model: "x"},
		},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.AgentForget{Provider: "openrouter"})
	saved := loadSaved(t)
	if saved.VibeProvider != "commandcode" {
		t.Fatalf("active provider = %q, want commandcode", saved.VibeProvider)
	}
	if got := saved.VibeProviders["openrouter"]; !reflect.DeepEqual(got, config.LLMProvider{Model: "x"}) {
		t.Fatalf("openrouter profile = %+v, want the model kept without the key", got)
	}
	if got := saved.VibeProviders["commandcode"].APIKey; got != "k" {
		t.Fatalf("commandcode key = %q, want k", got)
	}
}

// signedInStore writes a stored subscription session and opens it.
func signedInStore(t *testing.T) (*chatgpt.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chatgpt-auth.json")
	data := []byte(`{"access_token":"token","account_id":"acc","expires":"2099-01-01T00:00:00Z"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing the auth file: %v", err)
	}
	store, err := chatgpt.Open(path, nil)
	if err != nil {
		t.Fatalf("chatgpt.Open: %v", err)
	}
	return store, path
}

// AgentForget on the subscription signs out of the store and keeps its profile.
func TestSettingsAgentForgetChatGPT(t *testing.T) {
	isolatedHome(t)
	store, path := signedInStore(t)
	setOpenChatGPT(t, store)
	cfg := &config.Config{
		VibeProvider:  config.ChatGPTProvider,
		VibeProviders: map[string]config.LLMProvider{config.ChatGPTProvider: {Model: "gpt-6-luna"}},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	if !m.settingsInfo().Keys[config.ChatGPTProvider] {
		t.Fatal("a signed-in store is not marked in the settings keys")
	}
	m.vibeAIP.m.act(stage.AgentForget{Provider: config.ChatGPTProvider})
	if store.LoggedIn() {
		t.Fatal("the store is still signed in after AgentForget")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the auth file still exists after sign-out: %v", err)
	}
	if m.settingsInfo().Keys[config.ChatGPTProvider] {
		t.Fatal("a signed-out store is still marked in the settings keys")
	}
	saved := loadSaved(t)
	if saved.VibeProvider != config.ChatGPTProvider || saved.VibeProviders[config.ChatGPTProvider].Model != "gpt-6-luna" {
		t.Fatalf("saved config = (%q, %+v), want the ChatGPT profile kept", saved.VibeProvider, saved.VibeProviders)
	}
}

// LangSet lands in the config and refreshes the open overlay.
func TestSettingsLangSet(t *testing.T) {
	isolatedHome(t)
	cfg := testCfg()
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.st.OpenSettings(m.settingsInfo())
	s.act(stage.LangSet{Lang: "ru"})
	if cfg.VibeLang != "ru" {
		t.Fatalf("VibeLang = %q, want ru", cfg.VibeLang)
	}
	if saved := loadSaved(t); saved.VibeLang != "ru" {
		t.Fatalf("saved VibeLang = %q, want ru", saved.VibeLang)
	}
	if info := m.settingsInfo(); info.Lang != "ru" {
		t.Fatalf("settings lang = %q, want ru", info.Lang)
	}
	if !s.st.SettingsOpen() {
		t.Fatal("the overlay closed on a language change")
	}
}

// H5: LoadModels with no typed key uses the provider's stored one.
func TestSettingsLoadModelsStoredKey(t *testing.T) {
	isolatedHome(t)
	var gotURL, gotKey string
	setListModels(t, func(_ context.Context, _ *http.Client, url, key string) ([]llm.ModelInfo, error) {
		gotURL, gotKey = url, key
		return []llm.ModelInfo{{ID: "a/x", Tools: true, Known: true}}, nil
	})
	cfg := &config.Config{
		VibeProvider:  "openrouter",
		VibeProviders: map[string]config.LLMProvider{"openrouter": {APIKey: "or-key"}},
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.act(stage.LoadModels{Provider: "openrouter", URL: "https://openrouter.ai/api/v1", Key: ""})
	msgs := stageMsgs(t, s.flush())
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one", msgs)
	}
	if _, ok := msgs[0].(stageModelsMsg); !ok {
		t.Fatalf("msg = %T, want stageModelsMsg", msgs[0])
	}
	if gotURL != "https://openrouter.ai/api/v1" || gotKey != "or-key" {
		t.Fatalf("list models got url=%q key=%q, want the stored key", gotURL, gotKey)
	}
}

// A keyless provider never reaches the network with an empty Bearer.
func TestSettingsLoadModelsNoStoredKey(t *testing.T) {
	isolatedHome(t)
	called := false
	setListModels(t, func(context.Context, *http.Client, string, string) ([]llm.ModelInfo, error) {
		called = true
		return nil, nil
	})
	cfg := &config.Config{
		VibeProvider:  "openrouter",
		VibeProviders: map[string]config.LLMProvider{"openrouter": {Model: "m"}},
		VibeLang:      "en",
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.act(stage.LoadModels{Provider: "openrouter", URL: "https://openrouter.ai/api/v1"})
	msgs := stageMsgs(t, s.flush())
	if called {
		t.Fatal("an empty key reached the models endpoint")
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one", msgs)
	}
	sm, ok := msgs[0].(stageModelsMsg)
	if !ok || sm.err != "key rejected" {
		t.Fatalf("msg = %#v, want the key-rejected error", msgs[0])
	}
}

// H6: the subscription signs in once and lists its models, recommending luna first.
func TestSettingsChatGPTModels(t *testing.T) {
	isolatedHome(t)
	store, err := chatgpt.Open(filepath.Join(t.TempDir(), "chatgpt-auth.json"), nil)
	if err != nil {
		t.Fatalf("chatgpt.Open: %v", err)
	}
	setOpenChatGPT(t, store)
	logins := 0
	setChatGPTLogin(t, func(context.Context, *chatgpt.Store) error {
		logins++
		return nil
	})
	var gotStore llm.Tokens
	setListChatGPTModels(t, func(_ context.Context, _ *http.Client, base string, tok llm.Tokens) ([]llm.ModelInfo, error) {
		gotStore = tok
		return []llm.ModelInfo{{ID: "gpt-6.2-sol"}, {ID: "gpt-6-luna"}, {ID: "gpt-5.6-luna"}}, nil
	})
	m := newEnglishModel(t)
	s := m.vibeAIP.m
	s.act(stage.LoadModels{Provider: "chatgpt", URL: "https://chatgpt.com/backend-api/codex"})
	msgs := stageMsgs(t, s.flush())
	if logins != 1 {
		t.Fatalf("sign-in calls = %d, want 1", logins)
	}
	if gotStore != store {
		t.Fatalf("models fetched with %v, want the opened store", gotStore)
	}
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one", msgs)
	}
	sm, ok := msgs[0].(stageModelsMsg)
	if !ok {
		t.Fatalf("msg = %T, want stageModelsMsg", msgs[0])
	}
	want := []stage.ModelInfo{
		{ID: "gpt-6.2-sol"},
		{ID: "gpt-6-luna", Rec: true},
		{ID: "gpt-5.6-luna"},
	}
	if sm.err != "" || sm.total != 3 || !reflect.DeepEqual(sm.list, want) {
		t.Fatalf("reply = %+v, want list %+v with total 3", sm, want)
	}
}

// H6: a failed sign-in answers the provider form with the sign-in error.
func TestSettingsChatGPTLoginError(t *testing.T) {
	isolatedHome(t)
	store, err := chatgpt.Open(filepath.Join(t.TempDir(), "chatgpt-auth.json"), nil)
	if err != nil {
		t.Fatalf("chatgpt.Open: %v", err)
	}
	setOpenChatGPT(t, store)
	setChatGPTLogin(t, func(context.Context, *chatgpt.Store) error { return errors.New("denied") })
	m := newEnglishModel(t)
	s := m.vibeAIP.m
	s.act(stage.LoadModels{Provider: "chatgpt"})
	msgs := stageMsgs(t, s.flush())
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one", msgs)
	}
	sm, ok := msgs[0].(stageModelsMsg)
	if !ok || !strings.Contains(sm.err, "sign-in failed") {
		t.Fatalf("msg = %#v, want a sign-in failure", msgs[0])
	}
}

// H8: the limits callback feeds settingsInfo's usage line.
func TestSettingsUsage(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider:  config.ChatGPTProvider,
		VibeProviders: map[string]config.LLMProvider{config.ChatGPTProvider: {Model: "gpt-6-luna", Effort: "low"}},
		VibeLang:      "en",
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	ac := m.agentConfig()
	if ac.LLM != (llm.Config{BaseURL: llm.ChatGPTBaseURL, Model: "gpt-6-luna", Effort: "low"}) {
		t.Fatalf("agent LLM = %+v, want the ChatGPT Responses config", ac.LLM)
	}
	if ac.Tokens == nil {
		t.Fatal("agent config has no subscription tokens")
	}
	if ac.OnLimits == nil {
		t.Fatal("agent config has no limits callback")
	}
	ac.OnLimits(llm.Limits{
		PrimaryPercent:   34.4,
		SecondaryPercent: 12,
		PrimaryWindow:    5 * time.Hour,
		SecondaryWindow:  168 * time.Hour,
	})
	if got := m.settingsInfo().Usage; got != "5h: 34% · week: 12%" {
		t.Fatalf("usage = %q, want %q", got, "5h: 34% · week: 12%")
	}
}

// H9: the subscription limit error names the limit and its reset.
func TestSettingsLimitError(t *testing.T) {
	isolatedHome(t)
	m := newEnglishModel(t)
	m.vibeAIP.m.SetError(&llm.LimitError{ResetsIn: 80 * time.Minute})
	frame := ansi.Strip(m.vibeAIP.m.Frame(110, 32))
	for _, want := range []string{"subscription limit", "1h20m"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the stage error lacks %q:\n%s", want, frame)
		}
	}
}

// The expired subscription sign-in points at the settings.
func TestSettingsSignedOutError(t *testing.T) {
	isolatedHome(t)
	m := newEnglishModel(t)
	m.vibeAIP.m.SetError(chatgpt.ErrSignedOut)
	frame := ansi.Strip(m.vibeAIP.m.Frame(110, 32))
	if !strings.Contains(frame, "sign-in expired") {
		t.Fatalf("the stage error lacks the expired sign-in:\n%s", frame)
	}
}

// H10: the Discord toggle and the Last.fm disconnect reach config and hooks.
func TestSettingsDisconnectHooks(t *testing.T) {
	isolatedHome(t)
	off := true
	var discordCalls int
	var gotDiscord bool
	var lastfmCalls int
	var gotLastfm string
	cfg := &config.Config{
		VibeProvider:     "commandcode",
		VibeProviders:    map[string]config.LLMProvider{"commandcode": {APIKey: "k"}},
		LastfmSessionKey: "sess",
		DiscordRPC:       &off,
	}
	m := New(cfg, &mockProvider{}, nil, Options{
		Discord: func(on bool) { discordCalls++; gotDiscord = on },
		Lastfm:  func(session string) { lastfmCalls++; gotLastfm = session },
	})
	s := m.vibeAIP.m
	s.act(stage.DiscordSet{On: false})
	if saved := loadSaved(t); saved.DiscordRPCEnabled() {
		t.Fatal("discord_rpc is still on in the saved config")
	}
	if discordCalls != 1 || gotDiscord {
		t.Fatalf("discord hook calls=%d on=%v, want one call with false", discordCalls, gotDiscord)
	}
	s.act(stage.LastfmDisconnect{})
	if saved := loadSaved(t); saved.LastfmSessionKey != "" {
		t.Fatalf("saved lastfm session = %q, want empty", saved.LastfmSessionKey)
	}
	if lastfmCalls != 1 || gotLastfm != "" {
		t.Fatalf("lastfm hook calls=%d session=%q, want one call with an empty session", lastfmCalls, gotLastfm)
	}
}

// H11: the Apple sign-out persists and leaves the program.
func TestSettingsAppleSignOut(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{
		VibeProvider:   "commandcode",
		VibeProviders:  map[string]config.LLMProvider{"commandcode": {APIKey: "k"}},
		AppleUserToken: "apple-user",
	}
	m := New(cfg, &mockProvider{}, nil, Options{})
	s := m.vibeAIP.m
	s.act(stage.AppleSignOut{})
	if saved := loadSaved(t); saved.AppleUserToken != "" {
		t.Fatalf("saved apple token = %q, want empty", saved.AppleUserToken)
	}
	quit := false
	for _, msg := range stageMsgs(t, s.flush()) {
		if _, ok := msg.(tea.QuitMsg); ok {
			quit = true
		}
	}
	if !quit {
		t.Fatal("AppleSignOut did not send tea.Quit")
	}
}

// H12: a failed Last.fm connect with the overlay open lands in the settings error.
func TestSettingsStepError(t *testing.T) {
	isolatedHome(t)
	m := newEnglishModel(t)
	s := m.vibeAIP.m
	s.st.OpenSettings(m.settingsInfo())
	m.handleVibeAIMsg(stageStepMsg{step: 2, err: "Last.fm did not answer in time"})
	frame := ansi.Strip(s.Frame(100, 40))
	if !strings.Contains(frame, "Last.fm did not answer in time") {
		t.Fatalf("the settings overlay lacks the Last.fm error:\n%s", frame)
	}
}

// The subscription provider counts as ready only with a sign-in.
func TestStageOptionsChatGPT(t *testing.T) {
	isolatedHome(t)
	orig := chatgptSignedIn
	t.Cleanup(func() { chatgptSignedIn = orig })
	cfg := &config.Config{VibeProvider: config.ChatGPTProvider}
	chatgptSignedIn = func() bool { return false }
	if o := stageOptions(cfg, false); !o.NoAgent || o.Connected[1] {
		t.Fatalf("signed out: noAgent=%v connected=%v, want true/false", o.NoAgent, o.Connected[1])
	}
	chatgptSignedIn = func() bool { return true }
	if o := stageOptions(cfg, false); o.NoAgent || !o.Connected[1] {
		t.Fatalf("signed in: noAgent=%v connected=%v, want false/true", o.NoAgent, o.Connected[1])
	}
}
