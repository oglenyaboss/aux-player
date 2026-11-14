package stage

import (
	"fmt"
	"strings"
	"testing"
)

// paletteSetup is a model in the given look, in English, at the given size.
func paletteSetup(t *testing.T, w, h int, style string) (*Model, *recorder) {
	t.Helper()
	prev := lang
	SetLang("en")
	t.Cleanup(func() { SetLang(prev) })
	r := &recorder{}
	m := New(Options{OnAction: r.on, MusicStyle: style})
	m.SetSize(w, h)
	return m, r
}

// mlMarked is the row the highlight marker sits on, without the marker.
func mlMarked(m *Model) string {
	f := m.render()
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if c := f.at(x, y); c != nil && c.ch == '▌' {
				return strings.TrimSpace(rowText(f, y, x+1, m.w-x-1))
			}
		}
	}
	return ""
}

// mlPosOf is where the text starts, -1 when it is off screen.
func mlPosOf(m *Model, s string) (int, int) {
	f := m.render()
	want := []rune(s)
	for y := range m.h {
		row := []rune(rowText(f, y, 0, m.w))
		for x := 0; x+len(want) <= len(row); x++ {
			if string(row[x:x+len(want)]) == s {
				return x, y
			}
		}
	}
	return -1, -1
}

// mlRowOf is the row that shows the text, -1 when it is off screen.
func mlRowOf(m *Model, s string) int {
	f := m.render()
	for y := 0; y < m.h; y++ {
		if strings.Contains(rowText(f, y, 0, m.w), s) {
			return y
		}
	}
	return -1
}

func mlLastLoad(t *testing.T, r *recorder) MusicLoad {
	t.Helper()
	loads := recorded[MusicLoad](r)
	if len(loads) == 0 {
		t.Fatal("no MusicLoad went out")
	}
	return loads[len(loads)-1]
}

func mlType(m *Model, s string) {
	for _, r := range s {
		m.Key(string(r), string(r))
	}
}

// A1: ctrl+o in the player drops the palette over the running stage; the root shows
// the four entries and the launch's tracks, and asks for nothing yet.
func TestPaletteRoot(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.SetRecent([]Track{{ID: "r1", Artist: "Frank Ocean", Title: "Nights", Dur: 307}})
	if !m.Key("ctrl+o", "") {
		t.Fatal("ctrl+o in the player not taken")
	}
	if !m.MusicOpen() {
		t.Fatal("ctrl+o did not open the palette")
	}
	steps(m, 0.3)
	frame := stripANSI(m.View())
	for _, want := range []string{"favorites", "playlists", "albums", "songs", "library", "recent", "Nights"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the root does not show %q:\n%s", want, frame)
		}
	}
	if got := mlMarked(m); !strings.Contains(got, "favorites") {
		t.Errorf("selection = %q, want the favorites entry", got)
	}
	if loads := recorded[MusicLoad](r); len(loads) != 0 {
		t.Errorf("the root asked for rows: %+v", loads)
	}
	if !strings.Contains(frame, "open") {
		t.Errorf("the bar does not offer to open the entry:\n%s", frame)
	}
}

// A2: enter on an entry opens the section and asks for it; the chip names the level;
// esc comes back with the highlight on the entry it left.
func TestPaletteDrill(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	m.Key("down", "")
	m.Key("down", "")
	if got := mlMarked(m); !strings.Contains(got, "albums") {
		t.Fatalf("selection = %q, want the albums entry", got)
	}
	m.Key("enter", "")
	if got := mlLastLoad(t, r); got.Section != MusicAlbums || got.Parent != nil || got.Query != "" {
		t.Fatalf("load = %+v, want the albums section", got)
	}
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("a1", "Travis Scott", "UTOPIA", 10)}, "")
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "UTOPIA") {
		t.Errorf("the section rows are not drawn:\n%s", frame)
	}
	input := ""
	for _, line := range strings.Split(frame, "\n") {
		if strings.Contains(line, "search tracks") {
			input = line
		}
	}
	if !strings.Contains(input, "albums") {
		t.Errorf("the chip is not on the query row: %q\n%s", input, frame)
	}
	m.Key("esc", "")
	frame = stripANSI(m.View())
	if strings.Contains(frame, "UTOPIA") {
		t.Errorf("esc did not leave the section:\n%s", frame)
	}
	if got := mlMarked(m); !strings.Contains(got, "albums") {
		t.Errorf("selection = %q, want the albums entry back", got)
	}
	// Backspace on an empty query is the same step back.
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("a1", "Travis Scott", "UTOPIA", 10)}, "")
	m.Key("backspace", "")
	if m.mu.coll != nil || !m.mu.list.root {
		t.Errorf("backspace did not pop the level back to the root")
	}
	if got := mlMarked(m); !strings.Contains(got, "albums") {
		t.Errorf("selection = %q after backspace, want the albums entry", got)
	}
}

// A3: the search waits for a pause in the typing, one load per pause, and the answer
// of an older query never lands.
func TestPaletteSearchDebounce(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	mlType(m, "gn")
	m.Step(0.1)
	if loads := recorded[MusicLoad](r); len(loads) != 0 {
		t.Fatalf("the search went out before the pause: %+v", loads)
	}
	m.Step(0.3)
	loads := recorded[MusicLoad](r)
	if len(loads) != 1 || loads[0].Section != MusicSearch || loads[0].Query != "gn" {
		t.Fatalf("loads = %+v, want one search for gn", loads)
	}
	stale := loads[0].Gen
	mlType(m, "x")
	m.Step(0.3)
	loads = recorded[MusicLoad](r)
	if len(loads) != 2 || loads[1].Query != "gnx" || loads[1].Gen == stale {
		t.Fatalf("loads = %+v, want a new search for gnx", loads)
	}
	m.MusicRows(stale, []MusicItem{musicSong("s1", "Alpha", "Stale One", 200)}, "")
	if frame := stripANSI(m.View()); strings.Contains(frame, "Stale One") {
		t.Errorf("the stale generation landed:\n%s", frame)
	}
	m.MusicRows(loads[1].Gen, []MusicItem{musicSong("s2", "Alpha", "Fresh One", 200)}, "")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "Fresh One") {
		t.Errorf("the fresh generation did not land:\n%s", frame)
	}
	m.Key("esc", "")
	if q := m.mu.list.query; q != "" {
		t.Errorf("esc left the query at %q", q)
	}
	if got := mlMarked(m); !strings.Contains(got, "favorites") {
		t.Errorf("clearing the query left the selection at %q, want the level rows", got)
	}
}

// A4: the results settle on the top result of D1, the agent fallback is the last row,
// and enter on it sends the query to the agent.
func TestPaletteAgentFallback(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	mlType(m, "gn")
	steps(m, 0.4)
	rows := []MusicItem{
		musicSong("s1", "Kendrick Lamar", "squabble up", 157),
		musicAlbum("a1", "Kendrick Lamar", "GNX", 10),
	}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	if got := mlMarked(m); !strings.Contains(got, "GNX") {
		t.Fatalf("selection = %q, want the top result GNX", got)
	}
	m.Key("end", "")
	m.render()
	steps(m, 0.5)
	got := mlMarked(m)
	if !strings.Contains(got, "ask the agent") || !strings.Contains(got, "gn") {
		t.Fatalf("last row = %q, want the agent fallback with the query", got)
	}
	if y, a := mlRowOf(m, "GNX"), mlRowOf(m, "ask the agent"); y < 0 || a <= y {
		t.Errorf("the agent row (%d) is not under the results (%d)", a, y)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "ask") {
		t.Errorf("the bar does not offer to ask the agent:\n%s", frame)
	}
	m.Key("enter", "")
	says := recorded[Say](r)
	if len(says) != 1 || says[0].Text != "gn" {
		t.Fatalf("says = %+v, want one Say{gn}", r.acts)
	}
}

// A5: the intro without the palette is the agent's screen, exactly as before.
func TestPaletteIntroUntouched(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	mlType(m, "chill")
	if got := m.Input(); got != "chill" {
		t.Fatalf("input = %q, want chill", got)
	}
	m.Key("enter", "")
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "chill" {
		t.Fatalf("queries = %+v, want one Query{chill}", r.acts)
	}
	if loads := recorded[MusicLoad](r); len(loads) != 0 {
		t.Errorf("the intro sent loads: %+v", loads)
	}
	if m.MusicOpen() {
		t.Error("the music screen opened by itself")
	}
}

// A6: over the intro the fallback hands the query to the screen's own launch; in the
// player a picked song plays and the palette stays up.
func TestPaletteAgentOverIntro(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	if !m.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro not taken")
	}
	mlType(m, "zzz")
	steps(m, 0.4)
	if loads := recorded[MusicLoad](r); len(loads) != 1 {
		t.Fatalf("loads = %+v, want the one search", loads)
	}
	m.MusicRows(mlLastLoad(t, r).Gen, nil, "")
	if got := mlMarked(m); !strings.Contains(got, "ask the agent") {
		t.Fatalf("selection = %q, want the agent fallback", got)
	}
	m.Key("enter", "")
	if m.MusicOpen() {
		t.Error("the palette did not close")
	}
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "zzz" {
		t.Fatalf("queries = %+v, want one Query{zzz}", r.acts)
	}
	if m.Phase() != PhLaunch {
		t.Errorf("phase = %v, want the launch of the agent flow", m.Phase())
	}

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	if !m2.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro not taken")
	}
	mlType(m2, "gn")
	steps(m2, 0.4)
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{musicSong("s1", "Kendrick Lamar", "squabble up", 157)}, "")
	m2.Key("enter", "")
	plays := recorded[MusicPlay](r2)
	if len(plays) != 1 || len(plays[0].Items) != 1 || plays[0].Start != 0 {
		t.Fatalf("plays = %+v, want the one song", r2.acts)
	}
	m2.JumpPlay(threeTracks(), 0, 0)
	if !m2.MusicOpen() {
		t.Fatal("the palette closed on the jump to the player")
	}
	if got := mlMarked(m2); !strings.Contains(got, "squabble up") {
		t.Errorf("selection after the jump = %q, want it kept", got)
	}
}

// A7: tab switches the palette between the music and the agent and never queues;
// the queue lives in the actions menu now.
func TestPaletteTab(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "play") || !strings.Contains(frame, "actions") {
		t.Errorf("the bar does not offer to play and act on the song:\n%s", frame)
	}
	before := len(r.acts)
	m.Key("tab", "")
	if m.mu.mode != mlModeAgent {
		t.Fatalf("tab did not switch to the agent mode")
	}
	if len(r.acts) != before {
		t.Fatalf("tab sent %+v, want no queue any more", r.acts[before:])
	}
	m.Key("tab", "")
	if m.mu.mode != mlModeMusic {
		t.Fatalf("tab did not switch back to the music")
	}
	m.Key("ctrl+k", "")
	m.Key("down", "")
	m.Key("enter", "")
	app := recorded[MusicAppend](r)
	if len(app) != 1 || app[0].Item.ID != "s1" {
		t.Fatalf("appends = %+v, want s1 from the menu", r.acts)
	}

	m3, r3 := paletteSetup(t, 160, 45, "palette")
	m3.Key("tab", "")
	if got := recorded[Settings](r3); len(got) != 1 {
		t.Errorf("tab on the intro = %+v, want Settings", r3.acts)
	}
}

// A8: an unplayable row never plays or queues, and the bar carries the reason.
func TestPaletteUnplayable(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	up := MusicItem{ID: "u1", Kind: MusicSong, Title: "voice memo 03", Artist: "me", Dur: 42,
		Unplayable: "upload without catalog match — can't play on Linux"}
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200), up}, "")
	m.Key("down", "")
	before := len(r.acts)
	m.Key("enter", "")
	if len(r.acts) != before {
		t.Fatalf("an unplayable row sent %+v, want nothing", r.acts[before:])
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "can't play on Linux") {
		t.Errorf("the reason is not on screen:\n%s", frame)
	}
}

// A9: the open palette dims the stage and gives the dim back when it closes.
func TestPaletteDimsTheStage(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	steps(m, 1)
	before := m.dim.to
	m.Key("ctrl+o", "")
	steps(m, 2)
	if m.dim.x >= 0.7 {
		t.Errorf("dim = %v while the palette is open, want the stage under 0.7", m.dim.x)
	}
	m.Key("esc", "")
	if m.dim.to != before {
		t.Errorf("dim target = %v after closing, want %v", m.dim.to, before)
	}
	steps(m, 1)
	if m.dim.x < before-0.05 {
		t.Errorf("dim = %v after closing, want it back at %v", m.dim.x, before)
	}
}

// A10: the drawer takes its columns and lays the stage out in the rest.
func TestDrawerLaysOutTheStage(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "drawer")
	m.JumpPlay(threeTracks(), 0, 0)
	steps(m, 1)
	m.Key("ctrl+o", "")
	steps(m, 0.4)
	f := m.render()
	dw := m.musicDrawerWidth()
	if dw != 64 {
		t.Fatalf("drawer width = %d, want 64 at 160 columns", dw)
	}
	left := f.w - dw
	if m.titleBox.Min.X < 0 || m.titleBox.Max.X > left {
		t.Errorf("the title box %v is not inside the stage columns 0..%d", m.titleBox, left)
	}
	found := false
	for y := range f.h {
		row := rowText(f, y, 0, f.w)
		if at := strings.Index(row, "favorites"); at >= 0 {
			found = true
			if at < left {
				t.Errorf("drawer row %d starts at %d, want it past %d", y, at, left)
			}
		}
	}
	if !found {
		t.Fatalf("no drawer rows in the frame:\n%s", stripANSI(m.View()))
	}
	// The drawer walks the same levels as the palette.
	m.Key("down", "")
	m.Key("down", "")
	m.Key("enter", "")
	if got := mlLastLoad(t, r).Section; got != MusicAlbums {
		t.Fatalf("drawer load = %v, want the albums section", got)
	}
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("a1", "Travis Scott", "UTOPIA", 10)}, "")
	f = m.render()
	for y := range f.h {
		if at := strings.Index(rowText(f, y, 0, f.w), "UTOPIA"); at >= 0 {
			if at < left {
				t.Errorf("the drawer row %d is at %d, want it past %d", y, at, left)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("the drilled rows never reached the drawer:\n%s", stripANSI(m.View()))
	}
	if row := strings.TrimSpace(rowText(f, 6, 0, left)); row == "" {
		t.Errorf("the stage draws nothing in the columns the drawer leaves")
	}
}

// A11: both looks fit every size the player runs at.
func TestMusicListSmallFrames(t *testing.T) {
	sizes := [][2]int{{160, 45}, {120, 35}, {90, 25}, {60, 18}}
	for _, sz := range sizes {
		for _, style := range []string{"palette", "drawer"} {
			for _, intro := range []bool{false, true} {
				m, rec := paletteSetup(t, sz[0], sz[1], style)
				if !intro {
					m.JumpPlay(threeTracks(), 0, 0)
				}
				m.Key("ctrl+o", "")
				steps(m, 0.3)
				mlType(m, "gn")
				steps(m, 0.4)
				m.MusicRows(mlLastLoad(t, rec).Gen, []MusicItem{
					musicAlbum("a1", "Travis Scott", "UTOPIA", 10),
					musicSong("s1", "Frank Ocean", "Nights", 307),
				}, "")
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Fatalf("%dx%d %s intro=%v: render panicked: %v", sz[0], sz[1], style, intro, p)
						}
					}()
					f := m.render()
					if f.w != sz[0] || f.h != sz[1] {
						t.Fatalf("%dx%d: frame is %dx%d", sz[0], sz[1], f.w, f.h)
					}
					for y := range f.h {
						if got := len([]rune(rowText(f, y, 0, f.w))); got != f.w {
							t.Errorf("%dx%d %s: row %d has %d cells", sz[0], sz[1], style, y, got)
						}
					}
				}()
			}
		}
	}
}

// A12: space is text while the query is up and the host's play key while it is empty.
func TestPaletteSpace(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	if m.Key("space", " ") {
		t.Error("space with an empty query must reach the host")
	}
	m.Key("a", "a")
	if !m.Key("space", " ") {
		t.Fatal("space with a query must be typed")
	}
	if q := m.mu.list.query; q != "a " {
		t.Errorf("query = %q, want the space typed in", q)
	}
}

// F1: the keycap is " key " on its own background, then one plain space and the
// label in the dim color: no cap background beyond the cap.
func TestPaletteKeycapCells(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	steps(m, 0.3)
	f := m.render()
	x, y := -1, -1
	for yy := range f.h {
		for xx := range f.w {
			if c := f.at(xx, yy); c != nil && c.ch == '↵' {
				x, y = xx, yy
			}
		}
	}
	if x < 0 {
		t.Fatalf("no enter keycap in the bar:\n%s", stripANSI(m.View()))
	}
	capBg := m.th.a1.mul(0.35)
	for _, d := range []int{-1, 1} {
		if c := f.at(x+d, y); c == nil || c.bg != capBg {
			t.Errorf("cell %+d of the cap is not on the cap background", d)
		}
	}
	for d, want := range map[int]string{2: " ", 3: "p"} {
		c := f.at(x+d, y)
		if c == nil {
			t.Fatalf("cell %+d of the bar is off the frame", d)
		}
		if c.bg == capBg {
			t.Errorf("cell %+d after the cap keeps the cap background", d)
		}
		if string(c.ch) != want {
			t.Errorf("cell %+d = %q, want %q", d, string(c.ch), want)
		}
	}
	if c := f.at(x+3, y); c == nil || c.fg != m.th.dim {
		t.Errorf("the label is not in the dim color")
	}
	if got := rowText(f, y, x+3, 4); got != "play" {
		t.Errorf("label = %q, want play", got)
	}
}

// F2: the agent fallback stays pinned under the scrolling results of D3, and the
// highlight walks through the show-more row into it and back.
func TestPaletteAgentPinned(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	mlType(m, "gn")
	steps(m, 0.4)
	rows := make([]MusicItem, 0, 20)
	for i := range 20 {
		rows = append(rows, musicSong(fmt.Sprintf("s%02d", i), "Kendrick Lamar", fmt.Sprintf("track %02d", i), 200))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "ask the agent") {
		t.Fatalf("the fallback row is not pinned:\n%s", frame)
	}
	if !strings.Contains(frame, "track 00") {
		t.Errorf("the first result scrolled away with the fallback pinned:\n%s", frame)
	}
	if got := mlMarked(m); !strings.Contains(got, "track 00") {
		t.Errorf("selection = %q, want the first result", got)
	}
	for range 25 {
		m.Key("down", "")
	}
	m.render()
	steps(m, 0.5)
	if got := mlMarked(m); !strings.Contains(got, "ask the agent") {
		t.Fatalf("down to the end landed on %q, want the agent fallback", got)
	}
	f := m.render()
	agent, last := mlRowOf(m, "ask the agent"), -1
	for y := range f.h {
		if strings.Contains(rowText(f, y, 0, f.w), "track 05") {
			last = y
		}
	}
	if last < 0 {
		t.Errorf("the last result is not in view:\n%s", stripANSI(m.View()))
	} else if agent <= last {
		t.Errorf("the fallback row (%d) is not under the last result (%d)", agent, last)
	}
	m.Key("up", "")
	m.render()
	steps(m, 0.5)
	if got := mlMarked(m); !strings.Contains(got, "more") {
		t.Errorf("up from the fallback landed on %q, want the show-more row", got)
	}
}

// mlOpenSection walks the root to one of its entries and opens it.
func mlOpenSection(t *testing.T, m *Model, sec MusicSection) {
	t.Helper()
	for range len(mlEntries) + 1 {
		if l, ok := m.mlSelected(); ok && l.kind == mlEntry && l.sec == sec {
			m.Key("enter", "")
			return
		}
		m.Key("down", "")
	}
	t.Fatalf("no %v entry in the root", sec)
}

// A1: / in the player opens the agent, m and ctrl+o open the music.
func TestPaletteModeKeys(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	if !m.Key("/", "/") {
		t.Fatal("/ in the player not taken")
	}
	if !m.MusicOpen() || m.mu.mode != mlModeAgent {
		t.Fatalf("open = %v mode = %v, want the agent palette", m.MusicOpen(), m.mu.mode)
	}
	steps(m, 0.2)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "tell the agent where") {
		t.Errorf("the agent placeholder is missing:\n%s", frame)
	}
	m.Key("esc", "")
	if m.MusicOpen() {
		t.Fatal("esc did not close the palette")
	}
	if !m.Key("m", "m") || m.mu.mode != mlModeMusic {
		t.Fatalf("m in the player = %v, want the music mode", m.mu.mode)
	}
	steps(m, 0.2)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "search tracks") {
		t.Errorf("the music placeholder is missing:\n%s", frame)
	}
	m.Key("esc", "")
	if !m.Key("ctrl+o", "") || m.mu.mode != mlModeMusic {
		t.Fatalf("ctrl+o in the player = %v, want the music mode", m.mu.mode)
	}
}

// A2: the agent mode sends the whole query on enter, closes, and never falls back to
// the old inline reply.
func TestPaletteAgentSend(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("/", "/")
	steps(m, 0.2)
	mlType(m, "softer")
	m.Key("enter", "")
	says := recorded[Say](r)
	if len(says) != 1 || says[0].Text != "softer" {
		t.Fatalf("says = %+v, want one Say{softer}", r.acts)
	}
	if m.MusicOpen() {
		t.Error("the palette did not close after sending")
	}
	if m.typing {
		t.Error("m.typing is set, want the inline reply gone")
	}
	steps(m, 0.4)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "got it:") {
		t.Errorf("no confirmation line after sending:\n%s", frame)
	}
}

// A3: the empty agent mode shows the quick words; down to the first and enter sends it.
func TestPaletteAgentSteers(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("/", "/")
	steps(m, 0.2)
	m.NextGathering("")
	steps(m, 0.1)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "picking the next") {
		t.Errorf("the agent status does not show the gathering line:\n%s", frame)
	}
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "now playing") || !strings.Contains(frame, "One") {
		t.Errorf("the agent detail does not show the playing track:\n%s", frame)
	}
	for _, s := range []string{"calmer", "more energy", "more like this track", "something unfamiliar"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the agent mode does not show %q:\n%s", s, frame)
		}
	}
	m.Key("down", "")
	m.render()
	steps(m, 0.5)
	if got := mlMarked(m); !strings.Contains(got, "calmer") {
		t.Fatalf("selection = %q, want the calmer steer", got)
	}
	m.Key("enter", "")
	says := recorded[Say](r)
	if len(says) != 1 || says[0].Text != "calmer" {
		t.Fatalf("says = %+v, want one Say{calmer}", r.acts)
	}
	if m.MusicOpen() {
		t.Error("the palette did not close after the steer")
	}
}

// A4: tab keeps the typed query both ways and does not re-search a query already sent.
func TestPaletteTabKeepsQuery(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlType(m, "gn")
	steps(m, 0.4)
	loads := recorded[MusicLoad](r)
	if len(loads) != 1 {
		t.Fatalf("loads = %+v, want the one search", loads)
	}
	m.MusicRows(loads[0].Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	before := len(recorded[MusicLoad](r))
	m.Key("tab", "")
	if m.mu.mode != mlModeAgent || m.mu.list.query != "gn" {
		t.Fatalf("mode = %v query = %q after tab, want the agent with gn", m.mu.mode, m.mu.list.query)
	}
	m.Key("tab", "")
	if m.mu.mode != mlModeMusic || m.mu.list.query != "gn" {
		t.Fatalf("mode = %v query = %q after the second tab, want the music with gn", m.mu.mode, m.mu.list.query)
	}
	steps(m, 0.4)
	if got := len(recorded[MusicLoad](r)); got != before {
		t.Errorf("the toggle back sent %d more loads, want none", got-before)
	}
}

// A5: a song row is two cells tall, so the next title sits two cells lower.
func TestPaletteTwoLineRows(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	rows := []MusicItem{
		musicSong("s1", "Alpha", "Song A", 200),
		musicSong("s2", "Beta", "Song B", 201),
		musicSong("s3", "Gamma", "Song C", 202),
	}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	y1, y2 := mlRowOf(m, "Song A"), mlRowOf(m, "Song B")
	if y1 < 0 || y2 < 0 {
		t.Fatalf("rows are not in view: A=%d B=%d", y1, y2)
	}
	if y2-y1 != 2 {
		t.Errorf("Song B is %d cells under Song A, want 2", y2-y1)
	}
	if line := rowText(m.render(), y1+1, 0, m.w); !strings.Contains(line, "Alpha · 3:20") {
		t.Errorf("the second line = %q, want the artist and the length", line)
	}
	x, _, _, _ := m.mlPaletteGeom()
	f := m.render()
	pix := 0
	for yy := y1; yy < y1+2; yy++ {
		for xx := x + 3; xx < x+7; xx++ {
			if c := f.at(xx, yy); c != nil && c.pix {
				pix++
			}
		}
	}
	if pix != 8 {
		t.Errorf("the thumb covers %d of the 4×2 cells", pix)
	}
	steps(m, 0.5)
	f = m.render()
	left, right := f.at(x+1, y1), f.at(x+40, y1)
	if left == nil || left.ch != '▌' || f.at(x+1, y1+1) == nil || f.at(x+1, y1+1).ch != '▌' {
		t.Errorf("the selected row has no accent bar on both lines")
	}
	if left != nil && right != nil && left.bg == right.bg {
		t.Errorf("the row tint does not fade to the right")
	}
}

// A6: a wide panel shows the highlight's title in the detail pane; a narrow one only
// has the list.
func TestPaletteDetailPane(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	rows := []MusicItem{
		musicSong("s1", "Kendrick Lamar", "squabble up", 157),
		musicSong("s2", "Kendrick Lamar", "luther", 177),
	}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	steps(m, 0.5)
	if got := strings.Count(stripANSI(m.View()), "squabble up"); got < 2 {
		t.Errorf("the wide detail pane shows the title %d times, want the list and the pane", got)
	}
	px, py, pw, _ := m.mlPaletteGeom()
	dx := px + 1 + m.mlListW(pw-2) + musicDetailGap
	f := m.render()
	pix := 0
	for yy := py + 3; yy < py+3+musicCoverY; yy++ {
		for xx := dx; xx < dx+musicCoverX; xx++ {
			if c := f.at(xx, yy); c != nil && c.pix {
				pix++
			}
		}
	}
	if pix != musicCoverX*musicCoverY {
		t.Errorf("the detail cover covers %d cells, want %d", pix, musicCoverX*musicCoverY)
	}
	m.Key("down", "")
	steps(m, 0.05)
	if m.mu.detKey != "s2" || m.mu.detPrev != "s1" {
		t.Fatalf("detail keys = %q from %q, want the crossfade from s1 to s2", m.mu.detKey, m.mu.detPrev)
	}
	if k := easeOut(prog(m.t-m.mu.detT, 0, 0.15)); k >= 1 {
		t.Errorf("the crossfade is already over after 0.02 s")
	}
	steps(m, 0.3)
	if k := easeOut(prog(m.t-m.mu.detT, 0, 0.15)); k < 1 {
		t.Errorf("the crossfade did not finish")
	}

	m2, r2 := paletteSetup(t, 90, 25, "palette")
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	m2.Key("enter", "")
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{musicSong("s1", "Kendrick Lamar", "squabble up", 157)}, "")
	steps(m2, 0.3)
	if got := strings.Count(stripANSI(m2.View()), "squabble up"); got != 1 {
		t.Errorf("the narrow list shows the title %d times, want once and no pane", got)
	}
}

// A7: the albums of the root are a cover grid; arrows walk it in two dimensions.
func TestPaletteAlbumGrid(t *testing.T) {
	m, r := paletteSetup(t, 70, 30, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	albums := make([]MusicItem, 0, 6)
	for i := range 6 {
		albums = append(albums, musicAlbum(fmt.Sprintf("a%d", i), "Artist", fmt.Sprintf("Album %d", i+1), 10))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, albums, "")
	if cols := m.mlGridColsNow(); cols != 4 {
		t.Fatalf("columns = %d, want 4 at 70x30", cols)
	}
	if m.mu.list.sel != 0 {
		t.Fatalf("selection = %d, want the first tile", m.mu.list.sel)
	}
	m.Key("right", "")
	if m.mu.list.sel != 1 {
		t.Fatalf("right = %d, want 1", m.mu.list.sel)
	}
	m.Key("down", "")
	if m.mu.list.sel != 5 {
		t.Fatalf("down from 1 = %d, want 5", m.mu.list.sel)
	}
	m.mu.list.sel = 3
	m.Key("down", "")
	if m.mu.list.sel != 5 {
		t.Fatalf("down from the last row = %d, want the last tile 5", m.mu.list.sel)
	}
	m.mu.list.sel = 3
	m.Key("right", "")
	if m.mu.list.sel != 0 {
		t.Fatalf("right from the row end = %d, want the row start 0", m.mu.list.sel)
	}
	if l, ok := m.mlSelected(); !ok || l.kind != mlTile || l.it.Title != "Album 1" {
		t.Errorf("selected line = %+v, want the first album tile", l)
	}
	x1, y1 := mlPosOf(m, "Album 1")
	x2, y2 := mlPosOf(m, "Album 2")
	x5, y5 := mlPosOf(m, "Album 5")
	if x1 < 0 || x2 < 0 || x5 < 0 {
		t.Fatalf("grid titles are not on screen: %d,%d %d,%d %d,%d", x1, y1, x2, y2, x5, y5)
	}
	if y2 != y1 || x2-x1 != mlTileW+mlTileGap {
		t.Errorf("Album 2 at %d,%d is not the next tile after Album 1 at %d,%d", x2, y2, x1, y1)
	}
	if y5 != y1+mlTileRows {
		t.Errorf("Album 5 at row %d is not the next grid row after Album 1 at row %d", y5, y1)
	}
	m.mu.list.sel = 99
	m.SetSize(60, 30)
	if m.mu.list.sel != 5 {
		t.Errorf("selection after the resize = %d, want it clamped to the last tile", m.mu.list.sel)
	}
	if cols := m.mlGridColsNow(); cols != 3 {
		t.Errorf("columns after the resize = %d, want 3 at 60 columns", cols)
	}
}

// A8: ctrl+k opens the actions menu; the library and favorites entries send MusicSave,
// play all sends MusicPlayAll.
func TestPaletteActionsMenu(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	m.Key("right", "")
	if !m.mu.menu.open {
		t.Fatal("right on a song did not open the menu")
	}
	m.Key("esc", "")
	if m.mu.menu.open {
		t.Fatal("esc did not close the menu")
	}
	m.Key("ctrl+k", "")
	if !m.mu.menu.open {
		t.Fatal("ctrl+k did not open the menu")
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "add to favorites") {
		t.Fatalf("the menu has no favorites entry:\n%s", frame)
	}
	for range 3 {
		m.Key("down", "")
	}
	m.Key("enter", "")
	saves := recorded[MusicSave](r)
	if len(saves) != 1 || saves[0].Item.ID != "s1" || !saves[0].Favorites {
		t.Fatalf("saves = %+v, want MusicSave{s1, favorites}", r.acts)
	}
	if m.mu.menu.open {
		t.Error("the menu stayed open after the choice")
	}
	m.Key("ctrl+k", "")
	m.Key("down", "")
	m.Key("down", "")
	m.Key("enter", "")
	saves = recorded[MusicSave](r)
	if len(saves) != 2 || saves[1].Item.ID != "s1" || saves[1].Favorites {
		t.Fatalf("saves = %+v, want a second MusicSave{s1, library}", r.acts)
	}

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	mlOpenSection(t, m2, MusicAlbums)
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{musicAlbum("a1", "Beta", "Album One", 9)}, "")
	m2.Key("ctrl+k", "")
	if frame := stripANSI(m2.View()); !strings.Contains(frame, "play all") {
		t.Fatalf("the collection menu has no play all:\n%s", frame)
	}
	m2.Key("down", "")
	m2.Key("enter", "")
	alls := recorded[MusicPlayAll](r2)
	if len(alls) != 1 || alls[0].Item.ID != "a1" {
		t.Fatalf("play all = %+v, want MusicPlayAll{a1}", r2.acts)
	}
}

// A9: the action bar names the keys of the mode.
func TestPaletteBarPerMode(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	frame := stripANSI(m.View())
	for _, s := range []string{"open", "actions", "agent", "back"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the music bar has no %q:\n%s", s, frame)
		}
	}
	m.Key("tab", "")
	frame = stripANSI(m.View())
	for _, s := range []string{"send", "music", "close"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the agent bar has no %q:\n%s", s, frame)
		}
	}
}

// A10: both modes, in the player and over the intro, fit every size without drawing
// outside the frame.
func TestPaletteFramesAllSizes(t *testing.T) {
	sizes := [][2]int{{160, 45}, {120, 35}, {90, 25}, {60, 18}}
	for _, sz := range sizes {
		for _, agent := range []bool{false, true} {
			for _, intro := range []bool{false, true} {
				m, r := paletteSetup(t, sz[0], sz[1], "palette")
				if !intro {
					m.JumpPlay(threeTracks(), 0, 0)
				}
				if intro {
					m.Key("ctrl+o", "")
					if agent {
						m.Key("tab", "")
					}
				} else if agent {
					m.Key("/", "/")
				} else {
					m.Key("m", "m")
				}
				steps(m, 0.3)
				if !agent {
					m.Key("enter", "")
					if loads := recorded[MusicLoad](r); len(loads) > 0 {
						m.MusicRows(loads[len(loads)-1].Gen, []MusicItem{
							musicAlbum("a1", "Travis Scott", "UTOPIA", 10),
							musicSong("s1", "Frank Ocean", "Nights", 307),
						}, "")
					}
				} else {
					mlType(m, "softer")
				}
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Fatalf("%dx%d agent=%v intro=%v: render panicked: %v", sz[0], sz[1], agent, intro, p)
						}
					}()
					f := m.render()
					if f.w != sz[0] || f.h != sz[1] {
						t.Fatalf("%dx%d: frame is %dx%d", sz[0], sz[1], f.w, f.h)
					}
					for y := range f.h {
						if got := len([]rune(rowText(f, y, 0, f.w))); got != f.w {
							t.Errorf("%dx%d: row %d has %d cells", sz[0], sz[1], y, got)
						}
					}
					lines := strings.Split(stripANSI(m.View()), "\n")
					if len(lines) != sz[1] {
						t.Fatalf("%dx%d: view has %d lines", sz[0], sz[1], len(lines))
					}
					for y, line := range lines {
						if got := len([]rune(line)); got != sz[0] {
							t.Errorf("%dx%d: view row %d has %d cells, want %d", sz[0], sz[1], y, got, sz[0])
						}
					}
				}()
			}
		}
	}
}

// The intro has no player: the agent mode still shows its wait and the empty cover.
func TestPaletteAgentOverIntroMode(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	if !m.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro not taken")
	}
	m.Key("tab", "")
	if m.mu.mode != mlModeAgent {
		t.Fatalf("mode = %v after tab, want the agent", m.mu.mode)
	}
	steps(m, 0.2)
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "the agent is listening") || !strings.Contains(frame, "silence") {
		t.Errorf("the intro agent mode lacks the quiet status or the empty cover:\n%s", frame)
	}
	if strings.Contains(frame, "now playing") {
		t.Errorf("the intro agent mode claims a now-playing track:\n%s", frame)
	}
	mlType(m, "chill")
	m.Key("enter", "")
	if m.MusicOpen() {
		t.Error("the palette did not close")
	}
	if got := m.Input(); got != "chill" {
		t.Errorf("input = %q, want the line handed to the intro", got)
	}
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "chill" {
		t.Fatalf("queries = %+v, want one Query{chill}", r.acts)
	}
}

// The panel box follows the frame: 124 wide at most, six cells of air, y = 2, height
// up to h-5; the detail pane comes at 92 columns, the compact rows at 60.
func TestPaletteGeometry(t *testing.T) {
	for _, tc := range []struct{ w, h, wantW int }{
		{160, 45, 124}, {120, 35, 114}, {90, 25, 84}, {60, 18, 54},
	} {
		m, _ := paletteSetup(t, tc.w, tc.h, "palette")
		m.JumpPlay(threeTracks(), 0, 0)
		m.Key("m", "m")
		steps(m, 0.2)
		x, y, w, h := m.mlPaletteGeom()
		if w != tc.wantW || y != 2 || x != (tc.w-tc.wantW)/2 {
			t.Errorf("%dx%d: box = %d,%d %d wide, want %d at y=2", tc.w, tc.h, x, y, w, tc.wantW)
		}
		if h > tc.h-5 {
			t.Errorf("%dx%d: panel height = %d, want at most h-5", tc.w, tc.h, h)
		}
		if got := m.mlDetailOn(); got != (tc.wantW >= 92) {
			t.Errorf("%dx%d: detail = %v", tc.w, tc.h, got)
		}
		if got := m.mlCompact(); got != (tc.wantW < 60) {
			t.Errorf("%dx%d: compact = %v", tc.w, tc.h, got)
		}
	}
}

// The drop shadow darkens the one-cell band under and to the right of the panel by 40%.
func TestPaletteShadowBand(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	f := newFB(20, 20)
	base := col{0.5, 0.5, 0.5}
	f.fill(0, 0, 20, 20, base)
	m.mlShadow(f, 2, 2, 10, 10, 1)
	dark := base.mul(0.6)
	if got := f.at(12, 5).bg; got != dark {
		t.Errorf("right band = %+v, want %+v", got, dark)
	}
	if got := f.at(5, 12).bg; got != dark {
		t.Errorf("below band = %+v, want %+v", got, dark)
	}
	if got := f.at(1, 1).bg; got != base {
		t.Errorf("the cell outside the shadow changed to %+v", got)
	}
}
