package stage

import "math"

// Currents: the cover is a flow of lines bending round a chrome ball, a red line
// running through it. The whole backdrop becomes that flow: streamlines of a current
// round the cover, the cover is the ball; the lines drift and ripple, more so in
// the wake, and the red line runs through the middle of the stream.
func init() {
	eggRules = append(eggRules, eggRule{artist: "Tame Impala", album: "Currents", e: egg{
		theme: &currentsTheme, bg: currentsBg, ink: currentsInk, over: currentsOver,
		knob: '●', knobCol: hex(0xff4a2a)}})
}

var currentsTheme = theme{
	bg:   hex(0x1a0a26),
	a1:   hex(0xff4a2a),
	a2:   hex(0xb48ef0),
	a3:   hex(0xffb347),
	text: hex(0xf3eaff),
	dim:  hex(0x8a74a8),
	bad:  hex(0xff4a2a),
}

var (
	currentsDeep = hex(0x1a0a26)
	currentsLine = hex(0xa47ae0)
	currentsHot  = hex(0xff5fa8)
	currentsRed  = hex(0xff3b2f)
	currentsGold = hex(0xffb347)
)

// currentsAngle is the direction of the stream, down to the right like on the cover.
const currentsAngle = 0.6

func currentsBg(m *Model, c col, x, y int) col { return c.lerp(currentsDeep, 0.9) }

// currentsFlow is the colour of the stream at one pixel over base: the stream
// function of a flow past a cylinder the size of the cover, its lines rippled by
// noise carried along the stream. ok is false inside the ball.
func currentsFlow(m *Model, g playGeom, base col, px, py int) (col, bool) {
	R := float64(g.ch)
	dx := float64(px) - (float64(g.cx) + float64(g.cw)/2)
	dy := float64(py) - (float64(g.cy*2) + float64(g.ch))
	ca, sa := math.Cos(currentsAngle), math.Sin(currentsAngle)
	u, v := dx*ca+dy*sa, -dx*sa+dy*ca
	r2 := u*u + v*v
	if r2 < R*R {
		return base, false
	}
	k := R * R / r2
	psi, phi := v*(1-k), u*(1+k)
	t := m.trackT
	wake := smooth(phi / (R * 3))
	psi += (0.6 + 2.4*wake) * (vnoise(phi*0.06-t*0.9, psi*0.12, 71) - 0.5) * 2

	reach := (t-0.2)*float64(m.w)*1.2 - float64(m.w)
	if phi > reach {
		return base, true
	}
	if math.Abs(psi) < 1.2 && r2 > 2*R*R && math.Abs(v) < R {
		return currentsRed.lerp(currentsGold, smooth(phi/(R*4))), true
	}
	s := psi/3.4 - math.Floor(psi/3.4)
	line := smooth((0.2 - math.Min(s, 1-s)) / 0.1)
	lc := currentsLine.lerp(currentsHot, 0.4*vnoise(float64(px)*0.04, float64(py)*0.04+t*0.2, 72))
	return base.lerp(lc, 0.6*line), true
}

// currentsInk: the title is pale lilac, warming to the gold of the line at its foot.
func currentsInk(m *Model, c col, u, v float64) col {
	return currentsTheme.text.lerp(currentsGold, 0.5*v*v)
}

// currentsOver lays the stream over the player: blank cells pixel by pixel, text a
// faint trace, the cover untouched as the ball.
func currentsOver(m *Model, f *fb, g playGeom) {
	rows := m.h
	if m.typing {
		rows = m.h - 4
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < m.w; x++ {
			c := f.at(x, y)
			if c.pix {
				continue
			}
			if inText(f, x, y) {
				if fc, ok := currentsFlow(m, g, c.bg, x, 2*y+1); ok {
					c.bg = c.bg.lerp(fc, 0.25)
				}
				continue
			}
			base := c.bg
			top, ok1 := currentsFlow(m, g, base, x, 2*y)
			bot, ok2 := currentsFlow(m, g, base, x, 2*y+1)
			if !ok1 && !ok2 {
				continue
			}
			f.pixel(x, 2*y, top)
			f.pixel(x, 2*y+1, bot)
		}
	}
}
