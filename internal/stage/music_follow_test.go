package stage

import (
	"strings"
	"testing"
)

// The switch starts off, ctrl+n flips it, and it outlives closing the screen.
func TestMusicFollowToggle(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	if m.Following() {
		t.Fatal("the switch is on at launch")
	}
	refreshSongs(m, r, t, musicSong("a", "A", "Alpha", 100))
	m.Key("ctrl+n", "")
	if !m.Following() {
		t.Fatal("ctrl+n did not turn the switch on")
	}
	m.Key("ctrl+o", "")
	m.OpenMusic()
	if !m.Following() {
		t.Fatal("the switch did not survive closing the screen")
	}
	m.Key("ctrl+n", "")
	if m.Following() {
		t.Fatal("the second ctrl+n did not turn the switch off")
	}
	got := recorded[MusicFollow](r)
	if len(got) != 2 || !got[0].On || got[1].On {
		t.Fatalf("MusicFollow = %+v, want on then off", got)
	}
}

// The bar names the key and the state on a playable row.
func TestMusicFollowInBar(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	refreshSongs(m, r, t, musicSong("a", "A", "Alpha", 100))
	if txt := m.renderText(); !strings.Contains(txt, "ctrl+n") || !strings.Contains(txt, "agent continues: off") {
		t.Fatalf("the bar does not offer the switch:\n%s", txt)
	}
	m.Key("ctrl+n", "")
	m.MusicNotice("")
	if txt := m.renderText(); !strings.Contains(txt, "agent continues: on") {
		t.Fatalf("the bar does not show the switch on:\n%s", txt)
	}
}

// The actions menu of a collection carries the switch too.
func TestMusicFollowMenu(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	m.OpenMusic()
	mlOpenSection(t, m, MusicAlbums)
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{musicAlbum("l.1", "A", "Album", 2)}, "")
	m.Key("ctrl+k", "")
	items := m.mlMenuItems()
	last := items[len(items)-1]
	if last.kind != mlMenuFollow || !strings.Contains(last.label, "agent continues: off") {
		t.Fatalf("menu = %+v, want the switch last", items)
	}
	for range items {
		m.Key("down", "")
	}
	m.Key("enter", "")
	if !m.Following() {
		t.Fatal("the menu entry did not turn the switch on")
	}
}

// Without an agent the switch stays off and the agent step of the onboarding opens.
func TestMusicFollowNoAgent(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, musicStylePalette)
	m.o.NoAgent = true
	refreshSongs(m, r, t, musicSong("a", "A", "Alpha", 100))
	m.Key("ctrl+n", "")
	if m.Following() {
		t.Fatal("the switch turned on without an agent")
	}
	if m.ph != PhOnboard {
		t.Fatalf("phase = %v, want the onboarding", m.ph)
	}
	if got := recorded[MusicFollow](r); len(got) != 0 {
		t.Fatalf("MusicFollow = %+v, want none", got)
	}
}
