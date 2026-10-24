package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/simone-vibes/vibez/internal/stage"
)

// hashStr is the FNV-1a of the track key the cover cache names carry; it must
// match the old cache in $TMPDIR/vibez-scene-covers byte for byte.
func hashStr(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return h
}

// loadCovers pulls artwork through the public iTunes Search API and caches it on disk.
// Missing covers fall back to a generated gradient inside the stage.
func loadCovers(tracks []stage.Track) map[string]image.Image {
	dir := filepath.Join(os.TempDir(), "vibez-scene-covers")
	_ = os.MkdirAll(dir, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out := map[string]image.Image{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range tracks {
		wg.Add(1)
		go func(t stage.Track) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("%x.jpg", hashStr(t.Key())))
			if _, err := os.Stat(path); err != nil {
				if err := fetchCover(ctx, t, path); err != nil {
					return
				}
			}
			f, err := os.Open(path)
			if err != nil {
				return
			}
			defer f.Close()
			img, _, err := image.Decode(f)
			if err != nil {
				return
			}
			mu.Lock()
			out[t.Key()] = img
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	return out
}

func fetchCover(ctx context.Context, t stage.Track, path string) error {
	q := url.Values{"term": {t.Artist + " " + t.Title}, "entity": {"song"}, "limit": {"1"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://itunes.apple.com/search?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var res struct {
		Results []struct {
			ArtworkURL100 string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || len(res.Results) == 0 {
		return fmt.Errorf("no result")
	}
	art := strings.Replace(res.Results[0].ArtworkURL100, "100x100bb", "300x300bb", 1)
	req, _ = http.NewRequestWithContext(ctx, "GET", art, nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	data, err := io.ReadAll(resp2.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
