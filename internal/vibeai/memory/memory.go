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
	Text     string // the preference itself, ≤ 300 runes
	Evidence string // what it rests on: the owner's words or action, ≤ 200 runes
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
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is the user's own memory file
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
	fmt.Fprintf(&b, "## Memory about the listener (%d/%d chars)\n", s.Size(), s.Limit)
	if len(s.entries) == 0 {
		b.WriteString("(empty)\n")
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
		return fmt.Errorf("a similar entry already exists: \"%s\" — use replace", other.Text)
	}
	t := s.today()
	newEntries := append(s.Entries(), Entry{Text: e.Text, Evidence: e.Evidence, Created: t, Updated: t})
	return s.commitWithinLimit(newEntries)
}

func (s *Store) Replace(old, text, evidence string) error {
	old = strings.TrimSpace(old)
	idx := findMatches(s.entries, old)
	if len(idx) == 0 {
		return fmt.Errorf("entry not found: \"%s\"", old)
	}
	if len(idx) > 1 {
		return fmt.Errorf("ambiguous: %d entries contain \"%s\" — be more specific", len(idx), old)
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
		return fmt.Errorf("a similar entry already exists: \"%s\" — use replace", other.Text)
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
		return errors.New("give a reason for the removal")
	}
	old = strings.TrimSpace(old)
	idx := findMatches(s.entries, old)
	if len(idx) == 0 {
		return fmt.Errorf("entry not found: \"%s\"", old)
	}
	if len(idx) > 1 {
		return fmt.Errorf("ambiguous: %d entries contain \"%s\" — be more specific", len(idx), old)
	}
	newEntries := s.Entries()
	newEntries = append(newEntries[:idx[0]], newEntries[idx[0]+1:]...)
	return s.commit(newEntries)
}

func (s *Store) Consolidate(entries []Entry) error {
	cur := s.Size()
	if cur*100 < 70*s.Limit {
		return fmt.Errorf("consolidation is not needed: memory is %d%% full", cur*100/s.Limit)
	}
	if len(entries) == 0 {
		return errors.New("pass a non-empty list of entries")
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
				return fmt.Errorf("duplicate entries: \"%s\"", prev.Text)
			}
		}
		newEntries = append(newEntries, Entry{Text: checked.Text, Evidence: checked.Evidence, Created: t, Updated: t})
	}
	total := sizeOf(newEntries)
	if total*100 > 60*s.Limit {
		return fmt.Errorf("after consolidation %d%% — compress harder (need ≤ 60%%)", total*100/s.Limit)
	}
	return s.commit(newEntries)
}

func checkEntry(text, evidence string) (Entry, error) {
	text = strings.TrimSpace(text)
	evidence = strings.TrimSpace(evidence)
	if text == "" {
		return Entry{}, errors.New("give the entry text")
	}
	if n := utf8.RuneCountInString(text); n > maxTextLen {
		return Entry{}, fmt.Errorf("text is too long: %d characters, maximum %d — shorten it", n, maxTextLen)
	}
	if evidence == "" {
		return Entry{}, errors.New("give evidence — what the preference is based on")
	}
	if n := utf8.RuneCountInString(evidence); n > maxEvidenceLen {
		return Entry{}, fmt.Errorf("evidence is too long: %d characters, maximum %d — shorten it", n, maxEvidenceLen)
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
		return fmt.Errorf("memory is full (%d/%d) — memory_consolidate first", sz, s.Limit)
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
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(render(entries)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ponytail: an entry text that holds a "§" line or a line like "<!-- ... -->"
// breaks the parsing of the file on read; add a ban to checkEntry when needed.
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
			return errors.New("a metadata line <!-- ... --> is expected at the end of each entry")
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
	for line := range strings.SplitSeq(strings.TrimRight(content, "\n\r"), "\n") {
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
