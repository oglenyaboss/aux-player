package apple_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/provider/apple"
)

// newTestProvider creates an AppleProvider pointed at a test server.
func newTestProvider(t *testing.T, srv *httptest.Server) *apple.AppleProvider {
	t.Helper()
	cfg := &config.Config{ //nolint:gosec // G101: test credentials, not real secrets
		AppleDeveloperToken: "test-dev-token",
		AppleUserToken:      "test-user-token",
		StoreFront:          "us",
	}
	p := apple.New(cfg)
	p.SetBaseURL(srv.URL) // injected for tests
	return p
}

func newAuthedConfig() *config.Config {
	return &config.Config{
		AppleDeveloperToken: "dev",
		AppleUserToken:      "usr",
		StoreFront:          "us",
		AuthPort:            7777,
	}
}

// --- IsAuthenticated ---

func TestIsAuthenticated_BothTokensPresent(t *testing.T) {
	p := apple.New(newAuthedConfig())
	if !p.IsAuthenticated() {
		t.Error("expected authenticated, got false")
	}
}

func TestIsAuthenticated_MissingDevToken(t *testing.T) {
	cfg := newAuthedConfig()
	cfg.AppleDeveloperToken = ""
	p := apple.New(cfg)
	if p.IsAuthenticated() {
		t.Error("expected not authenticated (missing dev token)")
	}
}

func TestIsAuthenticated_MissingUserToken(t *testing.T) {
	cfg := newAuthedConfig()
	cfg.AppleUserToken = ""
	p := apple.New(cfg)
	if p.IsAuthenticated() {
		t.Error("expected not authenticated (missing user token)")
	}
}

func TestName(t *testing.T) {
	p := apple.New(newAuthedConfig())
	if p.Name() != "apple" {
		t.Errorf("Name() = %q, want %q", p.Name(), "apple")
	}
}

// --- Search ---

// searchHandler routes the two Search sub-requests by path:
//
//	/me/library/search   → libraryResp
//	/catalog/{sf}/search → catalogResp  (albums/playlists only; no songs)
func searchHandler(t *testing.T, w http.ResponseWriter, r *http.Request, libraryResp, catalogResp any) {
	t.Helper()
	if strings.Contains(r.URL.Path, "/me/library/search") {
		writeJSON(t, w, libraryResp)
	} else {
		writeJSON(t, w, catalogResp)
	}
}

func TestSearch_ReturnsTracksAlbumsPlaylists(t *testing.T) {
	// Library song comes first; catalog adds album and playlist.
	libSong := songJSON("i.Abc123", "Humble", "Kendrick Lamar", "DAMN.", 212000, "https://art/{w}x{h}.jpg")
	libResp := map[string]any{
		"results": map[string]any{
			"library-songs":     map[string]any{"data": []any{libSong}},
			"library-albums":    map[string]any{"data": []any{}},
			"library-playlists": map[string]any{"data": []any{}},
		},
	}
	catResp := map[string]any{
		"results": map[string]any{
			"songs":     map[string]any{"data": []any{}},
			"albums":    map[string]any{"data": []any{albumJSON("a1", "DAMN.", "Kendrick Lamar", 14)}},
			"playlists": map[string]any{"data": []any{playlistJSON("p1", "Hip Hop Hits", 50)}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-dev-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Music-User-Token") != "test-user-token" {
			t.Errorf("Music-User-Token header = %q", r.Header.Get("Music-User-Token"))
		}
		searchHandler(t, w, r, libResp, catResp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "kendrick")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(result.Tracks) != 1 {
		t.Fatalf("Tracks: got %d, want 1", len(result.Tracks))
	}
	tk := result.Tracks[0]
	if tk.ID != "i.Abc123" {
		t.Errorf("Track.ID = %q, want i.Abc123", tk.ID)
	}
	if tk.Title != "Humble" {
		t.Errorf("Track.Title = %q", tk.Title)
	}
	if tk.Artist != "Kendrick Lamar" {
		t.Errorf("Track.Artist = %q", tk.Artist)
	}
	if tk.Duration != 212*time.Second {
		t.Errorf("Track.Duration = %v, want 212s", tk.Duration)
	}
	if tk.ArtworkURL != "https://art/300x300.jpg" {
		t.Errorf("Track.ArtworkURL = %q", tk.ArtworkURL)
	}
	if len(result.Albums) != 1 || result.Albums[0].ID != "a1" {
		t.Errorf("Albums mismatch: %+v", result.Albums)
	}
	if len(result.Playlists) != 1 || result.Playlists[0].ID != "p1" {
		t.Errorf("Playlists mismatch: %+v", result.Playlists)
	}
}

func TestSearch_CatalogTracksIncluded(t *testing.T) {
	// Catalog tracks appear when they are not already in the library.
	catSong := songJSON("999", "New Song", "New Artist", "New Album", 180000, "")
	libEmpty := map[string]any{"results": map[string]any{}}
	catResp := map[string]any{
		"results": map[string]any{
			"songs":     map[string]any{"data": []any{catSong}},
			"albums":    map[string]any{"data": []any{}},
			"playlists": map[string]any{"data": []any{}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, libEmpty, catResp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "new song")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "999" {
		t.Errorf("expected catalog track 999, got %+v", result.Tracks)
	}
}

func TestSearch_UnplayableCatalogTracksDropped(t *testing.T) {
	// What the standard catalog endpoint can tell us: playParams must be
	// present (absent means radio-only, which always fails in MusicKit) and
	// kind must be "song" (excluding music videos and radio episodes).
	playable := songJSON("111", "Playable Song", "Artist A", "Album A", 200000, "")
	noPlayParams := songJSONNoPlay("222", "Radio Song", "Artist B", "Album B", 200000)
	musicVideo := songJSONKind("444", "Video Song", "Artist D", "Album D", 200000, "musicVideo")
	libEmpty := map[string]any{"results": map[string]any{}}
	catResp := map[string]any{
		"results": map[string]any{
			"songs":     map[string]any{"data": []any{playable, noPlayParams, musicVideo}},
			"albums":    map[string]any{"data": []any{}},
			"playlists": map[string]any{"data": []any{}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, libEmpty, catResp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "song")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "111" {
		t.Errorf("expected only playable track 111, got %+v", result.Tracks)
	}
}

// Streamability is no longer part of the filter. Only amp-api reports it, and
// requiring a field the supported endpoint never sends is what emptied the
// queue in #93. A purchase-only or region-locked track now reaches the queue
// and fails at playback, which is how catalog album and playlist tracks have
// behaved since #82.
func TestSearch_CatalogSongWithoutAssetURLsKept(t *testing.T) {
	purchaseOnly := songJSONNoStream("333", "Buy Only Song", "Artist C", "Album C", 200000)
	libEmpty := map[string]any{"results": map[string]any{}}
	catResp := map[string]any{
		"results": map[string]any{
			"songs":     map[string]any{"data": []any{purchaseOnly}},
			"albums":    map[string]any{"data": []any{}},
			"playlists": map[string]any{"data": []any{}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, libEmpty, catResp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "song")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "333" {
		t.Errorf("Tracks: got %+v, want track 333 kept despite carrying no streaming URLs", result.Tracks)
	}
}

func TestSearch_LibraryTracksOnly(t *testing.T) {
	// Library song deduplicates the same song from catalog (library wins).
	libSong := songJSON("i.AbCdEf", "Humble", "Kendrick Lamar", "DAMN.", 212000, "")
	catSong := songJSON("1234", "Humble", "Kendrick Lamar", "DAMN.", 212000, "")

	libResp := map[string]any{
		"results": map[string]any{
			"library-songs":     map[string]any{"data": []any{libSong}},
			"library-albums":    map[string]any{"data": []any{}},
			"library-playlists": map[string]any{"data": []any{}},
		},
	}
	catResp := map[string]any{
		"results": map[string]any{
			"songs":     map[string]any{"data": []any{catSong}},
			"albums":    map[string]any{"data": []any{}},
			"playlists": map[string]any{"data": []any{}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, libResp, catResp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "humble")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 1 {
		t.Fatalf("Tracks: got %d, want 1 (deduped)", len(result.Tracks))
	}
	if result.Tracks[0].ID != "i.AbCdEf" {
		t.Errorf("expected library ID i.AbCdEf, got %q", result.Tracks[0].ID)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	empty := map[string]any{"results": map[string]any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, empty, empty)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "zzznoresults")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 0 || len(result.Albums) != 0 || len(result.Playlists) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

func TestSearch_PartialFailureRecordedAsWarning(t *testing.T) {
	// One failing leg must not look like an empty catalog. When the catalog-song
	// search fails but the library and album/playlist legs succeed, Search still
	// returns the library results — and must say the result is incomplete.
	libSong := songJSON("i.AbCdEf", "Humble", "Kendrick Lamar", "DAMN.", 212000, "")
	libResp := map[string]any{
		"results": map[string]any{
			"library-songs":     map[string]any{"data": []any{libSong}},
			"library-playlists": map[string]any{"data": []any{}},
		},
	}
	empty := map[string]any{"results": map[string]any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/me/library/search"):
			writeJSON(t, w, libResp)
		case r.URL.Query().Get("types") == "songs":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			writeJSON(t, w, empty)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "humble")
	if err != nil {
		t.Fatalf("Search: got error %v, want nil (two legs succeeded)", err)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "i.AbCdEf" {
		t.Errorf("Tracks: got %+v, want the library song", result.Tracks)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings: got %d (%v), want 1", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "catalog song search") {
		t.Errorf("Warnings[0] = %q, want it to name the catalog song search", result.Warnings[0])
	}
}

func TestSearch_NoWarningsWhenAllLegsSucceed(t *testing.T) {
	empty := map[string]any{"results": map[string]any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, empty, empty)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "zzznoresults")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings: got %v, want none — an empty result is not a failure", result.Warnings)
	}
}

// The catalog songs leg goes to the supported host, unconditionally and on the
// first attempt. amp-api answered it until #118 demoted it to a fallback; it
// spent months 401ing token pairs the supported endpoints accept, and each
// rejection left discovery refilling from the user's library alone. There is
// no retry left to hide that, so the request must not ask for
// extendedAssetUrls, and a 401 must surface as a warning rather than silently
// producing an empty songs leg.
func TestSearch_CatalogSongsUseTheSupportedHost(t *testing.T) {
	song := songJSONNoStream("777", "Catalog Song", "Artist", "Album", 210000)
	empty := map[string]any{"results": map[string]any{}}

	var songCalls, extendCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "extend=extendedAssetUrls") {
			extendCalls.Add(1)
		}
		switch {
		case strings.Contains(r.URL.Path, "/me/library/search"):
			writeJSON(t, w, empty)
		case r.URL.Query().Get("types") != "songs":
			writeJSON(t, w, empty) // albums/playlists leg
		default:
			songCalls.Add(1)
			writeJSON(t, w, map[string]any{"results": map[string]any{
				"songs": map[string]any{"data": []any{song}},
			}})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "catalog song")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got := songCalls.Load(); got != 1 {
		t.Errorf("catalog song calls: got %d, want exactly 1", got)
	}
	if got := extendCalls.Load(); got != 0 {
		t.Errorf("extend=extendedAssetUrls calls: got %d, want 0", got)
	}
	if len(result.Tracks) != 1 || result.Tracks[0].ID != "777" {
		t.Fatalf("Tracks: got %+v, want the catalog track 777", result.Tracks)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings: got %v, want none", result.Warnings)
	}
}

func TestSearch_CatalogSongRejectionWarnsInsteadOfSilentlyEmptying(t *testing.T) {
	empty := map[string]any{"results": map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/me/library/search"):
			writeJSON(t, w, empty)
		case r.URL.Query().Get("types") != "songs":
			writeJSON(t, w, empty)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "song")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 0 {
		t.Errorf("Tracks: got %+v, want none", result.Tracks)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "catalog song search") {
		t.Errorf("Warnings: got %v, want one naming the catalog song search", result.Warnings)
	}
}

func TestSearch_QueryEncoded(t *testing.T) {
	// Search fans its catalog requests out across goroutines, so the handler
	// runs concurrently and the request log needs a lock.
	var (
		mu      sync.Mutex
		gotURLs []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURLs = append(gotURLs, r.URL.RawQuery)
		mu.Unlock()
		writeJSON(t, w, map[string]any{"results": map[string]any{}})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, _ = p.Search(context.Background(), "lofi hip hop")

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, q := range gotURLs {
		if containsStr(q, "lofi+hip+hop") || containsStr(q, "lofi%20hip%20hop") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("query not encoded correctly in any request: %v", gotURLs)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if !strings.Contains(err.Error(), "Re-run `aux auth login`") {
		t.Fatalf("expected actionable auth guidance, got %q", err)
	}
}

func TestSearch_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writeJSON(t, w, map[string]any{"results": map[string]any{}})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := newTestProvider(t, srv)
	_, err := p.Search(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// --- GetLibraryTracks ---

func TestGetLibraryTracks_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data": []any{
				songJSON("10", "Song A", "Artist A", "Album A", 180000, ""),
				songJSON("11", "Song B", "Artist B", "Album B", 200000, ""),
			},
			"next": "",
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	if tracks[0].Title != "Song A" {
		t.Errorf("tracks[0].Title = %q", tracks[0].Title)
	}
}

func TestGetLibraryTracks_AlbumPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		song := songJSON("i.1", "Evil", "Interpol", "Antics", 216000, "")
		attrs := song["attributes"].(map[string]any)
		attrs["discNumber"] = 1
		attrs["trackNumber"] = 2
		writeJSON(t, w, map[string]any{"data": []any{song}, "next": ""})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	if tracks[0].DiscNumber != 1 || tracks[0].TrackNumber != 2 {
		t.Errorf("disc/track = %d/%d, want 1/2", tracks[0].DiscNumber, tracks[0].TrackNumber)
	}
}

func TestGetLibraryTracks_Pagination(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("1", "Track 1", "Art", "Alb", 100000, "")},
				"next": "/me/library/songs?limit=100&offset=100",
			})
		case 2:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("2", "Track 2", "Art", "Alb", 100000, "")},
				"next": "",
			})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2 (pagination)", len(tracks))
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls for pagination, got %d", calls)
	}
}

func TestGetLibraryTracks_Pagination_404OnNextPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("1", "Track 1", "Art", "Alb", 100000, "")},
				"next": "/me/library/songs?limit=100&offset=100",
			})
		case 2:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 Not Found"))
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: unexpected error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1 (gracefully stopped on 404 next page)", len(tracks))
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

func TestGetLibraryTracks_PaginationWithV1Prefix(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Contains(r.URL.Path, "/v1/v1/") {
			t.Errorf("unexpected path with double /v1/: %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch calls {
		case 1:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("1", "Track 1", "Art", "Alb", 100000, "")},
				"next": "/v1/me/library/songs?limit=100&offset=100",
			})
		case 2:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("2", "Track 2", "Art", "Alb", 100000, "")},
				"next": "",
			})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2 (pagination with v1 prefix)", len(tracks))
	}
	if calls != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

func TestGetLibraryTracks_PaginationParallel(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := atomic.AddInt32(&calls, 1)
		if val == 1 {
			var songs []any
			for range 100 {
				songs = append(songs, songJSON("1", "Track 1", "Art", "Alb", 100000, ""))
			}
			writeJSON(t, w, map[string]any{
				"data": songs,
				"next": "/me/library/songs?limit=100&offset=100",
				"meta": map[string]any{
					"total": 101,
				},
			})
		} else {
			if r.URL.Query().Get("offset") != "100" {
				t.Errorf("unexpected offset: %q", r.URL.Query().Get("offset"))
			}
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("2", "Track 2", "Art", "Alb", 100000, "")},
			})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 101 {
		t.Fatalf("got %d tracks, want 101", len(tracks))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

// --- GetLibraryPlaylists ---

func TestGetLibraryPlaylists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data": []any{
				playlistJSON("pl1", "Favorites", 10),
				playlistJSON("pl2", "Workout", 25),
			},
			"next": "",
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	lists, err := p.GetLibraryPlaylists(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryPlaylists: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("got %d playlists, want 2", len(lists))
	}
	if lists[0].Name != "Favorites" {
		t.Errorf("Name = %q", lists[0].Name)
	}
	if lists[1].TrackCount != 25 {
		t.Errorf("TrackCount = %d", lists[1].TrackCount)
	}
}

func TestGetLibraryPlaylists_PaginationParallel(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := atomic.AddInt32(&calls, 1)
		if val == 1 {
			var playlists []any
			for range 100 {
				playlists = append(playlists, playlistJSON("pl", "Workout", 25))
			}
			writeJSON(t, w, map[string]any{
				"data": playlists,
				"next": "/me/library/playlists?limit=100&offset=100",
				"meta": map[string]any{
					"total": 101,
				},
			})
		} else {
			if r.URL.Query().Get("offset") != "100" {
				t.Errorf("unexpected offset: %q", r.URL.Query().Get("offset"))
			}
			writeJSON(t, w, map[string]any{
				"data": []any{playlistJSON("pl2", "Another", 10)},
			})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	lists, err := p.GetLibraryPlaylists(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryPlaylists: %v", err)
	}
	if len(lists) != 102 {
		t.Fatalf("got %d playlists, want 102", len(lists))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", calls)
	}
}

// --- GetLibraryAlbums ---

func TestGetLibraryAlbums_ReturnsAllPagesInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/albums" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		switch r.URL.Query().Get("offset") {
		case "":
			writeJSON(t, w, map[string]any{
				"data": []any{
					libraryAlbumJSON("l.a1", "Blonde", "Frank Ocean", 17, "https://art/{w}x{h}.jpg"),
					libraryAlbumJSON("l.a2", "SOS", "SZA", 23, ""),
				},
				"next": "/v1/me/library/albums?offset=100",
			})
		case "100":
			if got := r.URL.Query().Get("limit"); got != "100" {
				t.Errorf("next page limit = %q, want 100", got)
			}
			writeJSON(t, w, map[string]any{
				"data": []any{libraryAlbumJSON("l.a3", "Isolation", "Kali Uchis", 15, "")},
			})
		default:
			t.Errorf("unexpected offset %q", r.URL.Query().Get("offset"))
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	albums, err := p.GetLibraryAlbums(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryAlbums: %v", err)
	}
	want := []struct {
		id, title, artist, art string
		trackCount             int
	}{
		{"l.a1", "Blonde", "Frank Ocean", "https://art/300x300.jpg", 17},
		{"l.a2", "SOS", "SZA", "", 23},
		{"l.a3", "Isolation", "Kali Uchis", "", 15},
	}
	if len(albums) != len(want) {
		t.Fatalf("albums = %+v, want %d", albums, len(want))
	}
	for i, expected := range want {
		got := albums[i]
		if got.ID != expected.id || got.Title != expected.title || got.Artist != expected.artist ||
			got.ArtworkURL != expected.art || got.TrackCount != expected.trackCount {
			t.Errorf("album %d = %+v, want %+v", i, got, expected)
		}
	}
}

func TestGetLibraryAlbums_EmptyLibrary404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":404,"message":"HTTP 404 Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	albums, err := p.GetLibraryAlbums(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on 404 Not Found, got: %v", err)
	}
	if len(albums) != 0 {
		t.Errorf("expected 0 albums, got %d", len(albums))
	}
}

// --- GetPlaylistTracks ---

func TestGetPlaylistTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !containsStr(r.URL.Path, "pl-abc") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(t, w, map[string]any{
			"data": []any{
				songJSON("s1", "Playlist Track", "Some Artist", "Some Album", 240000, ""),
			},
			"next": "",
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetPlaylistTracks(context.Background(), "pl-abc")
	if err != nil {
		t.Fatalf("GetPlaylistTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	if tracks[0].Title != "Playlist Track" {
		t.Errorf("Title = %q", tracks[0].Title)
	}
}

func TestGetPlaylistTracks_Pagination_404OnNextPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			writeJSON(t, w, map[string]any{
				"data": []any{songJSON("1", "Track 1", "Art", "Alb", 100000, "")},
				"next": "/me/library/playlists/pl-abc/tracks?limit=100&offset=100",
			})
		case 2:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("404 Not Found"))
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetPlaylistTracks(context.Background(), "pl-abc")
	if err != nil {
		t.Fatalf("GetPlaylistTracks: unexpected error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1 (gracefully stopped on 404 next page)", len(tracks))
	}
}

// --- GetAlbumTracks ---

func TestGetAlbumTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data": []any{
				songJSON("a1", "Album Track 1", "Artist", "Album", 200000, ""),
				songJSON("a2", "Album Track 2", "Artist", "Album", 220000, ""),
			},
		})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetAlbumTracks(context.Background(), "alb-xyz")
	if err != nil {
		t.Fatalf("GetAlbumTracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
}

// --- Track preview URL ---

func TestSearch_TrackPreviewURL(t *testing.T) {
	// PreviewURL must be preserved for library songs.
	libSong := map[string]any{
		"id": "i.preview99",
		"attributes": map[string]any{
			"name":             "Test Song",
			"artistName":       "Test Artist",
			"albumName":        "Test Album",
			"durationInMillis": 180000,
			"artwork":          map[string]any{"url": "", "width": 300, "height": 300},
			"previews": []any{
				map[string]any{"url": "https://preview.example.com/song.m4a"},
			},
			"genreNames": []string{"pop"},
		},
	}
	libResp := map[string]any{
		"results": map[string]any{
			"library-songs":     map[string]any{"data": []any{libSong}},
			"library-albums":    map[string]any{"data": []any{}},
			"library-playlists": map[string]any{"data": []any{}},
		},
	}
	catEmpty := map[string]any{"results": map[string]any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHandler(t, w, r, libResp, catEmpty)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "test")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) == 0 {
		t.Fatal("no tracks returned")
	}
	if result.Tracks[0].PreviewURL != "https://preview.example.com/song.m4a" {
		t.Errorf("PreviewURL = %q", result.Tracks[0].PreviewURL)
	}
}

// --- helpers ---

func songJSON(id, name, artist, album string, durationMs int, artURL string) map[string]any {
	return map[string]any{
		"id": id,
		"attributes": map[string]any{
			"name":             name,
			"artistName":       artist,
			"albumName":        album,
			"durationInMillis": durationMs,
			"artwork":          map[string]any{"url": artURL, "width": 300, "height": 300},
			"previews":         []any{},
			"genreNames":       []string{},
			// playParams with kind "song" is the whole playability contract
			// the standard catalog endpoint can report.
			"playParams": map[string]any{"id": id, "kind": "song"},
		},
	}
}

// songJSONKind produces a catalog song fixture whose playParams carry a kind
// other than "song" (music video, radio episode), which Search drops.
func songJSONKind(id, name, artist, album string, durationMs int, kind string) map[string]any {
	s := songJSON(id, name, artist, album, durationMs, "")
	s["attributes"].(map[string]any)["playParams"] = map[string]any{"id": id, "kind": kind}
	return s
}

// songJSONNoPlay produces a catalog song fixture without playParams (radio-only / unplayable).
func songJSONNoPlay(id, name, artist, album string, durationMs int) map[string]any {
	return map[string]any{
		"id": id,
		"attributes": map[string]any{
			"name":             name,
			"artistName":       artist,
			"albumName":        album,
			"durationInMillis": durationMs,
			"artwork":          map[string]any{"url": "", "width": 300, "height": 300},
			"previews":         []any{},
			"genreNames":       []string{},
			// no playParams — filtered out by Search()
		},
	}
}

// songJSONNoStream produces a catalog song with playParams but an explicit
// empty extendedAssetUrls, the shape of a purchase-only or region-locked
// track. Search no longer reads that field (the standard catalog endpoint does
// not send it), so this is kept only to prove such a song now survives the
// filter rather than being dropped for a field AUX cannot see.
func songJSONNoStream(id, name, artist, album string, durationMs int) map[string]any {
	s := songJSON(id, name, artist, album, durationMs, "")
	s["attributes"].(map[string]any)["extendedAssetUrls"] = map[string]any{}
	return s
}

func albumJSON(id, name, artist string, trackCount int) map[string]any {
	return map[string]any{
		"id": id,
		"attributes": map[string]any{
			"name":       name,
			"artistName": artist,
			"trackCount": trackCount,
			"artwork":    map[string]any{"url": "", "width": 300, "height": 300},
		},
	}
}

func libraryAlbumJSON(id, name, artist string, trackCount int, artURL string) map[string]any {
	a := albumJSON(id, name, artist, trackCount)
	a["attributes"].(map[string]any)["artwork"] = map[string]any{"url": artURL, "width": 300, "height": 300}
	return a
}

func playlistJSON(id, name string, trackCount int) map[string]any {
	return map[string]any{
		"id": id,
		"attributes": map[string]any{
			"name":       name,
			"trackCount": trackCount,
			"artwork":    map[string]any{"url": "", "width": 300, "height": 300},
		},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writeJSON: %v", err)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// --- Error path tests ---

func TestGetLibraryTracks_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetLibraryTracks(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestGetLibraryTracks_EmptyLibrary404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":404,"message":"HTTP 404 Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on 404 Not Found, got: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(tracks))
	}
}

func TestGetLibraryPlaylists_EmptyLibrary404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":404,"message":"HTTP 404 Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	playlists, err := p.GetLibraryPlaylists(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on 404 Not Found, got: %v", err)
	}
	if len(playlists) != 1 || playlists[0].ID != "aux:favorites" {
		t.Errorf("expected exactly 1 playlist (Favorites), got: %+v", playlists)
	}
}

func TestGetLibraryPlaylists_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetLibraryPlaylists(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 403, got nil")
	}
}

func TestGetLibraryTracks_HTTPUnauthorizedHasGuidance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetLibraryTracks(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 401, got nil")
	}
	if !strings.Contains(err.Error(), "refresh apple_developer_token") {
		t.Fatalf("expected developer token guidance, got %q", err)
	}
}

func TestGetPlaylistTracks_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetPlaylistTracks(context.Background(), "pl-id")
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
}

func TestGetPlaylistTracks_FallbackToIncludeTracksOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me/library/playlists/pl-id/tracks":
			http.Error(w, "not found", http.StatusNotFound)
		case "/me/library/playlists/pl-id":
			if r.URL.Query().Get("include") != "tracks" {
				t.Fatalf("include query = %q", r.URL.Query().Get("include"))
			}
			writeJSON(t, w, map[string]any{"data": []any{map[string]any{
				"id":            "pl-id",
				"attributes":    map[string]any{"name": "Mix", "trackCount": 1, "artwork": map[string]any{"url": "", "width": 300, "height": 300}},
				"relationships": map[string]any{"tracks": map[string]any{"data": []any{songJSON("s1", "Playlist Track", "Some Artist", "Some Album", 240000, "")}}},
			}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetPlaylistTracks(context.Background(), "pl-id")
	if err != nil {
		t.Fatalf("GetPlaylistTracks fallback: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Title != "Playlist Track" {
		t.Fatalf("tracks = %+v", tracks)
	}
}

func TestGetAlbumTracks_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetAlbumTracks(context.Background(), "alb-id")
	if err == nil {
		t.Fatal("expected error for HTTP 503, got nil")
	}
}

func TestSearch_NoResultsKey(t *testing.T) {
	// Valid JSON but no recognized keys — both endpoints return empty objects.
	empty := map[string]any{"results": map[string]any{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, empty)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	result, err := p.Search(context.Background(), "test")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Tracks) != 0 || len(result.Albums) != 0 || len(result.Playlists) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

// --- CreatePlaylist ---

func TestCreatePlaylist_Success(t *testing.T) {
	resp := map[string]any{
		"data": []any{
			map[string]any{
				"id": "p.NewPlaylist123",
				"attributes": map[string]any{
					"name":       "My New Playlist",
					"trackCount": 2,
					"artwork":    map[string]any{"url": "", "width": 300, "height": 300},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "library/playlists") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	pl, err := p.CreatePlaylist(context.Background(), "My New Playlist", []string{"111", "222"})
	if err != nil {
		t.Fatalf("CreatePlaylist: %v", err)
	}
	if pl.ID != "p.NewPlaylist123" {
		t.Errorf("ID = %q, want %q", pl.ID, "p.NewPlaylist123")
	}
	if pl.Name != "My New Playlist" {
		t.Errorf("Name = %q, want %q", pl.Name, "My New Playlist")
	}
}

func TestCreatePlaylist_EmptyResponse(t *testing.T) {
	// API returns 201 with empty data array — should succeed with Name only.
	resp := map[string]any{"data": []any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	pl, err := p.CreatePlaylist(context.Background(), "Empty Body Playlist", []string{})
	if err != nil {
		t.Fatalf("CreatePlaylist(empty body): %v", err)
	}
	if pl.Name != "Empty Body Playlist" {
		t.Errorf("Name = %q, want %q", pl.Name, "Empty Body Playlist")
	}
}

func TestCreatePlaylist_LibraryTrackIDs(t *testing.T) {
	// Library IDs (i. prefix) should use "library-songs" type.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.CreatePlaylist(context.Background(), "Library Playlist", []string{"i.Abc123", "456"})
	if err != nil {
		t.Fatalf("CreatePlaylist(library IDs): %v", err)
	}
}

func TestCreatePlaylist_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.CreatePlaylist(context.Background(), "Failing Playlist", []string{})
	if err == nil {
		t.Error("CreatePlaylist: expected error for 500 response, got nil")
	}
}

// --- GetStationTracks ---

func TestGetStationTracks_Success(t *testing.T) {
	resp := map[string]any{
		"data": []any{
			songJSON("1001", "Related Song", "Related Artist", "Related Album", 200000, ""),
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "stations/next-tracks/ra.500") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if strings.TrimSpace(string(body)) != "{}" {
			t.Errorf("unexpected request body: %s", body)
		}
		writeJSON(t, w, resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetStationTracks(context.Background(), "500")
	if err != nil {
		t.Fatalf("GetStationTracks: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1", len(tracks))
	}
	if tracks[0].ID != "1001" || tracks[0].Title != "Related Song" {
		t.Errorf("unexpected track: %+v", tracks[0])
	}
}

func TestGetStationTracks_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	_, err := p.GetStationTracks(context.Background(), "500")
	if err == nil {
		t.Error("GetStationTracks: expected error for 500 response, got nil")
	}
}

func TestGetStationTracks_Batches(t *testing.T) {
	// Apple returns ~2 tracks per next-tracks call; GetStationTracks should page
	// several times and accumulate a larger, deduped batch.
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		n := calls.Add(1)
		writeJSON(t, w, map[string]any{"data": []any{
			songJSON("s"+strconv.FormatInt(n*2-1, 10), "T", "A", "Al", 200000, ""),
			songJSON("s"+strconv.FormatInt(n*2, 10), "T", "A", "Al", 200000, ""),
		}})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetStationTracks(context.Background(), "500")
	if err != nil {
		t.Fatalf("GetStationTracks: %v", err)
	}
	if len(tracks) != 10 {
		t.Fatalf("expected 10 accumulated tracks, got %d (server calls=%d)", len(tracks), calls.Load())
	}
}

func TestGetStationTracks_ResolvesLibraryID(t *testing.T) {
	// A library seed ("i.XXXX") 500s at Apple, so it must be resolved to its
	// catalog id (playParams.catalogId) before seeding the station.
	var stationPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/me/library/songs/i.LIB1"):
			writeJSON(t, w, map[string]any{"data": []any{
				songJSONWithCatalog("i.LIB1", "777", "Lib Song", "Artist", "Album", 200000, ""),
			}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "stations/next-tracks/"):
			stationPath = r.URL.Path
			writeJSON(t, w, map[string]any{"data": []any{
				songJSON("1001", "Related", "A", "Al", 200000, ""),
			}})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	tracks, err := p.GetStationTracks(context.Background(), "i.LIB1")
	if err != nil {
		t.Fatalf("GetStationTracks: %v", err)
	}
	if !strings.Contains(stationPath, "ra.777") {
		t.Errorf("station should be seeded with resolved catalog id ra.777, got path %q", stationPath)
	}
	if len(tracks) == 0 {
		t.Error("expected tracks after resolving library id")
	}
}

func TestGetStationTracks_LibraryNoCatalog(t *testing.T) {
	// A library song with no catalog equivalent has no per-song station: error
	// out cleanly instead of seeding with a doomed library id.
	var stationCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "stations/next-tracks/") {
			stationCalled.Store(true)
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/me/library/songs/") {
			writeJSON(t, w, map[string]any{"data": []any{
				songJSON("i.LIB2", "Uploaded", "Artist", "Album", 200000, ""), // no catalogId
			}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if _, err := p.GetStationTracks(context.Background(), "i.LIB2"); err == nil {
		t.Error("expected error for library song with no catalog equivalent")
	}
	if stationCalled.Load() {
		t.Error("should not hit the station endpoint when the catalog id cannot be resolved")
	}
}

// --- LoveSong ---

func TestLoveSong_LoveSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Add to library request.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPut {
			// Rating PUT request.
			if !strings.Contains(r.URL.Path, "ratings/songs") {
				t.Errorf("unexpected PUT path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(t, w, map[string]any{})
			return
		}
		t.Errorf("unexpected method: %s", r.Method)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if err := p.LoveSong(context.Background(), "123456", true); err != nil {
		t.Fatalf("LoveSong(loved=true): %v", err)
	}
}

func TestLoveSong_UnloveSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected method: %s", r.Method)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if err := p.LoveSong(context.Background(), "123456", false); err != nil {
		t.Fatalf("LoveSong(loved=false): %v", err)
	}
}

func TestLoveSong_ContinuesAfterAddFailure(t *testing.T) {
	// Even if add-to-library fails (non-fatal), the rating PUT should proceed.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict) // already in library — non-fatal
			return
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK)
			writeJSON(t, w, map[string]any{})
		}
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	if err := p.LoveSong(context.Background(), "abc", true); err != nil {
		t.Fatalf("LoveSong: %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected >= 2 HTTP calls (add+rate), got %d", callCount)
	}
}

// --- GetSongRating ---

func TestGetSongRating_Loved(t *testing.T) {
	resp := map[string]any{
		"data": []any{
			map[string]any{
				"attributes": map[string]any{"value": 1},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "ratings/songs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		writeJSON(t, w, resp)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	loved, err := p.GetSongRating(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if !loved {
		t.Error("GetSongRating = false, want true (value=1)")
	}
}

func TestGetSongRating_NotRated_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	loved, err := p.GetSongRating(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if loved {
		t.Error("GetSongRating = true for 404, want false")
	}
}

func TestGetSongRating_NoContent_204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	loved, err := p.GetSongRating(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if loved {
		t.Error("GetSongRating = true for 204 NoContent, want false")
	}
}

func TestGetSongRating_ServerError_ReturnsFalse(t *testing.T) {
	// 500 errors are treated as "not rated" — non-fatal.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	loved, err := p.GetSongRating(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if loved {
		t.Error("GetSongRating = true for 500, want false")
	}
}

func TestGetSongRating_InvalidJSON_ReturnsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	loved, err := p.GetSongRating(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetSongRating: %v", err)
	}
	if loved {
		t.Error("GetSongRating = true for invalid JSON, want false")
	}
}

func TestGetLibraryPlaylists_AddsSyntheticFavorites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": []any{playlistJSON("pl2", "Workout", 25)}, "next": ""})
	}))
	defer srv.Close()

	p := newTestProvider(t, srv)
	lists, err := p.GetLibraryPlaylists(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryPlaylists: %v", err)
	}
	if len(lists) != 2 || lists[0].Name != "Favorites" || !lists[0].ReadOnly {
		t.Fatalf("synthetic favorites not prepended/read-only: %+v", lists)
	}
}

func TestGetPlaylistTracks_FavoritesUsesInFavorites(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/songs" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{"data": []any{
			favoriteSongJSON("i.one", "cat1", "Kept", true),
			favoriteSongJSON("i.two", "cat2", "Dropped", false),
		}, "next": ""})
	})

	tracks, err := p.GetPlaylistTracks(context.Background(), "aux:favorites")
	if err != nil {
		t.Fatalf("GetPlaylistTracks(favorites): %v", err)
	}
	if len(tracks) != 1 || tracks[0].ID != "i.one" {
		t.Fatalf("favorites = %+v, want exactly i.one", tracks)
	}
	if tracks[0].CatalogID != "cat1" {
		t.Errorf("catalog id = %q, want cat1", tracks[0].CatalogID)
	}
	got := calls()
	if len(got) == 0 {
		t.Fatal("no requests recorded")
	}
	for _, c := range got {
		if strings.HasPrefix(c.path, "/me/ratings") {
			t.Errorf("favorites used the legacy ratings endpoint: %s", c.path)
		}
		if extend := c.query.Get("extend"); extend != "inFavorites" {
			t.Errorf("%s: extend = %q, want inFavorites", c.path, extend)
		}
	}
}

func TestGetPlaylistTracks_FavoritesNextLinkKeepsExtend(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "100" {
			writeJSON(t, w, map[string]any{"data": []any{
				favoriteSongJSON("i.two", "cat2", "Second page", true),
			}})
			return
		}
		writeJSON(t, w, map[string]any{
			"data": []any{favoriteSongJSON("i.one", "cat1", "First page", true)},
			"next": "/v1/me/library/songs?offset=100",
		})
	})

	tracks, err := p.GetPlaylistTracks(context.Background(), "aux:favorites")
	if err != nil {
		t.Fatalf("GetPlaylistTracks(favorites): %v", err)
	}
	ids := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		ids = append(ids, tr.ID)
	}
	if strings.Join(ids, ",") != "i.one,i.two" {
		t.Fatalf("favorites = %v, want [i.one i.two]", ids)
	}
	sawPage2 := false
	for _, c := range calls() {
		if extend := c.query.Get("extend"); extend != "inFavorites" {
			t.Errorf("%s?%s: extend = %q, want inFavorites", c.path, c.query.Encode(), extend)
		}
		if c.query.Get("offset") == "100" {
			sawPage2 = true
		}
	}
	if !sawPage2 {
		t.Fatalf("page 2 was never requested: %+v", calls())
	}
}

func TestGetPlaylistTracks_FavoritesOffsetsKeepExtend(t *testing.T) {
	const total = 150
	page := func(start, count int) []any {
		songs := make([]any, count)
		for i := range songs {
			n := start + i
			songs[i] = favoriteSongJSON("i.s"+strconv.Itoa(n), "cat"+strconv.Itoa(n), "Song", n == 0 || n == 100)
		}
		return songs
	}
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, _ = strconv.Atoi(v)
		}
		if offset < 0 || offset >= total {
			t.Errorf("unexpected offset %d", offset)
		}
		writeJSON(t, w, map[string]any{
			"data": page(offset, min(100, total-offset)),
			"meta": map[string]any{"total": total},
		})
	})

	tracks, err := p.GetPlaylistTracks(context.Background(), "aux:favorites")
	if err != nil {
		t.Fatalf("GetPlaylistTracks(favorites): %v", err)
	}
	ids := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		ids = append(ids, tr.ID)
	}
	if strings.Join(ids, ",") != "i.s0,i.s100" {
		t.Fatalf("favorites = %v, want [i.s0 i.s100]", ids)
	}
	got := calls()
	if len(got) != 2 {
		t.Fatalf("requests = %d, want 2: %+v", len(got), got)
	}
	for _, c := range got {
		if extend := c.query.Get("extend"); extend != "inFavorites" {
			t.Errorf("%s?%s: extend = %q, want inFavorites", c.path, c.query.Encode(), extend)
		}
	}
}

func songJSONWithCatalog(id, catalogID, name, artist, album string, durationMs int, artURL string) map[string]any {
	s := songJSON(id, name, artist, album, durationMs, artURL)
	s["attributes"].(map[string]any)["playParams"] = map[string]any{"id": id, "kind": "song", "catalogId": catalogID}
	return s
}

func favoriteSongJSON(id, catalogID, name string, inFavorites bool) map[string]any {
	s := songJSONWithCatalog(id, catalogID, name, "Artist", "Album", 1000, "")
	s["attributes"].(map[string]any)["inFavorites"] = inFavorites
	return s
}

type recordedAppleCall struct {
	method string
	path   string
	query  url.Values
}

func recordingProvider(t *testing.T, handle func(http.ResponseWriter, *http.Request)) (*apple.AppleProvider, func() []recordedAppleCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []recordedAppleCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, recordedAppleCall{method: r.Method, path: r.URL.Path, query: r.URL.Query()})
		mu.Unlock()
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return newTestProvider(t, srv), func() []recordedAppleCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedAppleCall(nil), calls...)
	}
}

// --- LibrarySongCount ---

func TestGetLibraryTracks_ReportsFavorites(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/songs" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{"data": []any{
			favoriteSongJSON("i.one", "cat1", "Loved", true),
			favoriteSongJSON("i.two", "cat2", "Plain", false),
		}, "next": ""})
	})

	tracks, err := p.GetLibraryTracks(context.Background())
	if err != nil {
		t.Fatalf("GetLibraryTracks: %v", err)
	}
	if len(tracks) != 2 || !tracks[0].Favorite || tracks[1].Favorite {
		t.Fatalf("favorites flags = %+v, want [true false]", tracks)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1: %+v", len(got), got)
	}
	if extend := got[0].query.Get("extend"); extend != "inFavorites" {
		t.Errorf("extend = %q, want inFavorites", extend)
	}
}

func TestLibrarySongCount(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/songs" {
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{"data": []any{}, "meta": map[string]any{"total": 137}})
	})

	n, err := p.LibrarySongCount(context.Background())
	if err != nil {
		t.Fatalf("LibrarySongCount: %v", err)
	}
	if n != 137 {
		t.Errorf("count = %d, want 137", n)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1: %+v", len(got), got)
	}
	if limit := got[0].query.Get("limit"); limit != "1" {
		t.Errorf("limit = %q, want 1", limit)
	}
}

func TestLibrarySongCount_ZeroTotal(t *testing.T) {
	p, _ := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"data": []any{}, "meta": map[string]any{"total": 0}})
	})
	n, err := p.LibrarySongCount(context.Background())
	if err != nil {
		t.Fatalf("LibrarySongCount: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0 with no error", n)
	}
}

func TestAddToLibrary_PostsCatalogID(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	if err := p.AddToLibrary(context.Background(), "123"); err != nil {
		t.Fatalf("AddToLibrary: %v", err)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1: %+v", len(got), got)
	}
	if got[0].method != http.MethodPost || got[0].path != "/me/library" {
		t.Errorf("request = %s %s, want POST /me/library", got[0].method, got[0].path)
	}
	if id := got[0].query.Get("ids[songs]"); id != "123" {
		t.Errorf("ids[songs] = %q, want 123", id)
	}
}

func TestAddToFavorites_PostsCatalogID(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	if err := p.AddToFavorites(context.Background(), "123"); err != nil {
		t.Fatalf("AddToFavorites: %v", err)
	}
	got := calls()
	if len(got) != 1 {
		t.Fatalf("requests = %d, want 1: %+v", len(got), got)
	}
	if got[0].method != http.MethodPost || got[0].path != "/me/favorites" {
		t.Errorf("request = %s %s, want POST /me/favorites", got[0].method, got[0].path)
	}
	if id := got[0].query.Get("ids[songs]"); id != "123" {
		t.Errorf("ids[songs] = %q, want 123", id)
	}
}

func TestAddToFavorites_ResolvesLibraryID(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(t, w, map[string]any{"data": []any{map[string]any{
				"id":         "i.abc",
				"attributes": map[string]any{"playParams": map[string]any{"catalogId": "555"}},
			}}})
		case http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	if err := p.AddToFavorites(context.Background(), "i.abc"); err != nil {
		t.Fatalf("AddToFavorites: %v", err)
	}
	got := calls()
	if len(got) != 2 {
		t.Fatalf("requests = %d, want 2: %+v", len(got), got)
	}
	if got[0].method != http.MethodGet || got[0].path != "/me/library/songs/i.abc" {
		t.Errorf("resolve request = %s %s, want GET /me/library/songs/i.abc", got[0].method, got[0].path)
	}
	if got[1].method != http.MethodPost || got[1].path != "/me/favorites" {
		t.Errorf("post request = %s %s, want POST /me/favorites", got[1].method, got[1].path)
	}
	if id := got[1].query.Get("ids[songs]"); id != "555" {
		t.Errorf("ids[songs] = %q, want 555", id)
	}
	if strings.Contains(got[1].query.Encode(), "i.abc") {
		t.Errorf("library id leaked into the POST: %s", got[1].query.Encode())
	}
}

func TestAddToLibrary_ResolveFailurePostsNothing(t *testing.T) {
	p, calls := recordingProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(t, w, map[string]any{"data": []any{map[string]any{"id": "i.abc", "attributes": map[string]any{}}}})
	})
	if err := p.AddToLibrary(context.Background(), "i.abc"); err == nil {
		t.Fatal("AddToLibrary: expected an error for a library song with no catalog id")
	}
	for _, c := range calls() {
		if c.method == http.MethodPost {
			t.Errorf("POSTed %s after a failed resolve", c.path)
		}
	}
}

func TestAddToFavorites_Unauthorized(t *testing.T) {
	p, _ := recordingProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if err := p.AddToFavorites(context.Background(), "123"); err == nil {
		t.Fatal("AddToFavorites: expected an error on 401")
	}
}

func TestAddToLibrary_ServerError(t *testing.T) {
	p, _ := recordingProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := p.AddToLibrary(context.Background(), "123"); err == nil {
		t.Fatal("AddToLibrary: expected an error on 500")
	}
}
