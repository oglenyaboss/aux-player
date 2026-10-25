package stage

import (
	"math"
	"strings"
)

// HEROES & VILLAINS: the cover is a man on fire shaking hands, and the trailer burns
// Metropolis down. The player stands in the dark of the cover while the city burns
// below it. The instrumentals of the Heroes Version are the morning after: the fire
// is out, smoke and embers rise over the standing city.
func init() {
	eggRules = append(eggRules, eggRule{artist: "Metro Boomin", album: "HEROES & VILLAINS", e: egg{
		theme: &hvTheme, bg: hvBg, cover: hvCover, ink: hvInk, over: hvOver,
		knob: '✹', knobCol: hex(0xff8a2a)}})
}

var hvTheme = theme{
	bg:   hex(0x0c1515),
	a1:   hex(0xff7a1a),
	a2:   hex(0xc9a27a),
	a3:   hex(0xffe2b0),
	text: hex(0xf2ece2),
	dim:  hex(0x6f7a76),
	bad:  hex(0xff5a3a),
}

var (
	hvNight  = hex(0x0c1515)
	hvTower  = hex(0x040707)
	hvEmber  = hex(0xff6a10)
	hvWindow = hex(0xffc96a)
	hvSmoke  = hex(0x4a5250)
)

// hvFire is the heat ramp from the dark to white hot.
var hvFire = []col{hex(0x3a0600), hex(0xa82000), hex(0xf05a08), hex(0xff9a22), hex(0xffd060), hex(0xfff1b8)}

// hvAfter: the instrumentals of the Heroes Version play the morning after.
func hvAfter(m *Model) bool { return strings.Contains(fold(m.curTrack().Title), "instrumental") }

func hvHeat(c float64) col {
	c = clamp01(c) * float64(len(hvFire)-1)
	i := min(int(c), len(hvFire)-2)
	return hvFire[i].lerp(hvFire[i+1], c-float64(i))
}

// hvRoof is how many pixels the skyline stands at column x: blocks of seven columns,
// some with a spire in the middle.
func hvRoof(m *Model, x int) int {
	b := x / 7
	r := float64(m.h) * (0.2 + 0.4*hash(b, 0, 37))
	if hash(b, 1, 37) > 0.8 && x%7 == 3 {
		r += float64(m.h) * 0.16
	}
	return int(r)
}

// hvBg darkens the backdrop to the night of the cover; the city and the fire are drawn over it.
func hvBg(m *Model, c col, x, y int) col { return c.lerp(hvNight, 0.85) }

// hvLayer is the city at one pixel over base: the towers with their windows, and
// above them the fire, or the morning after smoke and embers.
func hvLayer(m *Model, base col, px, py int) col {
	up := 2*m.h - 1 - py
	t := m.trackT
	after := hvAfter(m)
	if roof := hvRoof(m, px); up < roof {
		if px%2 == 1 && py%4 == 1 && up < roof-2 && hash(px, py/4, 38) > 0.7 {
			if after {
				return hvTower.lerp(hvWindow, 0.55)
			}
			return hvTower.lerp(hvEmber, 0.25+0.35*vnoise(float64(px)*0.3, t*2, 39))
		}
		return hvTower
	}
	fx, fy := float64(px), float64(py)
	if after {
		for i := 0; i < 24; i++ {
			ex := int(hash(i, 0, 41)*float64(m.w) + 3*math.Sin(t*0.7+float64(i)))
			ey := 2*m.h - 1 - int(math.Mod(t*(2.4+2*hash(i, 1, 41))+hash(i, 2, 41)*float64(2*m.h), float64(2*m.h)))
			if ex == px && ey == py {
				return base.lerp(hvEmber, 0.9)
			}
		}
		k := vnoise(fx*0.06, fy*0.08+t*0.9, 40) * smooth(1-float64(up)/(float64(m.h)*1.3))
		return base.lerp(hvSmoke, 0.5*k)
	}
	height := float64(2*m.h) * 0.6 * easeOut(prog(t, 0.4, 4)) * (0.65 + 0.7*vnoise(fx*0.035, t*0.25, 46))
	if height <= 0 {
		return base
	}
	w := vnoise(fx*0.05, fy*0.05+t*0.8, 47)
	n := 0.6*vnoise(fx*0.1+w*2, fy*0.13+t*3.2, 42) + 0.4*vnoise(fx*0.23, fy*0.3+t*5.5, 43)
	heat := n*1.55 - float64(up)/height
	if heat <= 0 {
		return base.lerp(hvFire[0], 0.4*smooth(1-float64(up)/(height*1.5)))
	}
	return base.lerp(hvHeat(heat), smooth(heat*3))
}

// hvOver lays the city over the player: blank cells get it pixel by pixel, cells
// with text only a glimpse, so the queue stays readable over the fire.
func hvOver(m *Model, f *fb, g playGeom) {
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
				c.bg = c.bg.lerp(hvLayer(m, c.bg, x, 2*y+1), 0.3)
				continue
			}
			base := c.bg
			top, bot := hvLayer(m, base, x, 2*y), hvLayer(m, base, x, 2*y+1)
			if top == base && bot == base {
				continue
			}
			f.pixel(x, 2*y, top)
			f.pixel(x, 2*y+1, bot)
		}
	}
}

// hvCover: the flames on the cover flicker, the rest of the photo stays still.
func hvCover(m *Model, at func(px, py int) col, px, py int) col {
	c := at(px, py)
	if c.lum() < 0.45 || c.r < c.b+0.2 {
		return c
	}
	return c.mul(0.82 + 0.36*vnoise(float64(px)*0.25, float64(py)*0.18+m.trackT*2.5, 44))
}

// hvInk: the title smoulders, hot at its bottom edge; the morning after it has cooled.
func hvInk(m *Model, c col, u, v float64) col {
	if hvAfter(m) {
		return c
	}
	return c.lerp(hvEmber, 0.7*v*(0.7+0.3*vnoise(u*8, m.trackT*2, 45)))
}
