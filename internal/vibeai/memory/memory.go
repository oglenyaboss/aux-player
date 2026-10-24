package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const DefaultLimit = 25000

const (
	dateLayout     = "2006-01-02"
	maxTextLen     = 300
	maxEvidenceLen = 200
)

type Entry struct {
	Text     string // само предпочтение, ≤ 300 рун
	Evidence string // на чём основано: слова/действие владельца, ≤ 200 рун
	Created  string // YYYY-MM-DD
	Updated  string // YYYY-MM-DD
}

type Store struct {
	path    string
	Limit   int
	entries []Entry
	now     func() time.Time
}

func Open(path string, limit int) (*Store, error) {
	s := &Store{path: path, Limit: limit, now: time.Now}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	entries, err := parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.entries = entries
	return s, nil
}

func (s *Store) Entries() []Entry {
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

func (s *Store) Size() int { return sizeOf(s.entries) }

func sizeOf(entries []Entry) int { return utf8.RuneCountInString(render(entries)) }

func (s *Store) Section() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Память о владельце (%d/%d символов)\n", s.Size(), s.Limit)
	if len(s.entries) == 0 {
		b.WriteString("(пусто)\n")
		return b.String()
	}
	for _, e := range s.entries {
		b.WriteString("- ")
		b.WriteString(strings.ReplaceAll(e.Text, "\n", " "))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *Store) Add(text, evidence string) error {
	e, err := checkEntry(text, evidence)
	if err != nil {
		return err
	}
	if other, dup := findDup(s.entries, e.Text); dup {
		return fmt.Errorf("уже есть похожая запись: «%s» — используй replace", other.Text)
	}
	t := s.today()
	newEntries := append(s.Entries(), Entry{Text: e.Text, Evidence: e.Evidence, Created: t, Updated: t})
	return s.commitWithinLimit(newEntries)
}

func (s *Store) Replace(old, text, evidence string) error {
	old = strings.TrimSpace(old)
	idx := findMatches(s.entries, old)
	if len(idx) == 0 {
		return fmt.Errorf("запись не найдена: «%s»", old)
	}
	if len(idx) > 1 {
		return fmt.Errorf("неоднозначно: %d записей содержат «%s» — уточни", len(idx), old)
	}
	e, err := checkEntry(text, evidence)
	if err != nil {
		return err
	}
	others := make([]Entry, 0, len(s.entries)-1)
	for i, en := range s.entries {
		if i != idx[0] {
			others = append(others, en)
		}
	}
	if other, dup := findDup(others, e.Text); dup {
		return fmt.Errorf("уже есть похожая запись: «%s» — используй replace", other.Text)
	}
	t := s.today()
	newEntries := s.Entries()
	newEntries[idx[0]] = Entry{
		Text:     e.Text,
		Evidence: e.Evidence,
		Created:  newEntries[idx[0]].Created,
		Updated:  t,
	}
	return s.commitWithinLimit(newEntries)
}

func (s *Store) Remove(old, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("укажи причину удаления")
	}
	old = strings.TrimSpace(old)
	idx := findMatches(s.entries, old)
	if len(idx) == 0 {
		return fmt.Errorf("запись не найдена: «%s»", old)
	}
	if len(idx) > 1 {
		return fmt.Errorf("неоднозначно: %d записей содержат «%s» — уточни", len(idx), old)
	}
	newEntries := s.Entries()
	newEntries = append(newEntries[:idx[0]], newEntries[idx[0]+1:]...)
	return s.commit(newEntries)
}

func (s *Store) Consolidate(entries []Entry) error {
	cur := s.Size()
	if cur*100 < 70*s.Limit {
		return fmt.Errorf("консолидация не нужна: память заполнена на %d%%", cur*100/s.Limit)
	}
	if len(entries) == 0 {
		return errors.New("передай непустой список записей")
	}
	t := s.today()
	newEntries := make([]Entry, 0, len(entries))
	for _, e := range entries {
		checked, err := checkEntry(e.Text, e.Evidence)
		if err != nil {
			return err
		}
		for _, prev := range newEntries {
			n, m := normText(checked.Text), normText(prev.Text)
			if n == m || strings.Contains(n, m) || strings.Contains(m, n) {
				return fmt.Errorf("дублирующиеся записи: «%s»", prev.Text)
			}
		}
		newEntries = append(newEntries, Entry{Text: checked.Text, Evidence: checked.Evidence, Created: t, Updated: t})
	}
	total := sizeOf(newEntries)
	if total*100 > 60*s.Limit {
		return fmt.Errorf("после консолидации %d%% — сожми сильнее (нужно ≤ 60%%)", total*100/s.Limit)
	}
	return s.commit(newEntries)
}

func checkEntry(text, evidence string) (Entry, error) {
	text = strings.TrimSpace(text)
	evidence = strings.TrimSpace(evidence)
	if text == "" {
		return Entry{}, errors.New("укажи текст записи")
	}
	if n := utf8.RuneCountInString(text); n > maxTextLen {
		return Entry{}, fmt.Errorf("текст слишком длинный: %d символов, максимум %d — сократи", n, maxTextLen)
	}
	if evidence == "" {
		return Entry{}, errors.New("укажи evidence — на чём основано предпочтение")
	}
	if n := utf8.RuneCountInString(evidence); n > maxEvidenceLen {
		return Entry{}, fmt.Errorf("evidence слишком длинный: %d символов, максимум %d — сократи", n, maxEvidenceLen)
	}
	return Entry{Text: text, Evidence: evidence}, nil
}

func findDup(entries []Entry, text string) (Entry, bool) {
	n := normText(text)
	for _, e := range entries {
		m := normText(e.Text)
		if n == m || strings.Contains(m, n) || strings.Contains(n, m) {
			return e, true
		}
	}
	return Entry{}, false
}

func findMatches(entries []Entry, old string) []int {
	if old == "" {
		return nil
	}
	needle := strings.ToLower(old)
	var idx []int
	for i, e := range entries {
		if strings.Contains(strings.ToLower(e.Text), needle) {
			idx = append(idx, i)
		}
	}
	return idx
}

func normText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) today() string { return s.now().Format(dateLayout) }

func (s *Store) commitWithinLimit(entries []Entry) error {
	if sz := sizeOf(entries); sz > s.Limit {
		return fmt.Errorf("память заполнена (%d/%d) — сначала memory_consolidate", sz, s.Limit)
	}
	return s.commit(entries)
}

func (s *Store) commit(entries []Entry) error {
	if err := s.save(entries); err != nil {
		return err
	}
	s.entries = entries
	return nil
}

func (s *Store) save(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(render(entries)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ponytail: текст записи, содержащий строку "§" или строку вида "<!-- ... -->",
// сломает разбор файла при чтении; при необходимости добавить запрет в checkEntry.
func render(entries []Entry) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("§\n")
		}
		b.WriteString(e.Text)
		b.WriteString("\n<!-- created=")
		b.WriteString(e.Created)
		b.WriteString(" updated=")
		b.WriteString(e.Updated)
		b.WriteString(` evidence="`)
		b.WriteString(escapeMeta(e.Evidence))
		b.WriteString("\" -->\n")
	}
	return b.String()
}

func escapeMeta(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func parse(content string) ([]Entry, error) {
	var entries []Entry
	var block []string
	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		empty := true
		for _, line := range block {
			if line != "" {
				empty = false
				break
			}
		}
		if empty {
			block = block[:0]
			return nil
		}
		created, updated, evidence, ok := parseMeta(block[len(block)-1])
		if !ok {
			return errors.New("ожидается строка метаданных <!-- ... --> в конце каждой записи")
		}
		entries = append(entries, Entry{
			Text:     strings.Join(block[:len(block)-1], "\n"),
			Evidence: evidence,
			Created:  created,
			Updated:  updated,
		})
		block = block[:0]
		return nil
	}
	for _, line := range strings.Split(strings.TrimRight(content, "\n\r"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "§" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		block = append(block, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return entries, nil
}

func parseMeta(line string) (created, updated, evidence string, ok bool) {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "<!--") || !strings.HasSuffix(s, "-->") {
		return "", "", "", false
	}
	s = strings.TrimSpace(s[len("<!--") : len(s)-len("-->")])
	for _, p := range []struct {
		name string
		dst  *string
	}{{"created=", &created}, {"updated=", &updated}} {
		if !strings.HasPrefix(s, p.name) {
			return "", "", "", false
		}
		s = s[len(p.name):]
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			return "", "", "", false
		}
		*p.dst, s = s[:i], s[i+1:]
	}
	const evidencePrefix = `evidence="`
	if !strings.HasPrefix(s, evidencePrefix) {
		return "", "", "", false
	}
	s = s[len(evidencePrefix):]
	var b strings.Builder
	for j := 0; j < len(s); j++ {
		switch c := s[j]; {
		case c == '\\' && j+1 < len(s):
			j++
			b.WriteByte(s[j])
		case c == '"':
			if strings.TrimSpace(s[j+1:]) != "" {
				return "", "", "", false
			}
			return created, updated, b.String(), true
		default:
			b.WriteByte(c)
		}
	}
	return "", "", "", false
}
