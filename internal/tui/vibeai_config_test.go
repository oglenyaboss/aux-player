package tui

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/config"
)

func TestVibeAIConfigPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("expected paths cover the Linux layout")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg := vibeAIConfig(&config.Config{})
	if want := filepath.Join(home, ".config", "aux", "vibe-memory.md"); cfg.MemoryPath != want {
		t.Errorf("MemoryPath = %q, want %q", cfg.MemoryPath, want)
	}
	if want := filepath.Join(home, ".config", "aux", "vibe-signals.jsonl"); cfg.SignalsPath != want {
		t.Errorf("SignalsPath = %q, want %q", cfg.SignalsPath, want)
	}
	if want := filepath.Join(home, ".cache", "aux", "vibe", "runs"); cfg.RunsDir != want {
		t.Errorf("RunsDir = %q, want %q", cfg.RunsDir, want)
	}
}
