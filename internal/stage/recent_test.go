package stage

import (
	"strings"
	"testing"
)

func recentTrack(id, artist, title string) Track {
	return Track{ID: id, Artist: artist, Title: title}
}

func TestSaveTrackPlaying(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("l", "l")
	m.Key("f", "f")
	saves := recorded[SaveTrack](r)
	if len(saves) != 2 {
		t.Fatalf("save actions = %v, want two", r.acts)
	}
	if saves[0].ID != "" || saves[0].Favorites {
		t.Errorf("l = %+v, want the playing track for the library", saves[0])
	}
	if saves[1].ID != "" || !saves[1].Favorites {
		t.Errorf("f = %+v, want the playing track for Favorites", saves[1])
	}
}

func TestSaveTrackTypingWins(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("/", "/")
	m.Key("l", "l")
	if saves := recorded[SaveTrack](r); len(saves) != 0 {
		t.Fatalf("typing l sent %v, want no save", saves)
	}
	if got := m.mu.list.query; got != "l" {
		t.Fatalf("query = %q, want l typed into the agent palette", got)
	}
}

func TestRecentSelectSavesRow(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	a := recentTrack("a", "A", "One")
	b := recentTrack("b", "B", "Two")
	c := recentTrack("c", "C", "Three")
	m.SetRecent([]Track{a, b, c})
	m.Key("h", "h")
	m.Key("down", "down")
	m.Key("l", "l")
	m.Key("f", "f")
	saves := recorded[SaveTrack](r)
	if len(saves) != 2 {
		t.Fatalf("save actions = %v, want two", r.acts)
	}
	if saves[0].ID != b.ID || saves[0].Favorites {
		t.Errorf("l = %+v, want library for %s", saves[0], b.ID)
	}
	if saves[1].ID != b.ID || !saves[1].Favorites {
		t.Errorf("f = %+v, want Favorites for %s", saves[1], b.ID)
	}
	if !m.recentOpen {
		t.Error("the overlay must stay open after a save")
	}
	m.Key("esc", "")
	if m.recentOpen {
		t.Error("esc must close the overlay")
	}
}

func TestRecentRowsSnapshot(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	a := recentTrack("a", "A", "One")
	b := recentTrack("b", "B", "Two")
	m.SetRecent([]Track{a, b})
	m.Key("h", "h")
	m.Key("down", "down")
	m.SetRecent([]Track{recentTrack("x", "X", "Zero"), a, b})
	m.Key("l", "l")
	saves := recorded[SaveTrack](r)
	if len(saves) != 1 || saves[0].ID != b.ID {
		t.Fatalf("save actions = %v, want the selected row %s", r.acts, b.ID)
	}
}

func TestRecentSelectionClamps(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	a := recentTrack("a", "A", "One")
	b := recentTrack("b", "B", "Two")
	m.SetRecent([]Track{a, b})
	m.Key("h", "h")
	m.Key("up", "up")
	if m.recentSel != 0 {
		t.Errorf("up from the first row: recentSel = %d, want 0", m.recentSel)
	}
	m.Key("down", "down")
	m.Key("down", "down")
	m.Key("down", "down")
	if m.recentSel != 1 {
		t.Errorf("down past the last row: recentSel = %d, want 1", m.recentSel)
	}
}

func TestRecentEmpty(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("h", "h")
	m.Key("l", "l")
	m.Key("f", "f")
	if saves := recorded[SaveTrack](r); len(saves) != 0 {
		t.Fatalf("empty overlay sent %v, want nothing", saves)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "nothing yet") {
		t.Errorf("frame without the empty line:\n%s", frame)
	}
}

func TestRecentSurvivesRestart(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	a := recentTrack("a", "Artist", "Kept Song")
	m.SetRecent([]Track{a})
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("h", "h")
	m.Key("r", "r")
	if m.recentOpen {
		t.Error("r must close the overlay")
	}
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("h", "h")
	if frame := stripANSI(m.View()); !strings.Contains(frame, a.Title) {
		t.Errorf("frame after r lacks %q:\n%s", a.Title, frame)
	}
}

func TestRecentPassesOtherKeys(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.SetRecent([]Track{recentTrack("a", "A", "One")})
	m.Key("h", "h")
	if m.Key("n", "n") {
		t.Error("n inside the overlay must fall through to the host")
	}
}

func TestRecentKeysAndFrame(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("?", "?")
	frame := stripANSI(m.View())
	for _, want := range []string{"add to Favorites", "recently played"} {
		if !strings.Contains(frame, want) {
			t.Errorf("help frame lacks %q:\n%s", want, frame)
		}
	}
	m.Key("?", "?")
	m.SetRecent([]Track{{ID: "a", Artist: "Artist", Title: "Selected Song"}})
	m.Key("h", "h")
	frame = stripANSI(m.View())
	for _, want := range []string{"Selected Song", "esc close"} {
		if !strings.Contains(frame, want) {
			t.Errorf("overlay frame lacks %q:\n%s", want, frame)
		}
	}
	m.Key("esc", "")
	m.SetRecent([]Track{{ID: "long", Artist: "Artist", Title: strings.Repeat("X", 200)}})
	m.Key("h", "h")
	for i, line := range strings.Split(stripANSI(m.View()), "\n") {
		if n := len([]rune(line)); n > 110 {
			t.Errorf("line %d is %d cells wide, want at most 110", i, n)
		}
	}
}

func TestRecentAndHelpExclusive(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("?", "?")
	if !m.help || m.recentOpen {
		t.Fatalf("after ?: help = %v, recentOpen = %v, want help only", m.help, m.recentOpen)
	}
	m.Key("h", "h")
	if m.help || !m.recentOpen {
		t.Fatalf("after h: help = %v, recentOpen = %v, want the list only", m.help, m.recentOpen)
	}
	m.SetRecent([]Track{recentTrack("a", "A", "One")})
	m.Key("?", "?")
	if m.recentOpen || !m.help {
		t.Fatalf("after ? in the list: help = %v, recentOpen = %v, want help only", m.help, m.recentOpen)
	}
	m.Key("l", "l")
	m.Key("f", "f")
	saves := recorded[SaveTrack](r)
	if len(saves) != 2 {
		t.Fatalf("save actions = %v, want two", r.acts)
	}
	for _, s := range saves {
		if s.ID != "" {
			t.Errorf("save = %+v, want the playing track, not a recent row", s)
		}
	}
	if !saves[1].Favorites {
		t.Errorf("f = %+v, want Favorites", saves[1])
	}
}

func toastCount(m *Model, text string) int {
	n := 0
	for _, t := range m.toasts {
		if t.text == text {
			n++
		}
	}
	return n
}

// F3/R1: a host line said outside the player waits for the next play and is not repeated.
func TestToastHeldUntilPlay(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Toast("held line")
	if n := toastCount(m, "held line"); n != 0 {
		t.Fatalf("toasts before the player = %d, want none", n)
	}
	m.JumpPlay(threeTracks(), 0, 0)
	if n := toastCount(m, "held line"); n != 1 {
		t.Fatalf("toasts in the player = %d, want one", n)
	}
	m.JumpPlay(threeTracks(), 1, 0)
	if n := toastCount(m, "held line"); n != 1 {
		t.Fatalf("re-entering the player repeated the line: %d toasts", n)
	}
	steps(m, 6)
	m.JumpPlay(threeTracks(), 2, 0)
	if n := toastCount(m, "held line"); n != 0 {
		t.Fatalf("an old line came back after it timed out: %d toasts", n)
	}
}

// F3/R1: a line said after r is not lost with the reset.
func TestToastHeldAcrossReset(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("r", "r")
	m.Toast("after reset")
	if n := toastCount(m, "after reset"); n != 0 {
		t.Fatalf("toasts outside the player = %d, want none", n)
	}
	m.JumpPlay(threeTracks(), 0, 0)
	if n := toastCount(m, "after reset"); n != 1 {
		t.Fatalf("toasts in the player = %d, want one", n)
	}
}
