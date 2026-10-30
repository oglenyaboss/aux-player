package stage

import (
	"image"
	"image/color"
	"math"
	"strings"
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

// mbdtfCensored is a stand-in for the Apple cover: red canvas, and in the mosaic box
// an 8×8 grid where each cell has its own colour.
func mbdtfCensored() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := range 200 {
		for x := range 200 {
			u, v := float64(x)/200, float64(y)/200
			c := color.RGBA{238, 40, 67, 255}
			if u >= mbdtfMosaicU0 && u < mbdtfMosaicU1 && v >= mbdtfMosaicU0 && v < mbdtfMosaicU1 {
				i := int((u - mbdtfMosaicU0) / (mbdtfMosaicU1 - mbdtfMosaicU0) * 8)
				j := int((v - mbdtfMosaicU0) / (mbdtfMosaicU1 - mbdtfMosaicU0) * 8)
				c = color.RGBA{uint8(i * 32), uint8(j * 32), 200, 255} //nolint:gosec // G115: i and j are cell indexes in [0,8)
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func mbdtfWallFrame(t *testing.T, w, h int, pos, secs float64) (*Model, *fb) {
	t.Helper()
	m := New(Options{SelfPlay: true})
	m.SetSize(w, h)
	m.SetCover(mbdtfFantasy.Key(), mbdtfCensored())
	m.JumpPlay([]Track{mbdtfFantasy}, 0, pos)
	steps(m, secs)
	return m, m.render()
}

// The wall takes the screen in landscape; portrait and narrow windows keep the player.
func TestMbdtfLayout(t *testing.T) {
	m, _ := mbdtfWallFrame(t, 160, 45, 20, 2)
	if g := m.geom(); !g.noCover || !g.noQueue {
		t.Errorf("160x45: noCover %v noQueue %v, ждём стену", g.noCover, g.noQueue)
	}
	m, _ = mbdtfWallFrame(t, 80, 40, 20, 2)
	if g := m.geom(); g.noCover {
		t.Errorf("80x40: стена вместо обычного плеера")
	}
}

// Every cell of the blown-up mosaic shows its own cell of the cover.
func TestMbdtfMosaic(t *testing.T) {
	m, f := mbdtfWallFrame(t, 160, 45, 20, 2)
	b, _ := mbdtfPlan(m)
	if b.cell < 4 {
		t.Fatalf("клетка мозаики %d пикселей, ждём крупную", b.cell)
	}
	for j := range 8 {
		for i := range 8 {
			c := f.pixelAt(b.mx+i*b.cell+b.cell/2, b.my+j*b.cell+b.cell/2)
			want := col{float64(i*32) / 255, float64(j*32) / 255, 200.0 / 255}
			if !mbdtfNear(c, want, 0.03) {
				t.Errorf("клетка (%d,%d) = %s, ждём %s", i, j, c.css(), want.css())
			}
		}
	}
}

// The mosaic falls in block by block at the start of the track.
func TestMbdtfMosaicEntrance(t *testing.T) {
	m, f := mbdtfWallFrame(t, 160, 45, 20, 0.6)
	b, _ := mbdtfPlan(m)
	empty := 0
	for j := range 8 {
		for i := range 8 {
			if c := f.pixelAt(b.mx+i*b.cell+b.cell/2, b.my+j*b.cell+b.cell/2); c.b < 0.5 {
				empty++
			}
		}
	}
	if empty == 0 || empty == 64 {
		t.Errorf("на 0.6 с пустых клеток %d из 64, ждём часть", empty)
	}
}

// Each track hangs its own painting: the ballerina for Runaway, the crown and the sword
// for POWER and All of the Lights, the censored mosaic for Dark Fantasy.
func TestMbdtfHung(t *testing.T) {
	cases := map[string]string{
		"Dark Fantasy":                  "",
		"POWER":                         "crown",
		"Runaway (feat. Pusha T)":       "ballerina",
		"All of the Lights":             "sword",
		"All of the Lights (Interlude)": "sword",
		"Monster (feat. Jay-Z, Rick Ross, Nicki Minaj & Bon Iver)": "mouth",
	}
	for title, want := range cases {
		if got := mbdtfPainting(Track{Title: title}); got != want {
			t.Errorf("%s: картина %q, ждём %q", title, got, want)
		}
	}
	seen := map[string]bool{}
	for _, title := range []string{"Gorgeous", "So Appalled", "Devil in a New Dress", "Hell of a Life", "Blame Game", "Lost in the World", "Who Will Survive in America", "See Me Now"} {
		p := mbdtfPainting(Track{Title: title})
		if p == "ballerina" {
			t.Errorf("%s: балерина не только у Runaway", title)
		}
		if p != mbdtfPainting(Track{Title: title}) {
			t.Errorf("%s: картина плавает", title)
		}
		seen[p] = true
	}
	if len(seen) < 3 {
		t.Errorf("остальные треки висят на %d картинах, ждём разнообразие", len(seen))
	}
}

// The painting fills the frame: the ballerina stands on green, the crown under a blue sky.
func TestMbdtfPainting(t *testing.T) {
	cases := []struct {
		title string
		ok    func(c col) bool
	}{
		{"Runaway", func(c col) bool { return c.g > c.r && c.g > c.b }},
		{"POWER", func(c col) bool { return c.b > c.r+0.1 }},
	}
	for _, tc := range cases {
		tr := mbdtfFantasy
		tr.Title = tc.title
		m := New(Options{SelfPlay: true})
		m.SetSize(160, 45)
		m.SetCover(tr.Key(), mbdtfCensored())
		m.JumpPlay([]Track{tr}, 0, 20)
		steps(m, 2)
		f := m.render()
		b, _ := mbdtfPlan(m)
		if c := f.pixelAt(b.mx+b.ms/10, b.my+b.ms/10); !tc.ok(c) {
			t.Errorf("%s: угол картины %s", tc.title, c.css())
		}
	}
}

// The frame gilds clockwise from the top left as the track plays.
func TestMbdtfGilding(t *testing.T) {
	gold := func(c col) bool { return mbdtfNear(c, mbdtfGold, 0.02) }
	cases := []struct {
		pos                  float64
		top, right, bot, lft bool
	}{
		{0, false, false, false, false},
		{281 * 0.4, true, true, false, false},
		{281 * 0.99, true, true, true, true},
	}
	for _, tc := range cases {
		m, f := mbdtfWallFrame(t, 160, 45, tc.pos, 2)
		b, _ := mbdtfPlan(m)
		mid := b.ms / 2
		got := []bool{
			gold(f.pixelAt(b.mx+mid/2, b.my-1)),
			gold(f.pixelAt(b.mx+b.ms, b.my+mid)),
			gold(f.pixelAt(b.mx+mid, b.my+b.ms)),
			gold(f.pixelAt(b.mx-1, b.my+mid)),
		}
		want := []bool{tc.top, tc.right, tc.bot, tc.lft}
		for k := range got {
			if got[k] != want[k] {
				t.Errorf("на %.0f с сторона %d золотая %v, ждём %v", m.pos, k, got[k], want[k])
			}
		}
	}
}

// The wall label carries the track, the artist, the painter and the time.
func TestMbdtfLabel(t *testing.T) {
	m, f := mbdtfWallFrame(t, 160, 45, 62, 2)
	b, _ := mbdtfPlan(m)
	screen := stripANSI(f.String())
	for _, s := range []string{"Dark Fantasy", "Kanye West", mbdtfCredit, "1:03 / 4:41"} {
		if !strings.Contains(screen, s) {
			t.Errorf("на табличке нет %q", s)
		}
	}
	if c := f.at(b.lx, b.ly); c == nil || !mbdtfNear(c.bg, mbdtfCard, 0.02) {
		t.Errorf("табличка не светлая")
	}
	if b.lx <= b.mx+b.ms {
		t.Errorf("табличка (%d) не справа от картины (%d)", b.lx, b.mx+b.ms)
	}
}

// The wall is the red canvas of the cover.
func TestMbdtfCanvas(t *testing.T) {
	_, f := mbdtfWallFrame(t, 160, 45, 20, 2)
	for _, p := range [][2]int{{2, 20}, {150, 40}, {80, 1}} {
		if c := f.at(p[0], p[1]).bg; c.r <= 2*c.g || c.r < 0.5 {
			t.Errorf("холст в (%d,%d) не красный: %s", p[0], p[1], c.css())
		}
	}
}

// Narrow windows: the usual player, the cover in a gold frame after the reveal.
func TestMbdtfFrameFallback(t *testing.T) {
	m, f := mbdtfWallFrame(t, 80, 40, 20, 8)
	g := m.geom()
	cases := []struct {
		x, y int
		want col
	}{
		{g.cx - 1, g.cy*2 + g.ch, hex(0xe2bd55)},
		{g.cx - 2, g.cy*2 + g.ch, hex(0x8a6a22)},
		{g.cx + g.cw, g.cy*2 + g.ch, hex(0xe2bd55)},
	}
	for _, tc := range cases {
		if got := f.pixelAt(tc.x, tc.y); !mbdtfNear(got, tc.want, 0.02) {
			t.Errorf("pixel(%d,%d) = %s, want %s", tc.x, tc.y, got.css(), tc.want.css())
		}
	}
	m, f = mbdtfWallFrame(t, 80, 40, 20, 3)
	g = m.geom()
	if got := f.pixelAt(g.cx-1, g.cy*2+g.ch); mbdtfNear(got, hex(0xe2bd55), 0.02) {
		t.Errorf("до раскрытия рама уже стоит: %s", got.css())
	}
}

// G2: NoEggs keeps the usual player, without the frame and the crown.
func TestMbdtfNoEggs(t *testing.T) {
	m, f := eggTrackFrame(t, Options{NoEggs: true}, mbdtfFantasy, 20, 9)
	g := m.geom()
	if g.noCover {
		t.Fatal("NoEggs вешает стену")
	}
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
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}, {200, 20}, {300, 80}} {
		for _, pos := range []float64{0, 7, 281 * 0.5, 266, 281} {
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

var mbdtfRunaway = Track{Artist: "Kanye West", Title: "Runaway (feat. Pusha T)", Album: "My Beautiful Dark Twisted Fantasy", Dur: 548}

// mbdtfDancerCells counts black cells of the set: outside the frame and the label.
func mbdtfDancerCells(m *Model, f *fb) int {
	b, _ := mbdtfPlan(m)
	n := 0
	for py := 0; py < 2*m.h; py++ {
		for px := 0; px < m.w; px++ {
			inFrame := px >= b.mx-2 && px < b.mx+b.ms+2 && py >= b.my-2 && py < b.my+b.ms+2
			inLabel := px >= b.lx && px < b.lx+b.lw && py/2 >= b.ly && py/2 < b.ly+b.lh
			if !inFrame && !inLabel && f.pixelAt(px, py).lum() < 0.1 {
				n++
			}
		}
	}
	return n
}

// Runaway plays on the white set of the film, ballerinas in black tutus cross it.
func TestMbdtfRunawaySet(t *testing.T) {
	m := New(Options{SelfPlay: true})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{mbdtfRunaway}, 0, 200)
	steps(m, 3)
	f := m.render()
	if c := f.at(80, 1).bg; c.lum() < 0.75 || c.r > c.g+0.05 {
		t.Errorf("сет не белый: %s", c.css())
	}
	if n := mbdtfDancerCells(m, f); n < 40 {
		t.Errorf("на сете %d чёрных пикселей, ждём балерин", n)
	}
	b, _ := mbdtfPlan(m)
	if c := f.at(b.lx, b.ly).bg; c.lum() > 0.1 {
		t.Errorf("табличка на белом не чёрная: %s", c.css())
	}
	m, f = mbdtfWallFrame(t, 160, 45, 20, 3)
	if n := mbdtfDancerCells(m, f); n != 0 {
		t.Errorf("Dark Fantasy: на стене %d чёрных пикселей, балерины только в Runaway", n)
	}
}

// The ballerinas move with the playback and freeze on pause.
func TestMbdtfRunawayFreeze(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{mbdtfRunaway}, 0, 200)
	m.Now(mbdtfRunaway.Key(), 200, true)
	steps(m, 3)
	a := m.render().String()
	steps(m, 1)
	if m.render().String() == a {
		t.Fatal("балерины стоят во время игры")
	}
	m.Now(mbdtfRunaway.Key(), m.pos, false)
	steps(m, 0.2)
	b := m.render().String()
	steps(m, 1)
	if m.render().String() != b {
		t.Errorf("на паузе кадр меняется, ждём замерших балерин")
	}
}
