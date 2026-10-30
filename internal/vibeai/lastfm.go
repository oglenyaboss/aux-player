package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type tag struct {
	Name  string
	Count int
}

type lfTopTagsEnvelope struct {
	Error   json.RawMessage `json:"error"`
	Message string          `json:"message"`
	Toptags struct {
		Tag json.RawMessage `json:"tag"`
	} `json:"toptags"`
}

type lfTag struct {
	Name  string          `json:"name"`
	Count json.RawMessage `json:"count"`
}

func parseTopTags(body []byte) ([]tag, error) {
	var env lfTopTagsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("lastfm: response is not JSON: %v", err)
	}
	arr, err := decodeLastfmList[lfTag](env.Error, env.Message, env.Toptags.Tag, "toptags.tag")
	if err != nil || arr == nil {
		return nil, err
	}
	return toTags(arr), nil
}

func toTags(arr []lfTag) []tag {
	out := make([]tag, 0, len(arr))
	for _, t := range arr {
		out = append(out, tag{Name: t.Name, Count: parseCount(t.Count)})
	}
	return out
}

func parseCount(raw json.RawMessage) int {
	n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(string(raw)), "\""))
	if err != nil {
		return 0
	}
	return n
}

func isYearTag(name string) bool {
	s := name
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "S") {
		s = s[:len(s)-1]
	}
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func filterTags(tags []tag, artist string, n int) []string {
	if n <= 0 {
		return nil
	}
	artistKey := normKey(artist)
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, min(n, len(tags)))
	for _, t := range tags {
		name := strings.TrimSpace(t.Name)
		k := normKey(name)
		if isYearTag(name) || k == artistKey || k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, name)
		if len(out) == n {
			break
		}
	}
	return out
}

const lastfmBaseURL = "https://ws.audioscrobbler.com/2.0/"

type lastfmClient struct {
	http *http.Client
	key  string
	last time.Time
}

func (l *lastfmClient) call(ctx context.Context, method string, params url.Values) ([]byte, error) {
	if l.key == "" {
		return nil, errors.New("lastfm: no key")
	}
	if !l.last.IsZero() {
		if d := time.Since(l.last); d < 200*time.Millisecond {
			time.Sleep(200*time.Millisecond - d)
		}
	}
	l.last = time.Now()
	v := make(url.Values, len(params)+3)
	maps.Copy(v, params)
	v.Set("method", method)
	v.Set("api_key", l.key)
	v.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lastfmBaseURL+"?"+v.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("lastfm %s: %v", method, safeErr(err))
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lastfm %s: %v", method, safeErr(err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lastfm %s: reading the response: %v", method, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("lastfm %s: HTTP %d", method, resp.StatusCode)
	}
	return body, nil
}

func (l *lastfmClient) similar(ctx context.Context, artist string) ([]similar, error) {
	body, err := l.call(ctx, "artist.getsimilar", url.Values{
		"artist":      {artist},
		"autocorrect": {"1"},
		"limit":       {"30"},
	})
	if err != nil {
		return nil, err
	}
	return parseSimilar(body)
}

func (l *lastfmClient) albumTags(ctx context.Context, artist, album string) ([]tag, error) {
	body, err := l.call(ctx, "album.gettoptags", url.Values{
		"artist":      {artist},
		"album":       {album},
		"autocorrect": {"1"},
	})
	if err != nil {
		return nil, err
	}
	return parseTopTags(body)
}

func (l *lastfmClient) artistTags(ctx context.Context, artist string) ([]tag, error) {
	body, err := l.call(ctx, "artist.gettoptags", url.Values{
		"artist":      {artist},
		"autocorrect": {"1"},
	})
	if err != nil {
		return nil, err
	}
	return parseTopTags(body)
}

func (l *lastfmClient) trackTags(ctx context.Context, artist, track string) ([]tag, error) {
	body, err := l.call(ctx, "track.gettoptags", url.Values{
		"artist":      {artist},
		"track":       {track},
		"autocorrect": {"1"},
	})
	if err != nil {
		return nil, err
	}
	return parseTopTags(body)
}
