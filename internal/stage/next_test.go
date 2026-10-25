package stage

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
)

func pk(i int) Track {
	return Track{ID: fmt.Sprintf("n%d", i), Artist: "N", Title: fmt.Sprintf("Next%d", i), Dur: 120}
}

// playStart is the common opening of every next-portion case: the player on three tracks.
func playStart(t *testing.T) *Model {
	t.Helper()
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	steps(m, 1)
	return m
}

func nextRow(m *Model) string {
	f := m.render()
	g := m.geom()
	return strings.TrimSpace(rowText(f, m.barBox.Min.Y+1, g.px, g.pw))
}

// R1: nothing on the next-portion line until the agent says it is picking.
func TestNextOffByDefault(t *testing.T) {
	m := playStart(t)
	m.Progress("читаю про Blonde")
	steps(m, 0.3)
	if row := nextRow(m); row != "" {
		t.Errorf("строка под полосой %q, ждём пустую", row)
	}
}

// R2: the gathering line names the portion size and carries the agent's words.
func TestNextGatheringLine(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	steps(m, 0.3)
	if row := nextRow(m); row != "собираю следующие 5" {
		t.Errorf("строка %q, ждём «собираю следующие 5»", row)
	}
	m.Progress("читаю про Blonde")
	if row := nextRow(m); row != "собираю следующие 5 · читаю про Blonde" {
		t.Errorf("строка %q, ждём слова агента через ·", row)
	}
}

// R2: a reply recolours the gathering line and says what is being rebuilt.
func TestNextReplyLine(t *testing.T) {
	m := playStart(t)
	m.NextGathering("потише")
	if row := nextRow(m); row != "учитываю «потише», пересобираю следующие 5" {
		t.Errorf("строка %q, ждём «учитываю «потише», пересобираю следующие 5»", row)
	}
	m.Progress("ищу похожее")
	if row := nextRow(m); row != "учитываю «потише» · ищу похожее" {
		t.Errorf("строка %q, ждём «учитываю «потише» · ищу похожее»", row)
	}
}

// R2/R4: the ready line counts the tracks of the gathered portion.
func TestNextReadyLine(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextReady([]Track{pk(1), pk(2)})
	steps(m, 0.5)
	if row := nextRow(m); row != "✓ следующие 2 готовы" {
		t.Errorf("строка %q, ждём «✓ следующие 2 готовы»", row)
	}
}

// R2: the failed line says the portion did not come together.
func TestNextFailedLine(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextFailed()
	if row := nextRow(m); row != "✗ следующие не собрались" {
		t.Errorf("строка %q, ждём «✗ следующие не собрались»", row)
	}
}

// R2: a long line is cut at the column width.
func TestNextLineTrunc(t *testing.T) {
	m := playStart(t)
	m.SetSize(80, 24)
	m.NextGathering("")
	m.Progress("читаю про очень длинное название альбома")
	f := m.render()
	g := m.geom()
	barY := m.barBox.Min.Y
	row := strings.TrimSpace(rowText(f, barY+1, g.px, g.pw))
	if !strings.HasPrefix(row, "собираю следующие 5 ·") || !strings.HasSuffix(row, "…") {
		t.Fatalf("строка %q, ждём начало «собираю следующие 5 ·» и обрезку многоточием", row)
	}
	if c := f.at(g.px+g.pw, barY+1); c != nil && c.ch != ' ' && strings.ContainsRune(row, c.ch) {
		t.Errorf("клетка (%d,%d) за строкой %q держит руну %q", g.px+g.pw, barY+1, row, c.ch)
	}
}

// R3: the strip has one slot per portion seat, fills in arrival order and ignores
// duplicates and extras.
func TestNextStripCount(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	if s := stripANSI(m.View()); !strings.Contains(s, "0 из 5") {
		t.Fatalf("кадр без «0 из 5»")
	}
	m.NextPick(pk(1))
	m.NextPick(pk(1))
	m.NextPick(pk(2))
	if s := stripANSI(m.View()); !strings.Contains(s, "2 из 5") {
		t.Fatalf("кадр без «2 из 5»: дубль не игнорируется")
	}
	for i := 3; i <= 6; i++ {
		m.NextPick(pk(i))
	}
	if s := stripANSI(m.View()); !strings.Contains(s, "5 из 5") {
		t.Fatalf("кадр без «5 из 5»: лишний трек не игнорируется")
	}
}

// R3: a picked track paints its thumbnail into the strip slot, dimmed.
func TestNextStripThumb(t *testing.T) {
	m := playStart(t)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 255, A: 255}
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, red)
		}
	}
	m.SetCover("n1", img)
	m.NextGathering("")
	m.NextPick(pk(1))
	steps(m, 1)
	f := m.render()
	g := m.geom()
	y, lx := -1, -1
	for yy := 0; yy+1 < m.h && y < 0; yy++ {
		for xx := 0; xx+6 <= m.w; xx++ {
			if rowText(f, yy, xx, 6) == "1 из 5" {
				y, lx = yy, xx
				break
			}
		}
	}
	if y < 0 {
		t.Fatalf("подпись «1 из 5» не найдена в кадре")
	}
	sx := lx - 1 - 5*5
	if sx != g.px {
		t.Fatalf("лента начинается в колонке %d, ждём %d", sx, g.px)
	}
	for yy := y; yy <= y+1; yy++ {
		for xx := sx; xx < sx+4; xx++ {
			if c := f.at(xx, yy); c == nil || !c.pix {
				t.Fatalf("миниатюра: клетка (%d,%d) не пиксель", xx, yy)
			}
		}
	}
	if c := f.pixelAt(sx, y*2); c.r <= c.g {
		t.Errorf("пиксель %v, ждём красный", c)
	}
}

// R4: the ready portion keeps its thumbnails and loses the count.
func TestNextReadyNoCount(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextPick(pk(1))
	m.NextReady([]Track{pk(1), pk(2)})
	steps(m, 0.5)
	if s := stripANSI(m.View()); strings.Contains(s, " из 5") {
		t.Errorf("в кадре осталась подпись счёта: %q", s)
	}
}

// R5: gathering again starts the count from zero and dissolves the shown tracks.
func TestNextRegatherResets(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextPick(pk(1))
	m.NextPick(pk(2))
	m.NextGathering("тише")
	if s := stripANSI(m.View()); !strings.Contains(s, "0 из 5") {
		t.Fatalf("кадр без «0 из 5» после нового сбора")
	}
	m.NextPick(pk(3))
	if s := stripANSI(m.View()); !strings.Contains(s, "1 из 5") {
		t.Fatalf("кадр без «1 из 5» после первого пика")
	}
}

// R6: the strip always finds room: a long queue gives up its last two rows.
func TestNextStripFitsLongQueue(t *testing.T) {
	m := New(Options{Slots: 5})
	m.SetSize(80, 24)
	var tr []Track
	for i := 0; i < 5; i++ {
		tr = append(tr, Track{ID: fmt.Sprintf("q%d", i), Artist: "Q", Title: fmt.Sprintf("T%d", i), Dur: 120})
	}
	m.JumpPlay(tr, 0, 0)
	steps(m, 1)
	if s := stripANSI(m.View()); strings.Contains(s, " из 5") {
		t.Fatalf("лента нарисована до сбора")
	}
	m.NextGathering("")
	s := stripANSI(m.View())
	if !strings.Contains(s, "0 из 5") {
		t.Fatalf("кадр без ленты «0 из 5»")
	}
	if !strings.Contains(s, "T1") {
		t.Errorf("длинная очередь потеряла строку T1")
	}
}

// R7: clearing the next portion takes the line and the strip away.
func TestNextClear(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextPick(pk(1))
	m.NextClear()
	steps(m, 0.5)
	if row := nextRow(m); row != "" {
		t.Errorf("строка %q, ждём пустую", row)
	}
	if s := stripANSI(m.View()); strings.Contains(s, " из 5") {
		t.Errorf("лента осталась в кадре")
	}
}

// R8: the mini layout draws no strip and does not panic.
func TestNextMini(t *testing.T) {
	m := playStart(t)
	m.SetSize(100, 12)
	m.NextGathering("")
	m.NextPick(pk(1))
	steps(m, 0.5)
	if s := stripANSI(m.View()); strings.Contains(s, " из 5") {
		t.Errorf("лента в mini-раскладке")
	}
}

// Trap: a pick that lands after the ready call is ignored.
func TestNextPickIgnoredWhenReady(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextReady([]Track{pk(1)})
	m.NextPick(pk(2))
	if row := nextRow(m); row != "✓ следующие 1 готовы" {
		t.Errorf("строка %q, ждём «✓ следующие 1 готовы»", row)
	}
}

// A short first portion trims the slots; the next portion is still the full size.
func TestNextSizeAfterShortPortion(t *testing.T) {
	m := playStart(t)
	m.slots = m.slots[:3]
	m.NextGathering("")
	if row := nextRow(m); row != "собираю следующие 5" {
		t.Errorf("строка %q, ждём «собираю следующие 5»", row)
	}
	for i := 0; i < 5; i++ {
		m.NextPick(pk(i))
	}
	tracks := make([]Track, 5)
	for i := range tracks {
		tracks[i] = pk(i)
	}
	m.NextReady(tracks)
	if row := nextRow(m); row != "✓ следующие 5 готовы" {
		t.Errorf("строка %q, ждём «✓ следующие 5 готовы»", row)
	}
}

// The failed line goes away by itself: no retry comes for this track.
func TestNextFailedFades(t *testing.T) {
	m := playStart(t)
	m.NextGathering("")
	m.NextFailed()
	steps(m, 8)
	if row := nextRow(m); row != "" {
		t.Errorf("строка %q через 8 с, ждём пустую", row)
	}
}
