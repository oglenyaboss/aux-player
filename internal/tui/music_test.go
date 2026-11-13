package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/oglenyaboss/aux-player/internal/library"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
)

// musicProvider is the fake Apple Music the screen browses: it embeds the nil
// mock and records every call with its argument.
type musicProvider struct {
	mockProvider
	favTracks  []provider.Track // the library's Favorites
	libOnly    []provider.Track // the rest of the library
	favErr     error
	albums     []provider.Album
	playlists  []provider.Playlist
	searchRes  *provider.SearchResult
	collTracks map[string][]provider.Track

	calls  []string
	favIDs []string
}

func (p *musicProvider) called(call string) bool {
	for _, c := range p.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (p *musicProvider) callCount(call string) int {
	n := 0
	for _, c := range p.calls {
		if c == call {
			n++
		}
	}
	return n
}

func (p *musicProvider) GetPlaylistTracks(_ context.Context, id string) ([]provider.Track, error) {
	p.calls = append(p.calls, "GetPlaylistTracks:"+id)
	if id == provider.FavoritesPlaylistID {
		return p.favTracks, p.favErr
	}
	return p.collTracks[id], nil
}

func (p *musicProvider) GetLibraryTracks(_ context.Context) ([]provider.Track, error) {
	p.calls = append(p.calls, "GetLibraryTracks")
	if p.favErr != nil {
		return nil, p.favErr
	}
	var out []provider.Track
	for _, t := range p.favTracks {
		t.Favorite = true
		out = append(out, t)
	}
	return append(out, p.libOnly...), nil
}

func (p *musicProvider) GetLibraryPlaylists(_ context.Context) ([]provider.Playlist, error) {
	p.calls = append(p.calls, "GetLibraryPlaylists")
	return p.playlists, nil
}

func (p *musicProvider) GetLibraryAlbums(_ context.Context) ([]provider.Album, error) {
	p.calls = append(p.calls, "GetLibraryAlbums")
	return p.albums, nil
}

func (p *musicProvider) GetLibraryAlbumTracks(_ context.Context, id string) ([]provider.Track, error) {
	p.calls = append(p.calls, "GetLibraryAlbumTracks:"+id)
	return p.collTracks[id], nil
}

func (p *musicProvider) GetAlbumTracks(_ context.Context, id string) ([]provider.Track, error) {
	p.calls = append(p.calls, "GetAlbumTracks:"+id)
	return p.collTracks[id], nil
}

func (p *musicProvider) Search(_ context.Context, query string) (*provider.SearchResult, error) {
	p.calls = append(p.calls, "Search:"+query)
	return p.searchRes, nil
}

func (p *musicProvider) AddToFavorites(_ context.Context, id string) error {
	p.favIDs = append(p.favIDs, id)
	return nil
}

func musicFavorites() []provider.Track {
	return []provider.Track{
		{ID: "i.1", CatalogID: "101", Title: "Nikes", Artist: "Frank Ocean", Album: "Blonde", Duration: 314 * time.Second},
		{ID: "i.2", CatalogID: "102", Title: "Ivy", Artist: "Frank Ocean", Album: "Blonde", Duration: 249 * time.Second},
		{ID: "i.3", CatalogID: "", Title: "Memo", Artist: "Me", Duration: 42 * time.Second},
	}
}

func musicAlbums() []provider.Album {
	return []provider.Album{
		{ID: "l.a1", Title: "Blonde", Artist: "Frank Ocean", TrackCount: 2, ReleaseDate: "2016-08-20"},
		{ID: "999", Title: "GNX", Artist: "Kendrick Lamar"},
	}
}

func musicPlaylists() []provider.Playlist {
	return []provider.Playlist{
		{ID: provider.FavoritesPlaylistID, Name: "Favorites", TrackCount: 3},
		{ID: "p.1", Name: "Chill", TrackCount: 5},
	}
}

func musicBlondeTracks() []provider.Track {
	return []provider.Track{
		{ID: "i.b1", CatalogID: "201", Title: "Nikes", Artist: "Frank Ocean", Album: "Blonde", Duration: 314 * time.Second},
		{ID: "i.b2", CatalogID: "202", Title: "Ivy", Artist: "Frank Ocean", Album: "Blonde", Duration: 249 * time.Second},
	}
}

func newMusicProvider() *musicProvider {
	return &musicProvider{
		favTracks: musicFavorites(),
		albums:    musicAlbums(),
		playlists: musicPlaylists(),
		searchRes: &provider.SearchResult{
			Tracks: []provider.Track{{ID: "s.1", CatalogID: "401", Title: "Nights", Artist: "Frank Ocean"}},
			Albums: []provider.Album{{ID: "l.a1", Title: "Blonde", Artist: "Frank Ocean", TrackCount: 2}},
		},
		collTracks: map[string][]provider.Track{
			"l.a1": musicBlondeTracks(),
			"999":  {{ID: "i.g1", CatalogID: "301", Title: "squabble up", Artist: "Kendrick Lamar", Duration: 157 * time.Second}},
		},
	}
}

func musicTestModel(t *testing.T, p *musicProvider) (*Model, *mockPlayer) {
	t.Helper()
	mp := newMockPlayer()
	m := saveTestModel(t, p, mp)
	stage.SetLang("en")
	t.Cleanup(func() { stage.SetLang("en") })
	return m, mp
}

// musicFeed drives the stage's buffered actions to the end: each msg goes through
// Update and the cmd it returns runs and lands the same way.
func musicFeed(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	for _, msg := range stageMsgs(t, cmd) {
		if msg == nil {
			continue
		}
		if _, ok := msg.(stageTickMsg); ok {
			continue
		}
		_, next := m.Update(msg)
		musicFeed(t, m, next)
	}
}

func musicStep(m *Model, secs float64) {
	for range int(secs / 0.033) {
		m.vibeAIP.m.st.Step(0.033)
	}
}

func musicFrame(m *Model, secs float64) string {
	musicStep(m, secs)
	return ansi.Strip(m.vibeAIP.m.Frame(160, 45))
}

// openFavorites drops the palette in the player and enters the favorites section.
func openFavorites(t *testing.T, m *Model) {
	t.Helper()
	musicFeed(t, m, press(m, "ctrl+o"))
	musicFeed(t, m, press(m, "enter"))
}

// openSection enters the palette's root entry d steps below favorites.
func openSection(t *testing.T, m *Model, downs int) {
	t.Helper()
	musicFeed(t, m, press(m, "ctrl+o"))
	for range downs {
		musicFeed(t, m, press(m, "down"))
	}
	musicFeed(t, m, press(m, "enter"))
}

// A1: the palette's favorites entry shows the library songs flagged as Favorites.
func TestMusicFavoritesLoad(t *testing.T) {
	p := newMusicProvider()
	p.libOnly = []provider.Track{{ID: "i.9", CatalogID: "109", Title: "Not loved", Artist: "X"}}
	m, _ := musicTestModel(t, p)
	openFavorites(t, m)
	if !p.called("GetLibraryTracks") {
		t.Fatalf("provider calls = %v, want the library scan", p.calls)
	}
	if frame := saveFrame(m); strings.Contains(frame, "Not loved") {
		t.Errorf("a song outside Favorites is listed:\n%s", frame)
	}
	frame := saveFrame(m)
	for _, want := range []string{"Nikes", "Ivy", "Memo"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the favorites rows lack %q:\n%s", want, frame)
		}
	}
}

// A2: enter on Ivy plays it alone; the upload after it never queues; a stale vibe
// turn cannot replace the manual queue; the player shows the track.
func TestMusicPlayFromRow(t *testing.T) {
	p := newMusicProvider()
	m, mp := musicTestModel(t, p)
	openFavorites(t, m)
	staleGen := m.vibeAI.gen
	musicFeed(t, m, press(m, "down"))
	musicFeed(t, m, press(m, "enter"))
	if got := mp.setQueueIDs; len(got) != 1 || got[0] != "i.2" {
		t.Fatalf("setQueueIDs = %v, want [i.2] with the upload dropped", got)
	}
	m.Update(vibeAIDoneMsg{gen: staleGen, tracks: vibeTracks()})
	if got := mp.setQueueIDs; len(got) != 1 || got[0] != "i.2" {
		t.Fatalf("a stale vibe turn replaced the manual queue: %v", got)
	}
	if len(m.vibeAI.tracks) != 0 {
		t.Fatal("a stale vibe turn landed its portion over the manual play")
	}
	press(m, "esc")
	press(m, "esc")
	musicStep(m, 1)
	frame := ansi.Strip(m.vibeAIP.m.Frame(40, 10))
	if !strings.Contains(frame, "Ivy") {
		t.Errorf("the player does not show Ivy:\n%s", frame)
	}
}

// A3: enter on the first favorite queues the playable rest of the list from the top.
func TestMusicPlayFromTop(t *testing.T) {
	p := newMusicProvider()
	m, mp := musicTestModel(t, p)
	openFavorites(t, m)
	musicFeed(t, m, press(m, "enter"))
	want := []string{"i.1", "i.2"}
	if got := mp.setQueueIDs; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("setQueueIDs = %v, want %v", got, want)
	}
}

// A4: the albums grid loads the library albums, a library album opens through its
// library endpoint, a catalog one through the catalog, and the row shows the year.
func TestMusicAlbumsRouting(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openSection(t, m, 2)
	if !p.called("GetLibraryAlbums") {
		t.Fatalf("provider calls = %v, want the library albums", p.calls)
	}
	frame := musicFrame(m, 0.4)
	if !strings.Contains(frame, "2016 · album") {
		t.Errorf("the Blonde row hides its year:\n%s", frame)
	}
	musicFeed(t, m, press(m, "enter"))
	if !p.called("GetLibraryAlbumTracks:l.a1") {
		t.Fatalf("provider calls = %v, want the library album tracks", p.calls)
	}
	musicFeed(t, m, press(m, "esc"))
	musicFeed(t, m, press(m, "right"))
	musicFeed(t, m, press(m, "enter"))
	if !p.called("GetAlbumTracks:999") {
		t.Fatalf("provider calls = %v, want the catalog album tracks", p.calls)
	}
}

// A5: the playlists section drops the synthetic favorites playlist.
func TestMusicPlaylistsDropFavorites(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openSection(t, m, 1)
	if !p.called("GetLibraryPlaylists") {
		t.Fatalf("provider calls = %v, want the library playlists", p.calls)
	}
	frame := musicFrame(m, 0.4)
	if !strings.Contains(frame, "Chill") {
		t.Errorf("the playlists rows lack Chill:\n%s", frame)
	}
	if strings.Contains(frame, "Favorites") {
		t.Errorf("the favorites playlist leaked into the rows:\n%s", frame)
	}
}

// A6: typing past the debounce sends one search; the album lands as the hero.
func TestMusicSearchDebounce(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	musicFeed(t, m, press(m, "ctrl+o"))
	musicFeed(t, m, press(m, "b"))
	musicFeed(t, m, press(m, "l"))
	m.vibeAIP.m.st.Step(0.1)
	if p.callCount("Search:bl") != 0 {
		t.Fatalf("the search went out before the pause: %v", p.calls)
	}
	m.vibeAIP.m.st.Step(0.3)
	musicFeed(t, m, m.vibeAIP.m.flush())
	if got := p.callCount("Search:bl"); got != 1 {
		t.Fatalf("Search calls = %d, want one: %v", got, p.calls)
	}
	frame := musicFrame(m, 0.5)
	if !strings.Contains(frame, "Blonde ›") {
		t.Errorf("the search results lack the Blonde hero:\n%s", frame)
	}
}

// A7: ctrl+e on a song appends it to the player queue and says so.
func TestMusicAppendQueues(t *testing.T) {
	p := newMusicProvider()
	m, mp := musicTestModel(t, p)
	openFavorites(t, m)
	musicFeed(t, m, press(m, "ctrl+e"))
	if len(mp.appendQueueIDs) != 1 || len(mp.appendQueueIDs[0]) != 1 || mp.appendQueueIDs[0][0] != "i.1" {
		t.Fatalf("appendQueueIDs = %v, want one call with [i.1]", mp.appendQueueIDs)
	}
	if frame := musicFrame(m, 0.2); !strings.Contains(frame, "queued:") {
		t.Errorf("the queue notice is missing:\n%s", frame)
	}
}

// A8: the row menu's add to favorites reaches the provider through the save path.
func TestMusicMenuFavorite(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openFavorites(t, m)
	musicFeed(t, m, press(m, "ctrl+k"))
	if frame := strings.ReplaceAll(musicFrame(m, 0.05), "▀", " "); !strings.Contains(frame, "add to favorites") {
		t.Fatalf("the menu does not offer add to favorites:\n%s", frame)
	}
	for range 3 {
		musicFeed(t, m, press(m, "down"))
	}
	musicFeed(t, m, press(m, "enter"))
	if len(p.favIDs) != 1 || p.favIDs[0] != "101" {
		t.Fatalf("AddToFavorites = %v, want one call for 101", p.favIDs)
	}
}

// A9: play all on an album tile expands it and queues its tracks.
func TestMusicPlayAll(t *testing.T) {
	p := newMusicProvider()
	m, mp := musicTestModel(t, p)
	openSection(t, m, 2)
	musicFeed(t, m, press(m, "ctrl+k"))
	musicFeed(t, m, press(m, "down"))
	musicFeed(t, m, press(m, "enter"))
	want := []string{"i.b1", "i.b2"}
	if len(mp.setQueueIDs) != 2 || mp.setQueueIDs[0] != want[0] || mp.setQueueIDs[1] != want[1] {
		t.Fatalf("setQueueIDs = %v, want %v", mp.setQueueIDs, want)
	}
}

// A10: a failed load shows the provider's error inside the screen.
func TestMusicLoadError(t *testing.T) {
	p := newMusicProvider()
	p.favErr = errors.New("boom")
	m, _ := musicTestModel(t, p)
	openFavorites(t, m)
	if frame := musicFrame(m, 0.2); !strings.Contains(frame, "Apple Music: boom") {
		t.Errorf("the load error is missing:\n%s", frame)
	}
}

// A11: cover fetches start at most eight at a time; an answered one frees its slot.
func TestMusicCoversThrottled(t *testing.T) {
	m := newModel(nil)
	m.supportsArtColor = func() bool { return true }
	songs := make([]provider.Track, 20)
	for i := range songs {
		songs[i] = provider.Track{ID: fmt.Sprintf("i.%d", i+1), CatalogID: "c", Title: fmt.Sprintf("T%d", i+1),
			ArtworkURL: "https://x/{w}x{h}.jpg"}
	}
	for _, t := range songs {
		m.vibeAIP.m.wantArt(t.ID, t.ArtworkURL)
	}
	if len(m.vibeAIP.m.want) != 20 {
		t.Fatalf("queued covers = %d, want 20", len(m.vibeAIP.m.want))
	}
	if cmd := m.vibeAIP.m.coverCmds(m.artHTTP); cmd == nil {
		t.Fatal("the first coverCmds started nothing")
	}
	if got := len(m.vibeAIP.m.want); got != 12 {
		t.Fatalf("covers left waiting = %d, want 12", got)
	}
	if got := m.vibeAIP.m.inflight; got != 8 {
		t.Fatalf("covers in flight = %d, want 8", got)
	}
	m.handleVibeAIMsg(stageCoverMsg{key: "i.1"})
	if got := m.vibeAIP.m.inflight; got != 7 {
		t.Fatalf("covers in flight after one answer = %d, want 7", got)
	}
	m.vibeAIP.m.coverCmds(m.artHTTP)
	if got := m.vibeAIP.m.inflight; got != 8 {
		t.Fatalf("covers in flight after the next tick = %d, want 8", got)
	}
	if got := len(m.vibeAIP.m.want); got != 11 {
		t.Fatalf("covers left waiting = %d, want 11", got)
	}
}

// A10: the rows alone queue no covers; a frame that draws them puts their keys in
// the queue, and the queue starts at most a full batch of fetches.
func TestMusicCoversOnDemand(t *testing.T) {
	p := newMusicProvider()
	songs := make([]provider.Track, 20)
	for i := range songs {
		songs[i] = provider.Track{ID: fmt.Sprintf("i.%d", i+1), CatalogID: "c", Title: fmt.Sprintf("T%d", i+1),
			ArtworkURL: fmt.Sprintf("https://x/%d/{w}x{h}.jpg", i+1)}
	}
	p.favTracks = songs
	m, _ := musicTestModel(t, p)
	m.supportsArtColor = func() bool { return true }
	openFavorites(t, m)
	v := m.vibeAIP.m
	if got := len(v.want); got != 0 {
		t.Fatalf("covers queued before a draw = %d, want 0", got)
	}
	_ = v.Frame(160, 45)
	_, _ = m.Update(stageTickMsg(time.Now()))
	if v.inflight == 0 || v.inflight > stageCoverMax {
		t.Fatalf("covers in flight = %d, want 1..%d", v.inflight, stageCoverMax)
	}
	if got := len(v.want) + v.inflight; got > len(songs) {
		t.Fatalf("covers queued = %d, want at most the %d drawn rows", got, len(songs))
	}
	for _, c := range v.want {
		if c.key == "" || c.key[0] != 'i' {
			t.Fatalf("a cover was queued for a row the frame did not show: %+v", c)
		}
	}
}

// A12: the agent play path keeps its behavior (covered by the existing tests).
func TestMusicVibeAIPlayUntouched(t *testing.T) {
	m := newModel(newMockPlayer())
	r := m.vibeAI
	r.gen++
	cmd := m.vibeAIPlay(vibeTracks())
	if cmd == nil {
		t.Fatal("vibeAIPlay must start the queue")
	}
	_ = cmd()
	if got := m.queueIDs; len(got) != 3 || got[0] != "1" {
		t.Fatalf("queueIDs = %v, want the three vibe tracks", got)
	}
	cmd = m.vibeAIAppend(vibeTracks())
	if cmd == nil {
		t.Fatal("vibeAIAppend must grow the queue")
	}
	_ = cmd()
	if len(m.queueIDs) != 6 {
		t.Fatalf("queueIDs = %v, want six after the append", m.queueIDs)
	}
}

// B1: a second launch serves the stored favorites even when Apple fails.
func TestMusicLibraryServedFromDisk(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openFavorites(t, m)

	p2 := newMusicProvider()
	p2.favErr = errors.New("offline")
	m2 := newModel(newMockPlayer())
	m2.provider = p2
	m2.stateCh = nil
	stageIntoPlayer(m2)
	openFavorites(t, m2)
	frame := musicFrame(m2, 0.5)
	for _, want := range []string{"Nikes", "Ivy"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the stored favorites lack %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "offline") {
		t.Errorf("a failed background refresh replaced the stored rows:\n%s", frame)
	}
}

// B2: the background refresh on the first use swaps in what changed in Apple.
func TestMusicLibraryBackgroundRefresh(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openFavorites(t, m)

	p2 := newMusicProvider()
	p2.favTracks = append(p2.favTracks, provider.Track{ID: "i.4", CatalogID: "104", Title: "Pink + White", Artist: "Frank Ocean"})
	m2 := newModel(newMockPlayer())
	m2.provider = p2
	m2.stateCh = nil
	stageIntoPlayer(m2)
	openFavorites(t, m2)
	if got := p2.callCount("GetLibraryTracks"); got != 1 {
		t.Fatalf("GetLibraryTracks = %d, want one background refresh", got)
	}
	if frame := musicFrame(m2, 0.5); !strings.Contains(frame, "Pink + White") {
		t.Errorf("the refreshed favorite is missing:\n%s", frame)
	}
}

// B3: ctrl+r over a loaded section asks Apple again and shows the fresh rows.
func TestMusicCtrlRRefreshes(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openSection(t, m, 3)
	p.libOnly = []provider.Track{{ID: "i.9", CatalogID: "109", Title: "Fresh one", Artist: "X"}}
	before := p.callCount("GetLibraryTracks")
	musicFeed(t, m, press(m, "ctrl+r"))
	if got := p.callCount("GetLibraryTracks"); got != before+1 {
		t.Fatalf("GetLibraryTracks = %d, want %d after ctrl+r", got, before+1)
	}
	if frame := musicFrame(m, 0.5); !strings.Contains(frame, "Fresh one") {
		t.Errorf("ctrl+r did not show the fresh row:\n%s", frame)
	}
}

// B4: the same search inside ten minutes is answered from memory.
func TestMusicSearchRemembered(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	musicFeed(t, m, press(m, "ctrl+o"))
	search := func() {
		musicFeed(t, m, press(m, "b"))
		m.vibeAIP.m.st.Step(0.4)
		musicFeed(t, m, m.vibeAIP.m.flush())
	}
	search()
	musicFeed(t, m, press(m, "backspace"))
	m.vibeAIP.m.st.Step(0.4)
	musicFeed(t, m, m.vibeAIP.m.flush())
	search()
	if got := p.callCount("Search:b"); got != 1 {
		t.Fatalf("Search calls = %d, want one: %v", got, p.calls)
	}
}

// B5: a song saved to Favorites shows in the stored Favorites without a rescan.
func TestMusicSaveNotesLibrary(t *testing.T) {
	p := newMusicProvider()
	p.libOnly = []provider.Track{{ID: "i.9", CatalogID: "109", Title: "Not loved", Artist: "X"}}
	m, _ := musicTestModel(t, p)
	openSection(t, m, 3)
	musicFeed(t, m, func() tea.Msg { return SaveTrackMsg{Track: p.libOnly[0], Favorite: true} })
	r, err := m.musicLibrary().Load(context.Background(), library.Songs, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, t := range library.Favorites(r.Songs) {
		found = found || t.ID == "i.9"
	}
	if !found {
		t.Fatalf("the saved song is not among the stored Favorites")
	}
}

// B6: the agent reads the library from the store, catalog IDs first.
func TestMusicAgentLibraryFromStore(t *testing.T) {
	p := newMusicProvider()
	p.libOnly = []provider.Track{{ID: "i.9", CatalogID: "109", Title: "Not loved", Artist: "X"}}
	m, _ := musicTestModel(t, p)
	songs, favs, err := m.vibeAILibrary()(context.Background())
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}
	if len(songs) != 4 || songs[0].ID != "101" || songs[2].ID != "i.3" {
		t.Fatalf("songs = %+v, want catalog IDs and the upload's library ID", songs)
	}
	if len(favs) != 3 || favs[1].Title != "Ivy" {
		t.Fatalf("favorites = %+v", favs)
	}
}

// B7: a playlist the agent created brings the playlists section up to date.
func TestMusicAgentPlaylistRefreshes(t *testing.T) {
	p := newMusicProvider()
	m, _ := musicTestModel(t, p)
	openSection(t, m, 1)
	p.playlists = append(p.playlists, provider.Playlist{ID: "p.new", Name: "aux mix", TrackCount: 5})
	musicFeed(t, m, func() tea.Msg { return vibeAISavedMsg{id: "p.new", name: "aux mix", n: 5, created: true} })
	if frame := musicFrame(m, 0.5); !strings.Contains(frame, "aux mix") {
		t.Errorf("the new playlist is missing:\n%s", frame)
	}
}
