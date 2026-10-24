package vibeai

import (
	"math"
	"strings"
)

type Seed struct {
	Artist string
	Mode   string // "not_it" | "and_similar" | "want_it"
}

type Plan struct {
	Seeds      []Seed
	Exclusions []string // имена артистов
}

type Entry struct {
	Song     Song
	Why      string
	Favorite bool // трек из Favorites владельца
	Known    bool // хотя бы один артист песни — известный
}

type Verdict struct {
	Suggestion Suggestion
	Song       Song // пусто, если не найден
	Accepted   bool
	Reason     string // "" если принят
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
}

func (s *Session) SetSignals(sig *Signals, sessionID string) {
	s.sig = sig
	s.sessionID = sessionID
}

func (s *Session) Need() int {
	n := s.Size - len(s.portion)
	if n < 0 {
		n = 0
	}
	return n
}

func (s *Session) Offer(sug Suggestion, song Song, found bool) Verdict {
	v := Verdict{Suggestion: sug}
	if !found {
		v.Reason = "нет в Apple"
		return v
	}
	v.Song = song
	if s.Need() == 0 {
		v.Reason = "порция заполнена"
		s.addReserve(sug, song)
		return v
	}
	arts := songArtists(song)
	for _, a := range arts {
		if name, ok := s.excl[a]; ok {
			v.Reason = "исключение: " + name
			return v
		}
	}
	for _, a := range arts {
		if s.seeds[a] == "not_it" {
			v.Reason = "сам ориентир (не он)"
			return v
		}
	}
	k := songKey(song)
	if s.given[k] || s.inPortion[k] {
		v.Reason = "уже было"
		return v
	}
	if s.sig != nil {
		if s.sig.skippedBefore(song, s.sessionID) {
			v.Reason = "бросал раньше, не дослушав"
			return v
		}
		for _, a := range arts {
			if r := s.sig.blockedArtist(a, s.sessionID); r != "" {
				if strings.HasPrefix(r, "владелец скипает") && (s.seeds[a] == "want_it" || s.seeds[a] == "and_similar") {
					continue
				}
				v.Reason = r
				return v
			}
		}
	}
	if s.lib.Has(song) && !s.favs.Has(song) && !s.hasSeedArtist(arts) {
		v.Reason = "известный трек (библиотека)"
		return v
	}
	for _, a := range arts {
		if s.seeds[a] == "want_it" {
			continue
		}
		if s.portionArts[a] >= s.PerArtist {
			v.Reason = "лимит артиста: " + a
			return v
		}
	}
	maxSimilar := int(math.Ceil(float64(s.Size)*0.15 - 1e-9))
	for _, a := range arts {
		if s.seeds[a] == "and_similar" && s.portionArts[a] >= maxSimilar {
			v.Reason = "лимит ориентира"
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
	if knownCounted && s.knownCount >= s.maxKnown() {
		v.Reason = "лимит известных"
		return v
	}
	s.portion = append(s.portion, Entry{Song: song, Why: sug.Why, Favorite: s.favs.Has(song), Known: isKnown})
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

func (s *Session) DrainReserve() []Verdict {
	reserve := s.reserve
	s.reserve = nil
	verdicts := make([]Verdict, 0, len(reserve))
	for _, r := range reserve {
		verdicts = append(verdicts, s.Offer(r.sug, r.song, true))
	}
	return verdicts
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
	return int(math.Floor(float64(s.Size)*0.2 + 1e-9))
}

func (s *Session) KnownLeft() int {
	return max(s.maxKnown()-s.knownCount, 0)
}

func (s *Session) Portion() []Entry {
	return s.portion
}

func (s *Session) ClosePortion() []Entry {
	out := s.portion
	for _, e := range out {
		s.given[songKey(e.Song)] = true
	}
	s.portion = nil
	s.inPortion = make(map[string]bool)
	s.portionArts = make(map[string]int)
	s.knownCount = 0
	return out
}
