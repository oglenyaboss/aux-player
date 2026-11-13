package vibeai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	libraryCacheName   = "library.json"
	favoritesCacheName = "favorites.json"
	defaultCacheMaxAge = 24 * time.Hour
)

var cacheMu sync.Mutex

type rememberedSave struct {
	dir      string
	song     Song
	favorite bool
}

var remembered []rememberedSave

func readCache(path string) ([]Song, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is a file inside the user cache dir
	if err != nil {
		return nil, err
	}
	var songs []Song
	if err := json.Unmarshal(data, &songs); err != nil {
		return nil, fmt.Errorf("cache %s: %v", path, err)
	}
	return songs, nil
}

func writeCache(path string, songs []Song) error {
	data, err := json.MarshalIndent(songs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadCaches(ctx context.Context, api *appleAPI, dir string, maxAge time.Duration, progress func(string)) (library, favs []Song, err error) {
	if cacheFresh(dir, maxAge, time.Now()) {
		library, err = readCache(filepath.Join(dir, libraryCacheName))
		if err != nil {
			return nil, nil, err
		}
		favs, err = readCache(filepath.Join(dir, favoritesCacheName))
		if err != nil {
			return nil, nil, err
		}
		cacheMu.Lock()
		library, favs = mergeRemembered(dir, library, favs)
		cacheMu.Unlock()
		return library, favs, nil
	}
	if progress != nil {
		progress("loading the Apple Music library…")
	}
	library, favs, err = fetchLibrary(ctx, api)
	if err != nil {
		return nil, nil, err
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	library, favs = mergeRemembered(dir, library, favs)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, err
	}
	if err := writeCache(filepath.Join(dir, libraryCacheName), library); err != nil {
		return nil, nil, err
	}
	if err := writeCache(filepath.Join(dir, favoritesCacheName), favs); err != nil {
		return nil, nil, err
	}
	return library, favs, nil
}

func mergeRemembered(dir string, library, favs []Song) ([]Song, []Song) {
	for _, r := range remembered {
		if r.dir != dir {
			continue
		}
		if r.favorite {
			favs = withSong(favs, r.song)
			continue
		}
		library = withSong(library, r.song)
	}
	return library, favs
}

func withSong(songs []Song, song Song) []Song {
	for _, s := range songs {
		if s.ID == song.ID {
			return songs
		}
	}
	return append(songs, song)
}

func remember(dir string, song Song, favorite bool) {
	for _, r := range remembered {
		if r.dir == dir && r.song.ID == song.ID && r.favorite == favorite {
			return
		}
	}
	remembered = append(remembered, rememberedSave{dir: dir, song: song, favorite: favorite})
}

func RememberSaved(dir string, song Song, favorite bool) error {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	remember(dir, song, favorite)
	if !cacheFresh(dir, defaultCacheMaxAge, time.Now()) {
		return nil
	}
	name := libraryCacheName
	if favorite {
		name = favoritesCacheName
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	songs, err := readCache(path)
	if err != nil {
		return err
	}
	for _, s := range songs {
		if s.ID == song.ID {
			return nil
		}
	}
	if err := writeCache(path, append(songs, song)); err != nil {
		return err
	}
	return os.Chtimes(path, info.ModTime(), info.ModTime())
}

func cacheFresh(dir string, maxAge time.Duration, now time.Time) bool {
	for _, name := range []string{libraryCacheName, favoritesCacheName} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || now.Sub(info.ModTime()) >= maxAge {
			return false
		}
	}
	return true
}
