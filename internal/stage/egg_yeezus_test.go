package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

var yeezusSkin = Track{Artist: "Kanye West", Title: "Black Skinhead", Album: "Yeezus", Dur: 188}

var yeezusNext = Track{Artist: "Someone Else", Title: "Queued Next", Dur: 200}

// yeezusCaseCover stands in for the Apple cover: the left half blue, the right half
// green, and the red tape in its box on the right edge.
func yeezusCaseCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := range 200 {
		for x := range 200 {
			u, v := float64(x)/200, float64(y)/200
			c := color.RGBA{0, 0, 255, 255}
			if u >= 0.5 {
				c = color.RGBA{0, 200, 0, 255}
			}
			if u >= 0.69 && u <= 0.97 && v >= 0.37 && v <= 0.65 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// yeezusCaseImg is the case cover drawn once: every frame test reuses it.
var yeezusCaseImg = yeezusCaseCover()

func yeezusModel(t *testing.T, o Options, w, h int, pos, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(w, h)
	m.SetCover(yeezusSkin.Key(), yeezusCaseImg)
	m.JumpPlay([]Track{yeezusSkin, yeezusNext}, 0, pos)
	steps(m, secs)
	return m, m.render()
}

func yeezusFrame(t *testing.T, o Options, pos, secs float64) (*Model, *fb) {
	t.Helper()
	return yeezusModel(t, o, 160, 45, pos, secs)
}

func yeezusRedish(c col) bool { return c.r > 0.75 && c.g < 0.2 && c.b < 0.25 }

func yeezusOneBit(c col) bool { return c.lum() > 0.9 || c.lum() < 0.1 || yeezusRedish(c) }

func yeezusBlueOrGreen(c col) bool {
	return c.b > 0.8 && c.r < 0.2 || c.g > 0.6 && c.r < 0.2 && c.b < 0.2
}

// yeezusInCase: the pixel lies on the case.
func yeezusInCase(b yeezusBox, px, py int) bool {
	return px >= b.cx && px < b.cx+b.cs && py >= b.cy && py < b.cy+b.cs
}

func TestYeezusRule(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(yeezusSkin)
	if e == nil || e.theme != &yeezusTheme || e.layout == nil || e.knob != '■' {
		t.Fatalf("eggOf(skin) = %+v, want the theme, the layout and the ■ knob", e)
	}
	if gr := m.eggOf(Track{Artist: "Kanye West", Title: "Power", Album: "Graduation", Dur: 264}); gr != nil && gr.theme == &yeezusTheme {
		t.Errorf("Graduation picked up the Yeezus theme")
	}
	if e := m.eggOf(Track{Artist: "Kanye", Title: "Black Skinhead", Album: "Yeezus"}); e != nil {
		t.Errorf("an artist Kanye without West must not catch Yeezus: %+v", e)
	}
}

// The empty case at full play: no cover and no queue, the cover drawn big on the
// left at the full height with the disc in it; off the case one bit — white,
// black and the red of the tape; the black title stands right of the case. One
// render feeds all four checks.
func TestYeezusCase(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 60, 9)
	b, _ := yeezusPlan(m)
	if g := m.geom(); !g.noCover || !g.noQueue {
		t.Fatalf("geom %+v, want noCover and noQueue", g)
	}
	if strings.Contains(stripANSI(f.String()), "Queued Next") {
		t.Errorf("the queue is visible in the Yeezus layout")
	}
	for py := 0; py < m.h*2; py++ {
		for px := 0; px < m.w; px++ {
			if yeezusInCase(b, px, py) {
				continue
			}
			if c := f.pixelAt(px, py); !yeezusOneBit(c) {
				t.Fatalf("pixel (%d, %d) outside the case: %+v, want white, black or red", px, py, c)
			}
		}
	}
	if b.cs < m.h*2*8/10 {
		t.Errorf("the case is %d pixels with a window height of %d, want 80%% at the least", b.cs, m.h*2)
	}
	if c := f.pixelAt(b.cx+b.cs/10, b.cy+b.cs/10); !yeezusBlueOrGreen(c) {
		t.Errorf("corner of the case %+v, want a cover pixel", c)
	}
	minX, maxX, minY, maxY := m.w, 0, m.h*2, 0
	for py := 0; py < m.h*2; py++ {
		for px := b.cx + b.cs; px < m.w; px++ {
			if c := f.at(px, py/2); c.pix && f.pixelAt(px, py).lum() < 0.1 {
				minX, maxX, minY, maxY = min(minX, px), max(maxX, px), min(minY, py), max(maxY, py)
			}
		}
	}
	if span := m.w - b.cx - b.cs; maxX-minX < span*6/10 || maxY-minY < m.h*2/5 {
		t.Errorf("black letters within %d..%d × %d..%d, want wider than %d and taller than %d pixels", minX, maxX, minY, maxY, span*6/10, m.h*2/5)
	}
	if !strings.Contains(stripANSI(f.String()), "Kanye West") {
		t.Errorf("the artist is not in the frame")
	}
}

func TestYeezusFallback(t *testing.T) {
	m, f := yeezusModel(t, Options{}, 80, 40, 60, 9)
	if g := m.geom(); g.noCover || g.noQueue {
		t.Fatalf("portrait 80x40: geom %+v, want the usual layout", g)
	}
	if !strings.Contains(stripANSI(f.String()), "Queued Next") {
		t.Errorf("portrait 80x40: no queue")
	}
	m = New(Options{SelfPlay: true})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{{Artist: "Kanye West", Title: "Send It Up ★", Album: "Yeezus", Dur: 178}}, 0, 0)
	steps(m, 2)
	if g := m.geom(); g.noCover {
		t.Errorf("the title is outside the font: Yeezus layout, want the usual one")
	}
}

// The entrance: black for a blink, then a hard cut to the white case.
func TestYeezusEntrance(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 60, 0.05)
	if lum := f.at(m.w-4, 3).bg.lum(); lum >= 0.1 {
		t.Errorf("at 0.05 s the screen is not black: lum %.3f", lum)
	}
	m, _ = yeezusFrame(t, Options{}, 60, 1)
	if lum := m.render().at(m.w-4, 3).bg.lum(); lum <= 0.9 {
		t.Errorf("at 1 s the case is not white: lum %.3f", lum)
	}
}

// The tape of the cover never shows on the case except where our tape lies.
func TestYeezusCoverTapeHidden(t *testing.T) {
	for _, secs := range []float64{1, 3, 5, 7, 9} {
		m, _ := yeezusFrame(t, Options{}, 0, secs)
		f := m.render()
		b, _ := yeezusPlan(m)
		for py := b.cy; py < b.cy+b.cs; py++ {
			for px := b.cx; px < b.cx+b.cs; px++ {
				onTape := px >= b.tapeX0 && py >= b.tapeY0 && py <= b.tapeY1
				if c := f.pixelAt(px, py); !onTape && yeezusRedish(c) {
					t.Fatalf("at %.0f s a red cover pixel (%d, %d) lies outside our tape", secs, px, py)
				}
			}
		}
	}
}

// yeezusDiscProbe: the pixel of the disc halfway from the centre to the left rim.
func yeezusDiscProbe(m *Model, f *fb) col {
	b, _ := yeezusPlan(m)
	n := float64(b.cs)
	return f.pixelAt(b.cx+int((yeezusDiscU-yeezusDiscR/2)*n), b.cy+int(yeezusDiscV*n))
}

// The disc turns while the track plays and stands still on pause.
func TestYeezusSpin(t *testing.T) {
	m, _ := yeezusFrame(t, Options{}, 60, 9)
	seen := map[bool]bool{}
	for range 16 {
		c := yeezusDiscProbe(m, m.render())
		if !yeezusBlueOrGreen(c) {
			t.Fatalf("disc %+v, want a cover pixel", c)
		}
		seen[c.b > 0.8] = true
		steps(m, 0.5)
	}
	if len(seen) < 2 {
		t.Errorf("in 8 s the disc point did not cross half the cover: the disc does not spin")
	}
	m.Now(yeezusSkin.Key(), m.pos, false)
	m.o.SelfPlay = false
	steps(m, 0.1)
	a := yeezusSpin(m)
	steps(m, 1)
	if b := yeezusSpin(m); b != a {
		t.Errorf("the disc spins on pause: %.3f → %.3f", a, b)
	}
}

// The spin-up: in the first second the disc turns many times faster than later.
func TestYeezusSpinUp(t *testing.T) {
	m, _ := yeezusFrame(t, Options{}, 0, 0)
	a := yeezusSpin(m)
	steps(m, 0.5)
	early := yeezusSpin(m) - a
	steps(m, 9.5)
	a = yeezusSpin(m)
	steps(m, 0.5)
	late := yeezusSpin(m) - a
	if early < 3*late {
		t.Errorf("spin-up %.3f rad in 0.5 s at the start against %.3f later, want three times more", early, late)
	}
}

// The tape: where it lies on the cover at the start, pulled to the screen edge by the end.
func TestYeezusTape(t *testing.T) {
	for _, tc := range []struct{ pos, at float64 }{{0, 0}, {94, 0.5}, {187.4, 1}} {
		m, _ := yeezusFrame(t, Options{}, tc.pos, 0.5)
		f := m.render()
		b, _ := yeezusPlan(m)
		if b.tapeY1-b.tapeY0 < m.h*2/4 {
			t.Errorf("the tape is %d pixels, want a thickness of %d at the least", b.tapeY1-b.tapeY0, m.h*2/4)
		}
		mid := (b.tapeY0 + b.tapeY1) / 2
		left, right := -1, -1
		for x := 0; x < m.w; x++ {
			if yeezusRedish(f.pixelAt(x, mid)) {
				if left < 0 {
					left = x
				}
				right = x
			}
		}
		want := b.tapeX1 + int(tc.at*float64(m.w-b.tapeX1))
		if left != b.tapeX0 || right < want-3 || right > want+1 {
			t.Errorf("pos %.0f: tape %d..%d, want %d..%d", tc.pos, left, right, b.tapeX0, want)
		}
		if !strings.Contains(stripANSI(f.String()), fmtDur(m.pos)) {
			t.Errorf("pos %.0f: the time %s is not in the frame", tc.pos, fmtDur(m.pos))
		}
	}
}

func TestYeezusNoEggs(t *testing.T) {
	m, f := yeezusFrame(t, Options{NoEggs: true}, 60, 9)
	if lum := f.at(150, 30).bg.lum(); lum >= 0.3 {
		t.Errorf("NoEggs: backdrop %.3f, want it dark (< 0.3)", lum)
	}
	if g := m.geom(); g.noCover {
		t.Errorf("NoEggs: Yeezus layout")
	}
}

// G3 (TestYeezusDeterminism) lives in determinism_norace_test.go: it sits out of
// the -race run and runs in the plain, pre-commit and CI runs.

// G4: no panic on a wide and a narrow window, early and late positions.
func TestYeezusBounds(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {60, 12}} {
		for _, pos := range []float64{0, 7, 94, 187.9} {
			for _, secs := range []float64{0, 0.3, 1, 9} {
				yeezusModel(t, Options{}, sz[0], sz[1], pos, secs)
			}
		}
	}
}
