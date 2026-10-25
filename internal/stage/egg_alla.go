package stage

import "math"

// AT.LONG.LAST.A$AP: a black-and-white portrait, one acid stain over the eye, the title
// down a black spine; a slow, smoky, dark record. The player is a black-and-white
// film: grain, smoke drifting up from below, the stain on the cover the only colour,
// breathing. The spine runs down the left edge. On L$D the smoke takes the colour of
// the stain.
func init() {
	eggRules = append(eggRules, eggRule{artist: "A$AP Rocky", album: "AT.LONG.LAST.A$AP", e: egg{
		theme: &allaTheme, bg: allaBg, cover: allaCover, ink: allaInk, over: allaOver,
		knob: '✝', knobCol: hex(0xeeeeee)}})
}

var allaTheme = theme{
	bg:   hex(0x0e0e0e),
	a1:   hex(0xe0e0e0),
	a2:   hex(0x9a9a9a),
	a3:   hex(0xffffff),
	text: hex(0xeeeeee),
	dim:  hex(0x7a7a7a),
	bad:  hex(0xff5a7a),
}

var allaSmoke = hex(0x9a9a9a)

const allaSpine = "AT. LONG. LAST. A$AP"

func allaTrip(m *Model) bool { return foldTitle(m.curTrack().Title) == "l$d" }

func allaGrey(c col) col {
	l := c.lum()
	return col{l, l, l}
}

func allaBg(m *Model, c col, x, y int) col {
	return allaGrey(c).lerp(allaTheme.bg, 0.85)
}

// allaCover keeps the cover black and white; the colour of the stain breathes slowly.
func allaCover(m *Model, at func(px, py int) col, px, py int) col {
	c := at(px, py)
	h, s, v := toHSV(c)
	if s < 0.2 {
		return allaGrey(c)
	}
	return hsv(h, s*(0.7+0.3*math.Sin(m.trackT*0.7)), v)
}

// allaInk: the title in white, greying toward its foot like the print.
func allaInk(m *Model, c col, u, v float64) col { return hex(0xf4f4f4).lerp(hex(0x9a9a9a), v*0.6) }

// allaHaze is how thick the smoke is on a pixel, 0..1: billows rising from the bottom
// of the screen, curling as they go, thinning toward the top.
func allaHaze(m *Model, px, py int) float64 {
	t := m.trackT
	fx, fy := float64(px), float64(py)
	w := vnoise(fx*0.02, fy*0.03+t*0.15, 66)
	n := vnoise(fx*0.035+w*2.5, fy*0.05+t*0.35, 67)
	n = 0.7*n + 0.3*vnoise(fx*0.08, fy*0.1+t*0.6, 68)
	low := smooth(fy / float64(2*m.h) * 1.3)
	return smooth((n-0.42)/0.4) * (0.25 + 0.75*low) * smooth(t/3)
}

// allaFog is the colour of the smoke: grey, on L$D the drifting colour of the stain.
func allaFog(m *Model, px int) col {
	if !allaTrip(m) {
		return allaSmoke
	}
	return hsv(300+40*math.Sin(m.trackT*0.2+float64(px)*0.01), 0.55, 0.85)
}

// allaFilm lays smoke and grain on one backdrop pixel.
func allaFilm(m *Model, c col, px, py int) col {
	k := 0.45
	if allaTrip(m) {
		k = 0.65
	}
	c = allaGrey(c).lerp(allaFog(m, px), k*allaHaze(m, px, py))
	return c.mul(0.94 + 0.12*hash(px, py, int(m.trackT*12)))
}

// allaOver films the player: smoke and grain over the backdrop pixel by pixel, a
// thin veil of it behind text, the queue art drained of colour; then the spine.
func allaOver(m *Model, f *fb, g playGeom) {
	rows := m.h
	if m.typing {
		rows = m.h - 4
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < m.w; x++ {
			c := f.at(x, y)
			switch {
			case c.pix && (x < g.cx || x >= g.cx+g.cw || y < g.cy || y >= g.cy+g.ch):
				c.fg, c.bg = allaGrey(c.fg), allaGrey(c.bg)
			case c.pix:
			case inText(f, x, y):
				c.bg = c.bg.lerp(allaFilm(m, c.bg, x, 2*y+1), 0.4)
			default:
				base := c.bg
				f.pixel(x, 2*y, allaFilm(m, base, x, 2*y))
				f.pixel(x, 2*y+1, allaFilm(m, base, x, 2*y+1))
			}
		}
	}
	if m.h < len(allaSpine)+4 || g.cx < 4 {
		return
	}
	f.fill(0, 0, 3, m.h, hex(0x050505))
	y0 := (m.h - len(allaSpine)) / 2
	for i, r := range allaSpine {
		f.put(1, y0+i, r, hex(0xe8e8e8))
	}
}
