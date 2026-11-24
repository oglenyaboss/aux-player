package vibeai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

type Outcome struct {
	ID, Artist, Title string
	Kind              string // "played" | "skip_early" | "skip_mid" | "more" | "less" | "rate_clear"
	PosSec, DurSec    int
}

// RatedTrack is the current standing of one track across all sessions: Kind is
// "more" or "less", Time is the moment of the latest rating.
type RatedTrack struct {
	ID, Artist, Title, Kind string
	Time                    time.Time
}

type signalRec struct {
	Time    time.Time `json:"t"`
	Session string    `json:"session"`
	Query   string    `json:"query"`
	Kind    string    `json:"kind"` // "played" | "skip_early" | "skip_mid" | "more" | "less" | "rate_clear"
	ID      string    `json:"id"`
	Artist  string    `json:"artist"`
	Title   string    `json:"title"`
	PosSec  int       `json:"pos,omitempty"`
	DurSec  int       `json:"dur,omitempty"`
	Source  string    `json:"source,omitempty"` // "manual": played by the listener's own hand, never an agent outcome
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
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is the user's own signals file
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
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

// Rated returns every track whose latest rating is "more" or "less", newest first;
// rate_clear removes the track, other kinds are not ratings.
func (s *Signals) Rated() []RatedTrack {
	byKey := make(map[string]RatedTrack)
	for _, r := range s.recs {
		if r.manual() {
			continue
		}
		if r.Kind != "more" && r.Kind != "less" && r.Kind != "rate_clear" {
			continue
		}
		k := ratedKey(r.Artist, r.Title, r.ID)
		if r.Kind == "rate_clear" {
			delete(byKey, k)
			continue
		}
		byKey[k] = RatedTrack{ID: r.ID, Artist: r.Artist, Title: r.Title, Kind: r.Kind, Time: r.Time}
	}
	out := make([]RatedTrack, 0, len(byKey))
	for _, rt := range byKey {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.Time.Equal(b.Time) {
			return a.Time.After(b.Time)
		}
		if a.Artist != b.Artist {
			return a.Artist < b.Artist
		}
		return a.Title < b.Title
	})
	return out
}

// SetRating rewrites the rating of a track from outside a listening session: every
// earlier rating of that track in any session is cleared first (so skip counts drop
// it too), and a fresh one lands under the "settings" session.
func (s *Signals) SetRating(id, artist, title, kind string) error {
	switch kind {
	case "more", "less", "rate_clear":
	default:
		return fmt.Errorf("unknown rating kind: %q", kind)
	}
	key := songKey(Song{Artist: artist, Title: title})
	byName := strings.TrimSpace(artist) != "" || strings.TrimSpace(title) != ""
	seen := make(map[string]bool)
	now := s.now()
	var toAppend []signalRec
	for _, r := range s.recs {
		pair := r.Session + "\x00" + r.ID
		if seen[pair] {
			continue
		}
		matches := byName && songKey(Song{Artist: r.Artist, Title: r.Title}) == key || id != "" && r.ID == id
		if !matches {
			continue
		}
		seen[pair] = true
		if s.lastRating(r.Session, r.ID) == "" {
			continue
		}
		toAppend = append(toAppend, signalRec{
			Time: now, Session: r.Session, Query: "settings", Kind: "rate_clear",
			ID: r.ID, Artist: artist, Title: title,
		})
	}
	if kind != "rate_clear" {
		toAppend = append(toAppend, signalRec{
			Time: now, Session: "settings", Query: "settings", Kind: kind,
			ID: id, Artist: artist, Title: title,
		})
	}
	if len(toAppend) == 0 {
		return nil
	}
	return s.append(toAppend...)
}

func ratedKey(artist, title, id string) string {
	if strings.TrimSpace(artist) == "" && strings.TrimSpace(title) == "" {
		return id
	}
	return songKey(Song{Artist: artist, Title: title})
}

// manual marks a neutral line for a track the listener played by hand: it
// never counts as an outcome, a skip or a rating.
func (r signalRec) manual() bool { return r.Source == "manual" }

func (s *Signals) append(recs ...signalRec) error {
	s.recs = append(s.recs, recs...)
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
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
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(b.String())
	return err
}

// AppendManualPlay appends one neutral "played" line for a track the listener
// played by hand, outside any agent session: the file keeps the same mode and
// layout as agent lines, the readers keep ignoring it. An empty path drops
// the line.
func AppendManualPlay(path string, out Outcome, now time.Time) error {
	if path == "" {
		return nil
	}
	return (&Signals{path: path}).append(signalRec{
		Time: now, Kind: "played", Source: "manual",
		ID: out.ID, Artist: out.Artist, Title: out.Title,
		PosSec: out.PosSec, DurSec: out.DurSec,
	})
}

func (s *Signals) skippedElsewhere(session string) []Song {
	cutoff := s.now().Add(-14 * 24 * time.Hour)
	played := make(map[string]bool)
	for _, r := range s.recs {
		if r.manual() {
			continue
		}
		if r.Kind == "played" {
			played[songKey(Song{Artist: r.Artist, Title: r.Title})] = true
		}
	}
	seen := make(map[string]bool)
	out := make([]Song, 0, 8)
	for i := len(s.recs) - 1; i >= 0 && len(out) < 100; i-- {
		r := s.recs[i]
		if r.manual() {
			continue
		}
		if r.Kind != "skip_early" && r.Kind != "skip_mid" || r.Session == session || r.Time.Before(cutoff) {
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
		if r.manual() {
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
		if r.manual() {
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
		if r.manual() {
			continue
		}
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
		if r.manual() {
			continue
		}
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
		if r.manual() {
			continue
		}
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

// trackStats is the listener's standing for one track across all sessions:
// the latest rating (rate_clear resets it to none), the play count with the
// moment of the latest play, and the kinds of the last outcomes in file order.
// Records with any source count, manual plays included.
func (s *Signals) trackStats(song Song) (rating string, plays int, last time.Time, outcomes []string) {
	key := songKey(song)
	for _, r := range s.recs {
		if songKey(Song{Artist: r.Artist, Title: r.Title}) != key {
			continue
		}
		switch r.Kind {
		case "more", "less":
			rating = r.Kind
		case "rate_clear":
			rating = ""
		case "played":
			plays++
			if r.Time.After(last) {
				last = r.Time
			}
		}
		if r.Kind == "played" || r.Kind == "skip_early" || r.Kind == "skip_mid" {
			outcomes = append(outcomes, r.Kind)
		}
	}
	if len(outcomes) > 3 {
		outcomes = outcomes[len(outcomes)-3:]
	}
	return rating, plays, last, outcomes
}

func recHasArtist(r signalRec, artist string) bool {
	return slices.Contains(songArtists(Song{Artist: r.Artist, Title: r.Title}), artist)
}
