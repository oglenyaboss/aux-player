package stage

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// mlBarRows are the frame rows the selection marker sits on.
func mlBarRows(f *fb) []int {
	var out []int
	for y := range f.h {
		for x := range f.w {
			if c := f.at(x, y); c != nil && c.ch == '▌' {
				out = append(out, y)
				break
			}
		}
	}
	return out
}

// mlDist is how far apart two colors are.
func mlDist(a, b col) float64 {
	return math.Sqrt((a.r-b.r)*(a.r-b.r) + (a.g-b.g)*(a.g-b.g) + (a.b-b.b)*(a.b-b.b))
}

// A1: the player draws the same frame twice, and the halo of its cover stays inside
// the clip it is given.
func TestPaletteFxPlayFrameIdentity(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	steps(m, 2)
	a, b := m.render(), m.render()
	for y := range a.h {
		for x := range a.w {
			if *a.at(x, y) != *b.at(x, y) {
				t.Fatalf("cell %d,%d differs between two renders of the same frame", x, y)
			}
		}
	}

	f := newFB(30, 20)
	f.fill(0, 0, 30, 20, col{0.2, 0.3, 0.4})
	before := append([]cell(nil), f.c...)
	m.coverHalo(f, "halo", 12, 6, 8, 6, 1, [4]int{5, 5, 25, 15})
	for y := range f.h {
		for x := range f.w {
			if x >= 5 && x < 25 && y >= 5 && y < 15 {
				continue
			}
			if got, want := *f.at(x, y), before[y*f.w+x]; got != want {
				t.Errorf("the halo reached %d,%d outside the clip: %+v, want %+v", x, y, got, want)
			}
		}
	}
}

// A2: the detail cover glows into the air beside its middle and never touches the border.
func TestPaletteFxDetailHalo(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicSongs)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{
		musicSong("s1", "Kendrick Lamar", "squabble up", 157),
		musicSong("s2", "Kendrick Lamar", "luther", 177),
	}, "")
	m.render()
	steps(m, 0.5)
	f := m.render()
	px, py, pw, _ := m.mlPaletteGeom()
	listRight := px + 1 + m.mlListW(pw-2)
	coverX := listRight + musicDetailGap
	bg := m.th.bg.lerp(m.th.text, 0.07)
	for y := py + 3 + musicCoverY/4; y < py+3+musicCoverY*3/4; y++ {
		for x := listRight; x < coverX; x++ {
			if c := f.at(x, y); c == nil || mlDist(c.bg, bg) < 0.01 {
				t.Errorf("the gap cell %d,%d carries no glow", x, y)
			}
		}
	}
	for y := py + 3; y < py+3+musicCoverY; y++ {
		if c := f.at(px+pw-1, y); c == nil || mlDist(c.bg, bg) > 1e-9 {
			t.Errorf("the right border at %d,%d is tinted by the glow: %+v, want %+v", px+pw-1, y, c, bg)
		}
	}

	m2, _ := paletteSetup(t, 160, 45, "palette")
	if !m2.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro did not open the palette")
	}
	steps(m2, 0.3)
	px2, py2, pw2, _ := m2.mlPaletteGeom()
	f2 := m2.render()
	want := m2.th.bg.lerp(m2.th.text, 0.07)
	for y := py2 + 3; y < py2+3+musicCoverY; y++ {
		if c := f2.at(px2+pw2-1, y); c == nil || mlDist(c.bg, want) > 1e-9 {
			t.Errorf("the border with nothing playing at %d,%d is not the panel background", px2+pw2-1, y)
		}
	}
}

// A3: with nothing highlighted the detail shows the playing track, its progress and
// the two times; over the intro the box stays empty.
func TestPaletteFxNowPlaying(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 3)
	m.Key("m", "m")
	steps(m, 0.5)
	if l, ok := m.mlSelected(); !ok || l.kind != mlEntry {
		t.Fatalf("selected line = %+v, want a root entry", l)
	}
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "now playing") {
		t.Errorf("the detail has no now-playing caption:\n%s", frame)
	}
	if pane := mlPaneText(m); !strings.Contains(pane, "One") {
		t.Errorf("the detail does not name the playing track:\n%s", pane)
	}
	f := m.render()
	heavy, time := 0, ""
	for y := range f.h {
		for x := range f.w {
			if c := f.at(x, y); c != nil && c.ch == '━' {
				heavy++
			}
		}
		if row := rowText(f, y, 0, m.w); strings.Contains(row, "0:") && strings.Contains(row, "/") {
			time = row
		}
	}
	if heavy == 0 {
		t.Errorf("the progress line has no played cell")
	}
	if time == "" {
		t.Errorf("the progress line has no times under it")
	}

	m2, _ := paletteSetup(t, 160, 45, "palette")
	if !m2.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro did not open the palette")
	}
	steps(m2, 0.3)
	if frame := stripANSI(m2.View()); strings.Contains(frame, "now playing") {
		t.Errorf("the intro claims something plays:\n%s", frame)
	}
}

// mlPaneText is what the detail pane says, row by row.
func mlPaneText(m *Model) string {
	f := m.render()
	px, py, pw, ph := m.mlPaletteGeom()
	dx := px + 1 + m.mlListW(pw-2) + musicDetailGap
	var b strings.Builder
	for y := py + 3; y < py+3+ph-6; y++ {
		b.WriteString(rowText(f, y, dx, musicCoverX))
		b.WriteByte('\n')
	}
	return b.String()
}

// A4: the tracks of a collection already opened are shown in the detail of its row,
// and reopening the collection shows them at once while asking again.
func TestPaletteFxPeek(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("alb-utopia", "Travis Scott", "UTOPIA", 3)}, "")
	m.Key("enter", "")
	load := mlLastLoad(t, r)
	if load.Parent == nil || load.Parent.Title != "UTOPIA" {
		t.Fatalf("load = %+v, want the tracks of UTOPIA", load)
	}
	m.MusicRows(load.Gen, []MusicItem{
		musicSong("t1", "Travis Scott", "A", 100),
		musicSong("t2", "Travis Scott", "B", 200),
		musicSong("t3", "Travis Scott", "C", 300),
	}, "")
	steps(m, 0.2)
	m.Key("esc", "")
	m.render()
	steps(m, 0.5)
	if l, ok := m.mlSelected(); !ok || l.it.Title != "UTOPIA" {
		t.Fatalf("selection after esc = %+v, want the album", l)
	}
	pane := mlPaneText(m)
	for _, want := range []string{"01 A", "02 B", "03 C"} {
		if !strings.Contains(pane, want) {
			t.Errorf("the detail of the album does not peek %q:\n%s", want, pane)
		}
	}

	before := len(recorded[MusicLoad](r))
	m.Key("enter", "")
	if got := len(recorded[MusicLoad](r)); got != before+1 {
		t.Fatalf("reopening sent %d loads, want one more", got-before)
	}
	frame := stripANSI(m.View())
	if strings.Contains(frame, "· · · ·") {
		t.Errorf("the cached tracks came with the loading rows:\n%s", frame)
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the reopened collection does not show %q:\n%s", want, frame)
		}
	}

	peeked := len(m.mu.peek)
	mlType(m, "ni")
	steps(m, 0.4)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s9", "Alpha", "Night", 100)}, "")
	if got := len(m.mu.peek); got != peeked {
		t.Errorf("the search grew the peek map to %d keys, want the %d of the collections", got, peeked)
	}
}

// A5: the marker glides to the row the highlight moved to, and lands at once on a
// level change.
func TestPaletteFxGlide(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.3)
	rows := mlBarRows(m.render())
	if len(rows) != 2 {
		t.Fatalf("the marker covers %d rows, want the two of an entry", len(rows))
	}
	old := rows[0]
	m.Key("down", "")
	m.render()
	if got := mlBarRows(m.render()); len(got) == 0 || got[0] != old {
		t.Fatalf("the marker moved without a tick: %v, want it on %d", got, old)
	}
	m.render()
	steps(m, 0.1)
	if got := mlBarRows(m.render()); len(got) == 0 || got[0] <= old || got[0] >= old+2 {
		t.Errorf("after 0.1 s the marker rows are %v, want them between %d and %d", got, old, old+2)
	}
	m.render()
	steps(m, 0.5)
	if got := mlBarRows(m.render()); len(got) == 0 || got[0] != old+2 {
		t.Errorf("the settled marker rows are %v, want %d", got, old+2)
	}

	m.Key("up", "")
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{
		musicSong("s1", "Alpha", "Song A", 200),
		musicSong("s2", "Beta", "Song B", 201),
	}, "")
	got := mlBarRows(m.render())
	if len(got) == 0 {
		t.Fatal("the new level has no marker")
	}
	if row := mlRowOf(m, "Song A"); got[0] != row {
		t.Errorf("the marker of the new level is on row %d, want the selected row %d", got[0], row)
	}
}

// A6: the rows come in faded and settle on the usual color.
func TestPaletteFxCascade(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.3)
	mlOpenSection(t, m, MusicSongs)
	rows := make([]MusicItem, 0, 8)
	for i := 1; i <= 8; i++ {
		rows = append(rows, musicSong(fmt.Sprintf("s%d", i), "Alpha", fmt.Sprintf("Song %02d", i), 200))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	x, y := mlPosOf(m, "Song 08")
	if x < 0 {
		t.Fatalf("the last row is not in view:\n%s", stripANSI(m.View()))
	}
	bg := m.th.bg.lerp(m.th.text, 0.07)
	if got := m.render().at(x+1, y).fg; mlDist(got, bg) > 1e-9 {
		t.Errorf("the last row came in at %+v, want the panel background %+v", got, bg)
	}
	m.render()
	steps(m, 0.6)
	settled := m.render().at(x+1, y).fg
	if settled != m.th.text {
		t.Errorf("the settled title = %+v, want the row color %+v", settled, m.th.text)
	}
	if mlDist(settled, bg) <= 0 {
		t.Errorf("the settled title stays on the panel background")
	}
}

// A7: the title of the detail resolves through the scramble of the stage.
func TestPaletteFxScramble(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.3)
	mlOpenSection(t, m, MusicSongs)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{
		musicSong("s1", "Travis Scott", "NIGHTS", 300),
		musicSong("s2", "Travis Scott", "TELEKINESIS", 300),
	}, "")
	steps(m, 0.2)
	m.Key("down", "")
	m.render()
	m.Step(1.0 / 20)
	if early := mlPaneText(m); strings.Contains(early, "TELEKINESIS") {
		t.Errorf("the detail title at +0.05 s already reads the whole title:\n%s", early)
	}
	m.render()
	steps(m, 0.5)
	if late := mlPaneText(m); !strings.Contains(late, "TELEKINESIS") {
		t.Errorf("the detail title at +0.55 s did not resolve:\n%s", late)
	}
}
