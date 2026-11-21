package stage

import (
	"fmt"
	"math"
	"strings"
)

// musicMargin is the air on both sides of the screen: the header sits on row 1, the tabs
// on row 3, the list on row 5, the strip on h-3 with its rule on h-4 and the hints on
// h-2, as the play screen's top bar.
const (
	musicMargin = 4
	musicHeadY  = 1
	musicTabsY  = 3
	musicBodyY  = 5
)

// drawMusicFull is the full-screen music screen: header with the search field, the
// section tabs, the list, the preview of the highlight and the now-playing strip. It
// covers the whole frame but lets the backdrop behind it flow through.
func (m *Model) drawMusicFull(f *fb) {
	m.musicWipe(f)
	m.drawMusicHead(f)
	m.drawMusicTabs(f)

	top, _ := m.musicArea()
	if c := m.mu.coll; c != nil {
		m.drawMusicCrumb(f, c)
	}
	inner := m.musicRight() - musicMargin
	lw := inner
	if m.w >= 100 {
		lw = int(float64(inner) * 0.58)
		m.drawMusicPreview(f, musicMargin+lw+3, musicBodyY)
	}
	m.drawMusicList(f, top, lw)
	m.drawMusicNotice(f)
	m.drawMusicNow(f)
}

// musicRight is the first column past the screen's content.
func (m *Model) musicRight() int { return m.w - musicMargin }

// musicArea is the list area: the first row and the exclusive bottom row.
func (m *Model) musicArea() (top, limit int) {
	top = musicBodyY
	if m.mu.coll != nil {
		top++
	}
	limit = m.h - musicMargin
	if m.musicNoticeVisible() {
		limit--
	}
	return top, max(top, limit)
}

// musicWipe lays the screen's own backdrop: the record on show, flowing, over the stage theme.
func (m *Model) musicWipe(f *fb) {
	key := ""
	if it, ok := m.musicPreviewItem(); ok {
		key = it.Key()
	}
	for y := range f.h {
		for x := range f.w {
			c := &f.c[y*f.w+x]
			c.ch, c.pix, c.bold, c.ital = ' ', false, false, false
			c.bg = m.th.bg
			if key == "" {
				continue
			}
			field := m.coverField(key, float64(x)/float64(f.w), float64(y*2)/float64(f.h*2))
			c.bg = m.th.bg.lerp(field, 0.9)
		}
	}
}

func (m *Model) drawMusicHead(f *fb) {
	right, top := m.musicRight(), musicHeadY
	hint := tr("? клавиши", "? keys")
	hx := right - len([]rune(hint))
	f.textS(musicMargin, top, "aux · ", m.th.a1, true, false)
	f.textS(musicMargin+6, top, tr("музыка", "music"), m.th.a1, true, false)
	if hx > 24 {
		f.text(hx, top, hint, m.th.dim)
	}
	fw := min(26, hx-2-18)
	if fw < 8 {
		return
	}
	fx := hx - 2 - fw
	f.put(fx, top, '/', m.th.dim)
	q := []rune(m.mu.tabs[MusicSearch].query)
	shown := q
	if over := len(shown) - (fw - 3); over > 0 {
		shown = shown[over:]
	}
	f.text(fx+2, top, string(shown), m.th.text)
	line := m.th.dim.mul(0.6)
	if m.mu.focus {
		line = m.th.a1
	}
	for i := range fw {
		f.put(fx+i, top+1, '─', line)
	}
	if m.mu.focus {
		cx := min(fx+2+len(shown), fx+fw-1)
		f.fade(cx, top, "▎", m.th.a1, 0.35+0.65*(0.5+0.5*math.Sin(m.t*6)), false)
	}
}

func (m *Model) drawMusicTabs(f *fb) {
	right := m.musicRight()
	gap := 3
	if m.w < 80 {
		gap = 2
	}
	if m.w < 60 {
		gap = 1
	}
	y := musicTabsY
	x := musicMargin
	for s := MusicFavorites; s <= MusicSearch; s++ {
		label := musicTabLabel(s)
		if s == m.mu.tab && mlSortable(s) {
			label += " · " + m.mu.tabs[s].sort.label()
		}
		lab := []rune(label)
		if x+len(lab) > right {
			return
		}
		act := s == m.mu.tab
		fg := m.th.dim
		if act {
			fg = m.th.a1
		}
		f.textS(x, y, string(lab), fg, act, false)
		if act {
			for i := range lab {
				f.put(x+i, y+1, '━', m.th.a1)
			}
		}
		x += len(lab) + gap
	}
}

func (m *Model) drawMusicCrumb(f *fb, c *musicColl) {
	right, y := m.musicRight(), musicBodyY
	root := []rune(musicTabLabel(c.section))
	f.text(musicMargin, y, string(root), m.th.dim)
	f.text(musicMargin+len(root)+1, y, "›", m.th.dim)
	x := musicMargin + len(root) + 3
	f.textS(x, y, trunc(c.parent.Title, max(1, right-x)), m.th.text, true, false)
}

// drawMusicList is the left column: grouped search results or the rows of a tab or a collection.
func (m *Model) drawMusicList(f *fb, top, w int) {
	_, limit := m.musicArea()
	vis := limit - top
	if vis <= 0 || w < 4 {
		return
	}
	x := musicMargin
	v := m.musicViewState()
	switch {
	case v.err != "":
		f.text(x, top, trunc(v.err, w), m.th.bad)
		if vis > 1 {
			f.text(x, top+1, trunc(tr("r — повторить", "r — retry"), w), m.th.dim)
		}
		return
	case len(v.items) == 0 && v.loading:
		for i := range min(6, vis) {
			b := 0.18 + 0.14*(0.5+0.5*math.Sin(m.t*3-float64(i)*0.8))
			f.text(x+2, top+i, "· · · ·", m.th.dim.mul(b))
			if w > 20 {
				f.text(x+12, top+i, strings.Repeat("·", min(w-14, 18)), m.th.dim.mul(b*0.6))
			}
		}
		return
	case len(v.items) == 0:
		if m.mu.coll == nil && m.mu.tab == MusicSearch && !m.mu.tabs[MusicSearch].sent {
			return
		}
		msg := tr("пусто", "nothing here")
		if m.mu.tab == MusicSearch && m.mu.coll == nil {
			msg = tr("ничего не нашлось", "no matches")
		}
		f.text(x+max(0, (w-len([]rune(msg)))/2), top+max(0, vis/2-1), msg, m.th.dim)
		return
	}
	lines := musicLines(v.items, v.grouped)
	win := musicWindow(lines, v.sel, vis)
	for i := range vis {
		if win+i >= len(lines) {
			return
		}
		l := lines[win+i]
		y := top + i
		if l.item < 0 {
			f.textS(x+2, y, l.header, m.th.dim, true, false)
			continue
		}
		m.musicRow(f, x, y, w, v.items[l.item], l.item == v.sel, v.coll != nil, l.item+1)
	}
}

// musicLine is one drawn row of the list: a group header or an item.
type musicLine struct {
	header string
	item   int
}

// musicLines interleaves the group headers of the search results with the items.
func musicLines(items []MusicItem, grouped bool) []musicLine {
	out := make([]musicLine, 0, len(items)+3)
	last := MusicKind(-1)
	for i, it := range items {
		if grouped && it.Kind != last {
			out = append(out, musicLine{header: musicKindLabel(it.Kind), item: -1})
			last = it.Kind
		}
		out = append(out, musicLine{item: i})
	}
	return out
}

// musicWindow is the first line drawn so that the selected item stays in sight.
func musicWindow(lines []musicLine, sel, vis int) int {
	if vis <= 0 || len(lines) <= vis {
		return 0
	}
	at := 0
	for i, l := range lines {
		if l.item == sel {
			at = i
			break
		}
	}
	return min(max(0, at-vis/2), len(lines)-vis)
}

func (m *Model) musicRow(f *fb, x, y, w int, it MusicItem, sel, numbered bool, num int) {
	bad := it.Unplayable != ""
	cur := !bad && m.musicCurrent(it)
	if sel {
		for xx := x; xx < x+w; xx++ {
			f.tint(xx, y, m.th.a1, 0.15)
		}
		c := m.th.a1
		if bad {
			c = m.th.dim
		}
		f.put(x, y, '›', c)
	} else if cur {
		f.put(x, y, '♪', m.th.a1)
	}
	tx := x + 2
	if numbered {
		f.text(tx, y, fmt.Sprintf("%02d", num), m.th.dim.mul(0.85))
		tx += 3
	} else if m.w >= 60 {
		g := m.cover(it.Key(), 2, 2)
		for py := range 2 {
			for px := range 2 {
				f.pixel(tx+px, y*2+py, g.at(px, py))
			}
		}
		tx += 3
	}
	meta := ""
	if m.w >= 60 {
		meta = musicMeta(it)
		if bad {
			meta = "✕"
		}
	}
	mx := x + w
	if meta != "" {
		mx -= len([]rune(meta)) + 1
	}
	room := mx - 1 - tx
	if room < 1 {
		return
	}
	fg, dfg := m.th.text, m.th.dim
	if bad {
		fg, dfg = m.th.dim.mul(0.9), m.th.dim.mul(0.55)
	}
	if sel && cur {
		fg = m.th.a1
	}
	if fl := m.musicFlash(it); fl > 0 {
		fg = fg.lerp(white, fl)
	}
	title := it.Title
	if it.Kind != MusicSong {
		title += " ›"
	}
	t := trunc(title, room)
	n := f.textS(tx, y, t, fg, sel && !bad, false)
	if a := room - n - 2; a > 4 {
		f.text(tx+n+2, y, trunc(it.Artist, a), dfg)
	}
	if meta != "" {
		c := m.th.dim
		if bad {
			c = m.th.bad
		}
		f.text(mx, y, meta, c)
	}
}

// musicFlash is how lit the row that just went to the host is.
func (m *Model) musicFlash(it MusicItem) float64 {
	if it.Key() != m.mu.playKey || m.t-m.mu.playT > 0.6 {
		return 0
	}
	return 1 - prog(m.t-m.mu.playT, 0, 0.5)
}

// musicCurrent: the row is the track the stage plays right now.
func (m *Model) musicCurrent(it MusicItem) bool {
	return m.ph == phPlay && len(m.queue) > 0 && m.curTrack().Key() == it.Key()
}

// drawMusicPreview is the right column: the cover of the item and its name, big.
func (m *Model) drawMusicPreview(f *fb, x, top int) {
	_, limit := m.musicArea()
	w := m.musicRight() - x
	avail := limit - top
	if w < 10 || avail < 10 {
		return
	}
	it, ok := m.musicPreviewItem()
	ch := min(w, 2*max(6, avail-10))
	if ch >= 8 {
		if ok {
			cov := m.cover(it.Key(), ch, ch)
			for py := range ch {
				for px := range ch {
					f.pixel(x+px, top*2+py, cov.at(px, py))
				}
			}
		} else if ch/2 > 2 {
			m.musicFrame(f, x, top, ch, ch/2)
		}
	}
	y := top + ch/2 + 1
	rows := 0
	if ok && limit-y >= 7 {
		rows, _ = m.bigText(f, x, y, it.Title, w, easeOut(prog(m.t-m.mu.selT, 0, 0.5)), 0, 7, nil, m.mu.selT)
	}
	if ok && rows == 0 && y < limit {
		rows = 1
		f.textS(x, y, trunc(it.Title, w), m.th.a1.lerp(m.th.text, 0.3), true, false)
	}
	y += rows + 1
	if !ok || y >= limit {
		return
	}
	f.text(x, y, trunc(it.Artist, w), m.th.text)
	y++
	if it.Sub != "" && y < limit {
		f.text(x, y, trunc(it.Sub, w), m.th.dim)
		y++
	}
	if it.Kind != MusicSong && it.Count > 0 && y < limit {
		f.text(x, y, trunc(fmt.Sprintf(tr("%d треков", "%d tracks"), it.Count), w), m.th.dim)
	}
}

// musicFrame is the empty cover: a quiet box where the artwork will land.
func (m *Model) musicFrame(f *fb, x, y, w, h int) {
	if w < 2 || h < 2 {
		return
	}
	for _, q := range perim(x, y, w, h) {
		f.put(q.x, q.y, boxRune(q, x, y, w, h), m.th.dim.mul(0.6))
	}
	if w > 4 && h > 2 {
		f.put(x+w/2, y+h/2, '·', m.th.dim.mul(0.5))
	}
}

func (m *Model) drawMusicNotice(f *fb) {
	if !m.musicNoticeVisible() {
		return
	}
	y := m.h - musicMargin - 1
	if y < musicBodyY {
		return
	}
	a := 1 - prog(m.t-m.mu.noticeT, 2.4, 0.8)
	if a <= 0 {
		return
	}
	f.fade(musicMargin, y, trunc(m.mu.notice, m.musicRight()-musicMargin), m.th.a2.lerp(m.th.text, 0.4), a, false)
}

func (m *Model) drawMusicNow(f *fb) {
	right, x := m.musicRight(), musicMargin
	y := m.h - 3
	if y < musicBodyY+1 {
		return
	}
	for i := x; i < right; i++ {
		f.put(i, y-1, '─', m.th.dim.mul(0.35))
	}
	if hints := []rune(tr("enter играть · a в очередь · / поиск · esc назад", "enter play · a queue · / search · esc back")); len(hints) <= right-x {
		f.text(right-len(hints), m.h-2, string(hints), m.th.dim.mul(0.9))
	}
	if m.ph != phPlay || len(m.queue) == 0 {
		f.fade(x, y, trunc(tr("тишина — выбери что-нибудь", "silence — pick something"), right-x), m.th.dim, 0.85, false)
		return
	}
	tk := m.curTrack()
	icon := "▶"
	if m.pause > 0.5 {
		icon = "❚❚"
	}
	f.text(x, y, icon, m.th.a1)
	time := fmtDur(m.pos) + " / " + fmtDur(tk.Dur)
	tw := len([]rune(time))
	bw := min(24, m.w/4)
	tx := right - tw
	bx := tx - 2 - bw
	label := x + len([]rune(icon)) + 1
	if bw < 6 || bx < label+8 {
		bw, bx = 0, 0
	}
	room := right - label
	if bw > 0 {
		room = bx - 2 - label
	}
	f.text(label, y, trunc(tk.Title+" — "+tk.Artist, max(0, room)), m.th.text)
	if bw <= 0 {
		return
	}
	frac := 0.0
	if tk.Dur > 0 {
		frac = min(1, m.pos/tk.Dur)
	}
	for i := range bw {
		r, c := '─', m.th.dim.mul(0.55)
		if float64(i)/float64(bw) < frac {
			r, c = '━', m.th.a1
		}
		f.put(bx+i, y, r, c)
	}
	f.text(tx, y, time, m.th.dim)
}
