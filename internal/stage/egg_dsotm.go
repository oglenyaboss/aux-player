package stage

import (
	"math"
)

// Pink Floyd, The Dark Side of the Moon: light crosses the player. A white beam
// reaches the cover from the left edge of the screen, the spectrum leaves the
// cover and becomes the progress bar — a six-band rainbow with the prism riding
// its head. The prism beats with the heart on Speak to Me, all track long, and
// on Eclipse, in the last thirty seconds.

var dsotmTheme = theme{
	bg:   hex(0x000000),
	a1:   hex(0xf5f5f5),
	a2:   hex(0xbdbdbd),
	a3:   hex(0xffffff),
	text: hex(0xf5f5f5),
	dim:  hex(0x666666),
	bad:  hex(0xff5c7a),
}

// dsotmSpectrum is the prism split, top to bottom: red through violet.
var dsotmSpectrum = []col{
	hex(0xe8222f),
	hex(0xf26724),
	hex(0xffcd14),
	hex(0x30b252),
	hex(0x2aa8b6),
	hex(0x7862a0),
}

// dsotmPrism is the 5x6 silhouette on the ribbon head: # is the white edge, o the dark inside.
var dsotmPrism = []string{
	"..#..",
	".#o#.",
	".#o#.",
	"#ooo#",
	"#ooo#",
	"#####",
}

func init() {
	eggRules = append(eggRules,
		eggRule{artist: "Pink Floyd", album: "The Dark Side of the Moon", e: egg{
			theme: &dsotmTheme, bg: dsotmBg, ink: dsotmInk, over: dsotmOver}},
		eggRule{artist: "Pink Floyd", title: "Speak to Me", e: egg{over: dsotmSpeakOver}},
		eggRule{artist: "Pink Floyd", title: "Eclipse", e: egg{over: dsotmEclipseOver}},
	)
}

// dsotmBg blacks the field out; dsotmInk washes the title to near-white light.
func dsotmBg(m *Model, c col, x, y int) col { return c.mul(0.12) }

func dsotmInk(m *Model, c col, u, v float64) col { return c.lerp(hex(0xf5f5f5), 0.85) }

// dsotmFrac is the played share of the track: 0 when the length is unknown.
func dsotmFrac(m *Model) float64 {
	if d := m.curTrack().Dur; d > 0 {
		return math.Min(1, m.pos/d)
	}
	return 0
}

// dsotmRibbonOn: the ribbon, and the prism on its head, run after 7.4 s and never in the mini layout.
func dsotmRibbonOn(m *Model, g playGeom) bool {
	return !g.mini && m.trackT >= 7.4 && !m.barBox.Empty() && m.barBox.Dx() > 0
}

// dsotmOver draws the beam, the cover-to-bar bridge and the ribbon in that order.
func dsotmOver(m *Model, f *fb, g playGeom) {
	dsotmBeam(m, f, g)
	dsotmBridge(m, f, g)
	dsotmRibbon(m, f, g)
}

// dsotmBeam: one white beam runs from the left edge of the screen into the cover,
// its head crawling in at the reveal; the rows next to it catch a quarter of it.
func dsotmBeam(m *Model, f *fb, g playGeom) {
	hx := int(easeOut(prog(m.trackT, eggKnobAt, 0.8)) * float64(g.cx))
	if hx <= 0 {
		return
	}
	by := g.cy*2 + int(0.49*float64(g.ch*2))
	light := hex(0xfdfdff)
	for x := range hx {
		f.pixel(x, by, light)
		f.pixel(x, by-1, f.pixelAt(x, by-1).lerp(light, 0.25))
		f.pixel(x, by+1, f.pixelAt(x, by+1).lerp(light, 0.25))
	}
}

// dsotmBridge: the six bands leave the cover on the right and fall into the bar line.
func dsotmBridge(m *Model, f *fb, g playGeom) {
	if g.mini || m.barBox.Empty() {
		return
	}
	x0, x1 := g.cx+g.cw, m.barBox.Min.X
	if x1 <= x0+1 {
		return
	}
	n := x1 - x0
	head := int(easeOut(prog(m.trackT, 6.8, 0.6)) * float64(n))
	if head <= 0 {
		return
	}
	for i := range dsotmSpectrum {
		ys := float64(g.cy*2) + (0.41+0.02*float64(i))*float64(g.ch*2)
		ye := float64((m.barBox.Min.Y-1)*2 + i)
		for dx := range head {
			x := x0 + dx
			t := (float64(dx) + 0.5) / float64(n)
			f.pixel(x, int(math.Round(lerpf(ys, ye, t))), dsotmSpectrum[i])
		}
	}
}

// dsotmRibbon: the bar becomes a rainbow — played columns hold six breathing
// bands, unplayed ones a thin dim line. The plain bar and the knob are erased first.
func dsotmRibbon(m *Model, f *fb, g playGeom) {
	if !dsotmRibbonOn(m, g) {
		return
	}
	bw := m.barBox.Dx()
	frac := dsotmFrac(m)
	b0 := (m.barBox.Min.Y - 1) * 2
	for row := m.barBox.Min.Y - 1; row <= m.barBox.Min.Y+1; row++ {
		f.blank(m.barBox.Min.X, row, bw)
	}
	rv := easeOut(prog(m.trackT, 7.4, 1.2)) * float64(bw)
	breath := 0.85 + 0.15*math.Sin(2*math.Pi*m.trackT/64)
	for i := 0; i < bw && float64(i) < rv; i++ {
		x := m.barBox.Min.X + i
		if float64(i)/float64(bw) < frac {
			for b, c := range dsotmSpectrum {
				f.pixel(x, b0+b, c.mul(breath))
			}
			continue
		}
		f.pixel(x, b0+2, hex(0x333333))
	}
	if rv > frac*float64(bw) {
		dsotmPrismDraw(m, f, m.barBox.Min.X+int(frac*float64(bw)), b0)
	}
}

// dsotmPrismDraw lays the silhouette on the ribbon head, mask column 2 on hx,
// mask row r on pixel row b0+r; cells carrying player text stay untouched.
func dsotmPrismDraw(m *Model, f *fb, hx, b0 int) {
	edge, dark := hex(0xf5f5f5), hex(0x050505)
	for r, row := range dsotmPrism {
		for c := 0; c < len(row); c++ {
			x, y := hx+c-2, b0+r
			if cel := f.at(x, y/2); cel != nil && cel.ch != ' ' && !cel.pix {
				continue
			}
			switch row[c] {
			case '#':
				f.pixel(x, y, edge)
			case 'o':
				f.pixel(x, y, dark)
			}
		}
	}
}

// dsotmBeat is the heart: a double strike, peaks at 0 and 0.28 of an 0.83 s cycle.
func dsotmBeat(phi float64) float64 {
	b := math.Exp(-phi / 0.12)
	if phi >= 0.28 {
		b += 0.8 * math.Exp(-(phi-0.28)/0.12)
	}
	return math.Min(1, b)
}

// dsotmGlow lights the halo around the prism on the beat; amp brings the track layer in.
func dsotmGlow(m *Model, f *fb, g playGeom, amp float64) {
	if amp <= 0 || !dsotmRibbonOn(m, g) {
		return
	}
	bw := m.barBox.Dx()
	frac := dsotmFrac(m)
	if rv := easeOut(prog(m.trackT, 7.4, 1.2)) * float64(bw); rv <= frac*float64(bw) {
		return
	}
	hx := m.barBox.Min.X + int(frac*float64(bw))
	b0 := (m.barBox.Min.Y - 1) * 2
	cy := float64(b0) + 2.5
	strength := 0.6 * amp * dsotmBeat(math.Mod(m.pos, 0.83))
	if strength <= 0 {
		return
	}
	for x := hx - 4; x <= hx+4; x++ {
		for y := b0 - 3; y <= b0+8; y++ {
			d := math.Hypot(float64(x-hx), float64(y)-cy)
			if d > 4.5 || dsotmInMask(x-hx, y-b0) {
				continue
			}
			if cel := f.at(x, y/2); cel != nil && (cel.ch == ' ' || cel.pix) {
				f.pixel(x, y, f.pixelAt(x, y).lerp(hex(0xf5f5f5), strength*math.Exp(-d*d/6)))
			}
		}
	}
}

// dsotmInMask: the offset (dx, dy) from the prism head falls on # or o of the silhouette.
func dsotmInMask(dx, dy int) bool {
	if dy < 0 || dy >= len(dsotmPrism) || dx < -2 || dx > 2 {
		return false
	}
	r := dsotmPrism[dy][dx+2]
	return r == '#' || r == 'o'
}

// dsotmSpeakOver: the prism beats the heart the whole track.
func dsotmSpeakOver(m *Model, f *fb, g playGeom) { dsotmGlow(m, f, g, 1) }

// dsotmEclipseOver: the heart shows in the last thirty seconds and fades into the end.
func dsotmEclipseOver(m *Model, f *fb, g playGeom) {
	d := m.curTrack().Dur
	if d == 0 {
		return
	}
	dsotmGlow(m, f, g, smooth(prog(m.pos, d-30, 3))*(1-smooth(prog(m.pos, d-4, 4))))
}
