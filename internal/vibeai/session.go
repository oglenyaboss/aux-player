package vibeai

import (
	"fmt"
	"sort"
	"strings"
)

const windowSize = 20

type Seed struct {
	Artist string
	Mode   string // "not_it" | "and_similar" | "want_it"
}

type PlaylistPlan struct {
	Size        int    `json:"size"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Plan struct {
	Seeds         []Seed
	Exclusions    []string // artist names
	FromFavorites bool
	Playlist      *PlaylistPlan // nil — stream mode
}

type Entry struct {
	Song     Song
	Why      string
	Favorite bool // the track is from the owner's Favorites
}

type Verdict struct {
	Suggestion Suggestion
	Song       Song // empty if not found
	Accepted   bool
	Reason     string // "" if accepted
}

type reserved struct {
	sug  Suggestion
	song Song
}

type Session struct {
	lib, favs  *Library
	known      map[string]bool
	plan       Plan
	seeds      map[string]string
	given      map[string]bool
	recent     []Entry
	lastClosed int
	portion    []Entry
	inPortion  map[string]bool
	reserve    []reserved
	sig        *Signals
	sessionID  string
	stream     int
	Size       int
}

func NewSession(library, favorites []Song, size int) *Session {
	return &Session{
		lib:       NewLibrary(library),
		favs:      NewLibrary(favorites),
		known:     KnownArtists(favorites),
		given:     make(map[string]bool),
		inPortion: make(map[string]bool),
		stream:    size,
		Size:      size,
	}
}

func (s *Session) SetPlan(p Plan) {
	if p.Playlist == nil && s.plan.Playlist != nil {
		p.Playlist = s.plan.Playlist
	}
	s.plan = p
	if p.Playlist != nil {
		s.Size = p.Playlist.Size
	} else {
		s.Size = s.stream
	}
	s.seeds = make(map[string]string, len(p.Seeds))
	for _, sd := range p.Seeds {
		if k := normKey(sd.Artist); k != "" {
			s.seeds[k] = sd.Mode
		}
	}
}

func (s *Session) Playlist() *PlaylistPlan { return s.plan.Playlist }

func (s *Session) SetSignals(sig *Signals, sessionID string) {
	s.sig = sig
	s.sessionID = sessionID
}

func (s *Session) NoteSaved(song Song, favorite bool) {
	if favorite {
		s.favs.add(song)
		for _, a := range splitArtists(song.Artist) {
			s.known[a] = true
		}
		return
	}
	s.lib.add(song)
}

func (s *Session) Need() int {
	return max(s.Size-len(s.portion), 0)
}

func (s *Session) Offer(sug Suggestion, song Song, found bool) Verdict {
	v := Verdict{Suggestion: sug}
	if !found {
		v.Reason = "not in Apple"
		return v
	}
	v.Song = song
	k := songKey(song)
	if s.inPortion[k] {
		v.Reason = "already served"
		return v
	}
	if s.given[k] && !sug.Again {
		v.Reason = "already served"
		return v
	}
	if s.Need() == 0 {
		v.Reason = "portion is full"
		s.addReserve(sug, song)
		return v
	}
	return s.accept(sug, song)
}

func (s *Session) Take(sug Suggestion, song Song) Verdict {
	v := Verdict{Suggestion: sug, Song: song}
	k := songKey(song)
	if s.given[k] || s.inPortion[k] {
		v.Reason = "already served"
		return v
	}
	if s.Need() == 0 {
		v.Reason = "portion is full"
		return v
	}
	return s.accept(sug, song)
}

func (s *Session) accept(sug Suggestion, song Song) Verdict {
	s.portion = append(s.portion, Entry{Song: song, Why: sug.Why, Favorite: s.favs.Has(song)})
	s.inPortion[songKey(song)] = true
	return Verdict{Suggestion: sug, Song: song, Accepted: true}
}

func (s *Session) addReserve(sug Suggestion, song Song) {
	if len(s.reserve) >= s.Size {
		return
	}
	k := songKey(song)
	for _, r := range s.reserve {
		if songKey(r.song) == k {
			return
		}
	}
	s.reserve = append(s.reserve, reserved{sug: sug, song: song})
}

func (s *Session) TakeReserve() []string {
	names := make([]string, 0, len(s.reserve))
	for _, r := range s.reserve {
		names = append(names, r.song.Artist+" — "+r.song.Title)
	}
	s.reserve = nil
	return names
}

func (s *Session) ClearReserve() { s.reserve = nil }

func (s *Session) Done() bool {
	if s.plan.Playlist != nil {
		return s.Need() == 0
	}
	return s.Need() <= s.Size/10
}

func (s *Session) Portion() []Entry {
	return s.portion
}

// windowSummary renders the Window the model sees every turn: tracks per
// artist over the last windowSize served entries, tracks by Favorites
// artists and tracks by referents, with per-referent counts.
func (s *Session) windowSummary() string {
	if len(s.recent) == 0 {
		return ""
	}
	win := s.recent
	if len(win) > windowSize {
		win = win[len(win)-windowSize:]
	}
	type count struct {
		name  string
		count int
	}
	less := func(a, b count) bool {
		if a.count != b.count {
			return a.count > b.count
		}
		return a.name < b.name
	}
	artistCounts := make(map[string]int)
	refCounts := make(map[string]int)
	favTracks, refTracks := 0, 0
	for _, e := range win {
		arts := songArtists(e.Song)
		referent := false
		for _, a := range arts {
			artistCounts[a]++
			if _, ok := s.seeds[a]; ok {
				referent = true
				refCounts[a]++
			}
		}
		if referent {
			refTracks++
		}
		for _, a := range arts {
			if s.known[a] {
				favTracks++
				break
			}
		}
	}
	multi := make([]count, 0, len(artistCounts))
	singles := 0
	for a, c := range artistCounts {
		if c >= 2 {
			multi = append(multi, count{name: a, count: c})
		} else {
			singles++
		}
	}
	sort.Slice(multi, func(i, j int) bool { return less(multi[i], multi[j]) })
	artistPart := make([]string, 0, len(multi)+1)
	for _, m := range multi {
		artistPart = append(artistPart, fmt.Sprintf("%s %d", m.name, m.count))
	}
	switch {
	case len(multi) == 0:
		artistPart = append(artistPart, fmt.Sprintf("%d artists with 1 each", singles))
	case singles > 0:
		artistPart = append(artistPart, fmt.Sprintf("%d more artists with 1 each", singles))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Window, the last %d served tracks: by artist — %s; by Favorites artists: %d; by referents: %d",
		len(win), strings.Join(artistPart, ", "), favTracks, refTracks)
	if refTracks > 0 {
		names := make(map[string]string, len(s.plan.Seeds))
		for _, sd := range s.plan.Seeds {
			if k := normKey(sd.Artist); k != "" {
				names[k] = sd.Artist
			}
		}
		refs := make([]count, 0, len(refCounts))
		for a, c := range refCounts {
			name := a
			if n, ok := names[a]; ok {
				name = n
			}
			refs = append(refs, count{name: name, count: c})
		}
		sort.Slice(refs, func(i, j int) bool { return less(refs[i], refs[j]) })
		refPart := make([]string, 0, len(refs))
		for _, r := range refs {
			refPart = append(refPart, fmt.Sprintf("%s %d", r.name, r.count))
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(refPart, ", "))
	}
	b.WriteByte('.')
	return b.String()
}

func (s *Session) ClosePortion() []Entry {
	out := s.portion
	for _, e := range out {
		s.given[songKey(e.Song)] = true
	}
	s.recent = append(s.recent, out...)
	s.lastClosed = len(out)
	s.portion = nil
	s.inPortion = make(map[string]bool)
	return out
}

// Give counts songs as served without putting them in a portion.
func (s *Session) Give(songs []Song) {
	for _, so := range songs {
		s.given[songKey(so)] = true
	}
}

func (s *Session) ResetPortion() {
	s.portion = nil
	s.inPortion = make(map[string]bool)
}

func (s *Session) DropLast() {
	if s.lastClosed == 0 || len(s.recent) < s.lastClosed {
		return
	}
	for _, e := range s.recent[len(s.recent)-s.lastClosed:] {
		delete(s.given, songKey(e.Song))
	}
	s.recent = s.recent[:len(s.recent)-s.lastClosed]
	s.lastClosed = 0
}
