package stage

import (
	"os"
	"strconv"
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

// The counted nouns of the music screen: the ru forms for one, few and many, en for
// the singular and the plural.
var (
	trackForms      = [3]string{"трек", "трека", "треков"}
	trackFormsEn    = [2]string{"track", "tracks"}
	playlistForms   = [3]string{"плейлист", "плейлиста", "плейлистов"}
	playlistFormsEn = [2]string{"playlist", "playlists"}
	albumForms      = [3]string{"альбом", "альбома", "альбомов"}
	albumFormsEn    = [2]string{"album", "albums"}
)

// countOf is the number with the noun form it takes in the active language.
func countOf(n int, ru [3]string, en [2]string) string {
	s := strconv.Itoa(n)
	if lang != "ru" {
		if n == 1 {
			return s + " " + en[0]
		}
		return s + " " + en[1]
	}
	switch {
	case n%10 == 1 && n%100 != 11:
		return s + " " + ru[0]
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return s + " " + ru[1]
	}
	return s + " " + ru[2]
}

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
		{"opencode-go", "OpenCode Go", "opencode.ai/inference/go/v1", "opencode.ai", false},
		{"commandcode", "Command Code", "api.commandcode.ai/provider/v1", "commandcode.ai", false},
		{"chatgpt", tr("ChatGPT (подписка)", "ChatGPT (subscription)"), "chatgpt.com/backend-api/codex", "", true},
		{"openrouter", "OpenRouter", "openrouter.ai/api/v1", "openrouter.ai/keys", false},
		{"openai", "OpenAI", "api.openai.com/v1", "platform.openai.com/api-keys", false},
		{"anthropic", "Anthropic", "api.anthropic.com/v1", "console.anthropic.com", false},
		{"deepseek", "DeepSeek", "api.deepseek.com", "platform.deepseek.com", false},
		{"custom", tr("свой адрес", "custom address"), tr("OpenAI-совместимый", "OpenAI-compatible"), "", false},
	}
	efforts = []string{tr("низкий", "low"), tr("средний", "medium"), tr("высокий", "high")}
}
