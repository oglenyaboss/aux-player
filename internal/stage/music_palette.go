package stage

import (
	"fmt"
	"math"
)

// The palette: a Raycast-style panel that drops from the top and leaves the stage
// running behind it, dimmed. Wide panels carry the detail pane of the highlight.
const (
	musicPaletteW    = 124 // the widest the panel gets
	musicPaletteY    = 2   // where the settled panel starts
	musicPaletteEdge = 6   // the air on both sides
	musicDetailW     = 26  // the detail column, the 24-cell cover and its air
	musicDetailGap   = 2   // between the list and the detail
	musicPaletteH    = 34  // the settled height of the panel
	musicCoverX      = 24  // the detail cover, in cells
	musicCoverY      = 12  // half-block rows: 24 pixels tall
)

// mlPaletteW is the panel width at the current frame.
func (m *Model) mlPaletteW() int {
	if m.w <= 0 {
		return 0
	}
	w := min(musicPaletteW, m.w-musicPaletteEdge)
	if w < 20 && m.w >= 20 {
		w = 20
	}
	return max(1, min(w, m.w))
}

// mlDetailOn reports whether the panel has room for the detail pane.
func (m *Model) mlDetailOn() bool {
	return m.musicStyle() == musicStylePalette && m.mlPaletteW() >= 110
}

// mlListW is the width the rows take: the part the detail pane leaves free.
func (m *Model) mlListW(inner int) int {
	if !m.mlDetailOn() {
		return inner
	}
	return max(20, inner-musicDetailW-musicDetailGap)
}

// mlPaletteGeom is the panel box: one height for every level, capped by the frame.
func (m *Model) mlPaletteGeom() (x, y, w, h int) {
	w = m.mlPaletteW()
	x = max(0, (m.w-w)/2)
	h = min(musicPaletteH, max(6, m.h-5))
	if musicPaletteY+h > m.h {
		h = max(1, m.h-musicPaletteY)
	}
	return x, musicPaletteY, w, h
}

// drawMusicPalette slides the panel in over the stage: it starts two rows higher and
// fades in, and both reverse while it closes.
func (m *Model) drawMusicPalette(f *fb) {
	a := m.mlProgress()
	if a <= 0 {
		return
	}
	lines := m.mlLines()
	x, y, w, h := m.mlPaletteGeom()
	p := newFB(w, h)
	m.mlPanel(p, lines, m.mu.list.sel)
	y -= int(math.Round((1 - a) * 2))
	m.mlShadow(f, x, y, w, h, a)
	mlBlit(f, p, x, y, a)
}

// mlShadow darkens the one-cell band under and to the right of the panel.
func (m *Model) mlShadow(f *fb, x, y, w, h int, a float64) {
	k := 1 - 0.4*a
	for i := range w + 1 {
		if c := f.at(x+i, y+h); c != nil {
			c.bg = c.bg.mul(k)
			if c.ch != ' ' && c.ch != 0 || c.pix {
				c.fg = c.fg.mul(k)
			}
		}
	}
	for j := range h {
		if c := f.at(x+w, y+j); c != nil {
			c.bg = c.bg.mul(k)
			if c.ch != ' ' && c.ch != 0 || c.pix {
				c.fg = c.fg.mul(k)
			}
		}
	}
}

// mlPanel is the whole palette inside its own frame: the border, the query row, the
// list, the detail pane, the action bar and the open actions menu.
func (m *Model) mlPanel(f *fb, lines []mlLine, sel int) {
	w, h := f.w, f.h
	f.fill(0, 0, w, h, m.th.bg.lerp(m.th.text, 0.07))
	m.glowBox(f, 0, 0, w, h, 1)
	m.mlDrawInput(f, 1, 1, w-2)
	m.mlRule(f, 1, 2, w-2)
	inner := w - 2
	listW := m.mlListW(inner)
	m.mlDrawBody(f, 1, 3, listW, h-6, lines, sel)
	if m.mlDetailOn() {
		m.mlDrawDetail(f, 1+listW+musicDetailGap, 3, musicCoverX, h-6)
	}
	if h >= 6 {
		m.mlRule(f, 1, h-3, w-2)
		m.mlDrawBar(f, 1, h-2, w-2)
	}
	m.mlDrawMenu(f)
}

// mlVeil quiets the running stage behind the palette; the drawer leaves it bright.
func (m *Model) mlVeil(f *fb, a float64) {
	if a <= 0 {
		return
	}
	bg := m.th.bg
	for y := range f.h {
		for x := range f.w {
			c := f.at(x, y)
			if c == nil {
				continue
			}
			c.bg = c.bg.lerp(bg, a)
			if c.ch != ' ' && c.ch != 0 || c.pix {
				c.fg = c.fg.lerp(bg, a)
			}
		}
	}
}

// fadeRect quiets the cells of one rectangle toward a color.
func fadeRect(f *fb, x, y, w, h int, to col, a float64) {
	if a <= 0 {
		return
	}
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c := f.at(xx, yy)
			if c == nil {
				continue
			}
			c.bg = c.bg.lerp(to, a)
			if c.ch != ' ' && c.ch != 0 || c.pix {
				c.fg = c.fg.lerp(to, a)
			}
		}
	}
}

// mlBlit puts a panel drawn on its own frame over the stage, faded in by a.
func mlBlit(dst, src *fb, x, y int, a float64) {
	for j := range src.h {
		for i := range src.w {
			c := dst.at(x+i, y+j)
			if c == nil {
				continue
			}
			s := &src.c[j*src.w+i]
			c.bg = c.bg.lerp(s.bg, a)
			switch {
			case s.ch != 0 && s.ch != ' ':
				c.ch, c.fg, c.bold, c.ital, c.pix = s.ch, c.bg.lerp(s.fg, a), s.bold, s.ital, s.pix
			case a > 0.5:
				c.ch, c.pix, c.bold, c.ital = ' ', false, false, false
			case c.ch != 0 && c.ch != ' ':
				c.fg = c.fg.lerp(c.bg, a*2)
			}
		}
	}
}

// mlDetailItem is what the detail pane shows: the highlighted item in the music, the
// playing track in the agent mode, and the playing track when no row is a song.
func (m *Model) mlDetailItem() (MusicItem, string, bool) {
	if m.mu.mode == mlModeAgent {
		if m.ph == phPlay && len(m.queue) > 0 {
			return m.mlPlayingItem(), tr("сейчас играет", "now playing"), true
		}
		return MusicItem{}, "", false
	}
	if it, ok := m.musicPreviewItem(); ok {
		return it, "", true
	}
	if m.ph == phPlay && len(m.queue) > 0 {
		return m.mlPlayingItem(), tr("сейчас играет", "now playing"), true
	}
	return MusicItem{}, "", false
}

// mlPlayingItem is the track that plays now, as a row of the detail pane.
func (m *Model) mlPlayingItem() MusicItem {
	t := *m.curTrack()
	return MusicItem{ID: t.ID, Kind: MusicSong, Title: t.Title, Artist: t.Artist, Album: t.Album, Dur: t.Dur}
}

// mlNoteDetail records the cover change, so the detail pane crossfades on the frame clock.
func (m *Model) mlNoteDetail() {
	if !m.mu.open {
		m.mu.detKey, m.mu.detPrev = "", ""
		return
	}
	key := ""
	if it, _, ok := m.mlDetailItem(); ok {
		key = it.Key()
	}
	if key == m.mu.detKey {
		return
	}
	m.mu.detPrev, m.mu.detT = m.mu.detKey, m.t
	m.mu.detKey = key
}

// mlDetailFrame is the cover key to draw and the one it fades from over 0.15 s.
func (m *Model) mlDetailFrame() (key, prev string, k float64) {
	it, _, ok := m.mlDetailItem()
	if !ok {
		return "", "", 1
	}
	key = it.Key()
	if key == "" || key != m.mu.detKey {
		return key, "", 1
	}
	k = easeOut(prog(m.t-m.mu.detT, 0, 0.15))
	if m.mu.detPrev == "" || k >= 1 {
		return key, "", 1
	}
	return key, m.mu.detPrev, k
}

// mlDrawDetail paints the detail pane at (x, y): the caption of the playing track, the
// cover over its halo, then the title, the artist, the subtitle and the count or the
// length, the progress of the playing track and the tracks of the collection.
func (m *Model) mlDrawDetail(f *fb, x, y, w, h int) {
	if w < 8 || h < 2 {
		return
	}
	m.mlNoteDetail()
	it, caption, ok := m.mlDetailItem()
	cy := y
	if caption != "" {
		f.text(x, cy, trunc(caption, w), m.th.dim)
		cy++
	}
	rows := min(musicCoverY, y+h-cy)
	key, prev, k := m.mlDetailFrame()
	if key == "" {
		if rows > 1 {
			m.mlEmptyCover(f, x, cy, musicCoverX, rows)
			if m.mu.mode == mlModeAgent {
				msg := tr("тишина", "silence")
				f.text(x+max(0, (musicCoverX-len([]rune(msg)))/2), cy+rows/2, msg, m.th.dim)
			}
		}
		return
	}
	if rows > 0 {
		m.mlCoverGlow(f, x, cy, musicCoverX, rows, [4]int{1, 3, f.w - 1, f.h - 3})
		m.mlCoverFade(f, x, cy, rows, key, prev, k)
		if ok && m.eggOf(it.eggTrack()) != nil {
			m.mlCoverGlint(f, x, cy, rows)
		}
	}
	cy += rows + 1
	if !ok || cy >= y+h {
		return
	}
	reveal := prog(m.t-m.mu.detT, 0, 0.35)
	seed := int(hashStr(key) % 1000)
	mark, mc, egg := m.mlEggMark(it)
	tw := w
	if egg {
		tw -= 2
	}
	title := wrapWords(it.Title, tw, 2)
	for i, line := range title {
		if cy >= y+h {
			return
		}
		f.textS(x, cy, mlReveal(scramble(line, reveal, m.t, seed)), m.th.text, true, false)
		if egg && i == len(title)-1 && reveal >= 1 {
			f.put(x+len([]rune(line))+1, cy, mark, mc)
		}
		cy++
	}
	if it.Artist != "" && cy < y+h {
		f.text(x, cy, trunc(it.Artist, w), m.th.dim)
		cy++
	}
	if it.Sub != "" && cy < y+h {
		f.text(x, cy, trunc(it.Sub, w), m.th.dim)
		cy++
	}
	if meta := mlDetailMeta(it); meta != "" && cy < y+h {
		f.text(x, cy, trunc(meta, w), m.th.dim)
		cy++
	}
	if caption != "" && it.Dur > 0 {
		cy = m.mlProgressLine(f, x, cy, w, y+h, it)
	}
	if it.Kind != MusicSong && (m.mu.coll == nil || m.mu.list.query != "") {
		if tracks := m.mu.peek[it.Key()]; len(tracks) > 0 {
			cy++
			for i, t := range tracks {
				if cy >= y+h {
					return
				}
				n := fmt.Sprintf("%02d", i+1)
				f.text(x, cy, n, m.th.dim)
				if room := w - len(n) - 1; room > 0 {
					f.text(x+len(n)+1, cy, trunc(t.Title, room), m.th.text.lerp(m.th.dim, 0.35))
				}
				cy++
			}
		}
	}
}

// mlReveal is the scramble of a title with the hidden runes drawn as blanks.
func mlReveal(rs []rune) string {
	out := make([]rune, len(rs))
	for i, r := range rs {
		if r == 0 {
			r = ' '
		}
		out[i] = r
	}
	return string(out)
}

// mlProgressLine draws how far the playing track has come: the played edge of the
// pane in the accent, the rest quiet, the two times under it.
func (m *Model) mlProgressLine(f *fb, x, y, w, limit int, it MusicItem) int {
	if y >= limit {
		return y
	}
	done := min(int(math.Ceil(clamp01(m.pos/it.Dur)*float64(w))), w)
	for i := range w {
		if i < done {
			f.put(x+i, y, '━', m.th.a1)
			continue
		}
		f.put(x+i, y, '─', m.th.dim.mul(0.5))
	}
	y++
	if y < limit {
		f.text(x, y, fmtDur(m.pos)+" / "+fmtDur(it.Dur), m.th.dim)
		y++
	}
	return y
}

// mlCoverGlow lights the panel around the detail cover in the cover's accent, fading
// to nothing at the edge of an ellipse; clip bounds the cells it may touch.
func (m *Model) mlCoverGlow(f *fb, cx, cy, cw, ch int, clip [4]int) {
	const rx, ry = 12.0, 2.0
	breathe := 0.9 + 0.1*math.Sin(m.t*1.6)
	for y := cy - int(ry); y < cy+ch+int(ry); y++ {
		for x := cx - int(rx); x < cx+cw+int(rx); x++ {
			if x < clip[0] || x >= clip[2] || y < clip[1] || y >= clip[3] {
				continue
			}
			if x >= cx && x < cx+cw && y >= cy && y < cy+ch {
				continue
			}
			nx := math.Max(float64(cx), math.Min(float64(cx+cw-1), float64(x)))
			ny := math.Max(float64(cy), math.Min(float64(cy+ch-1), float64(y)))
			d := math.Hypot((float64(x)-nx)/rx, (float64(y)-ny)/ry)
			if d >= 1 {
				continue
			}
			mid := float64(cy) + float64(ch-1)/2
			a := (1 - smooth(d)) * (1 - smooth(math.Abs(float64(y)-mid)/(float64(ch)/2+ry)))
			f.tint(x, y, m.th.a1, 0.22*a*a*breathe)
		}
	}
}

// mlCoverFade paints the 24×24 pixel cover, blending the previous one in on a change.
func (m *Model) mlCoverFade(f *fb, x, y, rows int, key, prev string, k float64) {
	g := m.cover(key, musicCoverX, musicCoverY*2)
	var pg *pixgrid
	if prev != "" && k < 1 {
		pg = m.cover(prev, musicCoverX, musicCoverY*2)
	}
	for py := range rows * 2 {
		for px := range musicCoverX {
			c := g.at(px, py)
			if pg != nil {
				c = pg.at(px, py).lerp(c, k)
			}
			f.pixel(x+px, y*2+py, c)
		}
	}
}

// mlEmptyCover is the dim frame that stands where the cover would be.
func (m *Model) mlEmptyCover(f *fb, x, y, w, h int) {
	c := m.th.dim.mul(0.45)
	for i := range w {
		f.put(x+i, y, '─', c)
		f.put(x+i, y+h-1, '─', c)
	}
	for j := range h {
		f.put(x, y+j, '│', c)
		f.put(x+w-1, y+j, '│', c)
	}
	f.put(x, y, '╭', c)
	f.put(x+w-1, y, '╮', c)
	f.put(x, y+h-1, '╰', c)
	f.put(x+w-1, y+h-1, '╯', c)
}

// mlDetailMeta is the last line of the detail: the length of a song, the size of a collection.
func mlDetailMeta(it MusicItem) string {
	switch it.Kind {
	case MusicSong:
		if it.Dur > 0 {
			return fmtDur(it.Dur)
		}
	case MusicAlbum, MusicPlaylist:
		s := ""
		if it.Count > 0 {
			s = countOf(it.Count, trackForms, trackFormsEn)
		}
		if it.Dur > 0 {
			if s != "" {
				s += " · "
			}
			s += fmtDur(it.Dur)
		}
		return s
	}
	return ""
}

// mlEggMark is the sign of an egg on a row: the egg's knob in its colour, or a star in
// its accent; it twinkles now and then and never sinks into the panel.
func (m *Model) mlEggMark(it MusicItem) (rune, col, bool) {
	e := m.eggOf(it.eggTrack())
	if e == nil {
		return 0, col{}, false
	}
	r, c := e.knob, e.knobCol
	if r == 0 {
		r, c = '✦', m.th.a1
		if e.theme != nil {
			c = e.theme.a1
		}
	}
	if a, b := c.lum()+0.05, m.th.bg.lerp(m.th.text, 0.07).lum()+0.05; math.Max(a, b)/math.Min(a, b) < 2.5 {
		c = m.th.text
	}
	tw := math.Max(0, math.Sin(m.t*1.7+float64(hashStr(it.Key())%628)/100))
	return r, c.lerp(white, 0.45*math.Pow(tw, 12)), true
}

// mlCoverGlint runs a band of light across the detail cover of an egg, once every few seconds.
func (m *Model) mlCoverGlint(f *fb, x, y, rows int) {
	const period, sweep = 4.5, 0.9
	p := math.Mod(m.t-m.mu.detT, period) - 0.6
	if p < 0 || p >= sweep {
		return
	}
	span := float64(musicCoverX + rows*2)
	c := -6 + (span+12)*easeInOut(p/sweep)
	for py := range rows * 2 {
		for px := range musicCoverX {
			d := math.Abs(float64(px+py) - c)
			if d >= 5 {
				continue
			}
			k := (1 - d/5) * (1 - d/5)
			f.pixel(x+px, y*2+py, f.pixelAt(x+px, y*2+py).lerp(white, 0.4*k))
		}
	}
}
