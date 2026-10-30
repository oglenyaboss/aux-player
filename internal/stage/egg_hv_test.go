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

func hvFrame(t *testing.T, o Options, tr Track, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), hvBurning())
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
			t.Errorf("%s: egg %+v, ждём H&V", tr.Title, e)
		}
	}
	if e := m.eggOf(Track{Artist: "Metro Boomin", Title: "No Complaints", Album: "NOT ALL HEROES WEAR CAPES"}); e != nil {
		t.Errorf("другой альбом Метро поймал пасхалку: %+v", e)
	}
}

// The city burns below the player once the fire has grown.
func TestHVFire(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvSuperhero, 0.2)
	if n := hvFlames(m, f); n > 20 {
		t.Errorf("на 0.2 с уже %d пикселей огня, ждём разгорание", n)
	}
	for _, tr := range []Track{hvSuperhero, hvCreepin} {
		m, f = hvFrame(t, Options{}, tr, 9)
		if n := hvFlames(m, f); n < 300 {
			t.Errorf("%s: %d пикселей огня, ждём горящий город", tr.Title, n)
		}
	}
}

// The skyline stands black along the bottom of the screen.
func TestHVSkyline(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvSuperhero, 9)
	dark := 0
	for px := 0; px < m.w; px++ {
		if f.pixelAt(px, 2*m.h-3).lum() < 0.05 {
			dark++
		}
	}
	if dark < m.w*2/3 {
		t.Errorf("у низа экрана тёмных пикселей %d из %d, ждём силуэт города", dark, m.w)
	}
}

// The instrumentals are the morning after: no fire, only embers.
func TestHVAfter(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvAroundMe, 9)
	if n := hvFlames(m, f); n > 40 {
		t.Errorf("инструментал: %d пикселей огня, ждём потушенный город", n)
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
			t.Errorf("очередь потеряла %q", s)
		}
	}
}

// The flames on the cover flicker, the dark of the photo stays still.
func TestHVCoverFlicker(t *testing.T) {
	m, f := hvFrame(t, Options{}, hvSuperhero, 9)
	g := m.geom()
	fire := func(f *fb) col { return f.pixelAt(g.cx+g.cw*17/60, g.cy*2+g.ch) }
	dark := func(f *fb) col { return f.pixelAt(g.cx+g.cw*45/60, g.cy*2+g.ch) }
	a, da := fire(f), dark(f)
	changed := false
	for i := 0; i < 6 && !changed; i++ {
		steps(m, 0.3)
		changed = fire(m.render()) != a
	}
	if !changed {
		t.Errorf("огонь на обложке не мерцает")
	}
	if d := dark(m.render()); d != da {
		t.Errorf("тёмная часть обложки дрожит: %s → %s", da.css(), d.css())
	}
}

func TestHVNoEggs(t *testing.T) {
	m, f := hvFrame(t, Options{NoEggs: true}, hvSuperhero, 9)
	if n := hvFlames(m, f); n > 20 {
		t.Errorf("NoEggs: %d пикселей огня", n)
	}
}

func TestHVDeterminism(t *testing.T) {
	_, a := hvFrame(t, Options{}, hvSuperhero, 7)
	_, b := hvFrame(t, Options{}, hvSuperhero, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestHVSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}, {300, 80}} {
		for _, tr := range []Track{hvSuperhero, hvAroundMe} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{tr}, 0, 30)
			steps(m, 5)
			m.render()
		}
	}
}
