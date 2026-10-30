package stage

import (
	"math"
	"testing"
)

func pulsarFrame(t *testing.T, o Options, pos, secs float64) (*Model, *fb) {
	t.Helper()
	disorder := Track{Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209}
	return eggTrackFrame(t, o, disorder, pos, secs)
}

// pulsarCell collects a bar-strip cell; dot is true when the glyph is a filled braille pattern.
func pulsarCell(f *fb, x, y int) (c *cell, dot bool) {
	c = f.at(x, y)
	if c == nil {
		return nil, false
	}
	return c, c.ch >= 0x2801 && c.ch <= 0x28FF
}

func TestPulsarRule(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(Track{ID: "jd-up", Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209})
	if e == nil {
		t.Fatal("egg nil на треке альбома")
	}
	if e.theme != &pulsarTheme {
		t.Errorf("theme %v, want &pulsarTheme", e.theme)
	}
	if e.over == nil {
		t.Error("over nil")
	}
	if e.theme == nil || e.theme.bg != hex(0x03070c) || e.theme.a1 != hex(0xf0f0f0) {
		t.Error("цвета темы не из брифа")
	}
	if e := m.eggOf(Track{ID: "jd-closer", Artist: "Joy Division", Title: "Disorder", Album: "Closer", Dur: 209}); e != nil {
		t.Errorf("Closer: egg %+v, want nil", *e)
	}
	if e := m.eggOf(Track{ID: "no-up", Artist: "New Order", Title: "Disorder", Album: "Unknown Pleasures", Dur: 209}); e != nil {
		t.Errorf("New Order: egg %+v, want nil", *e)
	}
}

func TestPulsarTrace(t *testing.T) {
	m, f := pulsarFrame(t, Options{}, 50, 10)
	b := m.barBox
	bw := b.Dx()
	dots := 0
	for row := b.Min.Y - 2; row <= b.Min.Y; row++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c, isDot := pulsarCell(f, x, row)
			if c == nil {
				continue
			}
			if c.ch == '━' || c.ch == '─' || c.ch == '●' {
				t.Errorf("(%d, %d): %q обычный бар не стёрт", x, row, c.ch)
			}
			if isDot && row == b.Min.Y {
				dots++
			}
		}
	}
	if dots < bw/2 {
		t.Errorf("в строке бара %d ячеек с точками, нужно не меньше %d", dots, bw/2)
	}
}

func TestPulsarFlat(t *testing.T) {
	m, f := pulsarFrame(t, Options{}, 0, 8)
	b := m.barBox
	for _, row := range []int{b.Min.Y - 2, b.Min.Y - 1} {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c, dot := pulsarCell(f, x, row); c != nil && dot {
				t.Errorf("(%d, %d): точка %04X над ровной линией", x, row, c.ch)
			}
		}
	}
	dots := false
	for x := b.Min.X; x < b.Max.X && !dots; x++ {
		_, dots = pulsarCell(f, x, b.Min.Y)
	}
	if !dots {
		t.Error("в строке бара нет точек")
	}
}

func TestPulsarStack(t *testing.T) {
	m, f := pulsarFrame(t, Options{}, 50, 10)
	b := m.barBox
	up1, up2 := false, false
	for x := b.Min.X; x < b.Max.X; x++ {
		if _, dot := pulsarCell(f, x, b.Min.Y-1); dot {
			up1 = true
		}
		if _, dot := pulsarCell(f, x, b.Min.Y-2); dot {
			up2 = true
		}
	}
	if !up1 {
		t.Error("в строке Min.Y-1 нет точек")
	}
	if !up2 {
		t.Error("в строке Min.Y-2 нет точек")
	}
}

func TestPulsarPlayedUnplayed(t *testing.T) {
	m, f := pulsarFrame(t, Options{}, 50, 10)
	b := m.barBox
	if x := f.at(b.Min.X, b.Min.Y); x == nil || x.fg.lum() <= 0.8 {
		t.Errorf("(Min.X, Min.Y) fg %+v lum %v, want lum > 0.8", x.fg, x.fg.lum())
	}
	c, dot := pulsarCell(f, b.Max.X-1, b.Min.Y)
	if !dot {
		t.Error("(Max.X-1, Min.Y) нет точек")
	}
	have, want := c.fg.q(), hex(0x7d868f).q()
	for i, d := range have {
		if d2 := d - want[i]; d2 < -3 || d2 > 3 {
			t.Errorf("(Max.X-1, Min.Y) fg %v от #%02x по каналу %d, want в пределах 0.03", have, 0x7d868f, i)
		}
	}
}

func TestPulsarBeat(t *testing.T) {
	m, _ := pulsarFrame(t, Options{}, 50, 10)
	b := m.barBox
	bw := b.Dx()
	glow := func() float64 {
		f := m.render()
		x1 := f.at(b.Min.X+bw/2-1, b.Min.Y-1).fg.lum()
		x2 := f.at(b.Min.X+bw/2, b.Min.Y-1).fg.lum()
		return max(x1, x2)
	}
	peak, trough := 0.0, -1.0
	for range 200 {
		steps(m, 1.0/30)
		p := math.Mod(m.pos, 1.3373)
		switch {
		case p < 0.034:
			peak = max(peak, glow())
		case p >= 0.65 && p <= 0.75 && peak > 0:
			trough = max(trough, glow())
		}
		if trough >= 0 && p > 0.75 {
			break
		}
	}
	if trough < 0 {
		t.Fatal("не дошагал до фазы в [0.65, 0.75]")
	}
	if !(peak > trough+0.1) {
		t.Errorf("peak %v, trough %v, нужно peak > trough + 0.1", peak, trough)
	}
}

func TestPulsarNewPulse(t *testing.T) {
	m1, f1 := pulsarFrame(t, Options{}, 50, 10)
	m2, f2 := pulsarFrame(t, Options{}, 67, 10)
	b1, b2 := m1.barBox, m2.barBox
	if b1.Min.Y != b2.Min.Y || b1.Dx() != b2.Dx() {
		t.Fatalf("разная геометрия бара: %v и %v", b1, b2)
	}
	same := true
	for x := b1.Min.X; x < b1.Max.X && same; x++ {
		ch1 := f1.at(x, b1.Min.Y).ch
		ch2 := f2.at(x, b2.Min.Y).ch
		if ch1 != ch2 {
			same = false
		}
	}
	if same {
		t.Errorf("профиль строки бара не изменился: pos %#v -> %#v", m1.pos, m2.pos)
	}
}

func TestPulsarBeforeReveal(t *testing.T) {
	m, f := pulsarFrame(t, Options{}, 50, 3)
	b := m.barBox
	found := false
	for x := b.Min.X; x < b.Max.X && !found; x++ {
		if c := f.at(x, b.Min.Y); c != nil && (c.ch == '━' || c.ch == '─') {
			found = true
		}
	}
	if !found {
		t.Error("до раскрытия в строке бара нет обычного бара")
	}
}

func TestPulsarMini(t *testing.T) {
	disorder := Track{Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209}
	m := New(Options{SelfPlay: true})
	m.SetSize(60, 12)
	m.JumpPlay([]Track{disorder}, 0, 50)
	steps(m, 10)
	if !m.geom().mini {
		t.Fatal("60x12 не мини-раскладка")
	}
	f := m.render()
	b := m.barBox
	found := false
	for x := b.Min.X; x < b.Max.X && !found; x++ {
		if c := f.at(x, b.Min.Y); c != nil && (c.ch == '━' || c.ch == '─') {
			found = true
		}
	}
	if !found {
		t.Error("в мини в строке бара нет обычного бара")
	}
	for row := b.Min.Y - 1; row <= b.Min.Y; row++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c, dot := pulsarCell(f, x, row); c != nil && dot {
				t.Errorf("mini (%d, %d): точка брайля %04X", x, row, c.ch)
			}
		}
	}
}

func TestPulsarNoEggs(t *testing.T) {
	m, f := pulsarFrame(t, Options{NoEggs: true}, 50, 10)
	b := m.barBox
	found := false
	for x := b.Min.X; x < b.Max.X && !found; x++ {
		if c := f.at(x, b.Min.Y); c != nil && (c.ch == '━' || c.ch == '─') {
			found = true
		}
	}
	if !found {
		t.Error("NoEggs: в строке бара нет ━/─")
	}
	for row := b.Min.Y - 2; row <= b.Min.Y; row++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c, dot := pulsarCell(f, x, row); c != nil && dot {
				t.Errorf("NoEggs (%d, %d): точка брайля %04X", x, row, c.ch)
			}
		}
	}
}

func TestPulsarDeterminism(t *testing.T) {
	disorder := Track{Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209}
	run := func() string {
		m := New(Options{SelfPlay: true})
		m.SetSize(160, 45)
		m.JumpPlay([]Track{disorder}, 0, 50)
		steps(m, 10)
		return m.View()
	}
	first, second := run(), run()
	if first != second {
		t.Error("два прогона дают разные кадры")
	}
}

func TestPulsarSizes(t *testing.T) {
	disorder := Track{Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209}
	for _, size := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 104.5, 194} {
			for _, secs := range []float64{1, 9} {
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Errorf("%dx%d pos %v secs %v: паника %v", size[0], size[1], pos, secs, p)
						}
					}()
					m := New(Options{SelfPlay: true})
					m.SetSize(size[0], size[1])
					m.JumpPlay([]Track{disorder}, 0, pos)
					steps(m, secs)
					m.render()
				}()
			}
		}
	}
}
