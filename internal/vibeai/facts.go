package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Release struct {
	Name   string `json:"name"`
	Date   string `json:"date"`             // releaseDate as is
	Tracks int    `json:"tracks,omitempty"` // trackCount
}

type ArtistFacts struct {
	Name      string    `json:"name"`
	Genres    []string  `json:"genres"`
	Editorial string    `json:"editorial,omitempty"`
	Albums    []Release `json:"albums"`
	Singles   []Release `json:"singles"`
	Latest    *Release  `json:"latest,omitempty"`
	TopSongs  []string  `json:"top_songs"` // "Artist — Title"
	Tags      []string  `json:"tags,omitempty"`
}

type AlbumFacts struct {
	Artist    string   `json:"artist"`
	Name      string   `json:"name"`
	Date      string   `json:"date"`
	Genres    []string `json:"genres"`
	Editorial string   `json:"editorial,omitempty"`
	Tracks    []string `json:"tracks"`
	Tags      []string `json:"tags,omitempty"`
}

type TrackFacts struct {
	Artist         string   `json:"artist"`
	Title          string   `json:"title"`
	Album          string   `json:"album"`
	Date           string   `json:"date"`
	Genres         []string `json:"genres"`
	AlbumEditorial string   `json:"album_editorial,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func cleanEditorial(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	const maxRunes = 1200
	if n := utf8.RuneCountInString(s); n > maxRunes {
		r := []rune(s)
		s = string(r[:maxRunes]) + "…"
	}
	return s
}

type appleEditorialNotes struct {
	Standard string `json:"standard"`
	Short    string `json:"short"`
}

func editorialText(n *appleEditorialNotes) string {
	if n == nil {
		return ""
	}
	if n.Standard != "" {
		return cleanEditorial(n.Standard)
	}
	return cleanEditorial(n.Short)
}

func cleanGenres(genres []string) []string {
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		if g == "Music" {
			continue
		}
		out = append(out, g)
	}
	return out
}

type appleReleaseView struct {
	Attributes struct {
		Name        string `json:"name"`
		ReleaseDate string `json:"releaseDate"`
		TrackCount  int    `json:"trackCount"`
	} `json:"attributes"`
}

func (v appleReleaseView) release() Release {
	return Release{Name: v.Attributes.Name, Date: v.Attributes.ReleaseDate, Tracks: v.Attributes.TrackCount}
}

type appleArtistFactsResponse struct {
	Data []struct {
		Attributes struct {
			Name           string               `json:"name"`
			GenreNames     []string             `json:"genreNames"`
			EditorialNotes *appleEditorialNotes `json:"editorialNotes"`
		} `json:"attributes"`
		Views struct {
			FullAlbums struct {
				Data []appleReleaseView `json:"data"`
			} `json:"full-albums"`
			Singles struct {
				Data []appleReleaseView `json:"data"`
			} `json:"singles"`
			LatestRelease struct {
				Data []appleReleaseView `json:"data"`
			} `json:"latest-release"`
			TopSongs struct {
				Data []struct {
					Attributes struct {
						Name       string `json:"name"`
						ArtistName string `json:"artistName"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"top-songs"`
		} `json:"views"`
	} `json:"data"`
}

func parseArtistFacts(body []byte) (ArtistFacts, error) {
	var r appleArtistFactsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return ArtistFacts{}, fmt.Errorf("apple: response is not JSON: %v", err)
	}
	if len(r.Data) == 0 {
		return ArtistFacts{}, errors.New("apple: artist not found")
	}
	a := r.Data[0]
	out := ArtistFacts{
		Name:      a.Attributes.Name,
		Genres:    cleanGenres(a.Attributes.GenreNames),
		Editorial: editorialText(a.Attributes.EditorialNotes),
		Albums:    make([]Release, 0, len(a.Views.FullAlbums.Data)),
		Singles:   make([]Release, 0, len(a.Views.Singles.Data)),
		TopSongs:  make([]string, 0, len(a.Views.TopSongs.Data)),
	}
	for _, v := range a.Views.FullAlbums.Data {
		out.Albums = append(out.Albums, v.release())
	}
	for _, v := range a.Views.Singles.Data {
		out.Singles = append(out.Singles, v.release())
	}
	if len(a.Views.LatestRelease.Data) > 0 {
		rel := a.Views.LatestRelease.Data[0].release()
		out.Latest = &rel
	}
	for _, s := range a.Views.TopSongs.Data {
		out.TopSongs = append(out.TopSongs, s.Attributes.ArtistName+" — "+s.Attributes.Name)
	}
	return out, nil
}

type appleAlbumFactsResponse struct {
	Data []struct {
		Attributes struct {
			Name           string               `json:"name"`
			ArtistName     string               `json:"artistName"`
			ReleaseDate    string               `json:"releaseDate"`
			GenreNames     []string             `json:"genreNames"`
			EditorialNotes *appleEditorialNotes `json:"editorialNotes"`
		} `json:"attributes"`
		Relationships struct {
			Tracks struct {
				Data []struct {
					Attributes struct {
						Name string `json:"name"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"tracks"`
		} `json:"relationships"`
	} `json:"data"`
}

func parseAlbumFacts(body []byte) (AlbumFacts, error) {
	var r appleAlbumFactsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return AlbumFacts{}, fmt.Errorf("apple: response is not JSON: %v", err)
	}
	if len(r.Data) == 0 {
		return AlbumFacts{}, errors.New("apple: album not found")
	}
	a := r.Data[0]
	out := AlbumFacts{
		Artist:    a.Attributes.ArtistName,
		Name:      a.Attributes.Name,
		Date:      a.Attributes.ReleaseDate,
		Genres:    cleanGenres(a.Attributes.GenreNames),
		Editorial: editorialText(a.Attributes.EditorialNotes),
		Tracks:    make([]string, 0, len(a.Relationships.Tracks.Data)),
	}
	for _, t := range a.Relationships.Tracks.Data {
		out.Tracks = append(out.Tracks, t.Attributes.Name)
	}
	return out, nil
}

type appleSongFactsResponse struct {
	Data []struct {
		Attributes struct {
			Name        string   `json:"name"`
			ArtistName  string   `json:"artistName"`
			AlbumName   string   `json:"albumName"`
			ReleaseDate string   `json:"releaseDate"`
			GenreNames  []string `json:"genreNames"`
		} `json:"attributes"`
		Relationships struct {
			Albums struct {
				Data []struct {
					Attributes struct {
						EditorialNotes *appleEditorialNotes `json:"editorialNotes"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"albums"`
		} `json:"relationships"`
	} `json:"data"`
}

func parseSongFacts(body []byte) (TrackFacts, error) {
	var r appleSongFactsResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return TrackFacts{}, fmt.Errorf("apple: response is not JSON: %v", err)
	}
	if len(r.Data) == 0 {
		return TrackFacts{}, errors.New("apple: track not found")
	}
	s := r.Data[0]
	out := TrackFacts{
		Artist: s.Attributes.ArtistName,
		Title:  s.Attributes.Name,
		Album:  s.Attributes.AlbumName,
		Date:   s.Attributes.ReleaseDate,
		Genres: cleanGenres(s.Attributes.GenreNames),
	}
	if len(s.Relationships.Albums.Data) > 0 {
		out.AlbumEditorial = editorialText(s.Relationships.Albums.Data[0].Attributes.EditorialNotes)
	}
	return out, nil
}

func describeArtist(ctx context.Context, api *appleAPI, lfm *lastfmClient, sf, name string) (ArtistFacts, error) {
	id, _, err := searchArtistApple(ctx, api, sf, name)
	if err != nil {
		return ArtistFacts{}, err
	}
	if id == "" {
		return ArtistFacts{}, fmt.Errorf("apple: artist \"%s\" not found", name)
	}
	body, err := api.get(ctx, "/catalog/"+sf+"/artists/"+url.PathEscape(id)+"?views=full-albums,singles,latest-release,top-songs&l=en-US")
	if err != nil {
		return ArtistFacts{}, err
	}
	facts, err := parseArtistFacts(body)
	if err != nil {
		return ArtistFacts{}, err
	}
	tags, err := lfm.artistTags(ctx, name)
	if err == nil {
		facts.Tags = filterTags(tags, name, 8)
	}
	return facts, nil
}

func describeAlbum(ctx context.Context, api *appleAPI, lfm *lastfmClient, sf, artist, album string) (AlbumFacts, error) {
	results, err := searchAlbums(ctx, api, sf, artist, album)
	if err != nil {
		return AlbumFacts{}, err
	}
	m, ok := matchAlbum(artist, album, results)
	if !ok {
		return AlbumFacts{}, fmt.Errorf("apple: album \"%s — %s\" not found", artist, album)
	}
	body, err := api.get(ctx, "/catalog/"+sf+"/albums/"+url.PathEscape(m.ID)+"?include=tracks&l=en-US")
	if err != nil {
		return AlbumFacts{}, err
	}
	facts, err := parseAlbumFacts(body)
	if err != nil {
		return AlbumFacts{}, err
	}
	tags, err := lfm.albumTags(ctx, artist, album)
	if err == nil {
		facts.Tags = filterTags(tags, artist, 8)
	}
	return facts, nil
}
