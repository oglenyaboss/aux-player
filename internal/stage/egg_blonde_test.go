package stage

import (
	"image"
	"image/color"
	"testing"
)

var (
	blondeNikes   = Track{Artist: "Frank Ocean", Title: "Nikes", Album: "Blonde", Dur: 314}
	blondeNights  = Track{Artist: "Frank Ocean", Title: "Nights", Album: "Blonde", Dur: 307}
	blondeFerrari = Track{Artist: "Frank Ocean", Title: "White Ferrari", Album: "Blonde", Dur: 248}
	blondeSolo    = Track{Artist: "Frank Ocean", Title: "Solo", Album: "Blonde", Dur: 257}
	blondeReprise = Track{Artist: "Frank Ocean", Title: "Solo (Reprise)", Album: "Blonde", Dur: 78}
)

// blondeGreen is a flat green own cover: the day keeps it, the night swaps it for the smoke.
func blondeGreen() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := range 60 {
		for x := range 60 {
			img.Set(x, y, color.RGBA{40, 160, 60, 255})
		}
	}
	return img
}

func blondeFrame(t *testing.T, o Options, tr Track, pos, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), blondeGreen())
	m.JumpPlay([]Track{tr}, 0, pos)
	steps(m, secs)
	return m, m.render()
}

// blondeCoverMid: the middle pixel of the cover on the screen.
func blondeCoverMid(m *Model, f *fb) col {
	g := m.geom()
	return f.pixelAt(g.cx+g.cw/2, g.cy*2+g.ch)
}

func blondeSmoky(c col) bool { return c.r > c.b+0.08 && c.r >= c.g-0.05 }

func blondeGreenish(c col) bool { return c.g > c.r+0.15 && c.g > c.b+0.15 }

func TestBlondeRule(t *testing.T) {
	m := New(Options{})
	if e := m.eggOf(blondeNikes); e == nil || e.cover == nil || e.bg == nil || e.switchAt != 0 {
		t.Fatalf("Nikes: egg %+v, ждём хуки обложки и фона без свитча", e)
	}
	if e := m.eggOf(blondeNights); e == nil || e.switchAt != 210 {
		t.Fatalf("Nights: egg %+v, ждём свитч на 210", e)
	}
	if e := m.eggOf(blondeFerrari); e == nil || e.theme != &blondeNightTheme {
		t.Fatalf("White Ferrari: egg %+v, ждём ночную тему", e)
	}
	if e := m.eggOf(blondeSolo); e == nil || e.theme == &blondeNightTheme {
		t.Errorf("Solo — дневная сторона, а тема ночная")
	}
	if e := m.eggOf(blondeReprise); e == nil || e.theme != &blondeNightTheme {
		t.Errorf("Solo (Reprise) — ночная сторона, а тема не ночная")
	}
	if e := m.eggOf(Track{Artist: "Frank Ocean", Title: "Thinkin Bout You", Album: "channel ORANGE"}); e != nil {
		t.Errorf("channel ORANGE поймал пасхалку: %+v", e)
	}
}

func TestBlondeNightsLiveNoSwitch(t *testing.T) {
	e := New(Options{}).eggOf(Track{Artist: "Frank Ocean", Title: "Nights (Live)", Album: "Blonde"})
	if e == nil || e.switchAt != 0 {
		t.Errorf("Nights (Live): egg %+v, want the album egg without a switch", e)
	}
}

// The day: the usual player, the cover of the track, washed out like an overexposed film.
func TestBlondeDayWashed(t *testing.T) {
	m, f := blondeFrame(t, Options{}, blondeNikes, 60, 9)
	_, p := blondeFrame(t, Options{NoEggs: true}, blondeNikes, 60, 9)
	c, plain := blondeCoverMid(m, f), blondeCoverMid(m, p)
	if !blondeGreenish(c) {
		t.Fatalf("днём обложка %+v, ждём свою зелёную", c)
	}
	if c.lum() < plain.lum()+0.03 {
		t.Errorf("днём обложка %.3f против %.3f без пасхалки, ждём засвет", c.lum(), plain.lum())
	}
	if g := m.geom(); g.noCover {
		t.Errorf("днём раскладка перестроена, ждём обычный плеер")
	}
}

// The day light: a warm leak drifts across the backdrop.
func TestBlondeDayLeak(t *testing.T) {
	lo, hi := 2.0, 0.0
	m, _ := blondeFrame(t, Options{}, blondeNikes, 0, 9)
	for range 15 {
		l := m.render().at(m.w-6, 6).bg.lum()
		lo, hi = min(lo, l), max(hi, l)
		steps(m, 2)
	}
	if hi-lo < 0.04 {
		t.Errorf("блик не проходит: яркость фона в %.3f..%.3f", lo, hi)
	}
}

// The night side of the album: the smoke cover in place of the track cover.
func TestBlondeNightSide(t *testing.T) {
	m, f := blondeFrame(t, Options{}, blondeFerrari, 60, 9)
	if c := blondeCoverMid(m, f); !blondeSmoky(c) {
		t.Errorf("White Ferrari: обложка %+v, ждём дымную", c)
	}
	if g := m.geom(); g.noCover {
		t.Errorf("ночью раскладка перестроена, ждём обычный плеер")
	}
}

// The night drive: blurred city lights, red among them, drift across the backdrop.
func TestBlondeCityLights(t *testing.T) {
	m, f := blondeFrame(t, Options{}, blondeFerrari, 60, 9)
	g := m.geom()
	outside := func(x, y int) bool { return x >= g.cx+g.cw+2 || y >= g.cy+g.ch+2 }
	bright, red := 0, 0
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if c := f.at(x, y).bg; outside(x, y) {
				if c.lum() > 0.2 {
					bright++
				}
				if c.r > c.g+0.12 && c.r > 0.3 {
					red++
				}
			}
		}
	}
	if bright < 20 || red < 3 {
		t.Errorf("огней: светлых ячеек %d, красных %d; ждём от 20 и от 3", bright, red)
	}
	steps(m, 1)
	f2 := m.render()
	moved := 0
	for i := range f.c {
		if d := f.c[i].bg.lum() - f2.c[i].bg.lum(); d > 0.03 || d < -0.03 {
			moved++
		}
	}
	if moved < 30 {
		t.Errorf("за секунду сменилось %d ячеек фона, ждём огни в движении", moved)
	}
}

// Nights: the day until 3:30, then a hard cut to the smoke cover, no fade.
func TestBlondeNightsCut(t *testing.T) {
	m, f := blondeFrame(t, Options{}, blondeNights, 200, 9.8)
	if c := blondeCoverMid(m, f); !blondeGreenish(c) {
		t.Errorf("Nights на %.2f с: обложка %+v, ждём дневную", m.pos, c)
	}
	m, f = blondeFrame(t, Options{}, blondeNights, 200, 10.1)
	if c := blondeCoverMid(m, f); !blondeSmoky(c) {
		t.Errorf("Nights на %.2f с: обложка %+v, ждём дымную сразу после свитча", m.pos, c)
	}
}

func TestBlondeNoEggs(t *testing.T) {
	m, f := blondeFrame(t, Options{NoEggs: true}, blondeFerrari, 60, 9)
	if c := blondeCoverMid(m, f); !blondeGreenish(c) {
		t.Errorf("NoEggs: обложка %+v, ждём свою", c)
	}
}

func TestBlondeDeterminism(t *testing.T) {
	_, a := blondeFrame(t, Options{}, blondeNights, 205, 7)
	_, b := blondeFrame(t, Options{}, blondeNights, 205, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestBlondeSurvives(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, tr := range []Track{blondeNikes, blondeNights, blondeFerrari} {
			for _, pos := range []float64{0, 157, 209.5, 210.2, 240} {
				m := New(Options{SelfPlay: true})
				m.SetSize(sz[0], sz[1])
				m.JumpPlay([]Track{tr}, 0, pos)
				steps(m, 1)
				m.render()
			}
		}
	}
}
