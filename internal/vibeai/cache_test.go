package vibeai

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func seedCaches(t *testing.T, dir string, library, favorites []Song) {
	t.Helper()
	if err := writeCache(filepath.Join(dir, libraryCacheName), library); err != nil {
		t.Fatalf("write library cache: %v", err)
	}
	if err := writeCache(filepath.Join(dir, favoritesCacheName), favorites); err != nil {
		t.Fatalf("write favorites cache: %v", err)
	}
}

func readCacheBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file inside the test's temp dir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestRememberSavedTouchesOnlyTargetFile(t *testing.T) {
	library := []Song{{ID: "1", Artist: "A", Title: "One"}}
	favorites := []Song{{ID: "2", Artist: "B", Title: "Two"}}
	saved := Song{ID: "3", Artist: "C", Title: "Three", Album: "Three"}

	t.Run("library", func(t *testing.T) {
		dir := t.TempDir()
		seedCaches(t, dir, library, favorites)
		favoritesPath := filepath.Join(dir, favoritesCacheName)
		before := readCacheBytes(t, favoritesPath)
		if err := RememberSaved(dir, saved, false); err != nil {
			t.Fatalf("RememberSaved: %v", err)
		}
		got, err := readCache(filepath.Join(dir, libraryCacheName))
		if err != nil {
			t.Fatalf("read library cache: %v", err)
		}
		want := append(append([]Song{}, library...), saved)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("library.json = %+v, want %+v", got, want)
		}
		if after := readCacheBytes(t, favoritesPath); !bytes.Equal(before, after) {
			t.Errorf("favorites.json changed: %q -> %q", before, after)
		}
	})

	t.Run("favorites", func(t *testing.T) {
		dir := t.TempDir()
		seedCaches(t, dir, library, favorites)
		libraryPath := filepath.Join(dir, libraryCacheName)
		before := readCacheBytes(t, libraryPath)
		if err := RememberSaved(dir, saved, true); err != nil {
			t.Fatalf("RememberSaved: %v", err)
		}
		got, err := readCache(filepath.Join(dir, favoritesCacheName))
		if err != nil {
			t.Fatalf("read favorites cache: %v", err)
		}
		want := append(append([]Song{}, favorites...), saved)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("favorites.json = %+v, want %+v", got, want)
		}
		if after := readCacheBytes(t, libraryPath); !bytes.Equal(before, after) {
			t.Errorf("library.json changed: %q -> %q", before, after)
		}
	})
}

func TestRememberSavedDuplicateSong(t *testing.T) {
	dir := t.TempDir()
	seedCaches(t, dir, []Song{}, []Song{})
	song := Song{ID: "1", Artist: "A", Title: "One"}
	for range 2 {
		if err := RememberSaved(dir, song, false); err != nil {
			t.Fatalf("RememberSaved: %v", err)
		}
	}
	got, err := readCache(filepath.Join(dir, libraryCacheName))
	if err != nil {
		t.Fatalf("read library cache: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("library.json = %+v, want one entry", got)
	}
}

func TestRememberSavedKeepsMtime(t *testing.T) {
	dir := t.TempDir()
	seedCaches(t, dir, []Song{}, []Song{})
	old := time.Now().Add(-time.Hour)
	for _, name := range []string{libraryCacheName, favoritesCacheName} {
		path := filepath.Join(dir, name)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, libraryCacheName)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RememberSaved(dir, Song{ID: "1", Artist: "A", Title: "One"}, false); err != nil {
		t.Fatalf("RememberSaved: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("mtime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
}

func TestRememberSavedNoOpOnStaleOrMissingCache(t *testing.T) {
	song := Song{ID: "1", Artist: "A", Title: "One"}

	t.Run("missing library cache", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeCache(filepath.Join(dir, favoritesCacheName), []Song{}); err != nil {
			t.Fatal(err)
		}
		if err := RememberSaved(dir, song, false); err != nil {
			t.Fatalf("RememberSaved: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, libraryCacheName)); !os.IsNotExist(err) {
			t.Errorf("missing library cache was created: %v", err)
		}
	})

	t.Run("expired caches", func(t *testing.T) {
		dir := t.TempDir()
		seedCaches(t, dir, []Song{}, []Song{})
		old := time.Now().Add(-25 * time.Hour)
		paths := []string{filepath.Join(dir, libraryCacheName), filepath.Join(dir, favoritesCacheName)}
		for _, path := range paths {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
		before := make([][]byte, len(paths))
		for i, path := range paths {
			before[i] = readCacheBytes(t, path)
		}
		if err := RememberSaved(dir, song, false); err != nil {
			t.Fatalf("RememberSaved(library): %v", err)
		}
		if err := RememberSaved(dir, song, true); err != nil {
			t.Fatalf("RememberSaved(favorite): %v", err)
		}
		for i, path := range paths {
			if after := readCacheBytes(t, path); !bytes.Equal(before[i], after) {
				t.Errorf("%s changed: %q -> %q", path, before[i], after)
			}
		}
	})
}

func TestRememberSavedConcurrent(t *testing.T) {
	dir := t.TempDir()
	seedCaches(t, dir, []Song{}, []Song{})
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			song := Song{ID: fmt.Sprintf("s%d", i), Artist: fmt.Sprintf("a%d", i), Title: fmt.Sprintf("t%d", i)}
			if err := RememberSaved(dir, song, false); err != nil {
				t.Errorf("RememberSaved: %v", err)
			}
		})
	}
	wg.Wait()
	got, err := readCache(filepath.Join(dir, libraryCacheName))
	if err != nil {
		t.Fatalf("read library cache: %v", err)
	}
	if len(got) != n {
		t.Fatalf("library.json has %d songs, want %d", len(got), n)
	}
	seen := make(map[string]bool, n)
	for _, song := range got {
		seen[song.ID] = true
	}
	for i := range n {
		if id := fmt.Sprintf("s%d", i); !seen[id] {
			t.Errorf("missing song %s", id)
		}
	}
}

func countSongID(songs []Song, id string) int {
	n := 0
	for _, s := range songs {
		if s.ID == id {
			n++
		}
	}
	return n
}

// F7/R2: a save confirmed before the fetch that reloads the cache is not dropped by
// the reload, not on the returned list and not in the file it writes.
func TestLoadCachesKeepsRememberedSaveOnFetch(t *testing.T) {
	const page = `{"data":[{"id":"i.a","attributes":{"artistName":"A","name":"Alpha"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/songs" {
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	dir := t.TempDir()
	saved := Song{ID: "10", Artist: "X", Title: "Ex"}
	if err := RememberSaved(dir, saved, false); err != nil {
		t.Fatalf("RememberSaved: %v", err)
	}
	api := &appleAPI{http: srv.Client(), dev: "dev", user: "user", base: srv.URL}
	library, favs, err := loadCaches(context.Background(), api, dir, time.Hour, nil)
	if err != nil {
		t.Fatalf("loadCaches: %v", err)
	}
	if len(library) != 2 || countSongID(library, saved.ID) != 1 {
		t.Errorf("library = %+v, want the fetched song and one %s", library, saved.ID)
	}
	if len(favs) != 0 {
		t.Errorf("favorites = %+v, want none", favs)
	}
	onDisk, err := readCache(filepath.Join(dir, libraryCacheName))
	if err != nil {
		t.Fatalf("read library cache: %v", err)
	}
	if len(onDisk) != 2 || countSongID(onDisk, saved.ID) != 1 {
		t.Errorf("library.json = %+v, want the fetched song and one %s", onDisk, saved.ID)
	}
}

// F7/R2: a fresh cache already holding the saved song gains no second copy, and the
// process-wide registry stays inside the dir it was written for.
func TestLoadCachesKeepsRememberedFavoriteOnFreshCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fresh caches fetched %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	api := &appleAPI{http: srv.Client(), dev: "dev", user: "user", base: srv.URL}

	saved := Song{ID: "10", Artist: "X", Title: "Ex"}
	dir := t.TempDir()
	seedCaches(t, dir, []Song{saved}, []Song{saved})
	if err := RememberSaved(dir, saved, true); err != nil {
		t.Fatalf("RememberSaved: %v", err)
	}
	library, favs, err := loadCaches(context.Background(), api, dir, time.Hour, nil)
	if err != nil {
		t.Fatalf("loadCaches: %v", err)
	}
	if len(library) != 1 || countSongID(favs, saved.ID) != 1 {
		t.Errorf("library = %+v, favorites = %+v, want one %s each", library, favs, saved.ID)
	}

	other := t.TempDir()
	seedCaches(t, other, []Song{}, []Song{})
	otherLibrary, otherFavs, err := loadCaches(context.Background(), api, other, time.Hour, nil)
	if err != nil {
		t.Fatalf("loadCaches(other): %v", err)
	}
	if countSongID(otherLibrary, saved.ID) != 0 || countSongID(otherFavs, saved.ID) != 0 {
		t.Errorf("another dir got the save: library = %+v, favorites = %+v", otherLibrary, otherFavs)
	}
}
