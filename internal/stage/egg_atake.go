package stage

import "math"

// egg for Lil Uzi Vert's album "Eternal Atake": the UFO on the cover comes
// alive. From the reveal an emerald beam lowers from the saucer and reaches
// the bottom of the cover at 90 s, then particles ride the beam up every 3 s.
// The screen is a still starfield, the title grades purple into emerald and
// the knob is a star; the album replaces the artist image, so no pink jewel.

var atakeTheme = theme{
	bg:   hex(0x06040d),
	a1:   hex(0xb48cff),
	a2:   hex(0x6ff0c0),
	a3:   hex(0xff7ad9),
	text: hex(0xefeaff),
	dim:  hex(0x6a6380),
	bad:  hex(0xff5c7a),
}

func init() {
	eggRules = append(eggRules, eggRule{
		artist: "Lil Uzi Vert",
		album:  "Eternal Atake",
		e: egg{
			theme:   &atakeTheme,
			bg:      atakeBg,
			cover:   atakeCover,
			ink:     atakeInk,
			over:    atakeOver,
			knob:    '✦',
			knobCol: hex(0x6ff0c0),
		},
	})
}

// atakeBg dims the whole backdrop into the night sky once the egg reveals.
func atakeBg(m *Model, c col, x, y int) col {
	return c.lerp(hex(0x06040d), easeInOut(prog(m.trackT, 6, 2)))
}

// atakeCover lowers the emerald beam from the saucer at 0.77 of the cover's
// width: from the reveal it widens as it falls and reaches the bottom at 90 s.
func atakeCover(m *Model, at func(px, py int) col, px, py int) col {
	u := (float64(px) + 0.5) / float64(m.geom().cw)
	v := (float64(py) + 0.5) / float64(m.geom().ch*2)
	c := at(px, py)
	lb := 0.19 + 0.81*easeInOut(prog(m.trackT, 6, 84))
	if v < 0.19 || v > lb {
		return c
	}
	hw := 0.03 + 0.25*(v-0.19)
	dist := math.Abs(u - 0.77)
	if dist > hw {
		return c
	}
	edge := smooth(1 - dist/hw)
	k := (0.28 + 0.08*(0.5+0.5*math.Sin(float64(py)*0.9-m.t*1.2))) * edge * smooth((lb-v)/0.04)
	return c.lerp(hex(0x6ff0c0), k)
}

// atakeInk grades the title purple into emerald top down.
func atakeInk(m *Model, c col, u, v float64) col {
	return hex(0xb48cff).lerp(hex(0x6ff0c0), v)
}

// atakeOver draws the starfield as glyphs over empty cells once the egg reveals,
// then sends a particle up the beam every 3 s once it has landed.
func atakeOver(m *Model, f *fb, g playGeom) {
	if rv := easeInOut(prog(m.trackT, 6, 2)); rv > 0 {
		for y := 0; y < f.h; y++ {
			for x := 0; x < f.w; x++ {
				atakeStar(f, x, y, rv)
			}
		}
	}
	if m.trackT < 90 {
		return
	}
	top := int((m.trackT - 90) / 3)
	for n := top; n >= top-1 && n >= 0; n-- {
		tb := 90 + 3.0*float64(n)
		if tb > m.trackT || m.trackT >= tb+6 {
			continue
		}
		q := (m.trackT - tb) / 6
		v := 1 - q*0.81
		u := 0.77 + 0.02*math.Sin(float64(n)*1.7+q*3)
		alpha := math.Sin(math.Pi * q)
		x := g.cx + int(u*float64(g.cw))
		y := g.cy*2 + int(v*float64(g.ch*2))
		f.pixel(x, y, f.pixelAt(x, y).lerp(hex(0xd9fff0), alpha))
	}
}

// atakeStar puts one star glyph on an empty cell whose row neighbours are empty too:
// stars never sit between words or against the text.
func atakeStar(f *fb, x, y int, rv float64) {
	if hash(x, y, 21) <= 0.985 {
		return
	}
	c := f.at(x, y)
	if c == nil || c.ch != ' ' || c.pix {
		return
	}
	for _, nx := range [2]int{x - 1, x + 1} {
		if n := f.at(nx, y); n != nil && (n.ch != ' ' || n.pix) {
			return
		}
	}
	t := hash(x, y, 24)
	ch := '·'
	switch {
	case t < 0.08:
		ch = '✦'
	case t < 0.25:
		ch = '+'
	}
	b := 0.25 + 0.45*hash(x, y, 22)
	h := hash(x, y, 23)
	tone := white
	switch {
	case h < 0.1:
		tone = hex(0xb48cff)
	case h < 0.2:
		tone = hex(0x6ff0c0)
	}
	f.put(x, y, ch, c.bg.lerp(tone, b*rv))
}
