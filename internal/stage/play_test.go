package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func rowText(f *fb, y, x, n int) string {
	var b strings.Builder
	for i := range n {
		c := f.at(x+i, y)
		if c == nil {
			break
		}
		b.WriteRune(c.ch)
	}
	return b.String()
}

// R1: the side layout caps the cover at 26 rows; smaller windows keep their own height.
func TestGeomCoverCap(t *testing.T) {
	m := New(Options{})
	m.SetSize(220, 60)
	if g := m.geom(); g.ch != 26 || g.cw != 52 {
		t.Errorf("220x60: ch=%d cw=%d, ждём 26/52", g.ch, g.cw)
	}
	m.SetSize(110, 32)
	if g := m.geom(); g.ch != 22 {
		t.Errorf("110x32: ch=%d, ждём 22", g.ch)
	}
	m.SetSize(160, 45)
	if g := m.geom(); g.ch != 26 {
		t.Errorf("160x45: ch=%d, ждём 26", g.ch)
	}
}

// R2: the next track's thumbnail is 4 cells wide and 2 rows tall, title on the first
// row, artist on the second.
func TestQueueThumb(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tr := threeTracks()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 255, A: 255}
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, red)
		}
	}
	m.SetCover(tr[1].Key(), img)
	m.JumpPlay(tr, 0, 0)
	steps(m, 3)
	f := m.render()
	g := m.geom()
	y := -1
	for yy := 0; yy+1 < m.h; yy++ {
		if rowText(f, yy, g.px+6, 3) == "Two" {
			y = yy
			break
		}
	}
	if y < 0 {
		t.Fatalf("строка Two не найдена в кадре")
	}
	for yy := y; yy <= y+1; yy++ {
		for xx := g.px; xx < g.px+4; xx++ {
			c := f.at(xx, yy)
			if c == nil || !c.pix {
				t.Fatalf("миниатюра: клетка (%d,%d) не пиксель", xx, yy)
			}
		}
	}
	if got := rowText(f, y+1, g.px+6, 4); got != "Beta" {
		t.Errorf("вторая строка %q, ждём Beta", got)
	}
	for xx := 0; xx+4 <= m.w; xx++ {
		if rowText(f, y, xx, 4) == "Beta" {
			t.Errorf("артист Beta на первой строке в колонке %d", xx)
		}
	}
}

// R3: the header shows the real queue length.
func TestHeaderCount(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 1, 0)
	steps(m, 1)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "2 / 3   aux") {
		t.Errorf("кадр без «2 / 3   aux»:\n%s", frame)
	}
}

// R4: an unknown track from the host does not push the position past the duration.
func TestPosClampUnknownTrack(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tk := Track{Artist: "A", Title: "T", Dur: 40}
	m.JumpPlay([]Track{tk}, 0, 0)
	m.Now(tk.Key(), 0, true)
	m.Now("LIBRARY-1", 5, true)
	steps(m, 45)
	if m.pos != 40 {
		t.Errorf("pos=%v, ждём 40", m.pos)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "0:40 / 0:40") {
		t.Errorf("кадр без «0:40 / 0:40»:\n%s", frame)
	}
}

// R5: a track with no duration neither panics nor advances on its own.
func TestZeroDur(t *testing.T) {
	m := New(Options{SelfPlay: true})
	m.SetSize(110, 32)
	q := []Track{
		{Artist: "A", Title: "First", Dur: 200},
		{Artist: "B", Title: "Zero", Dur: 0},
		{Artist: "C", Title: "Third", Dur: 180},
	}
	m.JumpPlay(q, 1, 0)
	steps(m, 1)
	if m.cur != 1 {
		t.Errorf("cur=%d, ждём 1", m.cur)
	}
	if frame := stripANSI(m.View()); strings.Contains(frame, "NaN") {
		t.Errorf("в кадре NaN:\n%s", frame)
	}
}

// R1: portrait windows at least 50 wide split into a cover on the left and text on
// the right; narrower ones keep the stacked layout.
func TestGeomSplit(t *testing.T) {
	m := New(Options{})
	for _, c := range []struct {
		w, h           int
		ch, cw, px, pw int
	}{
		{60, 60, 13, 26, 32, 25},
		{80, 60, 20, 40, 46, 31},
	} {
		m.SetSize(c.w, c.h)
		g := m.geom()
		if !g.split {
			t.Errorf("%dx%d: split=false, ждём split", c.w, c.h)
		}
		if g.ch != c.ch || g.cw != c.cw || g.px != c.px || g.pw != c.pw {
			t.Errorf("%dx%d: ch=%d cw=%d px=%d pw=%d, ждём %d/%d/%d/%d", c.w, c.h, g.ch, g.cw, g.px, g.pw, c.ch, c.cw, c.px, c.pw)
		}
	}
	m.SetSize(46, 60)
	if g := m.geom(); g.split || g.px != 3 {
		t.Errorf("46x60: split=%v px=%d, ждём стопку с px=3", g.split, g.px)
	}
	m.SetSize(110, 32)
	if g := m.geom(); g.split || g.ch != 22 {
		t.Errorf("110x32: split=%v ch=%d, ждём боковую с ch=22", g.split, g.ch)
	}
}

// R4: in the split player the artist and the time bar sit right of the cover, and the
// next track starts below the cover on the full width.
func TestSplitLayout(t *testing.T) {
	m := New(Options{})
	m.SetSize(60, 60)
	m.JumpPlay(threeTracks(), 0, 0)
	steps(m, 3)
	lines := strings.Split(stripANSI(m.View()), "\n")
	g := m.geom()
	inCover := func(y int) bool { return y >= g.cy && y < g.cy+g.ch }
	row := func(y int) []rune {
		if y < 0 || y >= len(lines) {
			return nil
		}
		return []rune(lines[y])
	}
	at := func(y, x int) string {
		r := row(y)
		if x < 0 || x > len(r) {
			return ""
		}
		return string(r[x:])
	}
	alpha, time := -1, -1
	for y := range lines {
		if strings.HasPrefix(at(y, g.px), "Alpha") {
			alpha = y
		}
		if strings.Contains(lines[y], "0:0") && strings.Contains(lines[y], "/ 2:00") {
			time = y
		}
	}
	if alpha < 0 {
		t.Fatalf("строка Alpha не найдена справа от обложки")
	}
	if !inCover(alpha) {
		t.Errorf("Alpha в строке %d, ждём внутри [%d,%d)", alpha, g.cy, g.cy+g.ch)
	}
	if time < 0 {
		t.Fatalf("строка времени не найдена")
	}
	if !inCover(time) {
		t.Errorf("полоска времени в строке %d, ждём внутри [%d,%d)", time, g.cy, g.cy+g.ch)
	}
	next := -1
	for y := range lines {
		if strings.HasPrefix(at(y, 3+6), "Two") {
			next = y
			break
		}
	}
	if next < 0 {
		t.Fatalf("строка Two не найдена")
	}
	if next < g.cy+g.ch {
		t.Errorf("Two в строке %d, ждём не выше %d", next, g.cy+g.ch)
	}
}

func TestPauseGoesDark(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	key := threeTracks()[0].Key()
	light := func(playing bool) float64 {
		m.Now(key, 1, playing)
		steps(m, 2)
		sum := 0.0
		for _, c := range m.render().c {
			sum += c.bg.r + c.bg.g + c.bg.b
		}
		return sum
	}
	on := light(true)
	off := light(false)
	again := light(true)
	if !(off < 0.35*on) {
		t.Fatalf("на паузе экран не потемнел: %v против %v", off, on)
	}
	if !(again > 0.9*on) {
		t.Fatalf("после паузы экран не вернулся: %v против %v", again, on)
	}
}
