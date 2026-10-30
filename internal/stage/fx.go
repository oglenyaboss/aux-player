package stage

import (
	"math"
)

func prog(t, start, dur float64) float64 { return clamp01((t - start) / dur) }
func easeOut(t float64) float64 {
	t = clamp01(t)
	return 1 - math.Pow(1-t, 3) //nolint:staticcheck // QF1005: keeps the float result pixel-exact
}
func easeIn(t float64) float64 { t = clamp01(t); return t * t * t }

func easeInOut(t float64) float64 {
	t = clamp01(t)
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2 //nolint:staticcheck // QF1005: keeps the float result pixel-exact
}

type spring struct{ x, v, to float64 }

func (s *spring) step(dt float64) {
	const k, c = 240.0, 31.0
	for range 4 {
		h := dt / 4
		s.v += (k*(s.to-s.x) - c*s.v) * h
		s.x += s.v * h
	}
}

func hash(x, y, s int) float64 {
	//nolint:gosec // G115: the wrap-around of negative coordinates is what mixes the hash
	h := uint32(x)*374761393 + uint32(y)*668265263 + uint32(s)*2246822519
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h&0xffffff) / float64(0xffffff)
}

var glyphs = []rune("!<>-_\\/[]{}=+*^?#░▒▓")

// scramble reveals s left to right; a few noise glyphs run ahead of the cursor, the rest is hidden (0).
func scramble(s string, p, t float64, seed int) []rune {
	rs := []rune(s)
	shown := int(p*float64(len(rs)) + 1e-9)
	out := make([]rune, len(rs))
	for i, r := range rs {
		switch {
		case i < shown || r == ' ':
			out[i] = r
		case i < shown+5 && p > 0:
			out[i] = glyphs[int(hash(i, int(t*24), seed)*float64(len(glyphs)-1))]
		}
	}
	if p >= 1 {
		return rs
	}
	return out
}

type particle struct {
	x, y, vx, vy float64
	ch           rune
	c            col
	age, life    float64
	grav, drag   float64
	pixel        bool

	home           bool
	sx, sy, tx, ty float64
	delay, arc     float64
	tch            rune
	onLand         func()
}

func (p *particle) step(dt float64) bool {
	p.age += dt
	if p.home {
		k := easeInOut(prog(p.age, p.delay, p.life))
		p.x = lerpf(p.sx, p.tx, k)
		p.y = lerpf(p.sy, p.ty, k) - p.arc*math.Sin(math.Pi*k)
		if p.age >= p.delay+p.life {
			if p.onLand != nil {
				p.onLand()
			}
			return false
		}
		return true
	}
	p.vy += p.grav * dt
	d := math.Pow(p.drag, dt*30)
	if p.drag == 0 {
		d = 1
	}
	p.vx *= d
	p.vy *= d
	p.x += p.vx * dt
	p.y += p.vy * dt
	return p.age < p.life
}

func (p *particle) alpha() float64 {
	if p.home {
		if p.age < p.delay {
			return 0.35
		}
		return 1
	}
	return 1 - easeIn(p.age/p.life)
}

type theme struct{ bg, a1, a2, a3, text, dim, bad col }

var night = theme{
	bg:   hex(0x0d0d10),
	a1:   hex(0x6ee7c8),
	a2:   hex(0xc9f7ea),
	a3:   hex(0x6ee7c8),
	text: hex(0xf0eeea),
	dim:  hex(0x75726d),
	bad:  hex(0xff5c7a),
}

func (a theme) lerp(b theme, t float64) theme {
	return theme{a.bg.lerp(b.bg, t), a.a1.lerp(b.a1, t), a.a2.lerp(b.a2, t), a.a3.lerp(b.a3, t), a.text.lerp(b.text, t), a.dim.lerp(b.dim, t), a.bad.lerp(b.bad, t)}
}

// grad is the single accent breathing toward its pale tint for u in [0,1).
func (a theme) grad(u float64) col {
	u -= math.Floor(u)
	return a.a1.lerp(a.a2, 0.5-0.5*math.Cos(2*math.Pi*u))
}

func hsv(h, s, v float64) col {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return col{r + m, g + m, b + m}
}

func toHSV(c col) (h, s, v float64) {
	mx := math.Max(c.r, math.Max(c.g, c.b))
	mn := math.Min(c.r, math.Min(c.g, c.b))
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	switch {
	case d == 0:
		h = 0
	case mx == c.r:
		h = 60 * math.Mod((c.g-c.b)/d, 6)
	case mx == c.g:
		h = 60 * ((c.b-c.r)/d + 2)
	default:
		h = 60 * ((c.r-c.g)/d + 4)
	}
	if h < 0 {
		h += 360
	}
	return
}

var font5 = map[rune][5]string{
	'A': {".###.", "#...#", "#####", "#...#", "#...#"},
	'U': {"#...#", "#...#", "#...#", "#...#", ".###."},
	'X': {"#...#", ".#.#.", "..#..", ".#.#.", "#...#"},
	'v': {"#...#", "#...#", "#...#", ".#.#.", "..#.."},
	'i': {"..#..", ".....", "..#..", "..#..", "..#.."},
	'b': {"#....", "####.", "#...#", "#...#", "####."},
	'e': {".###.", "#...#", "#####", "#....", ".####"},
	's': {".####", "#....", ".###.", "....#", "####."},
}

var font3 = map[rune][5]string{
	'A':  {".#.", "#.#", "###", "#.#", "#.#"},
	'B':  {"##.", "#.#", "##.", "#.#", "##."},
	'C':  {".##", "#..", "#..", "#..", ".##"},
	'D':  {"##.", "#.#", "#.#", "#.#", "##."},
	'E':  {"###", "#..", "##.", "#..", "###"},
	'F':  {"###", "#..", "##.", "#..", "#.."},
	'G':  {".##", "#..", "#.#", "#.#", ".##"},
	'H':  {"#.#", "#.#", "###", "#.#", "#.#"},
	'I':  {"###", ".#.", ".#.", ".#.", "###"},
	'J':  {"..#", "..#", "..#", "#.#", ".#."},
	'K':  {"#.#", "#.#", "##.", "#.#", "#.#"},
	'L':  {"#..", "#..", "#..", "#..", "###"},
	'M':  {"#...#", "##.##", "#.#.#", "#...#", "#...#"},
	'N':  {"#..#", "##.#", "#.##", "#..#", "#..#"},
	'O':  {".#.", "#.#", "#.#", "#.#", ".#."},
	'P':  {"##.", "#.#", "##.", "#..", "#.."},
	'Q':  {".#.", "#.#", "#.#", "##.", ".##"},
	'R':  {"##.", "#.#", "##.", "#.#", "#.#"},
	'S':  {".##", "#..", ".#.", "..#", "##."},
	'T':  {"###", ".#.", ".#.", ".#.", ".#."},
	'U':  {"#.#", "#.#", "#.#", "#.#", "###"},
	'V':  {"#.#", "#.#", "#.#", "#.#", ".#."},
	'W':  {"#...#", "#...#", "#.#.#", "##.##", "#...#"},
	'X':  {"#.#", "#.#", ".#.", "#.#", "#.#"},
	'Y':  {"#.#", "#.#", ".#.", ".#.", ".#."},
	'Z':  {"###", "..#", ".#.", "#..", "###"},
	'0':  {"###", "#.#", "#.#", "#.#", "###"},
	'1':  {".#.", "##.", ".#.", ".#.", "###"},
	'2':  {"##.", "..#", ".#.", "#..", "###"},
	'3':  {"##.", "..#", ".#.", "..#", "##."},
	'4':  {"#.#", "#.#", "###", "..#", "..#"},
	'5':  {"###", "#..", "##.", "..#", "##."},
	'6':  {".##", "#..", "###", "#.#", "###"},
	'7':  {"###", "..#", ".#.", ".#.", ".#."},
	'8':  {"###", "#.#", "###", "#.#", "###"},
	'9':  {"###", "#.#", "###", "..#", "##."},
	'!':  {".#.", ".#.", ".#.", "...", ".#."},
	'$':  {".##", "##.", ".#.", ".##", "##."},
	'\'': {".#.", ".#.", "...", "...", "..."},
	'.':  {"...", "...", "...", "...", ".#."},
	',':  {"...", "...", "...", ".#.", "#.."},
	'-':  {"...", "...", "###", "...", "..."},
	'&':  {".#.", "#.#", ".#.", "#.#", ".##"},
	'(':  {".#.", "#..", "#..", "#..", ".#."},
	')':  {".#.", "..#", "..#", "..#", ".#."},
	'?':  {"##.", "..#", ".#.", "...", ".#."},
	' ':  {"...", "...", "...", "...", "..."},
}

// vnoise is smooth value noise over the plane, 0..1.
func vnoise(x, y float64, s int) float64 {
	ix, iy := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := smooth(x-float64(ix)), smooth(y-float64(iy))
	a := lerpf(hash(ix, iy, s), hash(ix+1, iy, s), fx)
	b := lerpf(hash(ix, iy+1, s), hash(ix+1, iy+1, s), fx)
	return lerpf(a, b, fy)
}
