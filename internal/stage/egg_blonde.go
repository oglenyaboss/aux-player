package stage

import "math"

// blondeRule hangs the fogged shower on Frank Ocean's Blonde; the Nights rule adds
// its beat switch on top as the title layer.
var blondeRule = eggRule{artist: "Frank Ocean", album: "Blonde", e: egg{
	theme:   &blondeTheme,
	bg:      blondeBg,
	ink:     blondeInk,
	knob:    '●',
	knobCol: hex(0x9be37a),
}}

func init() {
	eggRules = append(eggRules, blondeRule)
	blondeWordW = pixWidth("BLOND")
	at := 0
	for i, g := range blondeGlyphs {
		blondeWordX[i] = at
		at += len(g[0]) + 1
	}
}

var blondeTheme = theme{
	bg:   hex(0x0d0f0e),
	a1:   hex(0x9be37a),
	a2:   hex(0xc8d6c4),
	a3:   hex(0xffffff),
	text: hex(0xf2f2f2),
	dim:  hex(0x6c736e),
	bad:  hex(0xff5c7a),
}

// blondeInk bleaches the title towards the pale field of the cover.
func blondeInk(m *Model, c col, u, v float64) col {
	return c.lerp(hex(0xf2f2f2), 0.8)
}

// blondeGlyphs is the cover's «blond» in font3, lifted into a package table so the
// per-cell bg hook does no map lookups and allocates nothing.
var blondeGlyphs = [5][5]string{font3['B'], font3['L'], font3['O'], font3['N'], font3['D']}

// blondeWordW is the width of BLOND in cells; pixWidth runs a ToUpper per call, so it is measured once.
var blondeWordW int

// blondeWordX is the left column of each letter inside the word.
var blondeWordX [5]int

// blondeLevel is how high the steam has climbed by track position t: the share of
// the screen height it claims from below.
func blondeLevel(t, dur float64) float64 {
	if dur <= 0 {
		return 0.05
	}
	return 0.05 + 0.65*clamp01(t/dur)
}

// blondeBg turns the backdrop into a fogged shower: dark tile from the egg reveal,
// steam climbing by the track's position, drop trails refogging over 30 s, and the
// word written in the steam a finger's width deep.
func blondeBg(m *Model, c col, x, y int) col {
	W, H := m.w, m.h
	rv := easeInOut(prog(m.trackT, 6, 2))
	if rv == 0 {
		return c
	}
	Dur := m.curTrack().Dur
	if xk, yd, on := blondeDrop(m, W, H, Dur); on && x == xk && y == int(math.Round(yd)) {
		return c.lerp(hex(0xe8f0ea), rv)
	}
	tile := hex(0x161b18)
	if x%6 == 0 || y%3 == 0 {
		tile = hex(0x0b0d0c)
	}
	yb := (float64(H-y) - 0.5) / float64(H)
	a := 0.24 * smooth((blondeLevel(m.pos, Dur)-yb)/(3/float64(H))+0.5)
	a *= 0.8 + 0.2*math.Sin(float64(x)*0.21+float64(y)*0.35+m.t/9)
	a *= blondeStreaks(m, W, H, x, y, Dur)
	a *= blondeWord(m, W, H, x, y, Dur)
	return c.lerp(tile.lerp(hex(0xc9d1cc), a), rv)
}

// blondeDrop is where the drop sliding right now sits, if one is on the glass:
// drops start every 23 s at a hashed column and run 9 s to the bottom, so the
// position alone tells whether the cell carries the drop itself.
func blondeDrop(m *Model, W, H int, dur float64) (xk int, yd float64, on bool) {
	k := int(m.pos / 23)
	d := m.pos - 23*float64(k)
	if k < 1 || d >= 9 {
		return 0, 0, false
	}
	xk = 1 + int(hash(k, 1, 51)*float64(W-2))
	t0 := 23 * float64(k)
	y0 := int((1 - blondeLevel(t0, dur)) * float64(H))
	return xk, float64(y0) + float64(H-1-y0)*easeIn(d/9), true
}

// blondeStreaks multiplies the steam by how much has flocked back over the last
// two drop trails in the cell's column: each trail clears to the bottom over 30 s
// after the drop passes its row.
func blondeStreaks(m *Model, W, H, x, y int, dur float64) float64 {
	s := 1.0
	k := int(m.pos / 23)
	for i := k; i > k-2 && i >= 1; i-- {
		if x != 1+int(hash(i, 1, 51)*float64(W-2)) {
			continue
		}
		t0 := 23 * float64(i)
		y0 := int((1 - blondeLevel(t0, dur)) * float64(H))
		yd := float64(y0) + float64(H-1-y0)*easeIn(prog(m.pos, t0, 9))
		if y < y0 || float64(y) >= yd {
			continue
		}
		s *= clamp01((m.pos - blondeSince(t0, y0, y, H)) / 30)
	}
	return s
}

// blondeSince is the position of track k when the drop crossed the row above.
func blondeSince(t0 float64, y0, y, H int) float64 {
	if den := float64(H - 1 - y0); den > 0 {
		return t0 + 9*math.Cbrt(float64(y-y0)/den)
	}
	return t0
}

// blondeWord is the share of the steam left over a cell of the word written in it:
// letters surface one by one from 0.45*dur in half-second steps, fog back over
// 32.5 s later, and the dots clear the steam without touching the tile.
func blondeWord(m *Model, W, H, x, y int, dur float64) float64 {
	if dur <= 0 {
		return 1
	}
	T0 := 0.45 * dur
	bx, by := blondeWordXY(m, W, H)
	dx, dy := x-bx, y-by
	if dy < 0 || dy >= 5 || dx < 0 || dx >= blondeWordW {
		return 1
	}
	for i := 4; i >= 0; i-- {
		w := len(blondeGlyphs[i][0])
		if dx < blondeWordX[i] || dx >= blondeWordX[i]+w {
			continue
		}
		if blondeGlyphs[i][dy][dx-blondeWordX[i]] != '#' {
			return 1
		}
		la := smooth(prog(m.pos, T0+0.5*float64(i), 0.5))
		la *= 1 - smooth(prog(m.pos, T0+32.5, 20))
		return 1 - la
	}
	return 1
}

// blondeWordXY sets where the word sits: under the cover while five rows fit above
// the bottom hints, otherwise tucked into the bottom right corner.
func blondeWordXY(m *Model, W, H int) (bx, by int) {
	bx, by = W-3-blondeWordW, H-8
	if g := m.geom(); g.cy+g.ch+1+5 <= H-3 {
		bx, by = g.cx, g.cy+g.ch+1
	}
	return bx, by
}
