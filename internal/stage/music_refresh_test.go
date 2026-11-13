package stage

import (
	"strings"
	"testing"
)

func refreshSongs(m *Model, r *recorder, t *testing.T, items ...MusicItem) {
	t.Helper()
	m.OpenMusic()
	mlOpenSection(t, m, MusicSongs)
	m.MusicRows(mlLastLoad(t, r).Gen, items, "")
}

// MusicUpdate swaps a loaded tab's rows and keeps the highlight on the same song.
func TestMusicUpdateKeepsSelectionByKey(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	refreshSongs(m, r, t,
		musicSong("a", "Artist", "Alpha", 100),
		musicSong("b", "Artist", "Bravo", 100),
		musicSong("c", "Artist", "Charlie", 100))
	m.Key("down", "")
	if l, ok := m.mlSelected(); !ok || l.it.ID != "b" {
		t.Fatalf("selected %+v, want b", l.it)
	}
	m.MusicUpdate(MusicSongs, "", []MusicItem{
		musicSong("z", "Artist", "Zulu", 100),
		musicSong("a", "Artist", "Alpha", 100),
		musicSong("b", "Artist", "Bravo", 100),
	})
	if got := len(m.mu.tabs[MusicSongs].items); got != 3 || m.mu.tabs[MusicSongs].items[0].ID != "z" {
		t.Fatalf("rows = %+v, want z first", m.mu.tabs[MusicSongs].items)
	}
	if l, ok := m.mlSelected(); !ok || l.it.ID != "b" {
		t.Fatalf("selected after update %+v, want b", l.it)
	}
	if !strings.Contains(m.renderText(), "Zulu") {
		t.Error("the fresh row is not drawn")
	}
}

// A tab never loaded stays unloaded: the update does not fake a load.
func TestMusicUpdateSkipsUnloadedTab(t *testing.T) {
	m, _ := paletteSetup(t, 160, 45, musicStylePalette)
	m.MusicUpdate(MusicAlbums, "", []MusicItem{musicAlbum("l.1", "A", "Album", 3)})
	if len(m.mu.tabs[MusicAlbums].items) != 0 {
		t.Fatal("an unloaded tab took rows")
	}
}

// An open collection and its peek take the fresh tracks.
func TestMusicUpdateCollection(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	m.OpenMusic()
	mlOpenSection(t, m, MusicAlbums)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("l.1", "A", "Album", 2)}, "")
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("s1", "A", "One", 100)}, "")
	m.MusicUpdate(MusicAlbums, "l.1", []MusicItem{musicSong("s1", "A", "One", 100), musicSong("s2", "A", "Two", 100)})
	if c := m.mu.coll; c == nil || len(c.items) != 2 {
		t.Fatalf("collection = %+v, want two tracks", c)
	}
	if got := len(m.mu.peek["l.1"]); got != 2 {
		t.Fatalf("peek = %d tracks, want 2", got)
	}
}

// ctrl+r over loaded rows asks the host to check them; at the root it asks for all.
func TestMusicCtrlRRefreshes(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	m.OpenMusic()
	m.Key("ctrl+r", "")
	refs := recorded[MusicRefresh](r)
	if len(refs) != 1 || !refs[0].All {
		t.Fatalf("root ctrl+r = %+v, want one All refresh", refs)
	}
	mlOpenSection(t, m, MusicSongs)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicSong("a", "A", "Alpha", 100)}, "")
	m.Key("ctrl+r", "")
	refs = recorded[MusicRefresh](r)
	if len(refs) != 2 || refs[1].All || refs[1].Section != MusicSongs || refs[1].Parent != nil {
		t.Fatalf("songs ctrl+r = %+v, want a Songs refresh", refs)
	}
}

// The bar says the rows are being checked while the host refreshes.
func TestMusicRefreshingHint(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	refreshSongs(m, r, t, musicSong("a", "A", "Alpha", 100))
	m.MusicRefreshing(true)
	if !strings.Contains(m.renderText(), "refreshing") {
		t.Fatal("no refreshing hint while the host refreshes")
	}
	m.MusicRefreshing(false)
	if strings.Contains(m.renderText(), "refreshing") {
		t.Fatal("the refreshing hint outlived the refresh")
	}
}

func (m *Model) renderText() string {
	steps(m, 1)
	f := m.render()
	var b strings.Builder
	for y := range m.h {
		b.WriteString(rowText(f, y, 0, m.w))
		b.WriteByte('\n')
	}
	return b.String()
}
