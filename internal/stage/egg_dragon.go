package stage

import "math"

var dragonTheme = theme{
	bg:   hex(0x1a0505),
	a1:   hex(0xe0a43a),
	a2:   hex(0xb8321f),
	a3:   hex(0xf3d9a4),
	text: hex(0xf3d9a4),
	dim:  hex(0x8a5a3a),
	bad:  hex(0xff5c7a),
}

func init() {
	eggRules = append(eggRules, eggRule{artist: "Big Baby Tape", album: "Dragonborn", e: egg{
		theme: &dragonTheme, bg: dragonBg, ink: dragonInk, over: dragonOver}})
}

// dragonBg turns the screen into dark-red parchment on the reveal; right after,
// one Thu'um "Fus Ro Dah" ring runs from the cover centre across the screen once.
func dragonBg(m *Model, c col, x, y int) col {
	rv := easeInOut(prog(m.trackT, 6, 1.5))
	p := hex(0x200606).mul(0.94 + 0.12*hash(x/2, y, 31))
	c = c.lerp(p, rv)
	if m.trackT >= 6 && m.trackT < 7.6 {
		g := m.geom()
		ccx := float64(g.cx) + float64(g.cw)/2
		ccy := float64(g.cy) + float64(g.ch)/2
		d := math.Hypot(float64(x)-ccx, 2*(float64(y)-ccy))
		k := prog(m.trackT, 6, 1.6)
		R := 0.9 * float64(m.w) * easeOut(k)
		a := (1 - k) * smooth(1-math.Abs(d-R)/1.5)
		c = c.lerp(hex(0xf3d9a4), 0.22*a)
	}
	return c
}

// dragonInk paints the title gold, its top fifth pale as parchment light.
func dragonInk(m *Model, c col, u, v float64) col {
	if v < 0.2 {
		return hex(0xf3d9a4)
	}
	return hex(0xe0a43a)
}

// dragonTape is the compass ribbon; a cardinal every nine cells.
var dragonTape = []rune("N  ·  ·  E  ·  ·  S  ·  ·  W  ·  ·  ")

// dragonOver draws the Skyrim compass in place of the bar and once per track
// lets the dragon cross the top of the screen.
func dragonOver(m *Model, f *fb, g playGeom) {
	_ = g
	if m.trackT >= 6 && !m.barBox.Empty() {
		dragonCompass(m, f)
	}
	if d := m.curTrack().Dur; d > 0 {
		if q := prog(m.pos, 0.35*d, 12); q > 0 && q < 1 {
			dragonFlyby(m, f, q)
		}
	}
}

// dragonCompass replaces the bar and the knob: the ribbon of compass points
// drifts left with the track, brighter where it has been played, and a quest
// marker sits at the current spot.
func dragonCompass(m *Model, f *fb) {
	x, y, bw := m.barBox.Min.X, m.barBox.Min.Y, m.barBox.Dx()
	f.blank(x, y, bw)
	fr := 0.0
	if d := m.curTrack().Dur; d > 0 {
		fr = min(1, m.pos/d)
	}
	offset := int(fr * float64(bw) * 3)
	dr := easeOut(prog(m.trackT, 6, 1))
	for i := range bw {
		if hash(i, 0, 71) >= dr {
			continue
		}
		var r rune
		var c col
		switch i {
		case 0:
			r, c = '╟', hex(0x8a5a3a)
		case bw - 1:
			r, c = '╢', hex(0x8a5a3a)
		default:
			r = dragonTape[(i+offset)%len(dragonTape)]
			if r == ' ' {
				continue
			}
			played := float64(i)/float64(bw) < fr
			c = hex(0xe0a43a)
			if r == '·' {
				c = hex(0x8a5a3a)
			}
			if played {
				c = hex(0xf3d9a4)
				if r == '·' {
					c = hex(0xc08a4a)
				}
			}
		}
		f.style(x+i, y, r, c, false, false)
	}
	if bw >= 3 {
		f.style(x+max(1, min(int(fr*float64(bw)), bw-2)), y, '▼', hex(0xf3d9a4), true, false)
	}
}

// dragonFlyby sweeps the sprite from right to left along pixel rows 2..6,
// wings beat once every half second; cells with text of the player are spared.
func dragonFlyby(m *Model, f *fb, q float64) {
	xd := int(lerpf(float64(m.w)+13, -13, q))
	frame := dragonSprite[int(m.pos*2)%2]
	for row, line := range frame {
		for colu := 0; colu < len(line); colu++ {
			var c col
			switch line[colu] {
			case '#':
				c = hex(0x050101)
			case '+':
				c = hex(0x3a0d0d)
			default:
				continue
			}
			px, py := xd+colu, 2+row
			if c0 := f.at(px, py/2); c0 != nil && (c0.pix || c0.ch == ' ') {
				f.pixel(px, py, c)
			}
		}
	}
}

var dragonSprite = [2][5]string{
	{"....+###.....", ".....+##+....", "##+######++..", ".###########+", "............#"},
	{".............", ".............", "##+######++..", ".###########+", "....+####+..#"},
}
