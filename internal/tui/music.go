package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
)

// musicTimeout bounds every Apple Music call the music screen makes.
const musicTimeout = 30 * time.Second

type musicLoadMsg struct {
	gen    int
	sec    stage.MusicSection
	query  string
	parent *stage.MusicItem
}

// musicRowsMsg is the provider's answer to a load, still in provider types: the
// rows are built on the Update goroutine once it lands.
type musicRowsMsg struct {
	gen       int
	songs     []provider.Track
	albums    []provider.Album
	playlists []provider.Playlist
	err       error
}

type musicPlayMsg struct {
	items []stage.MusicItem
	start int
}

type musicPlayAllMsg struct {
	item stage.MusicItem
}

// musicCollectionMsg is the expanded track list behind a play-all.
type musicCollectionMsg struct {
	item   stage.MusicItem
	tracks []provider.Track
	err    error
}

type musicAppendMsg struct {
	item stage.MusicItem
}

// handleMusicMsg answers the music screen. Loads fetch on a command goroutine and
// land here again as rows; play, append and the landed answers work the queue and
// the stage directly.
func (m *Model) handleMusicMsg(msg tea.Msg) tea.Cmd {
	st := m.vibeAIP.m.st
	switch msg := msg.(type) {
	case musicLoadMsg:
		prov := m.provider
		if prov == nil {
			st.MusicRows(msg.gen, nil, stage.Tr("Apple Music не подключён", "Apple Music is not connected"))
			return nil
		}
		gen, sec, query := msg.gen, msg.sec, msg.query
		var parent *stage.MusicItem
		if msg.parent != nil {
			p := *msg.parent
			parent = &p
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), musicTimeout)
			defer cancel()
			out := musicRowsMsg{gen: gen}
			var err error
			switch {
			case parent != nil:
				out.songs, err = musicCollectionTracks(ctx, prov, *parent)
			case sec == stage.MusicFavorites:
				out.songs, err = prov.GetPlaylistTracks(ctx, provider.FavoritesPlaylistID)
			case sec == stage.MusicPlaylists:
				var pls []provider.Playlist
				if pls, err = prov.GetLibraryPlaylists(ctx); err == nil {
					for _, p := range pls {
						if p.ID != provider.FavoritesPlaylistID {
							out.playlists = append(out.playlists, p)
						}
					}
				}
			case sec == stage.MusicAlbums:
				out.albums, err = prov.GetLibraryAlbums(ctx)
			case sec == stage.MusicSongs:
				out.songs, err = prov.GetLibraryTracks(ctx)
			default:
				var res *provider.SearchResult
				if res, err = prov.Search(ctx, query); err == nil && res != nil {
					out.songs, out.albums, out.playlists = res.Tracks, res.Albums, res.Playlists
				}
			}
			if err != nil {
				return musicRowsMsg{gen: gen, err: err}
			}
			return out
		}
	case musicRowsMsg:
		if msg.err != nil {
			st.MusicRows(msg.gen, nil, "Apple Music: "+msg.err.Error())
			return nil
		}
		v := m.vibeAIP.m
		items := make([]stage.MusicItem, 0, len(msg.songs)+len(msg.albums)+len(msg.playlists))
		for _, t := range msg.songs {
			it := musicSongItem(t)
			items = append(items, it)
			m.musicRemember(t)
			v.wantArt(it.ID, t.ArtworkURL)
		}
		for _, a := range msg.albums {
			it := musicAlbumItem(a)
			items = append(items, it)
			v.wantArt(it.ID, a.ArtworkURL)
		}
		for _, p := range msg.playlists {
			it := musicPlaylistItem(p)
			items = append(items, it)
			v.wantArt(it.ID, p.ArtworkURL)
		}
		st.MusicRows(msg.gen, items, "")
		return nil
	case musicPlayMsg:
		var tracks []provider.Track
		for _, it := range msg.items {
			if it.Unplayable != "" {
				continue
			}
			tracks = append(tracks, m.musicTrackFor(it))
		}
		if len(tracks) == 0 {
			return nil
		}
		start := max(0, min(msg.start, len(tracks)-1))
		// ponytail: the rows before Start are not queued because SetQueue starts at the
		// first ID; queue the whole list and skip to Start once the player can.
		return m.musicPlayTracks(tracks[start:])
	case musicPlayAllMsg:
		prov := m.provider
		if prov == nil {
			st.MusicNotice(stage.Tr("Apple Music не подключён", "Apple Music is not connected"))
			return nil
		}
		item := msg.item
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), musicTimeout)
			defer cancel()
			tracks, err := musicCollectionTracks(ctx, prov, item)
			return musicCollectionMsg{item: item, tracks: tracks, err: err}
		}
	case musicCollectionMsg:
		if msg.err != nil {
			st.MusicNotice("Apple Music: " + msg.err.Error())
			return nil
		}
		tracks := make([]provider.Track, 0, len(msg.tracks))
		for _, t := range msg.tracks {
			if musicUploaded(t) {
				continue
			}
			tracks = append(tracks, t)
		}
		return m.musicPlayTracks(tracks)
	case musicAppendMsg:
		if msg.item.Unplayable != "" {
			return nil
		}
		t := m.musicTrackFor(msg.item)
		var cmd tea.Cmd
		if st.Phase() == stage.PhPlay {
			cmd = m.appendQueue([]provider.Track{t})
			st.PortionDone([]stage.Track{musicStageTrack(t)}, "")
		} else {
			cmd = m.musicPlayTracks([]provider.Track{t})
		}
		st.MusicNotice(stage.Tr("в очереди: ", "queued: ") + msg.item.Title)
		return cmd
	}
	return nil
}

// musicPlayTracks replaces the queue with tracks and opens the player on the first.
func (m *Model) musicPlayTracks(tracks []provider.Track) tea.Cmd {
	if len(tracks) == 0 {
		return nil
	}
	m.cancelVibeAI()
	cmd := m.setQueue(tracks)
	cards := make([]stage.Track, len(tracks))
	for i, t := range tracks {
		cards[i] = musicStageTrack(t)
	}
	st := m.vibeAIP.m.st
	st.JumpPlay(cards, 0, 0)
	st.MusicNotice(stage.Tr("играет: ", "playing: ") + tracks[0].Title)
	return cmd
}

func musicStageTrack(t provider.Track) stage.Track {
	return stage.Track{ID: PlaybackID(t), Artist: t.Artist, Title: t.Title, Album: t.Album, Dur: t.Duration.Seconds()}
}

// musicTrackFor is the provider track behind a row: the one its load brought, or a
// bare one rebuilt from the row when it is no longer remembered.
func (m *Model) musicTrackFor(it stage.MusicItem) provider.Track {
	if t, ok := m.musicTracks[it.ID]; ok {
		return t
	}
	return provider.Track{ID: it.ID, CatalogID: it.ID, Title: it.Title, Artist: it.Artist,
		Album: it.Album, Duration: time.Duration(it.Dur * float64(time.Second))}
}

func (m *Model) musicRemember(t provider.Track) {
	if m.musicTracks == nil {
		m.musicTracks = map[string]provider.Track{}
	}
	m.musicTracks[PlaybackID(t)] = t
}

// musicCollectionTracks loads the tracks of a collection the way a load parent goes.
func musicCollectionTracks(ctx context.Context, prov provider.Provider, it stage.MusicItem) ([]provider.Track, error) {
	switch it.Kind {
	case stage.MusicPlaylist:
		return prov.GetPlaylistTracks(ctx, it.ID)
	case stage.MusicAlbum:
		if strings.HasPrefix(it.ID, "l.") {
			return prov.GetLibraryAlbumTracks(ctx, it.ID)
		}
		return prov.GetAlbumTracks(ctx, it.ID)
	}
	return nil, fmt.Errorf("%s is not a collection", it.Title)
}

// musicUploaded reports a library track without a catalog match: it cannot play here.
func musicUploaded(t provider.Track) bool {
	return strings.HasPrefix(t.ID, "i.") && t.CatalogID == ""
}

func musicSongItem(t provider.Track) stage.MusicItem {
	it := stage.MusicItem{ID: PlaybackID(t), Kind: stage.MusicSong, Title: t.Title, Artist: t.Artist,
		Album: t.Album, Sub: t.Album, Dur: t.Duration.Seconds()}
	if musicUploaded(t) {
		it.Unplayable = stage.Tr("загрузка без каталога — не играет на Linux", "upload without catalog match — can't play on Linux")
	}
	return it
}

func musicAlbumItem(a provider.Album) stage.MusicItem {
	sub := stage.Tr("альбом", "album")
	if len(a.ReleaseDate) >= 4 {
		sub = a.ReleaseDate[:4] + " · " + sub
	}
	return stage.MusicItem{ID: a.ID, Kind: stage.MusicAlbum, Title: a.Title, Artist: a.Artist,
		Count: a.TrackCount, Sub: sub}
}

func musicPlaylistItem(p provider.Playlist) stage.MusicItem {
	return stage.MusicItem{ID: p.ID, Kind: stage.MusicPlaylist, Title: p.Name, Count: p.TrackCount,
		Sub: stage.Tr("плейлист", "playlist")}
}
