package stage

import (
	"bytes"
	_ "embed"
	"image"
	"image/jpeg"
	"math"
)

// Blonde is a day and a night. The first half of the album plays as an overexposed
// film with a warm light drifting across; Nights switches at 3:30 with a hard cut to
// the smoke cover «blonde», and the second half of the album is a night drive: the
// blurred lights of the city drift past behind the glass.
var blondeNightTitles = []string{"Solo (Reprise)", "Pretty Sweet", "Facebook Story",
	"Close to You", "White Ferrari", "Seigfried", "Godspeed", "Futura Free"}

var blondeNightSet = map[string]bool{}

func init() {
	eggRules = append(eggRules, eggRule{artist: "Frank Ocean", album: "Blonde", e: egg{
		theme: &blondeDayTheme, bg: blondeBg, cover: blondeCover, step: blondeStep,
		knob: '●', knobCol: hex(0xf2c46b)}})
	for _, t := range blondeNightTitles {
		blondeNightSet[foldTitle(t)] = true
		eggRules = append(eggRules, eggRule{artist: "Frank Ocean", album: "Blonde", title: t,
			e: egg{theme: &blondeNightTheme}})
	}
}

// blondeDayTheme is a faded print: light warm paper, dark ink.
var blondeDayTheme = theme{
	bg:   hex(0xd8cfbd),
	a1:   hex(0xc4643a),
	a2:   hex(0x8f7f66),
	a3:   hex(0x3a3127),
	text: hex(0x2b251c),
	dim:  hex(0x665c4c),
	bad:  hex(0xb23a2e),
}

var blondeNightTheme = theme{
	bg:   hex(0x110d05),
	a1:   hex(0xf2a94a),
	a2:   hex(0xd9c38f),
	a3:   hex(0xfff1d6),
	text: hex(0xf5efe0),
	dim:  hex(0x857a5c),
	bad:  hex(0xff6b5a),
}

var (
	blondeFilm  = hex(0xfff3dc)
	blondeLeakC = hex(0xffe2b0)
)

// blondeSmoke is the smoke cover of the Boys Don't Cry CD; Apple has only the main one.
//
//go:embed assets/blonde-smoke.jpg
var blondeSmoke []byte

const blondeSmokeKey = "egg:blonde-smoke"

// blondeNight: the track is on the night side of the album, or Nights is past its switch.
func blondeNight(m *Model) bool {
	t := m.curTrack()
	if blondeNightSet[foldTitle(t.Title)] {
		return true
	}
	e := m.eggOf(*t)
	return e != nil && e.switchAt > 0 && m.pos >= e.switchAt
}

// blondeLeak is the day light: a soft diagonal band that drifts across the screen every 30 s.
func blondeLeak(m *Model, x, y int) float64 {
	d := float64(x)*0.5 + float64(y)
	c := math.Mod(m.pos/30, 1)*(float64(m.w)*0.5+float64(m.h)+60) - 30
	return math.Exp(-(d - c) * (d - c) / 196)
}

// blondeCity are the city lights behind the glass: mostly warm white, some brake
// red, a rare green.
var blondeCity = []col{hex(0xfff1d6), hex(0xfff1d6), hex(0xffe4b8), hex(0xff3b30), hex(0xff3b30), hex(0x3ddc84)}

// blondeLights lays the blurred lights over one backdrop cell: fourteen of them in the
// lower two thirds, the bigger ones closer and faster, drifting right to left with the
// track; after the Nights switch they fade in.
func blondeLights(m *Model, c col, x, y int) col {
	k := 1.0
	if e := m.eggOf(*m.curTrack()); e != nil && e.switchAt > 0 {
		k = prog(m.pos, e.switchAt, 2.5)
	}
	if k == 0 {
		return c
	}
	w := float64(m.w)
	for i := 0; i < 14; i++ {
		r := 3 + 5*hash(i, 1, 51)
		span := w + 4*r
		lx := math.Mod(hash(i, 2, 51)*span-m.pos*r*0.9, span)
		if lx < 0 {
			lx += span
		}
		dx := float64(x) - (lx - 2*r)
		dy := (float64(y) - float64(m.h)*(0.35+0.5*hash(i, 3, 51))) * 2
		d := (dx*dx + dy*dy) / (r * r)
		if d > 9 {
			continue
		}
		lc := blondeCity[int(hash(i, 4, 51)*float64(len(blondeCity)))]
		c = c.lerp(lc, k*(0.16+0.18*hash(i, 5, 51))*math.Exp(-d))
	}
	return c
}

// blondeStep: past the Nights switch the ink turns light, the switch itself cuts the rest of the palette to warm.
func blondeStep(m *Model) {
	if e := m.eggOf(*m.curTrack()); e != nil && e.switchAt > 0 && m.pos >= e.switchAt {
		m.to.text, m.to.dim = blondeNightTheme.text, blondeNightTheme.dim
	}
}

func blondeBg(m *Model, c col, x, y int) col {
	if blondeNight(m) {
		return blondeLights(m, c.lerp(blondeNightTheme.bg, 0.6), x, y)
	}
	return c.lerp(blondeDayTheme.bg, 0.75).lerp(blondeFilm, 0.2).lerp(blondeLeakC, 0.45*blondeLeak(m, x, y))
}

// blondeCover: by day the track cover, washed out under the drifting light; by night
// the smoke cover.
func blondeCover(m *Model, at func(px, py int) col, px, py int) col {
	g := m.geom()
	x, y := g.cx+px, (g.cy*2+py)/2
	if !blondeNight(m) {
		return at(px, py).lerp(blondeFilm, 0.14).lerp(blondeLeakC, 0.3*blondeLeak(m, x, y))
	}
	if _, ok := m.covers[blondeSmokeKey]; !ok {
		img, err := jpeg.Decode(bytes.NewReader(blondeSmoke))
		if err != nil {
			img = image.NewRGBA(image.Rect(0, 0, 1, 1))
		}
		m.covers[blondeSmokeKey] = img
	}
	return m.cover(blondeSmokeKey, g.cw, g.ch*2).at(px, py)
}
