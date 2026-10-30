package stage

import (
	"image"
	"math"
	"strings"
)

func init() {
	eggRules = append(eggRules, eggRule{artist: "Kanye West", album: "Yeezus", e: egg{
		theme: &yeezusTheme, bg: yeezusBg, cover: yeezusCover, ink: yeezusInk,
		bar: yeezusBar, layout: yeezusLayout, over: yeezusOver,
		knob: '■', knobCol: hex(0xe3001b)}})
}

var yeezusTheme = theme{
	bg:   hex(0xffffff),
	a1:   hex(0xe3001b),
	a2:   hex(0x8a8a8a),
	a3:   hex(0x111111),
	text: hex(0x111111),
	dim:  hex(0x6e6e6e),
	bad:  hex(0xe3001b),
}

var (
	yeezusPaper = hex(0xffffff)
	yeezusInkC  = hex(0x111111)
	yeezusTapeC = hex(0xfe0702)
	yeezusFold  = hex(0xff2a1c)
)

// The Apple cover is the jewel case: the disc centre and radius and the box of the red
// tape on its right edge, as fractions of the cover.
const (
	yeezusDiscU, yeezusDiscV, yeezusDiscR = 0.52, 0.502, 0.393
	yeezusTapeU0, yeezusTapeU1            = 0.69, 0.97
	yeezusTapeV0, yeezusTapeV1            = 0.37, 0.65
)

// yeezusBox is where the case, the tape and the title sit, in pixels unless noted.
type yeezusBox struct {
	cx, cy, cs     int // the case: top left and side
	lines          [][]rune
	sc, x0, y0     int // title pixel scale and its top left
	tapeX0, tapeX1 int // the tape at the start of the track, columns
	tapeY0, tapeY1 int
	textY, maxCols int // the artist row
}

// yeezusPlan puts the case on the left at the full height of the window, the title
// right of it above the tape, artist and time under the tape. ok is false when the
// title has a letter outside font3 or the window is too narrow.
func yeezusPlan(m *Model) (b yeezusBox, ok bool) {
	var words [][]rune
	for w := range strings.FieldsSeq(strings.ToUpper(m.curTrack().Title)) {
		rs := []rune(w)
		for _, r := range rs {
			if _, ok := font3[r]; !ok {
				return b, false
			}
		}
		words = append(words, rs)
	}
	if len(words) == 0 {
		return b, false
	}
	for len(words) > 1 {
		best, short := 0, false
		for i := 0; i < len(words)-1; i++ {
			if len(words[i]) <= 2 || len(words[i+1]) <= 2 {
				best, short = i, true
				break
			}
			if pixWidth3(words[i])+pixWidth3(words[i+1]) < pixWidth3(words[best])+pixWidth3(words[best+1]) {
				best = i
			}
		}
		if !short && len(words) <= 3 {
			break
		}
		joined := append(append(append([]rune{}, words[best]...), ' '), words[best+1]...)
		words = append(append(words[:best:best], joined), words[best+2:]...)
	}
	for _, w := range words {
		b.maxCols = max(b.maxCols, pixWidth3(w))
	}
	b.cs = 2*m.h - 8
	b.cx, b.cy = 3, 3
	b.tapeX0 = b.cx + int(yeezusTapeU0*float64(b.cs))
	b.tapeX1 = b.cx + int(yeezusTapeU1*float64(b.cs))
	b.tapeY0 = b.cy + int(yeezusTapeV0*float64(b.cs))
	b.tapeY1 = b.cy + int(yeezusTapeV1*float64(b.cs))
	b.lines, b.x0, b.y0 = words, b.cx+b.cs+4, 4
	n := len(words)
	b.sc = min((b.tapeY0-2-b.y0)/(n*6-1), (m.w-b.x0)*11/10/b.maxCols)
	b.textY = (b.tapeY1+1)/2 + 2
	return b, b.sc >= 1 && m.w-b.x0 >= 24
}

func pixWidth3(rs []rune) int {
	w := -1
	for _, r := range rs {
		w += len(font3[r][0]) + 1
	}
	return w
}

// yeezusLayout opens the empty jewel case: no cover and no queue, the egg draws the
// whole screen itself. Narrow and portrait windows keep the usual player.
func yeezusLayout(m *Model, g playGeom) playGeom {
	if !g.side || g.mini || g.split {
		return g
	}
	if _, ok := yeezusPlan(m); !ok {
		return g
	}
	g.noCover, g.noQueue = true, true
	return g
}

// yeezusBg: the plastic of the case, flat white.
func yeezusBg(m *Model, c col, x, y int) col { return yeezusPaper }

// yeezusSpin is the disc angle: it spins up like a drive in the first second, then
// turns with the playback, so it stands still on pause.
func yeezusSpin(m *Model) float64 {
	return 2*math.Pi/8*m.pos + 10*easeOut(prog(m.trackT, 0, 1.2))
}

// yeezusCase draws the real cover big as the case: the disc in it turns, and where
// the turn would bring the tape of the cover onto the disc the opposite side shows;
// under the tape the edge of the case repeats the row just above it.
func yeezusCase(m *Model, f *fb, b yeezusBox) {
	cg := m.cover(m.curTrack().Key(), b.cs, b.cs)
	s := yeezusSpin(m)
	n := float64(b.cs)
	for py := b.cy; py < b.cy+b.cs; py++ {
		for px := b.cx; px < b.cx+b.cs; px++ {
			u, v := (float64(px-b.cx)+0.5)/n, (float64(py-b.cy)+0.5)/n
			du, dv := u-yeezusDiscU, v-yeezusDiscV
			switch d := math.Hypot(du, dv); {
			case d <= yeezusDiscR:
				a := math.Atan2(dv, du) - s
				du, dv = d*math.Cos(a), d*math.Sin(a)
				u, v = yeezusDiscU+du, yeezusDiscV+dv
				if yeezusUnderTape(u, v) {
					u, v = yeezusDiscU-du, yeezusDiscV-dv
				}
			case yeezusUnderTape(u, v):
				v = yeezusTapeV0 - 0.04
			}
			f.pixel(px, py, cg.at(int(u*n), int(v*n)))
		}
	}
}

func yeezusUnderTape(u, v float64) bool {
	return u >= yeezusTapeU0-0.025 && v >= yeezusTapeV0-0.025 && v <= yeezusTapeV1+0.025
}

// yeezusOver draws the screen: the case with the turning disc on the left, the
// giant black title right of it, the red tape pulled from the case to the screen
// edge as the progress, artist and time under it.
func yeezusOver(m *Model, f *fb, g playGeom) {
	if !g.noCover {
		return
	}
	b, _ := yeezusPlan(m)
	if m.trackT < 0.12 {
		f.fill(0, 0, m.w, m.h, col{})
		return
	}
	rows := m.h
	if m.typing {
		rows = m.h - 4
	}
	f.fill(0, 0, m.w, rows, yeezusPaper)
	yeezusCase(m, f, b)

	for i, line := range b.lines {
		gx := 0
		for _, r := range line {
			gl := font3[r]
			for gy := range 5 {
				for x := 0; x < len(gl[0]); x++ {
					if gl[gy][x] != '#' {
						continue
					}
					for sy := 0; sy < b.sc; sy++ {
						for sx := 0; sx < b.sc; sx++ {
							f.pixel(b.x0+(gx+x)*b.sc+sx, b.y0+(i*6+gy)*b.sc+sy, yeezusInkC)
						}
					}
				}
			}
			gx += len(gl[0]) + 1
		}
	}
	m.titleBox = image.Rect(b.x0, b.y0, b.x0+b.maxCols*b.sc, b.y0+(len(b.lines)*6-1)*b.sc)

	tk := m.curTrack()
	frac := 0.0
	if tk.Dur > 0 {
		frac = min(1, m.pos/tk.Dur)
	}
	x1 := b.tapeX1 + int(frac*float64(m.w-b.tapeX1))
	for py := b.tapeY0; py <= b.tapeY1; py++ {
		for x := b.tapeX0; x < x1; x++ {
			if py == b.tapeY1 && x < b.tapeX0+2 || py == b.tapeY1-1 && x == b.tapeX0 {
				continue
			}
			c := yeezusTapeC
			if x == b.cx+b.cs-2 {
				c = yeezusFold
			}
			f.pixel(x, py, c)
		}
	}
	m.barBox = image.Rect(b.tapeX1, b.tapeY0/2, m.w, b.tapeY1/2+1)

	f.textS(b.x0, b.textY, trunc(tk.Artist, m.w-b.x0-2), yeezusInkC, true, false)
	f.text(b.x0, b.textY+1, fmtDur(m.pos)+" / "+fmtDur(tk.Dur), yeezusInkC)
	if !m.typing {
		help := tr("? клавиши", "? keys")
		f.text(3, m.h-2, tr("/ сказать агенту", "/ talk to the agent"), yeezusInkC)
		f.text(m.w-3-len([]rune(help)), m.h-2, help, yeezusInkC)
	}
}

// yeezusCover turns the glare on the disc once a minute, like the record is spinning;
// the red tape on the right edge and the sleeve around the disc stay still. Only the
// usual player of narrow windows shows the cover.
func yeezusCover(m *Model, at func(px, py int) col, px, py int) col {
	g := m.geom()
	if g.cw <= 0 || g.ch <= 0 {
		return at(px, py)
	}
	u := (float64(px) + 0.5) / float64(g.cw)
	v := (float64(py) + 0.5) / float64(g.ch*2)
	c := at(px, py)
	if u >= 0.69 && u <= 0.97 && v >= 0.37 && v <= 0.65 {
		return c
	}
	if math.Hypot(u-0.45, v-0.5) > 0.43 {
		return c
	}
	h, s, val := toHSV(c)
	if s <= 0.2 {
		return c
	}
	return hsv(h+360*math.Max(0, m.trackT-6)/60, s, val)
}

// yeezusInk paints the title black, no gradient and no sweep.
func yeezusInk(m *Model, c col, u, v float64) col { return hex(0x111111) }

// yeezusBar: the played part is the red tape, the rest the grey fold of the sleeve.
func yeezusBar(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) {
	switch {
	case bw > 0 && float64(i)/float64(bw) < frac:
		return '█', hex(0xe3001b)
	case r == '─':
		return '─', hex(0x9a9a9a)
	}
	return r, c
}
