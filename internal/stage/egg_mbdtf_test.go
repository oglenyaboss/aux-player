package stage

import (
	"image"
	"image/color"
	"math"
	"testing"
)

var mbdtfFantasy = Track{Artist: "Kanye West", Title: "Dark Fantasy", Album: "My Beautiful Dark Twisted Fantasy", Dur: 281}

func mbdtfNear(c, want col, tol float64) bool {
	return math.Abs(c.r-want.r) <= tol && math.Abs(c.g-want.g) <= tol && math.Abs(c.b-want.b) <= tol
}

// G1: the rule fires on the album, the deluxe edition folds to the same album,
// another album or another artist stays plain.
func TestMbdtfTrigger(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(mbdtfFantasy)
	if e == nil {
		t.Fatal("egg nil")
	}
	if e.theme != &mbdtfTheme || e.over == nil || e.knob != '♛' {
		over := false
		if e.over != nil {
			over = true
		}
		t.Errorf("egg: theme %+v, over %v, knob %q", e.theme != nil, over, e.knob)
	}
	luxe := Track{Artist: "Kanye West", Title: "Devil in a New Dress", Album: "My Beautiful Dark Twisted Fantasy (Deluxe Edition)"}
	if e := m.eggOf(luxe); e == nil || e.theme != &mbdtfTheme {
		t.Error("deluxe edition не опознан")
	}
	if e := m.eggOf(Track{Artist: "Kanye West", Title: "New Slaves", Album: "Yeezus"}); e != nil && e.theme == &mbdtfTheme {
		t.Error("Yeezus подхватил тему MBDTF")
	}
	if e := m.eggOf(Track{Artist: "Kid Cudi", Title: "REVOFEV", Album: "My Beautiful Dark Twisted Fantasy"}); e != nil {
		t.Errorf("другой артист получил пасхалку: %+v", *e)
	}
}

// R5: the frame rings around the cover once the reveal is done.
func TestMbdtfFrame(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 8)
	g := m.geom()
	cases := []struct {
		x, y int
		want col
	}{
		{g.cx - 1, g.cy*2 + g.ch, hex(0xe2bd55)},
		{g.cx - 2, g.cy*2 + g.ch, hex(0x8a6a22)},
		{g.cx + g.cw, g.cy*2 + g.ch, hex(0xe2bd55)},
		{g.cx + g.cw/2, g.cy*2 - 2, hex(0x8a6a22)},
	}
	for _, tc := range cases {
		if got := f.pixelAt(tc.x, tc.y); !mbdtfNear(got, tc.want, 0.02) {
			t.Errorf("pixel(%d,%d) = %s, want %s", tc.x, tc.y, got.css(), tc.want.css())
		}
	}
}

// R5: before the reveal there is no frame.
func TestMbdtfFrameBeforeReveal(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 3)
	g := m.geom()
	if got := f.pixelAt(g.cx-1, g.cy*2+g.ch); mbdtfNear(got, hex(0xe2bd55), 0.02) {
		t.Errorf("до раскрытия рама уже стоит: %s", got.css())
	}
}

// R3: the mosaic samples around with own cover: a clean pixel at 2 s, a block of four at 40 s.
func TestMbdtfMosaicBreath(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			if x < 60 {
				img.Set(x, y, color.Black)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	m := New(Options{SelfPlay: true})
	m.SetSize(160, 45)
	m.SetCover(mbdtfFantasy.Key(), img)
	m.JumpPlay([]Track{mbdtfFantasy}, 0, 0)
	steps(m, 2)
	f := m.render()
	g := m.geom()
	if c := f.pixelAt(g.cx+27, g.cy*2+10); c.lum() <= 0.8 {
		t.Errorf("на 2 с пиксель не чистый: lum %.2f, want > 0.8", c.lum())
	}
	m2 := New(Options{SelfPlay: true})
	m2.SetSize(160, 45)
	m2.SetCover(mbdtfFantasy.Key(), img)
	m2.JumpPlay([]Track{mbdtfFantasy}, 0, 0)
	steps(m2, 40)
	f2 := m2.render()
	g2 := m2.geom()
	if c := f2.pixelAt(g2.cx+27, g2.cy*2+10); c.lum() >= 0.2 {
		t.Errorf("на 40 с пиксель не замозаичен: lum %.2f, want < 0.2", c.lum())
	}
}

// R4: the title ink is gold at every lit pixel of the title box.
func TestMbdtfGoldTitle(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 2)
	if m.titleBox.Empty() {
		t.Fatal("titleBox пустой")
	}
	for py := m.titleBox.Min.Y; py < m.titleBox.Max.Y; py++ {
		for px := m.titleBox.Min.X; px < m.titleBox.Max.X; px++ {
			if c := f.pixelAt(px, py); c.lum() > 0.3 && c.r <= c.b+0.25 {
				t.Fatalf("пиксель заголовка (%d,%d) не золото: %s", px, py, c.css())
			}
		}
	}
}

// R4: the bar carries the crown.
func TestMbdtfKnob(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	for x := 0; x < m.w; x++ {
		if p := f.at(x, m.barBox.Min.Y); p != nil && p.ch == '♛' {
			return
		}
	}
	t.Error("в строке бара нет ♛")
}

// R2: the wall reads dark red on both counts.
func TestMbdtfWall(t *testing.T) {
	_, f := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	p := f.at(2, 40)
	if p == nil {
		t.Fatal("ячейки (2,40) нет")
	}
	if p.bg.r <= 2*p.bg.g || p.bg.r <= 2*p.bg.b {
		t.Errorf("стена не красная: %s", p.bg.css())
	}
}

// G2: NoEggs takes the frame and the crown away.
func TestMbdtfNoEggs(t *testing.T) {
	m, f := eggTrackFrame(t, Options{NoEggs: true}, mbdtfFantasy, 20, 9)
	g := m.geom()
	if got := f.pixelAt(g.cx-1, g.cy*2+g.ch); mbdtfNear(got, hex(0xe2bd55), 0.02) {
		t.Errorf("NoEggs рисует раму: %s", got.css())
	}
	for x := 0; x < m.w; x++ {
		if p := f.at(x, m.barBox.Min.Y); p != nil && p.ch == '♛' {
			t.Fatal("NoEggs ставит ♛")
		}
	}
}

// G3: two runs of the same track render identically.
func TestMbdtfDeterminism(t *testing.T) {
	m1, _ := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	m2, _ := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	if m1.View() != m2.View() {
		t.Error("два одинаковых прогона дают разные кадры")
	}
}

// G4: the egg survives every window size, early and late positions.
func TestMbdtfSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range []float64{0, 7, 281 * 0.5, 266} {
			for _, secs := range []float64{1, 9} {
				m := New(Options{SelfPlay: true})
				m.SetSize(sz[0], sz[1])
				m.JumpPlay([]Track{mbdtfFantasy}, 0, pos)
				steps(m, secs)
				m.render()
			}
		}
	}
}
