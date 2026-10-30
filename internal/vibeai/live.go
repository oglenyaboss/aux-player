package vibeai

import (
	"context"
	"fmt"
	"net/url"
	"sync"
)

type liveBackend struct {
	api *appleAPI
	lfm *lastfmClient
	sf  string
	mu  sync.Mutex // сериализует вызовы lfm: lastfmClient не потокобезопасен
}

func (b *liveBackend) describe(ctx context.Context, kind, artist, name string) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch kind {
	case "artist":
		return describeArtist(ctx, b.api, b.lfm, b.sf, artist)
	case "album":
		return describeAlbum(ctx, b.api, b.lfm, b.sf, artist, name)
	case "track":
		return describeTrack(ctx, b.api, b.lfm, b.sf, artist, name)
	default:
		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

func (b *liveBackend) similarArtists(ctx context.Context, artist string) ([]similar, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lfm.similar(ctx, artist)
}

func (b *liveBackend) findSong(ctx context.Context, sug Suggestion) (Song, bool, error) {
	results, err := searchSongs(ctx, b.api, b.sf, sug.Artist, sug.Title)
	if err != nil {
		return Song{}, false, err
	}
	song, ok := matchSong(sug, results)
	return song, ok, nil
}

func describeTrack(ctx context.Context, api *appleAPI, lfm *lastfmClient, sf, artist, title string) (TrackFacts, error) {
	results, err := searchSongs(ctx, api, sf, artist, title)
	if err != nil {
		return TrackFacts{}, err
	}
	m, ok := matchSong(Suggestion{Artist: artist, Title: title}, results)
	if !ok {
		return TrackFacts{}, fmt.Errorf("apple: track \"%s — %s\" not found", artist, title)
	}
	body, err := api.get(ctx, "/catalog/"+sf+"/songs/"+url.PathEscape(m.ID)+"?include=albums&l=en-US")
	if err != nil {
		return TrackFacts{}, err
	}
	facts, err := parseSongFacts(body)
	if err != nil {
		return TrackFacts{}, err
	}
	tags, err := lfm.trackTags(ctx, artist, title)
	if err == nil {
		facts.Tags = filterTags(tags, artist, 8)
	}
	return facts, nil
}
