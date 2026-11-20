package stage

// The actions menu of the palette: ctrl+k, or right on a song, opens the few things
// the host can do with the highlighted row.
type mlMenuKind int

const (
	mlMenuPlay mlMenuKind = iota
	mlMenuOpen
	mlMenuQueue
	mlMenuLibrary
	mlMenuFavorite
	mlMenuPlayAll
	mlMenuFollow
	mlMenuSettings
)

type mlMenuEntry struct {
	kind  mlMenuKind
	label string
}

type mlMenu struct {
	open bool
	sel  int
}

// mlMenuItems is what the selected row offers: a song plays, queues and saves, a
// collection opens and plays whole; every row, the root and the empty one included,
// offers the settings.
func (m *Model) mlMenuItems() []mlMenuEntry {
	settings := mlMenuEntry{mlMenuSettings, tr("настройки", "settings")}
	l, ok := m.mlSelected()
	if !ok || (l.kind != mlSong && l.kind != mlTile) {
		return []mlMenuEntry{settings}
	}
	if l.it.Kind == MusicSong {
		return []mlMenuEntry{
			{mlMenuPlay, tr("играть", "play")},
			{mlMenuQueue, tr("в очередь", "add to queue")},
			{mlMenuLibrary, tr("в библиотеку", "add to library")},
			{mlMenuFavorite, tr("в избранное", "add to favorites")},
			{mlMenuFollow, m.mlFollowLabel()},
			settings,
		}
	}
	return []mlMenuEntry{
		{mlMenuOpen, tr("открыть", "open")},
		{mlMenuPlayAll, tr("играть всё", "play all")},
		{mlMenuFollow, m.mlFollowLabel()},
		settings,
	}
}

// mlMenuHint is the key that runs the entry, drawn at the right of its row.
func mlMenuHint(k mlMenuKind) string {
	switch k {
	case mlMenuPlay, mlMenuOpen:
		return "↵"
	case mlMenuQueue:
		return "ctrl+e"
	case mlMenuFollow:
		return "ctrl+n"
	}
	return ""
}

// mlMenuOpenAt opens the menu on the selected row.
func (m *Model) mlMenuOpenAt() bool {
	if m.musicStyle() != musicStylePalette || m.mu.mode != mlModeMusic {
		return false
	}
	if len(m.mlMenuItems()) == 0 {
		return false
	}
	m.mu.menu = mlMenu{open: true}
	return true
}

// mlMenuCanRight reports whether right belongs to the menu: only on a song row.
func (m *Model) mlMenuCanRight() bool {
	if m.musicStyle() != musicStylePalette || m.mu.mode != mlModeMusic || m.mlGridShown() {
		return false
	}
	l, ok := m.mlSelected()
	return ok && l.kind == mlSong && l.it.Kind == MusicSong
}

// mlMenuKey is the menu while it is open: up and down walk it, enter runs the entry,
// esc or ctrl+k closes it and any other key gives the list its keys back.
func (m *Model) mlMenuKey(k string) bool {
	items := m.mlMenuItems()
	switch k {
	case "esc", "ctrl+k":
		m.mu.menu.open = false
	case "up":
		m.mu.menu.sel = max(0, min(m.mu.menu.sel-1, len(items)-1))
	case "down":
		m.mu.menu.sel = min(len(items)-1, m.mu.menu.sel+1)
	case "enter":
		m.mlMenuChoose()
		return true
	default:
		return false
	}
	return true
}

// mlMenuChoose runs the highlighted entry and closes the menu.
func (m *Model) mlMenuChoose() {
	items := m.mlMenuItems()
	if len(items) == 0 {
		m.mu.menu.open = false
		return
	}
	x := items[max(0, min(m.mu.menu.sel, len(items)-1))]
	m.mu.menu.open = false
	if x.kind == mlMenuSettings {
		m.closeMusic()
		m.send(Settings{})
		return
	}
	l, ok := m.mlSelected()
	if !ok {
		return
	}
	switch x.kind {
	case mlMenuPlay:
		m.mlPlay(l)
	case mlMenuOpen:
		m.mlEnter()
	case mlMenuQueue:
		m.mlAppend()
	case mlMenuLibrary:
		m.send(MusicSave{Item: l.it})
	case mlMenuFavorite:
		m.send(MusicSave{Item: l.it, Favorites: true})
	case mlMenuPlayAll:
		m.send(MusicPlayAll{Item: l.it})
	case mlMenuFollow:
		m.mlToggleFollow()
	}
}

// mlDrawMenu paints the open menu of the selected row inside the panel frame.
func (m *Model) mlDrawMenu(f *fb) {
	if !m.mu.menu.open {
		return
	}
	items := m.mlMenuItems()
	if len(items) == 0 {
		return
	}
	w := 20
	for _, it := range items {
		w = max(w, len([]rune(it.label))+8)
		if hint := mlMenuHint(it.kind); hint != "" {
			w = max(w, len([]rune(it.label))+len([]rune(hint))+7)
		}
	}
	w = min(w, max(8, f.w-2))
	h := len(items) + 2
	var x, y int
	if m.mlGridShown() {
		x, y = m.mu.tileX, m.mu.tileY+mlTileH+2
		if y+h > f.h-2 {
			y = m.mu.tileY - h
		}
		x, y = max(1, min(x, f.w-w-1)), max(1, min(y, f.h-h-1))
	} else {
		x, y = m.mu.rowL+7, m.mu.rowY+max(1, m.mu.rowH)
		if y+h > f.h-2 {
			y = m.mu.rowY - h
		}
		x, y = max(1, min(x, f.w-w-1)), max(1, min(y, f.h-h-1))
	}
	bg := m.th.bg.lerp(m.th.text, 0.07)
	f.fill(x, y, w, h, bg)
	m.glowBox(f, x, y, w, h, 1)
	for i, it := range items {
		yy := y + 1 + i
		if i == m.mu.menu.sel {
			for xx := x + 1; xx < x+w-1; xx++ {
				f.tint(xx, yy, m.th.a1, 0.18)
			}
		}
		fg := m.th.text
		if i == m.mu.menu.sel {
			fg = m.th.a1
		}
		f.textS(x+2, yy, trunc(it.label, w-7), fg, i == m.mu.menu.sel, false)
		if hint := mlMenuHint(it.kind); hint != "" {
			f.text(x+w-1-len([]rune(hint)), yy, hint, m.th.dim)
		}
	}
}
