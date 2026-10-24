package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func rowText(f *fb, y, x, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
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
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
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
