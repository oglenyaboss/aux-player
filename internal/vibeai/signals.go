package vibeai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Outcome struct {
	ID, Artist, Title string
	Kind              string // "played" | "skip_early" | "skip_mid" | "more" | "less" | "rate_clear"
	PosSec, DurSec    int
}

type signalRec struct {
	Time    time.Time `json:"t"`
	Session string    `json:"session"`
	Query   string    `json:"query"`
	Kind    string    `json:"kind"` // "served" | "played" | "skip_early" | "skip_mid" | "more" | "less" | "rate_clear"
	ID      string    `json:"id"`
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	PosSec  int       `json:"pos,omitempty"`
	DurSec  int       `json:"dur,omitempty"`
}

type Signals struct {
	path string
	recs []signalRec
	now  func() time.Time
}

func OpenSignals(path string) (*Signals, error) {
	s := &Signals{path: path, now: time.Now}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r signalRec
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		s.recs = append(s.recs, r)
	}
	return s, nil
}

func (s *Signals) append(recs ...signalRec) error {
	s.recs = append(s.recs, recs...)
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, r := range recs {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}

func (s *Signals) servedBefore(song Song, session string) bool {
	key := songKey(song)
	cutoff := s.now().Add(-14 * 24 * time.Hour)
	served, played := false, false
	for _, r := range s.recs {
		if r.Kind != "served" && r.Kind != "played" {
			continue
		}
		if songKey(Song{Artist: r.Artist, Title: r.Title}) != key {
			continue
		}
		if r.Kind == "played" {
			played = true
		} else if r.Session != session && !r.Time.Before(cutoff) {
			served = true
		}
	}
	return served && !played
}

func (s *Signals) servedElsewhere(session string) []Song {
	cutoff := s.now().Add(-14 * 24 * time.Hour)
	played := make(map[string]bool)
	for _, r := range s.recs {
		if r.Kind == "played" {
			played[songKey(Song{Artist: r.Artist, Title: r.Title})] = true
		}
	}
	seen := make(map[string]bool)
	out := make([]Song, 0, 8)
	for i := len(s.recs) - 1; i >= 0 && len(out) < 100; i-- {
		r := s.recs[i]
		if r.Kind != "served" || r.Session == session || r.Time.Before(cutoff) {
			continue
		}
		song := Song{Artist: r.Artist, Title: r.Title}
		k := songKey(song)
		if seen[k] || played[k] {
			continue
		}
		seen[k] = true
		out = append(out, song)
	}
	return out
}

func (s *Signals) blockedArtist(artist, session string) string {
	for _, r := range s.recs {
		if r.Kind != "skip_early" || r.Session != session {
			continue
		}
		if recHasArtist(r, artist) {
			return "скипнут в этой сессии"
		}
	}
	// ponytail: окно 60 дней вместо затухания весов; апгрейд — экспоненциальный вес по давности
	early, played, sessions, _ := s.skipCounts(artist)
	if sessions >= 2 && played < early {
		return fmt.Sprintf("владелец скипает (%d сессий)", sessions)
	}
	return ""
}

type skipStat struct {
	Artist                  string
	Early, Sessions, Played int
	Queries                 []string
}

func (s *Signals) skipCounts(artist string) (early, played, sessions int, queries []string) {
	cutoff := s.now().Add(-60 * 24 * time.Hour)
	ratings := make(map[string]string)
	for _, r := range s.recs {
		if r.Time.Before(cutoff) {
			continue
		}
		switch r.Kind {
		case "more", "less", "rate_clear":
			ratings[r.Session+"\x00"+r.ID] = r.Kind
		}
	}
	seen := make(map[string]bool)
	sess := make(map[string]bool)
	counted := make(map[string]bool)
	for _, r := range s.recs {
		if r.Time.Before(cutoff) {
			continue
		}
		if r.Kind != "skip_early" && r.Kind != "played" && r.Kind != "less" && r.Kind != "more" {
			continue
		}
		if !recHasArtist(r, artist) {
			continue
		}
		if r.Kind == "less" || r.Kind == "more" {
			key := r.Session + "\x00" + r.ID
			if ratings[key] != r.Kind || counted[key] {
				continue
			}
			counted[key] = true
		}
		if r.Kind == "skip_early" || r.Kind == "less" {
			early++
			sess[r.Session] = true
			if !seen[r.Query] && len(queries) < 3 {
				seen[r.Query] = true
				queries = append(queries, r.Query)
			}
			continue
		}
		played++
	}
	sessions = len(sess)
	return early, played, sessions, queries
}

func (s *Signals) globalSkips() []skipStat {
	cands := make(map[string]bool)
	for _, r := range s.recs {
		if r.Kind != "skip_early" && r.Kind != "less" {
			continue
		}
		for _, a := range songArtists(Song{Artist: r.Artist, Title: r.Title}) {
			cands[a] = true
		}
	}
	out := make([]skipStat, 0, len(cands))
	for a := range cands {
		early, played, sessions, queries := s.skipCounts(a)
		if sessions >= 2 && played < early {
			out = append(out, skipStat{Artist: a, Early: early, Sessions: sessions, Played: played, Queries: queries})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Early != out[j].Early {
			return out[i].Early > out[j].Early
		}
		return out[i].Artist < out[j].Artist
	})
	return out
}

func (s *Signals) lastOutcome(session, id string) (signalRec, bool) {
	for i := len(s.recs) - 1; i >= 0; i-- {
		r := s.recs[i]
		if r.Session != session || r.ID != id {
			continue
		}
		switch r.Kind {
		case "played", "skip_early", "skip_mid":
			return r, true
		}
	}
	return signalRec{}, false
}

func (s *Signals) lastRating(session, id string) string {
	for i := len(s.recs) - 1; i >= 0; i-- {
		r := s.recs[i]
		if r.Session != session || r.ID != id {
			continue
		}
		switch r.Kind {
		case "more", "less":
			return r.Kind
		case "rate_clear":
			return ""
		}
	}
	return ""
}

func recHasArtist(r signalRec, artist string) bool {
	for _, a := range songArtists(Song{Artist: r.Artist, Title: r.Title}) {
		if a == artist {
			return true
		}
	}
	return false
}
