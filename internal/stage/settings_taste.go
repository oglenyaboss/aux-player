package stage

import (
	"fmt"
	"math"
	"strings"
)

// MemoryEntry is one line of the picker agent's memory, as the settings show it.
type MemoryEntry struct {
	Text     string
	Evidence string
	Created  string
	Updated  string
}

// RatedSong is one track the picker agent was told to play more or less of.
type RatedSong struct {
	ID, Artist, Title, Kind string // Kind "more" or "less"
}

// memUI is the memory sub-screen of the settings: the list, its editor and the armed delete.
type memUI struct {
	entries []MemoryEntry
	sel     int
	loading bool
	confirm bool
	editing bool
	editIdx int // -1 for a new entry
	buf     []rune
	errMsg  string
}

// ratedUI is the liked/disliked sub-screen of the settings, one tab per kind.
type ratedUI struct {
	songs   []RatedSong
	tab     string // "more" or "less"
	sel     int
	loading bool
	errMsg  string
}

func (u *ratedUI) tabSongs() []RatedSong {
	var out []RatedSong
	for _, s := range u.songs {
		if s.Kind == u.tab {
			out = append(out, s)
		}
	}
	return out
}

// singleLine flattens a stored memory line for the screen: breaks become spaces.
func singleLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}

// ratedLabel names a rated track on screen: the artist and the title, or whichever is set.
func ratedLabel(t RatedSong) string {
	switch {
	case t.Artist != "" && t.Title != "":
		return t.Artist + " — " + t.Title
	case t.Artist != "":
		return t.Artist
	case t.Title != "":
		return t.Title
	}
	return t.ID
}

// SetMemory answers LoadMemory: it fills the memory sub-screen, or reports why it could
// not while keeping the previous entries on screen.
func (m *Model) SetMemory(entries []MemoryEntry, err string) {
	if m.settings == nil {
		return
	}
	if err == "" {
		m.settings.info.MemoryCount = len(entries)
	}
	if s := m.settings.mem; s != nil {
		if err == "" {
			s.entries = entries
		}
		s.loading, s.confirm, s.editing = false, false, false
		s.errMsg = err
		s.sel = max(0, min(s.sel, len(s.entries)-1))
	}
}

// SetRated answers LoadRated: it fills the liked/disliked sub-screen, or reports why it
// could not while keeping the previous list on screen.
func (m *Model) SetRated(songs []RatedSong, err string) {
	if m.settings == nil {
		return
	}
	if err == "" {
		m.settings.info.RatedCount = len(songs)
	}
	if s := m.settings.rated; s != nil {
		if err == "" {
			s.songs = songs
		}
		s.loading = false
		s.errMsg = err
		s.sel = max(0, min(s.sel, len(s.tabSongs())-1))
	}
}

func (m *Model) settingsMemKey(k, text string) {
	s := m.settings.mem
	if s.loading {
		if k == "esc" {
			m.settings.mem = nil
		}
		return
	}
	if s.editing {
		m.memEditKey(k, text)
		return
	}
	if k == "esc" {
		m.settings.mem = nil
		return
	}
	armed := s.confirm
	s.confirm = false
	switch k {
	case "up", "k":
		s.sel = max(0, s.sel-1)
	case "down", "j":
		s.sel = min(max(0, len(s.entries)-1), s.sel+1)
	case "a":
		s.editing, s.editIdx, s.buf, s.errMsg = true, -1, nil, ""
	case "enter", "e":
		switch {
		case armed:
			m.memDelete()
		case len(s.entries) > 0:
			s.editing, s.editIdx, s.buf, s.errMsg = true, s.sel, []rune(singleLine(s.entries[s.sel].Text)), ""
		}
	case "d", "backspace":
		if len(s.entries) > 0 {
			if armed {
				m.memDelete()
			} else {
				s.confirm = true
			}
		}
	}
}

func (m *Model) memDelete() {
	s := m.settings.mem
	m.send(MemoryDelete{Index: s.sel})
	s.loading, s.confirm = true, false
}

func (m *Model) memEditKey(k, text string) {
	s := m.settings.mem
	switch k {
	case "esc":
		s.editing, s.buf, s.errMsg = false, nil, ""
	case "enter":
		txt := strings.TrimSpace(string(s.buf))
		if txt == "" {
			s.errMsg = tr("введи текст", "enter text")
			return
		}
		if s.editIdx < 0 {
			m.send(MemoryAdd{Text: txt})
		} else {
			m.send(MemoryEdit{Index: s.editIdx, Text: txt})
		}
		s.loading, s.editing = true, false
	case "backspace":
		if len(s.buf) > 0 {
			s.buf = s.buf[:len(s.buf)-1]
		}
	default:
		if room := 300 - len(s.buf); room > 0 && text != "" {
			rs := []rune(text)
			s.buf = append(s.buf, rs[:min(room, len(rs))]...)
		}
	}
}

func (m *Model) settingsRatedKey(k, text string) {
	s := m.settings.rated
	if k == "esc" {
		m.settings.rated = nil
		return
	}
	if s.loading {
		return
	}
	vis := s.tabSongs()
	picked := func() (RatedSong, bool) {
		if s.sel < 0 || s.sel >= len(vis) {
			return RatedSong{}, false
		}
		return vis[s.sel], true
	}
	switch k {
	case "tab", "left", "right", "h", "l":
		if s.tab == "more" {
			s.tab = "less"
		} else {
			s.tab = "more"
		}
		s.sel = 0
	case "up", "k":
		s.sel = max(0, s.sel-1)
	case "down", "j":
		s.sel = min(max(0, len(vis)-1), s.sel+1)
	case "enter", "space":
		if t, ok := picked(); ok {
			kind := "more"
			if t.Kind == "more" {
				kind = "less"
			}
			m.send(RatedSet{ID: t.ID, Artist: t.Artist, Title: t.Title, Kind: kind})
			s.loading = true
		}
	case "d", "backspace":
		if t, ok := picked(); ok {
			m.send(RatedSet{ID: t.ID, Artist: t.Artist, Title: t.Title, Kind: "rate_clear"})
			s.loading = true
		}
	}
}

func (m *Model) drawMemUI(f *fb, x, y, w, h int) {
	s := m.settings.mem
	if s.loading {
		m.wave(f, x, y, tr("читаю память", "loading memory"), m.th.a1)
		return
	}
	if s.editing {
		m.drawMemEdit(f, x, y, w, h)
		return
	}
	f.textS(x, y, trunc(tr("память", "memory"), w), m.th.a1, true, false)
	hintY := y + h
	statusY := hintY - 2
	pvTop := statusY - 3
	listTop := y + 2
	rows := min(6, max(0, pvTop-listTop))
	first := 0
	if rows > 0 && s.sel >= rows {
		first = s.sel - rows + 1
	}
	for i := first; i < len(s.entries) && i < first+rows; i++ {
		yy := listTop + i - first
		if i == s.sel {
			for xx := x; xx < x+w; xx++ {
				f.tint(xx, yy, m.th.a1, 0.18)
			}
			f.text(x, yy, "›", m.th.a1)
		}
		fg := m.th.dim
		if i == s.sel {
			fg = m.th.text
		}
		f.text(x+2, yy, trunc(singleLine(s.entries[i].Text), w-3), fg)
	}
	if len(s.entries) == 0 {
		f.text(x, listTop, tr("пусто", "empty"), m.th.dim)
	} else if pvTop >= listTop {
		for j, line := range wrapWords(singleLine(s.entries[min(s.sel, len(s.entries)-1)].Text), w-4, 3) {
			f.text(x+2, pvTop+j, line, m.th.dim)
		}
	}
	switch {
	case s.confirm:
		f.text(x, statusY, trunc(tr("d или enter ещё раз — удалить", "d or enter again to delete"), w), m.th.bad)
	case s.errMsg != "":
		f.text(x, statusY, trunc(s.errMsg, w), m.th.bad)
	default:
		f.text(x, statusY, trunc(tr("изменения применятся с новой сессии", "changes apply on the next session"), w), m.th.dim)
	}
	dx := m.mlCap(f, x, hintY, "a", tr("добавить", "add")) + 2
	dx += m.mlCap(f, x+dx, hintY, "enter", tr("изменить", "edit")) + 2
	dx += m.mlCap(f, x+dx, hintY, "d", tr("удалить", "delete")) + 2
	if x+dx < x+w {
		m.mlCap(f, x+dx, hintY, "esc", tr("назад", "back"))
	}
}

func (m *Model) drawMemEdit(f *fb, x, y, w, h int) {
	s := m.settings.mem
	f.textS(x, y, "›", m.th.a1, true, false)
	buf := s.buf
	if n := max(0, w-5); len(buf) > n {
		buf = buf[len(buf)-n:]
	}
	n := f.text(x+2, y, string(buf), m.th.text)
	f.fade(x+2+n, y, "▎", m.th.a1, 0.35+0.65*(0.5+0.5*math.Sin(m.t*6)), false)
	if s.errMsg != "" {
		f.text(x, y+2, trunc(s.errMsg, w), m.th.bad)
	}
	hy := y + h - 2
	dx := m.mlCap(f, x, hy, "enter", tr("готово", "done"))
	dx += 2
	if x+dx < x+w {
		m.mlCap(f, x+dx, hy, "esc", tr("назад", "back"))
	}
}

func (m *Model) drawRatedUI(f *fb, x, y, w, h int) {
	s := m.settings.rated
	if s.loading {
		m.wave(f, x, y, tr("читаю оценки", "loading ratings"), m.th.a1)
		return
	}
	var more, less int
	for _, sg := range s.songs {
		switch sg.Kind {
		case "more":
			more++
		case "less":
			less++
		}
	}
	dx := x
	for i, label := range [2]string{
		fmt.Sprintf("%s (%d)", tr("нравится", "liked"), more),
		fmt.Sprintf("%s (%d)", tr("не нравится", "disliked"), less),
	} {
		if i == 1 {
			dx += f.text(dx, y, " · ", m.th.dim)
		}
		fg, bold := m.th.dim, false
		if (i == 0) == (s.tab == "more") {
			fg, bold = m.th.a1, true
		}
		dx += f.textS(dx, y, label, fg, bold, false)
	}
	hintY := y + h
	errY := hintY - 2
	listTop := y + 2
	rows := min(8, max(0, errY-listTop))
	vis := s.tabSongs()
	first := 0
	if rows > 0 && s.sel >= rows {
		first = s.sel - rows + 1
	}
	for i := first; i < len(vis) && i < first+rows; i++ {
		yy := listTop + i - first
		if i == s.sel {
			for xx := x; xx < x+w; xx++ {
				f.tint(xx, yy, m.th.a1, 0.18)
			}
			f.text(x, yy, "›", m.th.a1)
		}
		fg := m.th.dim
		if i == s.sel {
			fg = m.th.text
		}
		f.text(x+2, yy, trunc(ratedLabel(vis[i]), w-3), fg)
	}
	if len(vis) == 0 {
		f.text(x, listTop, tr("пусто", "empty"), m.th.dim)
	}
	if s.errMsg != "" {
		f.text(x, errY, trunc(s.errMsg, w), m.th.bad)
	}
	cx := m.mlCap(f, x, hintY, "tab", tr("вкладки", "tabs")) + 2
	cx += m.mlCap(f, x+cx, hintY, "enter", tr("переключить", "toggle")) + 2
	cx += m.mlCap(f, x+cx, hintY, "d", tr("убрать", "clear")) + 2
	if x+cx < x+w {
		m.mlCap(f, x+cx, hintY, "esc", tr("назад", "back"))
	}
}
