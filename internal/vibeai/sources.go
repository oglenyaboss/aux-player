package vibeai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	appleHost    = "https://amp-api.music.apple.com"
	appleBaseURL = appleHost + "/v1"
)

type similar struct {
	Name  string
	Match float64
}

type lfEnvelope struct {
	Error          json.RawMessage `json:"error"`
	Message        string          `json:"message"`
	Similarartists struct {
		Artist json.RawMessage `json:"artist"`
	} `json:"similarartists"`
}

type lfArtist struct {
	Name  string          `json:"name"`
	Match json.RawMessage `json:"match"`
}

func decodeLastfmList[T any](errRaw json.RawMessage, message string, raw json.RawMessage, what string) ([]T, error) {
	if code := strings.Trim(strings.TrimSpace(string(errRaw)), "\""); code != "" && code != "0" && code != "null" {
		return nil, fmt.Errorf("lastfm: error %s: %s", code, message)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var arr []T
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var one T
	if err := json.Unmarshal(raw, &one); err == nil {
		return []T{one}, nil
	}
	return nil, fmt.Errorf("lastfm: неожиданный формат %s", what)
}

func parseSimilar(body []byte) ([]similar, error) {
	var env lfEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("lastfm: ответ не JSON: %v", err)
	}
	arr, err := decodeLastfmList[lfArtist](env.Error, env.Message, env.Similarartists.Artist, "similarartists.artist")
	if err != nil || arr == nil {
		return nil, err
	}
	return toSimilar(arr), nil
}

func toSimilar(arr []lfArtist) []similar {
	out := make([]similar, 0, len(arr))
	for _, a := range arr {
		out = append(out, similar{Name: a.Name, Match: parseMatch(a.Match)})
	}
	return out
}

func parseMatch(raw json.RawMessage) float64 {
	v, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(string(raw)), "\""), 64)
	if err != nil {
		return 0
	}
	return v
}

type appleSearchResponse struct {
	Results struct {
		Artists struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"artists"`
	} `json:"results"`
}

func parseArtistSearch(body []byte) (id, name string, err error) {
	var r appleSearchResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", fmt.Errorf("apple: ответ не JSON: %v", err)
	}
	if len(r.Results.Artists.Data) == 0 {
		return "", "", nil
	}
	d := r.Results.Artists.Data[0]
	return d.ID, d.Attributes.Name, nil
}

func searchArtistApple(ctx context.Context, api *appleAPI, sf, name string) (id, aname string, err error) {
	body, err := api.get(ctx, "/catalog/"+sf+"/search?term="+url.QueryEscape(name)+"&types=artists&limit=1")
	if err != nil {
		return "", "", err
	}
	return parseArtistSearch(body)
}

func safeErr(err error) error {
	if u := errors.Unwrap(err); u != nil {
		return u
	}
	return err
}

type appleAPI struct {
	http *http.Client
	dev  string
	user string
	base string // пусто → appleBaseURL; для тестов на httptest
}

type appleErrorResponse struct {
	Errors []struct {
		Status json.RawMessage `json:"status"`
	} `json:"errors"`
}

func (a *appleAPI) resolveApplePath(path string) (string, error) {
	host, api := appleHost, appleBaseURL
	if a.base != "" {
		host = strings.TrimRight(a.base, "/")
		api = host
	}
	switch {
	case strings.HasPrefix(path, "/v1/"):
		return host + path, nil
	case strings.HasPrefix(path, "http://"), strings.HasPrefix(path, "https://"):
		pu, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("apple: плохой next: %v", err)
		}
		if pu.Host != "amp-api.music.apple.com" {
			return "", fmt.Errorf("apple: next на чужом хосте: %s", pu.Host)
		}
		return path, nil
	default:
		return api + path, nil
	}
}

func (a *appleAPI) getRaw(ctx context.Context, path string) ([]byte, int, error) {
	u, err := a.resolveApplePath(path)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("apple: %v", safeErr(err))
	}
	req.Header.Set("Authorization", "Bearer "+a.dev)
	req.Header.Set("Music-User-Token", a.user)
	req.Header.Set("Origin", "https://music.apple.com")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("apple: %v", safeErr(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("apple: чтение ответа: %v", err)
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("apple: HTTP %d", resp.StatusCode)
	}
	var er appleErrorResponse
	if json.Unmarshal(body, &er) == nil && len(er.Errors) > 0 {
		return nil, resp.StatusCode, fmt.Errorf("apple: %s", strings.Trim(strings.TrimSpace(string(er.Errors[0].Status)), "\""))
	}
	return body, resp.StatusCode, nil
}

func (a *appleAPI) get(ctx context.Context, path string) ([]byte, error) {
	body, _, err := a.getRaw(ctx, path)
	return body, err
}

func errSnippet(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

func (a *appleAPI) post(ctx context.Context, path string, body any) ([]byte, int, error) {
	u, err := a.resolveApplePath(path)
	if err != nil {
		return nil, 0, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("apple: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, fmt.Errorf("apple: %v", safeErr(err))
	}
	req.Header.Set("Authorization", "Bearer "+a.dev)
	req.Header.Set("Music-User-Token", a.user)
	req.Header.Set("Origin", "https://music.apple.com")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("apple: %v", safeErr(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("apple: чтение ответа: %v", err)
	}
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("apple: HTTP %d: %s", resp.StatusCode, errSnippet(data))
	}
	return data, resp.StatusCode, nil
}

type appleLibraryPage struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			ArtistName string `json:"artistName"`
			Name       string `json:"name"`
			AlbumName  string `json:"albumName"`
			PlayParams *struct {
				CatalogID string `json:"catalogId"`
			} `json:"playParams"`
		} `json:"attributes"`
	} `json:"data"`
	Next string `json:"next"`
}

func parseLibraryPage(body []byte) ([]Song, []string, string, error) {
	var r appleLibraryPage
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, nil, "", fmt.Errorf("apple: ответ не JSON: %v", err)
	}
	out := make([]Song, 0, len(r.Data))
	libIDs := make([]string, 0, len(r.Data))
	for _, d := range r.Data {
		id := ""
		if d.Attributes.PlayParams != nil {
			id = d.Attributes.PlayParams.CatalogID
		}
		if id == "" {
			id = d.ID
		}
		out = append(out, Song{
			ID:     id,
			Artist: d.Attributes.ArtistName,
			Title:  d.Attributes.Name,
			Album:  d.Attributes.AlbumName,
		})
		libIDs = append(libIDs, d.ID)
	}
	return out, libIDs, r.Next, nil
}

type appleRatingsResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Value int `json:"value"`
		} `json:"attributes"`
	} `json:"data"`
}

func parseRatings(body []byte) (map[string]bool, error) {
	var r appleRatingsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("apple: ответ не JSON: %v", err)
	}
	loved := make(map[string]bool, len(r.Data))
	for _, d := range r.Data {
		if d.Attributes.Value == 1 {
			loved[strings.TrimPrefix(d.ID, "r.")] = true
		}
	}
	return loved, nil
}

func fetchLibrary(ctx context.Context, api *appleAPI) ([]Song, []string, error) {
	return fetchPagedSongs(ctx, api, "/me/library/songs?limit=100")
}

func fetchPagedSongs(ctx context.Context, api *appleAPI, firstPath string) ([]Song, []string, error) {
	path := firstPath
	var out []Song
	var libIDs []string
	for {
		body, status, err := api.getRaw(ctx, path)
		if err != nil {
			if status == http.StatusNotFound {
				return out, libIDs, nil
			}
			return nil, nil, err
		}
		songs, ids, next, err := parseLibraryPage(body)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, songs...)
		libIDs = append(libIDs, ids...)
		if next == "" {
			return out, libIDs, nil
		}
		path = next
	}
}

func fetchFavorites(ctx context.Context, api *appleAPI, library []Song, libIDs []string) ([]Song, error) {
	ids := make([]string, 0, len(libIDs))
	seen := make(map[string]bool, len(libIDs))
	for _, id := range libIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	loved := make(map[string]bool)
	for st := 0; st < len(ids); st += 100 {
		l, err := fetchRatingsBatch(ctx, api, ids[st:min(st+100, len(ids))])
		if err != nil {
			return nil, err
		}
		for k, v := range l {
			loved[k] = v
		}
	}
	favs := make([]Song, 0, len(loved))
	for i, s := range library {
		if loved[libIDs[i]] {
			favs = append(favs, s)
		}
	}
	return favs, nil
}

func fetchRatingsBatch(ctx context.Context, api *appleAPI, ids []string) (map[string]bool, error) {
	body, status, err := api.getRaw(ctx, "/me/ratings/library-songs?ids="+url.QueryEscape(strings.Join(ids, ",")))
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusNoContent {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("ratings: %v", err)
	}
	l, err := parseRatings(body)
	if err != nil {
		return nil, fmt.Errorf("ratings: %v", err)
	}
	return l, nil
}
