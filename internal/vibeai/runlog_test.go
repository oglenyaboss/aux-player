package vibeai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunLogSave(t *testing.T) {
	var l runLog
	l.f("первая строка")
	l.f("вторая %d", 2)
	path, err := l.save(t.TempDir(), "Don Toliver — HARDSTONE PSYCHO", time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if !strings.HasSuffix(path, "20261021-150405-dontoliverhardstonepsycho.md") {
		t.Errorf("path = %q, want suffix 20261021-150405-dontoliverhardstonepsycho.md", path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file the test just wrote under its temp dir
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "первая строка\nвторая 2\n"; string(data) != want {
		t.Errorf("content = %q, want %q", string(data), want)
	}
}

func TestRunLogSlugTruncatedTo40(t *testing.T) {
	var l runLog
	path, err := l.save(t.TempDir(), strings.Repeat("a", 60), time.Date(2026, 10, 21, 15, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	want := "20261021-150405-" + strings.Repeat("a", 40) + ".md"
	if base := filepath.Base(path); base != want {
		t.Errorf("base = %q, want %q", base, want)
	}
}
