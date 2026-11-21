package stage

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// musicRecentRows is how many of the launch's tracks the root offers.
const musicRecentRows = 5

// mlMode is what the palette talks to: the agent, or the music screen.
type mlMode int

const (
	mlModeMusic mlMode = iota
	mlModeAgent
)

// mlSort is the row order of a song tab: the host's rows newest first, or one of
// the orders the listener cycles with ctrl+s.
type mlSort int

const (
	mlSortFresh mlSort = iota
	mlSortTitle
	mlSortArtist
	mlSortFav
	mlSortCount
)

var mlSortLabels = [mlSortCount][2]string{
	{"свежие", "recent"},
	{"название", "title"},
	{"артист", "artist"},
	{"фавориты", "favorites"},
}

func (s mlSort) label() string { return tr(mlSortLabels[s][0], mlSortLabels[s][1]) }

// mlSortable is whether a section's rows can be re-ordered: the song tabs. The
// search, the grid tabs and a drilled collection keep their own order.
func mlSortable(sec MusicSection) bool { return sec == MusicSongs || sec == MusicFavorites }

// mlSorted lays rows in the given sort; fresh keeps the host's order, which the
// library fetch sorts newest first, and every other sort is stable over it.
func mlSorted(mode mlSort, items []MusicItem) []MusicItem {
	out := slices.Clone(items)
	switch mode {
	case mlSortTitle:
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	case mlSortArtist:
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Artist) < strings.ToLower(out[j].Artist) })
	case mlSortFav:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Fav && !out[j].Fav })
	}
	return out
}

// mlCycleSort turns the tab's sort over, keeping the highlight on its row.
func (m *Model) mlCycleSort() {
	if m.mu.list.root || m.mu.coll != nil || m.mu.list.query != "" || !mlSortable(m.mu.tab) {
		return
	}
	t := &m.mu.tabs[m.mu.tab]
	key := ""
	if m.musicStyle() == musicStyleFull {
		if t.sel >= 0 && t.sel < len(t.items) {
			key = t.items[t.sel].Key()
		}
	} else if l, ok := m.mlSelected(); ok {
		key = l.it.Key()
	}
	old := t.items
	t.sort = (t.sort + 1) % mlSortCount
	t.items = mlSorted(t.sort, t.fresh)
	m.mu.selT, m.mu.glideSnap = m.t, true
	if m.musicStyle() == musicStyleFull {
		t.sel = musicKeepSel(old, t.sel, t.items)
		return
	}
	m.mlReselect(key)
}

// musicList is the navigation of the palette and the drawer: the level, the query and
// the line under the highlight. A level is the root entries, a section of them, or the
// tracks of one collection, so the list never goes deeper than two pushes.
type musicList struct {
	root  bool
	query string
	sel   int
	qSel  int                // the highlight the query started from
	ret   []int              // the highlight to come back to when a level pops
	typeT float64            // the last edit of the query
	sentQ string             // the query of the last search sent
	more  map[MusicKind]bool // the search groups the user expanded
	openT float64
	shutT float64
}

// mlKind is what a line of the list is.
type mlKind int

const (
	mlGap    mlKind = iota // the air above a group header
	mlHead                 // a group header
	mlEntry                // a root entry: opens a section
	mlSong                 // an item of the current list
	mlTile                 // one album or playlist of the cover grid
	mlMore                 // the rest of a search group, hidden behind a cap
	mlAgent                // the agent fallback under the query
	mlStatus               // what the agent is doing, top of the agent mode
	mlSteer                // a quick word for the agent
	mlQuery                // the query sent whole to the agent
	mlBusy                 // a placeholder while the rows load
	mlEmpty                // the dim line of an empty list
	mlErr                  // the failed load
)

// mlLine is one drawn line of the list; h is how many cells it takes.
type mlLine struct {
	kind mlKind
	h    int
	text string
	sec  MusicSection
	it   MusicItem
	pos  int  // the index of the item in the current list
	hero bool // the top result of a search
}

func (l mlLine) height() int { return max(1, l.h) }

func mlSelectable(k mlKind) bool {
	switch k {
	case mlEntry, mlSong, mlTile, mlMore, mlAgent, mlStatus, mlSteer, mlQuery:
		return true
	}
	return false
}

// mlState is the list the palette shows now: the root entries, a section's rows, a
// collection's tracks, the results of the query, or the agent's quick words.
type mlState struct {
	root    bool
	agent   bool
	grid    bool
	query   string
	sec     MusicSection
	items   []MusicItem
	loading bool
	err     string
	grouped bool // the items carry group headers
	single  bool // a song plays alone instead of with the whole list
	coll    *musicColl
}

// mlState reads the list out of the model: the query wins over the level, so typing
// inside a drilled level searches everything and clearing goes back to the rows.
func (m *Model) mlState() mlState {
	if m.mu.mode == mlModeAgent {
		return mlState{agent: true, query: m.mu.list.query}
	}
	if q := m.mu.list.query; q != "" {
		t := &m.mu.tabs[MusicSearch]
		return mlState{query: q, sec: MusicSearch, items: t.items, loading: t.loading, err: t.err, grouped: true, single: true}
	}
	if c := m.mu.coll; c != nil {
		return mlState{sec: c.section, items: c.items, loading: c.loading, err: c.err, coll: c}
	}
	if m.mu.list.root {
		return mlState{root: true, sec: m.mu.tab, items: m.mlRecent(), single: true}
	}
	t := &m.mu.tabs[m.mu.tab]
	grid := m.musicStyle() == musicStylePalette && (m.mu.tab == MusicAlbums || m.mu.tab == MusicPlaylists)
	return mlState{sec: m.mu.tab, items: t.items, loading: t.loading, err: t.err, single: m.mu.tab == MusicSongs, grid: grid}
}

// mlRecent is the launch's tracks as rows of the root.
func (m *Model) mlRecent() []MusicItem {
	out := make([]MusicItem, 0, min(len(m.recent), musicRecentRows))
	for _, t := range m.recent {
		if len(out) == musicRecentRows {
			break
		}
		out = append(out, MusicItem{ID: t.ID, Kind: MusicSong, Title: t.Title, Artist: t.Artist, Dur: t.Dur})
	}
	return out
}

// mlSectionRow is one row of the root: a section and the glyph that opens it.
type mlSectionRow struct {
	glyph  string
	ru, en string
	sec    MusicSection
}

var mlEntries = [4]mlSectionRow{
	{"★", "избранное", "favorites", MusicFavorites},
	{"▤", "плейлисты", "playlists", MusicPlaylists},
	{"◉", "альбомы", "albums", MusicAlbums},
	{"♪", "песни", "songs", MusicSongs},
}

// mlSteers are the quick words of the agent mode.
var mlSteers = [4][2]string{
	{"поспокойнее", "calmer"},
	{"поэнергичнее", "more energy"},
	{"больше похожего на этот трек", "more like this track"},
	{"что-то незнакомое", "something unfamiliar"},
}

func (e mlSectionRow) label() string { return e.glyph + " " + tr(e.ru, e.en) }

// mlRowH is the height of a list row: two cells with a thumb, one in the narrow palette.
func (m *Model) mlRowH() int {
	if m.mlCompact() {
		return 1
	}
	return 2
}

// mlCompact is the narrow palette: no thumbs, single-line rows.
func (m *Model) mlCompact() bool {
	if m.musicStyle() != musicStylePalette {
		return false
	}
	return m.mlPaletteW() < 60
}

// mlLines lays the list out: group headers with the air above them, the entries, the
// rows of the level and, for a query, the agent fallback as the very last row.
func (m *Model) mlLines() []mlLine {
	c := m.mlState()
	switch {
	case c.agent:
		return m.mlAgentLines(c)
	case c.grid:
		return m.mlGridLines(c)
	}
	var out []mlLine
	head := func(s string) {
		if len(out) > 0 {
			out = append(out, mlLine{kind: mlGap, h: 1})
		}
		out = append(out, mlLine{kind: mlHead, h: 1, text: s})
	}
	if c.root {
		head(tr("музыка", "library"))
		for _, e := range mlEntries {
			out = append(out, mlLine{kind: mlEntry, h: m.mlRowH(), text: e.label(), sec: e.sec})
		}
		if len(c.items) > 0 {
			head(tr("недавние", "recent"))
			for i, it := range c.items {
				out = append(out, mlLine{kind: mlSong, h: m.mlRowH(), it: it, pos: i})
			}
		}
		return out
	}
	switch {
	case len(c.items) > 0 && c.grouped && c.query != "":
		out = m.mlSearchLines(c)
	case len(c.items) > 0:
		last := MusicKind(-1)
		for i, it := range c.items {
			if c.grouped && it.Kind != last {
				head(musicKindLabel(it.Kind))
				last = it.Kind
			}
			out = append(out, mlLine{kind: mlSong, h: m.mlRowH(), it: it, pos: i})
		}
	case c.err != "":
		out = append(out, mlLine{kind: mlErr, h: 1, text: c.err})
	case c.loading:
		for range 4 {
			out = append(out, mlLine{kind: mlBusy, h: 1})
		}
	default:
		out = append(out, mlLine{kind: mlEmpty, h: 1})
	}
	if c.query != "" {
		head(tr("агент", "agent"))
		out = append(out, mlLine{kind: mlAgent, h: 1})
	}
	return out
}

// mlHeroH is the height of the top result: the tall card, or the usual row when narrow.
func (m *Model) mlHeroH() int {
	if m.mlCompact() {
		return m.mlRowH()
	}
	return 4
}

// mlHeroIndex picks the top result: the first collection whose title starts with the
// query, then the first row that does, then the first row.
func mlHeroIndex(items []MusicItem, query string) int {
	if len(items) == 0 {
		return -1
	}
	q := strings.ToLower(strings.TrimSpace(query))
	for i, it := range items {
		if it.Kind != MusicSong && strings.HasPrefix(strings.ToLower(it.Title), q) {
			return i
		}
	}
	for i, it := range items {
		if strings.HasPrefix(strings.ToLower(it.Title), q) {
			return i
		}
	}
	return 0
}

// mlGroupCap is how many rows of a group a search shows before its show-more row.
func mlGroupCap(k MusicKind) int {
	if k == MusicSong {
		return 5
	}
	return 4
}

// mlSearchLines is the search: the top result card, then the groups of the rest in
// the host's order, each capped until the user asks for all of it.
func (m *Model) mlSearchLines(c mlState) []mlLine {
	hero := mlHeroIndex(c.items, c.query)
	out := []mlLine{
		{kind: mlHead, h: 1, text: tr("лучшее совпадение", "top result")},
		{kind: mlSong, h: m.mlHeroH(), it: c.items[hero], pos: hero, hero: true},
	}
	var kinds []MusicKind
	var seen [3]bool
	for _, it := range c.items {
		if !seen[it.Kind] {
			seen[it.Kind] = true
			kinds = append(kinds, it.Kind)
		}
	}
	for _, k := range kinds {
		var idx []int
		for i, it := range c.items {
			if i != hero && it.Kind == k {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		out = append(out, mlLine{kind: mlGap, h: 1}, mlLine{kind: mlHead, h: 1, text: musicKindLabel(k)})
		shown := idx
		if limit := mlGroupCap(k); len(idx) > limit && !m.mu.list.more[k] {
			shown = idx[:limit]
		}
		for _, i := range shown {
			out = append(out, mlLine{kind: mlSong, h: m.mlRowH(), it: c.items[i], pos: i})
		}
		if hidden := len(idx) - len(shown); hidden > 0 {
			out = append(out, mlLine{kind: mlMore, h: 1, it: MusicItem{Kind: k}, pos: hidden})
		}
	}
	return out
}

// mlAgentLines is the agent side of the palette: the agent's status and the quick
// words while the field is empty, the whole query as one row once there is one.
func (m *Model) mlAgentLines(c mlState) []mlLine {
	if c.query != "" {
		h := 1
		if !m.mlCompact() {
			h = 2
		}
		return []mlLine{{kind: mlQuery, h: h, text: c.query}}
	}
	out := []mlLine{{kind: mlStatus, h: 1, text: m.mlStatusText()}}
	out = append(out, mlLine{kind: mlHead, h: 1, text: tr("быстро", "quick")})
	for _, s := range mlSteers {
		out = append(out, mlLine{kind: mlSteer, h: 1, text: tr(s[0], s[1])})
	}
	return out
}

// mlGridLines is the cover grid: one line per tile, its index the item's index.
func (m *Model) mlGridLines(c mlState) []mlLine {
	switch {
	case len(c.items) > 0:
		out := make([]mlLine, 0, len(c.items))
		for i, it := range c.items {
			out = append(out, mlLine{kind: mlTile, h: mlTileRows, it: it, pos: i})
		}
		return out
	case c.err != "":
		return []mlLine{{kind: mlErr, h: 1, text: c.err}}
	case c.loading:
		out := make([]mlLine, 0, 4)
		for range 4 {
			out = append(out, mlLine{kind: mlBusy, h: 1})
		}
		return out
	}
	return []mlLine{{kind: mlEmpty, h: 1}}
}

// mlStatusText is what the agent is doing: the player's own line, or the quiet wait.
func (m *Model) mlStatusText() string {
	if s, ok := m.nextLineText(); ok {
		return s
	}
	return tr("агент ждёт", "the agent is listening")
}

// mlSettle puts the highlight on a selectable line: it stays where it is while that
// line still is one, otherwise it takes the nearest one after it, then the one before.
func (m *Model) mlSettle() {
	if m.musicStyle() == musicStyleFull {
		return
	}
	lines := m.mlLines()
	if len(lines) == 0 {
		m.mu.list.sel = 0
		return
	}
	s := max(0, min(m.mu.list.sel, len(lines)-1))
	if s == m.mu.list.sel && mlSelectable(lines[s].kind) {
		return
	}
	for i := s; i < len(lines); i++ {
		if mlSelectable(lines[i].kind) {
			m.mu.list.sel = i
			return
		}
	}
	for i := s - 1; i >= 0; i-- {
		if mlSelectable(lines[i].kind) {
			m.mu.list.sel = i
			return
		}
	}
	m.mu.list.sel = 0
}

// mlSelected is the line the highlight is on.
func (m *Model) mlSelected() (mlLine, bool) {
	lines := m.mlLines()
	if s := m.mu.list.sel; s >= 0 && s < len(lines) && mlSelectable(lines[s].kind) {
		return lines[s], true
	}
	return mlLine{}, false
}

// mlFirstSel is the first selectable line, the last one when step is negative.
func mlFirstSel(lines []mlLine, step int) int {
	if step >= 0 {
		for i := range lines {
			if mlSelectable(lines[i].kind) {
				return i
			}
		}
		return 0
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if mlSelectable(lines[i].kind) {
			return i
		}
	}
	return 0
}

// mlMove walks d selectable lines, never past either end.
func (m *Model) mlMove(d int) {
	if m.mlGridShown() {
		return
	}
	lines := m.mlLines()
	if len(lines) == 0 || d == 0 {
		return
	}
	step := 1
	if d < 0 {
		step, d = -1, -d
	}
	i := m.mu.list.sel
	if i < 0 || i >= len(lines) || !mlSelectable(lines[i].kind) {
		i = mlFirstSel(lines, step)
	}
	for range d {
		j := i + step
		for j >= 0 && j < len(lines) && !mlSelectable(lines[j].kind) {
			j += step
		}
		if j < 0 || j >= len(lines) {
			break
		}
		i = j
	}
	m.mu.list.sel = i
	m.mu.selT = m.t
}

// mlTypeQuery edits the query: the search goes out once the typing pauses. Clearing
// it puts the highlight back where the query started from.
func (m *Model) mlTypeQuery(q string) {
	l := &m.mu.list
	if l.query == q {
		return
	}
	switch {
	case l.query == "":
		l.qSel = l.sel
	case q == "":
		l.sel = l.qSel
	}
	l.query, l.typeT, l.more = q, m.t, nil
	m.mlSettle()
}

// mlToggleMode switches the palette between the agent and the music, keeping the line.
func (m *Model) mlToggleMode() {
	if m.musicStyle() != musicStylePalette {
		return
	}
	if m.mu.mode == mlModeAgent {
		m.mu.mode = mlModeMusic
	} else {
		m.mu.mode = mlModeAgent
	}
	m.mu.menu.open = false
	m.mu.glideSnap, m.mu.cascT = true, m.t
	m.mlSettle()
}

// mlAfterRows settles the highlight when a load lands: the first music row when the
// list had none, otherwise where the user left it while it stays in range.
func (m *Model) mlAfterRows(had bool) {
	if !had && len(m.mlState().items) > 0 {
		m.mu.list.sel = mlFirstSel(m.mlLines(), 1)
		return
	}
	m.mlSettle()
}

// mlBack is esc: the query clears, then a level pops, then the screen closes.
func (m *Model) mlBack() {
	m.mu.glideSnap, m.mu.cascT = true, m.t
	switch {
	case m.mu.list.query != "":
		m.mlTypeQuery("")
	case m.mu.coll != nil:
		m.mu.coll = nil
		m.mu.list.sel, m.mu.gridTop = m.mlPop(), 0
		m.mlSettle()
	case !m.mu.list.root:
		m.mu.list.root = true
		m.mu.list.sel, m.mu.gridTop = m.mlPop(), 0
		m.mlSettle()
	default:
		m.closeMusic()
	}
}

func (m *Model) mlPush(sel int) { m.mu.list.ret = append(m.mu.list.ret, sel) }

func (m *Model) mlPop() int {
	if n := len(m.mu.list.ret); n > 0 {
		s := m.mu.list.ret[n-1]
		m.mu.list.ret = m.mu.list.ret[:n-1]
		return s
	}
	return 0
}

// mlPushSection opens a section of the root.
func (m *Model) mlPushSection(sec MusicSection) {
	m.mlPush(m.mu.list.sel)
	m.mu.list.root, m.mu.list.query, m.mu.list.sentQ = false, "", ""
	m.mu.list.sel, m.mu.list.more, m.mu.gridTop = 0, nil, 0
	m.mu.coll = nil
	m.mu.tab = sec
	m.mu.glideSnap, m.mu.cascT = true, m.t
	m.musicLoadTab(sec)
	m.mlSettle()
}

// mlPushColl opens the tracks of an album or a playlist.
func (m *Model) mlPushColl(it MusicItem) {
	m.mlPush(m.mu.list.sel)
	parent := it
	c := &musicColl{parent: parent, section: m.mu.tab}
	if items, ok := m.mu.peek[parent.Key()]; ok {
		c.items = slices.Clone(items)
	} else {
		c.loading = true
	}
	m.mu.coll = c
	m.mu.reqTab, m.mu.reqColl = m.mu.tab, true
	m.musicSend(m.mu.tab, &parent)
	m.mu.list.sel, m.mu.gridTop = 0, 0
	m.mu.glideSnap, m.mu.cascT = true, m.t
	m.mlSettle()
}

// mlSearch asks the host for the results of the query.
func (m *Model) mlSearch() {
	q := m.mu.list.query
	if q == "" {
		return
	}
	m.mu.list.sentQ = q
	m.mu.gen++
	m.mu.reqTab, m.mu.reqColl = MusicSearch, false
	t := &m.mu.tabs[MusicSearch]
	t.sent, t.loading, t.err, t.query = true, true, "", q
	m.mlSettle()
	m.send(MusicLoad{Gen: m.mu.gen, Section: MusicSearch, Query: q})
}

// mlRetry asks again for the rows that failed, and asks the host to check the rows
// that loaded.
func (m *Model) mlRetry() {
	c := m.mlState()
	switch {
	case c.query != "":
		if t := &m.mu.tabs[MusicSearch]; t.err != "" || !t.sent {
			m.mlSearch()
		}
	case c.coll != nil:
		col := c.coll
		switch {
		case col.err != "":
			col.loading, col.err = true, ""
			m.mu.reqTab, m.mu.reqColl = col.section, true
			m.musicSend(col.section, &col.parent)
		case !col.loading:
			p := col.parent
			m.send(MusicRefresh{Section: col.section, Parent: &p})
		}
	case !c.root && c.err != "":
		m.musicLoadTab(m.mu.tab)
	case c.root:
		m.send(MusicRefresh{All: true})
	case !c.loading && m.mu.tabs[m.mu.tab].sent:
		m.send(MusicRefresh{Section: m.mu.tab})
	}
}

// mlReselect puts the highlight back on the row keyed key after the rows changed
// under it, or settles it when the row left.
func (m *Model) mlReselect(key string) {
	if key != "" {
		for i, l := range m.mlLines() {
			if mlSelectable(l.kind) && l.it.Key() == key {
				m.mu.list.sel = i
				return
			}
		}
	}
	m.mlSettle()
}

// mlEnter opens the row under the highlight: a section, a collection, a track, the
// agent, or one of its quick words.
func (m *Model) mlEnter() {
	l, ok := m.mlSelected()
	if !ok {
		return
	}
	switch l.kind {
	case mlEntry:
		m.mlPushSection(l.sec)
	case mlStatus:
		return
	case mlSteer:
		m.mlAgentSend(l.text)
	case mlQuery:
		m.mlAsk()
	case mlMore:
		if m.mu.list.more == nil {
			m.mu.list.more = map[MusicKind]bool{}
		}
		m.mu.list.more[l.it.Kind] = true
	case mlAgent:
		m.mlAsk()
	case mlSong, mlTile:
		if l.it.Unplayable != "" {
			m.MusicNotice(l.it.Unplayable)
			return
		}
		if l.it.Kind != MusicSong {
			m.mlPushColl(l.it)
			return
		}
		m.mlPlay(l)
	}
}

// mlPlay sends the row to the host: a song of a collection plays with it, a song of a
// list of them plays alone.
func (m *Model) mlPlay(l mlLine) {
	c := m.mlState()
	if c.single {
		m.send(MusicPlay{Items: []MusicItem{l.it}, Start: 0})
	} else {
		from := &MusicItem{Kind: MusicPlaylist, Title: tr("избранное", "favorites")}
		if c.coll != nil {
			from = &c.coll.parent
		}
		items, start := musicSongs(c.items, l.pos)
		m.send(MusicPlay{Items: items, Start: start, From: from})
	}
	m.mu.playKey, m.mu.playT = l.it.Key(), m.t
}

// mlAppend puts the row under the highlight at the end of the queue: one song,
// or the whole collection, which the host expands.
func (m *Model) mlAppend() {
	l, ok := m.mlSelected()
	if !ok || (l.kind != mlSong && l.kind != mlTile) {
		return
	}
	if l.it.Unplayable != "" {
		m.MusicNotice(l.it.Unplayable)
		return
	}
	m.send(MusicAppend{Item: l.it})
}

// mlAsk hands the query to the agent: over the intro the palette closes and the
// screen's own launch takes the line, in the player it is the confirmation and Say.
func (m *Model) mlAsk() { m.mlAgentSend(m.mu.list.query) }

// mlAgentSend sends one line to the agent and closes the palette.
func (m *Model) mlAgentSend(line string) {
	q := strings.TrimSpace(line)
	if q == "" {
		return
	}
	intro := m.ph == phIntro
	m.closeMusic()
	if m.o.NoAgent {
		m.OnboardAgent()
		return
	}
	if intro {
		m.input = []rune(q)
		m.launch()
		return
	}
	m.say(tr("понял: «"+q+"», перестраиваю дальше", "got it: \""+q+"\", rebuilding the queue"))
	m.send(Say{Text: q})
}

// mlAgentText is the fallback row of the music mode: tab then enter asks the agent.
func (m *Model) mlAgentText() string {
	return "tab ✦ " + tr("спросить агента", "ask the agent") + " «" + m.mu.list.query + "»"
}

// mlCrumb is the breadcrumb of the level, empty at the root and under a query.
// A song tab names the sort it is in.
func (m *Model) mlCrumb() string {
	if m.mu.list.query != "" || m.mu.list.root {
		return ""
	}
	root := musicTabLabel(m.mu.tab)
	if c := m.mu.coll; c != nil {
		return root + " › " + c.parent.Title
	}
	if mlSortable(m.mu.tab) {
		return root + " · " + m.mu.tabs[m.mu.tab].sort.label()
	}
	return root
}

// mlProgress is how far the palette is in or out: the slide and the fade share it.
func (m *Model) mlProgress() float64 {
	if m.mu.open {
		return easeOut(prog(m.t-m.mu.list.openT, 0, musicSlide))
	}
	if m.mu.shut {
		return 1 - easeOut(prog(m.t-m.mu.list.shutT, 0, musicSlide))
	}
	return 0
}

// mlRowCap is how many rows the look shows at most.
func (m *Model) mlRowCap() int {
	if m.musicStyle() == musicStyleDrawer {
		return max(1, m.h-6)
	}
	_, _, _, h := m.mlPaletteGeom()
	return max(1, (h-6)/2)
}

func (m *Model) mlPage() int { return max(1, m.mlRowCap()) }

// musicListKey is the palette and the drawer: every printable key is query text, the
// named keys walk the list and the levels.
func (m *Model) musicListKey(k, text string) bool {
	palette := m.musicStyle() == musicStylePalette
	if m.mu.menu.open {
		if m.mlMenuKey(k) {
			return true
		}
		m.mu.menu.open = false
	}
	if m.mu.mode == mlModeAgent {
		return m.mlAgentKey(k, text)
	}
	if palette && m.mlGridShown() {
		switch k {
		case "left", "right", "up", "down", "home", "end", "pgup", "pgdown":
			return m.mlGridKey(k)
		}
	}
	switch k {
	case "ctrl+o":
		m.closeMusic()
	case "esc":
		m.mlBack()
	case "up":
		m.mlMove(-1)
	case "down":
		m.mlMove(1)
	case "pgup":
		m.mlMove(-m.mlPage())
	case "pgdown":
		m.mlMove(m.mlPage())
	case "home":
		m.mu.list.sel = mlFirstSel(m.mlLines(), 1)
	case "end":
		m.mu.list.sel = mlFirstSel(m.mlLines(), -1)
	case "enter":
		m.mlEnter()
	case "/":
		if palette && m.mu.list.query == "" {
			m.mlToggleMode()
			break
		}
		m.mlTypeQuery(m.mu.list.query + "/")
	case "tab":
		if palette {
			m.mlToggleMode()
		}
	case "ctrl+k":
		if palette {
			m.mlMenuOpenAt()
		}
	case "ctrl+e":
		m.mlAppend()
	case "ctrl+n":
		m.mlToggleFollow()
	case "ctrl+s":
		m.mlCycleSort()
	case "right":
		if !palette {
			return false
		}
		if m.mlMenuCanRight() {
			m.mlMenuOpenAt()
		}
	case "left":
		if !palette {
			return false
		}
	case "backspace":
		if q := m.mu.list.query; q != "" {
			r := []rune(q)
			m.mlTypeQuery(string(r[:len(r)-1]))
			break
		}
		m.mlBack()
	case "ctrl+r":
		m.mlRetry()
	case "space":
		if m.mu.list.query == "" && m.ph == phPlay {
			return false
		}
		m.mlTypeQuery(m.mu.list.query + " ")
	default:
		if text == "" {
			return false
		}
		m.mlTypeQuery(m.mu.list.query + text)
	}
	return true
}

// mlAgentKey is the agent mode: the whole field is its line, tab goes back to the
// music, enter sends the query or the quick word, esc closes.
func (m *Model) mlAgentKey(k, text string) bool {
	switch k {
	case "esc":
		m.closeMusic()
	case "tab":
		m.mlToggleMode()
	case "up":
		m.mlMove(-1)
	case "down":
		m.mlMove(1)
	case "pgup":
		m.mlMove(-m.mlPage())
	case "pgdown":
		m.mlMove(m.mlPage())
	case "home":
		m.mu.list.sel = mlFirstSel(m.mlLines(), 1)
	case "end":
		m.mu.list.sel = mlFirstSel(m.mlLines(), -1)
	case "enter":
		m.mlEnter()
	case "backspace":
		if q := m.mu.list.query; q != "" {
			r := []rune(q)
			m.mlTypeQuery(string(r[:len(r)-1]))
			break
		}
		m.closeMusic()
	case "ctrl+r":
		m.mlRetry()
	case "left", "right", "ctrl+k":
	case "space":
		m.mlTypeQuery(m.mu.list.query + " ")
	default:
		if text == "" {
			return true
		}
		m.mlTypeQuery(m.mu.list.query + text)
	}
	return true
}

// mlAccessory is the right cell of an item row.
func mlAccessory(it MusicItem) string {
	switch it.Kind {
	case MusicSong:
		if it.Dur > 0 {
			return fmtDur(it.Dur)
		}
	case MusicAlbum:
		return mlCollectionMeta(tr("альбом", "album"), it.Count)
	case MusicPlaylist:
		return mlCollectionMeta(tr("плейлист", "playlist"), it.Count)
	}
	return ""
}

func mlCollectionMeta(what string, n int) string {
	if n > 0 {
		return fmt.Sprintf("%s · %d", what, n)
	}
	return what
}

// mlEntryMeta is the right cell of a root entry: the rows it holds once they are known.
func (m *Model) mlEntryMeta(sec MusicSection) string {
	t := &m.mu.tabs[sec]
	if !t.sent || t.loading || t.err != "" {
		return "›"
	}
	return strconv.Itoa(len(t.items))
}

// mlEntryNote is the second line of a root entry: how many rows it holds, or what it is.
func (m *Model) mlEntryNote(sec MusicSection) string {
	if t := &m.mu.tabs[sec]; t.sent && !t.loading && t.err == "" {
		switch sec {
		case MusicPlaylists:
			return countOf(len(t.items), playlistForms, playlistFormsEn)
		case MusicAlbums:
			return countOf(len(t.items), albumForms, albumFormsEn)
		}
		return countOf(len(t.items), trackForms, trackFormsEn)
	}
	switch sec {
	case MusicFavorites:
		return tr("любимые треки", "your loved tracks")
	case MusicPlaylists:
		return tr("твои плейлисты", "your playlists")
	case MusicAlbums:
		return tr("альбомы в библиотеке", "albums in your library")
	}
	return tr("все песни библиотеки", "every song in your library")
}

// mlRowBox is one line of the body and where it lands, in panel rows.
type mlRowBox struct {
	i, y, h int
}

// mlDrawBody draws the lines into the w×h area at (x, y), the highlight in sight and
// the agent fallback pinned under the rows that scroll.
func (m *Model) mlDrawBody(f *fb, x, y, w, h int, lines []mlLine, sel int) {
	if w < 8 || h < 1 {
		return
	}
	if m.mlGridShown() {
		m.mlDrawGrid(f, x, y, w, h, lines, sel)
		return
	}
	body, tail := mlAgentTail(lines)
	tailH := mlLinesH(tail)
	rowsH := max(0, h-tailH)
	win := mlWindowH(body, sel, rowsH)
	boxes := mlRowBoxes(body, tail, win, y, rowsH, h)
	palette := m.musicStyle() == musicStylePalette
	if palette {
		m.mlDrawGlide(f, x, y, w, h, lines, boxes, sel)
	}
	for i, b := range boxes {
		m.mlDrawLine(f, x, b.y, w, lines[b.i], b.i == sel)
		if b.i == sel {
			m.mu.rowY, m.mu.rowX = b.y, x+w
			m.mu.rowL, m.mu.rowH = x, b.h
		}
		if palette {
			m.mlCascade(f, x, b.y, w, b.h, i)
		}
	}
}

// mlRowBoxes lays the lines out: the scrolling ones from the window, then the pinned tail.
func mlRowBoxes(body, tail []mlLine, win, y, rowsH, h int) []mlRowBox {
	var out []mlRowBox
	yy := y
	for i := win; i < len(body); i++ {
		lh := body[i].height()
		if yy+lh > y+rowsH {
			break
		}
		out = append(out, mlRowBox{i, yy, lh})
		yy += lh
	}
	ty := y + h - mlLinesH(tail)
	for i, l := range tail {
		if ty >= y && ty < y+h {
			out = append(out, mlRowBox{len(body) + i, ty, l.height()})
		}
		ty += l.height()
	}
	return out
}

// mlCascade fades one line of the rows in, the deeper lines a beat after the first.
func (m *Model) mlCascade(f *fb, x, y, w, h, i int) {
	a := easeOut(prog(m.t-m.mu.cascT-0.025*float64(i), 0, 0.2))
	if a >= 1 {
		return
	}
	fadeRect(f, x, y, w, h, m.th.bg.lerp(m.th.text, 0.07), 1-a)
}

// mlDrawGlide walks the selection tint to the selected row and leaves it there; the
// tint is drawn under the lines so the cascade takes it with them.
func (m *Model) mlDrawGlide(f *fb, x, y, w, h int, lines []mlLine, boxes []mlRowBox, sel int) {
	at, rowH, bad, ok := 0, 0, false, false
	for _, b := range boxes {
		if b.i == sel && mlSelectable(lines[b.i].kind) {
			at, rowH, bad, ok = b.y, b.h, mlLineBad(lines[b.i]), true
			break
		}
	}
	if !ok {
		m.mu.glide.x, m.mu.glide.v, m.mu.glideSnap = m.mu.glide.to, 0, false
		return
	}
	if m.mu.glideSnap {
		m.mu.glide.x, m.mu.glide.v, m.mu.glideSnap = float64(at), 0, false
	}
	m.mu.glide.to = float64(at)
	bar := m.th.a1
	if bad {
		bar = m.th.dim
	}
	yy := int(math.Round(m.mu.glide.x))
	for j := range rowH {
		row := yy + j
		if row < y || row >= y+h {
			continue
		}
		for xx := range w {
			a := 0.22 - (0.22-0.05)*float64(xx)/float64(max(1, w-1))
			f.tint(x+xx, row, m.th.a1, a)
		}
		f.put(x, row, '▌', bar)
	}
}

// mlLineBad reports whether the row is one that cannot play.
func mlLineBad(l mlLine) bool {
	return (l.kind == mlSong || l.kind == mlTile) && l.it.Unplayable != ""
}

// mlAgentTail splits the list into the rows that scroll and the agent fallback,
// which keeps its own group at the bottom of the area.
func mlAgentTail(lines []mlLine) (body, tail []mlLine) {
	for i, l := range lines {
		if l.kind != mlAgent {
			continue
		}
		at := i
		for at > 0 && (lines[at-1].kind == mlHead || lines[at-1].kind == mlGap) {
			at--
		}
		return lines[:at], lines[at:]
	}
	return lines, nil
}

func mlLinesH(lines []mlLine) int {
	n := 0
	for _, l := range lines {
		n += l.height()
	}
	return n
}

// mlWindowH is the first line drawn so the selected one stays in sight; the end of
// the list is what shows while the highlight sits on the pinned tail.
func mlWindowH(lines []mlLine, sel, vis int) int {
	if vis <= 0 || len(lines) == 0 {
		return 0
	}
	if sel >= len(lines) {
		start, used := len(lines), 0
		for start > 0 {
			h := lines[start-1].height()
			if used+h > vis {
				break
			}
			used += h
			start--
		}
		return start
	}
	start, used := 0, 0
	for i := 0; i <= sel; i++ {
		used += lines[i].height()
		for used > vis && start < i {
			used -= lines[start].height()
			start++
		}
	}
	return start
}

func (m *Model) mlDrawLine(f *fb, x, y, w int, l mlLine, sel bool) {
	switch l.kind {
	case mlGap:
	case mlHead:
		f.textS(x+2, y, trunc(l.text, max(0, w-2)), m.th.dim.mul(0.9), true, false)
	case mlEntry:
		m.mlRowTint(f, x, y, w, l.height(), sel, false)
		if l.height() > 1 {
			f.textS(x+2, y, trunc(l.text, max(0, w-4)), m.th.text, sel, false)
			f.put(x+w-2, y, '›', m.th.dim)
			f.text(x+2, y+1, trunc(m.mlEntryNote(l.sec), max(0, w-4)), m.th.dim)
			break
		}
		acc := m.mlEntryMeta(l.sec)
		room := w - 2
		if w > len([]rune(acc))+6 {
			room = w - len([]rune(acc)) - 3
			f.text(x+w-len([]rune(acc)), y, acc, m.th.dim)
		}
		f.text(x+2, y, trunc(l.text, max(0, room)), m.th.text)
	case mlSong:
		m.mlDrawItem(f, x, y, w, l, sel)
	case mlTile:
		m.mlDrawItem(f, x, y, w, l, sel)
	case mlMore:
		m.mlRowTint(f, x, y, w, 1, sel, false)
		f.text(x+2, y, trunc(fmt.Sprintf(tr("ещё %d ›", "%d more ›"), l.pos), w-2), m.th.dim)
	case mlAgent:
		m.mlRowTint(f, x, y, w, 1, sel, false)
		if w > 8 {
			f.text(x+2, y, trunc(m.mlAgentText(), w-4), m.th.text)
			f.put(x+w-1, y, '↵', m.th.dim)
		}
	case mlStatus:
		m.mlRowTint(f, x, y, w, 1, sel, false)
		f.put(x+2, y, '✧', m.th.a1)
		f.text(x+4, y, trunc(l.text, w-6), m.th.text)
	case mlSteer:
		m.mlRowTint(f, x, y, w, 1, sel, false)
		f.put(x+2, y, '›', m.th.a2)
		f.text(x+4, y, trunc(l.text, w-7), m.th.text)
		if w > 8 {
			f.put(x+w-1, y, '↵', m.th.dim)
		}
	case mlQuery:
		m.mlRowTint(f, x, y, w, l.height(), sel, false)
		f.textS(x+2, y, trunc("✦ "+l.text, w-4), m.th.a1, true, false)
		if w > 8 {
			f.put(x+w-1, y, '↵', m.th.dim)
		}
		if l.height() > 1 {
			f.text(x+4, y+1, trunc(tr("отправить агенту", "send to the agent"), w-6), m.th.dim)
		}
	case mlBusy:
		b := 0.18 + 0.14*(0.5+0.5*math.Sin(m.t*3-float64(y)*0.8))
		f.text(x+2, y, "· · · ·", m.th.dim.mul(b))
		if w > 20 {
			f.text(x+12, y, strings.Repeat("·", min(w-14, 18)), m.th.dim.mul(b*0.6))
		}
	case mlEmpty:
		msg := tr("пусто", "nothing here")
		if m.mu.list.query != "" {
			msg = tr("ничего не нашлось", "no matches")
		}
		f.text(x+max(0, (w-len([]rune(msg)))/2), y, msg, m.th.dim)
	case mlErr:
		hint := "ctrl+r " + tr("повторить", "retry")
		room := w - 2 - len([]rune(hint)) - 2
		f.text(x+2, y, trunc(l.text, max(0, room)), m.th.bad)
		if w > len([]rune(hint))+6 {
			f.text(x+w-len([]rune(hint)), y, hint, m.th.dim)
		}
	}
}

// mlRowTint is the selected row of the drawer: the accent bar on every line and a
// tint that fades from the left edge to the right; the palette walks its own marker.
func (m *Model) mlRowTint(f *fb, x, y, w, h int, sel, bad bool) {
	if !sel || m.musicStyle() == musicStylePalette {
		return
	}
	for yy := y; yy < y+h; yy++ {
		for xx := range w {
			a := 0.22 - (0.22-0.05)*float64(xx)/float64(max(1, w-1))
			f.tint(x+xx, yy, m.th.a1, a)
		}
	}
	c := m.th.a1
	if bad {
		c = m.th.dim
	}
	for yy := y; yy < y+h; yy++ {
		f.put(x, yy, '▌', c)
	}
}

func (m *Model) mlDrawItem(f *fb, x, y, w int, l mlLine, sel bool) {
	if l.hero && !m.mlCompact() {
		m.mlDrawHero(f, x, y, w, l, sel)
		return
	}
	it := l.it
	h := m.mlRowH()
	bad := it.Unplayable != ""
	cur := m.musicCurrent(it)
	num := m.mlNumbered(l)
	m.mlRowTint(f, x, y, w, h, sel, bad)
	if !sel && cur && !num {
		f.put(x, y, '♪', m.th.a1)
	}
	tx := x + 2
	if h > 1 && w >= 24 {
		switch {
		case num && cur:
			m.mlBars(f, x+3, y)
		case num:
			n := fmt.Sprintf("%02d", l.pos+1)
			f.text(x+6-len([]rune(n)), y, n, m.th.dim)
		default:
			g := m.cover(it.Key(), 4, 4)
			for py := range 4 {
				for px := range 4 {
					f.pixel(x+2+px, y*2+py, g.at(px, py))
				}
			}
		}
		tx = x + 7
	}
	fg, dfg := m.th.text, m.th.dim
	if bad {
		fg, dfg = m.th.dim.mul(0.9), m.th.dim.mul(0.55)
	} else if cur && (sel || num) {
		fg = m.th.a1
	}
	title := it.Title
	if it.Kind != MusicSong {
		title += " ›"
	}
	acc := mlAccessory(it)
	if bad {
		acc = "✕"
	}
	room := x + w - tx
	if room < 1 {
		return
	}
	mark, mc, egg := m.mlEggMark(it)
	egg = egg && room > 4
	tw := room
	if egg {
		tw -= 2
	}
	n := f.textS(tx, y, trunc(title, tw), fg, sel && !bad, false)
	if egg {
		f.put(tx+n+1, y, mark, mc)
		n += 2
	}
	if h > 1 {
		line := it.Artist
		if acc != "" {
			if line != "" {
				line += " · "
			}
			line += acc
		}
		if line != "" {
			f.text(tx, y+1, trunc(line, x+w-tx), dfg)
		}
		return
	}
	if a := room - n - 2; a > 4 {
		f.text(tx+n+2, y, trunc(it.Artist, a), dfg)
	}
	if acc != "" {
		c := m.th.dim
		if bad {
			c = m.th.bad
		}
		f.text(x+w-len([]rune(acc)), y, acc, c)
	}
}

// mlDrawHero is the top result: the eight-pixel cover, the title, the artist and what
// the row is, in the tall card of a search.
func (m *Model) mlDrawHero(f *fb, x, y, w int, l mlLine, sel bool) {
	it := l.it
	bad := it.Unplayable != ""
	cur := m.musicCurrent(it)
	m.mlRowTint(f, x, y, w, l.height(), sel, bad)
	if !sel && cur {
		f.put(x, y, '♪', m.th.a1)
	}
	g := m.cover(it.Key(), 8, 8)
	for py := range 8 {
		for px := range 8 {
			f.pixel(x+2+px, y*2+py, g.at(px, py))
		}
	}
	tx := x + 12
	room := x + w - tx
	if room < 1 {
		return
	}
	title := it.Title
	if it.Kind != MusicSong {
		title += " ›"
	}
	fg, dfg := m.th.text, m.th.dim
	if bad {
		fg, dfg = m.th.dim.mul(0.9), m.th.dim.mul(0.55)
	} else if cur && sel {
		fg = m.th.a1
	}
	f.textS(tx, y, trunc(title, room), fg, true, false)
	if it.Artist != "" {
		f.text(tx, y+1, trunc(it.Artist, room), dfg)
	}
	sub := it.Sub
	if it.Kind == MusicSong {
		sub = tr("песня", "song") + " · " + fmtDur(it.Dur)
	} else if sub == "" {
		sub = musicKindLabel(it.Kind)
	}
	if sub != "" {
		f.text(tx, y+2, trunc(sub, room), dfg)
	}
}

// mlNumbered reports whether the row is a track of an opened collection: it takes a
// number where the cover would stand.
func (m *Model) mlNumbered(l mlLine) bool {
	return l.kind == mlSong && l.it.Kind == MusicSong && m.mu.list.query == "" && m.mu.coll != nil
}

// mlBars are the three cells of the now-playing equalizer, still while paused.
func (m *Model) mlBars(f *fb, x, y int) {
	const ramp = "▁▂▃▄▅▆▇█"
	for i := range 3 {
		lvl := []int{1, 3, 1}[i]
		if m.playing {
			lvl = int(3.5 + 3.5*math.Sin(m.t*(5+1.7*float64(i))+2*float64(i)))
		}
		f.put(x+i, y, []rune(ramp)[lvl], m.th.a1)
	}
}

// mlModeChip is the word on the mode chip of the input row.
func (m *Model) mlModeChip() string {
	if m.mu.mode == mlModeAgent {
		return "✦ " + tr("агент", "agent")
	}
	return "♪ " + tr("музыка", "music")
}

// mlDrawInput is the query row of both looks: the mode chip, the caret, the placeholder.
func (m *Model) mlDrawInput(f *fb, x, y, w int) {
	if w < 10 {
		return
	}
	chip := m.mlModeChip()
	if cw := len([]rune(chip)) + 2; cw > w-8 {
		chip = trunc(chip, max(0, w-10))
	}
	cw := len([]rune(chip)) + 2
	for i := range cw {
		if c := f.at(x+i, y); c != nil {
			c.bg, c.ch, c.pix, c.bold, c.ital = c.bg.lerp(m.th.a1, 0.18), ' ', false, false, false
		}
	}
	f.textS(x+1, y, chip, m.th.a1, true, false)
	qx := x + cw + 1
	if qx >= x+w-2 {
		return
	}
	f.put(qx, y, '›', m.th.a1)
	crumb := ""
	if m.mu.mode == mlModeMusic {
		crumb = m.mlCrumb()
	}
	crumbW := 0
	if crumb != "" {
		crumb = trunc(crumb, w/3)
		crumbW = len([]rune(crumb)) + 2
	}
	room := x + w - crumbW - qx - 2
	if room < 2 {
		return
	}
	q := []rune(m.mu.list.query)
	shown := q
	if over := len(shown) - (room - 1); over > 0 {
		shown = shown[over:]
	}
	if len(q) == 0 {
		placeholder := tr("найти трек, альбом, плейлист…", "search tracks, albums, playlists…")
		if m.mu.mode == mlModeAgent {
			placeholder = tr("скажи агенту, куда повернуть…", "tell the agent where to take it…")
			if m.o.NoAgent {
				placeholder = tr("агент не подключён — enter откроет настройку", "agent not connected — enter sets it up")
			}
		}
		f.fade(qx+2, y, "▎", m.th.a1, 0.35+0.65*(0.5+0.5*math.Sin(m.t*6)), false)
		f.text(qx+3, y, trunc(placeholder, room-2), m.th.dim)
	} else {
		f.text(qx+2, y, string(shown), m.th.text)
		if cx := qx + 2 + len(shown); cx < x+w-crumbW-1 {
			f.fade(cx, y, "▎", m.th.a1, 0.35+0.65*(0.5+0.5*math.Sin(m.t*6)), false)
		}
	}
	if crumb != "" {
		f.text(x+w-len([]rune(crumb)), y, crumb, m.th.dim)
	}
}

// mlDrawBar is the bottom row of both looks: the now-playing line on the left, the
// keycaps of the mode and the selected row on the right.
func (m *Model) mlDrawBar(f *fb, x, y, w int) {
	if w < 8 {
		return
	}
	caps := m.mlBarCaps()
	const barText = 16 // the room the now-playing line keeps before a cap goes
	if m.musicNoticeVisible() && mlCapsWidth(caps) > w-len([]rune(m.mu.notice))-2 {
		caps = mlDropCap(caps, "ctrl+n")
	}
	for _, drop := range []string{"ctrl+n", "tab", "esc", "ctrl+k", "ctrl+e"} {
		if mlCapsWidth(caps) <= w-barText {
			break
		}
		caps = mlDropCap(caps, drop)
	}
	if len(caps) > 1 && mlCapsWidth(caps) > w-barText {
		caps = caps[:1]
	}
	cw := mlCapsWidth(caps)
	if room := w - cw - 2; room > 0 {
		f.text(x, y, trunc(m.mlBarText(), room), m.th.dim)
	}
	cx := x + w - cw
	for i, c := range caps {
		if i > 0 {
			cx += 2
		}
		cx += m.mlCap(f, cx, y, c[0], c[1])
	}
}

func mlCapWidth(key, label string) int {
	return len([]rune(key)) + 3 + len([]rune(label))
}

func mlCapsWidth(caps [][2]string) int {
	n := 0
	for i, c := range caps {
		if i > 0 {
			n += 2
		}
		n += mlCapWidth(c[0], c[1])
	}
	return n
}

// mlDropCap takes a keycap out of the bar, keeping the order of the rest.
func mlDropCap(caps [][2]string, key string) [][2]string {
	out := caps[:0]
	for _, c := range caps {
		if c[0] != key {
			out = append(out, c)
		}
	}
	return out
}

// mlCap draws " key " on the cap background and the label after one plain space.
func (m *Model) mlCap(f *fb, x, y int, key, label string) int {
	bg := m.th.a1.mul(0.35)
	for i, r := range []rune(" " + key + " ") {
		if c := f.at(x+i, y); c != nil {
			c.ch, c.fg, c.bg, c.pix, c.bold, c.ital = r, m.th.text, bg, false, false, false
		}
	}
	n := len([]rune(key)) + 2
	f.text(x+n+1, y, label, m.th.dim)
	return n + 1 + len([]rune(label))
}

// mlBarText is the left of the bar: why the row cannot play, or what plays now.
func (m *Model) mlBarText() string {
	if l, ok := m.mlSelected(); ok && l.kind == mlSong && l.it.Unplayable != "" {
		return "✕ " + l.it.Unplayable
	}
	if m.musicNoticeVisible() {
		return m.mu.notice
	}
	if m.mu.refreshing {
		return "↻ " + tr("обновляю…", "refreshing…")
	}
	if m.ph != phPlay || len(m.queue) == 0 {
		return tr("тишина", "silence")
	}
	tk := m.curTrack()
	return "♪ " + tk.Title + " — " + tk.Artist + " " + fmtDur(m.pos)
}

// mlBarCaps are the keys the bar offers for the mode and the selected row.
func (m *Model) mlBarCaps() [][2]string {
	if m.mu.mode == mlModeAgent {
		if m.o.NoAgent {
			return [][2]string{
				{"↵", tr("подключить агента", "connect the agent")},
				{"tab", tr("музыка", "music")},
				{"esc", tr("закрыть", "close")},
			}
		}
		return [][2]string{
			{"↵", tr("отправить", "send")},
			{"tab", tr("музыка", "music")},
			{"esc", tr("закрыть", "close")},
		}
	}
	var caps [][2]string
	l, ok := m.mlSelected()
	switch {
	case !ok:
	case l.kind == mlAgent:
		caps = append(caps, [2]string{"↵", tr("спросить", "ask")})
	case l.kind == mlMore:
		caps = append(caps, [2]string{"↵", tr("показать все", "show all")})
	case l.kind == mlEntry || (l.kind == mlSong && l.it.Kind != MusicSong) || (l.kind == mlTile && l.it.Kind != MusicSong):
		caps = append(caps, [2]string{"↵", tr("открыть", "open")})
	default:
		caps = append(caps, [2]string{"↵", tr("играть", "play")})
	}
	if ok && (l.kind == mlSong || l.kind == mlTile) && l.it.Unplayable == "" {
		caps = append(caps, [2]string{"ctrl+n", m.mlFollowLabel()})
	}
	if m.musicStyle() == musicStylePalette {
		if ok && (l.kind == mlSong || l.kind == mlTile) && l.it.Unplayable == "" {
			caps = append(caps, [2]string{"ctrl+e", tr("в очередь", "queue")})
		}
		if !ok || l.kind != mlMore {
			caps = append(caps, [2]string{"ctrl+k", tr("действия", "actions")})
		}
		caps = append(caps, [2]string{"tab", tr("агент", "agent")})
	}
	caps = append(caps, [2]string{"esc", tr("назад", "back")})
	return caps
}

// mlRule draws the quiet line between the rows and the chrome.
func (m *Model) mlRule(f *fb, x, y, w int) {
	for i := range w {
		f.put(x+i, y, '─', m.th.dim.mul(0.35))
	}
}
