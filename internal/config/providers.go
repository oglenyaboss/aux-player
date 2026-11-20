package config

import "strings"

// LLMProvider is one stored picker-agent credential; BaseURL only belongs to the custom provider.
type LLMProvider struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// CustomProvider is the id of the provider whose address the listener types.
const CustomProvider = "custom"

var presets = []struct{ id, url string }{
	{"opencode-go", "https://opencode.ai/inference/go/v1"},
	{"commandcode", "https://api.commandcode.ai/provider/v1"},
	{"openrouter", "https://openrouter.ai/api/v1"},
	{"openai", "https://api.openai.com/v1"},
	{"anthropic", "https://api.anthropic.com/v1"},
	{"deepseek", "https://api.deepseek.com"},
}

// PresetURL is the base URL of a preset provider; "" for an unknown id or the custom one.
func PresetURL(id string) string {
	for _, p := range presets {
		if p.id == id {
			return p.url
		}
	}
	return ""
}

// ProviderForURL maps a scheme-less host and path onto a preset id, or CustomProvider.
func ProviderForURL(u string) string {
	s := strings.TrimSpace(u)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimRight(s, "/")
	host, path, _ := strings.Cut(s, "/")
	key := strings.ToLower(host)
	if path != "" {
		key += "/" + path
	}
	if key == "opencode.ai/zen/go/v1" {
		return "opencode-go"
	}
	for _, p := range presets {
		if strings.TrimPrefix(p.url, "https://") == key {
			return p.id
		}
	}
	return CustomProvider
}

// ActiveLLM resolves the active provider profile and its base URL; zero values without one.
func (c *Config) ActiveLLM() (id string, p LLMProvider, baseURL string) {
	if c.VibeProvider == "" {
		return "", LLMProvider{}, ""
	}
	id = c.VibeProvider
	p = c.VibeProviders[id]
	if id == CustomProvider {
		return id, p, p.BaseURL
	}
	return id, p, PresetURL(id)
}

// SetLLM stores one provider profile and makes it active; Save persists it.
func (c *Config) SetLLM(id string, p LLMProvider) {
	if c.VibeProviders == nil {
		c.VibeProviders = map[string]LLMProvider{}
	}
	c.VibeProviders[id] = p
	c.VibeProvider = id
}
