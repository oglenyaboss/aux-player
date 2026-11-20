package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/config"
)

func readConfig(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("reading the saved config: %v", err)
	}
	return string(data)
}

// A1: the legacy base URL, key, model and effort become one opencode-go profile.
func TestLoadMigratesLegacyConfig(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"vibe_llm_base_url": "https://opencode.ai/zen/go/v1",
		"vibe_llm_api_key":  "k",
		"vibe_llm_model":    "m",
		"vibe_llm_effort":   "high",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, p, base := cfg.ActiveLLM()
	want := config.LLMProvider{APIKey: "k", Model: "m", Effort: "high"}
	if id != "opencode-go" || !reflect.DeepEqual(p, want) || base != "https://opencode.ai/inference/go/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want (opencode-go, %+v, https://opencode.ai/inference/go/v1)",
			id, p, base, want)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data := readConfig(t, path)
	if !strings.Contains(data, `"vibe_providers"`) {
		t.Fatalf("the saved config misses vibe_providers:\n%s", data)
	}
	if strings.Contains(data, `"vibe_llm_`) {
		t.Fatalf("the saved config still writes a vibe_llm_ key:\n%s", data)
	}
}

// A2: a bare legacy key gets the default model and the opencode-go preset.
func TestLoadMigratesLegacyKeyOnly(t *testing.T) {
	path := writeCfg(t, map[string]any{"vibe_llm_api_key": "k"})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, p, base := cfg.ActiveLLM()
	want := config.LLMProvider{APIKey: "k", Model: "deepseek-v4.1-flash"}
	if id != "opencode-go" || !reflect.DeepEqual(p, want) || base != "https://opencode.ai/inference/go/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want (opencode-go, %+v, https://opencode.ai/inference/go/v1)",
			id, p, base, want)
	}
}

// A3: the Command Code URL maps to its preset even with a trailing slash.
func TestLoadMigratesCommandCodeURL(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"vibe_llm_base_url": "https://api.commandcode.ai/provider/v1/",
		"vibe_llm_api_key":  "k",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, _, base := cfg.ActiveLLM()
	if id != "commandcode" || base != "https://api.commandcode.ai/provider/v1" {
		t.Fatalf("ActiveLLM = (%q, %q), want (commandcode, https://api.commandcode.ai/provider/v1)", id, base)
	}
}

// A4: an unknown legacy base becomes the custom profile with its own URL.
func TestLoadMigratesCustomURL(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"vibe_llm_base_url": "https://example.com/v1",
		"vibe_llm_api_key":  "k",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, p, base := cfg.ActiveLLM()
	want := config.LLMProvider{APIKey: "k", BaseURL: "https://example.com/v1", Model: "deepseek-v4.1-flash"}
	if id != config.CustomProvider || !reflect.DeepEqual(p, want) || base != "https://example.com/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want (custom, %+v, https://example.com/v1)", id, p, base, want)
	}
}

// A5: a config without an agent keeps builder fields out of the saved file.
func TestSaveOmitsEmptyProvider(t *testing.T) {
	path := writeCfg(t, map[string]any{"storefront": "us"})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if id, _, base := cfg.ActiveLLM(); id != "" || base != "" {
		t.Fatalf("ActiveLLM = (%q, %q), want empty", id, base)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data := readConfig(t, path)
	if strings.Contains(data, `"vibe_provider"`) || strings.Contains(data, `"vibe_providers"`) {
		t.Fatalf("the saved config writes a provider without an agent:\n%s", data)
	}
}

// A6: with profiles stored the stale legacy key is ignored, not migrated over them.
func TestLoadIgnoresLegacyWhenProfilesExist(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"vibe_provider": "commandcode",
		"vibe_providers": map[string]any{
			"opencode-go": map[string]any{"api_key": "a"},
			"commandcode": map[string]any{"api_key": "b", "model": "x"},
		},
		"vibe_llm_api_key": "stale",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, p, base := cfg.ActiveLLM()
	want := config.LLMProvider{APIKey: "b", Model: "x"}
	if id != "commandcode" || !reflect.DeepEqual(p, want) || base != "https://api.commandcode.ai/provider/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want (commandcode, %+v, https://api.commandcode.ai/provider/v1)",
			id, p, base, want)
	}
	if got := cfg.VibeProviders["opencode-go"].APIKey; got != "a" {
		t.Fatalf("opencode-go key = %q, want a", got)
	}
	if cfg.VibeLLMAPIKey != "" {
		t.Fatalf("legacy key = %q, want cleared", cfg.VibeLLMAPIKey)
	}
}

// A7: normalize runs on every Load, so a second Load of the saved file is stable.
func TestMigrationIsIdempotent(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"vibe_llm_base_url": "https://opencode.ai/zen/go/v1",
		"vibe_llm_api_key":  "k",
		"vibe_llm_model":    "m",
		"vibe_llm_effort":   "high",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id1, p1, base1 := cfg.ActiveLLM()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	again, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	id2, p2, base2 := again.ActiveLLM()
	if id1 != id2 || !reflect.DeepEqual(p1, p2) || base1 != base2 {
		t.Fatalf("ActiveLLM changed across Save/Load: (%q, %+v, %q) → (%q, %+v, %q)",
			id1, p1, base1, id2, p2, base2)
	}
}

// A8: the removed fields still load from old files and never come back on Save.
func TestLoadDropsDeadFields(t *testing.T) {
	path := writeCfg(t, map[string]any{
		"theme":         "x",
		"provider":      "apple",
		"album_art":     true,
		"apple_key_id":  "a",
		"apple_team_id": "b",
		"storefront":    "us",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.StoreFront != "us" {
		t.Fatalf("StoreFront = %q, want us", cfg.StoreFront)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data := readConfig(t, path)
	for _, key := range []string{`"theme"`, `"provider"`, `"album_art"`, `"apple_key_id"`, `"apple_team_id"`} {
		if strings.Contains(data, key) {
			t.Fatalf("the saved config still writes %s:\n%s", key, data)
		}
	}
}

// A9: preset URLs resolve by scheme-less host and path, trailing slash trimmed.
func TestProviderForURL(t *testing.T) {
	cases := map[string]string{
		"https://opencode.ai/inference/go/v1":     "opencode-go",
		"HTTPS://OpenCode.ai/zen/go/v1/":          "opencode-go",
		"https://openrouter.ai/api/v1":            "openrouter",
		"https://api.commandcode.ai/provider/v1/": "commandcode",
		"https://example.com/v1":                  config.CustomProvider,
	}
	for in, want := range cases {
		if got := config.ProviderForURL(in); got != want {
			t.Errorf("ProviderForURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// A10 config side: SetLLM keeps the other profiles and makes the id active.
func TestSetLLMKeepsOtherProfiles(t *testing.T) {
	cfg := &config.Config{}
	cfg.SetLLM("opencode-go", config.LLMProvider{APIKey: "a"})
	cfg.SetLLM("commandcode", config.LLMProvider{APIKey: "b"})
	id, p, base := cfg.ActiveLLM()
	if id != "commandcode" || p.APIKey != "b" || base != "https://api.commandcode.ai/provider/v1" {
		t.Fatalf("ActiveLLM = (%q, %+v, %q), want commandcode with b", id, p, base)
	}
	if got := cfg.VibeProviders["opencode-go"].APIKey; got != "a" {
		t.Fatalf("opencode-go key = %q, want a", got)
	}
}
