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
		t.Errorf("пустой path записал файлы на диск: %v", entries)
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("файл: %d строк, want 2", len(lines))
	}
	sig2, err := OpenSignals(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig2.recs) != 2 {
		t.Errorf("повторное открытие: recs = %d, want 2", len(sig2.recs))
	}
}

func TestOpenSignalsSkipsBrokenLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	content := "{\"kind\":\"served\",\"artist\":\"A\",\"title\":\"T\",\"session\":\"s1\"}\nмусор\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
		t.Error("skip в другой сессии: want true")
	}
	if sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Yale"}, "s1") {
		t.Error("skip в той же сессии: want false")
	}
}

func TestSkippedBeforeOldRecordsIgnored(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow.Add(-15 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: testNow.Add(-13 * 24 * time.Hour), Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Overseas"},
	)
	if sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Yale"}, "s2") {
		t.Error("skip 15 дней назад: want false")
	}
	if !sig.skippedBefore(Song{Artist: "Ken Carson", Title: "Overseas"}, "s2") {
		t.Error("skip 13 дней назад: want true")
	}
}

func TestSkippedBeforePlayedCancels(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"},
		signalRec{Time: testNow, Kind: "played", Session: "s1", PosSec: 10, DurSec: 190, Artist: "Ken Carson", Title: "Yale"},
	)
	if sig.skippedBefore(Song{Artist: "KEN CARSON", Title: "Yale"}, "s2") {
		t.Error("played снимает запрет: want false")
	}
}

func TestBlockedArtistSessionSkip(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Mercury: Retrograde"})
	key := songArtists(Song{Artist: "Ghostemane", Title: "Mercury: Retrograde"})[0]
	if got := sig.blockedArtist(key, "s2"); got != "skipped in this session" {
		t.Errorf("ta же сессия: got %q, want \"скипнут в этой сессии\"", got)
	}
	if got := sig.blockedArtist(key, "s3"); got != "" {
		t.Errorf("другая сессия: got %q, want \"\"", got)
	}
}

func TestBlockedArtistGlobalTwoSessions(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "the listener skips (2 sessions)" {
		t.Errorf("got %q, want \"владелец скипает (2 сессий)\"", got)
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
		t.Errorf("записи старше 60 дней: got %q, want \"\"", got)
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
		t.Errorf("played 2 не < early 2: got %q, want \"\"", got)
	}
}

func TestBlockedArtistGlobalOneSession(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Rapture"},
	)
	if got := sig.blockedArtist("ghostemane", "s3"); got != "" {
		t.Errorf("два скипа из одной сессии: got %q, want \"\"", got)
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
		t.Fatalf("stats = %+v, want 2 записи", stats)
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
		t.Errorf("got (%+v, %v), want последнюю — played", r, ok)
	}
	if _, ok := sig.lastOutcome("s2", "1"); ok {
		t.Error("другая сессия: want false")
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
		t.Errorf("нет строки сигнала в логе: %q", logText)
	}
	if strings.Contains(logText, "pause") {
		t.Errorf("лог содержит пропущенный kind: %q", logText)
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
		t.Error("трек только стоял в порции, до него не дошли: want false")
	}
	if got := sig.skippedElsewhere("s2"); len(got) != 0 {
		t.Errorf("skippedElsewhere = %+v, want пусто", got)
	}
}
