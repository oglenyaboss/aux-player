package stage

import (
	"strings"
	"testing"
)

// mlSearchWithRows opens the Russian palette, searches query and lands rows.
func mlSearchWithRows(t *testing.T, query string, rows []MusicItem) (*Model, *recorder) {
	t.Helper()
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlType(m, query)
	steps(m, 0.4)
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	return m, r
}

// mlSelectRow walks down until the highlighted row is the item keyed id.
func mlSelectRow(t *testing.T, m *Model, id string) {
	t.Helper()
	for range len(m.mlLines()) {
		if l, ok := m.mlSelected(); ok && l.it.ID == id {
			return
		}
		m.Key("down", "")
	}
	t.Fatalf("no row %q in the list", id)
}

// R1: enter on an album of the search starts the whole album, the top result
// included, and never queues it.
func TestPaletteSearchAlbumEnterPlaysAll(t *testing.T) {
	m, r := mlSearchSetup(t, "bl")
	if l, ok := m.mlSelected(); !ok || l.it.ID != "alb-blonde" {
		t.Fatalf("selection = %+v, want the Blonde hero", l)
	}
	m.Key("enter", "")
	alls := recorded[MusicPlayAll](r)
	if len(alls) != 1 || alls[0].Item.ID != "alb-blonde" {
		t.Fatalf("play all = %+v, want MusicPlayAll{alb-blonde}", r.acts)
	}
	if got := recorded[MusicAppend](r); len(got) != 0 {
		t.Errorf("enter queued %+v, want no append", got)
	}
	if got := recorded[MusicPlay](r); len(got) != 0 {
		t.Errorf("enter played %+v, want the play-all action alone", got)
	}

	rows := []MusicItem{
		musicSong("s1", "Frank Ocean", "Song One", 200),
		musicAlbum("alb-blonde", "Frank Ocean", "Blonde", 17),
		musicAlbum("alb-deluxe", "Frank Ocean", "Blonde (Deluxe)", 19),
	}
	m2, r2 := mlSearchWithRows(t, "bl", rows)
	mlSelectRow(t, m2, "alb-deluxe")
	m2.Key("enter", "")
	alls = recorded[MusicPlayAll](r2)
	if len(alls) != 1 || alls[0].Item.ID != "alb-deluxe" {
		t.Fatalf("play all = %+v, want the album of the row under the highlight", r2.acts)
	}
	if got := recorded[MusicAppend](r2); len(got) != 0 {
		t.Errorf("enter queued %+v, want no append", got)
	}
}

// R2: the album menu's open browses the tracks without playing, and esc puts the
// search text and its highlight back.
func TestPaletteSearchAlbumMenuOpensWithoutPlaying(t *testing.T) {
	m, r := mlSearchSetup(t, "bl")
	m.Key("ctrl+k", "")
	if !m.mu.menu.open {
		t.Fatal("ctrl+k did not open the album menu")
	}
	m.Key("enter", "")
	load := mlLastLoad(t, r)
	if load.Parent == nil || load.Parent.ID != "alb-blonde" || load.Query != "" {
		t.Fatalf("load = %+v, want the tracks of Blonde", load)
	}
	if m.mu.list.query != "" {
		t.Errorf("query = %q while browsing, want the search text out of the way", m.mu.list.query)
	}
	if got := recorded[MusicPlayAll](r); len(got) != 0 {
		t.Errorf("opening played all: %+v", got)
	}
	if got := recorded[MusicPlay](r); len(got) != 0 {
		t.Errorf("opening played: %+v", got)
	}

	tracks := []MusicItem{
		musicSong("t1", "Frank Ocean", "Nikes", 313),
		musicSong("t2", "Frank Ocean", "Ivy", 249),
	}
	m.MusicRows(load.Gen, tracks, "")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "Nikes") || !strings.Contains(frame, "Ivy") {
		t.Fatalf("the opened album does not show its tracks:\n%s", frame)
	}

	m.Key("esc", "")
	if m.mu.list.query != "bl" {
		t.Errorf("query after esc = %q, want the search text back", m.mu.list.query)
	}
	if l, ok := m.mlSelected(); !ok || l.it.ID != "alb-blonde" {
		t.Errorf("selection after esc = %+v, want the Blonde hero", l)
	}
	frame := stripANSI(m.View())
	for _, want := range []string{"лучшее совпадение", "Blonde", "Chill"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the results after esc lose %q:\n%s", want, frame)
		}
	}
	if got := recorded[MusicPlayAll](r); len(got) != 0 {
		t.Errorf("the browse sent play all: %+v", got)
	}

	before := len(recorded[MusicLoad](r))
	steps(m, 0.4)
	if got := len(recorded[MusicLoad](r)); got != before {
		t.Errorf("esc sent %d searches, want the rows already on screen", got-before)
	}
	m.Key("x", "x")
	steps(m, 0.4)
	if got := mlLastLoad(t, r); got.Query != "blx" {
		t.Errorf("load after typing = %+v, want the edited query blx", got)
	}
}

// R3: ctrl+e on a search album queues it, while enter on a search playlist and on
// a song keeps its old default.
func TestPaletteSearchRowDefaults(t *testing.T) {
	m, r := mlSearchSetup(t, "bl")
	m.Key("ctrl+e", "")
	apps := recorded[MusicAppend](r)
	if len(apps) != 1 || apps[0].Item.ID != "alb-blonde" {
		t.Fatalf("appends = %+v, want MusicAppend{alb-blonde}", r.acts)
	}
	if got := recorded[MusicPlayAll](r); len(got) != 0 {
		t.Errorf("ctrl+e played all: %+v", got)
	}

	mlSelectRow(t, m, "pl-chill")
	m.Key("enter", "")
	if load := mlLastLoad(t, r); load.Parent == nil || load.Parent.ID != "pl-chill" {
		t.Fatalf("load = %+v, want the tracks of the playlist", load)
	}
	if got := recorded[MusicPlayAll](r); len(got) != 0 {
		t.Errorf("enter on a search playlist played all: %+v", got)
	}

	m2, r2 := mlSearchSetup(t, "song 3")
	if l, ok := m2.mlSelected(); !ok || l.it.ID != "s03" {
		t.Fatalf("selection = %+v, want the Song 3 hero", l)
	}
	m2.Key("enter", "")
	plays := recorded[MusicPlay](r2)
	if len(plays) != 1 || len(plays[0].Items) != 1 || plays[0].Items[0].ID != "s03" {
		t.Fatalf("plays = %+v, want the one hero song", r2.acts)
	}
}

// R3 regression: an album of the library still opens on enter, with no playback.
func TestPaletteLibraryAlbumEnterStillOpens(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicAlbums)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("alb-utopia", "Travis Scott", "UTOPIA", 3)}, "")
	m.Key("enter", "")
	load := mlLastLoad(t, r)
	if load.Parent == nil || load.Parent.ID != "alb-utopia" {
		t.Fatalf("load = %+v, want the tracks of UTOPIA", load)
	}
	if got := recorded[MusicPlayAll](r); len(got) != 0 {
		t.Errorf("the library album played all: %+v", got)
	}
	if got := recorded[MusicPlay](r); len(got) != 0 {
		t.Errorf("the library album played: %+v", got)
	}
	m.Key("esc", "")
	if m.mu.coll != nil {
		t.Fatal("esc did not leave the library album")
	}
	if l, ok := m.mlSelected(); !ok || l.it.ID != "alb-utopia" {
		t.Errorf("selection after esc = %+v, want the UTOPIA tile", l)
	}
}

// mlOpenSearchAlbumFromAlbums opens the Albums section of the palette root, searches
// bl and opens Blonde from the menu, leaving its tracks loading.
func mlOpenSearchAlbumFromAlbums(t *testing.T, m *Model, r *recorder) {
	t.Helper()
	mlOpenSection(t, m, MusicAlbums)
	mlType(m, "bl")
	steps(m, 0.4)
	m.MusicRows(mlLastLoad(t, r).Gen, mlBlondeRows(), "")
	m.Key("ctrl+k", "")
	m.Key("enter", "")
	if load := mlLastLoad(t, r); load.Parent == nil || load.Parent.ID != "alb-blonde" {
		t.Fatalf("load = %+v, want the tracks of Blonde", load)
	}
}

// R1: an album opened from the search names the search as its origin, not the
// section the palette was browsing.
func TestPaletteSearchAlbumCrumbNamesSearch(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSearchAlbumFromAlbums(t, m, r)
	if got := m.mlCrumb(); got != "поиск › Blonde" {
		t.Errorf("crumb = %q, want the search origin", got)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "поиск › Blonde") {
		t.Errorf("the palette does not show the search breadcrumb:\n%s", frame)
	}
}

// R2: a search typed inside a browsed search album replaces the rows only while the
// album is open: esc twice puts the query, the bl results and Blonde back.
func TestPaletteSearchAlbumBackRestoresResults(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSearchAlbumFromAlbums(t, m, r)

	mlType(m, "x")
	steps(m, 0.4)
	xLoad := mlLastLoad(t, r)
	if xLoad.Parent != nil || xLoad.Query != "x" {
		t.Fatalf("load = %+v, want the search for x", xLoad)
	}
	m.MusicRows(xLoad.Gen, []MusicItem{
		musicAlbum("alb-x", "X", "Xylophone", 9),
		musicSong("s-x", "X", "X Song", 100),
	}, "")

	m.Key("esc", "")
	m.Key("esc", "")
	if m.mu.coll != nil {
		t.Fatal("esc did not leave the album")
	}
	if m.mu.list.query != "bl" {
		t.Errorf("query after esc = %q, want the search text back", m.mu.list.query)
	}
	if l, ok := m.mlSelected(); !ok || l.it.ID != "alb-blonde" {
		t.Errorf("selection after esc = %+v, want the Blonde hero", l)
	}
	frame := stripANSI(m.View())
	for _, want := range []string{"лучшее совпадение", "Blonde", "Chill"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the results after esc lose %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Xylophone") {
		t.Errorf("the x results are still on screen after esc:\n%s", frame)
	}

	before := len(recorded[MusicLoad](r))
	steps(m, 0.4)
	if got := len(recorded[MusicLoad](r)); got != before {
		t.Errorf("esc sent %d searches, want the rows already on screen", got-before)
	}

	m.Key("enter", "")
	alls := recorded[MusicPlayAll](r)
	if len(alls) != 1 || alls[0].Item.ID != "alb-blonde" {
		t.Fatalf("play all = %+v, want MusicPlayAll{alb-blonde}", r.acts)
	}
	if got := recorded[MusicPlay](r); len(got) != 0 {
		t.Errorf("enter played %+v, want the album of the bl search", got)
	}
}

// R2: a search already on its way when the listener backs out cannot replace the
// rows and the selection esc restored.
func TestPaletteSearchAlbumBackDropsLateSearch(t *testing.T) {
	m, r := paletteRU(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSearchAlbumFromAlbums(t, m, r)

	mlType(m, "x")
	steps(m, 0.4)
	xLoad := mlLastLoad(t, r)
	if xLoad.Parent != nil || xLoad.Query != "x" {
		t.Fatalf("load = %+v, want the search for x", xLoad)
	}

	m.Key("esc", "")
	m.Key("esc", "")
	m.MusicRows(xLoad.Gen, []MusicItem{musicAlbum("alb-x", "X", "Xylophone", 9)}, "")
	if m.mu.list.query != "bl" {
		t.Errorf("query after esc = %q, want the search text back", m.mu.list.query)
	}
	if l, ok := m.mlSelected(); !ok || l.it.ID != "alb-blonde" {
		t.Errorf("selection after esc = %+v, want the Blonde hero", l)
	}
	if frame := stripANSI(m.View()); strings.Contains(frame, "Xylophone") {
		t.Errorf("the late x answer reached the restored list:\n%s", frame)
	}

	m.Key("enter", "")
	alls := recorded[MusicPlayAll](r)
	if len(alls) != 1 || alls[0].Item.ID != "alb-blonde" {
		t.Fatalf("play all = %+v, want MusicPlayAll{alb-blonde}", r.acts)
	}
}

// R4: the bar names the new default on a search album, the old one elsewhere, and
// the album menu still offers both actions in either language.
func TestPaletteBarNamesSearchAlbumDefault(t *testing.T) {
	t.Run("en", func(t *testing.T) {
		m, r := paletteSetup(t, 160, 45, "palette")
		m.JumpPlay(threeTracks(), 0, 0)
		m.Key("m", "m")
		steps(m, 0.2)
		mlType(m, "bl")
		steps(m, 0.4)
		m.MusicRows(mlLastLoad(t, r).Gen, mlBlondeRows(), "")
		frame := stripANSI(m.View())
		if !strings.Contains(frame, "play all") {
			t.Errorf("the bar of a search album has no play all:\n%s", frame)
		}
		if strings.Contains(frame, "open") {
			t.Errorf("the bar of a search album still says open:\n%s", frame)
		}
		m.Key("ctrl+k", "")
		frame = stripANSI(m.View())
		for _, want := range []string{"open", "play all"} {
			if !strings.Contains(frame, want) {
				t.Errorf("the album menu loses %q:\n%s", want, frame)
			}
		}
		m.Key("esc", "")
		mlSelectRow(t, m, "pl-chill")
		frame = stripANSI(m.View())
		if !strings.Contains(frame, "open") || strings.Contains(frame, "play all") {
			t.Errorf("the bar of a search playlist is not open:\n%s", frame)
		}
	})

	t.Run("ru", func(t *testing.T) {
		m, _ := mlSearchSetup(t, "bl")
		frame := stripANSI(m.View())
		if !strings.Contains(frame, "играть всё") {
			t.Errorf("the bar of a search album has no играть всё:\n%s", frame)
		}
		if strings.Contains(frame, "открыть") {
			t.Errorf("the bar of a search album still says открыть:\n%s", frame)
		}
		m.Key("ctrl+k", "")
		frame = stripANSI(m.View())
		for _, want := range []string{"открыть", "играть всё"} {
			if !strings.Contains(frame, want) {
				t.Errorf("the album menu loses %q:\n%s", want, frame)
			}
		}
		m.Key("esc", "")
		mlSelectRow(t, m, "pl-chill")
		frame = stripANSI(m.View())
		if !strings.Contains(frame, "открыть") || strings.Contains(frame, "играть всё") {
			t.Errorf("the bar of a search playlist is not открыть:\n%s", frame)
		}
	})
}
