package stage

import (
	"os"
	"strings"
)

var lang = "en"

// Tr leans on the active language: Tr(ru, en) returns the ru line while lang is "ru".
func Tr(ru, en string) string {
	if lang == "ru" {
		return ru
	}
	return en
}

func tr(ru, en string) string { return Tr(ru, en) }

// DefaultLang: $LC_ALL, $LC_MESSAGES, $LANG — first non-empty; "ru" when it starts with ru.
func DefaultLang() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "ru") {
				return "ru"
			}
			return "en"
		}
	}
	return "en"
}

// SetLang (re)builds every language-dependent piece the screen owns on its own. Package
// variables are initialized before flags are parsed, so nothing visible may call Tr() at init time.
func SetLang(l string) {
	lang = l

	providers = []provider{
		{"OpenRouter", "openrouter.ai/api/v1", "openrouter.ai/keys"},
		{"OpenCode Go", "opencode.ai/zen/go/v1", "opencode.ai"},
		{"OpenAI", "api.openai.com/v1", "platform.openai.com/api-keys"},
		{"Anthropic", "api.anthropic.com/v1", "console.anthropic.com"},
		{"DeepSeek", "api.deepseek.com", "platform.deepseek.com"},
		{tr("свой адрес", "custom address"), tr("OpenAI-совместимый", "OpenAI-compatible"), ""},
	}
	efforts = []string{tr("низкий", "low"), tr("средний", "medium"), tr("высокий", "high")}
}
