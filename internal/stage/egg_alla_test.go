package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

var (
	allaEvery = Track{Artist: "A$AP Rocky", Title: "Everyday (feat. Rod Stewart, Miguel & Mark Ronson)", Album: "AT.LONG.LAST.A$AP", Dur: 261}
	allaLSD   = Track{Artist: "A$AP Rocky", Title: "L$D", Album: "AT.LONG.LAST.A$AP", Dur: 238}
)

// allaPortrait is a grey cover with a pink stain at the top left, like the eye.
func allaPortrait() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 60, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			c := color.RGBA{uint8(60 + 2*x), uint8(60 + 2*x), uint8(60 + 2*x), 255}
			if x < 20 && y < 20 {
				c = color.RGBA{220, 110, 150, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func allaFrame(t *testing.T, o Options, tr Track, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), allaPortrait())
	m.JumpPlay([]Track{tr, hvCreepin}, 0, 30)
	steps(m, secs)
	return m, m.render()
}

func allaChroma(c col) float64 { return max(c.r, c.g, c.b) - min(c.r, c.g, c.b) }

func TestAllaRule(t *testing.T) {
	m := New(Options{})
	for _, tr := range []Track{allaEvery, allaLSD} {
		if e := m.eggOf(tr); e == nil || e.theme != &allaTheme {
			t.Errorf("%s: egg %+v", tr.Title, e)
		}
	}
	if e := m.eggOf(Track{Artist: "A$AP Rocky", Title: "Praise the Lord (Da Shine)", Album: "TESTING"}); e != nil {
		t.Errorf("TESTING поймал пасхалку")
	}
}

// A black-and-white film: outside the cover nothing has colour, the queue art included.
func TestAllaMonochrome(t *testing.T) {
	m, f := allaFrame(t, Options{}, allaEvery, 8)
	n := count(m, f, func(c col) bool { return allaChroma(c) > 0.04 })
	if n > 0 {
		t.Errorf("%d цветных пикселей вне обложки", n)
	}
}

// The stain on the cover keeps its colour and breathes.
func TestAllaStainBreathes(t *testing.T) {
	m, f := allaFrame(t, Options{}, allaEvery, 1)
	g := m.geom()
	eye := func(f *fb) col { return f.pixelAt(g.cx+g.cw/6, g.cy*2+g.ch/3) }
	a := eye(f)
	if allaChroma(a) < 0.15 {
		t.Fatalf("пятно на глазу потеряло цвет: %s", a.css())
	}
	if grey := f.pixelAt(g.cx+g.cw*3/4, g.cy*2+g.ch); allaChroma(grey) > 0.02 {
		t.Errorf("обложка вне пятна цветная: %s", grey.css())
	}
	steps(m, 2.2)
	if b := eye(m.render()); allaChroma(b) == allaChroma(a) {
		t.Errorf("пятно не дышит")
	}
}

// Smoke rises from below: the bottom of the screen is hazier than the top, and it drifts.
func TestAllaSmoke(t *testing.T) {
	m, f := allaFrame(t, Options{}, allaEvery, 8)
	band := func(f *fb, y0 int) (s float64) {
		for py := y0; py < y0+6; py++ {
			for px := 0; px < m.w; px++ {
				s += f.pixelAt(px, py).lum()
			}
		}
		return s
	}
	low, high := band(f, 2*m.h-8), band(f, 2)
	if low < high*1.3 {
		t.Errorf("дым внизу %.1f, вверху %.1f; ждём, что снизу гуще", low, high)
	}
	steps(m, 1)
	if band(m.render(), 2*m.h-8) == low {
		t.Errorf("дым стоит")
	}
}

// On L$D the smoke takes the colour of the stain.
func TestAllaTrip(t *testing.T) {
	m, f := allaFrame(t, Options{}, allaLSD, 8)
	if n := count(m, f, func(c col) bool { return allaChroma(c) > 0.08 && c.r > c.g && c.b > c.g }); n < 1500 {
		t.Errorf("L$D: %d цветных пикселей дыма", n)
	}
}

func allaColumn(m *Model, f *fb) string {
	var b strings.Builder
	for y := 0; y < m.h; y++ {
		b.WriteRune(f.at(1, y).ch)
	}
	return b.String()
}

func TestAllaSpine(t *testing.T) {
	m, f := allaFrame(t, Options{}, allaEvery, 3)
	if s := allaColumn(m, f); !strings.Contains(s, allaSpine) {
		t.Errorf("корешок: %q", s)
	}
}

func TestAllaReadable(t *testing.T) {
	_, f := allaFrame(t, Options{}, allaEvery, 8)
	if !strings.Contains(stripANSI(f.String()), "Creepin'") {
		t.Errorf("очередь потеряла трек")
	}
}

func TestAllaNoEggs(t *testing.T) {
	m, f := allaFrame(t, Options{NoEggs: true}, allaEvery, 8)
	if strings.Contains(allaColumn(m, f), allaSpine) {
		t.Errorf("NoEggs рисует корешок")
	}
}

func TestAllaDeterminism(t *testing.T) {
	_, a := allaFrame(t, Options{}, allaEvery, 5)
	_, b := allaFrame(t, Options{}, allaEvery, 5)
	if a.String() != b.String() {
		t.Errorf("кадры разъехались")
	}
}

func TestAllaSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}, {300, 80}, {20, 6}} {
		for _, tr := range []Track{allaEvery, allaLSD} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{tr}, 0, 30)
			steps(m, 5)
			m.render()
		}
	}
}
