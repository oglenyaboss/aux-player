package stage

import (
	"strings"
	"testing"
)

// sortSongs is the host's rows in fetch order: the library comes back newest
// first, which is the fresh sort.
func sortSongs() []MusicItem {
	rows := []MusicItem{
		musicSong("s1", "Beta", "Two", 200),
		musicSong("s2", "Alpha", "one", 201),
		musicSong("s3", "Celine", "Three", 202),
		musicSong("s4", "Alpha", "Two Half", 203),
	}
	rows[2].Fav = true
	return rows
}

func sortTitles(m *Model) []string {
	out := []string{}
	for _, it := range m.mlState().items {
		out = append(out, it.Title)
	}
	return out
}

func wantTitles(t *testing.T, m *Model, want ...string) {
	t.Helper()
	got := sortTitles(m)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows = %v, want %v", got, want)
	}
}

// B1: ctrl+s cycles the song tab's sort: fresh (the host's order), title, artist,
// favorites first, and the crumb names the mode.
func TestMusicSortCycle(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	for range 3 {
		m.Key("down", "")
	}
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, sortSongs(), "")
	wantTitles(t, m, "Two", "one", "Three", "Two Half")
	if got := m.mlCrumb(); got != "songs · recent" {
		t.Fatalf("crumb = %q, want the fresh sort named", got)
	}
	m.Key("ctrl+s", "")
	wantTitles(t, m, "one", "Three", "Two", "Two Half")
	if got := m.mlCrumb(); got != "songs · title" {
		t.Fatalf("crumb = %q, want the title sort named", got)
	}
	m.Key("ctrl+s", "")
	wantTitles(t, m, "one", "Two Half", "Two", "Three")
	if got := m.mlCrumb(); got != "songs · artist" {
		t.Fatalf("crumb = %q, want the artist sort named", got)
	}
	m.Key("ctrl+s", "")
	wantTitles(t, m, "Three", "Two", "one", "Two Half")
	if got := m.mlCrumb(); got != "songs · favorites" {
		t.Fatalf("crumb = %q, want the favorites sort named", got)
	}
	m.Key("ctrl+s", "")
	wantTitles(t, m, "Two", "one", "Three", "Two Half")
	if got := m.mlCrumb(); got != "songs · recent" {
		t.Fatalf("crumb = %q, want the cycle back to fresh", got)
	}
}

// B1: a grid tab keeps its own order; ctrl+s does nothing there.
func TestMusicSortSkipsGridTabs(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	m.Key("down", "")
	m.Key("down", "")
	m.Key("enter", "")
	rows := []MusicItem{musicAlbum("a1", "Beta", "Zeta", 2), musicAlbum("a2", "Alpha", "Able", 3)}
	m.MusicRows(mlLastLoad(t, r).Gen, rows, "")
	m.Key("ctrl+s", "")
	if got := m.mlCrumb(); got != "albums" {
		t.Fatalf("crumb = %q, want no sort named on a grid tab", got)
	}
	if len(m.mlState().items) != 2 || m.mlState().items[0].Title != "Zeta" {
		t.Fatalf("rows = %v, want the host's order untouched", m.mlState().items)
	}
}

// B1: turning the sort keeps the highlight on its row.
func TestMusicSortKeepsHighlight(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	for range 3 {
		m.Key("down", "")
	}
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, sortSongs(), "")
	m.Key("down", "")
	if got := mlMarked(m); !strings.Contains(got, "one") {
		t.Fatalf("highlight = %q, want the second row", got)
	}
	m.Key("ctrl+s", "")
	if got := mlMarked(m); !strings.Contains(got, "one") {
		t.Fatalf("highlight = %q, want the same row after the sort", got)
	}
}

// B1: refreshed rows land in the host's order but the tab stays in the picked
// sort, and fresh still shows the host's order.
func TestMusicUpdateKeepsSort(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("ctrl+o", "")
	steps(m, 0.2)
	for range 3 {
		m.Key("down", "")
	}
	m.Key("enter", "")
	m.MusicRows(mlLastLoad(t, r).Gen, sortSongs(), "")
	m.Key("ctrl+s", "")
	m.MusicUpdate(MusicSongs, "", []MusicItem{
		musicSong("s9", "Zeta", "Wan", 100),
		musicSong("s1", "Beta", "Two", 200),
	})
	wantTitles(t, m, "Two", "Wan")
	m.Key("ctrl+s", "")
	m.Key("ctrl+s", "")
	m.Key("ctrl+s", "")
	wantTitles(t, m, "Wan", "Two")
}

// B1: the full screen sorts the same way and names the mode on its tab.
func TestMusicSortFullStyle(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	for range 3 {
		m.Key("right", "")
	}
	m.MusicRows(mlLastLoad(t, r).Gen, sortSongs(), "")
	m.Key("down", "")
	if got := musicSelected(m); !strings.Contains(got, "one") {
		t.Fatalf("selection = %q, want the second row before the sort", got)
	}
	m.Key("ctrl+s", "")
	var got []string
	for _, it := range m.musicViewState().items {
		got = append(got, it.Title)
	}
	if strings.Join(got, "|") != "one|Three|Two|Two Half" {
		t.Fatalf("rows = %v, want the title sort", got)
	}
	if sel := musicSelected(m); !strings.Contains(sel, "one") {
		t.Fatalf("selection = %q, want the same row after the sort", sel)
	}
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "songs · title") {
		t.Errorf("the tab does not name the sort:\n%s", frame)
	}
}
