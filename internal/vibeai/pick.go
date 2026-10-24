package vibeai

type Song struct {
	ID         string
	Artist     string
	Title      string
	Album      string
	Year       string
	ArtworkURL string `json:"artwork,omitempty"`
	DurationMs int    `json:"duration_ms,omitempty"`
}

type Library struct {
	songs map[string]map[string]bool
}

func NewLibrary(songs []Song) *Library {
	l := &Library{songs: make(map[string]map[string]bool)}
	for _, s := range songs {
		t := normTitle(s.Title)
		arts := splitArtists(s.Artist)
		if t == "" && len(arts) == 0 {
			continue
		}
		m := l.songs[t]
		if m == nil {
			m = make(map[string]bool)
			l.songs[t] = m
		}
		for _, a := range arts {
			m[a] = true
		}
	}
	return l
}

func (l *Library) Has(s Song) bool {
	if l == nil {
		return false
	}
	m := l.songs[normTitle(s.Title)]
	if len(m) == 0 {
		return false
	}
	for _, a := range splitArtists(s.Artist) {
		if m[a] {
			return true
		}
	}
	return false
}

func KnownArtists(favorites []Song) map[string]bool {
	out := make(map[string]bool)
	for _, s := range favorites {
		for _, a := range splitArtists(s.Artist) {
			out[a] = true
		}
	}
	return out
}
