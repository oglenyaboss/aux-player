package tui

import (
	"reflect"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

// A10: a Setup stores a new provider profile, keeps the others and feeds the agent.
func TestStageSetupStoresProviderProfile(t *testing.T) {
	isolatedHome(t)
	cfg := llmConfig("k1")
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.Setup{
		Lang:     "en",
		Provider: "commandcode",
		URL:      "https://api.commandcode.ai/provider/v1",
		Key:      "k2",
		Model:    "deepseek/deepseek-v4.1-flash",
	})
	id, p, base := cfg.ActiveLLM()
	if id != "commandcode" || p.APIKey != "k2" || base != "https://api.commandcode.ai/provider/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want commandcode with k2", id, p, base)
	}
	if got := cfg.VibeProviders["opencode-go"].APIKey; got != "k1" {
		t.Fatalf("opencode-go key = %q, want k1", got)
	}
	want := llm.Config{BaseURL: "https://api.commandcode.ai/provider/v1", APIKey: "k2", Model: "deepseek/deepseek-v4.1-flash"}
	if got := vibeAIConfig(cfg).LLM; got != want {
		t.Fatalf("agent LLM = %+v, want %+v", got, want)
	}
}

// A11: an empty key on the custom provider never overwrites the stored one.
func TestStageSetupCustomKeepsKey(t *testing.T) {
	isolatedHome(t)
	cfg := &config.Config{VibeProvider: "custom", VibeProviders: map[string]config.LLMProvider{
		"custom": {APIKey: "old", BaseURL: "https://a/v1"},
	}}
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.Setup{Provider: "custom", URL: "https://b/v1", Key: "", Model: "m"})
	want := config.LLMProvider{APIKey: "old", BaseURL: "https://b/v1", Model: "m"}
	if got := cfg.VibeProviders["custom"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("custom profile = %+v, want %+v", got, want)
	}
}

// A12: a Setup without a model leaves every profile alone.
func TestStageSetupWithoutModelKeepsProfiles(t *testing.T) {
	isolatedHome(t)
	cfg := testCfg()
	before := cfg.VibeProviders["opencode-go"]
	active := cfg.VibeProvider
	m := New(cfg, &mockProvider{}, nil, Options{})
	m.vibeAIP.m.act(stage.Setup{Lang: "ru"})
	if got := cfg.VibeProviders["opencode-go"]; got != before {
		t.Fatalf("profile changed without a model: %+v → %+v", before, got)
	}
	if cfg.VibeProvider != active {
		t.Fatalf("active provider changed without a model: %q → %q", active, cfg.VibeProvider)
	}
	if cfg.VibeLang != "ru" {
		t.Fatalf("VibeLang = %q, want ru", cfg.VibeLang)
	}
}

// A13: stageOptions reads the active profile, not any stored profile.
func TestStageOptionsReadsActiveProfile(t *testing.T) {
	cfg := &config.Config{VibeProviders: map[string]config.LLMProvider{"openrouter": {APIKey: "k"}}}
	o := stageOptions(cfg, false)
	if !o.NoAgent || o.Connected[1] {
		t.Fatalf("stored but inactive profile: noAgent=%v connected=%v, want true/false", o.NoAgent, o.Connected[1])
	}
	cfg.VibeProvider = "openrouter"
	o = stageOptions(cfg, false)
	if o.NoAgent || !o.Connected[1] {
		t.Fatalf("active profile: noAgent=%v connected=%v, want false/true", o.NoAgent, o.Connected[1])
	}
}
