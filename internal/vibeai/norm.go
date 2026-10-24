package vibeai

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normKey(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var featWords = []string{"feat", "featuring", "ft", "with", "prod"}

func featPrefix(inner string) bool {
	w := strings.ToLower(strings.TrimSpace(inner))
	for _, k := range featWords {
		if !strings.HasPrefix(w, k) {
			continue
		}
		rest := w[len(k):]
		if rest == "" {
			return true
		}
		r, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func stripBrackets(s string, drop func(inner string) bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		switch s[i] {
		case '(', '[':
			closeCh := byte(')')
			if s[i] == '[' {
				closeCh = ']'
			}
			if end := strings.IndexByte(s[i+1:], closeCh); end >= 0 && drop(s[i+1:i+1+end]) {
				i += end + 2
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

var featTails = []string{" - feat", " feat.", " ft."}

func indexFold(s, pat string) int {
	for i := 0; i+len(pat) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(pat)], pat) {
			return i
		}
	}
	return -1
}

func stripFeatTail(s string) string {
	cut := len(s)
	for _, p := range featTails {
		if i := indexFold(s, p); i >= 0 && i < cut {
			cut = i
		}
	}
	return s[:cut]
}

func normTitle(s string) string {
	return normKey(stripFeatTail(stripBrackets(s, featPrefix)))
}

var artistSeps = []string{" feat. ", " featuring ", " ft. ", " with ", " feat ", ",", "&"}

func splitArtists(s string) []string {
	var raw []string
	rest := s
	for {
		cut, skip := -1, 0
		for i := 0; i < len(rest) && cut < 0; i++ {
			for _, sep := range artistSeps {
				if i+len(sep) <= len(rest) && strings.EqualFold(rest[i:i+len(sep)], sep) {
					cut, skip = i, len(sep)
					break
				}
			}
		}
		if cut < 0 {
			raw = append(raw, rest)
			break
		}
		raw = append(raw, rest[:cut])
		rest = rest[cut+skip:]
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		k := normKey(p)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func songKey(s Song) string {
	return normTitle(s.Title) + "|" + strings.Join(splitArtists(s.Artist), ",")
}
