package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/oglenyaboss/aux-player/internal/audioquality"
)

type Config struct {
	// path remembers where this config was loaded from. It is deliberately not
	// serialized; it only ensures that Save("") persists back to the selected
	// --config file instead of silently falling back to the default profile.
	path string

	AppleDeveloperToken string `json:"apple_developer_token"`
	AppleUserToken      string `json:"apple_user_token"`
	StoreFront          string `json:"storefront"`
	AuthPort            int    `json:"auth_port"`
	AudioQuality        string `json:"audio_quality,omitempty"`
	// on-disk cache of encrypted audio segments in MB; 0 = 2048, negative = off
	AudioCacheMB int `json:"audio_cache_mb,omitempty"`
	// Volume is the last user-set playback volume (0.0–1.0). nil means
	// "not yet saved" and the player default (1.0) is used on startup.
	Volume *float64 `json:"volume,omitempty"`
	// Last.fm scrobbling. LastfmAPIKey and LastfmAPISecret are typically
	// embedded in the binary at build time via ldflags; set them manually here
	// only when building from source without the embedded keys.
	LastfmAPIKey     string `json:"lastfm_api_key,omitempty"`
	LastfmAPISecret  string `json:"lastfm_api_secret,omitempty"`
	LastfmSessionKey string `json:"lastfm_session_key,omitempty"`
	// EQBands stores the last saved 10-band equalizer settings. nil means flat.
	EQBands []EQBand `json:"eq_bands,omitempty"`
	// WSL enables audio tuning workarounds for WSL2 environments where Hyper-V
	// scheduler jitter causes audio underruns with default Chrome buffer sizes.
	WSL bool `json:"wsl,omitempty"`
	// MusicDir is the local directory to scan for audio files
	// Supports MP3, FLAC, M4A. Scanned recursively.
	MusicDir string `json:"music_dir,omitempty"`
	// DiscordRPC enables Discord Rich Presence (defaults to true).
	DiscordRPC *bool `json:"discord_rpc,omitempty"`
	// DiscordClientID overrides the default Discord application client ID.
	DiscordClientID string `json:"discord_client_id,omitempty"`
	// Vibe AI: the active picker-agent provider and one stored profile per provider.
	VibeProvider  string                 `json:"vibe_provider,omitempty"`
	VibeProviders map[string]LLMProvider `json:"vibe_providers,omitempty"`
	// Legacy single-provider fields: read by normalize, migrated and then cleared so Save never writes them.
	VibeLLMBaseURL string `json:"vibe_llm_base_url,omitempty"`
	VibeLLMAPIKey  string `json:"vibe_llm_api_key,omitempty"`
	VibeLLMModel   string `json:"vibe_llm_model,omitempty"`
	VibeLLMEffort  string `json:"vibe_llm_effort,omitempty"`
	VibeLang       string `json:"vibe_lang,omitempty"`
	VibeEggs       *bool  `json:"vibe_eggs,omitempty"` // nil = on
	// picker conversation size in tokens before it compacts; 0 = 200000
	VibeContextLimit int `json:"vibe_context_limit,omitempty"`
}

type EQBand struct {
	Frequency float64 `json:"frequency"`
	Q         float64 `json:"q"`
	Gain      float64 `json:"gain"`
}

// VolumeOrDefault returns the saved volume, or 1.0 if none has been stored yet.
func (c *Config) VolumeOrDefault() float64 {
	if c.Volume != nil {
		return *c.Volume
	}
	return 1.0
}

// DiscordRPCEnabled reports whether Discord Rich Presence is enabled (default true).
func (c *Config) DiscordRPCEnabled() bool {
	if c.DiscordRPC != nil {
		return *c.DiscordRPC
	}
	return true
}

// SetVolume updates the in-memory volume field. Call Save to persist it.
func (c *Config) SetVolume(v float64) {
	c.Volume = &v
}

func defaults() *Config {
	return &Config{
		AuthPort:     7777,
		AudioQuality: "high",
	}
}

func (c *Config) AudioBitrateKbps() (int, error) {
	return audioquality.Parse(c.AudioQuality)
}

func (c *Config) SetAudioBitrate(kbps int) error {
	value, err := audioquality.ConfigValue(kbps)
	if err != nil {
		return err
	}
	c.AudioQuality = value
	return nil
}

func ConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("could not determine config directory: %w", err)
		}
		return filepath.Join(dir, "aux", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "aux", "config.json"), nil
}

func Load(override string) (*Config, error) {
	path, err := ConfigPath(override)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path comes from user config, not external input
	if os.IsNotExist(err) {
		cfg := defaults()
		cfg.path = path
		if saveErr := cfg.save(path); saveErr != nil {
			return nil, fmt.Errorf("creating default config: %w", saveErr)
		}
		fmt.Printf("Created default config at %s\n", path)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.path = path
	cfg.normalize()
	return cfg, nil
}

// normalize replaces zero values with defaults so that an existing config file
// with missing or empty fields still behaves correctly.
func (c *Config) normalize() {
	if c.AuthPort == 0 {
		c.AuthPort = 7777
	}
	if c.AudioQuality == "" {
		c.AudioQuality = "high"
	}
	if len(c.VibeProviders) == 0 && c.VibeLLMAPIKey != "" {
		url := c.VibeLLMBaseURL
		if url == "" {
			url = "https://opencode.ai/zen/go/v1"
		}
		id := ProviderForURL(url)
		model := c.VibeLLMModel
		if model == "" {
			model = "deepseek-v4.1-flash"
		}
		p := LLMProvider{APIKey: c.VibeLLMAPIKey, Model: model, Effort: c.VibeLLMEffort}
		if id == CustomProvider {
			p.BaseURL = url
		}
		c.SetLLM(id, p)
	}
	c.VibeLLMBaseURL, c.VibeLLMAPIKey, c.VibeLLMModel, c.VibeLLMEffort = "", "", "", ""
}

func (c *Config) Save(override string) error {
	path := override
	if path == "" {
		path = c.path
	}
	if path == "" {
		var err error
		path, err = ConfigPath("")
		if err != nil {
			return err
		}
	}
	if err := c.save(path); err != nil {
		return err
	}
	c.path = path
	return nil
}

func (c *Config) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}
