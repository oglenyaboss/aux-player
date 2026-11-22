package vibeai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

func testSignals(t *testing.T, path string) *Signals {
	t.Helper()
	sig, err := OpenSignals(path)
	if err != nil {
		t.Fatal(err)
	}
	sig.now = func() time.Time { return testNow }
	return sig
}

func addSignals(t *testing.T, sig *Signals, recs ...signalRec) {
	t.Helper()
	if err := sig.append(recs...); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSignalsEmptyPath(t *testing.T) {
	dir := t.TempDir()
	sig, err := OpenSignals("")
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if len(sig.recs) != 0 {
		t.Fatalf("recs = %d, want 0", len(sig.recs))
	}
	addSignals(t, sig, signalRec{Kind: "served", Session: "s1", Artist: "A", Title: "T", Time: testNow})
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("an empty path wrote files to disk: %v", entries)
	}
}

func TestOpenSignalsMissingFileAndAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x", "s.jsonl")
	sig, err := OpenSignals(path)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if len(sig.recs) != 0 {
		t.Fatalf("recs = %d, want 0", len(sig.recs))
	}
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "served", Session: "s1", Artist: "A", Title: "T"},
		signalRec{Time: testNow, Kind: "played", Session: "s1", Artist: "B", Title: "U"},
	)
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file the test just wrote under its temp dir
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("file: %d lines, want 2", len(lines))
	}
	sig2, err := OpenSignals(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig2.recs) != 2 {
		t.Errorf("reopening: recs = %d, want 2", len(sig2.recs))
	}
}

func TestOpenSignalsSkipsBrokenLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	content := "{\"kind\":\"served\",\"artist\":\"A\",\"title\":\"T\",\"session\":\"s1\"}\nмусор\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sig, err := OpenSignals(path)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if len(sig.recs) != 1 {
		t.Fatalf("recs = %d, want 1", len(sig.recs))
	}
	r := sig.recs[0]
	if r.Kind != "served" || r.Artist != "A" || r.Title != "T" || r.Session != "s1" {
		t.Errorf("rec = %+v", r)
	}
}

func TestSkippedBeforeOtherSession(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"})
	if !sig.skippedBefore(Song{Artist: "KEN CARSON", Title: "Yale"}, "s2") {
		t.Error("skip in another session: want true")
	}
	if sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Yale"}, "s1") {
		t.Error("skip in the same session: want false")
	}
}

func TestSkippedBeforeOldRecordsIgnored(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow.Add(-15 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: testNow.Add(-13 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Overseas"},
	)
	if sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Yale"}, "s2") {
		t.Error("skip 15 days ago: want false")
	}
	if !sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Overseas"}, "s2") {
		t.Error("skip 13 days ago: want true")
	}
}

func TestSkippedBeforePlayedCancels(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: testNow, Kind: "played", Session: "s1", PosSec: 10, DurSec: 190, Artist: "Ken Carson", Title: "Yale"},
	)
	if sig.skippedBefore(Song{Artist: "KEN CARSON", Title: "Yale"}, "s2") {
		t.Error("played lifts the block: want false")
	}
}

func TestBlockedArtistSessionSkip(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Mercury: Retrograde"})
	key := songArtists(Song{Artist: "Ghostemane", Title: "Mercury: Retrograde"})[0]
	if got := sig.blockedArtist(key, "s2"); got != "skipped in this session" {
		t.Errorf("the same session: got %q, want \"skipped in this session\"", got)
	}
	if got := sig.blockedArtist(key, "s3"); got != "" {
		t.Errorf("another session: got %q, want \"\"", got)
	}
}

func TestBlockedArtistGlobalTwoSessions(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "the listener skips (2 sessions)" {
		t.Errorf("got %q, want \"the listener skips (2 sessions)\"", got)
	}
}

func TestBlockedArtistGlobalOldRecordsIgnored(t *testing.T) {
	sig := testSignals(t, "")
	old := testNow.Add(-61 * 24 * time.Hour)
	addSignals(t, sig,
		signalRec{Time: old, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: old, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "" {
		t.Errorf("records older than 60 days: got %q, want \"\"", got)
	}
}

func TestBlockedArtistGlobalPlayedCancels(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
		signalRec{Time: testNow, Kind: "played", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "played", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "" {
		t.Errorf("played 2 is not < early 2: got %q, want \"\"", got)
	}
}

func TestBlockedArtistGlobalOneSession(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "" {
		t.Errorf("two skips from one session: got %q, want \"\"", got)
	}
}

func TestGlobalSkipsOrderAndSingleSession(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "g1", Artist: "Scarlxrd", Title: "T1"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g2", Artist: "Scarlxrd", Title: "T2"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g1", Artist: "Ghostemane", Title: "T3"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g2", Artist: "Ghostemane", Title: "T4"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g3", Artist: "Ghostemane", Title: "T5"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g1", Artist: "Solo", Title: "T6"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g1", Artist: "Solo", Title: "T7"},
	)
	stats := sig.globalSkips()
	if len(stats) != 2 {
		t.Fatalf("stats = %+v, want 2 records", stats)
	}
	ghost := songArtists(Song{Artist: "Ghostemane", Title: "T3"})[0]
	scar := songArtists(Song{Artist: "Scarlxrd", Title: "T1"})[0]
	if stats[0].Artist != ghost || stats[0].Early != 3 || stats[0].Sessions != 3 {
		t.Errorf("stats[0] = %+v, want %s Early=3 Sessions=3", stats[0], ghost)
	}
	if stats[1].Artist != scar || stats[1].Early != 2 || stats[1].Sessions != 2 {
		t.Errorf("stats[1] = %+v, want %s Early=2 Sessions=2", stats[1], scar)
	}
}

func TestSkippedElsewhere(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow.Add(-20 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Old", Title: "Song"},
		signalRec{Time: testNow.Add(-2 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: testNow.Add(-24 * time.Hour), Kind: "skip_mid", Session: "s2", Artist: "Self", Title: "Own"},
		signalRec{Time: testNow.Add(-30 * time.Minute), Kind: "skip_mid", Session: "s1", Artist: "Played", Title: "Song"},
		signalRec{Time: testNow.Add(-30 * time.Minute), Kind: "played", Session: "s3", Artist: "Played", Title: "Song"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "SoFaygo", Title: "Knock Knock"},
	)
	got := sig.skippedElsewhere("s2")
	want := []Song{{Artist: "SoFaygo", Title: "Knock Knock"}, {Artist: "Ken Carson", Title: "Yale"}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
		if !sig.skippedBefore(w, "s2") {
			t.Errorf("skippedBefore(%+v) = false, want true", w)
		}
	}
}

func TestLastOutcomeLatestWins(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_mid", Session: "s1", ID: "1", PosSec: 50},
		signalRec{Time: testNow, Kind: "played", Session: "s1", ID: "1"},
	)
	r, ok := sig.lastOutcome("s1", "1")
	if !ok || r.Kind != "played" {
		t.Errorf("got (%+v, %v), want the last one — played", r, ok)
	}
	if _, ok := sig.lastOutcome("s2", "1"); ok {
		t.Error("another session: want false")
	}
}

func TestEngineRecord(t *testing.T) {
	sig := testSignals(t, filepath.Join(t.TempDir(), "sig.jsonl"))
	e := &Engine{sig: sig, log: &runLog{}, sessionID: "s1", query: "рейдж"}
	err := e.Record([]Outcome{
		{ID: "1", Artist: "Ken Carson", Title: "Yale", Kind: "played", PosSec: 30, DurSec: 120},
		{ID: "2", Artist: "X", Title: "Y", Kind: "pause", PosSec: 1, DurSec: 9},
		{ID: "3", Artist: "Ghostemane", Title: "Mercury", Kind: "skip_early", PosSec: 5, DurSec: 100},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(sig.recs) != 2 {
		t.Fatalf("recs = %d, want 2", len(sig.recs))
	}
	r := sig.recs[0]
	if r.Kind != "played" || r.Session != "s1" || r.Query != "рейдж" || r.ID != "1" ||
		r.Artist != "Ken Carson" || r.Title != "Yale" || r.PosSec != 30 || r.DurSec != 120 {
		t.Errorf("rec0 = %+v", r)
	}
	if sig.recs[1].Kind != "skip_early" {
		t.Errorf("rec1 = %+v", sig.recs[1])
	}
	logText := e.log.b.String()
	if !strings.Contains(logText, "signal: Ken Carson — Yale: played 30/120 s") {
		t.Errorf("no signal line in the log: %q", logText)
	}
	if strings.Contains(logText, "pause") {
		t.Errorf("the log contains a skipped kind: %q", logText)
	}
}

func TestLastRating(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "more", Session: "s1", ID: "1"},
		signalRec{Time: testNow, Kind: "rate_clear", Session: "s1", ID: "1"},
	)
	if got := sig.lastRating("s1", "1"); got != "" {
		t.Errorf("more→rate_clear: %q, want empty", got)
	}
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "more", Session: "s1", ID: "2"},
		signalRec{Time: testNow, Kind: "less", Session: "s1", ID: "2"},
	)
	if got := sig.lastRating("s1", "2"); got != "less" {
		t.Errorf("more→less: %q, want less", got)
	}
	if got := sig.lastRating("s2", "2"); got != "" {
		t.Errorf("other session: %q, want empty", got)
	}
	addSignals(t, sig, signalRec{Time: testNow, Kind: "played", Session: "s1", ID: "3"})
	if got := sig.lastRating("s1", "3"); got != "" {
		t.Errorf("played only: %q, want empty", got)
	}
}

func TestSkipCountsLess(t *testing.T) {
	sig := testSignals(t, "")
	a := songArtists(Song{Artist: "A", Title: "T1"})[0]
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "less", Session: "s1", Query: "q1", ID: "1", Artist: "A", Title: "T1"},
		signalRec{Time: testNow, Kind: "less", Session: "s2", Query: "q2", ID: "2", Artist: "A", Title: "T2"},
	)
	early, played, sessions, _ := sig.skipCounts(a)
	if early != 2 || played != 0 || sessions != 2 {
		t.Fatalf("skipCounts = (%d, %d, %d), want (2, 0, 2)", early, played, sessions)
	}
	if sig.blockedArtist(a, "s3") == "" {
		t.Fatal("two sessions of less must block the artist")
	}
	addSignals(t, sig, signalRec{Time: testNow, Kind: "rate_clear", Session: "s2", ID: "2", Artist: "A", Title: "T2"})
	early, played, sessions, _ = sig.skipCounts(a)
	if early != 1 || played != 0 || sessions != 1 {
		t.Fatalf("after rate_clear: (%d, %d, %d), want (1, 0, 1)", early, played, sessions)
	}
	if got := sig.blockedArtist(a, "s3"); got != "" {
		t.Errorf("blockedArtist = %q, want empty after the clear", got)
	}
}

func TestSkipCountsLessThenMore(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "less", Session: "s1", ID: "1", Artist: "A", Title: "T1"},
		signalRec{Time: testNow, Kind: "more", Session: "s1", ID: "1", Artist: "A", Title: "T1"},
	)
	early, played, sessions, _ := sig.skipCounts(songArtists(Song{Artist: "A", Title: "T1"})[0])
	if early != 0 || played != 1 || sessions != 0 {
		t.Fatalf("skipCounts = (%d, %d, %d), want (0, 1, 0)", early, played, sessions)
	}
}

func TestBlockedArtistIgnoresLessInSession(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "less", Session: "s1", ID: "1", Artist: "A", Title: "T1"})
	if got := sig.blockedArtist(songArtists(Song{Artist: "A", Title: "T1"})[0], "s1"); got != "" {
		t.Errorf("blockedArtist = %q, want empty: less must not block within the session", got)
	}
}

func TestGlobalSkipsFromLess(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "less", Session: "g1", ID: "1", Artist: "Solo", Title: "T1"},
		signalRec{Time: testNow, Kind: "less", Session: "g2", ID: "2", Artist: "Solo", Title: "T2"},
	)
	stats := sig.globalSkips()
	solo := songArtists(Song{Artist: "Solo", Title: "T1"})[0]
	if len(stats) != 1 || stats[0].Artist != solo || stats[0].Early != 2 || stats[0].Sessions != 2 {
		t.Fatalf("stats = %+v, want Solo Early=2 Sessions=2", stats)
	}
}

func TestEngineRecordRatings(t *testing.T) {
	sig := testSignals(t, filepath.Join(t.TempDir(), "sig.jsonl"))
	e := &Engine{sig: sig, log: &runLog{}, sessionID: "s1", query: "рейдж"}
	err := e.Record([]Outcome{
		{ID: "1", Artist: "A", Title: "T1", Kind: "more"},
		{ID: "1", Artist: "A", Title: "T1", Kind: "less"},
		{ID: "1", Artist: "A", Title: "T1", Kind: "rate_clear"},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	got := make([]string, len(sig.recs))
	for i, r := range sig.recs {
		got[i] = r.Kind
	}
	if strings.Join(got, ",") != "more,less,rate_clear" {
		t.Fatalf("recs = %v, want [more less rate_clear]", got)
	}
}

func TestSkippedBeforeNeedsSkip(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "served", Session: "s1", Artist: "Ken Carson", Title: "Yale"})
	if sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Yale"}, "s2") {
		t.Error("the track was only in the portion, it was never reached: want false")
	}
	if got := sig.skippedElsewhere("s2"); len(got) != 0 {
		t.Errorf("skippedElsewhere = %+v, want empty", got)
	}
}

func TestRated(t *testing.T) {
	sig := testSignals(t, "")
	t1 := testNow.Add(-3 * time.Hour)
	t2 := testNow.Add(-2 * time.Hour)
	t3 := testNow.Add(-1 * time.Hour)
	addSignals(t, sig,
		signalRec{Time: t1, Kind: "more", Session: "s1", ID: "1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: t1, Kind: "more", Session: "s1", ID: "3", Artist: "SoFaygo", Title: "Knock Knock"},
		signalRec{Time: t2, Kind: "more", Session: "s3", ID: "4", Artist: "C", Title: "Cleared"},
		signalRec{Time: t3, Kind: "less", Session: "s2", ID: "2", Artist: "KEN CARSON", Title: "Yale"},
		signalRec{Time: t3, Kind: "rate_clear", Session: "s3", ID: "4", Artist: "C", Title: "Cleared"},
		signalRec{Time: t3, Kind: "played", Session: "s1", ID: "5", Artist: "D", Title: "Played"},
		signalRec{Time: t3, Kind: "skip_early", Session: "s1", ID: "6", Artist: "E", Title: "Skipped"},
	)
	want := []RatedTrack{
		{ID: "2", Artist: "KEN CARSON", Title: "Yale", Kind: "less", Time: t3},
		{ID: "3", Artist: "SoFaygo", Title: "Knock Knock", Kind: "more", Time: t1},
	}
	got := sig.Rated()
	if len(got) != len(want) {
		t.Fatalf("Rated = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Rated[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestRatedTieBreak(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "less", Session: "s1", ID: "1", Artist: "B", Title: "T2"},
		signalRec{Time: testNow, Kind: "more", Session: "s2", ID: "2", Artist: "A", Title: "T2"},
		signalRec{Time: testNow, Kind: "more", Session: "s3", ID: "3", Artist: "A", Title: "T1"},
	)
	got := sig.Rated()
	want := []struct{ artist, title, kind string }{
		{"A", "T1", "more"}, {"A", "T2", "more"}, {"B", "T2", "less"},
	}
	if len(got) != len(want) {
		t.Fatalf("Rated = %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].Artist != w.artist || got[i].Title != w.title || got[i].Kind != w.kind {
			t.Errorf("Rated[%d] = %+v, want %v", i, got[i], w)
		}
	}
}

func TestSetRatingClearsAcrossSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sig.jsonl")
	sig := testSignals(t, path)
	a := songArtists(Song{Artist: "A", Title: "T1"})[0]
	addSignals(t, sig,
		signalRec{Time: testNow.Add(-2 * time.Hour), Kind: "less", Session: "s1", Query: "q1", ID: "1", Artist: "A", Title: "T1"},
		signalRec{Time: testNow.Add(-2 * time.Hour), Kind: "less", Session: "s2", Query: "q2", ID: "2", Artist: "a", Title: "T1 (feat. B)"},
	)
	if early, played, sessions, _ := sig.skipCounts(a); early != 2 || played != 0 || sessions != 2 {
		t.Fatalf("skipCounts = (%d, %d, %d), want (2, 0, 2)", early, played, sessions)
	}
	if err := sig.SetRating("1", "A", "T1", "rate_clear"); err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	if rated := sig.Rated(); len(rated) != 0 {
		t.Fatalf("Rated after clear = %+v, want empty", rated)
	}
	if early, _, sessions, _ := sig.skipCounts(a); early != 0 || sessions != 0 {
		t.Fatalf("skipCounts after clear = (%d, _, %d), want (0, _, 0)", early, sessions)
	}
	reopened, err := OpenSignals(path)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	reopened.now = func() time.Time { return testNow }
	if len(reopened.recs) != 4 {
		t.Fatalf("reopened recs = %d, want 4 (2 original + 2 rate_clear)", len(reopened.recs))
	}
	if rated := reopened.Rated(); len(rated) != 0 {
		t.Fatalf("Rated after reopen = %+v, want empty", rated)
	}
	if early, _, sessions, _ := reopened.skipCounts(a); early != 0 || sessions != 0 {
		t.Fatalf("skipCounts after reopen = (%d, _, %d), want (0, _, 0)", early, sessions)
	}
}

func TestSetRatingFlipToMore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sig.jsonl")
	sig := testSignals(t, path)
	a := songArtists(Song{Artist: "A", Title: "T1"})[0]
	addSignals(t, sig, signalRec{Time: testNow.Add(-time.Hour), Kind: "less", Session: "s1", Query: "q1", ID: "1", Artist: "A", Title: "T1"})
	if err := sig.SetRating("1", "A", "T1", "more"); err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	want := []RatedTrack{{ID: "1", Artist: "A", Title: "T1", Kind: "more", Time: testNow}}
	if rated := sig.Rated(); len(rated) != 1 || rated[0] != want[0] {
		t.Fatalf("Rated = %+v, want %+v", rated, want)
	}
	if early, played, sessions, _ := sig.skipCounts(a); early != 0 || played != 1 || sessions != 0 {
		t.Fatalf("skipCounts = (%d, %d, %d), want (0, 1, 0)", early, played, sessions)
	}
	reopened, err := OpenSignals(path)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if rated := reopened.Rated(); len(rated) != 1 || rated[0].Kind != "more" {
		t.Fatalf("Rated after reopen = %+v, want one more", rated)
	}
}

func TestSetRatingInvalidKindAndNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sig.jsonl")
	sig := testSignals(t, path)
	addSignals(t, sig, signalRec{Time: testNow, Kind: "less", Session: "s1", ID: "1", Artist: "A", Title: "T1"})
	if err := sig.SetRating("1", "A", "T1", "played"); err == nil {
		t.Fatal("SetRating with an invalid kind passed, want an error")
	}
	if len(sig.recs) != 1 {
		t.Fatalf("recs after invalid kind = %d, want 1", len(sig.recs))
	}
	if err := sig.SetRating("", "X", "Y", "rate_clear"); err != nil {
		t.Fatalf("SetRating with nothing to clear: %v", err)
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file the test just wrote under its temp dir
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n"); len(lines) != 1 {
		t.Fatalf("file after the noop clear: %d lines, want 1", len(lines))
	}
	if len(sig.recs) != 1 {
		t.Fatalf("recs after noop clear = %d, want 1", len(sig.recs))
	}
}
