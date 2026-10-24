package vibeai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type appleAlbum struct {
	ID     string
	Name   string
	Artist string
}

type appleAlbumSearchResponse struct {
	Results struct {
		Albums struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name       string `json:"name"`
					ArtistName string `json:"artistName"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"albums"`
	} `json:"results"`
}

func parseAlbumSearch(body []byte) ([]appleAlbum, error) {
	var r appleAlbumSearchResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("apple: ответ не JSON: %v", err)
	}
	out := make([]appleAlbum, 0, len(r.Results.Albums.Data))
	for _, d := range r.Results.Albums.Data {
		out = append(out, appleAlbum{ID: d.ID, Name: d.Attributes.Name, Artist: d.Attributes.ArtistName})
	}
	return out, nil
}

var editionWords = []string{"deluxe", "edition", "remaster", "version", "expanded", "bonus"}

func hasEditionWord(inner string) bool {
	w := strings.ToLower(inner)
	for _, k := range editionWords {
		if strings.Contains(w, k) {
			return true
		}
	}
	return false
}

func albumNameKey(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, " - single")
	s = strings.TrimSuffix(s, " - ep")
	s = stripBrackets(s, hasEditionWord)
	return normKey(s)
}

func artistsIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func matchAlbum(artist, name string, results []appleAlbum) (appleAlbum, bool) {
	wantArts := splitArtists(artist)
	nameKey := albumNameKey(name)
	var prefix *appleAlbum
	for i, r := range results {
		if !artistsIntersect(wantArts, splitArtists(r.Artist)) {
			continue
		}
		rk := albumNameKey(r.Name)
		if nameKey == rk {
			return r, true
		}
		if prefix == nil && nameKey != "" && rk != "" && (strings.HasPrefix(nameKey, rk) || strings.HasPrefix(rk, nameKey)) {
			prefix = &results[i]
		}
	}
	if prefix != nil {
		return *prefix, true
	}
	return appleAlbum{}, false
}

func searchAlbums(ctx context.Context, api *appleAPI, sf, artist, name string) ([]appleAlbum, error) {
	body, err := api.get(ctx, "/catalog/"+sf+"/search?term="+url.QueryEscape(artist+" "+name)+"&types=albums&limit=10")
	if err != nil {
		return nil, err
	}
	return parseAlbumSearch(body)
}
