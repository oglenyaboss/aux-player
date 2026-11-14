package stage

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// mlBlondeRows is the host answer of the search for "bl": twelve songs, then the
// album and the playlist.
func mlBlondeRows() []MusicItem {
	rows := make([]MusicItem, 0, 14)
	for i := 1; i <= 12; i++ {
		rows = append(rows, musicSong(fmt.Sprintf("s%02d", i), "Frank Ocean", fmt.Sprintf("Song %d", i), 200))
	}
	rows = append(rows,
		MusicItem{ID: "alb-blonde", Kind: MusicAlbum, Title: "Blonde", Artist: "Frank Ocean", Sub: "2016 · альбом", Count: 17},
		MusicItem{ID: "pl-chill", Kind: MusicPlaylist, Title: "Chill", Artist: "Aux", Count: 20},
	)
	return rows
}

// mlSearchSetup opens the palette in Russian and lands the rows of the query.
func mlSearchSetup(t *testing.T, query string) (*Model, *recorder) {
	t.Helper()
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlType(m, query)
	steps(m, 0.4)
	m.MusicRows(mlLastLoad(t, r).Gen, mlBlondeRows(), "")
	return m, r
}

func mlSearchSongs(lines []mlLine) int {
	n := 0
	for _, l := range lines {
		if l.kind == mlSong && !l.hero && l.it.Kind == MusicSong && strings.HasPrefix(l.it.Title, "Song ") {
			n++
		}
	}
	return n
}

func mlSearchHead(lines []mlLine, text string) bool {
	for _, l := range lines {
		if l.kind == mlHead && l.text == text {
			return true
		}
	}
	return false
}

// A1: a search opens on the top result, an album of the query, with its cover.
func TestPaletteSearchTopResult(t *testing.T) {
	m, _ := mlSearchSetup(t, "bl")
	head := mlRowOf(m, "лучшее совпадение")
	hero := mlRowOf(m, "Blonde ›")
	if head < 0 || hero < 0 {
		t.Fatalf("top result header=%d hero=%d, want both on screen:\n%s", head, hero, stripANSI(m.View()))
	}
	if hero != head+1 {
		t.Errorf("the hero starts at row %d, want the row under the header %d", hero, head+1)
	}
	if got := mlMarked(m); !strings.Contains(got, "Blonde") {
		t.Errorf("selection = %q, want the hero", got)
	}
	f := m.render()
	px, _, _, _ := m.mlPaletteGeom()
	pix := 0
	for yy := hero; yy < hero+4; yy++ {
		for xx := px + 1 + 2; xx <= px+1+9; xx++ {
			if c := f.at(xx, yy); c != nil && c.pix {
				pix++
			}
		}
	}
	if pix != 32 {
		t.Errorf("the hero cover covers %d cells of the 8×4, want 32", pix)
	}
	if next := rowText(f, hero+1, px+1, m.w-px-1); strings.Contains(next, "Blonde ›") {
		t.Errorf("the hero title repeats on its second row: %q", next)
	}
}

// A2: the groups under the hero keep five songs and four collections, and the list
// ends with the show-more row of the hidden songs.
func TestPaletteSearchGroupsCapped(t *testing.T) {
	m, _ := mlSearchSetup(t, "bl")
	lines := m.mlLines()
	if got := mlSearchSongs(lines); got != 5 {
		t.Errorf("the songs group = %d rows, want the cap of 5", got)
	}
	if !mlSearchHead(lines, "песни") || !mlSearchHead(lines, "плейлисты") {
		t.Errorf("the groups lose their headers: %+v", lines)
	}
	if mlSearchHead(lines, "альбомы") {
		t.Errorf("the albums group is shown with only the hero in it")
	}
	chill := false
	for _, l := range lines {
		if l.kind == mlSong && l.it.Title == "Chill" {
			chill = true
		}
	}
	if !chill {
		t.Errorf("the playlists group does not show Chill")
	}
	if row := mlRowOf(m, "ещё 7 ›"); row < 0 {
		t.Errorf("no show-more row for the seven hidden songs:\n%s", stripANSI(m.View()))
	}
}

// A3: enter on the show-more row expands the group; typing again caps it back.
func TestPaletteSearchShowMore(t *testing.T) {
	m, _ := mlSearchSetup(t, "bl")
	for range 32 {
		if l, ok := m.mlSelected(); ok && l.kind == mlMore {
			break
		}
		m.Key("down", "")
	}
	m.render()
	steps(m, 0.5)
	if got := mlMarked(m); !strings.Contains(got, "ещё") {
		t.Fatalf("selection = %q, want the show-more row", got)
	}
	m.Key("enter", "")
	if got := mlSearchSongs(m.mlLines()); got != 12 {
		t.Errorf("the expanded groups show %d songs, want all 12", got)
	}
	if row := mlRowOf(m, "ещё"); row >= 0 {
		t.Errorf("the song group keeps a show-more row after expanding:\n%s", stripANSI(m.View()))
	}
	m.Key("x", "x")
	if got := mlSearchSongs(m.mlLines()); got != 5 {
		t.Errorf("the new query shows %d songs, want the cap of 5", got)
	}
	if row := mlRowOf(m, "ещё"); row < 0 {
		t.Errorf("the new query has no show-more row:\n%s", stripANSI(m.View()))
	}
}

// A4: an exact song title wins over the host order, and with no prefix match the
// first row is the hero.
func TestPaletteSearchHeroRules(t *testing.T) {
	m, _ := mlSearchSetup(t, "song 3")
	if mlRowOf(m, "лучшее совпадение") < 0 {
		t.Errorf("no top result header for a song query:\n%s", stripANSI(m.View()))
	}
	if got := mlMarked(m); !strings.Contains(got, "Song 3") {
		t.Errorf("selection = %q, want Song 3 as the hero", got)
	}

	m2, _ := mlSearchSetup(t, "zz")
	if mlRowOf(m2, "лучшее совпадение") < 0 {
		t.Errorf("no top result header without a prefix match:\n%s", stripANSI(m2.View()))
	}
	if got := mlMarked(m2); !strings.Contains(got, "Song 1") {
		t.Errorf("selection = %q, want the first row as the hero", got)
	}
}

// A5: every action of a row works on the hero: play, open, queue.
func TestPaletteSearchHeroActions(t *testing.T) {
	m, r := mlSearchSetup(t, "song 3")
	m.Key("enter", "")
	plays := recorded[MusicPlay](r)
	if len(plays) != 1 || len(plays[0].Items) != 1 || plays[0].Items[0].Title != "Song 3" || plays[0].Start != 0 {
		t.Fatalf("plays = %+v, want the one hero Song 3", r.acts)
	}

	m2, r2 := mlSearchSetup(t, "bl")
	m2.Key("enter", "")
	load := mlLastLoad(t, r2)
	if load.Parent == nil || load.Parent.Title != "Blonde" {
		t.Fatalf("load = %+v, want the tracks of Blonde", load)
	}

	m3, r3 := mlSearchSetup(t, "song 3")
	m3.Key("ctrl+e", "")
	apps := recorded[MusicAppend](r3)
	if len(apps) != 1 || apps[0].Item.Title != "Song 3" {
		t.Fatalf("appends = %+v, want the hero Song 3", r3.acts)
	}
}

// A6: the bar names the show-all key while the show-more row is selected.
func TestPaletteSearchShowAllCap(t *testing.T) {
	m, _ := mlSearchSetup(t, "bl")
	for range 32 {
		if l, ok := m.mlSelected(); ok && l.kind == mlMore {
			break
		}
		m.Key("down", "")
	}
	m.render()
	steps(m, 0.5)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "показать все") {
		t.Errorf("the bar of the show-more row has no show all:\n%s", frame)
	}
}

// A7: the grid scrolls under the highlight and comes back to the top on a new level.
func TestPaletteGridScrolls(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	albums := make([]MusicItem, 0, 20)
	for i := 1; i <= 20; i++ {
		albums = append(albums, musicAlbum(fmt.Sprintf("a%02d", i), fmt.Sprintf("Performer %02d", i), fmt.Sprintf("Album %02d", i), 10))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, albums, "")
	steps(m, 0.2)
	if cols := m.mlGridColsNow(); cols != 6 {
		t.Fatalf("columns = %d, want 6 at 160x45", cols)
	}
	for range 3 {
		m.Key("down", "")
	}
	if m.mu.list.sel != 18 {
		t.Fatalf("selection = %d after three downs, want 18", m.mu.list.sel)
	}
	f := m.render()
	px, py, _, _ := m.mlPaletteGeom()
	if row := mlRowOf(m, "Album 07"); row != py+3+mlTileH {
		t.Errorf("the first tile row starts at row %d, want Album 07 at %d", row, py+3+mlTileH)
	}
	if row := py + 3 + 2*mlTileRows + mlTileH; !strings.Contains(rowText(f, row, px, m.w-px), "Album 19") {
		t.Errorf("the selected Album 19 is not in the last visible tile row %d: %q", row, rowText(f, row, px, m.w-px))
	}
	if c := f.at(px+m.mu.tileX, py+m.mu.tileY); c == nil || c.ch != '╭' {
		t.Errorf("the selected tile has no border at %d,%d", px+m.mu.tileX, py+m.mu.tileY)
	}
	m.Key("esc", "")
	mlOpenSection(t, m, MusicAlbums)
	steps(m, 0.1)
	if row := mlRowOf(m, "Album 01"); row != py+3+mlTileH {
		t.Errorf("the reopened grid starts at row %d, want Album 01 at %d", row, py+3+mlTileH)
	}
}

// A8: the grid keeps the selected tile bright and quiets the rest toward the panel.
func TestPaletteGridFocus(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	albums := make([]MusicItem, 0, 6)
	for i := 1; i <= 6; i++ {
		albums = append(albums, musicAlbum(fmt.Sprintf("a%02d", i), fmt.Sprintf("Performer %02d", i), fmt.Sprintf("Album %02d", i), 10))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, albums, "")
	steps(m, 3)
	f := m.render()
	x1, y1 := mlPosOf(m, "Album 01")
	x2, y2 := mlPosOf(m, "Album 02")
	if x1 < 0 || x2 < 0 {
		t.Fatalf("titles are not on screen: %d,%d %d,%d", x1, y1, x2, y2)
	}
	bg := m.th.bg.lerp(m.th.text, 0.07)
	sel, other := f.at(x1, y1), f.at(x2, y2)
	if sel == nil || sel.fg != m.th.a1 {
		t.Errorf("the selected title is not in the accent color")
	}
	dist := func(a, b col) float64 {
		return math.Sqrt((a.r-b.r)*(a.r-b.r) + (a.g-b.g)*(a.g-b.g) + (a.b-b.b)*(a.b-b.b))
	}
	if other == nil || dist(other.fg, bg) >= dist(sel.fg, bg) {
		t.Errorf("the rest of the grid is not faded toward the panel background")
	}
}

// A9: the list menu opens at the title column under the row, and above the last row.
func TestPaletteListMenuAnchor(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	m.Key("ctrl+k", "")
	if !m.mu.menu.open {
		t.Fatal("ctrl+k did not open the menu")
	}
	f := m.render()
	px, py, _, _ := m.mlPaletteGeom()
	rowY := py + m.mu.rowY
	wantX := px + 1 + 7
	top := rowY + m.mlRowH()
	if c := f.at(wantX, top); c == nil || c.ch != '╭' {
		t.Errorf("the menu does not start at the title column %d,%d: %q", wantX, top, string(c.ch))
	}
	if top <= rowY {
		t.Errorf("the menu top %d is not below the row %d", top, rowY)
	}

	m2, r2 := paletteRU(t, 160, 45)
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	m2.Key("enter", "")
	songs := make([]MusicItem, 0, 20)
	for i := range 20 {
		songs = append(songs, musicSong(fmt.Sprintf("s%02d", i), "Alpha", fmt.Sprintf("Track %02d", i), 200))
	}
	m2.MusicRows(mlLastLoad(t, r2).Gen, songs, "")
	m2.Key("end", "")
	m2.Key("ctrl+k", "")
	f = m2.render()
	h := mlPanelRows(t, m2)
	menuH := len(m2.mlMenuItems()) + 2
	localRow := m2.mu.rowY
	localTop := localRow + m2.mlRowH()
	if localTop+menuH > h-2 {
		localTop = localRow - menuH
	}
	localTop = max(1, min(localTop, h-menuH-1))
	if c := f.at(px+1+7, py+localTop); c == nil || c.ch != '╭' {
		t.Errorf("the menu of the last row does not start at %d,%d: %q", px+1+7, py+localTop, string(c.ch))
	}
	if localTop >= localRow {
		t.Errorf("the menu of the last row (%d) is not above the row (%d)", localTop, localRow)
	}
	if py+localTop+menuH-1 >= py+h-1 {
		t.Errorf("the menu of the last row reaches the panel border")
	}
}
