package stage

import (
	"math"
	"testing"
)

// dsotmMoney is the album track the acceptance cases run on.
var dsotmMoney = Track{Artist: "Pink Floyd", Title: "Money", Album: "The Dark Side of the Moon", Dur: 380}

// dsotmBarPlain: the plain progress bar glyphs are on the bar row of the frame.
func dsotmBarPlain(f *fb, m *Model) bool {
	for x := m.barBox.Min.X; x < m.barBox.Max.X; x++ {
		if c := f.at(x, m.barBox.Min.Y); c != nil && (c.ch == '━' || c.ch == '─') {
			return true
		}
	}
	return false
}

// dsotmPrRow: pixel row of ribbon band b on the bar box of the frame.
func dsotmPrRow(m *Model, b int) int { return (m.barBox.Min.Y-1)*2 + b }

// dsotmPulse steps the frame until the heart phase in hit (with its own head each
// frame) and sums the luminance of the pixels in the halo rectangle around the prism.
func dsotmPulse(t *testing.T, m *Model, hit func() bool) float64 {
	t.Helper()
	for i := 0; i < 120 && !hit(); i++ {
		steps(m, 1.0/30)
	}
	if !hit() {
		t.Fatal("фаза кардиограммы не наступила за 120 кадров")
	}
	f := m.render()
	hx := m.barBox.Min.X + int(m.pos/m.curTrack().Dur*float64(m.barBox.Dx()))
	n := 0.0
	for x := hx - 4; x <= hx+4; x++ {
		for y := dsotmPrRow(m, 0) - 3; y <= dsotmPrRow(m, 5)+3; y++ {
			n += f.pixelAt(x, y).lum()
		}
	}
	return n
}

func dsotmPeak(m *Model) bool { return math.Mod(m.pos, 0.83) < 0.034 }

func dsotmTrough(m *Model) bool {
	p := math.Mod(m.pos, 0.83)
	return p >= 0.55 && p <= 0.65
}

// 1. (G1) the album rule, and the title rules on top of it.
func TestDsotmTriggers(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(dsotmMoney)
	if e == nil {
		t.Fatal("egg nil на альбомном правиле")
	}
	if e.theme != &dsotmTheme || e.over == nil {
		t.Errorf("theme is d sotm прошивка: %p over present %v", e.theme, e.over != nil)
	}
	if e := m.eggOf(Track{Artist: "Pink Floyd", Title: "Time", Album: "Animals", Dur: 300}); e != nil {
		t.Errorf("трек с другого альбома: egg %+v, want nil", e)
	}
	if e := m.eggOf(Track{Artist: "Roger Waters", Title: "Money", Album: "The Dark Side of the Moon", Dur: 380}); e != nil {
		t.Errorf("другой артист, тот же альбом: egg %+v, want nil", e)
	}
	e = m.eggOf(Track{Artist: "Pink Floyd", Title: "Speak to Me", Album: "Pulse", Dur: 250})
	if e == nil || e.over == nil || e.theme != nil {
		t.Errorf("Speak to Me на другом альбоме: over %v theme %v", e != nil && e.over != nil, e != nil && e.theme != nil)
	}
}

// 2. played ribbon columns hold the six bands in order.
func TestDsotmRibbonColors(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	ranges := [][2]float64{{345, 360}, {12, 28}, {40, 55}, {128, 144}, {180, 192}, {254, 268}}
	for b, rg := range ranges {
		h, s, v := toHSV(f.pixelAt(m.barBox.Min.X+1, dsotmPrRow(m, b)))
		if v <= 0.1 {
			t.Errorf("полоса %d: h %.1f s %.2f v %.2f — пиксель пуст", b, h, s, v)
		}
		if !(h >= rg[0] && h < rg[1] || b == 0 && h >= 0 && h <= 10) {
			t.Errorf("полоса %d: h %.1f, want %.0f…%.0f", b, h, rg[0], rg[1])
		}
	}
}

// 3. unplayed columns keep the single dim pixel in the middle band.
func TestDsotmRibbonTail(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	x := m.barBox.Max.X - 2
	dim := float64(0x33) / 255
	c := f.pixelAt(x, dsotmPrRow(m, 2))
	if math.Abs(c.r-dim) > 0.03 || math.Abs(c.g-dim) > 0.03 || math.Abs(c.b-dim) > 0.03 {
		t.Errorf("несыгранная середина: %+v", c)
	}
	if c := f.pixelAt(x, dsotmPrRow(m, 0)); c.lum() >= 0.1 {
		t.Errorf("сверху несыгранной колонки полоса светит: lum %.2f", c.lum())
	}
}

// 4. the plain bar glyphs and the knob are gone from the bar row.
func TestDsotmRibbonCoversBar(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	for x := m.barBox.Min.X; x < m.barBox.Max.X; x++ {
		if c := f.at(x, m.barBox.Min.Y); c != nil {
			switch c.ch {
			case '━', '─', '●':
				t.Errorf("обычный бар продрался в колонке %d: %q", x, c.ch)
			}
		}
	}
}

// 5. the prism: white edge on top, dark inside below.
func TestDsotmPrism(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	hx := m.barBox.Min.X + int(m.pos/380*float64(m.barBox.Dx()))
	if c := f.pixelAt(hx, dsotmPrRow(m, 0)); c.lum() <= 0.85 {
		t.Errorf("верх призмы тёмный: lum %.2f", c.lum())
	}
	for dx := -1; dx <= 1; dx++ {
		if c := f.pixelAt(hx+dx, dsotmPrRow(m, 3)); c.lum() >= 0.05 {
			t.Errorf("внутри призмы светло: lum %.2f", c.lum())
		}
	}
}

// 6. before the reveal the beam is out and the plain bar is still there.
func TestDsotmBeamEarly(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 3)
	g := m.geom()
	by := g.cy*2 + int(0.49*float64(g.ch*2))
	if !dsotmBarPlain(f, m) {
		t.Errorf("до раскрытия на баре нет ━ или ─")
	}
	if c := f.pixelAt(2, by); c.lum() >= 0.5 {
		t.Errorf("луч до 6 с: lum %.2f", c.lum())
	}
}

// 7. after the reveal the beam runs from the screen edge to the cover.
func TestDsotmBeam(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	g := m.geom()
	by := g.cy*2 + int(0.49*float64(g.ch*2))
	for _, x := range []int{2, g.cx - 1} {
		if c := f.pixelAt(x, by); c.lum() <= 0.9 {
			t.Errorf("луч в x=%d не дошёл: lum %.2f", x, c.lum())
		}
	}
}

// 8. the bridge colours the gap next to the cover only after 6.8 s.
func TestDsotmBridge(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	g := m.geom()
	hits := 0
	for y := g.cy * 2; y < (g.cy+g.ch)*2; y++ {
		if _, s, v := toHSV(f.pixelAt(g.cx+g.cw+1, y)); s > 0.5 && v > 0.3 {
			hits++
		}
	}
	if hits < 4 {
		t.Errorf("мост: %d цветных пикселей, want ≥ 4", hits)
	}

	m, f = eggTrackFrame(t, Options{}, dsotmMoney, 100, 3)
	hits = 0
	for y := g.cy * 2; y < (g.cy+g.ch)*2; y++ {
		if _, s, v := toHSV(f.pixelAt(g.cx+g.cw+1, y)); s > 0.5 && v > 0.3 {
			hits++
		}
	}
	if hits != 0 {
		t.Errorf("мост до 6.8 с: %d цветных пикселей", hits)
	}
}

// 9. the title is near-white light: nothing saturated stays bright.
func TestDsotmTitle(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	for py := m.titleBox.Min.Y; py < m.titleBox.Max.Y; py++ {
		for px := m.titleBox.Min.X; px < m.titleBox.Max.X; px++ {
			c := f.pixelAt(px, py)
			if c.lum() > 0.3 {
				if _, s, _ := toHSV(c); s >= 0.15 {
					t.Errorf("заголовок цветной в %d,%d: s %.2f", px, py, s)
				}
			}
		}
	}
}

// 10. the heart shape: lub, dub, rest.
func TestDsotmBeat(t *testing.T) {
	if b := dsotmBeat(0); math.Abs(b-1) > 0.001 {
		t.Errorf("dsotmBeat(0) = %.4f, want 1", b)
	}
	if b := dsotmBeat(0.28); b < 0.85 || b > 0.95 {
		t.Errorf("dsotmBeat(0.28) = %.4f, want [0.85, 0.95]", b)
	}
	if b := dsotmBeat(0.6); b >= 0.1 {
		t.Errorf("dsotmBeat(0.6) = %.4f, want < 0.1", b)
	}
}

// 11. Speak to Me: the heart runs all track long.
func TestDsotmSpeakHeart(t *testing.T) {
	speak := Track{Artist: "Pink Floyd", Title: "Speak to Me", Album: "The Dark Side of the Moon", Dur: 64}
	m, _ := eggTrackFrame(t, Options{}, speak, 10, 10)
	peak := dsotmPulse(t, m, func() bool { return dsotmPeak(m) })
	trough := dsotmPulse(t, m, func() bool { return dsotmTrough(m) })
	if peak <= trough+1.0 {
		t.Errorf("сердце молчит: peak %.2f, trough %.2f + 1.0", peak, trough)
	}
}

// 12. Eclipse: silence at second 40, the heart in the last thirty seconds.
func TestDsotmEclipseHeart(t *testing.T) {
	ec := Track{Artist: "Pink Floyd", Title: "Eclipse", Album: "The Dark Side of the Moon", Dur: 127}
	m, _ := eggTrackFrame(t, Options{}, ec, 40, 10)
	peak := dsotmPulse(t, m, func() bool { return dsotmPeak(m) })
	trough := dsotmPulse(t, m, func() bool { return dsotmTrough(m) })
	if peak-trough >= 0.3 {
		t.Errorf("до тридцати секунд сердце стучит: peak %.2f − trough %.2f ≥ 0.3", peak, trough)
	}

	m, _ = eggTrackFrame(t, Options{}, ec, 100, 10)
	peak = dsotmPulse(t, m, func() bool { return dsotmPeak(m) })
	trough = dsotmPulse(t, m, func() bool { return dsotmTrough(m) })
	if peak <= trough+1.0 {
		t.Errorf("финальное сердце молчит: peak %.2f, trough %.2f + 1.0", peak, trough)
	}
}

// 13. mini layout: beam only, the plain bar stays.
func TestDsotmMini(t *testing.T) {
	m := New(Options{SelfPlay: true})
	m.SetSize(60, 12)
	m.JumpPlay([]Track{dsotmMoney}, 0, 100)
	steps(m, 10)
	f := m.render()
	if !m.geom().mini {
		t.Fatal("мини-раскладка не включилась в 60x12")
	}
	if !dsotmBarPlain(f, m) {
		t.Errorf("в мини-раскладке лента съела обычный бар")
	}
}

// 14. (G2) NoEggs leaves the player untouched.
func TestDsotmNoEggs(t *testing.T) {
	m, f := eggTrackFrame(t, Options{NoEggs: true}, dsotmMoney, 100, 10)
	g := m.geom()
	by := g.cy*2 + int(0.49*float64(g.ch*2))
	if !dsotmBarPlain(f, m) {
		t.Errorf("без пасхалок бар скрыт")
	}
	if c := f.pixelAt(2, by); c.lum() >= 0.5 {
		t.Errorf("без пасхалок луч светится: lum %.2f", c.lum())
	}
}

// 15. (G3) same track, same clock — same frame.
func TestDsotmDeterminism(t *testing.T) {
	a, _ := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	b, _ := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	if a.View() != b.View() {
		t.Errorf("два кадра одного трека разошлись")
	}
}

// 15. (G4) every size, every position — no panic.
func TestDsotmSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 190, 365} {
			for _, secs := range []float64{1, 9} {
				m := New(Options{SelfPlay: true})
				m.SetSize(sz[0], sz[1])
				m.JumpPlay([]Track{dsotmMoney}, 0, pos)
				steps(m, secs)
				m.render()
			}
		}
	}
}

// The prism stands over the queue too: the text check reads the cell under the pixel, not a cell further down.
func TestDsotmPrismOverQueue(t *testing.T) {
	tracks := []Track{dsotmMoney}
	for i := 0; i < 12; i++ {
		tracks = append(tracks, Track{Artist: "Queue Artist With A Long Name", Title: "Queue Title That Runs Far To The Right", Dur: 200})
	}
	m := New(Options{SelfPlay: true})
	m.SetSize(160, 45)
	m.JumpPlay(tracks, 0, 100)
	steps(m, 10)
	f := m.render()
	hx := m.barBox.Min.X + int(m.pos/dsotmMoney.Dur*float64(m.barBox.Dx()))
	if c := f.pixelAt(hx, (m.barBox.Min.Y-1)*2); c.lum() <= 0.85 {
		t.Errorf("вершина призмы над очередью: lum %.2f, want > 0.85", c.lum())
	}
}
