package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
}

func openStore(t *testing.T, path string, limit int) *Store {
	t.Helper()
	st, err := Open(path, limit)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	st.now = fixedNow
	return st
}

func mustAdd(t *testing.T, st *Store, text, evidence string) {
	t.Helper()
	if err := st.Add(text, evidence); err != nil {
		t.Fatalf("Add(%q, %q): %v", text, evidence, err)
	}
}

func TestOpenMissingFileEmpty(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	if got := len(st.Entries()); got != 0 {
		t.Fatalf("записей: %d, want 0", got)
	}
	if !strings.Contains(st.Section(), "(пусто)") {
		t.Fatalf("Section без (пусто): %q", st.Section())
	}
}

func TestAddPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	reloaded := openStore(t, path, DefaultLimit)
	entries := reloaded.Entries()
	if len(entries) != 1 {
		t.Fatalf("записей: %d, want 1", len(entries))
	}
	want := Entry{Text: "не нравится Gunna", Evidence: "владелец: «Gunna нет»", Created: "2026-10-21", Updated: "2026-10-21"}
	if entries[0] != want {
		t.Fatalf("запись: %+v, want %+v", entries[0], want)
	}
}

func TestAddEmptyEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	if err := st.Add("нравится джаз", ""); err == nil {
		t.Fatal("ожидалась ошибка при пустом evidence")
	}
	if got := len(st.Entries()); got != 0 {
		t.Fatalf("записей: %d, want 0", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("файл не должен существовать, err: %v", err)
	}
}

func TestAddTextLengthLimit(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	if err := st.Add(strings.Repeat("ы", 301), "e"); err == nil {
		t.Fatal("ожидалась ошибка при 301 руне")
	}
	mustAdd(t, st, strings.Repeat("ы", 300), "e")
	if got := len(st.Entries()); got != 1 {
		t.Fatalf("записей: %d, want 1", got)
	}
}

func TestAddDuplicate(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	err := st.Add("Не нравится gunna!", "x")
	if err == nil || !strings.Contains(err.Error(), "уже есть похожая запись") {
		t.Fatalf("err: %v", err)
	}
}

func TestReplaceKeepsCreated(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	if err := st.Replace("gunna", "не нравятся Gunna и Lil Tecca", "слова владельца"); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	entries := st.Entries()
	if len(entries) != 1 {
		t.Fatalf("записей: %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Text != "не нравятся Gunna и Lil Tecca" || e.Evidence != "слова владельца" || e.Created != "2026-10-21" || e.Updated != "2026-10-21" {
		t.Fatalf("запись: %+v", e)
	}
}

func TestReplaceAmbiguous(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Tecca", "слова владельца")
	mustAdd(t, st, "Tecca слабый", "слова владельца")
	err := st.Replace("tecca", "новый текст", "e")
	if err == nil || !strings.Contains(err.Error(), "неоднозначно: 2") {
		t.Fatalf("err: %v", err)
	}
}

func TestRemove(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	if err := st.Remove("нет такой", "r"); err == nil || !strings.Contains(err.Error(), "запись не найдена") {
		t.Fatalf("err: %v", err)
	}
	if err := st.Remove("Gunna", ""); err == nil || !strings.Contains(err.Error(), "причину") {
		t.Fatalf("err: %v", err)
	}
}

func TestFillAndConsolidate(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), 200)
	text := func(i int) string { return strings.Repeat("очень ", 12) + fmt.Sprintf("артист %d", i) }
	var lastErr error
	for i := 0; i < 50; i++ {
		if err := st.Add(text(i), "слова владельца"); err != nil {
			lastErr = err
			break
		}
	}
	if lastErr == nil || !strings.Contains(lastErr.Error(), "память заполнена") {
		t.Fatalf("lastErr: %v", lastErr)
	}
	if st.Size() == 0 {
		t.Fatal("ни одной записи не добавлено")
	}
	if err := st.Consolidate([]Entry{{Text: "нравится джаз", Evidence: "слова владельца"}}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if st.Size() > 120 {
		t.Fatalf("Size после консолидации: %d > 120", st.Size())
	}
}

func TestConsolidateNotNeeded(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), 1000)
	mustAdd(t, st, "нравится джаз", "слова владельца")
	err := st.Consolidate([]Entry{{Text: "сжато", Evidence: "e"}})
	if err == nil || !strings.Contains(err.Error(), "консолидация не нужна") {
		t.Fatalf("err: %v", err)
	}
}

func TestEvidenceQuotesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "не нравится автотюн", `сказал "нет"`)
	reloaded := openStore(t, path, DefaultLimit)
	if got := reloaded.Entries()[0].Evidence; got != `сказал "нет"` {
		t.Fatalf("evidence: %q", got)
	}
}

func TestMultilineTextRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "нравится атмосфера\nи глубокий бас", "настройка в чате")
	reloaded := openStore(t, path, DefaultLimit)
	if got := reloaded.Entries()[0].Text; got != "нравится атмосфера\nи глубокий бас" {
		t.Fatalf("text: %q", got)
	}
	section := reloaded.Section()
	if !strings.Contains(section, "- нравится атмосфера и глубокий бас\n") {
		t.Fatalf("section: %q", section)
	}
}
