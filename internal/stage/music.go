package stage

import (
	"fmt"
	"slices"
	"strings"
)

// MusicKind is what a row of the music screen is: a song, or a collection of them.
type MusicKind int

const (
	MusicSong MusicKind = iota
	MusicAlbum
	MusicPlaylist
)

// MusicSection is the tab the rows belong to.
type MusicSection int

const (
	MusicFavorites MusicSection = iota
	MusicPlaylists
	MusicAlbums
	MusicSongs
	MusicSearch
)

// MusicItem is one row of the music screen as the host knows it.
type MusicItem struct {
	ID         string
	Kind       MusicKind
	Title      string
	Artist     string
	Sub        string // "2023 · album"
	Album      string // the album of a song; the album eggs match on it
	Dur        float64
	Count      int    // tracks in a collection
	Unplayable string // why it cannot play here; "" plays
}

// Key is the cover and theme key of the row: ID when set, otherwise Artist + " " + Title.
func (it MusicItem) Key() string {
	if it.ID != "" {
		return it.ID
	}
	return it.Artist + " " + it.Title
}

// eggTrack is the row as the egg rules and the theme see it: an album is its own album.
func (it MusicItem) eggTrack() Track {
	t := Track{ID: it.ID, Artist: it.Artist, Title: it.Title, Album: it.Album}
	if it.Kind == MusicAlbum {
		t.Album = it.Title
	}
	return t
}

// MusicLoad asks the host for rows: a tab's rows, or the tracks of Parent when it is set.
type MusicLoad struct {
	Gen     int
	Section MusicSection
	Query   string
	Parent  *MusicItem
}

// MusicPlay asks the host to play Items from Start; From is the collection they came from.
type MusicPlay struct {
	Items []MusicItem
	Start int
	From  *MusicItem
}

// MusicAppend asks the host to put one song at the end of the queue.
type MusicAppend struct {
	Item MusicItem
}

// MusicSave asks the host to put one song into the library or into Favorites.
type MusicSave struct {
	Item      MusicItem
	Favorites bool
}

// MusicRefresh asks the host to check rows against Apple now: the collection Parent,
// the tab Section, or with All every library tab.
type MusicRefresh struct {
	Section MusicSection
	Parent  *MusicItem
	All     bool
}

// MusicPlayAll asks the host to play a whole collection; the host expands it, since the
// rows of a collection are not known before it is opened.
type MusicPlayAll struct {
	Item MusicItem
}

// musicTab is the rows and the request state of one section.
type musicTab struct {
	items   []MusicItem
	query   string
	err     string
	sent    bool // a load went out and has not failed
	loading bool
	sel     int
}

// musicColl is the tracks of one opened album or playlist.
type musicColl struct {
	parent  MusicItem
	section MusicSection
	items   []MusicItem
	err     string
	loading bool
	sel     int
}

// The looks of the music screen: the full-screen list, a palette dropping over the
// stage, a drawer beside the stage.
const (
	musicStylePalette = "palette"
	musicStyleDrawer  = "drawer"
	musicStyleFull    = "full"
)

const (
	musicDebounce   = 0.25 // a pause in the typing sends the search
	musicSlide      = 0.18 // the palette drops in over this much time
	musicPaletteDim = 0.55 // the stage behind an open palette
)

// musicStyle is the look the model was built for; the empty option is the palette.
func (m *Model) musicStyle() string {
	switch m.o.MusicStyle {
	case musicStyleDrawer, musicStyleFull:
		return m.o.MusicStyle
	}
	return musicStylePalette
}

// drawMusic draws the music screen in the look the model was built for.
func (m *Model) drawMusic(f *fb) {
	switch m.musicStyle() {
	case musicStyleDrawer:
		m.drawMusicDrawer(f)
	case musicStylePalette:
		m.drawMusicPalette(f)
	default:
		m.drawMusicFull(f)
	}
}

// music is the state of the music screen; it lives across reset() like the recent list.
type music struct {
	open bool

	tab  MusicSection
	tabs [5]musicTab
	coll *musicColl

	list    musicList // the palette and drawer navigation
	dimPrev float64   // the stage dim the palette is to give back
	shut    bool      // the palette is closing: it draws until the slide is over

	gen     int // the latest MusicLoad; older answers are dropped
	reqTab  MusicSection
	reqColl bool

	focus   bool // the search field owns the keys
	notice  string
	noticeT float64
	selT    float64 // the highlight changed here: the preview title reveals

	mode      mlMode // the palette talks to the agent or browses the music
	menu      mlMenu // the actions menu of the highlighted row
	detKey    string // the item the detail pane shows, and the one it fades from
	detPrev   string
	detT      float64
	rowY      int // the selected row inside the panel: the menu anchors to it
	rowX      int
	rowL      int // where the selected row's body starts and how tall it is
	rowH      int
	gridTop   int // the first visible tile row of the grid
	tileX     int // the top-left of the selected tile of the grid, same frame
	tileY     int
	glide     spring // the selection marker inside the list, in panel rows
	glideSnap bool
	cascT     float64                // the rows landed here: they cascade in from this clock
	peek      map[string][]MusicItem // the tracks of a collection already opened, by its key

	refreshing bool // the host is checking rows against Apple

	playKey string // the row a MusicPlay went out for
	playT   float64

	prevTo theme // the palette to come back to when the music closes outside the player
}

// OpenMusic opens the music screen over whatever the stage draws now.
func (m *Model) OpenMusic() { m.openMusic(mlModeMusic) }

// openMusic opens the music screen in the given mode.
func (m *Model) openMusic(mode mlMode) {
	if m.mu.open || m.ph == phOnboard {
		return
	}
	m.mu.mode = mode
	m.mu.menu = mlMenu{}
	m.mu.open, m.mu.focus, m.mu.coll = true, false, nil
	m.mu.notice = ""
	m.mu.prevTo = m.to
	m.mu.dimPrev = m.dim.to
	m.recentOpen, m.help = false, false
	if m.musicStyle() != musicStyleFull {
		m.mu.list, m.mu.shut = musicList{root: true, openT: m.t}, false
		m.mu.glideSnap, m.mu.cascT = true, m.t
		if m.musicStyle() == musicStylePalette {
			m.dim.to = musicPaletteDim
		}
		m.mlSettle()
		m.musicTheme()
		return
	}
	m.musicLoadTab(m.mu.tab)
	m.musicTheme()
}

// MusicOpen reports whether the music screen is up.
func (m *Model) MusicOpen() bool { return m.mu.open }

// MusicRows answers a MusicLoad; the rows of an older request are dropped.
func (m *Model) MusicRows(gen int, items []MusicItem, err string) {
	if gen != m.mu.gen {
		return
	}
	had := len(m.mlState().items) > 0
	if m.mu.reqColl {
		c := m.mu.coll
		if c == nil {
			return
		}
		c.loading, c.err = false, err
		if err == "" {
			c.items = slices.Clone(items)
			c.sel = musicClamp(c.sel, len(c.items))
			if m.mu.peek == nil {
				m.mu.peek = map[string][]MusicItem{}
			}
			m.mu.peek[c.parent.Key()] = slices.Clone(c.items)
		}
		m.mu.selT, m.mu.cascT, m.mu.glideSnap = m.t, m.t, true
		m.mlAfterRows(had)
		return
	}
	t := &m.mu.tabs[m.mu.reqTab]
	t.loading, t.err = false, err
	if err == "" {
		t.items = slices.Clone(items)
		t.sel = musicClamp(t.sel, len(t.items))
	}
	m.mu.selT, m.mu.cascT, m.mu.glideSnap = m.t, m.t, true
	m.mlAfterRows(had)
}

// MusicUpdate swaps in fresher rows for the tab sec, or for the collection keyed
// parentKey, keeping the highlight on the same row. A tab never loaded, a load in
// flight and a failed load are left alone: their own answer is still to come.
func (m *Model) MusicUpdate(sec MusicSection, parentKey string, items []MusicItem) {
	selKey := ""
	if l, ok := m.mlSelected(); ok && m.mu.open {
		selKey = l.it.Key()
	}
	swapped := false
	if parentKey != "" {
		if _, ok := m.mu.peek[parentKey]; ok {
			m.mu.peek[parentKey] = slices.Clone(items)
		}
		if c := m.mu.coll; c != nil && c.parent.Key() == parentKey && !c.loading && c.err == "" {
			c.sel = musicKeepSel(c.items, c.sel, items)
			c.items = slices.Clone(items)
			swapped = true
		}
	} else if t := &m.mu.tabs[sec]; t.sent && !t.loading && t.err == "" {
		t.sel = musicKeepSel(t.items, t.sel, items)
		t.items = slices.Clone(items)
		swapped = true
	}
	if swapped && m.musicStyle() != musicStyleFull {
		m.mlReselect(selKey)
	}
}

// musicKeepSel is where the row at sel in old sits in fresh, or sel clamped.
func musicKeepSel(old []MusicItem, sel int, fresh []MusicItem) int {
	if sel >= 0 && sel < len(old) {
		key := old[sel].Key()
		for i, it := range fresh {
			if it.Key() == key {
				return i
			}
		}
	}
	return musicClamp(sel, len(fresh))
}

// MusicRefreshing tells the screen whether the host is checking rows against Apple.
func (m *Model) MusicRefreshing(on bool) { m.mu.refreshing = on }

// MusicNotice is the transient line inside the music screen: what the host just did.
func (m *Model) MusicNotice(s string) {
	m.mu.notice, m.mu.noticeT = strings.TrimSpace(s), m.t
}

func musicClamp(sel, n int) int {
	if n == 0 || sel < 0 || sel >= n {
		return 0
	}
	return sel
}

func (m *Model) closeMusic() {
	m.mu.open, m.mu.focus, m.mu.notice = false, false, ""
	m.mu.menu = mlMenu{}
	if style := m.musicStyle(); style != musicStyleFull {
		m.mu.list.shutT = m.t
		if style == musicStylePalette {
			m.mu.shut = true
			m.dim.to = m.mu.dimPrev
		}
	}
	if m.ph == phPlay && len(m.queue) > 0 {
		m.to = m.themeFor(*m.curTrack())
		return
	}
	m.to = m.mu.prevTo
}

// musicSend bumps the generation and asks the host for the rows of one load.
func (m *Model) musicSend(sec MusicSection, parent *MusicItem) {
	m.mu.gen++
	query := ""
	if parent == nil {
		query = m.mu.tabs[sec].query
	}
	m.send(MusicLoad{Gen: m.mu.gen, Section: sec, Query: query, Parent: parent})
}

// musicLoadTab asks for a tab's rows unless it has them already.
func (m *Model) musicLoadTab(sec MusicSection) {
	t := &m.mu.tabs[sec]
	if t.sent && t.err == "" {
		return
	}
	t.sent, t.loading, t.err = true, true, ""
	m.mu.reqTab, m.mu.reqColl = sec, false
	m.musicSend(sec, nil)
}

// musicView is the list the screen shows now: a collection's tracks or a tab's rows.
type musicView struct {
	items   []MusicItem
	sel     int
	loading bool
	err     string
	coll    *musicColl
	grouped bool
}

func (m *Model) musicViewState() musicView {
	if c := m.mu.coll; c != nil {
		return musicView{items: c.items, sel: c.sel, loading: c.loading, err: c.err, coll: c}
	}
	t := &m.mu.tabs[m.mu.tab]
	return musicView{items: t.items, sel: t.sel, loading: t.loading, err: t.err,
		grouped: m.mu.tab == MusicSearch}
}

func (m *Model) musicSetSel(v *musicView, sel int) {
	if v.coll != nil {
		v.coll.sel = sel
		return
	}
	m.mu.tabs[m.mu.tab].sel = sel
}

func (m *Model) musicHighlight() (MusicItem, bool) {
	if m.musicStyle() != musicStyleFull {
		if l, ok := m.mlSelected(); ok && (l.kind == mlSong || l.kind == mlTile) {
			return l.it, true
		}
		return MusicItem{}, false
	}
	v := m.musicViewState()
	if v.sel < 0 || v.sel >= len(v.items) {
		return MusicItem{}, false
	}
	return v.items[v.sel], true
}

// musicPreviewItem is what the right column shows: inside a collection its cover, otherwise the highlight.
func (m *Model) musicPreviewItem() (MusicItem, bool) {
	if c := m.mu.coll; c != nil {
		return c.parent, true
	}
	return m.musicHighlight()
}

// musicTheme points the screen palette at the highlighted row's cover.
func (m *Model) musicTheme() {
	if !m.mu.open {
		return
	}
	if m.mu.mode == mlModeAgent {
		if m.ph == phPlay && len(m.queue) > 0 {
			m.to = m.themeFor(*m.curTrack())
		}
		return
	}
	if it, ok := m.musicHighlight(); ok {
		m.to = m.themeFor(it.eggTrack())
	}
}

// musicBody is the list area: the first row and the row the bottom strip starts at.
func (m *Model) musicBody() (top, limit int) {
	top = 4
	if m.mu.coll != nil {
		top++
	}
	limit = m.h - 3
	if m.musicNoticeVisible() {
		limit--
	}
	return top, max(top, limit)
}

func (m *Model) musicNoticeVisible() bool {
	return m.mu.notice != "" && m.t-m.mu.noticeT < 3.2
}

func (m *Model) musicPage() int {
	top, limit := m.musicBody()
	return max(1, limit-top-2)
}

func (m *Model) musicSelect(sel int) {
	v := m.musicViewState()
	n := len(v.items)
	if n == 0 {
		return
	}
	sel = max(0, min(sel, n-1))
	if sel == v.sel {
		return
	}
	m.musicSetSel(&v, sel)
	m.mu.selT = m.t
}

func (m *Model) musicMove(d int) { m.musicSelect(m.musicViewState().sel + d) }

func (m *Model) musicSwitch(sec MusicSection) {
	if sec < MusicFavorites {
		sec = MusicSearch
	}
	if sec > MusicSearch {
		sec = MusicFavorites
	}
	m.mu.coll = nil
	m.mu.focus = false
	m.mu.tab = sec
	m.musicLoadTab(sec)
}

func (m *Model) musicEnter() {
	v := m.musicViewState()
	if v.loading || v.err != "" || v.sel < 0 || v.sel >= len(v.items) {
		return
	}
	it := v.items[v.sel]
	if it.Unplayable != "" {
		m.MusicNotice(it.Unplayable)
		return
	}
	if it.Kind != MusicSong {
		parent := it
		m.mu.coll = &musicColl{parent: parent, section: m.mu.tab, loading: true}
		m.mu.reqTab, m.mu.reqColl = m.mu.tab, true
		m.musicSend(m.mu.tab, &parent)
		return
	}
	if v.coll != nil || m.mu.tab == MusicFavorites {
		from := &MusicItem{Kind: MusicPlaylist, Title: tr("избранное", "favorites")}
		if v.coll != nil {
			from = &v.coll.parent
		}
		items, start := musicSongs(v.items, v.sel)
		m.send(MusicPlay{Items: items, Start: start, From: from})
	} else {
		m.send(MusicPlay{Items: []MusicItem{it}, Start: 0})
	}
	m.mu.playKey, m.mu.playT = it.Key(), m.t
}

// musicSongs is the playable part of a list and where the selected row sits in it.
func musicSongs(items []MusicItem, sel int) (out []MusicItem, start int) {
	at := -1
	for i, it := range items {
		if it.Kind != MusicSong || it.Unplayable != "" {
			continue
		}
		if i == sel {
			at = len(out)
		}
		out = append(out, it)
	}
	if at < 0 {
		at = 0
	}
	return out, at
}

func (m *Model) musicAppend() {
	v := m.musicViewState()
	if v.sel < 0 || v.sel >= len(v.items) {
		return
	}
	it := v.items[v.sel]
	if it.Unplayable != "" {
		m.MusicNotice(it.Unplayable)
		return
	}
	if it.Kind != MusicSong {
		m.MusicNotice(tr("в очередь добавляется трек — открой коллекцию", "append works on a track — open the collection"))
		return
	}
	m.send(MusicAppend{Item: it})
}

func (m *Model) musicSearch() {
	m.mu.tab = MusicSearch
	t := &m.mu.tabs[MusicSearch]
	t.sent, t.loading, t.err = true, true, ""
	m.mu.reqTab, m.mu.reqColl = MusicSearch, false
	m.musicSend(MusicSearch, nil)
}

func (m *Model) musicRetry() {
	if c := m.mu.coll; c != nil {
		if c.err == "" {
			return
		}
		c.loading, c.err = true, ""
		m.mu.reqTab, m.mu.reqColl = c.section, true
		m.musicSend(c.section, &c.parent)
		return
	}
	t := &m.mu.tabs[m.mu.tab]
	if t.err == "" {
		return
	}
	t.loading, t.err = true, ""
	m.mu.reqTab, m.mu.reqColl = m.mu.tab, false
	m.musicSend(m.mu.tab, nil)
}

// musicKey is the music screen's own key handling: it takes every key while it is up.
func (m *Model) musicKey(k, text string) bool {
	if m.musicStyle() != musicStyleFull {
		return m.musicListKey(k, text)
	}
	if m.mu.focus {
		t := &m.mu.tabs[MusicSearch]
		switch k {
		case "esc":
			m.mu.focus = false
		case "enter":
			m.mu.focus = false
			t.query = strings.TrimSpace(t.query)
			m.musicSearch()
		case "backspace":
			if r := []rune(t.query); len(r) > 0 {
				t.query = string(r[:len(r)-1])
			}
		default:
			t.query += text
		}
		return true
	}
	switch k {
	case "esc":
		if m.mu.coll != nil {
			m.mu.coll = nil
			return true
		}
		m.closeMusic()
	case "up", "k":
		m.musicMove(-1)
	case "down", "j":
		m.musicMove(1)
	case "pgup":
		m.musicMove(-m.musicPage())
	case "pgdown":
		m.musicMove(m.musicPage())
	case "home", "g":
		m.musicSelect(0)
	case "end", "G":
		m.musicSelect(len(m.musicViewState().items) - 1)
	case "left", "shift+tab":
		m.musicSwitch(m.mu.tab - 1)
	case "right", "tab":
		m.musicSwitch(m.mu.tab + 1)
	case "enter":
		m.musicEnter()
	case "a":
		m.musicAppend()
	case "/":
		m.mu.coll = nil
		m.mu.tab = MusicSearch
		m.mu.focus = true
	case "r":
		m.musicRetry()
	default:
		return false
	}
	return true
}

// musicStep runs the screen's own clocks: the search debounce, the palette slide and
// the palette the stage wears while the highlight moves.
func (m *Model) musicStep() {
	if m.mu.open && m.musicStyle() != musicStyleFull {
		l := &m.mu.list
		if m.mu.mode == mlModeMusic && l.query != "" && l.query != l.sentQ && m.t-l.typeT >= musicDebounce {
			m.mlSearch()
		}
		m.mlClampGrid()
		m.mlNoteDetail()
		m.mu.glide.step(m.dt)
	}
	if m.mu.shut && m.t-m.mu.list.shutT >= musicSlide {
		m.mu.shut = false
	}
	m.musicTheme()
}

// musicKindLabel is the group header of a search result and the breadcrumb root.
func musicKindLabel(k MusicKind) string {
	switch k {
	case MusicAlbum:
		return tr("альбомы", "albums")
	case MusicPlaylist:
		return tr("плейлисты", "playlists")
	}
	return tr("песни", "songs")
}

// musicTabLabel is the word on a section tab.
func musicTabLabel(s MusicSection) string {
	switch s {
	case MusicFavorites:
		return tr("избранное", "favorites")
	case MusicPlaylists:
		return tr("плейлисты", "playlists")
	case MusicAlbums:
		return tr("альбомы", "albums")
	case MusicSongs:
		return tr("песни", "songs")
	}
	return tr("поиск", "search")
}

// musicMeta is the right-aligned cell of a row: the length of a song, the size of a collection.
func musicMeta(it MusicItem) string {
	if it.Kind == MusicSong {
		if it.Dur > 0 {
			return fmtDur(it.Dur)
		}
		return ""
	}
	if it.Count > 0 {
		return fmt.Sprintf(tr("%d трк", "%d tr."), it.Count)
	}
	return ""
}
