package vibeai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	libraryCacheName   = "library.json"
	favoritesCacheName = "favorites.json"
)

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
		return library, favs, nil
	}
	if progress != nil {
		progress("loading the Apple Music library…")
	}
	library, libIDs, err := fetchLibrary(ctx, api)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, err
	}
	if err := writeCache(filepath.Join(dir, libraryCacheName), library); err != nil {
		return nil, nil, err
	}
	if progress != nil {
		progress("looking for favorite tracks…")
	}
	favs, err = fetchFavorites(ctx, api, library, libIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := writeCache(filepath.Join(dir, favoritesCacheName), favs); err != nil {
		return nil, nil, err
	}
	return library, favs, nil
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
