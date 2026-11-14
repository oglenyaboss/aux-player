package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/provider"
)

// fakeProvider implements provider.Provider with canned data and counts
// calls per method. It deliberately has no LibrarySongCount method; the
// countProvider wrapper adds one.
type fakeProvider struct {
	mu    sync.Mutex
	calls map[string]int

	libraryTracks         []provider.Track
	libraryTracksErr      error
	libraryAlbums         []provider.Album
	libraryPlaylists      []provider.Playlist
	libraryAlbumTracks    map[string][]provider.Track
	albumTracks           map[string][]provider.Track
	playlistTracks        map[string][]provider.Track
	catalogPlaylistTracks map[string][]provider.Track

	blockLibraryAlbums func()
	blockLibraryTracks func()
}

func (f *fakeProvider) count(method string) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[method]++
	f.mu.Unlock()
}

func (f *fakeProvider) callsFor(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeProvider) setLibraryTracks(tracks []provider.Track) {
	f.mu.Lock()
	f.libraryTracks = tracks
	f.mu.Unlock()
}

func (f *fakeProvider) setLibraryTracksErr(err error) {
	f.mu.Lock()
	f.libraryTracksErr = err
	f.mu.Unlock()
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Search(_ context.Context, _ string) (*provider.SearchResult, error) {
	f.count("Search")
	return &provider.SearchResult{}, nil
}

func (f *fakeProvider) GetLibraryTracks(_ context.Context) ([]provider.Track, error) {
	f.count("GetLibraryTracks")
	if f.blockLibraryTracks != nil {
		f.blockLibraryTracks()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.libraryTracks, f.libraryTracksErr
}

func (f *fakeProvider) GetLibraryPlaylists(_ context.Context) ([]provider.Playlist, error) {
	f.count("GetLibraryPlaylists")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.libraryPlaylists, nil
}

func (f *fakeProvider) GetLibraryAlbums(_ context.Context) ([]provider.Album, error) {
	f.count("GetLibraryAlbums")
	if f.blockLibraryAlbums != nil {
		f.blockLibraryAlbums()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.libraryAlbums, nil
}

func (f *fakeProvider) GetPlaylistTracks(_ context.Context, playlistID string) ([]provider.Track, error) {
	f.count("GetPlaylistTracks")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playlistTracks[playlistID], nil
}

func (f *fakeProvider) GetAlbumTracks(_ context.Context, albumID string) ([]provider.Track, error) {
	f.count("GetAlbumTracks")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.albumTracks[albumID], nil
}

func (f *fakeProvider) GetLibraryAlbumTracks(_ context.Context, albumID string) ([]provider.Track, error) {
	f.count("GetLibraryAlbumTracks")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.libraryAlbumTracks[albumID], nil
}

func (f *fakeProvider) GetArtistAlbums(_ context.Context, _ string) ([]provider.Album, error) {
	f.count("GetArtistAlbums")
	return nil, nil
}

func (f *fakeProvider) GetCatalogPlaylistTracks(_ context.Context, playlistID string) ([]provider.Track, error) {
	f.count("GetCatalogPlaylistTracks")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catalogPlaylistTracks[playlistID], nil
}

func (f *fakeProvider) CreatePlaylist(_ context.Context, _ string, _ []string) (provider.Playlist, error) {
	f.count("CreatePlaylist")
	return provider.Playlist{}, nil
}

func (f *fakeProvider) LoveSong(_ context.Context, _ string, _ bool) error {
	f.count("LoveSong")
	return nil
}

func (f *fakeProvider) GetSongRating(_ context.Context, _ string) (bool, error) {
	f.count("GetSongRating")
	return false, nil
}

func (f *fakeProvider) AddToLibrary(_ context.Context, _ string) error {
	f.count("AddToLibrary")
	return nil
}

func (f *fakeProvider) AddToFavorites(_ context.Context, _ string) error {
	f.count("AddToFavorites")
	return nil
}

func (f *fakeProvider) AddToPlaylist(_ context.Context, _, _ string) error {
	f.count("AddToPlaylist")
	return nil
}

func (f *fakeProvider) GetRecommendations(_ context.Context) ([]provider.RecommendationGroup, error) {
	f.count("GetRecommendations")
	return nil, nil
}

func (f *fakeProvider) GetStationTracks(_ context.Context, _ string) ([]provider.Track, error) {
	f.count("GetStationTracks")
	return nil, nil
}

func (f *fakeProvider) IsAuthenticated() bool { return true }

// countProvider adds the optional LibrarySongCount to a fakeProvider.
type countProvider struct {
	*fakeProvider
	count     int
	countErr  error
	countHits int
}

func (c *countProvider) LibrarySongCount(_ context.Context) (int, error) {
	c.mu.Lock()
	c.countHits++
	count, err := c.count, c.countErr
	c.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return count, nil
}

func testTrack(id string, favorite bool) provider.Track {
	return provider.Track{ID: id, Title: "Song " + id, Artist: "Artist", Favorite: favorite}
}

func clockedStore(t *testing.T, prov provider.Provider, token string) (*Store, *time.Time) {
	t.Helper()
	s := Open(t.TempDir(), prov, token)
	now := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, &now
}

func readStored(t *testing.T, s *Store, k Kind, id string) entry {
	t.Helper()
	data, err := os.ReadFile(s.path(k, id))
	if err != nil {
		t.Fatalf("read %s: %v", s.path(k, id), err)
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("decode %s: %v", s.path(k, id), err)
	}
	return e
}

// A1: a cold Load fetches once and writes a 0600 file; a warm Load on a new
// Store serves the same songs from disk with no fetch, flagged Due.
func TestLoad_SongsColdThenWarm(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false), testTrack("i.2", true)}}
	dir := t.TempDir()
	s := Open(dir, fake, "token-a")

	r, err := s.Load(context.Background(), Songs, "")
	if err != nil {
		t.Fatalf("cold Load: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 1 {
		t.Fatalf("cold Load fetched %d times, want 1", got)
	}
	if r.Due {
		t.Error("cold Load Due = true, want false")
	}
	if len(r.Songs) != 2 || r.Songs[1].ID != "i.2" {
		t.Fatalf("cold Load songs = %+v", r.Songs)
	}
	ownerDir := filepath.Join(dir, s.owner)
	if info, err := os.Stat(ownerDir); err != nil {
		t.Fatalf("owner dir: %v", err)
	} else if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("owner dir mode = %o, want 700", perm)
	}
	info, err := os.Stat(filepath.Join(ownerDir, "songs.json"))
	if err != nil {
		t.Fatalf("songs file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("songs file mode = %o, want 600", perm)
	}

	warm := Open(dir, fake, "token-a")
	r2, err := warm.Load(context.Background(), Songs, "")
	if err != nil {
		t.Fatalf("warm Load: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 1 {
		t.Fatalf("warm Load fetched %d more times, want 0", got-1)
	}
	if len(r2.Songs) != 2 || r2.Songs[0].ID != "i.1" || !r2.Songs[1].Favorite {
		t.Fatalf("warm Load songs = %+v", r2.Songs)
	}
	if !r2.Due {
		t.Error("warm Load Due = false, want true (first use in this process)")
	}
}

// A2: Favorites keeps flagged tracks in order.
func TestFavoritesKeepsOrder(t *testing.T) {
	songs := []provider.Track{
		testTrack("a", false),
		testTrack("b", true),
		testTrack("c", false),
		testTrack("d", true),
	}
	got := Favorites(songs)
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "d" {
		t.Fatalf("Favorites = %+v, want [b d] in order", got)
	}
}

// A3: the count probe skips the scan inside the window and advances only
// checked; outside the window, on a changed count, or without the method it
// scans.
func TestRefresh_SongsCountProbe(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	cp := &countProvider{fakeProvider: fake, count: 1}
	s, now := clockedStore(t, cp, "tok")

	if _, _, err := s.Refresh(context.Background(), Songs, "", true); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}

	*now = now.Add(time.Hour)
	_, changed, err := s.Refresh(context.Background(), Songs, "", false)
	if err != nil {
		t.Fatalf("probe Refresh: %v", err)
	}
	if changed {
		t.Error("probe Refresh changed = true, want false")
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 1 {
		t.Fatalf("probe Refresh scanned %d times, want 0", got-1)
	}
	if cp.countHits != 1 {
		t.Errorf("count probed %d times, want 1", cp.countHits)
	}
	e := readStored(t, s, Songs, "")
	if !e.Fetched.Equal(now.Add(-time.Hour)) {
		t.Errorf("fetched = %v, want unchanged %v", e.Fetched, now.Add(-time.Hour))
	}
	if !e.Checked.Equal(*now) {
		t.Errorf("checked = %v, want %v", e.Checked, now)
	}

	*now = now.Add(7 * time.Hour)
	if _, changed, err := s.Refresh(context.Background(), Songs, "", false); err != nil {
		t.Fatalf("stale probe Refresh: %v", err)
	} else if changed {
		t.Error("stale scan changed = true, want false (same data)")
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 2 {
		t.Fatalf("fetch 7h later scanned %d times, want 1", got-1)
	}

	cp.mu.Lock()
	cp.count = 5
	cp.mu.Unlock()
	*now = now.Add(time.Minute)
	if _, _, err := s.Refresh(context.Background(), Songs, "", false); err != nil {
		t.Fatalf("changed-count Refresh: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 3 {
		t.Fatalf("changed count scanned %d times, want 1", got-2)
	}

	noCount := Open(t.TempDir(), fake, "tok")
	fresh := now.Add(time.Minute)
	noCount.now = func() time.Time { return fresh }
	if _, _, err := noCount.Refresh(context.Background(), Songs, "", false); err != nil {
		t.Fatalf("no-count Refresh: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 4 {
		t.Fatalf("no count method scanned %d times, want 1", got-3)
	}
}

// A4: changed is true when the content differs, false when it is equal.
func TestRefresh_ChangedDetection(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	s := Open(t.TempDir(), fake, "tok")
	ctx := context.Background()

	if _, changed, err := s.Refresh(ctx, Songs, "", true); err != nil || !changed {
		t.Fatalf("first Refresh = %v, %t; want ok, true", err, changed)
	}
	fake.setLibraryTracks([]provider.Track{{
		ID: "i.1", Title: "Remastered", Artist: "Artist",
	}})
	if _, changed, err := s.Refresh(ctx, Songs, "", true); err != nil || !changed {
		t.Fatalf("title change = %v, %t; want ok, true", err, changed)
	}
	if _, changed, err := s.Refresh(ctx, Songs, "", true); err != nil || changed {
		t.Fatalf("equal data = %v, %t; want ok, false", err, changed)
	}
}

// A5: concurrent refreshes share one fetch and its result.
func TestRefresh_SingleFlight(t *testing.T) {
	started := make(chan struct{})
	var startOnce sync.Once
	release := make(chan struct{})
	fake := &fakeProvider{
		libraryAlbums: []provider.Album{{ID: "l.1", Title: "Album", Artist: "Artist"}},
		blockLibraryAlbums: func() {
			startOnce.Do(func() { close(started) })
			<-release
		},
	}
	s := Open(t.TempDir(), fake, "tok")

	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	refresh := func(i int) {
		wg.Go(func() {
			results[i], _, errs[i] = s.Refresh(context.Background(), Albums, "", true)
		})
	}
	refresh(0)
	<-started
	refresh(1)
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := fake.callsFor("GetLibraryAlbums"); got != 1 {
		t.Fatalf("GetLibraryAlbums called %d times, want 1", got)
	}
	for i := range 2 {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if len(results[i].Albums) != 1 || results[i].Albums[0].ID != "l.1" {
			t.Fatalf("goroutine %d result = %+v", i, results[i])
		}
	}
	if !reflect.DeepEqual(results[0], results[1]) {
		t.Fatalf("results differ: %+v vs %+v", results[0], results[1])
	}
}

// A6: the due policy across kinds, ages and the per-process checked set.
func TestDue(t *testing.T) {
	ctx := context.Background()
	fake := &fakeProvider{
		libraryTracks:      []provider.Track{testTrack("i.1", false)},
		albumTracks:        map[string][]provider.Track{"123": {testTrack("1", false)}},
		libraryAlbumTracks: map[string][]provider.Track{"l.1": {testTrack("i.a", false)}},
	}
	dir := t.TempDir()

	s, now := clockedStore(t, fake, "tok")
	if _, err := s.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load songs: %v", err)
	}
	if r, err := s.Load(ctx, Songs, ""); err != nil || r.Due {
		t.Fatalf("just refreshed Due = %t (err %v), want false", r.Due, err)
	}
	*now = now.Add(16 * time.Minute)
	if r, err := s.Load(ctx, Songs, ""); err != nil || !r.Due {
		t.Fatalf("16 min later Due = %t (err %v), want true", r.Due, err)
	}

	cat := Open(dir, fake, "tok")
	catNow := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	cat.now = func() time.Time { return catNow }
	if _, err := cat.Load(ctx, Album, "123"); err != nil {
		t.Fatalf("Load catalog album: %v", err)
	}
	catNow = catNow.Add(29 * 24 * time.Hour)
	if r, err := cat.Load(ctx, Album, "123"); err != nil || r.Due {
		t.Fatalf("catalog album 29 days Due = %t (err %v), want false", r.Due, err)
	}
	catNow = catNow.Add(2 * 24 * time.Hour)
	if r, err := cat.Load(ctx, Album, "123"); err != nil || !r.Due {
		t.Fatalf("catalog album 31 days Due = %t (err %v), want true", r.Due, err)
	}

	lib := Open(dir, fake, "tok")
	libNow := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	lib.now = func() time.Time { return libNow }
	if _, err := lib.Load(ctx, Album, "l.1"); err != nil {
		t.Fatalf("Load library album: %v", err)
	}
	never := Open(dir, fake, "tok")
	never.now = func() time.Time { return libNow }
	if r, err := never.Load(ctx, Album, "l.1"); err != nil || !r.Due {
		t.Fatalf("library album never checked Due = %t (err %v), want true", r.Due, err)
	}
}

// A7: ids route to the right provider call and the Favorites entry is dropped.
func TestRouting(t *testing.T) {
	fake := &fakeProvider{
		libraryAlbumTracks:    map[string][]provider.Track{"l.1": {testTrack("i.a", false)}},
		albumTracks:           map[string][]provider.Track{"123": {testTrack("1", false)}},
		playlistTracks:        map[string][]provider.Track{"p.1": {testTrack("i.p", false)}},
		catalogPlaylistTracks: map[string][]provider.Track{"pl.9": {testTrack("9", false)}},
		libraryPlaylists: []provider.Playlist{
			{ID: provider.FavoritesPlaylistID, Name: "Favorites", ReadOnly: true},
			{ID: "p.1", Name: "Mix"},
		},
	}
	s := Open(t.TempDir(), fake, "tok")
	ctx := context.Background()

	if _, err := s.Load(ctx, Album, "l.1"); err != nil {
		t.Fatalf("Load l.1: %v", err)
	}
	if _, err := s.Load(ctx, Album, "123"); err != nil {
		t.Fatalf("Load 123: %v", err)
	}
	if _, err := s.Load(ctx, Playlist, "p.1"); err != nil {
		t.Fatalf("Load p.1: %v", err)
	}
	if _, err := s.Load(ctx, Playlist, "pl.9"); err != nil {
		t.Fatalf("Load pl.9: %v", err)
	}
	for method, want := range map[string]int{
		"GetLibraryAlbumTracks":    1,
		"GetAlbumTracks":           1,
		"GetPlaylistTracks":        1,
		"GetCatalogPlaylistTracks": 1,
	} {
		if got := fake.callsFor(method); got != want {
			t.Errorf("%s called %d times, want %d", method, got, want)
		}
	}

	r, err := s.Load(ctx, Playlists, "")
	if err != nil {
		t.Fatalf("Load playlists: %v", err)
	}
	if len(r.Playlists) != 1 || r.Playlists[0].ID != "p.1" {
		t.Fatalf("playlists = %+v, want only p.1", r.Playlists)
	}
}

// A8: NoteSaved adds or flags a track without touching fetched.
func TestNoteSaved(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	s := Open(t.TempDir(), fake, "tok")
	ctx := context.Background()

	if _, err := s.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
	fetched := readStored(t, s, Songs, "").Fetched

	s.NoteSaved(provider.Track{ID: "i.2", Title: "New"}, true)
	e := readStored(t, s, Songs, "")
	if len(e.Songs) != 2 || e.Songs[1].ID != "i.2" || !e.Songs[1].Favorite {
		t.Fatalf("after new save songs = %+v", e.Songs)
	}
	if !e.Fetched.Equal(fetched) {
		t.Errorf("fetched = %v, want unchanged %v", e.Fetched, fetched)
	}

	s.NoteSaved(testTrack("i.1", false), true)
	e = readStored(t, s, Songs, "")
	if len(e.Songs) != 2 || !e.Songs[0].Favorite {
		t.Fatalf("after existing save songs = %+v, want i.1 flagged and not duplicated", e.Songs)
	}
	if r, err := s.Load(ctx, Songs, ""); err != nil || len(r.Songs) != 2 {
		t.Fatalf("Load after saves = %v, %+v", err, r.Songs)
	}

	s.NoteSaved(provider.Track{ID: "i.3", CatalogID: "777", Title: "Library"}, false)
	s.NoteSaved(provider.Track{ID: "777", CatalogID: "777", Title: "From search"}, true)
	if e = readStored(t, s, Songs, ""); len(e.Songs) != 3 || e.Songs[2].ID != "i.3" || !e.Songs[2].Favorite {
		t.Fatalf("a catalog save of a library song = %+v, want i.3 flagged and not duplicated", e.Songs)
	}

	bare := Open(t.TempDir(), fake, "tok")
	bare.NoteSaved(testTrack("i.9", true), false)
	if _, err := os.Stat(bare.path(Songs, "")); !os.IsNotExist(err) {
		t.Fatalf("songs file created without an entry: %v", err)
	}
}

// A9: Forget makes the next Load fetch again.
func TestForget(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	s := Open(t.TempDir(), fake, "tok")
	ctx := context.Background()

	if _, err := s.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.Forget(Songs, "")
	if _, err := os.Stat(s.path(Songs, "")); !os.IsNotExist(err) {
		t.Fatalf("file survived Forget: %v", err)
	}
	if _, err := s.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load after Forget: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 2 {
		t.Fatalf("GetLibraryTracks called %d times after Forget, want 2", got)
	}
}

// A10: each token owns its directory; opening with another token wipes the
// previous owner's data, and an empty dir deletes nothing.
func TestOwnerIsolation(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	dir := t.TempDir()
	ctx := context.Background()

	a := Open(dir, fake, "a")
	if _, err := a.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load as a: %v", err)
	}
	aDir := filepath.Join(dir, a.owner)

	Open(dir, fake, "b")
	if _, err := os.Stat(aDir); err != nil {
		t.Fatalf("a recently used owner dir was deleted: %v", err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(aDir, old, old); err != nil {
		t.Fatal(err)
	}
	b := Open(dir, fake, "b")
	if _, err := os.Stat(aDir); !os.IsNotExist(err) {
		t.Fatalf("an owner dir unused for 31 days survived: %v", err)
	}
	if _, err := b.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load as b: %v", err)
	}
	if got := fake.callsFor("GetLibraryTracks"); got != 2 {
		t.Fatalf("GetLibraryTracks called %d times, want a miss for b (2)", got)
	}
	if _, err := os.Stat(filepath.Join(dir, b.owner, "songs.json")); err != nil {
		t.Fatalf("b's songs file: %v", err)
	}

	keep := t.TempDir()
	inner := filepath.Join(keep, "data")
	if err := os.Mkdir(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	Open("", fake, "b")
	if _, err := os.Stat(inner); err != nil {
		t.Fatalf("empty dir deleted something: %v", err)
	}
}

// A11: provider errors surface on cold loads and fail refreshes without
// harming stored data.
func TestProviderErrors(t *testing.T) {
	ctx := context.Background()

	cold := &fakeProvider{libraryTracksErr: errors.New("apple down")}
	s := Open(t.TempDir(), cold, "tok")
	if _, err := s.Load(ctx, Songs, ""); err == nil {
		t.Fatal("cold Load with a provider error returned nil error")
	}
	if _, err := os.Stat(s.path(Songs, "")); !os.IsNotExist(err) {
		t.Fatalf("cold Load wrote a file: %v", err)
	}

	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	dir := t.TempDir()
	s2 := Open(dir, fake, "tok")
	if _, err := s2.Load(ctx, Songs, ""); err != nil {
		t.Fatalf("Load: %v", err)
	}
	fake.setLibraryTracksErr(errors.New("apple down"))
	if _, _, err := s2.Refresh(ctx, Songs, "", true); err == nil {
		t.Fatal("Refresh with a provider error returned nil error")
	}
	r, err := s2.Load(ctx, Songs, "")
	if err != nil || len(r.Songs) != 1 {
		t.Fatalf("Load after failed Refresh = %v, %+v; want stored songs", err, r.Songs)
	}

	reopened := Open(dir, fake, "tok")
	r, err = reopened.Load(ctx, Songs, "")
	if err != nil || len(r.Songs) != 1 {
		t.Fatalf("reopened Load = %v, %+v; want stored songs without a fetch", err, r.Songs)
	}
}

// A refresh whose context ran out, or that came back empty over stored rows,
// keeps the stored rows: a cut scan must not pass for the whole library.
func TestRefreshKeepsStoredOnShortAnswer(t *testing.T) {
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false), testTrack("i.2", true)}}
	s := Open(t.TempDir(), fake, "tok")
	if _, err := s.Load(context.Background(), Songs, ""); err != nil {
		t.Fatal(err)
	}

	fake.setLibraryTracks([]provider.Track{testTrack("i.1", false)})
	cut, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.Refresh(cut, Songs, "", true); err == nil {
		t.Fatal("a refresh past its context stored its answer")
	}

	fake.setLibraryTracks(nil)
	if _, _, err := s.Refresh(context.Background(), Songs, "", true); err == nil {
		t.Fatal("an empty answer replaced stored songs")
	}

	r, err := s.Load(context.Background(), Songs, "")
	if err != nil || len(r.Songs) != 2 {
		t.Fatalf("stored songs = %v, %+v; want the two stored", err, r.Songs)
	}
	if e := readStored(t, s, Songs, ""); len(e.Songs) != 2 {
		t.Fatalf("songs file = %+v, want the two stored", e.Songs)
	}
}

// A save noted while a songs refresh is out survives the refresh's answer.
func TestNoteSavedDuringRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	fake := &fakeProvider{libraryTracks: []provider.Track{testTrack("i.1", false)}}
	s := Open(t.TempDir(), fake, "tok")
	if _, err := s.Load(context.Background(), Songs, ""); err != nil {
		t.Fatal(err)
	}
	fake.blockLibraryTracks = func() {
		once.Do(func() { close(started) })
		<-release
	}
	done := make(chan Result)
	go func() {
		r, _, _ := s.Refresh(context.Background(), Songs, "", true)
		done <- r
	}()
	<-started
	s.NoteSaved(provider.Track{ID: "i.9", Title: "Saved"}, true)
	close(release)
	r := <-done

	has := func(songs []provider.Track) bool {
		for _, t := range songs {
			if t.ID == "i.9" && t.Favorite {
				return true
			}
		}
		return false
	}
	if !has(r.Songs) {
		t.Fatalf("refresh result lost the save: %+v", r.Songs)
	}
	if e := readStored(t, s, Songs, ""); !has(e.Songs) {
		t.Fatalf("songs file lost the save: %+v", e.Songs)
	}
}
