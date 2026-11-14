package stage

import (
	"fmt"
	"strings"
	"testing"
)

func playlistTracks(n int) []Track {
	tracks := make([]Track, n)
	for i := range tracks {
		tracks[i] = Track{Artist: "Artist", Title: fmt.Sprintf("T%02d", i+1), Dur: 120}
	}
	return tracks
}

// feedPlaylist sends the whole playlist through the scan and waits for the done screen.
func feedPlaylist(t *testing.T, m *Model, n int) []Track {
	t.Helper()
	tracks := playlistTracks(n)
	m.PortionDone(tracks, "picked for the mood")
	for _, tr := range tracks {
		m.Candidate(tr, true, "")
	}
	if !runTo(m, PhDone, 6000) {
		t.Fatalf("phase %v, want PhDone", m.Phase())
	}
	return tracks
}

// playlistDone walks the usual first turn and fills a playlist of n tracks.
func playlistDone(t *testing.T, m *Model, n int) []Track {
	t.Helper()
	toScan(t, m)
	m.PlaylistMode(n)
	return feedPlaylist(t, m, n)
}

// A1: PlaylistMode grows side A to the playlist size and the scan fills all of it.
func TestPlaylistModeGrowsSide(t *testing.T) {
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	toScan(t, m)
	m.PlaylistMode(12)
	if len(m.slots) != 12 {
		t.Fatalf("slots=%d, want 12", len(m.slots))
	}
	feedPlaylist(t, m, 12)
	if m.filled != 12 {
		t.Fatalf("filled=%d, want 12", m.filled)
	}
}

// A2: a finished playlist waits on the done screen; the side does not roll into the player.
func TestPlaylistModeStaysOnDone(t *testing.T) {
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	playlistDone(t, m, 12)
	steps(m, 6)
	if m.Phase() != PhDone {
		t.Fatalf("phase %v, want PhDone", m.Phase())
	}
}

// A3: the done screen carries the save status and the key hint, and the save result replaces it.
func TestPlaylistDoneScreenText(t *testing.T) {
	SetLang("en")
	t.Cleanup(func() { SetLang("ru") })
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	playlistDone(t, m, 12)
	steps(m, 2)
	frame := stripANSI(m.View())
	for _, want := range []string{"saving to Apple Music…", "enter — listen · r — new request"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame is missing %q:\n%s", want, frame)
		}
	}
	m.PlaylistSaved("saved ♫ Haze · 12 tracks")
	steps(m, 1)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "saved ♫ Haze · 12 tracks") {
		t.Errorf("frame is missing the save line:\n%s", frame)
	}
}

// A4: enter on a finished playlist plays the side and tells the host.
func TestPlaylistEnterListens(t *testing.T) {
	r := &recorder{}
	m := New(Options{Slots: 5, OnAction: r.on})
	m.SetSize(110, 32)
	playlistDone(t, m, 12)
	if !m.Key("enter", "") {
		t.Fatal("enter was not taken on a finished playlist")
	}
	if m.Phase() != PhPlay {
		t.Fatalf("phase %v, want PhPlay", m.Phase())
	}
	if len(m.queue) != 12 {
		t.Fatalf("queue=%d, want the 12 tracks of the side", len(m.queue))
	}
	if n := len(recorded[Listen](r)); n != 1 {
		t.Fatalf("Listen sent %d times, want 1: %v", n, r.acts)
	}
}

// A5: r on a finished playlist starts over.
func TestPlaylistRStartsOver(t *testing.T) {
	r := &recorder{}
	m := New(Options{Slots: 5, OnAction: r.on})
	m.SetSize(110, 32)
	playlistDone(t, m, 12)
	if !m.Key("r", "") {
		t.Fatal("r was not taken on a finished playlist")
	}
	if m.Phase() != PhIntro {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	if n := len(recorded[Restart](r)); n != 1 {
		t.Fatalf("Restart sent %d times, want 1: %v", n, r.acts)
	}
}

// A6: a side taller than the panel scrolls to its newest filled row.
func TestSideScrollsToNewest(t *testing.T) {
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	playlistDone(t, m, 30)
	steps(m, 1)
	frame := stripANSI(m.View())
	for _, want := range []string{"A30", "T30"} {
		if !strings.Contains(frame, want) {
			t.Errorf("frame is missing the newest row %q:\n%s", want, frame)
		}
	}
}

// A7: without playlist mode the done screen still rolls into the player on its own.
func TestStreamDoneStillPlays(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tracks := threeTracks()
	toScan(t, m)
	m.PortionDone(tracks, "")
	for _, tr := range tracks {
		m.Candidate(tr, true, "")
	}
	if !runTo(m, PhPlay, 3000) {
		t.Fatalf("phase %v, want PhPlay", m.Phase())
	}
}

func TestPlaylistDoneWSaves(t *testing.T) {
	r := &recorder{}
	m := New(Options{Slots: 5, OnAction: r.on})
	m.SetSize(110, 32)
	playlistDone(t, m, 12)
	if !m.Key("w", "") {
		t.Fatal("w was not taken on a finished playlist")
	}
	if n := len(recorded[Save](r)); n != 1 {
		t.Fatalf("Save sent %d times, want 1: %v", n, r.acts)
	}
}
