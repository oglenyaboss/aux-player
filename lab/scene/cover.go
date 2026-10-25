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

	"github.com/oglenyaboss/aux-player/internal/stage"
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

// itunesResults decodes one iTunes Search / Lookup response: artist, collection,
// release date and song length when present.
type itunesResults struct {
	Results []struct {
		WrapperType     string `json:"wrapperType"`
		ArtistName      string `json:"artistName"`
		ArtistID        int64  `json:"artistId"`
		TrackName       string `json:"trackName"`
		CollectionName  string `json:"collectionName"`
		ReleaseDate     string `json:"releaseDate"`
		TrackTimeMillis int    `json:"trackTimeMillis"`
		ArtworkURL100   string `json:"artworkUrl100"`
	} `json:"results"`
}

// itunesGet runs one iTunes Search or Lookup call under the caller's timeout.
func itunesGet(ctx context.Context, query string) (itunesResults, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://itunes.apple.com/"+query, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return itunesResults{}, err
	}
	defer resp.Body.Close()
	var res itunesResults
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return itunesResults{}, err
	}
	return res, nil
}

// lookupTrack finds one exact song in the Apple catalog and its 300px artwork URL:
// the term splits into the artist and the title, looked up in two steps.
func lookupTrack(ctx context.Context, term string) (stage.Track, string, error) {
	words := strings.Fields(term)
	for cut := len(words) - 1; cut >= 1; cut-- {
		prefix := strings.Join(words[:cut], " ")
		res, err := itunesGet(ctx, "search?"+url.Values{"term": {prefix}, "entity": {"musicArtist"}, "limit": {"5"}}.Encode())
		if err != nil {
			return stage.Track{}, "", err
		}
		var artistID int64
		for _, a := range res.Results {
			if a.ArtistName != "" && strings.EqualFold(a.ArtistName, prefix) {
				artistID = a.ArtistID
				break
			}
		}
		if artistID == 0 {
			continue
		}
		title := strings.Join(words[cut:], " ")
		lk, err := itunesGet(ctx, "lookup?"+url.Values{"id": {fmt.Sprint(artistID)}, "entity": {"song"}, "limit": {"200"}}.Encode())
		if err != nil {
			return stage.Track{}, "", err
		}
		for _, t := range lk.Results {
			if t.WrapperType != "track" || !sameTitle(t.TrackName, title) {
				continue
			}
			if len(t.ReleaseDate) > 10 {
				t.ReleaseDate = t.ReleaseDate[:10]
			}
			art := strings.Replace(t.ArtworkURL100, "100x100bb", "300x300bb", 1)
			return stage.Track{Artist: t.ArtistName, Title: t.TrackName, Album: t.CollectionName,
				Released: t.ReleaseDate, Dur: float64(t.TrackTimeMillis) / 1000}, art, nil
		}
	}
	return stage.Track{}, "", fmt.Errorf("no exact match")
}

// sameTitle: equal, or Apple's title is ours plus a bracket — "(feat. …)", "(2019 Digital Master)".
func sameTitle(apple, want string) bool {
	a, w := strings.ToLower(apple), strings.ToLower(want)
	return a == w || strings.HasPrefix(a, w+" (") || strings.HasPrefix(a, w+" [")
}

// fetchImage downloads and decodes one picture.
func fetchImage(ctx context.Context, u string) (image.Image, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	img, _, err := image.Decode(resp.Body)
	return img, err
}
