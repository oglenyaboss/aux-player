package vibeai

import (
	"math"
	"strings"
)

const windowSize = 20

type Seed struct {
	Artist string
	Mode   string // "not_it" | "and_similar" | "want_it"
}

type Plan struct {
	Seeds         []Seed
	Exclusions    []string // artist names
	FromFavorites bool
}

type Entry struct {
	Song     Song
	Why      string
	Favorite bool // the track is from the owner's Favorites
	Known    bool // at least one artist of the song is known

	knownCounted bool
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
	lib, favs       *Library
	known           map[string]bool
	plan            Plan
	excl            map[string]string
	seeds           map[string]string
	given           map[string]bool
	recent          []Entry
	lastClosed      int
	portion         []Entry
	inPortion       map[string]bool
	portionArts     map[string]int
	reserve         []reserved
	knownCount      int
	sig             *Signals
	sessionID       string
	Size, PerArtist int
}

func NewSession(library, favorites []Song, size int) *Session {
	return &Session{
		lib:         NewLibrary(library),
		favs:        NewLibrary(favorites),
		known:       KnownArtists(favorites),
		given:       make(map[string]bool),
		inPortion:   make(map[string]bool),
		portionArts: make(map[string]int),
		Size:        size,
		PerArtist:   2,
	}
}

func (s *Session) SetPlan(p Plan) {
	s.plan = p
	s.excl = make(map[string]string, len(p.Exclusions))
	for _, name := range p.Exclusions {
		if k := normKey(name); k != "" {
			s.excl[k] = name
		}
	}
	s.seeds = make(map[string]string, len(p.Seeds))
	for _, sd := range p.Seeds {
		if k := normKey(sd.Artist); k != "" {
			s.seeds[k] = sd.Mode
		}
	}
	s.recountKnown()
}

func (s *Session) recountKnown() {
	s.knownCount = 0
	for i := range s.portion {
		counted := false
		for _, a := range songArtists(s.portion[i].Song) {
			if !s.known[a] {
				continue
			}
			if _, isSeed := s.seeds[a]; !isSeed {
				counted = true
				break
			}
		}
		s.portion[i].knownCounted = counted
		if counted {
			s.knownCount++
		}
	}
}

func (s *Session) SetSignals(sig *Signals, sessionID string) {
	s.sig = sig
	s.sessionID = sessionID
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
	arts := songArtists(song)
	for _, a := range arts {
		if name, ok := s.excl[a]; ok {
			v.Reason = "exclusion: " + name
			return v
		}
	}
	for _, a := range arts {
		if s.seeds[a] == "not_it" {
			v.Reason = "the referent itself (not_it)"
			return v
		}
	}
	k := songKey(song)
	if s.given[k] || s.inPortion[k] {
		v.Reason = "already served"
		return v
	}
	if s.Need() == 0 {
		v.Reason = "portion is full"
		s.addReserve(sug, song)
		return v
	}
	windowArts, windowKnown := s.windowCounts()
	if s.sig != nil {
		if s.sig.skippedBefore(song, s.sessionID) {
			v.Reason = "dropped early before"
			return v
		}
		for _, a := range arts {
			if r := s.sig.blockedArtist(a, s.sessionID); r != "" {
				if strings.HasPrefix(r, "the listener skips") && (s.seeds[a] == "want_it" || s.seeds[a] == "and_similar") {
					continue
				}
				v.Reason = r
				return v
			}
		}
	}
	if s.plan.FromFavorites {
		if !s.lib.Has(song) && !s.favs.Has(song) {
			v.Reason = "favorites mode: not in the library"
			return v
		}
		favArtist := false
		for _, a := range arts {
			if s.known[a] {
				favArtist = true
				break
			}
		}
		if !favArtist {
			v.Reason = "favorites mode: artist is not from Favorites"
			return v
		}
	} else if s.lib.Has(song) && !s.favs.Has(song) && !s.hasSeedArtist(arts) {
		v.Reason = "known track (library)"
		return v
	}
	for _, a := range arts {
		if s.seeds[a] == "want_it" {
			continue
		}
		if s.portionArts[a]+windowArts[a] >= s.PerArtist {
			v.Reason = "artist limit: " + a
			return v
		}
	}
	maxSimilar := int(math.Ceil(float64(windowSize)*0.15 - 1e-9))
	for _, a := range arts {
		if s.seeds[a] == "and_similar" && s.portionArts[a]+windowArts[a] >= maxSimilar {
			v.Reason = "referent limit"
			return v
		}
	}
	isKnown := false
	knownCounted := false
	for _, a := range arts {
		if !s.known[a] {
			continue
		}
		isKnown = true
		if _, isSeed := s.seeds[a]; !isSeed {
			knownCounted = true
		}
	}
	if !s.plan.FromFavorites && knownCounted && s.knownCount+windowKnown >= s.maxKnown() {
		v.Reason = "known limit"
		return v
	}
	s.portion = append(s.portion, Entry{Song: song, Why: sug.Why, Favorite: s.favs.Has(song), Known: isKnown, knownCounted: knownCounted})
	s.inPortion[k] = true
	for _, a := range arts {
		s.portionArts[a]++
	}
	if knownCounted {
		s.knownCount++
	}
	v.Accepted = true
	return v
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
	arts := songArtists(song)
	isKnown := false
	knownCounted := false
	for _, a := range arts {
		if !s.known[a] {
			continue
		}
		isKnown = true
		if _, isSeed := s.seeds[a]; !isSeed {
			knownCounted = true
		}
	}
	s.portion = append(s.portion, Entry{Song: song, Why: sug.Why, Favorite: s.favs.Has(song), Known: isKnown, knownCounted: knownCounted})
	s.inPortion[k] = true
	for _, a := range arts {
		s.portionArts[a]++
	}
	if knownCounted {
		s.knownCount++
	}
	v.Accepted = true
	return v
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
	return s.Need() <= s.Size/10
}

func (s *Session) hasSeedArtist(arts []string) bool {
	for _, a := range arts {
		if m := s.seeds[a]; m == "want_it" || m == "and_similar" {
			return true
		}
	}
	return false
}

func (s *Session) maxKnown() int {
	return int(math.Floor(float64(windowSize)*0.2 + 1e-9))
}

func (s *Session) KnownLeft() int {
	if s.plan.FromFavorites {
		return s.Need()
	}
	_, tailKnown := s.windowCounts()
	return max(s.maxKnown()-s.knownCount-tailKnown, 0)
}

func (s *Session) windowTail() []Entry {
	n := windowSize - s.Size
	if n <= 0 {
		return nil
	}
	if n >= len(s.recent) {
		return s.recent
	}
	return s.recent[len(s.recent)-n:]
}

func (s *Session) windowCounts() (map[string]int, int) {
	tailArts := make(map[string]int)
	tailKnown := 0
	for _, e := range s.windowTail() {
		for _, a := range songArtists(e.Song) {
			tailArts[a]++
		}
		if e.knownCounted {
			tailKnown++
		}
	}
	return tailArts, tailKnown
}

func (s *Session) Portion() []Entry {
	return s.portion
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
	s.portionArts = make(map[string]int)
	s.knownCount = 0
	return out
}

func (s *Session) ResetPortion() {
	s.portion = nil
	s.inPortion = make(map[string]bool)
	s.portionArts = make(map[string]int)
	s.knownCount = 0
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
