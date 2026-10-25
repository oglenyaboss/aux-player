package stage

import "math"

func init() {
	eggRules = append(eggRules, eggRule{artist: "Kanye West", album: "Yeezus", e: egg{
		theme: &yeezusTheme, bg: yeezusBg, cover: yeezusCover, ink: yeezusInk,
		bar: yeezusBar, knob: '■', knobCol: hex(0xe3001b)}})
}

var yeezusTheme = theme{
	bg:   hex(0xdcdcdc),
	a1:   hex(0xe3001b),
	a2:   hex(0x8a8a8a),
	a3:   hex(0x111111),
	text: hex(0x111111),
	dim:  hex(0x6e6e6e),
	bad:  hex(0xe3001b),
}

// yeezusBg washes the whole screen into the white plastic of the empty jewel case from the first second of the track.
func yeezusBg(m *Model, c col, x, y int) col {
	return c.lerp(hex(0xdcdcdc), easeInOut(prog(m.trackT, 0, 1.4)))
}

// yeezusCover turns the glare on the disc once a minute, like the record is spinning;
// the red tape on the right edge and the sleeve around the disc stay still.
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
