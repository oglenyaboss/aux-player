package stage

import "math"

func init() {
	eggRules = append(eggRules, eggRule{artist: "Joy Division", album: "Unknown Pleasures", e: egg{
		theme: &pulsarTheme, bg: pulsarBg, ink: pulsarInk, over: pulsarOver}})
}

var pulsarTheme = theme{
	bg:   hex(0x03070c),
	a1:   hex(0xf0f0f0),
	a2:   hex(0xaab2bb),
	a3:   hex(0xffffff),
	text: hex(0xf0f0f0),
	dim:  hex(0x59626c),
	bad:  hex(0xff5c7a),
}

func pulsarBg(m *Model, c col, x, y int) col {
	return hex(0x03070c)
}

func pulsarInk(m *Model, c col, u, v float64) col {
	return c.lerp(hex(0xf0f0f0), 0.9)
}

// pulsarDots is the braille bit for the two halves (dx) and four dot rows (dy) of a cell.
var pulsarDots = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}} // [dx][dy]

// pulsarOver turns the bar row into a braille trace of pulsar CP 1919: a new pulse
// every 17 s steps in front of the older ones, which ride up and dim, until a
// little Unknown Pleasures is standing over the bar.
func pulsarOver(m *Model, f *fb, g playGeom) {
	if g.mini || m.trackT < eggKnobAt {
		return
	}
	b := m.barBox
	bw := b.Dx()
	if bw <= 0 {
		return
	}
	n := 2 * bw
	frac := 0.0
	if d := m.curTrack().Dur; d > 0 {
		frac = min(1, m.pos/d)
	}
	for row := b.Min.Y - 2; row <= b.Min.Y; row++ {
		f.blank(b.Min.X, row, bw)
	}
	e := int(m.pos / 17)
	s := smooth(prog(m.pos-17*float64(e), 0, 4))
	if e == 0 {
		s = 1
	}
	A := smooth(prog(m.pos, 6, 20))
	phi := math.Mod(m.pos, 1.3373)
	rc := easeOut(prog(m.trackT, 6, 1.0)) * float64(n)
	cHead := int(frac * float64(n))

	// per-cell state: glyph bits and the colour of the first impulse through
	glyph := make([]rune, 3*bw)
	seen := make([]bool, 3*bw)
	first := make([]col, 3*bw)
	put := func(x, row int, bit rune, fg col, force bool) {
		o := (row-(b.Min.Y-2))*bw + (x - b.Min.X)
		glyph[o] |= bit
		switch {
		case !seen[o]:
			seen[o], first[o] = true, fg
		case force:
			first[o] = fg
		}
		f.put(x, row, rune(0x2800)|glyph[o], first[o])
	}

	top := make([]int, n)
	lvs := make([]int, n)
	for i := range top {
		top[i] = -1
	}
	for j := e; j >= max(0, e-3); j-- {
		d := 0.0
		if j < e {
			d = float64(e-j) - 1 + s
		}
		if d >= 3 {
			continue
		}
		front := j == e
		base := 1 + 3*d
		for c := 0; c < n; c++ {
			lvs[c] = pulsarLevel(c, j, n, base, A, s, front)
		}
		for c := 0; c < n; c++ {
			if float64(c) >= rc {
				continue
			}
			prev := lvs[0]
			if c > 0 {
				prev = lvs[c-1]
			}
			lv := lvs[c]
			x := b.Min.X + c/2
			fg := pulsarBackCol(d)
			if front {
				fg = pulsarFrontCol(n, c, frac, phi)
			}
			for L := min(lv, prev+1); L <= max(lv, prev-1); L++ {
				if L <= top[c] {
					continue
				}
				put(x, b.Min.Y-L/4, pulsarDots[c%2][3-L%4], fg, false)
			}
			if lv > top[c] {
				top[c] = lv
			}
		}
		if front && cHead < n && float64(cHead) < rc {
			if lh := lvs[cHead] + 1; lh <= 11 {
				x := b.Min.X + cHead/2
				put(x, b.Min.Y-lh/4, pulsarDots[cHead%2][3-lh%4], white, true)
			}
		}
	}
}

// pulsarLevel: the braille level (0..11) of pulse j in dot column c; the central
// spike sits at p 0.5 and j's own ridges ride behind it.
func pulsarLevel(c, j, n int, base, A, s float64, front bool) int {
	p := float64(c) / float64(n-1)
	noise := (hash(c, j, 41) - 0.5) * 0.8
	k := 3 + int(hash(j, 0, 42)*3)
	peaks := 0.0
	for i := 0; i < k; i++ {
		at, hgt := 0.5, 4.0
		if i > 0 {
			at, hgt = 0.32+0.36*hash(j, i, 43), 1.5+2*hash(j, i, 44)
		}
		wid := 0.025 + 0.02*hash(j, i, 45)
		q := (p - at) / wid
		peaks += hgt * math.Exp(-q*q)
	}
	mult := 1.0
	if front {
		mult = s
	}
	h := max(-0.5, min(4, noise+A*mult*peaks))
	return min(11, max(0, int(math.Round(base+h))))
}

// pulsarFrontCol: the played span is bright, the rest grey; the central spike
// flashes once every spin of CP 1919.
func pulsarFrontCol(n, c int, frac, phi float64) col {
	fg := hex(0x7d868f)
	if float64(c) < frac*float64(n) {
		fg = hex(0xf0f0f0)
	}
	if math.Abs(float64(c)/float64(n-1)-0.5) < 0.06 {
		fg = fg.lerp(white, 0.35*math.Exp(-phi/0.25))
	}
	return fg
}

// pulsarBackCol rides the ink from grey into the background as a pulse recedes, depth 0..3.
func pulsarBackCol(d float64) col {
	switch {
	case d <= 1:
		return hex(0x7d868f).lerp(hex(0x4a525b), d)
	case d <= 2:
		return hex(0x4a525b).lerp(hex(0x353b42), d-1)
	default:
		return hex(0x353b42).lerp(hex(0x03070c), d-2)
	}
}
