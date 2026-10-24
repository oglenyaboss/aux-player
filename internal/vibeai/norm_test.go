package vibeai

import (
	"testing"
)

func TestNormKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Ёлка", "елка"},
		{"Мало 2.0", "мало20"},
		{"A$AP Rocky", "aaprocky"},
	}
	for _, tt := range tests {
		if got := normKey(tt.in); got != tt.want {
			t.Errorf("normKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"goosebumps (feat. Kendrick Lamar)", "goosebumps"},
		{"SICKO MODE [feat. Drake]", "sickomode"},
		{"Song - feat. Someone", "song"},
		{"Highest in the Room (Remix)", "highestintheroomremix"},
		{"With You", "withyou"},
		{"Мало 2.0", "мало20"},
		{"Ёлка (prod. Someone)", "елка"},
	}
	for _, tt := range tests {
		if got := normTitle(tt.in); got != tt.want {
			t.Errorf("normTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitArtists(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"Offset & Metro Boomin", []string{"metroboomin", "offset"}},
		{"Metro Boomin, Offset", []string{"metroboomin", "offset"}},
		{"Travis Scott feat. Kendrick Lamar", []string{"kendricklamar", "travisscott"}},
		{"Lil Nas X & Billy Ray Cyrus", []string{"billyraycyrus", "lilnasx"}},
		{"A$AP Rocky", []string{"aaprocky"}},
		{"9mice, Kai Angel", []string{"9mice", "kaiangel"}},
	}
	for _, tt := range tests {
		if got := splitArtists(tt.in); !equal(got, tt.want) {
			t.Errorf("splitArtists(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
