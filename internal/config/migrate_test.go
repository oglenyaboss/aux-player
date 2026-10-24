package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/config"
)

func isolateMigration(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("migration tests cover the Linux config/cache layout")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func writeMigrationFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readMigrationFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestMigrateConfigCopies(t *testing.T) {
	home := isolateMigration(t)
	oldDir := filepath.Join(home, ".config", "vibez")
	writeMigrationFile(t, filepath.Join(oldDir, "config.json"), `{"a":1}`, 0o600)
	writeMigrationFile(t, filepath.Join(oldDir, "vibe-signals.jsonl"), "x", 0o644)
	writeMigrationFile(t, filepath.Join(oldDir, "themes", "t.json"), "{}", 0o644)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	newDir := filepath.Join(home, ".config", "aux")
	if got := readMigrationFile(t, filepath.Join(newDir, "config.json")); got != `{"a":1}` {
		t.Errorf("config.json = %q, want %q", got, `{"a":1}`)
	}
	if got := readMigrationFile(t, filepath.Join(newDir, "vibe-signals.jsonl")); got != "x" {
		t.Errorf("vibe-signals.jsonl = %q, want %q", got, "x")
	}
	if got := readMigrationFile(t, filepath.Join(newDir, "themes", "t.json")); got != "{}" {
		t.Errorf("themes/t.json = %q, want %q", got, "{}")
	}
	info, err := os.Stat(filepath.Join(newDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config.json mode = %o, want 600", got)
	}
	info, err = os.Stat(newDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("aux config dir mode = %o, want 700", got)
	}
	for _, name := range []string{"config.json", "vibe-signals.jsonl", filepath.Join("themes", "t.json")} {
		if _, err := os.Stat(filepath.Join(oldDir, name)); err != nil {
			t.Errorf("old %s missing after migration: %v", name, err)
		}
	}
}

func TestMigrateConfigKeepsExisting(t *testing.T) {
	home := isolateMigration(t)
	writeMigrationFile(t, filepath.Join(home, ".config", "vibez", "config.json"), "old", 0o600)
	writeMigrationFile(t, filepath.Join(home, ".config", "vibez", "vibe-signals.jsonl"), "x", 0o644)
	newDir := filepath.Join(home, ".config", "aux")
	writeMigrationFile(t, filepath.Join(newDir, "config.json"), "new", 0o600)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	if got := readMigrationFile(t, filepath.Join(newDir, "config.json")); got != "new" {
		t.Errorf("config.json = %q, want %q", got, "new")
	}
	if _, err := os.Stat(filepath.Join(newDir, "vibe-signals.jsonl")); !os.IsNotExist(err) {
		t.Errorf("vibe-signals.jsonl appeared in the existing target (stat err = %v)", err)
	}
}

func TestMigrateConfigLeftover(t *testing.T) {
	home := isolateMigration(t)
	writeMigrationFile(t, filepath.Join(home, ".config", "vibez", "config.json"), `{"a":1}`, 0o600)
	staging := filepath.Join(home, ".config", "aux.migrating")
	writeMigrationFile(t, filepath.Join(staging, "junk"), "junk", 0o644)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	newDir := filepath.Join(home, ".config", "aux")
	if got := readMigrationFile(t, filepath.Join(newDir, "config.json")); got != `{"a":1}` {
		t.Errorf("config.json = %q, want %q", got, `{"a":1}`)
	}
	if _, err := os.Stat(filepath.Join(newDir, "junk")); !os.IsNotExist(err) {
		t.Errorf("leftover junk was copied (stat err = %v)", err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Errorf("staging dir still present (stat err = %v)", err)
	}
}

func TestMigrateCacheRenames(t *testing.T) {
	home := isolateMigration(t)
	oldDir := filepath.Join(home, ".cache", "vibez")
	writeMigrationFile(t, filepath.Join(oldDir, "vibe", "runs", "r.md"), "run", 0o644)
	writeMigrationFile(t, filepath.Join(oldDir, "driver", "d"), "", 0o644)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	newDir := filepath.Join(home, ".cache", "aux")
	if got := readMigrationFile(t, filepath.Join(newDir, "vibe", "runs", "r.md")); got != "run" {
		t.Errorf("vibe/runs/r.md = %q, want %q", got, "run")
	}
	if _, err := os.Stat(filepath.Join(newDir, "driver", "d")); err != nil {
		t.Errorf("driver/d missing after migration: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old cache dir still present (stat err = %v)", err)
	}
}

func TestMigrateCacheKeepsExisting(t *testing.T) {
	home := isolateMigration(t)
	oldDir := filepath.Join(home, ".cache", "vibez")
	writeMigrationFile(t, filepath.Join(oldDir, "x"), "x", 0o644)
	newDir := filepath.Join(home, ".cache", "aux")
	writeMigrationFile(t, filepath.Join(newDir, "y"), "y", 0o644)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	if _, err := os.Stat(filepath.Join(oldDir, "x")); err != nil {
		t.Errorf("old cache file missing after migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "x")); !os.IsNotExist(err) {
		t.Errorf("old cache file copied into an existing target (stat err = %v)", err)
	}
}

func TestMigrateNothing(t *testing.T) {
	home := isolateMigration(t)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("MigrateLegacy: %v", err)
	}

	for _, dir := range []string{filepath.Join(home, ".config", "aux"), filepath.Join(home, ".cache", "aux")} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s was created for an empty home (stat err = %v)", dir, err)
		}
	}
}

func TestMigrateTwice(t *testing.T) {
	home := isolateMigration(t)
	writeMigrationFile(t, filepath.Join(home, ".config", "vibez", "config.json"), `{"a":1}`, 0o600)
	writeMigrationFile(t, filepath.Join(home, ".cache", "vibez", "vibe", "runs", "r.md"), "run", 0o644)

	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("first MigrateLegacy: %v", err)
	}
	if err := config.MigrateLegacy(); err != nil {
		t.Fatalf("second MigrateLegacy: %v", err)
	}

	if got := readMigrationFile(t, filepath.Join(home, ".config", "aux", "config.json")); got != `{"a":1}` {
		t.Errorf("config.json = %q, want %q", got, `{"a":1}`)
	}
	if got := readMigrationFile(t, filepath.Join(home, ".cache", "aux", "vibe", "runs", "r.md")); got != "run" {
		t.Errorf("runs/r.md = %q, want %q", got, "run")
	}
}

func TestConfigPathAux(t *testing.T) {
	home := isolateMigration(t)
	want := filepath.Join(home, ".config", "aux", "config.json")
	got, err := config.ConfigPath("")
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if got != want {
		t.Errorf("ConfigPath(\"\") = %q, want %q", got, want)
	}
}
