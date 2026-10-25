package stage

import "math"

func init() {
	eggRules = append(eggRules, eggRule{
		artist: "Kanye West",
		album:  "My Beautiful Dark Twisted Fantasy",
		e: egg{
			theme:   &mbdtfTheme,
			bg:      mbdtfBg,
			cover:   mbdtfCover,
			ink:     mbdtfInk,
			over:    mbdtfOver,
			knob:    '♛',
			knobCol: hex(0xd4a843),
		},
	})
}

// mbdtfTheme is the album's gallery room: dark red walls, gold and blood accents.
var mbdtfTheme = theme{
	bg:   hex(0x2a0508),
	a1:   hex(0xd4a843),
	a2:   hex(0xb0262f),
	a3:   hex(0xf6e7c1),
	text: hex(0xf6e7c1),
	dim:  hex(0x8a5a4a),
	bad:  hex(0xff5c7a),
}

// mbdtfBg: at the reveal the backdrop becomes a canvas-textured gallery wall
// with a warm gold halo radiating from the title.
func mbdtfBg(m *Model, c col, x, y int) col {
	rv := easeInOut(prog(m.trackT, 6, 1.5))
	if rv == 0 {
		return c
	}
	w := hex(0x3b070c).mul(0.97 + 0.06*hash(x, y, 7))
	if !m.titleBox.Empty() {
		tcx := float64(m.titleBox.Min.X+m.titleBox.Max.X) / 2
		tcy := float64(m.titleBox.Min.Y+m.titleBox.Max.Y) / 4
		ax := float64(m.titleBox.Dx())/2 + 4
		ay := float64(m.titleBox.Dy())/4 + 2
		d := math.Hypot((float64(x)-tcx)/ax, (float64(y)-tcy)/ay)
		w = w.lerp(hex(0xd4a843), 0.18*smooth(1-d))
	}
	return c.lerp(w, rv)
}

// mbdtfCover: the censorship breathes — the whole cover drifts into blocks
// of up to 4 pixels and back, once every 80 s.
func mbdtfCover(m *Model, at func(px, py int) col, px, py int) col {
	k := 1 + int(math.Round(3*(1-math.Cos(2*math.Pi*m.trackT/80))/2))
	return at(px-px%k, py-py%k)
}

// mbdtfInk: the title in gold, shading darker toward the bottom.
func mbdtfInk(m *Model, c col, u, v float64) col {
	return hex(0xf6dc8a).lerp(hex(0xb8862b), v)
}

// mbdtfOver: the cover hangs in a gold frame, two pixel rings on each side;
// the frame shimmers in over the second after the reveal.
func mbdtfOver(m *Model, f *fb, g playGeom) {
	a := easeOut(prog(m.trackT, 6, 1))
	if a == 0 {
		return
	}
	for x := g.cx - 2; x <= g.cx+g.cw+1; x++ {
		for y := g.cy*2 - 2; y <= g.cy*2+g.ch*2+1; y++ {
			dx := 0
			switch {
			case x < g.cx:
				dx = g.cx - x
			case x >= g.cx+g.cw:
				dx = x - (g.cx + g.cw - 1)
			}
			dy := 0
			switch {
			case y < g.cy*2:
				dy = g.cy*2 - y
			case y >= g.cy*2+g.ch*2:
				dy = y - (g.cy*2 + g.ch*2 - 1)
			}
			dist := max(dx, dy)
			if dist != 1 && dist != 2 || hash(x, y, 61) >= a {
				continue
			}
			c := hex(0xe2bd55)
			if dist == 2 {
				c = hex(0x8a6a22)
			}
			f.pixel(x, y, c)
		}
	}
}
