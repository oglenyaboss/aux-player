package vibeai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runLog struct{ b strings.Builder }

func (l *runLog) f(format string, args ...any) {
	fmt.Fprintf(&l.b, format+"\n", args...)
}

func (l *runLog) save(dir, name string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	slug := []rune(normKey(name))
	if len(slug) > 40 {
		slug = slug[:40]
	}
	path := filepath.Join(dir, now.Format("20060102-150405")+"-"+string(slug)+".md")
	if err := os.WriteFile(path, []byte(l.b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
