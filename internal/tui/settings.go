package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/chatgpt"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/openurl"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai"
	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

// openChatGPT, chatgptLogin and stageListChatGPTModels are the subscription's I/O
// seams; the tests swap them so no test opens a browser or reaches the network.
var openChatGPT = func() (*chatgpt.Store, error) {
	path, err := chatgpt.Path()
	if err != nil {
		return nil, err
	}
	return chatgpt.Open(path, nil)
}

var chatgptLogin = func(ctx context.Context, s *chatgpt.Store) error {
	return chatgpt.Login(ctx, s, openurl.Open)
}

var stageListChatGPTModels = llm.ListChatGPTModels

// chatgptSignedIn reports whether the subscription store holds a sign-in;
// stageOptions reads it without a Model and opens the store itself.
var chatgptSignedIn = func() bool {
	s, err := openChatGPT()
	return err == nil && s.LoggedIn()
}

// agentReady resolves the active provider and whether the picker can start with it:
// a key provider needs its stored key, the subscription a sign-in.
func agentReady(cfg *config.Config) (id string, ready bool) {
	id, profile, _ := cfg.ActiveLLM()
	switch {
	case id == "":
		return "", false
	case id == config.ChatGPTProvider:
		return id, chatgptSignedIn()
	default:
		return id, profile.APIKey != ""
	}
}

// chatGPT opens the subscription store once and keeps it on the Model; commands
// run beside Update, so the lazy open takes the mutex.
func (m *Model) chatGPT() (*chatgpt.Store, error) {
	m.chatgptMu.Lock()
	defer m.chatgptMu.Unlock()
	if m.chatgpt != nil {
		return m.chatgpt, nil
	}
	s, err := openChatGPT()
	if err != nil {
		return nil, err
	}
	m.chatgpt = s
	return s, nil
}

func (m *Model) signedIn() bool {
	s, err := m.chatGPT()
	return err == nil && s.LoggedIn()
}

func (m *Model) saveConfig() {
	if err := m.cfg.Save(""); err != nil {
		m.appendLog(fmt.Sprintf("[vibe] config save error: %v", err))
	}
}

// settingsInfo is what the settings overlay shows: the agent and the preferences as
// the config holds them right now, plus the subscription usage and the pending error.
func (m *Model) settingsInfo() stage.SettingsInfo {
	cfg := m.cfg
	id, profile, baseURL := cfg.ActiveLLM()
	info := stage.SettingsInfo{
		Provider: id,
		URL:      baseURL,
		Model:    profile.Model,
		Effort:   profile.Effort,
		Keys:     map[string]bool{},
		Lang:     cfg.VibeLang,
		Eggs:     cfg.VibeEggs == nil || *cfg.VibeEggs,
		Apple:    cfg.AppleUserToken != "",
		Lastfm:   cfg.LastfmSessionKey != "",
		Discord:  cfg.DiscordRPCEnabled(),
		Error:    m.settingsErr,
	}
	if info.Lang == "" {
		info.Lang = stage.DefaultLang()
	}
	for pid, p := range cfg.VibeProviders {
		if p.APIKey != "" {
			info.Keys[pid] = true
		}
	}
	if m.signedIn() {
		info.Keys[config.ChatGPTProvider] = true
	}
	if id == config.ChatGPTProvider {
		if l := m.limits.Load(); l != nil {
			info.Usage = usageText(*l)
		}
	}
	m.settingsErr = ""
	return info
}

// noteLimits is the subscription client's OnLimits callback: it runs on the engine
// goroutine, so it only touches the atomic pointer.
func (m *Model) noteLimits(l llm.Limits) { m.limits.Store(&l) }

// agentConfig is the engine config plus the subscription sign-in and its usage sink.
func (m *Model) agentConfig() vibeai.Config {
	cfg := vibeAIConfig(m.cfg)
	if id, _, _ := m.cfg.ActiveLLM(); id == config.ChatGPTProvider {
		if store, err := m.chatGPT(); err == nil {
			cfg.Tokens = store
		}
		cfg.OnLimits = m.noteLimits
	}
	return cfg
}

// chatgptModelsCmd signs in when the store has no session and lists the
// subscription's models; the overlay's model form answers it like a key provider.
func (m *Model) chatgptModelsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		store, err := m.chatGPT()
		if err != nil {
			return stageModelsMsg{err: stage.Tr("вход не удался: ", "sign-in failed: ") + err.Error()}
		}
		if !store.LoggedIn() {
			if err := chatgptLogin(ctx, store); err != nil {
				return stageModelsMsg{err: stage.Tr("вход не удался: ", "sign-in failed: ") + err.Error()}
			}
		}
		all, err := stageListChatGPTModels(ctx, nil, "", store)
		return chatgptModelsReply(all, err)
	}
}

func chatgptModelsReply(all []llm.ModelInfo, err error) stageModelsMsg {
	if err != nil {
		if errors.Is(err, chatgpt.ErrSignedOut) || errors.Is(err, llm.ErrUnauthorized) {
			return stageModelsMsg{err: stage.Tr("вход истёк — войди снова", "the sign-in expired, sign in again")}
		}
		return stageModelsMsg{err: stage.Tr("не достучался: ", "unreachable: ") + err.Error()}
	}
	list := make([]stage.ModelInfo, len(all))
	rec := slices.IndexFunc(all, func(mi llm.ModelInfo) bool {
		return strings.Contains(mi.ID, "luna") || strings.Contains(mi.ID, "mini")
	})
	if rec < 0 && len(all) > 0 {
		rec = 0
	}
	for i, mi := range all {
		list[i] = stage.ModelInfo{ID: mi.ID, Reasoning: mi.Reasoning, Rec: i == rec}
	}
	return stageModelsMsg{list: list, total: len(all)}
}

// usageText formats the subscription windows: "5h: 34% · week: 12%".
func usageText(l llm.Limits) string {
	return fmt.Sprintf("%s: %.0f%% · %s: %.0f%%",
		usageWindow(l.PrimaryWindow), math.Round(l.PrimaryPercent),
		usageWindow(l.SecondaryWindow), math.Round(l.SecondaryPercent))
}

func usageWindow(d time.Duration) string {
	switch {
	case d == 7*24*time.Hour:
		return stage.Tr("неделя", "week")
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	default:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
}

// refreshSettings re-reads the host state into the overlay after a settings action.
func (s *stageScreen) refreshSettings() { s.st.OpenSettings(s.host.settingsInfo()) }
