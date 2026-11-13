package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai"
)

// shutMusic closes the music screen if it is open.
func shutMusic(t *testing.T, m *Model) {
	t.Helper()
	if m.vibeAIP.m.st.MusicOpen() {
		musicFeed(t, m, press(m, "ctrl+o"))
	}
}

// playBlonde plays the whole Blonde album from the albums grid.
func playBlonde(t *testing.T, m *Model) {
	t.Helper()
	shutMusic(t, m)
	openSection(t, m, 2)
	musicFeed(t, m, press(m, "ctrl+k"))
	musicFeed(t, m, press(m, "down"))
	musicFeed(t, m, press(m, "enter"))
}

// toggleFollow presses ctrl+n on the music screen, opening it first if needed.
func toggleFollow(t *testing.T, m *Model) {
	t.Helper()
	if !m.vibeAIP.m.st.MusicOpen() {
		musicFeed(t, m, press(m, "ctrl+o"))
	}
	musicFeed(t, m, press(m, "ctrl+n"))
}

func followOn(t *testing.T, m *Model) {
	t.Helper()
	toggleFollow(t, m)
	if !m.vibeAIP.m.st.Following() {
		t.Fatal("ctrl+n did not turn the switch on")
	}
	shutMusic(t, m)
}

func playing(t *testing.T, m *Model, id string) {
	t.Helper()
	for _, tr := range m.queueTracks {
		if PlaybackID(tr) == id {
			_, _ = m.Update(playerStateMsg(player.State{Playing: true, Track: &tr, Position: time.Second}))
			return
		}
	}
	t.Fatalf("%s is not in the queue", id)
}

// With the switch off a manual play stays finite: no agent turn starts.
func TestMusicFollowOffNoAgent(t *testing.T) {
	m, mp := musicTestModel(t, newMusicProvider())
	playBlonde(t, m)
	if len(mp.setQueueIDs) != 2 {
		t.Fatalf("setQueueIDs = %v, want the album", mp.setQueueIDs)
	}
	if m.vibeAI.busy || m.vibeAI.follow {
		t.Fatal("an agent turn started with the switch off")
	}
}

// With the switch on the agent prepares the next portion while the album plays, and
// it joins the queue only behind the album's last track.
func TestMusicFollowHandoffAfterManualQueue(t *testing.T) {
	m, mp := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	r := m.vibeAI
	if !r.busy || !r.follow {
		t.Fatal("no agent turn started for the album")
	}
	if want := "i.b1,i.b2"; strings.Join(mp.setQueueIDs, ",") != want {
		t.Fatalf("setQueueIDs = %v, want %s", mp.setQueueIDs, want)
	}
	m.Update(vibeAIPickMsg{gen: r.gen, pick: vibeai.Pick{Track: vibeTracks()[0], Accepted: true}})
	m.Update(vibeAIDoneMsg{gen: r.gen, tracks: vibeTracks()})
	if strings.Join(mp.setQueueIDs, ",") != "i.b1,i.b2" || len(mp.appendQueueIDs) != 0 {
		t.Fatalf("the agent touched the album queue early: set %v, appended %v", mp.setQueueIDs, mp.appendQueueIDs)
	}
	if r.staged == nil {
		t.Fatal("the portion is not staged behind the album")
	}
	playing(t, m, "i.b2")
	if got := m.queueIDs; strings.Join(got, ",") != "i.b1,i.b2,1,2,3" {
		t.Fatalf("queue = %v, want the album then the portion", got)
	}
	if _, ok := r.served["i.b1"]; ok {
		t.Fatal("a manual track counts as served by the agent")
	}
}

// A new manual selection drops the turn started for the old one.
func TestMusicFollowNewSelectionDropsStale(t *testing.T) {
	m, mp := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	stale := m.vibeAI.gen
	shutMusic(t, m)
	openFavorites(t, m)
	musicFeed(t, m, press(m, "enter"))
	if !m.vibeAI.busy || m.vibeAI.gen == stale {
		t.Fatal("the new selection did not start its own turn")
	}
	m.Update(vibeAIDoneMsg{gen: stale, tracks: vibeTracks()})
	if m.vibeAI.staged != nil || len(mp.appendQueueIDs) != 0 {
		t.Fatal("the stale turn reached the queue")
	}
}

// Turning the switch off drops the turn in flight; turning it on during a manual
// play starts one for that play.
func TestMusicFollowSwitchDuringPlay(t *testing.T) {
	m, mp := musicTestModel(t, newMusicProvider())
	playBlonde(t, m)
	if m.vibeAI.busy {
		t.Fatal("a turn started with the switch off")
	}
	toggleFollow(t, m)
	if !m.vibeAI.busy || !m.vibeAI.follow {
		t.Fatal("turning the switch on during the album started no turn")
	}
	gen := m.vibeAI.gen
	toggleFollow(t, m)
	m.Update(vibeAIDoneMsg{gen: gen, tracks: vibeTracks()})
	if m.vibeAI.staged != nil || len(mp.appendQueueIDs) != 0 {
		t.Fatal("the turn outlived the switch")
	}
}

// A failed turn is said on screen and leaves the album playing.
func TestMusicFollowFailureVisible(t *testing.T) {
	m, mp := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	m.Update(vibeAIDoneMsg{gen: m.vibeAI.gen, err: errors.New("no LLM key")})
	if strings.Join(m.queueIDs, ",") != "i.b1,i.b2" || len(mp.appendQueueIDs) != 0 {
		t.Fatalf("queue = %v, want the album untouched", m.queueIDs)
	}
	if frame := musicFrame(m, 0.5); !strings.Contains(frame, "no LLM key") {
		t.Fatalf("the failure is not on screen:\n%s", frame)
	}
}

// Once the switch is off, a stream that began from a selection stops topping up.
func TestMusicFollowOffStopsStream(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	m.Update(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks()})
	playing(t, m, "i.b2")
	toggleFollow(t, m)
	gen := m.vibeAI.gen
	playing(t, m, "3")
	if m.vibeAI.busy || m.vibeAI.gen != gen {
		t.Fatal("the stream topped up after the switch went off")
	}
}

// The referent keeps the collection and its tracks, and says the choice is not a
// lasting preference.
func TestMusicFollowText(t *testing.T) {
	tracks := musicBlondeTracks()
	album := &stage.MusicItem{Kind: stage.MusicAlbum, Title: "Blonde", Artist: "Frank Ocean"}
	txt := musicFollowText(tracks, album)
	for _, want := range []string{`album "Blonde" by Frank Ocean`, "1. Frank Ocean — Nikes", "2. Frank Ocean — Ivy", "not a lasting preference"} {
		if !strings.Contains(txt, want) {
			t.Errorf("album text lacks %q:\n%s", want, txt)
		}
	}
	pl := &stage.MusicItem{ID: "p.1", Kind: stage.MusicPlaylist, Title: "Chill"}
	if txt := musicFollowText(tracks, pl); !strings.Contains(txt, `playlist "Chill"`) {
		t.Errorf("playlist text:\n%s", txt)
	}
	favs := &stage.MusicItem{Kind: stage.MusicPlaylist, Title: "favorites"}
	if txt := musicFollowText(tracks, favs); !strings.Contains(txt, "from their Favorites") {
		t.Errorf("favorites text:\n%s", txt)
	}
	one := []provider.Track{tracks[1]}
	if txt := musicFollowText(one, nil); !strings.Contains(txt, `track Frank Ocean — Ivy (album "Blonde")`) {
		t.Errorf("track text:\n%s", txt)
	}
	long := make([]provider.Track, 40)
	for i := range long {
		long[i] = provider.Track{Artist: "A", Title: "T"}
	}
	if txt := musicFollowText(long, pl); !strings.Contains(txt, "… and 15 more") {
		t.Errorf("a long list is not cut:\n%s", txt)
	}
}

// Flipping the switch off and on after the handoff keeps the running stream instead
// of restarting the finished selection.
func TestMusicFollowOffOnAfterHandoff(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	m.Update(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks()})
	playing(t, m, "i.b2")
	toggleFollow(t, m)
	toggleFollow(t, m)
	if r := m.vibeAI; r.busy || r.portion != 1 || len(r.feed.tracks) != 3 {
		t.Fatalf("busy=%v portion=%d feed=%d, want the handed-off stream kept", r.busy, r.portion, len(r.feed.tracks))
	}
}

// A track queued with ctrl+e during a follow is not served twice.
func TestMusicFollowAppendNotRepeated(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	shutMusic(t, m)
	openFavorites(t, m)
	musicFeed(t, m, press(m, "down"))
	musicFeed(t, m, press(m, "ctrl+e"))
	ivy := vibeai.Track{ID: "102", Artist: "Frank Ocean", Title: "Ivy"}
	m.Update(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: append([]vibeai.Track{ivy}, vibeTracks()...)})
	playing(t, m, "i.2")
	if got := strings.Join(m.queueIDs, ","); got != "i.b1,i.b2,i.2,1,2,3" {
		t.Fatalf("queue = %s, want Ivy once", got)
	}
	if s := m.vibeAI.followSongs; len(s) != 1 || s[0].Title != "Ivy" {
		t.Fatalf("followSongs = %+v, want Ivy for the next turn", s)
	}
}

// With the switch off a manual queue never tops up, even on a track the agent served.
func TestMusicManualQueueNoTopUp(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	m.vibeAI.served["i.2"] = vibeai.Track{ID: "i.2"}
	openFavorites(t, m)
	musicFeed(t, m, press(m, "down"))
	musicFeed(t, m, press(m, "enter"))
	playing(t, m, "i.2")
	if m.vibeAI.busy {
		t.Fatal("a manual queue started an agent turn")
	}
}

// An empty follow portion is said on screen.
func TestMusicFollowEmptyPortion(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	followOn(t, m)
	playBlonde(t, m)
	m.Update(vibeAIDoneMsg{gen: m.vibeAI.gen})
	if frame := musicFrame(m, 0.5); !strings.Contains(frame, "found nothing") {
		t.Fatalf("the empty portion is silent:\n%s", frame)
	}
}

// Turning the switch on leaves a typed request in flight alone.
func TestMusicFollowOnKeepsTypedTurn(t *testing.T) {
	m, _ := musicTestModel(t, newMusicProvider())
	playBlonde(t, m)
	shutMusic(t, m)
	m.handleVibeAIMsg(VibeAISubmitMsg{Text: "rage", Fresh: true})
	gen := m.vibeAI.gen
	toggleFollow(t, m)
	if m.vibeAI.gen != gen || m.vibeAI.follow {
		t.Fatal("the switch replaced the typed request")
	}
}
