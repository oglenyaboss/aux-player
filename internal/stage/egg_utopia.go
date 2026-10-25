package stage

import "math"

// UTOPIA: the cover is Travis in a dark void, caught by one hard light from the side.
// The player goes dark the same way: it shows only where a slow light falls, the
// light switches on like a stage light at the start and sweeps with the playback,
// so a pause holds it still. The type is the silver of the belt on the cover.
func init() {
	eggRules = append(eggRules, eggRule{artist: "Travis Scott", album: "UTOPIA", e: egg{
		theme: &utopiaTheme, bg: utopiaBg, ink: utopiaInk, over: utopiaOver,
		knob: '▲', knobCol: hex(0xd8d8d8)}})
}

var utopiaTheme = theme{
	bg:   hex(0x000000),
	a1:   hex(0xc8c8c8),
	a2:   hex(0x8c8c8c),
	a3:   hex(0xf2f2f2),
	text: hex(0xe6e6e6),
	dim:  hex(0x6a6a6a),
	bad:  hex(0xff5a4a),
}

var utopiaHaze = hex(0x3c3b38)

const utopiaDark = 0.05 // what the void leaves of the player outside the light

func utopiaBg(m *Model, c col, x, y int) col { return c.lerp(col{}, 0.92) }

// utopiaLight is how lit a pixel is, 0..1: the hard-edged cone of one spotlight
// hung above the screen, swinging across it and back every 50 s of playback. It is
// off for the first moment of the track, flickers on, then holds.
func utopiaLight(m *Model, px, py int) float64 {
	switch t := m.trackT; {
	case t < 0.8:
		return 0
	case t < 1.1 && hash(int(t*30), 0, 61) < 0.5:
		return 0
	}
	sx, sy := float64(m.w)*0.6, -float64(m.h)*1.2
	dir := 0.55 * math.Sin(2*math.Pi*m.pos/50)
	a := math.Atan2(float64(px)-sx, float64(py)-sy)
	return smooth((0.24 - math.Abs(a-dir)) / 0.05)
}

// utopiaOver darkens the whole player outside the light and lays a haze in the beam,
// dust drifting in it, so the light reads even over black; letters keep a trace in
// the dark.
func utopiaOver(m *Model, f *fb, g playGeom) {
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			c := f.at(x, y)
			lt, lb := utopiaLight(m, x, 2*y), utopiaLight(m, x, 2*y+1)
			kt, kb := utopiaDark+(1-utopiaDark)*lt, utopiaDark+(1-utopiaDark)*lb
			switch {
			case c.pix:
				c.fg = c.fg.mul(kt).lerp(utopiaHaze, 0.3*lt)
			case c.ch != ' ':
				c.fg = c.fg.mul(max(kb, 0.22))
			}
			c.bg = c.bg.mul(kb).lerp(utopiaHaze, 0.3*lb)
		}
	}
	for i := 0; i < 60; i++ {
		px := int(hash(i, 0, 62)*float64(m.w) + 2*math.Sin(m.trackT*0.3+float64(i)))
		py := int(math.Mod(hash(i, 1, 62)*float64(2*m.h)+m.trackT*(0.4+0.6*hash(i, 2, 62)), float64(2*m.h)))
		c := f.at(px, py/2)
		if l := utopiaLight(m, px, py); l < 0.5 || c == nil || !c.pix && c.ch != ' ' {
			continue
		}
		f.pixel(px, py, f.pixelAt(px, py).lerp(hex(0xbdbab2), 0.35))
	}
}

// utopiaInk: the title is polished silver, a highlight band across its middle.
func utopiaInk(m *Model, c col, u, v float64) col {
	return hex(0x7c7c7c).lerp(hex(0xf4f4f4), smooth(1-math.Abs(v-0.4)*2.2))
}
