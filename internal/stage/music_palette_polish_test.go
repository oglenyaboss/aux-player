package stage

import (
	"fmt"
	"strings"
	"testing"
)

// paletteRU is the palette in Russian at the given size.
func paletteRU(t *testing.T, w, h int) (*Model, *recorder) {
	t.Helper()
	prev := lang
	SetLang("ru")
	t.Cleanup(func() { SetLang(prev) })
	r := &recorder{}
	m := New(Options{OnAction: r.on, MusicStyle: "palette"})
	m.SetSize(w, h)
	return m, r
}

// mlPanelRows is the height of the panel box: its border rows and the rows between them.
func mlPanelRows(t *testing.T, m *Model) int {
	t.Helper()
	f := m.render()
	x, y, _, _ := m.mlPaletteGeom()
	if c := f.at(x, y); c == nil || c.ch != '╭' {
		t.Fatalf("no panel corner at %d,%d: %q", x, y, string(c.ch))
	}
	for yy := y + 1; yy < f.h; yy++ {
		if c := f.at(x, yy); c != nil && c.ch == '╰' {
			return yy - y + 1
		}
	}
	t.Fatalf("no panel bottom border under %d,%d", x, y)
	return 0
}

// A1: the counted noun takes the form of the number in both languages.
func TestCountOf(t *testing.T) {
	prev := lang
	t.Cleanup(func() { SetLang(prev) })
	SetLang("ru")
	for _, c := range []struct {
		n    int
		want string
	}{
		{1, "1 трек"}, {3, "3 трека"}, {11, "11 треков"}, {21, "21 трек"}, {112, "112 треков"},
	} {
		if got := countOf(c.n, trackForms, trackFormsEn); got != c.want {
			t.Errorf("countOf(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	SetLang("en")
	for _, c := range []struct {
		n    int
		want string
	}{
		{1, "1 track"}, {2, "2 tracks"},
	} {
		if got := countOf(c.n, trackForms, trackFormsEn); got != c.want {
			t.Errorf("countOf(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// A2: a root entry carries its chevron on the label line and the note under it.
func TestPaletteRootEntries(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.25)
	f := m.render()
	y := mlRowOf(m, "избранное")
	if y < 0 {
		t.Fatalf("no favorites entry in the root:\n%s", stripANSI(m.View()))
	}
	if row := rowText(f, y, 0, m.w); !strings.Contains(row, "›") {
		t.Errorf("the favorites row has no chevron: %q", row)
	}
	if row := rowText(f, y+1, 0, m.w); !strings.Contains(row, "любимые треки") {
		t.Errorf("the line under favorites = %q, want the description", row)
	}
	m.Key("enter", "")
	songs := make([]MusicItem, 0, 5)
	for i := range 5 {
		songs = append(songs, musicSong(fmt.Sprintf("s%d", i), "Alpha", fmt.Sprintf("Song %d", i+1), 200))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, songs, "")
	m.Key("esc", "")
	steps(m, 0.1)
	f = m.render()
	y = mlRowOf(m, "избранное")
	if y < 0 {
		t.Fatalf("the root is gone after esc:\n%s", stripANSI(m.View()))
	}
	if row := rowText(f, y+1, 0, m.w); !strings.Contains(row, "5 треков") {
		t.Errorf("the line under favorites = %q, want the count", row)
	}
}

// A3: the panel keeps one height whatever the rows are.
func TestPaletteFixedHeight(t *testing.T) {
	m1, _ := paletteSetup(t, 160, 45, "palette")
	m1.JumpPlay(threeTracks(), 0, 0)
	m1.Key("m", "m")
	steps(m1, 0.25)
	root := mlPanelRows(t, m1)

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	mlOpenSection(t, m2, MusicAlbums)
	albums := make([]MusicItem, 0, 8)
	for i := range 8 {
		albums = append(albums, musicAlbum(fmt.Sprintf("a%d", i), "Artist", fmt.Sprintf("Album %d", i+1), 10))
	}
	m2.MusicRows(mlLastLoad(t, r2).Gen, albums, "")
	grid := mlPanelRows(t, m2)

	m3, r3 := paletteSetup(t, 160, 45, "palette")
	m3.JumpPlay(threeTracks(), 0, 0)
	m3.Key("m", "m")
	steps(m3, 0.2)
	mlType(m3, "gn")
	steps(m3, 0.4)
	results := make([]MusicItem, 0, 30)
	for i := range 30 {
		results = append(results, musicSong(fmt.Sprintf("s%02d", i), "Alpha", fmt.Sprintf("Track %d", i+1), 200))
	}
	m3.MusicRows(mlLastLoad(t, r3).Gen, results, "")
	search := mlPanelRows(t, m3)

	if root != 34 || grid != 34 || search != 34 {
		t.Errorf("panel heights root=%d grid=%d search=%d, want 34 everywhere", root, grid, search)
	}
}

// A4: a track of an opened collection takes a number where the cover would be, and
// the playing one takes the equalizer.
func TestPaletteCollectionNumbers(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	tracks := []Track{
		{ID: "t1", Artist: "Artist1", Title: "Solo", Dur: 100},
		{ID: "t2", Artist: "Artist2", Title: "Duo", Dur: 200},
		{ID: "t3", Artist: "Artist3", Title: "Tre", Dur: 300},
	}
	m.JumpPlay(tracks, 1, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("a1", "Artist", "Album", 3)}, "")
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{
		musicSong("t1", "Artist1", "Solo", 100),
		musicSong("t2", "Artist2", "Duo", 200),
		musicSong("t3", "Artist3", "Tre", 300),
	}, "")
	steps(m, 0.2)
	f := m.render()
	for _, want := range []struct{ title, num string }{{"Solo", "01"}, {"Tre", "03"}} {
		y := mlRowOf(m, want.title)
		if y < 0 {
			t.Fatalf("%q is not in the collection:\n%s", want.title, stripANSI(m.View()))
		}
		if row := rowText(f, y, 0, m.w); !strings.Contains(row, want.num) {
			t.Errorf("the row of %q = %q, want the number %q", want.title, row, want.num)
		}
	}
	y := mlRowOf(m, "Duo")
	if y < 0 {
		t.Fatalf("the playing track is not in the collection:\n%s", stripANSI(m.View()))
	}
	row := rowText(f, y, 0, m.w)
	if strings.Contains(row, "02") {
		t.Errorf("the playing row keeps its number: %q", row)
	}
	if strings.Contains(row, "♪") {
		t.Errorf("the playing row keeps the gutter mark: %q", row)
	}
	if !strings.ContainsAny(row, "▁▂▃▄▅▆▇█") {
		t.Errorf("the playing row has no equalizer: %q", row)
	}
}

// A5: a row outside a collection keeps its thumbnail.
func TestPaletteListThumbs(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlType(m, "gn")
	steps(m, 0.4)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Frank Ocean", "Nights", 307)}, "")
	f := m.render()
	y := mlRowOf(m, "Nights")
	x, _ := mlPosOf(m, "Nights")
	if y < 0 || x < 6 {
		t.Fatalf("the row is not in view: %d,%d", x, y)
	}
	pix := 0
	for yy := y; yy < y+2; yy++ {
		for xx := x - 5; xx < x; xx++ {
			if c := f.at(xx, yy); c != nil && c.pix {
				pix++
			}
		}
	}
	if pix == 0 {
		t.Errorf("the search row has no thumbnail left of the title")
	}
}

// A6: the menu of the grid opens under the selected tile and stays inside the panel.
func TestPaletteGridMenuAnchor(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	albums := make([]MusicItem, 0, 24)
	for i := range 24 {
		albums = append(albums, musicAlbum(fmt.Sprintf("a%02d", i),
			fmt.Sprintf("Performer %02d", i+1), fmt.Sprintf("Album %02d", i+1), 10))
	}
	m.MusicRows(mlLastLoad(t, r).Gen, albums, "")
	m.Key("ctrl+k", "")
	if !m.mu.menu.open {
		t.Fatal("ctrl+k did not open the menu on the grid")
	}
	f := m.render()
	px, py, _, _ := m.mlPaletteGeom()
	tileX, tileY := px+m.mu.tileX, py+m.mu.tileY
	_, artist := mlPosOf(m, "Performer 01")
	top := tileY + mlTileH + 2
	if c := f.at(tileX, top); c == nil || c.ch != '╭' {
		t.Fatalf("the menu does not start at the tile: cell %d,%d is %q", tileX, top, string(c.ch))
	}
	if top <= artist {
		t.Errorf("the menu top row %d is not below the tile's artist row %d", top, artist)
	}

	m.mu.list.sel = 19
	steps(m, 0.05)
	f = m.render()
	row := mlRowOf(m, "play all")
	if row < 0 {
		t.Fatalf("the menu of the last tile is off the frame:\n%s", stripANSI(m.View()))
	}
	tileY = py + m.mu.tileY
	x0, _ := mlPosOf(m, "play all")
	if c := f.at(x0-2, row-2); c == nil || c.ch != '╭' {
		t.Errorf("the menu of the last tile does not start at its x: cell %d,%d is %q", x0-2, row-2, string(c.ch))
	}
	if row-2 != tileY+mlTileH+2 {
		t.Errorf("the menu top %d does not hang under the tile %d", row-2, tileY)
	}
	if row+1 > py+mlPanelRows(t, m)-1 || x0-2 < px || x0+len("play all") > px+m.mlPaletteW()-1 {
		t.Errorf("the menu of the last tile is not inside the panel")
	}
}

// A7: ctrl+e queues the selected song, nothing else, and never in the agent mode.
func TestPaletteQueueKey(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	m.Key("ctrl+e", "")
	app := recorded[MusicAppend](r)
	if len(app) != 1 || app[0].Item.ID != "s1" {
		t.Fatalf("appends = %+v, want one MusicAppend{s1}", r.acts)
	}

	m2, r2 := paletteRU(t, 160, 45)
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	mlOpenSection(t, m2, MusicAlbums)
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{musicAlbum("a1", "Beta", "Album One", 9)}, "")
	m2.Key("ctrl+e", "")
	if app := recorded[MusicAppend](r2); len(app) != 0 {
		t.Fatalf("appends = %+v, want none on a collection row", app)
	}
	steps(m2, 0.1)
	if frame := stripANSI(m2.View()); !strings.Contains(frame, "открой коллекцию") {
		t.Errorf("the collection row has no notice:\n%s", frame)
	}

	m3, r3 := paletteRU(t, 160, 45)
	m3.JumpPlay(threeTracks(), 0, 0)
	m3.Key("/", "/")
	steps(m3, 0.2)
	m3.Key("ctrl+e", "")
	if len(r3.acts) != 0 {
		t.Fatalf("the agent mode sent %+v on ctrl+e, want nothing", r3.acts)
	}
}

// A8: the bar offers ctrl+e on a song and drops tab before it.
func TestPaletteQueueCap(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	steps(m, 0.1)
	frame := stripANSI(m.View())
	for _, want := range []string{"ctrl+e", "в очередь"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the bar of a song has no %q:\n%s", want, frame)
		}
	}

	m2, r2 := paletteRU(t, 96, 30)
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	m2.Key("enter", "")
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	steps(m2, 0.1)
	frame = stripANSI(m2.View())
	if !strings.Contains(frame, "ctrl+e") {
		t.Errorf("the narrow bar lost ctrl+e:\n%s", frame)
	}
	if strings.Contains(frame, "tab") {
		t.Errorf("the narrow bar dropped ctrl+e before tab:\n%s", frame)
	}
}

// A9: the actions menu names the queue key of the entry.
func TestPaletteQueueHint(t *testing.T) {
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
	y := mlRowOf(m, "в очередь")
	if y < 0 {
		t.Fatalf("the menu has no queue entry:\n%s", stripANSI(m.View()))
	}
	if row := rowText(f, y, 0, m.w); !strings.Contains(row, "ctrl+e") {
		t.Errorf("the queue entry = %q, want its key", row)
	}
}

// A10: the detail pane comes at 110 panel columns.
func TestPaletteDetailBreakpoint(t *testing.T) {
	for _, tc := range []struct {
		w    int
		want bool
	}{{112, false}, {160, true}} {
		m, r := paletteSetup(t, tc.w, 45, "palette")
		m.JumpPlay(threeTracks(), 0, 0)
		m.Key("m", "m")
		steps(m, 0.2)
		m.Key("enter", "")
		m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "Kendrick Lamar", "squabble up", 157)}, "")
		if got := m.mlDetailOn(); got != tc.want {
			t.Errorf("%d wide: detail = %v, want %v", tc.w, got, tc.want)
		}
		x, _, w, _ := m.mlPaletteGeom()
		listRight := x + 1 + m.mlListW(w-2)
		f := m.render()
		pix := 0
		for yy := range f.h {
			for xx := listRight; xx < f.w; xx++ {
				if c := f.at(xx, yy); c != nil && c.pix {
					pix++
				}
			}
		}
		if (pix > 0) != tc.want {
			t.Errorf("%d wide: %d cover cells right of the list, want the pane %v", tc.w, pix, tc.want)
		}
	}
}
