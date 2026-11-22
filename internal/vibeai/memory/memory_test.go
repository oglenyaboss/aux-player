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
		t.Fatalf("entries: %d, want 0", got)
	}
	if !strings.Contains(st.Section(), "(empty)") {
		t.Fatalf("Section without (empty): %q", st.Section())
	}
}

func TestAddPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was not created: %v", err)
	}
	reloaded := openStore(t, path, DefaultLimit)
	entries := reloaded.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries: %d, want 1", len(entries))
	}
	want := Entry{Text: "не нравится Gunna", Evidence: "владелец: «Gunna нет»", Created: "2026-10-21", Updated: "2026-10-21"}
	if entries[0] != want {
		t.Fatalf("entry: %+v, want %+v", entries[0], want)
	}
}

func TestAddEmptyEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	if err := st.Add("нравится джаз", ""); err == nil {
		t.Fatal("an error was expected for empty evidence")
	}
	if got := len(st.Entries()); got != 0 {
		t.Fatalf("entries: %d, want 0", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the file must not exist, err: %v", err)
	}
}

func TestAddTextLengthLimit(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	if err := st.Add(strings.Repeat("ы", 301), "e"); err == nil {
		t.Fatal("an error was expected for 301 runes")
	}
	mustAdd(t, st, strings.Repeat("ы", 300), "e")
	if got := len(st.Entries()); got != 1 {
		t.Fatalf("entries: %d, want 1", got)
	}
}

func TestAddDuplicate(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	err := st.Add("Не нравится gunna!", "x")
	if err == nil || !strings.Contains(err.Error(), "a similar entry already exists") {
		t.Fatalf("err: %v", err)
	}
}

func TestReplaceAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "dislikes Gunna", "owner: no Gunna")
	mustAdd(t, st, "Tecca is weak", "owner's words")
	for _, idx := range []int{-1, 2} {
		if err := st.ReplaceAt(idx, "new text", "e"); err == nil {
			t.Fatalf("ReplaceAt(%d) passed, want an out-of-range error", idx)
		}
	}
	st.now = func() time.Time { return fixedNow().Add(24 * time.Hour) }
	if err := st.ReplaceAt(0, "dislikes Gunna and Lil Tecca", "chat on Oct 20"); err != nil {
		t.Fatalf("ReplaceAt: %v", err)
	}
	entries := st.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries: %d, want 2", len(entries))
	}
	if e := entries[0]; e.Text != "dislikes Gunna and Lil Tecca" || e.Evidence != "chat on Oct 20" ||
		e.Created != "2026-10-21" || e.Updated != "2026-10-22" {
		t.Fatalf("entry: %+v", e)
	}
	err := st.ReplaceAt(1, "dislikes Gunna and Lil Tecca", "e")
	if err == nil || !strings.Contains(err.Error(), "a similar entry already exists") {
		t.Fatalf("dup err: %v", err)
	}
	if err := st.ReplaceAt(1, "Tecca is weak!", "e"); err != nil {
		t.Fatalf("minor edit of the entry itself: %v", err)
	}
	reloaded := openStore(t, path, DefaultLimit)
	got := reloaded.Entries()
	if len(got) != 2 || got[0].Text != "dislikes Gunna and Lil Tecca" || got[1].Text != "Tecca is weak!" {
		t.Fatalf("reloaded: %+v", got)
	}
}

func TestRemoveAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	mustAdd(t, st, "first", "e1")
	mustAdd(t, st, "second", "e2")
	mustAdd(t, st, "third", "e3")
	for _, idx := range []int{-1, 3} {
		if err := st.RemoveAt(idx); err == nil {
			t.Fatalf("RemoveAt(%d) passed, want an out-of-range error", idx)
		}
	}
	if err := st.RemoveAt(1); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}
	if entries := st.Entries(); len(entries) != 2 || entries[0].Text != "first" || entries[1].Text != "third" {
		t.Fatalf("entries: %+v", entries)
	}
	reloaded := openStore(t, path, DefaultLimit)
	if got := reloaded.Entries(); len(got) != 2 || got[0].Text != "first" || got[1].Text != "third" {
		t.Fatalf("reloaded: %+v", got)
	}
}

func TestCheckEntryDelimiterGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	st := openStore(t, path, DefaultLimit)
	if err := st.Add("foo\n\u00a7\nbar", "ev"); err == nil {
		t.Fatal("a \u00a7 line in text must be rejected")
	}
	if err := st.Add("foo\n<!-- comment -->", "ev"); err == nil {
		t.Fatal("a <!-- line in text must be rejected")
	}
	if err := st.Add("foo", "ev\n<!-- meta -->"); err == nil {
		t.Fatal("a <!-- line in evidence must be rejected")
	}
	if err := st.Add("foo", "ev\n  \u00a7  "); err == nil {
		t.Fatal("a padded \u00a7 line in evidence must be rejected")
	}
	mustAdd(t, st, "section \u00a7 2 of the chat", "see \u00a7 3")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the valid entry must persist: %v", err)
	}
	reloaded := openStore(t, path, DefaultLimit)
	if got := reloaded.Entries()[0].Text; got != "section \u00a7 2 of the chat" {
		t.Fatalf("text: %q", got)
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
		t.Fatalf("entries: %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Text != "не нравятся Gunna и Lil Tecca" || e.Evidence != "слова владельца" || e.Created != "2026-10-21" || e.Updated != "2026-10-21" {
		t.Fatalf("entry: %+v", e)
	}
}

func TestReplaceAmbiguous(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Tecca", "слова владельца")
	mustAdd(t, st, "Tecca слабый", "слова владельца")
	err := st.Replace("tecca", "новый текст", "e")
	if err == nil || !strings.Contains(err.Error(), "ambiguous: 2") {
		t.Fatalf("err: %v", err)
	}
}

func TestRemove(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), DefaultLimit)
	mustAdd(t, st, "не нравится Gunna", "владелец: «Gunna нет»")
	if err := st.Remove("нет такой", "r"); err == nil || !strings.Contains(err.Error(), "entry not found") {
		t.Fatalf("err: %v", err)
	}
	if err := st.Remove("Gunna", ""); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("err: %v", err)
	}
}

func TestFillAndConsolidate(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), 200)
	text := func(i int) string { return strings.Repeat("очень ", 12) + fmt.Sprintf("артист %d", i) }
	var lastErr error
	for i := range 50 {
		if err := st.Add(text(i), "слова владельца"); err != nil {
			lastErr = err
			break
		}
	}
	if lastErr == nil || !strings.Contains(lastErr.Error(), "memory is full") {
		t.Fatalf("lastErr: %v", lastErr)
	}
	if st.Size() == 0 {
		t.Fatal("no entry was added")
	}
	if err := st.Consolidate([]Entry{{Text: "нравится джаз", Evidence: "слова владельца"}}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if st.Size() > 120 {
		t.Fatalf("Size after consolidation: %d > 120", st.Size())
	}
}

func TestConsolidateNotNeeded(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "memory.md"), 1000)
	mustAdd(t, st, "нравится джаз", "слова владельца")
	err := st.Consolidate([]Entry{{Text: "сжато", Evidence: "e"}})
	if err == nil || !strings.Contains(err.Error(), "consolidation is not needed") {
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
