package stage

import (
	"fmt"
	"math"
	"strings"
	"unicode"
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
	cur     int // cursor offset into buf
	scroll  int // first wrapped row on screen
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
		s.editing, s.editIdx, s.buf, s.cur, s.scroll, s.errMsg = true, -1, nil, 0, 0, ""
	case "enter", "e":
		switch {
		case armed:
			m.memDelete()
		case len(s.entries) > 0:
			buf := []rune(singleLine(s.entries[s.sel].Text))
			s.editing, s.editIdx, s.buf, s.cur, s.scroll, s.errMsg = true, s.sel, buf, len(buf), 0, ""
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

// editLine is one soft-wrapped row of the memory editor: buf[start:end].
type editLine struct{ start, end int }

// wrapEdit partitions buf into wrapped rows of at most w runes, breaking after the
// last space of the window and keeping every space verbatim; a full last row gets an
// empty trailing row so the end cursor still sits inside the width.
func wrapEdit(buf []rune, w int) []editLine {
	w = max(1, w)
	if len(buf) == 0 {
		return []editLine{{0, 0}}
	}
	var lines []editLine
	for pos := 0; pos < len(buf); {
		end := len(buf)
		if len(buf)-pos > w {
			end = -1
			for i := pos + w - 1; i >= pos; i-- {
				if buf[i] == ' ' {
					end = i + 1
					break
				}
			}
			if end < 0 {
				end = pos + w
			}
		}
		lines = append(lines, editLine{pos, end})
		pos = end
	}
	if last := lines[len(lines)-1]; last.end-last.start == w {
		lines = append(lines, editLine{len(buf), len(buf)})
	}
	return lines
}

// editCursorPos maps a buffer offset onto its wrapped row and column.
func editCursorPos(lines []editLine, cur int) (row, col int) {
	for i, l := range lines {
		if cur < l.end || i == len(lines)-1 {
			return i, cur - l.start
		}
	}
	return 0, 0
}

// memWordStart is the start of the word left of cur: trailing spaces first, then the word.
func memWordStart(buf []rune, cur int) int {
	i := cur
	for i > 0 && unicode.IsSpace(buf[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(buf[i-1]) {
		i--
	}
	return i
}

// memWordEnd is the end of the word right of cur: leading spaces first, then the word.
func memWordEnd(buf []rune, cur int) int {
	i := cur
	for i < len(buf) && unicode.IsSpace(buf[i]) {
		i++
	}
	for i < len(buf) && !unicode.IsSpace(buf[i]) {
		i++
	}
	return i
}

// memEditWidth is the editor's wrapping width: the settings box minus its margins.
func (m *Model) memEditWidth() int {
	return max(1, min(72, max(8, m.w-4))-7)
}

func (m *Model) memEditKey(k, text string) {
	s := m.settings.mem
	switch k {
	case "esc":
		s.editing, s.buf, s.cur, s.scroll, s.errMsg = false, nil, 0, 0, ""
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
	case "left":
		s.cur = max(0, s.cur-1)
	case "right":
		s.cur = min(len(s.buf), s.cur+1)
	case "up", "down":
		lines := wrapEdit(s.buf, m.memEditWidth())
		row, col := editCursorPos(lines, s.cur)
		if k == "up" {
			if row == 0 {
				s.cur = 0
				break
			}
			row--
		} else {
			if row == len(lines)-1 {
				s.cur = len(s.buf)
				break
			}
			row++
		}
		l := lines[row]
		end := l.end - 1
		if row == len(lines)-1 {
			end = l.end
		}
		s.cur = min(l.start+col, end)
	case "home", "ctrl+a":
		s.cur = 0
	case "end", "ctrl+e":
		s.cur = len(s.buf)
	case "alt+left", "ctrl+left", "alt+b":
		s.cur = memWordStart(s.buf, s.cur)
	case "alt+right", "ctrl+right", "alt+f":
		s.cur = memWordEnd(s.buf, s.cur)
	case "backspace":
		if s.cur > 0 {
			s.buf = append(s.buf[:s.cur-1], s.buf[s.cur:]...)
			s.cur--
			s.errMsg = ""
		}
	case "delete", "ctrl+d":
		if s.cur < len(s.buf) {
			s.buf = append(s.buf[:s.cur], s.buf[s.cur+1:]...)
			s.errMsg = ""
		}
	case "ctrl+w", "alt+backspace":
		start := memWordStart(s.buf, s.cur)
		s.buf = append(s.buf[:start], s.buf[s.cur:]...)
		s.cur = start
		s.errMsg = ""
	case "ctrl+u":
		s.buf = append(s.buf[:0], s.buf[s.cur:]...)
		s.cur = 0
		s.errMsg = ""
	case "ctrl+k":
		s.buf = s.buf[:s.cur]
		s.errMsg = ""
	default:
		if text == "" {
			return
		}
		rs := []rune(text)
		room := 300 - len(s.buf)
		if room <= 0 {
			return
		}
		rs = rs[:min(room, len(rs))]
		next := make([]rune, 0, len(s.buf)+len(rs))
		next = append(next, s.buf[:s.cur]...)
		next = append(next, rs...)
		next = append(next, s.buf[s.cur:]...)
		s.buf = next
		s.cur += len(rs)
		s.errMsg = ""
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
	title := tr("новая запись", "new entry")
	if s.editIdx >= 0 {
		title = tr("изменить запись", "edit entry")
	}
	tw := f.textS(x, y, title, m.th.a1, true, false)
	cnt := fmt.Sprintf("%d/300", len(s.buf))
	if cw := len([]rune(cnt)); cw+tw+1 <= w {
		fg := m.th.dim
		if len(s.buf) >= 300 {
			fg = m.th.bad
		}
		f.text(x+w-cw, y, cnt, fg)
	}
	hintY := y + h
	statusY := hintY - 2
	if s.errMsg != "" {
		f.text(x, statusY, trunc(s.errMsg, w), m.th.bad)
	}
	textTop := y + 2
	visRows := max(1, (statusY-1)-textTop+1)
	lines := wrapEdit(s.buf, m.memEditWidth())
	curRow, curCol := editCursorPos(lines, s.cur)
	s.scroll = min(max(0, len(lines)-visRows), max(0, s.scroll))
	s.scroll = min(s.scroll, curRow)
	if curRow > s.scroll+visRows-1 {
		s.scroll = curRow - visRows + 1
	}
	f.textS(x, textTop, "›", m.th.a1, true, false)
	pulse := 0.35 + 0.65*(0.5+0.5*math.Sin(m.t*6))
	for r := s.scroll; r < len(lines) && r < s.scroll+visRows; r++ {
		l := lines[r]
		ty := textTop + r - s.scroll
		for xx := x + 1; xx < x+w-1; xx++ {
			f.tint(xx, ty, m.th.a1, 0.08)
		}
		f.text(x+2, ty, string(s.buf[l.start:l.end]), m.th.text)
	}
	ty := textTop + curRow - s.scroll
	tx := x + 2 + curCol
	if s.cur == len(s.buf) {
		f.fade(tx, ty, "▎", m.th.a1, pulse, false)
	} else if c := f.at(tx, ty); c != nil {
		f.tint(tx, ty, m.th.a1, pulse)
		c.fg = m.th.text.lerp(m.th.bg, pulse)
	}
	dx := m.mlCap(f, x, hintY, "enter", tr("готово", "done")) + 2
	if x+dx < x+w {
		dx += m.mlCap(f, x+dx, hintY, "←↑↓→", tr("курсор", "move")) + 2
	}
	if x+dx < x+w {
		m.mlCap(f, x+dx, hintY, "esc", tr("назад", "back"))
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
