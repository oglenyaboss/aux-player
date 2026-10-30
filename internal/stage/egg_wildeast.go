package stage

import (
	"math"
	"strings"
	"unicode"
)

// WILD EA$T: SALUKI stands alone in a field of people lying close together, eyes shut,
// a black-and-white photo from Tbilisi. The field runs past the cover across the whole
// screen as an old film, the camera drifting over it with the playback. On the bangers
// the crowd is on its feet: silhouettes against the haze of a stage, jumping to the
// tempo with a flash on every bar, lighters held up on Огней; on Kreem's intro, which
// opens the second part, it gets up. Snow falls on the cold songs; on Glugicir the
// film goes quiet at the bridge and the mantra comes up under the cover; the finale is
// headlights over the field, a moonlit city of the dead for Ведьма, and warm sepia for
// Home, "в самом красивом кино".
func init() {
	eggRules = append(eggRules, eggRule{artist: "SALUKI", album: "WILD EA$T", e: egg{
		theme: &weTheme, bg: weBg, ink: weInk, over: weOver,
		knob: '❦', knobCol: hex(0xeeeeee)}})
}

var weTheme = theme{
	bg:   hex(0x0b0b0b),
	a1:   hex(0xe8e8e8),
	a2:   hex(0x9a9a9a),
	a3:   hex(0xffffff),
	text: hex(0xf0f0f0),
	dim:  hex(0x8a8a8a),
	bad:  hex(0xff6a5a),
}

type weMode int

const (
	weStill weMode = iota
	weBang
	weLights
	weWake
	weSnow
	weGlug
	weDrive
	weWitch
	weHome
)

type weTrack struct {
	mode weMode
	bpm  float64 // measured on the iTunes previews
}

var weTracks = map[string]weTrack{
	"огней":                {weLights, 124.5},
	"hahaha":               {weBang, 95.3},
	"thegoodthebadtheugly": {weBang, 152},
	"звучиткакплан":        {weWake, 0},
	"laggout":              {weBang, 119},
	"встретимсяукассы":     {weBang, 117.5},
	"soloist":              {weBang, 86.5},
	"pixiesfairies":        {weBang, 137},
	"немногокруче":         {weBang, 75},
	"вылечим":              {weSnow, 0},
	"lunianassong":         {weSnow, 0},
	"таклегкозамерзнуть":   {weSnow, 0},
	"glugicir":             {weGlug, 0},
	"moskvichka":           {weDrive, 0},
	"ведьма":               {weWitch, 0},
	"home":                 {weHome, 0},
}

// Glugicir, from the synced lyrics: the quiet bridge, the mantra, the last words.
const (
	weBridge = 119.4
	weMantra = 161.7
	weLast   = 191.8
)

// weKey folds a title to its letters: "THE GOOD, THE BAD & THE UGLY" and
// "The Good the Bad & the Ugly", "Замёрзнуть" and "Замерзнуть" meet.
func weKey(title string) string {
	s := foldTitle(title)
	if i := strings.Index(s, " ("); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 'ё':
			b.WriteRune('е')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func weNow(m *Model) weTrack { return weTracks[weKey(m.curTrack().Title)] }

func weGrey(c col) col {
	l := c.lum()
	return col{l, l, l}
}

func weBg(m *Model, c col, x, y int) col { return weGrey(c).lerp(weTheme.bg, 0.85) }

// weInk: the title in the white of the calligraphy.
func weInk(m *Model, c col, u, v float64) col { return hex(0xf6f6f6).lerp(hex(0xb8b8b8), v*0.5) }

// The crowd is cut from the cover: the band of people right of the figure, above the
// lettering, mirrored to tile without seams.
const (
	weU0, weU1 = 0.62, 1.0
	weV0, weV1 = 0.0, 0.42
	weGrid     = 300
)

func weCrowdAt(g *pixgrid, x, y float64) float64 {
	w, h := (weU1-weU0)*weGrid, (weV1-weV0)*weGrid
	mirror := func(v, n float64) float64 {
		v = math.Mod(v, 2*n)
		if v < 0 {
			v += 2 * n
		}
		if v >= n {
			v = 2*n - 1 - v
		}
		return v
	}
	return g.at(int(weU0*weGrid+mirror(x, w)), int(weV0*weGrid+mirror(y, h))).lum()
}

// weCamera is how far the camera has drifted over the field, in pixels: it moves with
// the playback, so a pause holds it, and on Glugicir it comes to rest at the bridge.
func weCamera(m *Model, tr weTrack) float64 {
	p := m.pos
	if tr.mode == weGlug && p > weBridge {
		p = weBridge + 2*easeOut(prog(m.pos, weBridge, 4))
	}
	return 1.4 * p
}

// weBeat is the beat clock of a banger: beats since the start, and how much the crowd
// is moving, 0..1. Kreem's intro has no steady beat: the crowd gets up over the track,
// swaying to no tempo.
func weBeat(m *Model, tr weTrack) (phi, amp float64) {
	switch tr.mode {
	case weBang, weLights:
		return m.pos * tr.bpm / 60, smooth(m.trackT / 2)
	case weWake:
		return m.pos * 1.2, 0.3 * weRise(m)
	}
	return 0, 0
}

// weRise is how far the crowd has got up on Kreem's intro, 0..1.
func weRise(m *Model) float64 {
	d := m.curTrack().Dur
	if d <= 0 {
		d = 75
	}
	return smooth(m.pos / (0.85 * d))
}

func weStanding(tr weTrack) bool {
	return tr.mode == weBang || tr.mode == weLights || tr.mode == weWake
}

// weField is the film at one screen pixel: the crowd of the cover lying under the
// camera, or on the bangers the crowd on its feet, and the grade of the track.
func weField(m *Model, g *pixgrid, tr weTrack, px, py int) col {
	var c col
	if weStanding(tr) {
		c = weStage(m, tr, px, py)
	} else {
		cam := weCamera(m, tr)
		l := 0.03 + 0.42*weCrowdAt(g, float64(px)+cam, float64(py)+3*math.Sin(cam*0.01))
		switch tr.mode {
		case weDrive:
			l = 0.6*l + 0.6*weHeadlights(m, px, py)*(0.3+2*l)
		case weWitch:
			l *= 0.5
		}
		c = col{l, l, l}
	}
	k := 0.93 + 0.14*hash(px, py, int(m.trackT*12))*weGrain(m, tr)
	c = c.mul(k)
	if s := weScratch(m, tr, px); s > 0 {
		c = c.lerp(white, s)
	}
	switch tr.mode {
	case weWitch:
		c = c.lerp(hex(0x8fa6c8).mul(c.lum()*1.6), 0.5)
	case weHome:
		c = weSepia(c.lum()).lerp(c, 1-smooth(m.trackT/6))
	}
	return c
}

// The crowd on its feet: three rows of silhouettes against the haze of the stage,
// back to front; base is the row's feet line as a share of the screen, s its scale.
var weRows = []struct{ base, s, tone float64 }{
	{0.69, 1.0, 0.17},
	{0.80, 1.45, 0.09},
	{0.955, 2.0, 0.02},
}

// weStage is the concert at one pixel: the haze lit from the stage, flashing on the
// first beat of every bar, and the silhouettes jumping to the beat in front of it;
// on Огней the raised hands hold lighters.
func weStage(m *Model, tr weTrack, px, py int) col {
	H, W := float64(2*m.h), float64(m.w)
	x, y := float64(px), float64(py)
	phi, amp := weBeat(m, tr)
	flash := 0.0
	if tr.mode != weWake {
		bar := phi/4 - math.Floor(phi/4)
		flash = amp * math.Exp(-bar*14)
	}
	dy := (y - 0.22*H) / (0.45 * H)
	dx := (x - W/2) / (0.45 * W)
	haze := (0.05 + 0.28*math.Exp(-dy*dy)) * (0.55 + 0.45*math.Exp(-dx*dx))
	c := col{haze, haze, haze}.lerp(white, 0.35*flash)
	sink := 0.0
	if tr.mode == weWake {
		sink = (1 - weRise(m)) * 0.5 * H
	}
	cam := weCamera(m, tr)
	for r := len(weRows) - 1; r >= 0; r-- {
		row := weRows[r]
		s := row.s * H / 90
		gap := 9.5 * s
		xc := x + cam*(0.3+0.35*float64(r))
		i0 := int(math.Floor(xc / gap))
		for i := i0 - 1; i <= i0+1; i++ {
			k := hash(i, r, 86)
			pxc := (float64(i)+0.5+0.3*(k-0.5))*gap - cam*(0.3+0.35*float64(r))
			j := amp * math.Pow(math.Max(0, math.Cos(2*math.Pi*(phi-0.3*k))), 3) * 2.5 * s
			feet := row.base*H + sink - j + (hash(i, r, 87)-0.5)*2*s
			hand := hash(i, r, 88) < 0.25 || tr.mode == weLights && hash(i, r, 88) < 0.45
			if hit, flame := weFigure(x-pxc, y-feet, s, hand, hash(i, r, 89) < 0.5); hit {
				if flame > 0 && tr.mode == weLights {
					return hex(0xffc35a).lerp(hex(0xff8a2a), flame).mul(amp*(0.8+0.2*hash(i, int(m.trackT*10), 84)) + 0.2)
				}
				return col{row.tone, row.tone, row.tone}
			}
		}
	}
	return c
}

// weFigure reports whether a point (relative to the figure's feet line) falls on a
// person seen from behind: head, neck, shoulders and the body below, maybe one arm
// raised; flame is above 0 on the hand of a raised arm, where a lighter burns.
func weFigure(dx, dy, s float64, hand, right bool) (hit bool, flame float64) {
	if math.Hypot(dx, dy+5*s) < 2.3*s {
		return true, 0
	}
	if math.Abs(dx) < 1.2*s && dy > -4*s && dy < 0 {
		return true, 0
	}
	if ex, ey := dx/(5*s), (dy-3*s)/(4.5*s); ex*ex+ey*ey < 1 || dy > 3*s && math.Abs(dx) < 5*s {
		return true, 0
	}
	if !hand {
		return false, 0
	}
	hx := 3.5 * s
	if !right {
		hx = -hx
	}
	if math.Abs(dx-hx) < 0.6*s && dy > -12*s && dy < 0 {
		return true, 0
	}
	if d := math.Hypot(dx-hx, dy+12.8*s); d < 1.1*s {
		return true, 1 - d/(1.1*s)
	}
	return false, 0
}

// weGrain is how much grain the film carries: on Glugicir it settles at the bridge.
func weGrain(m *Model, tr weTrack) float64 {
	if tr.mode == weGlug {
		return 1 - 0.85*prog(m.pos, weBridge, 4)
	}
	return 1
}

// weScratch is a white scratch down the film, now and then for a few frames.
func weScratch(m *Model, tr weTrack, px int) float64 {
	f := int(m.trackT * 12)
	if hash(f/3, 0, 82) > 0.06*weGrain(m, tr) {
		return 0
	}
	if px == int(hash(f/3, 1, 82)*float64(m.w)) {
		return 0.18
	}
	return 0
}

func weSepia(l float64) col { return hex(0x1e1309).lerp(hex(0xf3dcae), clamp01(l*1.3)) }

// weHeadlights is the light of a car passing in the night over the field: two lamps
// sweeping across every nine seconds of playback.
func weHeadlights(m *Model, px, py int) float64 {
	k := m.pos/9 - math.Floor(m.pos/9)
	cx := k*float64(m.w+80) - 40
	cy := float64(2*m.h) * 0.62
	s := 0.0
	for _, dx := range []float64{-14, 14} {
		d := math.Hypot((float64(px)-cx-dx)/18, (float64(py)-cy)/9)
		s += 0.45 * math.Exp(-d*d)
	}
	return s
}

// weOver films the player: the field pixel by pixel over the backdrop, a thin veil
// of it behind text, the queue art in black and white; then snow and words.
func weOver(m *Model, f *fb, g playGeom) {
	tr := weNow(m)
	crowd := m.cover(m.curTrack().Key(), weGrid, weGrid)
	rows := m.h
	if m.typing {
		rows = m.h - 4
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < m.w; x++ {
			c := f.at(x, y)
			switch {
			case c.pix && (x < g.cx || x >= g.cx+g.cw || y < g.cy || y >= g.cy+g.ch):
				c.fg, c.bg = weGrey(c.fg), weGrey(c.bg)
			case c.pix:
			case inText(f, x, y):
				c.bg = weGrey(c.bg).lerp(weField(m, crowd, tr, x, 2*y+1), 0.3)
			default:
				f.pixel(x, 2*y, weField(m, crowd, tr, x, 2*y))
				f.pixel(x, 2*y+1, weField(m, crowd, tr, x, 2*y+1))
			}
		}
	}
	switch tr.mode {
	case weSnow:
		weSnowfall(m, f, rows)
	case weGlug:
		weWords(m, f, g)
	}
}

// weFree reports whether a pixel may be painted over: it is backdrop, not a letter, and
// off the cover.
func weFree(f *fb, g playGeom, px, py, rows int) bool {
	c := f.at(px, py/2)
	if c == nil || py/2 >= rows || !c.pix && c.ch != ' ' {
		return false
	}
	return px < g.cx || px >= g.cx+g.cw || py/2 < g.cy || py/2 >= g.cy+g.ch
}

// weSnowfall: snow drifting down over the field, frozen with the playback.
func weSnowfall(m *Model, f *fb, rows int) {
	g := m.geom()
	h := float64(2 * m.h)
	for i := range 110 {
		speed := 2.5 + 3*hash(i, 1, 85)
		px := int(math.Mod(hash(i, 0, 85)*float64(m.w)+2*math.Sin(m.pos*0.5+float64(i)), float64(m.w)))
		py := int(math.Mod(hash(i, 2, 85)*h+m.pos*speed, h))
		if px < 0 || !weFree(f, g, px, py, rows) {
			continue
		}
		f.pixel(px, py, f.pixelAt(px, py).lerp(hex(0xf4f4f4), 0.55+0.3*hash(i, 3, 85)))
	}
}

// weWords: on Glugicir the mantra comes up under the cover in spaced capitals while it
// is sung, then the last words of the track; the words of the song, in any language.
func weWords(m *Model, f *fb, g playGeom) {
	y := g.cy + g.ch + 1
	if !g.side || g.mini || g.split || y >= m.h-2 {
		return
	}
	var s string
	var k float64
	switch p := m.pos; {
	case p >= weLast:
		s, k = "Д И К И Й   В О С Т О К", prog(p, weLast, 1.5)
	case p >= weMantra:
		s, k = "О С Т А В А Й С Я   Д И К О Й", prog(p, weMantra, 1.5)
	default:
		return
	}
	if n := len([]rune(s)); n <= g.cw {
		f.textS(g.cx+(g.cw-n)/2, y, s, f.at(g.cx, y).bg.lerp(hex(0xf6f6f6), easeOut(k)), true, false)
	}
}
