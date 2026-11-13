package vibeai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseSimilar(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		want  []similar
		isErr bool
	}{
		{
			name: "array of artists",
			body: `{"similarartists":{"artist":[{"name":"Pond","match":"1"},{"name":"GUM","match":"0.65"}]}}`,
			want: []similar{{Name: "Pond", Match: 1}, {Name: "GUM", Match: 0.65}},
		},
		{
			name: "single artist object",
			body: `{"similarartists":{"artist":{"name":"Solo","match":"0.5"}}}`,
			want: []similar{{Name: "Solo", Match: 0.5}},
		},
		{
			name:  "lastfm error",
			body:  `{"error":6,"message":"The artist you supplied could not be found"}`,
			isErr: true,
		},
	}
	for _, tt := range tests {
		got, err := parseSimilar([]byte(tt.body))
		if tt.isErr {
			if err == nil || !strings.Contains(err.Error(), "6") || !strings.Contains(err.Error(), "could not be found") {
				t.Errorf("%s: err = %v, want text with 6 and %q", tt.name, err, "could not be found")
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected err %v", tt.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseArtistSearch(t *testing.T) {
	id, name, err := parseArtistSearch([]byte(`{"results":{"artists":{"data":[{"id":"123","attributes":{"name":"Pond"}}]}}}`))
	if err != nil || id != "123" || name != "Pond" {
		t.Errorf("got (%q, %q, %v), want (\"123\", \"Pond\", nil)", id, name, err)
	}
	id, name, err = parseArtistSearch([]byte(`{"results":{}}`))
	if err != nil || id != "" || name != "" {
		t.Errorf("got (%q, %q, %v), want empty nil", id, name, err)
	}
}

func TestParseLibraryPage(t *testing.T) {
	songs, favorites, next, err := parseLibraryPage([]byte(`{"data":[{"id":"i.abc","attributes":{"artistName":"Artist A","name":"Title A","albumName":"Album A","inFavorites":true,"playParams":{"catalogId":"123"}}},{"id":"i.def","attributes":{"artistName":"Artist B","name":"Title B"}}],"next":"/v1/me/library/songs?offset=100"}`))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	want := []Song{
		{ID: "123", Artist: "Artist A", Title: "Title A", Album: "Album A"},
		{ID: "i.def", Artist: "Artist B", Title: "Title B"},
	}
	if !reflect.DeepEqual(songs, want) {
		t.Errorf("songs = %+v, want %+v", songs, want)
	}
	if !reflect.DeepEqual(favorites, want[:1]) {
		t.Errorf("favorites = %+v, want %+v", favorites, want[:1])
	}
	if next != "/v1/me/library/songs?offset=100" {
		t.Errorf("next = %q, want %q", next, "/v1/me/library/songs?offset=100")
	}

	_, favorites, next, err = parseLibraryPage([]byte(`{"data":[]}`))
	if err != nil || next != "" || len(favorites) != 0 {
		t.Errorf("empty page: (%v, %v, %q), want nil err, no favorites and empty next", err, favorites, next)
	}
}

func TestParseLibraryPageInFavorites(t *testing.T) {
	songs, favorites, _, err := parseLibraryPage([]byte(`{"data":[{"id":"i.abc","attributes":{"artistName":"Artist A","name":"Title A","inFavorites":true}},{"id":"i.def","attributes":{"artistName":"Artist B","name":"Title B"}}]}`))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("songs = %d, want 2", len(songs))
	}
	want := []Song{{ID: "i.abc", Artist: "Artist A", Title: "Title A"}}
	if !reflect.DeepEqual(favorites, want) {
		t.Errorf("favorites = %+v, want %+v", favorites, want)
	}
}

func TestLoadCachesFavoritesAcrossPages(t *testing.T) {
	const page1 = `{"data":[{"id":"i.a","attributes":{"name":"A","artistName":"AA","albumName":"AlA","inFavorites":true}},{"id":"i.b","attributes":{"name":"B","artistName":"BB","albumName":"AlB"}}],"next":"/v1/me/library/songs?offset=100"}`
	const page2 = `{"data":[{"id":"i.c","attributes":{"name":"C","artistName":"CC","albumName":"AlC","inFavorites":true}}]}`

	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case r.URL.Path == "/me/library/songs" && r.URL.Query().Get("offset") == "":
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("extend") != "inFavorites" {
				t.Errorf("first page query = %q, want limit=100&extend=inFavorites", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(page1))
		case r.URL.Path == "/v1/me/library/songs" && r.URL.Query().Get("offset") == "100":
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("extend") != "inFavorites" {
				t.Errorf("offset page query = %q, want limit=100&extend=inFavorites", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(page2))
		default:
			t.Errorf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	api := &appleAPI{http: srv.Client(), dev: "dev", user: "user", base: srv.URL}
	library, favorites, err := loadCaches(context.Background(), api, dir, time.Hour, nil)
	if err != nil {
		t.Fatalf("loadCaches: %v", err)
	}
	wantLibrary := []Song{
		{ID: "i.a", Artist: "AA", Title: "A", Album: "AlA"},
		{ID: "i.b", Artist: "BB", Title: "B", Album: "AlB"},
		{ID: "i.c", Artist: "CC", Title: "C", Album: "AlC"},
	}
	wantFavorites := []Song{wantLibrary[0], wantLibrary[2]}
	if !reflect.DeepEqual(library, wantLibrary) {
		t.Errorf("library = %+v, want %+v", library, wantLibrary)
	}
	if !reflect.DeepEqual(favorites, wantFavorites) {
		t.Errorf("favorites = %+v, want %+v", favorites, wantFavorites)
	}
	savedLibrary, err := readCache(filepath.Join(dir, libraryCacheName))
	if err != nil {
		t.Fatalf("read library cache: %v", err)
	}
	savedFavorites, err := readCache(filepath.Join(dir, favoritesCacheName))
	if err != nil {
		t.Fatalf("read favorites cache: %v", err)
	}
	if !reflect.DeepEqual(savedLibrary, wantLibrary) {
		t.Errorf("library.json = %+v, want %+v", savedLibrary, wantLibrary)
	}
	if !reflect.DeepEqual(savedFavorites, wantFavorites) {
		t.Errorf("favorites.json = %+v, want %+v", savedFavorites, wantFavorites)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range paths {
		if strings.Contains(path, "ratings") {
			t.Errorf("ratings request: %s", path)
		}
	}
}
