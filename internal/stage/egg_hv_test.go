package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

var (
	hvSuperhero = Track{Artist: "Metro Boomin, Future & Chris Brown", Title: "Superhero (Heroes & Villains)", Album: "HEROES & VILLAINS", Dur: 182}
	hvCreepin   = Track{Artist: "Metro Boomin, The Weeknd & 21 Savage", Title: "Creepin'", Album: "HEROES & VILLAINS (Heroes Version)", Dur: 221}
	hvAroundMe  = Track{Artist: "Metro Boomin", Title: "Around Me (Instrumental)", Album: "HEROES & VILLAINS (Heroes Version)", Dur: 192}
)

// hvBurning is a dark cover with a bright orange figure on the left, like the man on fire.
func hvBurning() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := range 60 {
		for x := range 60 {
			c := color.RGBA{12, 26, 26, 255}
			if x >= 10 && x < 25 && y >= 10 && y < 50 {
				c = color.RGBA{255, 190, 90, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// hvBurningImg is the burning cover drawn once: every frame test reuses it.
var hvBurningImg = hvBurning()

func hvFrame(t *testing.T, o Options, tr Track, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), hvBurningImg)
	m.JumpPlay([]Track{tr}, 0, 60)
	steps(m, secs)
	return m, m.render()
}

// hvFlames counts fire pixels outside the cover: hot orange to yellow.
func hvFlames(m *Model, f *fb) int {
	g := m.geom()
	n := 0
	for py := 0; py < 2*m.h; py++ {
		for px := 0; px < m.w; px++ {
			if px >= g.cx-1 && px < g.cx+g.cw+1 && py >= g.cy*2-2 && py < g.cy*2+g.ch*2+2 {
				continue
			}
			if c := f.pixelAt(px, py); c.r > 0.7 && c.g > 0.25 && c.r > c.b+0.4 {
				n++
			}
		}
	}
	return n
}

func TestHVRule(t *testing.T) {
	m := New(Options{})
	for _, tr := range []Track{hvSuperhero, hvCreepin, hvAroundMe} {
		if e := m.eggOf(tr); e == nil || e.theme != &hvTheme || e.over == nil {
			t.Errorf("%s: egg %+v, want H&V", tr.Title, e)
		}
	}
	if e := m.eggOf(Track{Artist: "Metro Boomin", Title: "No Complaints", Album: "NOT ALL HEROES WEAR CAPES"}); e != nil {
		t.Errorf("another Metro album caught the egg: %+v", e)
	}
}

// The burning city: the fire grows in below the player, the skyline stays black
// along the bottom, and the flames on the cover flicker while the dark of the
// photo holds still. The steady render feeds the fire, skyline and flicker checks.
func TestHVCity(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvSuperhero, 0.2)
	if n := hvFlames(m, f); n > 20 {
		t.Errorf("at 0.2 s already %d fire pixels, want it still kindling", n)
	}
	m, f = hvFrame(t, Options{}, hvCreepin, 9)
	if n := hvFlames(m, f); n < 300 {
		t.Errorf("%s: %d fire pixels, want a burning city", hvCreepin.Title, n)
	}
	m, f = hvFrame(t, Options{}, hvSuperhero, 9)
	if n := hvFlames(m, f); n < 300 {
		t.Errorf("%s: %d fire pixels, want a burning city", hvSuperhero.Title, n)
	}
	dark := 0
	for px := 0; px < m.w; px++ {
		if f.pixelAt(px, 2*m.h-3).lum() < 0.05 {
			dark++
		}
	}
	if dark < m.w*2/3 {
		t.Errorf("dark pixels at the bottom of the screen %d of %d, want a city skyline", dark, m.w)
	}
	g := m.geom()
	fire := func(f *fb) col { return f.pixelAt(g.cx+g.cw*17/60, g.cy*2+g.ch) }
	darkPx := func(f *fb) col { return f.pixelAt(g.cx+g.cw*45/60, g.cy*2+g.ch) }
	a, da := fire(f), darkPx(f)
	changed := false
	for i := 0; i < 6 && !changed; i++ {
		steps(m, 0.3)
		changed = fire(m.render()) != a
	}
	if !changed {
		t.Errorf("the fire on the cover does not flicker")
	}
	if d := darkPx(m.render()); d != da {
		t.Errorf("the dark part of the cover shakes: %s → %s", da.css(), d.css())
	}
}

// The instrumentals are the morning after: no fire, only embers.
func TestHVAfter(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvAroundMe, 9)
	if n := hvFlames(m, f); n > 40 {
		t.Errorf("instrumental: %d fire pixels, want the city put out", n)
	}
}

// The queue and the help stay on screen over the fire.
func TestHVReadable(t *testing.T) {
	m := New(Options{SelfPlay: true})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{hvSuperhero, hvCreepin, hvAroundMe}, 0, 60)
	steps(m, 9)
	screen := stripANSI(m.render().String())
	for _, s := range []string{"Creepin'", "Around Me (Instrumental)"} {
		if !strings.Contains(screen, s) {
			t.Errorf("queue lost %q", s)
		}
	}
}

func TestHVNoEggs(t *testing.T) {
	m, f := hvFrame(t, Options{NoEggs: true}, hvSuperhero, 9)
	if n := hvFlames(m, f); n > 20 {
		t.Errorf("NoEggs: %d fire pixels", n)
	}
}

// G3 (TestHVDeterminism) lives in determinism_norace_test.go: it sits out of the
// -race run and runs in the plain, pre-commit and CI runs.

// G4: the egg survives a wide and a narrow window.
func TestHVSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {60, 12}} {
		for _, tr := range []Track{hvSuperhero, hvAroundMe} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{tr}, 0, 30)
			steps(m, 5)
			m.render()
		}
	}
}
