package vibeai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Suggestion struct {
	Artist string
	Title  string
	Why    string
}

type appleSongSearchResponse struct {
	Results struct {
		Songs struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					ArtistName  string `json:"artistName"`
					Name        string `json:"name"`
					AlbumName   string `json:"albumName"`
					ReleaseDate string `json:"releaseDate"`
					Artwork     struct {
						URL string `json:"url"`
					} `json:"artwork"`
					DurationInMillis int `json:"durationInMillis"`
				} `json:"attributes"`
			} `json:"data"`
		} `json:"songs"`
	} `json:"results"`
}

func parseSongSearch(body []byte) ([]Song, error) {
	var r appleSongSearchResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("apple: ответ не JSON: %v", err)
	}
	data := r.Results.Songs.Data
	out := make([]Song, 0, len(data))
	for _, d := range data {
		year := "?"
		if len(d.Attributes.ReleaseDate) >= 4 {
			year = d.Attributes.ReleaseDate[:4]
		}
		out = append(out, Song{
			ID:         d.ID,
			Artist:     d.Attributes.ArtistName,
			Title:      d.Attributes.Name,
			Album:      d.Attributes.AlbumName,
			Year:       year,
			ArtworkURL: d.Attributes.Artwork.URL,
			DurationMs: d.Attributes.DurationInMillis,
		})
	}
	return out, nil
}

func searchSongs(ctx context.Context, api *appleAPI, sf, artist, title string) ([]Song, error) {
	body, err := api.get(ctx, "/catalog/"+sf+"/search?term="+url.QueryEscape(artist+" "+title)+"&types=songs&limit=10")
	if err != nil {
		return nil, err
	}
	return parseSongSearch(body)
}

var versionWordRe = regexp.MustCompile(`(?i)\b(live|remix|sped up|slowed|instrumental|acapella|a cappella|karaoke)\b`)

func hasVersionWord(s string) bool {
	return versionWordRe.MatchString(s)
}

func titleExtends(want, got string) bool {
	return strings.HasPrefix(got, want) && utf8.RuneCountInString(want) >= 4
}

func firstCleanSong(cands []Song) (Song, bool) {
	for _, s := range cands {
		if !hasVersionWord(s.Title) && !hasVersionWord(s.Album) {
			return s, true
		}
	}
	if len(cands) > 0 {
		return cands[0], true
	}
	return Song{}, false
}

func matchSong(sug Suggestion, results []Song) (Song, bool) {
	wantArts := splitArtists(sug.Artist)
	wantKey := normTitle(sug.Title)
	var exact, prefix []Song
	for _, r := range results {
		if !artistsIntersect(wantArts, songArtists(r)) {
			continue
		}
		rk := normTitle(r.Title)
		switch {
		case rk == wantKey:
			exact = append(exact, r)
		case wantKey != "" && rk != "" && titleExtends(wantKey, rk):
			prefix = append(prefix, r)
		}
	}
	if s, ok := firstCleanSong(exact); ok {
		return s, true
	}
	return firstCleanSong(prefix)
}

func songArtists(s Song) []string {
	out := splitArtists(s.Artist)
	seen := make(map[string]bool, len(out))
	for _, a := range out {
		seen[a] = true
	}
	stripBrackets(s.Title, func(inner string) bool {
		w := strings.ToLower(strings.TrimSpace(inner))
		if !featPrefix(inner) || strings.HasPrefix(w, "prod") {
			return false
		}
		fields := strings.Fields(inner)
		if len(fields) > 1 {
			for _, a := range splitArtists(strings.Join(fields[1:], " ")) {
				if !seen[a] {
					seen[a] = true
					out = append(out, a)
				}
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}
