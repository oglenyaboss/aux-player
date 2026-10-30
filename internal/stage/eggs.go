package stage

import (
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// egg is a touch on the player screen for one artist, album or track: a rule
// matches the artist, the album or the title, and the album and the release
// date come down from the Apple Music metadata.
type egg struct {
	knob    rune // progress knob, morphs in a few seconds into the track
	knobCol col

	cold   []string // producer tag as a cold open: the screen dims, the line types itself in sync with the voice
	credit string   // what the tag leaves behind next to the artist

	switchAt float64 // beat switch second: the mark on the bar breathes, then a hard cut to warm

	theme  *theme // album palette
	drips  bool   // title letters bleed, a little more all track long
	big    bool   // title one size up when it fits
	ragged bool   // title bottom edge torn
	grain  bool   // film grain on the backdrop

	jewel bool // a pink diamond set into the top of the cover
	tint  col  // backdrop leans toward this colour

	// hooks: a bespoke egg draws itself through these; nil changes nothing.
	layout func(m *Model, g playGeom) playGeom                                // the play layout from the window's g; never calls m.geom
	bg     func(m *Model, c col, x, y int) col                                // one backdrop cell of the player
	cover  func(m *Model, at func(px, py int) col, px, py int) col            // one cover pixel; at samples the layer below
	ink    func(m *Model, c col, u, v float64) col                            // one title pixel; u, v run 0..1 across the title
	bar    func(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) // one progress-bar cell
	over   func(m *Model, f *fb, g playGeom)                                  // on top of the player, under the cold open
	step   func(m *Model)                                                     // every frame of the track
}

const eggKnobAt = 6.0

// dripMax is how far below the title the drips may run, in title pixels; the artist line sits right under it.
const dripMax = 2

var wlr = theme{
	bg:   hex(0x090909),
	a1:   hex(0xe3242b),
	a2:   hex(0xb5161d),
	a3:   hex(0xe3242b),
	text: hex(0xf2f2f2),
	dim:  hex(0x7a7a7a),
	bad:  hex(0xe3242b),
}

// eggRule hangs an egg on the artist's tracks, narrowed to one album or one title when set.
type eggRule struct {
	artist, album, title string
	e                    egg
}

var eggRules = []eggRule{
	{artist: "Lil Uzi Vert", e: egg{knob: '◆', knobCol: hex(0xff5fae), jewel: true, tint: hex(0xff5fae)}},
	//nolint:gosec // G101: the egg fields are song titles and a producer credit, not credentials
	{artist: "Playboi Carti", title: "Magnolia", e: egg{
		cold:   []string{"YO PI'ERRE,", "YOU WANNA COME OUT HERE?"},
		credit: "prod. pi'erre bourne",
	}},
	{artist: "Frank Ocean", title: "Nights", e: egg{switchAt: 210}},
	{artist: "Playboi Carti", album: "Whole Lotta Red", e: egg{
		theme: &wlr, drips: true, big: true, ragged: true, grain: true, knob: '†', knobCol: hex(0xf2f2f2)}},
}

// inText: the cell holds a letter or a space between letters, so an egg that paints
// the backdrop pixel by pixel leaves it to the text.
func inText(f *fb, x, y int) bool {
	glyph := func(c *cell) bool { return c != nil && !c.pix && c.ch != ' ' }
	return glyph(f.at(x, y)) || glyph(f.at(x-1, y)) || glyph(f.at(x+1, y))
}

func (m *Model) eggOf(t Track) *egg {
	if m.o.NoEggs {
		return nil
	}
	k := t.Key()
	if e, ok := m.eggMemo[k]; ok {
		return e
	}
	e := eggRuleFor(t)
	m.eggMemo[k] = e
	return e
}

// eggRuleFor runs the rules once per track and folds every match into one egg,
// in layers: the naked artist at the bottom, then the album era, then title rules on top.
// One album match drops the naked-artiest layer whole: the album replaces the artist image.
func eggRuleFor(t Track) *egg {
	var arts, albums, titles []*eggRule
	for i := range eggRules {
		r := &eggRules[i]
		if !r.matches(t) {
			continue
		}
		switch ruleScore(r) {
		case 3:
			titles = append(titles, r)
		case 2:
			albums = append(albums, r)
		default:
			arts = append(arts, r)
		}
	}
	if len(albums) > 0 {
		arts = nil
	}
	if len(arts)+len(albums)+len(titles) == 0 {
		return nil
	}
	var e egg
	for _, ls := range [][]*eggRule{arts, albums, titles} {
		for _, r := range ls {
			e = merge(e, r.e)
		}
	}
	return &e
}

// merge folds the egg of the upper layer b over the lower one a: bespoke fields
// come from b whole, boolean looks add up, and the hooks chain, lower first.
func merge(a, b egg) egg {
	e := a
	if b.knob != 0 {
		e.knob, e.knobCol = b.knob, b.knobCol
	}
	if b.cold != nil || b.credit != "" {
		e.cold, e.credit = b.cold, b.credit
	}
	if b.switchAt != 0 {
		e.switchAt = b.switchAt
	}
	if b.theme != nil {
		e.theme = b.theme
	}
	e.drips = e.drips || b.drips
	e.big = e.big || b.big
	e.ragged = e.ragged || b.ragged
	e.grain = e.grain || b.grain
	e.jewel = e.jewel || b.jewel
	if b.tint != (col{}) {
		e.tint = b.tint
	}
	switch {
	case b.layout == nil:
	case a.layout == nil:
		e.layout = b.layout
	default:
		lo, hi := a.layout, b.layout
		e.layout = func(m *Model, g playGeom) playGeom { return hi(m, lo(m, g)) }
	}
	switch {
	case b.bg == nil:
	case a.bg == nil:
		e.bg = b.bg
	default:
		lo, hi := a.bg, b.bg
		e.bg = func(m *Model, c col, x, y int) col { return hi(m, lo(m, c, x, y), x, y) }
	}
	switch {
	case b.cover == nil:
	case a.cover == nil:
		e.cover = b.cover
	default:
		lo, hi := a.cover, b.cover
		e.cover = func(m *Model, at func(px, py int) col, px, py int) col {
			return hi(m, func(x, y int) col { return lo(m, at, x, y) }, px, py)
		}
	}
	switch {
	case b.ink == nil:
	case a.ink == nil:
		e.ink = b.ink
	default:
		lo, hi := a.ink, b.ink
		e.ink = func(m *Model, c col, u, v float64) col { return hi(m, lo(m, c, u, v), u, v) }
	}
	switch {
	case b.bar == nil:
	case a.bar == nil:
		e.bar = b.bar
	default:
		lo, hi := a.bar, b.bar
		e.bar = func(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) {
			r, c = lo(m, i, bw, frac, r, c)
			return hi(m, i, bw, frac, r, c)
		}
	}
	switch {
	case b.over == nil:
	case a.over == nil:
		e.over = b.over
	default:
		lo, hi := a.over, b.over
		e.over = func(m *Model, f *fb, g playGeom) { lo(m, f, g); hi(m, f, g) }
	}
	switch {
	case b.step == nil:
	case a.step == nil:
		e.step = b.step
	default:
		lo, hi := a.step, b.step
		e.step = func(m *Model) { lo(m); hi(m) }
	}
	return e
}

// ruleScore: a named title wins, then a named album, then the bare artist.
func ruleScore(r *eggRule) int {
	switch {
	case r.title != "":
		return 3
	case r.album != "":
		return 2
	default:
		return 1
	}
}

// matches: the right artist, and the album and the title each when the rule names one.
func (r *eggRule) matches(t Track) bool {
	if !artistFold(r.artist, t.Artist) {
		return false
	}
	if r.album != "" && foldAlbum(t.Album) != foldAlbum(r.album) {
		return false
	}
	if r.title != "" && foldTitle(t.Title) != foldTitle(r.title) {
		return false
	}
	return true
}

// artistFold: equal folds, or the rule's artist one of the track artist's ", " and " & " parts.
// Substrings do not count: Drake is not Nick Drake.
func artistFold(want, have string) bool {
	w, h := fold(want), fold(have)
	if w == h {
		return true
	}
	h = strings.ReplaceAll(h, ", ", "\x00")
	h = strings.ReplaceAll(h, " & ", "\x00")
	return slices.Contains(strings.Split(h, "\x00"), w)
}

// fold lowers the case and unwinds the typographic apostrophe.
func fold(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ToLower(s), "’", "'"))
}

// foldAlbum folds the album and cuts the edition marker: "Whole Lotta Red (Deluxe)"
// and "X - Single" fold to the bare album.
func foldAlbum(s string) string {
	s = fold(s)
	cut := -1
	for _, sep := range []string{" (", " [", " - "} {
		if i := strings.Index(s, sep); i >= 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	if cut < 0 {
		return s
	}
	return strings.TrimSpace(s[:cut])
}

// foldTitle folds the title and strips only the bracket with the feature:
// "(feat. …)" and "[with …]" go away, other brackets stay — live and remix
// versions are other songs.
func foldTitle(s string) string {
	return fold(stripFeat(s))
}

func stripFeat(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		cl := ""
		switch r {
		case '(':
			cl = ")"
		case '[':
			cl = "]"
		}
		if cl == "" {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		end := strings.Index(s[i:], cl)
		if end < 0 {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		if inner := fold(s[i+1 : i+end]); strings.HasPrefix(inner, "feat.") || strings.HasPrefix(inner, "with ") {
			i += end + 1
			continue
		}
		b.WriteString(s[i : i+end+1])
		i += end + 1
	}
	return b.String()
}

var warmLight = col{0.36, 0.15, 0.04}

// eggWarmth: after the beat switch a warm glow rises from the bottom of the screen; v is 0 at the top.
func (m *Model) eggWarmth(v float64) float64 {
	e := m.eggOf(*m.curTrack())
	if e == nil || e.switchAt == 0 || m.pos < e.switchAt {
		return 0
	}
	k := prog(m.pos, e.switchAt, 2.2)
	return 0.85 * smooth((k*1.4-(1-v))*2.5)
}

// eggBackdrop shifts one backdrop cell for the current egg.
func (m *Model) eggBackdrop(bg col, x, y int, v float64) col {
	e := m.eggOf(*m.curTrack())
	if e == nil {
		return bg
	}
	if e.bg != nil {
		bg = e.bg(m, bg, x, y)
	}
	if e.tint != (col{}) {
		bg = bg.lerp(e.tint.mul(0.22), 0.45*easeInOut(prog(m.trackT, eggKnobAt, 2.5)))
	}
	if e.grain {
		bg = bg.mul(1 + 0.16*(hash(x, y, int(m.t*8))-0.5))
	}
	if w := m.eggWarmth(v); w > 0 {
		bg = bg.lerp(warmLight, w*(0.4+0.6*v))
	}
	return bg
}

// eggCover grades a cover pixel: after the beat switch the cover turns warm, bottom first.
func (m *Model) eggCover(c col, v float64) col {
	if w := m.eggWarmth(v); w > 0 {
		duo := hex(0x1e0a04).lerp(hex(0xffc58f), smooth(c.lum()*1.2))
		c = c.lerp(duo, 0.6*w/0.85)
	}
	return c
}

// titleT is the clock of the title reveal: after the beat switch the title is written anew.
func (m *Model) titleT() float64 {
	if e := m.eggOf(*m.curTrack()); e != nil && e.switchAt > 0 && m.pos >= e.switchAt {
		return math.Min(m.trackT, m.pos-e.switchAt)
	}
	return m.trackT
}

func (m *Model) eggStep() {
	e := m.eggOf(*m.curTrack())
	if e != nil && e.step != nil {
		e.step(m)
	}
	if e != nil && e.switchAt > 0 && m.pos >= e.switchAt && !m.eggDone {
		m.eggDone = true
		m.to.a1, m.to.a3, m.to.a2 = hex(0xffa25e), hex(0xffa25e), hex(0xffd9b8)
		m.to.bg = hex(0x140904)
		m.th = m.to
		m.bloom = 0.35
	}
}

// eggMark is the colour of the beat-switch mark: it starts breathing five seconds before the switch.
func (m *Model) eggMark(e *egg) col {
	k := prog(m.pos, e.switchAt-5, 5)
	return m.th.dim.lerp(m.th.a2.lerp(white, 0.4), k*(0.55+0.45*math.Sin(m.t*5)))
}

// eggOverlay draws what sits on top of the whole player.
func (m *Model) eggOverlay(f *fb, g playGeom) {
	e := m.eggOf(*m.curTrack())
	if e == nil {
		return
	}
	if e.over != nil {
		e.over(m, f, g)
	}
	if e.jewel {
		m.jewel(f, g)
	}
	if e.cold != nil {
		m.coldOpen(f, e)
	}
}

// coldOpen dims the player and types the producer tag across the middle of the screen in sync with the voice.
func (m *Model) coldOpen(f *fb, e *egg) {
	env := smooth(prog(m.pos, 0, 0.3)) * (1 - smooth(prog(m.pos, 3.6, 0.9)))
	if env <= 0 {
		return
	}
	k := 1 - 0.8*env
	for i := range f.c {
		f.c[i].bg, f.c[i].fg = f.c[i].bg.mul(k), f.c[i].fg.mul(k)
	}
	widest := 0
	for _, l := range e.cold {
		widest = max(widest, pixWidth(l))
	}
	sc := 2
	if widest*sc > f.w-8 {
		sc = 1
	}
	total := 0
	for _, l := range e.cold {
		total += len([]rune(l))
	}
	typed := float64(total) * prog(m.pos, 0.15, 2.2)
	lh := 5*sc + 2*sc
	py := (2*f.h - len(e.cold)*lh) / 2
	py -= py % 2
	n := 0.0
	c := m.th.text.lerp(m.th.a2, 0.25)
	for _, l := range e.cold {
		px := (f.w - pixWidth(l)*sc) / 2
		for _, r := range l {
			a := clamp01(typed-n) * env
			n++
			g, ok := font3[r]
			if !ok {
				px += 2 * sc
				continue
			}
			for gy := 0; gy < 5 && a > 0; gy++ {
				for gx := 0; gx < len(g[0]); gx++ {
					if g[gy][gx] != '#' {
						continue
					}
					for sy := 0; sy < sc; sy++ {
						for sx := 0; sx < sc; sx++ {
							x, y := px+gx*sc+sx, py+gy*sc+sy
							if hash(x, y, 11) < a {
								f.pixel(x, y, c)
							}
						}
					}
				}
			}
			px += (len(g[0]) + 1) * sc
		}
		py += lh
	}
}

func pixWidth(s string) int {
	w := -1
	for _, r := range strings.ToUpper(s) {
		if g, ok := font3[r]; ok {
			w += len(g[0]) + 1
		} else {
			w += 2
		}
	}
	return max(0, w)
}

// jewel sets a cut pink diamond into the top of the cover, Uzi's forehead stone; it catches the light now and then.
func (m *Model) jewel(f *fb, g playGeom) {
	a := easeOut(prog(m.trackT, eggKnobAt, 0.8))
	if a <= 0 || g.cw < 16 {
		return
	}
	rows := gemRows
	cx, cy := g.cx+g.cw/2, g.cy*2+2
	base, light, shade, edge := hex(0xff5fae), hex(0xffd3ea), hex(0xa02a66), hex(0x2a0716)
	glint := math.Mod(m.trackT-eggKnobAt, 12)
	in := func(x, y int) bool {
		if y < 0 || y >= len(rows) {
			return false
		}
		h := rows[y]
		return x >= -h && x <= h
	}
	for y := -1; y <= len(rows); y++ {
		for x := -6; x <= 6; x++ {
			if hash(x, y, 5) > a {
				continue
			}
			if !in(x, y) {
				if in(x-1, y) || in(x+1, y) || in(x, y-1) || in(x, y+1) {
					f.pixel(cx+x, cy+y, f.pixelAt(cx+x, cy+y).lerp(edge, 0.8))
				}
				continue
			}
			c := base
			switch {
			case y == 0:
				c = base.lerp(light, 0.7)
			case y == 1 && x < 0:
				c = base.lerp(light, 0.45)
			case y == 1:
				c = base.lerp(light, 0.2)
			case y == 2:
				c = base.lerp(shade, 0.15)
			case x < 0:
				c = base.lerp(light, 0.15)
			case x > 0:
				c = base.lerp(shade, 0.5)
			default:
				c = base.lerp(shade, 0.25)
			}
			d := float64(x+y) - (glint*14 - 6)
			c = c.lerp(white, 0.85*math.Exp(-d*d/1.5))
			f.pixel(cx+x, cy+y, c)
		}
	}
}

// gemRows is the diamond silhouette: half-width of each pixel row, table on top, pavilion to a point.
var gemRows = []int{2, 3, 4, 3, 2, 1, 0}

// drawTag leaves the producer credit next to the artist once the cold open is over.
func (m *Model) drawTag(f *fb, x, y, w int, e *egg) {
	if w < 8 {
		return
	}
	if ca := prog(m.pos, 3.9, 0.8) * 0.7; ca > 0 {
		f.fade(x, y, trunc(e.credit, w), m.th.a2.lerp(m.th.dim, 0.4), ca, false)
	}
}

// drip lets letters of the title bleed downward, quickly at first, then a little more all track long.
func (m *Model) drip(f *fb, x, y int, bottom string, sc, seed int) {
	for gx, b := range bottom {
		if b != '#' || hash(gx, 7, seed) > 0.45 {
			continue
		}
		h := hash(gx, 9, seed)
		grow := easeOut(prog(m.trackT, 1.2+1.2*h, 3.5))*(0.8+0.8*h) + math.Min(1, m.trackT/120)*0.4*h
		l := min(int(math.Round(float64(sc)*grow)), dripMax*sc-1)
		px := x + gx*sc + sc/2
		for i := range l {
			f.pixel(px, y+i, m.th.a1.mul(0.9-0.3*float64(i)/float64(3*sc)))
		}
		if l > 0 {
			f.pixel(px, y+l, m.th.a1.lerp(white, 0.2))
		}
	}
}
