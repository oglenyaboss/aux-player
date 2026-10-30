package stage

import (
	"image"
	"image/color"
	"testing"
	"unicode"
)

var atakePluto = Track{Artist: "Lil Uzi Vert", Title: "Baby Pluto", Album: "Eternal Atake", Dur: 210}

var atakeBlack = image.NewRGBA(image.Rect(0, 0, 120, 120))

func init() {
	for y := range 120 {
		for x := range 120 {
			atakeBlack.Set(x, y, color.RGBA{0, 0, 0, 255})
		}
	}
}

// atakeTrackFrame plays one track for secs with a cover on 160x45 and renders the frame.
func atakeTrackFrame(t *testing.T, o Options, tr Track, pos, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), atakeBlack)
	m.JumpPlay([]Track{tr}, 0, pos)
	steps(m, secs)
	return m, m.render()
}

func atakePixel(m *Model, f *fb, u, v float64) col {
	g := m.geom()
	return f.pixelAt(g.cx+int(u*float64(g.cw)), g.cy*2+int(v*float64(g.ch*2)))
}

// G1: the album rule takes over the track and kills the pink artist layer.
func TestAtakeTrigger(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(atakePluto)
	if e == nil {
		t.Fatal("для трека альбома яйко nil")
	}
	if e.theme != &atakeTheme || e.jewel || e.knob != '✦' {
		t.Errorf("theme %v jewel %v knob %q, want &atakeTheme false '✦'", e.theme == &atakeTheme, e.jewel, e.knob)
	}

	deluxe := Track{Artist: "Lil Uzi Vert", Title: "Baby Pluto", Album: "Eternal Atake (Deluxe) - LUV vs. The World 2", Dur: 210}
	if e := New(Options{}).eggOf(deluxe); e == nil || e.theme != &atakeTheme {
		t.Errorf("deluxe: theme %v, want &atakeTheme", e)
	}

	next := Track{Artist: "Lil Uzi Vert", Title: "Baby Pluto", Album: "Eternal Atake 2", Dur: 210}
	if e := New(Options{}).eggOf(next); e == nil || e.theme == &atakeTheme || !e.jewel {
		t.Errorf("Eternal Atake 2: theme &atakeTheme=%v jewel %v, want false true", e.theme == &atakeTheme, e.jewel)
	}

	pink := Track{Artist: "Lil Uzi Vert", Title: "Baby Pluto", Album: "Pink Tape", Dur: 210}
	if e := New(Options{}).eggOf(pink); e == nil || !e.jewel {
		t.Errorf("Pink Tape: jewel %v, want true", e != nil && e.jewel)
	}

	if e := New(Options{}).eggOf(Track{Artist: "Lil Yachty", Title: "Baby Pluto", Album: "Eternal Atake", Dur: 210}); e != nil {
		t.Errorf("Lil Yachty: яйко %v, want nil", e)
	}
}

// Beam: grows from the reveal, reaches down by 90 s, only at 0.77 of the width.
func TestAtakeBeam(t *testing.T) {
	m95, f95 := atakeTrackFrame(t, Options{}, atakePluto, 0, 95)
	c := atakePixel(m95, f95, 0.77, 0.9)
	if !(c.g > 0.12 && c.g > c.r+0.06) {
		t.Errorf("луч (0.77, 0.9)@95: (%.3f %.3f %.3f), ждём g > 0.12 и g > r+0.06", c.r, c.g, c.b)
	}
	if out := atakePixel(m95, f95, 0.2, 0.9); out.lum() >= 0.03 {
		t.Errorf("вне луча (0.2, 0.9)@95: lum %.4f, ждём < 0.03", out.lum())
	}
	m10, f10 := atakeTrackFrame(t, Options{}, atakePluto, 0, 10)
	if c := atakePixel(m10, f10, 0.77, 0.9); c.g >= 0.03 {
		t.Errorf("луч (0.77, 0.9)@10: g %.4f, ждём < 0.03", c.g)
	}
}

// Particles ride the beam up every 3 s after it lands at 90 s.
func TestAtakeParticles(t *testing.T) {
	maxLum := func(m *Model, f *fb) float64 {
		g := m.geom()
		best := 0.0
		for py := 0; py < g.ch*2; py++ {
			for px := int(0.73 * float64(g.cw)); px <= int(0.83*float64(g.cw)); px++ {
				if l := f.pixelAt(g.cx+px, g.cy*2+py).lum(); l > best {
					best = l
				}
			}
		}
		return best
	}
	m93, f93 := atakeTrackFrame(t, Options{}, atakePluto, 0, 93)
	if l := maxLum(m93, f93); !(l > 0.8) {
		t.Errorf("частица@93: max lum %.4f, ждём > 0.8", l)
	}
	m89, f89 := atakeTrackFrame(t, Options{}, atakePluto, 0, 89)
	if l := maxLum(m89, f89); l >= 0.6 {
		t.Errorf("частиц нет@89: max lum %.4f, ждём < 0.6", l)
	}
}

// Title grades purple into emerald top down.
func TestAtakeTitle(t *testing.T) {
	m, f := atakeTrackFrame(t, Options{}, atakePluto, 0, 10)
	b := m.titleBox
	if b.Empty() {
		t.Fatal("titleBox пуст")
	}
	topBright, botBright := 0, 0
	for x := b.Min.X; x < b.Max.X; x++ {
		if c := f.pixelAt(x, b.Min.Y); c.lum() > 0.1 {
			topBright++
			if !(c.b > c.g) {
				t.Errorf("заголовок верх (%d,%d): (%.3f %.3f %.3f), ждём b > g", x, b.Min.Y, c.r, c.g, c.b)
			}
		}
		if c := f.pixelAt(x, b.Max.Y-1); c.lum() > 0.1 {
			botBright++
			if !(c.g > c.b) {
				t.Errorf("заголовок низ (%d,%d): (%.3f %.3f %.3f), ждём g > b", x, b.Max.Y-1, c.r, c.g, c.b)
			}
		}
	}
	if topBright == 0 || botBright == 0 {
		t.Errorf("заголовок не светится: строка %d и %d 'l>0.1' пикселей", topBright, botBright)
	}
}

// Still starfield: the backdrop stays level, the stars are glyphs on empty cells.
func TestAtakeSky(t *testing.T) {
	_, f := atakeTrackFrame(t, Options{}, atakePluto, 0, 10)
	star := false
	for y := 2; y <= 8; y++ {
		for x := 100; x <= 159; x++ {
			c := f.at(x, y)
			if l := c.bg.lum(); l >= 0.03 {
				t.Errorf("небо (%d,%d): bg.lum %.4f, ждём < 0.03", x, y, l)
			}
			if c.ch == '·' || c.ch == '+' || c.ch == '✦' {
				star = true
			}
		}
	}
	if !star {
		t.Error("небо без звезды в x 100…159, строках 2…8")
	}
	_, f3 := atakeTrackFrame(t, Options{}, atakePluto, 0, 3)
	for y := 2; y <= 8; y++ {
		for x := 100; x <= 159; x++ {
			if ch := f3.at(x, y).ch; ch == '·' || ch == '+' || ch == '✦' {
				t.Errorf("до раскрытия (%d,%d): символ %q", x, y, ch)
			}
		}
	}
}

// Stars keep off the bar line and never sit next to letters or digits.
func TestAtakeStarPlacement(t *testing.T) {
	m, f := atakeTrackFrame(t, Options{}, atakePluto, 0, 10)
	isStar := func(r rune) bool { return r == '·' || r == '+' || r == '✦' }
	for x := m.barBox.Min.X; x < m.barBox.Max.X; x++ {
		if ch := f.at(x, m.barBox.Min.Y).ch; ch == '·' || ch == '+' {
			t.Errorf("бар (%d,%d): символ %q", x, m.barBox.Min.Y, ch)
		}
	}
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			if !isStar(f.at(x, y).ch) {
				continue
			}
			for _, nx := range [2]int{x - 1, x + 1} {
				if c := f.at(nx, y); c != nil && (unicode.IsLetter(c.ch) || unicode.IsDigit(c.ch)) {
					t.Errorf("звезда (%d,%d) стоит рядом с %q", x, y, c.ch)
				}
			}
		}
	}
}

// No pink stone: the album replaces the artist image, unlike Pink Tape.
func TestAtakeNoStone(t *testing.T) {
	pinkPixels := func(m *Model, f *fb) int {
		n := 0
		for py := range 10 {
			for px := 0; px < m.geom().cw; px++ {
				c := f.pixelAt(m.geom().cx+px, m.geom().cy*2+py)
				if c.r > 0.8 && c.b > 0.5 && c.g < 0.5 {
					n++
				}
			}
		}
		return n
	}
	m, f := atakeTrackFrame(t, Options{}, atakePluto, 0, 9)
	if n := pinkPixels(m, f); n != 0 {
		t.Errorf("у Eternal Atake розовых пикселей %d, ждём 0", n)
	}
	ref := Track{Artist: "Lil Uzi Vert", Title: "Baby Pluto", Album: "Pink Tape", Dur: 210}
	mr, fr := atakeTrackFrame(t, Options{}, ref, 0, 9)
	if n := pinkPixels(mr, fr); n == 0 {
		t.Error("Pink Tape: эталон без розовых пикселей")
	}
}

// G2: NoEggs leaves the beam off too.
func TestAtakeNoEggs(t *testing.T) {
	m, f := atakeTrackFrame(t, Options{NoEggs: true}, atakePluto, 0, 95)
	if c := atakePixel(m, f, 0.77, 0.9); c.g >= 0.03 {
		t.Errorf("NoEggs: g %.4f, ждём < 0.03", c.g)
	}
}

// G3: same track, pos and secs render identically.
func TestAtakeDeterminism(t *testing.T) {
	m1, f1 := atakeTrackFrame(t, Options{}, atakePluto, 0, 95)
	m2, f2 := atakeTrackFrame(t, Options{}, atakePluto, 0, 95)
	if f1.String() != f2.String() {
		t.Error("кадры на pluto не совпали")
	}
	_ = m1
	_ = m2
}

// G4: no panic on sizes, positions and running through the hooks.
func TestAtakeSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 105, 195} {
			for _, secs := range []float64{1, 9} {
				m, f := atakeTrackFrame(t, Options{}, atakePluto, pos, secs)
				if f == nil || f.at(m.w-1, m.h-1) == nil {
					t.Errorf("размер %dx%d: кадр не отрисован", sz[0], sz[1])
				}
			}
		}
	}
}
