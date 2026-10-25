package stage

import (
	"image"
	"image/color"
	"testing"
)

var yeezusSkin = Track{Artist: "Kanye West", Title: "Black Skinhead", Album: "Yeezus", Dur: 188}

// yeezusRed is the solid own cover: every pixel of the case is pure red.
func yeezusRed() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, red)
		}
	}
	return img
}

func yeezusModel(t *testing.T, o Options, w, h int, pos, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(w, h)
	m.SetCover(yeezusSkin.Key(), yeezusRed())
	m.JumpPlay([]Track{yeezusSkin}, 0, pos)
	steps(m, secs)
	return m, m.render()
}

func yeezusFrame(t *testing.T, o Options, pos, secs float64) (*Model, *fb) {
	t.Helper()
	return yeezusModel(t, o, 160, 45, pos, secs)
}

// yeezusPix: the cover pixel of the disc fractions (u, v) on the screen.
func yeezusPix(f *fb, g playGeom, u, v float64) col {
	return f.pixelAt(g.cx+int(u*float64(g.cw)), g.cy*2+int(v*float64(g.ch*2)))
}

// yeezusHue: h in [lo, hi]; hi <= lo means the wrap [hi, 360) union [0, lo].
func yeezusHue(h, lo, hi float64) bool {
	if hi <= lo {
		return h >= lo || h <= hi
	}
	return h >= lo && h <= hi
}

func TestYeezusRule(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(yeezusSkin)
	if e == nil || e.theme != &yeezusTheme || e.cover == nil || e.knob != '■' {
		t.Fatalf("eggOf(skin) = %+v, ждём тему, хук обложки и ручку ■", e)
	}
	if gr := m.eggOf(Track{Artist: "Kanye West", Title: "Power", Album: "Graduation", Dur: 264}); gr != nil && gr.theme == &yeezusTheme {
		t.Errorf("Graduation поймал тему Yeezus")
	}
	if e := m.eggOf(Track{Artist: "Kanye", Title: "Black Skinhead", Album: "Yeezus"}); e != nil {
		t.Errorf("артист Kanye без West не должен ловить Yeezus: %+v", e)
	}
}

func TestYeezusCase(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 60, 9)
	if lum := f.at(150, 30).bg.lum(); lum <= 0.5 {
		t.Errorf("кейс не белый: lum %.3f", lum)
	}
	if lum := m.th.text.lum(); lum >= 0.1 {
		t.Errorf("текст темы не чёрный: %.3f", lum)
	}
	_, ef := yeezusFrame(t, Options{}, 60, 0.1)
	if lum := ef.at(150, 30).bg.lum(); lum >= 0.3 {
		t.Errorf("пластик ещё тёмный в начале трека: lum %.3f", lum)
	}
}

func TestYeezusSpin(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 0, 21)
	g := m.geom()
	if h, _, _ := toHSV(yeezusPix(f, g, 0.3, 0.5)); !yeezusHue(h, 78, 102) {
		t.Errorf("на 21 с блик в %.1f°, ждём [78, 102]", h)
	}
	m, f = yeezusFrame(t, Options{}, 0, 5)
	g = m.geom()
	if h, _, _ := toHSV(yeezusPix(f, g, 0.3, 0.5)); !yeezusHue(h, 350, 10) {
		t.Errorf("на 5 с блик в %.1f°, ждём исходный [350, 360)∪[0, 10]", h)
	}
}

func TestYeezusStill(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 0, 21)
	g := m.geom()
	for _, p := range [][2]float64{{0.83, 0.5}, {0.97, 0.03}} {
		if h, _, _ := toHSV(yeezusPix(f, g, p[0], p[1])); !yeezusHue(h, 350, 10) {
			t.Errorf("лента/край (%.2f, %.2f) на 21 с в %.1f°, ждём красный", p[0], p[1], h)
		}
	}
}

func TestYeezusBar(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 60, 9)
	bb := m.barBox
	if c := f.at(bb.Min.X, bb.Min.Y); c.ch != '█' || c.fg.r <= 0.8 || c.fg.g >= 0.1 {
		t.Errorf("игранная ячейка %q %+v, ждём █ #e3001b", c.ch, c.fg)
	}
	if c := f.at(bb.Max.X-1, bb.Min.Y); c.ch != '─' {
		t.Errorf("хвост бара %q, ждём ─", c.ch)
	}
	kn := false
	for x := bb.Min.X; x < bb.Max.X && !kn; x++ {
		kn = f.at(x, bb.Min.Y).ch == '■'
	}
	if !kn {
		t.Errorf("ручки ■ нет в строке бара %v", bb)
	}
}

func TestYeezusTitle(t *testing.T) {
	m, f := yeezusFrame(t, Options{}, 60, 9)
	if m.titleBox.Empty() {
		t.Fatal("titleBox пустой")
	}
	dark := 0
	for py := m.titleBox.Min.Y; py < m.titleBox.Max.Y; py++ {
		for px := m.titleBox.Min.X; px < m.titleBox.Max.X; px++ {
			if c := f.pixelAt(px, py); c.lum() < 0.1 {
				dark++
			}
		}
	}
	if dark <= 20 {
		t.Errorf("в заголовке только %d чёрных пикселей, ждём больше 20", dark)
	}
}

func TestYeezusNoEggs(t *testing.T) {
	_, f := yeezusFrame(t, Options{NoEggs: true}, 60, 9)
	if lum := f.at(150, 30).bg.lum(); lum >= 0.3 {
		t.Errorf("NoEggs: фон %.3f, ждём тёмный (< 0.3)", lum)
	}
}

func TestYeezusDeterminism(t *testing.T) {
	_, a := yeezusFrame(t, Options{}, 25, 9)
	_, b := yeezusFrame(t, Options{}, 25, 9)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestYeezusBounds(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 94, 173} {
			for _, secs := range []float64{1, 9} {
				yeezusModel(t, Options{}, sz[0], sz[1], pos, secs)
			}
		}
	}
}
