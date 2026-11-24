package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type searchQuery struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

func (a *Agent) searchTracks(ctx context.Context, qs []searchQuery) string {
	a.progress("search_tracks", fmt.Sprintf("%d queries", len(qs)))
	if len(qs) < 1 || len(qs) > 10 {
		return "error: 1–10 queries"
	}
	songs, errs := searchAll(ctx, a.Backend, qs)
	a.rememberFound(songs...)
	var b strings.Builder
	for i, q := range qs {
		if i > 0 {
			b.WriteString("\n")
		}
		if strings.TrimSpace(q.Title) != "" {
			fmt.Fprintf(&b, "%s — %s:\n", q.Artist, q.Title)
		} else {
			fmt.Fprintf(&b, "%s (top songs):\n", q.Artist)
		}
		switch {
		case errs[i] != nil:
			fmt.Fprintf(&b, "- error: %v\n", errs[i])
		case len(songs[i]) == 0:
			b.WriteString("- nothing found\n")
		default:
			hits := songs[i]
			if len(hits) > 5 {
				hits = hits[:5]
			}
			for _, s := range hits {
				fmt.Fprintf(&b, "- %s · %s — %s · %s · %s · %s\n", s.ID, s.Artist, s.Title, s.Album, s.Year, a.Session.marks(s))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func searchAll(ctx context.Context, b backend, qs []searchQuery) ([][]Song, []error) {
	songs := make([][]Song, len(qs))
	errs := make([]error, len(qs))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			songs[i], errs[i] = b.searchTracks(ctx, q.Artist, q.Title)
		})
	}
	wg.Wait()
	return songs, errs
}

func (a *Agent) rememberFound(groups ...[]Song) {
	if a.found == nil {
		a.found = make(map[string]Song)
	}
	for _, g := range groups {
		for _, s := range g {
			a.found[s.ID] = s
		}
	}
}

// nearestReasons answers every not-found free-text track with the nearest
// catalog hits by that artist: up to 3 intersecting hits of the artist + title
// search, else up to 3 of the artist's top songs.
func (a *Agent) nearestReasons(ctx context.Context, tracks []trackArg, missing []int) []string {
	byTitle := make([]searchQuery, len(missing))
	for j, i := range missing {
		byTitle[j] = searchQuery{Artist: tracks[i].Artist, Title: tracks[i].Title}
	}
	titled, _ := searchAll(ctx, a.Backend, byTitle)
	hits := make([][]Song, len(missing))
	retry := make([]int, 0)
	for j := range missing {
		want := splitArtists(tracks[missing[j]].Artist)
		for _, s := range titled[j] {
			if artistsIntersect(want, songArtists(s)) {
				hits[j] = append(hits[j], s)
			}
		}
		if len(hits[j]) == 0 {
			retry = append(retry, j)
		}
	}
	tops := make([][]Song, 0, len(retry))
	if len(retry) > 0 {
		byArtist := make([]searchQuery, len(retry))
		for k, j := range retry {
			byArtist[k] = searchQuery{Artist: tracks[missing[j]].Artist}
		}
		tops, _ = searchAll(ctx, a.Backend, byArtist)
		for k, j := range retry {
			hits[j] = tops[k]
		}
	}
	a.rememberFound(titled...)
	a.rememberFound(tops...)
	out := make([]string, len(missing))
	for j := range missing {
		if len(hits[j]) == 0 {
			out[j] = "not in Apple Music; nothing by this artist found"
			continue
		}
		if len(hits[j]) > 3 {
			hits[j] = hits[j][:3]
		}
		parts := make([]string, 0, len(hits[j]))
		for _, s := range hits[j] {
			parts = append(parts, fmt.Sprintf("%s · %s — %s (%s, %s)", s.ID, s.Artist, s.Title, s.Album, s.Year))
		}
		out[j] = "not in Apple Music; nearest by this artist: " + strings.Join(parts, "; ")
	}
	return out
}

func (s *Session) marks(song Song) string {
	parts := make([]string, 0, 5)
	if s.lib.Has(song) {
		parts = append(parts, "library")
	}
	if s.favs.Has(song) {
		parts = append(parts, "favorite")
	}
	var rating string
	var plays int
	var last time.Time
	var outcomes []string
	if s.sig != nil {
		rating, plays, last, outcomes = s.sig.trackStats(song)
	}
	switch rating {
	case "more":
		parts = append(parts, "▲")
	case "less":
		parts = append(parts, "▼")
	}
	if s.given[songKey(song)] {
		parts = append(parts, "in this stream")
	}
	if plays > 0 {
		parts = append(parts, fmt.Sprintf("%d× (last %s)", plays, last.Format("2006-01-02")))
	}
	if len(outcomes) > 0 {
		parts = append(parts, "last: "+strings.Join(outcomes, ", "))
	}
	if len(parts) == 0 {
		return "new"
	}
	return strings.Join(parts, ", ")
}

type appleTopSongsResponse struct {
	Data []struct {
		Views struct {
			TopSongs struct {
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
			} `json:"top-songs"`
		} `json:"views"`
	} `json:"data"`
}

func parseTopSongs(body []byte) ([]Song, error) {
	var r appleTopSongsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("apple: response is not JSON: %v", err)
	}
	if len(r.Data) == 0 {
		return nil, errors.New("apple: artist not found")
	}
	data := r.Data[0].Views.TopSongs.Data
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
			Released:   d.Attributes.ReleaseDate,
			ArtworkURL: d.Attributes.Artwork.URL,
			DurationMs: d.Attributes.DurationInMillis,
		})
	}
	return out, nil
}
