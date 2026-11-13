package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/library"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
)

const (
	// musicTimeout bounds every Apple Music call the music screen makes; a library
	// load or refresh may scan the whole library, so it gets longer.
	musicTimeout      = 30 * time.Second
	musicCheckTimeout = 2 * time.Minute

	musicSearchTTL = 10 * time.Minute
	musicSearchMax = 50
)

// musicTarget is the library entry behind a load, as the store keys it.
type musicTarget struct {
	kind library.Kind
	id   string
}

type musicSearchHit struct {
	res *provider.SearchResult
	at  time.Time
}

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

	check *musicTarget           // the stored rows are due a background refresh
	query string                 // the search found, to remember
	found *provider.SearchResult // nil unless Apple answered a search
}

// musicRefreshMsg is ctrl+r: check a section, a collection or with all every
// library section against Apple now.
type musicRefreshMsg struct {
	sec    stage.MusicSection
	parent *stage.MusicItem
	all    bool
}

// musicCheckedMsg is a finished library refresh; manual ones were asked for by
// the user and say how they went.
type musicCheckedMsg struct {
	tgt     musicTarget
	res     library.Result
	changed bool
	manual  bool
	err     error
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
		store := m.musicLibrary()
		if store == nil {
			st.MusicRows(msg.gen, nil, stage.Tr("Apple Music не подключён", "Apple Music is not connected"))
			return nil
		}
		gen, sec, query := msg.gen, msg.sec, msg.query
		tgt, ok := musicTargetFor(sec, msg.parent)
		switch {
		case ok:
			favorites := sec == stage.MusicFavorites && msg.parent == nil
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), musicCheckTimeout)
				defer cancel()
				r, err := store.Load(ctx, tgt.kind, tgt.id)
				if err != nil {
					return musicRowsMsg{gen: gen, err: err}
				}
				out := musicRowsMsg{gen: gen, songs: r.Songs, albums: r.Albums, playlists: r.Playlists}
				if favorites {
					out.songs = library.Favorites(r.Songs)
				}
				if r.Due {
					out.check = &tgt
				}
				return out
			}
		case msg.parent != nil:
			st.MusicRows(gen, nil, msg.parent.Title+stage.Tr(" — не коллекция", " is not a collection"))
			return nil
		}
		if hit, ok := m.musicFound[query]; ok && time.Since(hit.at) < musicSearchTTL {
			res := hit.res
			return func() tea.Msg {
				return musicRowsMsg{gen: gen, songs: res.Tracks, albums: res.Albums, playlists: res.Playlists}
			}
		}
		prov := m.provider
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), musicTimeout)
			defer cancel()
			res, err := prov.Search(ctx, query)
			if err != nil {
				return musicRowsMsg{gen: gen, err: err}
			}
			if res == nil {
				res = &provider.SearchResult{}
			}
			return musicRowsMsg{gen: gen, songs: res.Tracks, albums: res.Albums, playlists: res.Playlists, query: query, found: res}
		}
	case musicRowsMsg:
		if msg.err != nil {
			st.MusicRows(msg.gen, nil, "Apple Music: "+msg.err.Error())
			return nil
		}
		if msg.found != nil {
			m.musicRememberSearch(msg.query, msg.found)
		}
		st.MusicRows(msg.gen, m.musicItems(msg.songs, msg.albums, msg.playlists), "")
		if msg.check != nil {
			return m.musicCheck(*msg.check, false)
		}
		return nil
	case musicRefreshMsg:
		var tgts []musicTarget
		if msg.all {
			m.musicFound = nil
			tgts = []musicTarget{{kind: library.Songs}, {kind: library.Albums}, {kind: library.Playlists}}
		} else if tgt, ok := musicTargetFor(msg.sec, msg.parent); ok {
			tgts = []musicTarget{tgt}
		}
		cmds := make([]tea.Cmd, 0, len(tgts))
		for _, tgt := range tgts {
			cmds = append(cmds, m.musicCheck(tgt, true))
		}
		return tea.Batch(cmds...)
	case musicCheckedMsg:
		m.musicChecks = max(0, m.musicChecks-1)
		st.MusicRefreshing(m.musicChecks > 0)
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[music] refresh %s: %v", msg.tgt.kind, msg.err))
			if msg.manual {
				st.MusicNotice("Apple Music: " + msg.err.Error())
			}
			return nil
		}
		if msg.changed || msg.manual {
			m.musicShow(msg.tgt, msg.res.Songs, msg.res.Albums, msg.res.Playlists)
		}
		if msg.manual && m.musicChecks == 0 {
			st.MusicNotice(stage.Tr("обновлено", "up to date"))
		}
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
		store := m.musicLibrary()
		if store == nil {
			st.MusicNotice(stage.Tr("Apple Music не подключён", "Apple Music is not connected"))
			return nil
		}
		item := msg.item
		tgt, ok := musicTargetFor(0, &item)
		if !ok {
			st.MusicNotice(item.Title + stage.Tr(" — не коллекция", " is not a collection"))
			return nil
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), musicTimeout)
			defer cancel()
			r, err := store.Load(ctx, tgt.kind, tgt.id)
			return musicCollectionMsg{item: item, tracks: r.Songs, err: err}
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

// musicTargetFor is the library entry a load reads: the collection parent, or the
// tab. Favorites and Songs share the one library scan; search has no entry.
func musicTargetFor(sec stage.MusicSection, parent *stage.MusicItem) (musicTarget, bool) {
	if parent != nil {
		switch parent.Kind {
		case stage.MusicAlbum:
			return musicTarget{kind: library.Album, id: parent.ID}, true
		case stage.MusicPlaylist:
			return musicTarget{kind: library.Playlist, id: parent.ID}, true
		}
		return musicTarget{}, false
	}
	switch sec {
	case stage.MusicFavorites, stage.MusicSongs:
		return musicTarget{kind: library.Songs}, true
	case stage.MusicAlbums:
		return musicTarget{kind: library.Albums}, true
	case stage.MusicPlaylists:
		return musicTarget{kind: library.Playlists}, true
	}
	return musicTarget{}, false
}

// musicLibrary is the user's library store, opened on first use for the current
// provider and token.
func (m *Model) musicLibrary() *library.Store {
	if m.library == nil && m.provider != nil {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		m.library = library.Open(filepath.Join(dir, "aux", "library"), m.provider, m.cfg.AppleUserToken)
	}
	return m.library
}

// musicCheck refreshes one library entry in the background; a manual check skips
// the song count shortcut and reports back in the screen.
func (m *Model) musicCheck(tgt musicTarget, manual bool) tea.Cmd {
	store := m.musicLibrary()
	if store == nil {
		return nil
	}
	m.musicChecks++
	m.vibeAIP.m.st.MusicRefreshing(true)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), musicCheckTimeout)
		defer cancel()
		r, changed, err := store.Refresh(ctx, tgt.kind, tgt.id, manual)
		return musicCheckedMsg{tgt: tgt, res: r, changed: changed, manual: manual, err: err}
	}
}

// musicShow hands fresh library rows to the sections and collections that show them.
func (m *Model) musicShow(tgt musicTarget, songs []provider.Track, albums []provider.Album, playlists []provider.Playlist) {
	st := m.vibeAIP.m.st
	switch tgt.kind {
	case library.Songs:
		st.MusicUpdate(stage.MusicSongs, "", m.musicItems(songs, nil, nil))
		st.MusicUpdate(stage.MusicFavorites, "", m.musicItems(library.Favorites(songs), nil, nil))
	case library.Albums:
		st.MusicUpdate(stage.MusicAlbums, "", m.musicItems(nil, albums, nil))
	case library.Playlists:
		st.MusicUpdate(stage.MusicPlaylists, "", m.musicItems(nil, nil, playlists))
	case library.Album, library.Playlist:
		st.MusicUpdate(stage.MusicFavorites, tgt.id, m.musicItems(songs, nil, nil))
	}
}

// musicItems turns provider rows into screen rows, remembering the tracks and
// where each row's artwork lives.
func (m *Model) musicItems(songs []provider.Track, albums []provider.Album, playlists []provider.Playlist) []stage.MusicItem {
	v := m.vibeAIP.m
	items := make([]stage.MusicItem, 0, len(songs)+len(albums)+len(playlists))
	for _, t := range songs {
		it := musicSongItem(t)
		items = append(items, it)
		m.musicRemember(t)
		v.noteArt(it.ID, t.ArtworkURL)
	}
	for _, a := range albums {
		it := musicAlbumItem(a)
		items = append(items, it)
		v.noteArt(it.ID, a.ArtworkURL)
	}
	for _, p := range playlists {
		it := musicPlaylistItem(p)
		items = append(items, it)
		v.noteArt(it.ID, p.ArtworkURL)
	}
	return items
}

func (m *Model) musicRememberSearch(query string, res *provider.SearchResult) {
	if m.musicFound == nil || len(m.musicFound) >= musicSearchMax {
		// ponytail: a full map starts over; keep an LRU if searches repeat past fifty queries.
		m.musicFound = map[string]musicSearchHit{}
	}
	m.musicFound[query] = musicSearchHit{res: res, at: time.Now()}
}
