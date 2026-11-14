package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

// crateSong is one track of the demo library; the broken upload has no catalog match.
type crateSong struct {
	id, artist, album, title string
	dur                      float64
	upload                   bool
}

// crateAlbum is one record of the crate and the tracks on it.
type crateAlbum struct {
	id, artist, title string
	year              int
	songs             []crateSong
}

// cratePlaylist is a hand-picked list of the crate; broken ones always answer with an error.
type cratePlaylist struct {
	id, title string
	year      int
	picks     []string // "Artist|Title"
	broken    bool
	tracks    []crateSong
}

// crate is the fake Apple Music library the music screen browses.
type crate struct {
	albums    []crateAlbum
	playlists []cratePlaylist
	songs     []crateSong
}

func newCrate() *crate {
	c := &crate{albums: []crateAlbum{
		{id: "utopia", artist: "Travis Scott", title: "UTOPIA", year: 2023, songs: []crateSong{
			{title: "HYAENA", dur: 222}, {title: "THANK GOD", dur: 184}, {title: "MODERN JAM", dur: 255},
			{title: "MY EYES", dur: 251}, {title: "FE!N", dur: 191}, {title: "CIRCUS MAXIMUS", dur: 258},
			{title: "DELRESTO (ECHOES)", dur: 274}, {title: "I KNOW ?", dur: 211},
			{title: "TELEKINESIS", dur: 352}, {title: "TIL FURTHER NOTICE", dur: 314},
		}},
		{id: "blonde", artist: "Frank Ocean", title: "Blonde", year: 2016, songs: []crateSong{
			{title: "Nikes", dur: 314}, {title: "Ivy", dur: 249}, {title: "Pink + White", dur: 184},
			{title: "Solo", dur: 257}, {title: "Skyline To", dur: 199}, {title: "Self Control", dur: 249},
			{title: "Nights", dur: 307}, {title: "Pretty Sweet", dur: 158}, {title: "White Ferrari", dur: 248},
			{title: "Seigfried", dur: 334}, {title: "Godspeed", dur: 177},
		}},
		{id: "wlr", artist: "Playboi Carti", title: "Whole Lotta Red", year: 2020, songs: []crateSong{
			{title: "Rockstar Made", dur: 183}, {title: "Stop Breathing", dur: 219}, {title: "New Tank", dur: 145},
			{title: "Vamp Anthem", dur: 127}, {title: "Sky", dur: 193}, {title: "Over", dur: 163},
			{title: "Teen X", dur: 159}, {title: "Slay!", dur: 180},
		}},
		{id: "chromakopia", artist: "Tyler, The Creator", title: "CHROMAKOPIA", year: 2024, songs: []crateSong{
			{title: "St. Chroma", dur: 206}, {title: "Rah Tah Tah", dur: 179}, {title: "Noid", dur: 254},
			{title: "Darling, I", dur: 266}, {title: "Hey Jane", dur: 276}, {title: "I Killed You", dur: 196},
			{title: "Judge Judy", dur: 224}, {title: "Sticky", dur: 262}, {title: "Balloon", dur: 219},
			{title: "Like Him", dur: 266},
		}},
		{id: "gnx", artist: "Kendrick Lamar", title: "GNX", year: 2024, songs: []crateSong{
			{title: "wacced out murals", dur: 304}, {title: "squabble up", dur: 157}, {title: "luther", dur: 177},
			{title: "man at the garden", dur: 218}, {title: "hey now", dur: 217}, {title: "reincarnated", dur: 264},
			{title: "tv off", dur: 213}, {title: "dodger blue", dur: 220}, {title: "peekaboo", dur: 148},
			{title: "heart pt. 6", dur: 291},
		}},
		{id: "inrainbows", artist: "Radiohead", title: "In Rainbows", year: 2007, songs: []crateSong{
			{title: "15 Step", dur: 237}, {title: "Bodysnatchers", dur: 242}, {title: "Nude", dur: 255},
			{title: "Weird Fishes/Arpeggi", dur: 318}, {title: "All I Need", dur: 228}, {title: "Faust Arp", dur: 129},
			{title: "Reckoner", dur: 290}, {title: "House of Cards", dur: 328},
			{title: "Jigsaw Falling into Place", dur: 248}, {title: "Videotape", dur: 279},
		}},
	}}
	for ai := range c.albums {
		a := &c.albums[ai]
		for si := range a.songs {
			s := &a.songs[si]
			s.id = fmt.Sprintf("tr-%s-%02d", a.id, si+1)
			s.artist, s.album = a.artist, a.title
			c.songs = append(c.songs, *s)
		}
	}
	c.songs = append(c.songs, crateSong{id: "lib-memo-03", artist: "me", title: "voice memo 03", dur: 42, upload: true})

	c.playlists = []cratePlaylist{
		{id: "pl-night", title: "night drive", year: 2024, picks: []string{
			"Travis Scott|HYAENA", "Frank Ocean|Nights", "Radiohead|Weird Fishes/Arpeggi",
			"Kendrick Lamar|luther", "Tyler, The Creator|Darling, I", "Playboi Carti|Sky",
		}},
		{id: "pl-rage", title: "rage radio", year: 2025, picks: []string{
			"Playboi Carti|Stop Breathing", "Travis Scott|FE!N", "Playboi Carti|Rockstar Made",
			"Travis Scott|CIRCUS MAXIMUS", "Playboi Carti|New Tank",
		}},
		{id: "pl-kitchen", title: "sunday kitchen", year: 2023, picks: []string{
			"Frank Ocean|Pink + White", "Radiohead|Nude", "Tyler, The Creator|Hey Jane",
			"Frank Ocean|White Ferrari", "Kendrick Lamar|dodger blue",
		}},
		{id: "pl-broken", title: "broken mix", year: 2022, broken: true},
	}
	for pi := range c.playlists {
		p := &c.playlists[pi]
		for _, pick := range p.picks {
			if s, ok := c.find(pick); ok {
				p.tracks = append(p.tracks, s)
			}
		}
	}
	return c
}

// find looks a song up by "Artist|Title".
func (c *crate) find(pick string) (crateSong, bool) {
	for _, s := range c.songs {
		if s.artist+"|"+s.title == pick {
			return s, true
		}
	}
	return crateSong{}, false
}

func (c *crate) album(id string) *crateAlbum {
	for i := range c.albums {
		if c.albums[i].id == id {
			return &c.albums[i]
		}
	}
	return nil
}

func (c *crate) playlist(id string) *cratePlaylist {
	for i := range c.playlists {
		if c.playlists[i].id == id {
			return &c.playlists[i]
		}
	}
	return nil
}

// songItem is one row of the crate as the screen takes it.
func (c *crate) songItem(s crateSong) stage.MusicItem {
	it := stage.MusicItem{ID: s.id, Kind: stage.MusicSong, Title: s.title, Artist: s.artist, Sub: s.album, Album: s.album, Dur: s.dur}
	if s.upload {
		it.Unplayable = stage.Tr("загрузка без каталога — не играет на Linux", "upload without catalog match — can't play on Linux")
	}
	return it
}

func (c *crate) items(songs []crateSong) []stage.MusicItem {
	out := make([]stage.MusicItem, 0, len(songs))
	for _, s := range songs {
		out = append(out, c.songItem(s))
	}
	return out
}

func (c *crate) albumItem(a crateAlbum) stage.MusicItem {
	return stage.MusicItem{ID: "alb-" + a.id, Kind: stage.MusicAlbum, Title: a.title, Artist: a.artist,
		Sub: fmt.Sprintf("%d · %s", a.year, stage.Tr("альбом", "album")), Count: len(a.songs)}
}

func (c *crate) playlistItem(p cratePlaylist) stage.MusicItem {
	return stage.MusicItem{ID: "pl-" + p.id, Kind: stage.MusicPlaylist, Title: p.title, Artist: stage.Tr("собрано вручную", "handpicked"),
		Sub: fmt.Sprintf("%d · %s", p.year, stage.Tr("плейлист", "playlist")), Count: len(p.tracks)}
}

func (c *crate) favorites() []crateSong {
	picks := []string{
		"Travis Scott|FE!N", "Travis Scott|MY EYES", "Frank Ocean|Nights", "Frank Ocean|Ivy",
		"Playboi Carti|Sky", "Tyler, The Creator|Noid", "Tyler, The Creator|Sticky",
		"Kendrick Lamar|luther", "Kendrick Lamar|squabble up", "Radiohead|Nude",
		"Radiohead|Weird Fishes/Arpeggi", "Playboi Carti|Vamp Anthem", "Frank Ocean|White Ferrari",
		"Kendrick Lamar|heart pt. 6", "Tyler, The Creator|Judge Judy",
	}
	out := make([]crateSong, 0, len(picks)+1)
	for _, p := range picks {
		if s, ok := c.find(p); ok {
			out = append(out, s)
		}
	}
	out = append(out, c.songs[len(c.songs)-1])
	return out
}

func (c *crate) sortedSongs() []crateSong {
	out := append([]crateSong(nil), c.songs...)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].title) < strings.ToLower(out[j].title)
	})
	return out
}

// answer is the crate's reply to one MusicLoad.
func (c *crate) answer(a stage.MusicLoad) ([]stage.MusicItem, string) {
	if a.Parent != nil {
		parent := a.Parent
		if p := c.playlist(strings.TrimPrefix(parent.ID, "pl-")); p != nil {
			if p.broken {
				return nil, "Apple Music: 503"
			}
			return c.items(p.tracks), ""
		}
		if al := c.album(strings.TrimPrefix(parent.ID, "alb-")); al != nil {
			return c.items(al.songs), ""
		}
		return nil, stage.Tr("в Apple Music этого нет", "not in Apple Music")
	}
	switch a.Section {
	case stage.MusicFavorites:
		return c.items(c.favorites()), ""
	case stage.MusicPlaylists:
		out := make([]stage.MusicItem, 0, len(c.playlists))
		for _, p := range c.playlists {
			out = append(out, c.playlistItem(p))
		}
		return out, ""
	case stage.MusicAlbums:
		out := make([]stage.MusicItem, 0, len(c.albums))
		for _, al := range c.albums {
			out = append(out, c.albumItem(al))
		}
		return out, ""
	case stage.MusicSongs:
		return c.items(c.sortedSongs()), ""
	}
	return c.search(a.Query)
}

// search is the case-insensitive substring lookup over titles, artists and albums.
func (c *crate) search(query string) ([]stage.MusicItem, string) {
	q := strings.ToLower(strings.TrimSpace(query))
	var songs, albums, playlists []stage.MusicItem
	for _, s := range c.songs {
		if crateMatch(q, s.title, s.artist, s.album) {
			songs = append(songs, c.songItem(s))
		}
	}
	for _, al := range c.albums {
		if crateMatch(q, al.title, al.artist) {
			albums = append(albums, c.albumItem(al))
		}
	}
	for _, p := range c.playlists {
		if crateMatch(q, p.title) {
			playlists = append(playlists, c.playlistItem(p))
		}
	}
	return append(append(songs, albums...), playlists...), ""
}

func crateMatch(q string, fields ...string) bool {
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// coverKeys is every record of the crate as a track: the keys the covers are fetched for.
func (c *crate) coverTracks() []stage.Track {
	var out []stage.Track
	for _, s := range c.songs {
		if s.upload {
			continue
		}
		out = append(out, stage.Track{ID: s.id, Artist: s.artist, Title: s.title})
	}
	for _, al := range c.albums {
		it := c.albumItem(al)
		out = append(out, stage.Track{ID: it.ID, Artist: it.Artist, Title: it.Title})
	}
	for _, p := range c.playlists {
		it := c.playlistItem(p)
		out = append(out, stage.Track{ID: it.ID, Artist: p.title, Title: stage.Tr("плейлист", "playlist")})
	}
	return out
}

// musicHost answers the music screen: a fake latency on a deterministic clock, then the rows.
type musicHost struct {
	scene *scene
	crate *crate

	pending []musicAnswer
	notes   []musicNote
	n       int
}

type musicNote struct {
	left float64
	text string
}

// musicAnswer is one load on its way back: the seconds left and what will land.
type musicAnswer struct {
	left  float64
	gen   int
	items []stage.MusicItem
	err   string
}

func newMusicHost(s *scene) *musicHost {
	return &musicHost{scene: s, crate: newCrate()}
}

// step delivers the answers whose fake work is done.
func (h *musicHost) step(dt float64) {
	keep := h.pending[:0]
	for _, a := range h.pending {
		a.left -= dt
		if a.left > 0 {
			keep = append(keep, a)
			continue
		}
		h.scene.m.MusicRows(a.gen, a.items, a.err)
	}
	h.pending = keep
	notes := h.notes[:0]
	for _, n := range h.notes {
		n.left -= dt
		if n.left > 0 {
			notes = append(notes, n)
			continue
		}
		h.scene.m.MusicNotice(n.text)
	}
	h.notes = notes
}

// load takes one MusicLoad: the crate answers after 0.3–0.9 s, a search after 0.6 s.
func (h *musicHost) load(a stage.MusicLoad) {
	items, err := h.crate.answer(a)
	lat := 0.3 + 0.6*float64(h.n%7)/6
	if a.Parent == nil && a.Section == stage.MusicSearch {
		lat = 0.6
	}
	h.n++
	h.pending = append(h.pending, musicAnswer{left: lat, gen: a.Gen, items: items, err: err})
}

// play queues the items the screen picked and says what started.
func (h *musicHost) play(a stage.MusicPlay) {
	tracks := make([]stage.Track, 0, len(a.Items))
	for _, it := range a.Items {
		tracks = append(tracks, stage.Track{ID: it.ID, Artist: it.Artist, Title: it.Title, Album: it.Album, Dur: it.Dur})
	}
	if len(tracks) == 0 {
		return
	}
	start := max(0, min(a.Start, len(tracks)-1))
	h.scene.m.JumpPlay(tracks, start, 0)
	h.scene.m.MusicNotice(stage.Tr("играет: ", "playing: ") + tracks[start].Title)
}

// append puts one song at the end of the queue, or opens the player on it when there is none.
func (h *musicHost) append(a stage.MusicAppend) {
	tr := stage.Track{ID: a.Item.ID, Artist: a.Item.Artist, Title: a.Item.Title, Album: a.Item.Album, Dur: a.Item.Dur}
	if h.scene.m.Phase() == stage.PhPlay {
		h.scene.m.PortionDone([]stage.Track{tr}, "")
	} else {
		h.scene.m.JumpPlay([]stage.Track{tr}, 0, 0)
	}
	h.scene.m.MusicNotice(stage.Tr("в очереди: ", "queued: ") + a.Item.Title)
}

// save answers a library or Favorites save with the line the real host would show.
func (h *musicHost) save(a stage.MusicSave) {
	line := stage.Tr("в библиотеке: ", "in library: ")
	if a.Favorites {
		line = stage.Tr("в избранном: ", "in favorites: ")
	}
	h.notes = append(h.notes, musicNote{left: 0.4, text: line + a.Item.Title})
}

// playAll expands a collection from the crate and starts it from the top.
func (h *musicHost) playAll(a stage.MusicPlayAll) {
	var songs []crateSong
	if p := h.crate.playlist(strings.TrimPrefix(a.Item.ID, "pl-")); p != nil {
		songs = p.tracks
	} else if al := h.crate.album(strings.TrimPrefix(a.Item.ID, "alb-")); al != nil {
		songs = al.songs
	}
	if len(songs) == 0 {
		return
	}
	h.play(stage.MusicPlay{Items: h.crate.items(songs), Start: 0, From: &a.Item})
}
