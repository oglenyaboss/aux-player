package stage

import (
	"math"
	"strings"
	"testing"
)

// dragonborn is the Dragonborn track: the album that names the whole egg after Skyrim.
var dragonborn = Track{Artist: "Big Baby Tape", Title: "Dragonborn", Album: "Dragonborn", Dur: 189}

// dragonBarRow is one bar row: the runes of the compass, over the bar width.
func dragonBarRow(m *Model, f *fb) string {
	y := m.barBox.Min.Y
	var b strings.Builder
	for x := m.barBox.Min.X; x < m.barBox.Max.X; x++ {
		if c := f.at(x, y); c != nil {
			b.WriteRune(c.ch)
		}
	}
	return b.String()
}

// dragonDarkPixels counts dark pixels of the top rows: the dragon's body.
func dragonDarkPixels(f *fb) int {
	n := 0
	for py := 2; py <= 6; py++ {
		for px := 0; px < f.w; px++ {
			if f.pixelAt(px, py).lum() < 0.015 {
				n++
			}
		}
	}
	return n
}

// R1 (G1): the egg hangs on the album; a half of the same artist with a different album
// or a half of another artist on the same album gets nothing.
func TestDragonRule(t *testing.T) {
	e := New(Options{}).eggOf(dragonborn)
	if e == nil {
		t.Fatal("egg nil")
	}
	if e.theme != &dragonTheme {
		t.Errorf("eggOf(dragonborn).theme != &dragonTheme")
	}
	if e.over == nil {
		t.Error("eggOf(dragonborn).over == nil")
	}
	other := dragonborn
	other.Album = "BANDANA I"
	if e := New(Options{}).eggOf(other); e != nil && e.theme == &dragonTheme {
		t.Error("the egg theme is hanging on the album BANDANA I")
	}
	if e := New(Options{}).eggOf(Track{Artist: "Kizaru", Title: "Dragonborn", Album: "Dragonborn"}); e != nil {
		t.Error("eggOf for the track of a different artist is not nil")
	}
}

// R3 (кейс 2): after 6 s there is a compass: edges, points of the world, quest marker; the bar is gone.
func TestDragonCompass(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dragonborn, 20, 8)
	y := m.barBox.Min.Y
	if got := f.at(m.barBox.Min.X, y).ch; got != '╟' {
		t.Errorf("in the first cell of the bar %q, want '╟'", got)
	}
	if got := f.at(m.barBox.Max.X-1, y).ch; got != '╢' {
		t.Errorf("in the last cell of the bar %q, want '╢'", got)
	}
	row := dragonBarRow(m, f)
	cardinals := 0
	for _, r := range row {
		switch r {
		case 'N', 'E', 'S', 'W':
			cardinals++
		}
	}
	if cardinals < 2 {
		t.Errorf("there are %d points of the world in the bar row, no less than 2 needed: %q", cardinals, row)
	}
	if !strings.ContainsRune(row, '▼') {
		t.Errorf("there is no quest marker on the compass: %q", row)
	}
	for _, r := range "━─●" {
		if strings.ContainsRune(row, r) {
			t.Errorf("the rune %q of the old bar remained: %q", r, row)
		}
	}
}

// R3 (кейс 3): the points of the world drift along the track position.
func TestDragonCompassFlow(t *testing.T) {
	m1, f1 := eggTrackFrame(t, Options{}, dragonborn, 20, 8)
	m2, f2 := eggTrackFrame(t, Options{}, dragonborn, 80, 8)
	r1, r2 := dragonBarRow(m1, f1), dragonBarRow(m2, f2)
	if r1 == r2 {
		t.Errorf("the compass has not moved: %q", r1)
	}
}

// R3 (кейс 4): up to 6 s the bar row is still the usual bar.
func TestDragonBarBeforeReveal(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dragonborn, 20, 3)
	row := dragonBarRow(m, f)
	if !strings.ContainsAny(row, "━─") {
		t.Errorf("the bar row before the reveal: %q, want ━ or ─", row)
	}
}

// R2 (кейс 5): at 6.3 s a wave is visible in pixels; after 7.6 s it's gone, even outside the cover.
func TestDragonThuum(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dragonborn, 20, 6.3)
	_, plain := eggTrackFrame(t, Options{NoEggs: true}, dragonborn, 20, 6.3)
	g := m.geom()
	row := g.cy + g.ch/2
	best := 0.0
	for x := 0; x < f.w; x++ {
		if l1, l2 := f.at(x, row).bg.lum(), plain.at(x, row).bg.lum(); math.Abs(l1-l2) > best {
			best = math.Abs(l1 - l2)
		}
	}
	if best <= 0.08 {
		t.Errorf("the maximum difference of the wave row is %.3f, want > 0.08", best)
	}
	m2, f2 := eggTrackFrame(t, Options{}, dragonborn, 20, 9)
	_, plain2 := eggTrackFrame(t, Options{NoEggs: true}, dragonborn, 20, 9)
	g2 := m2.geom()
	row2 := g2.cy + g2.ch/2
	residue := 0.0
	for x := 0; x < f2.w; x++ {
		if x >= g2.cx && x < g2.cx+g2.cw {
			continue
		}
		if d := f2.at(x, row2).bg.lum() - plain2.at(x, row2).bg.lum(); d > residue {
			residue = d
		}
	}
	if residue >= 0.05 {
		t.Errorf("after the wave the residue in the row %d is %.4f, want below 0.05", row2, residue)
	}
}

// R4 (кейс 6): at 35% of the track a dragon silhouette flies, at a rest position there are none.
func TestDragonFlyby(t *testing.T) {
	_, f := eggTrackFrame(t, Options{}, dragonborn, 64.15, 8)
	if n := dragonDarkPixels(f); n < 15 {
		t.Errorf("the dragon's dark pixels are %d, want no less than 15", n)
	}
	_, f2 := eggTrackFrame(t, Options{}, dragonborn, 20, 8)
	if n := dragonDarkPixels(f2); n != 0 {
		t.Errorf("there are dark pixels outside the flight: %d", n)
	}
}

// R3 (кейс 7): the compass line with a marker lives even in the mini layout.
func TestDragonMini(t *testing.T) {
	o := Options{SelfPlay: true}
	m := New(o)
	m.SetSize(60, 12)
	m.JumpPlay([]Track{dragonborn}, 0, 20)
	steps(m, 8)
	f := m.render()
	if row := dragonBarRow(m, f); !strings.ContainsRune(row, '▼') {
		t.Errorf("in the mini layout there is no marker in the compass row: %q", row)
	}
}

// G2: NoEggs turns everything off: the bar is usual, no marker.
func TestDragonNoEggs(t *testing.T) {
	m, f := eggTrackFrame(t, Options{NoEggs: true}, dragonborn, 20, 8)
	row := dragonBarRow(m, f)
	if !strings.ContainsAny(row, "━─") {
		t.Errorf("NoEggs: bar row %q, want ━ or ─", row)
	}
	if strings.ContainsRune(row, '▼') {
		t.Errorf("NoEggs drew the marker: %q", row)
	}
}

// G3: two identical models render identical frames.
func TestDragonSteady(t *testing.T) {
	m1, _ := eggTrackFrame(t, Options{}, dragonborn, 30, 8)
	m2, _ := eggTrackFrame(t, Options{}, dragonborn, 30, 8)
	if stripANSI(m1.View()) != stripANSI(m2.View()) {
		t.Error("the frames diverged")
	}
}

// G4: in no layout and no point in the track the egg falls apart.
func TestDragonSizes(t *testing.T) {
	for _, sz := range [4][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 94.5, 174} {
			for _, secs := range []float64{1, 9} {
				m := New(Options{SelfPlay: true})
				m.SetSize(sz[0], sz[1])
				m.JumpPlay([]Track{dragonborn}, 0, pos)
				steps(m, secs)
				m.render()
			}
		}
	}
}
