package stage

import (
	"embed"
	"hash/fnv"
	"image"
	"image/jpeg"
	"math"
)

func init() {
	eggRules = append(eggRules, eggRule{
		artist: "Kanye West",
		album:  "My Beautiful Dark Twisted Fantasy",
		e: egg{
			theme:   &mbdtfTheme,
			bg:      mbdtfBg,
			cover:   mbdtfCover,
			ink:     mbdtfInk,
			layout:  mbdtfLayout,
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

// mbdtfBg: on the gallery wall the red canvas, under the prompt too. In the usual
// player at the reveal the backdrop becomes a canvas-textured gallery wall
// with a warm gold halo radiating from the title.
func mbdtfBg(m *Model, c col, x, y int) col {
	if g := m.geom(); g.noCover {
		b, _ := mbdtfPlan(m)
		return mbdtfCanvas(m, b, x, 2*y+1)
	}
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

// mbdtfFrame: in the usual player of narrow windows the cover hangs in a gold frame,
// two pixel rings on each side; the frame shimmers in over the second after the reveal.
func mbdtfFrame(m *Model, f *fb, g playGeom) {
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

// The Apple cover is the censored one: an 8×8 mosaic in a thin gold frame on red
// canvas. The mosaic box as fractions of the cover.
const mbdtfMosaicU0, mbdtfMosaicU1 = 0.31, 0.69

var (
	mbdtfCanvasC = hex(0xee2843)
	mbdtfGold    = hex(0xe2bd55)
	mbdtfGoldLo  = hex(0x8a6a22)
	mbdtfDull    = hex(0x5c3d1a)
	mbdtfDullLo  = hex(0x3a2410)
	mbdtfGlint   = hex(0xfff4c8)
	mbdtfCard    = hex(0xf3ece0)
	mbdtfCardInk = hex(0x2b1d14)
	mbdtfCardDim = hex(0x7a6a5c)
	mbdtfSet     = hex(0xf4f4f1)
	mbdtfTutu    = hex(0x121212)
	mbdtfShadow  = hex(0xd6d6d1)
)

const mbdtfCredit = "George Condo, 2010" //nolint:gosec // G101: album cover credit, not a credential

// mbdtfPaintings are the other covers Condo painted for the album, the inside of the
// gold frame; the phoenix original is left out.
//
//go:embed assets/mbdtf-*.jpg
var mbdtfPaintings embed.FS

// mbdtfHung is which painting hangs for a track; "" is the censored mosaic of the
// track cover. The ballerina came from Runaway, the crown and the sword from POWER.
var mbdtfHung = map[string]string{
	"dark fantasy":                  "",
	"power":                         "crown",
	"all of the lights":             "sword",
	"all of the lights (interlude)": "sword",
	"runaway":                       "ballerina",
	"monster":                       "mouth",
}

// mbdtfRest goes round the other tracks by their title.
var mbdtfRest = []string{"", "crown", "sword", "mouth", "priest"}

func mbdtfPainting(t Track) string {
	k := foldTitle(t.Title)
	if p, ok := mbdtfHung[k]; ok {
		return p
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(k))
	//nolint:gosec // G115: mbdtfRest is a five-element literal slice, the conversion cannot overflow
	return mbdtfRest[h.Sum32()%uint32(len(mbdtfRest))]
}

// mbdtfCanvasOf is the grid inside the frame: a painting, or the track cover for the mosaic.
func mbdtfCanvasOf(m *Model, name string, n int) *pixgrid {
	if name == "" {
		return m.cover(m.curTrack().Key(), 200, 200)
	}
	key := "egg:mbdtf-" + name
	if _, ok := m.covers[key]; !ok {
		var img image.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
		if f, err := mbdtfPaintings.Open("assets/mbdtf-" + name + ".jpg"); err == nil {
			if d, err := jpeg.Decode(f); err == nil {
				img = d
			}
			_ = f.Close()
		}
		m.covers[key] = img
	}
	return m.cover(key, n, n)
}

// mbdtfBox is the gallery wall: the mosaic in pixels, the label beside it in cells.
type mbdtfBox struct {
	mx, my, ms, cell int
	lx, ly, lw, lh   int
	white            bool // Runaway: the white set of the film, the ballerinas on it
}

// mbdtfPlan hangs the painting as big as the window height allows and the wall label
// right of it, the pair centred. ok is false when the window is too small.
func mbdtfPlan(m *Model) (b mbdtfBox, ok bool) {
	tk := m.curTrack()
	b.lw = min(30, max(len([]rune(tk.Title)), len([]rune(tk.Artist)), len(mbdtfCredit), 13)+4)
	b.lh = 7
	gap := 6
	b.ms = min((2*m.h-16)/8*8, (m.w-b.lw-gap-10)/8*8)
	b.cell = b.ms / 8
	fw := b.ms + 4
	x0 := (m.w - fw - gap - b.lw) / 2
	b.mx = x0 + 2
	b.my = (2*(m.h-2)-fw)/2 + 2
	b.lx = x0 + fw + gap
	b.ly = (b.my+b.ms/2)/2 - 1
	b.white = foldTitle(tk.Title) == "runaway"
	return b, b.ms >= 32
}

// mbdtfLayout takes the whole screen for the wall: no cover and no queue. Narrow and
// portrait windows keep the usual player with the framed cover.
func mbdtfLayout(m *Model, g playGeom) playGeom {
	if !g.side || g.mini || g.split {
		return g
	}
	if _, ok := mbdtfPlan(m); !ok {
		return g
	}
	g.noCover, g.noQueue = true, true
	return g
}

// mbdtfCanvas is the red of the cover with diagonal brush strokes, lit by a soft
// gallery spot on the painting.
func mbdtfCanvas(m *Model, b mbdtfBox, px, py int) col {
	d := (float64(px)*0.6 - float64(py)) / 5
	i := int(math.Floor(d))
	stroke := hash(i, 0, 81) + (hash(i+1, 0, 81)-hash(i, 0, 81))*smooth(d-float64(i))
	cx, cy := float64(b.mx+b.ms/2), float64(b.my+b.ms/2)
	dx, dy := (float64(px)-cx)/(float64(m.w)*0.55), (float64(py)-cy)/(float64(m.h)*1.6)
	light := math.Exp(-(dx*dx + dy*dy))
	if b.white {
		return mbdtfSet.mul((0.975 + 0.025*hash(px, py, 82)) * (0.86 + 0.14*light))
	}
	c := mbdtfCanvasC.mul(0.93 + 0.08*stroke + 0.04*hash(px, py, 82))
	return c.mul(0.72 + 0.28*light)
}

// mbdtfAlong is where a frame pixel sits along the frame, clockwise from the top left, 0..1.
func mbdtfAlong(b mbdtfBox, px, py int) float64 {
	s := float64(b.ms + 3)
	fx, fy := float64(px-b.mx+2), float64(py-b.my+2)
	var t float64
	switch {
	case fy <= 1 && fx < s-1:
		t = fx
	case fx >= s-1 && fy < s-1:
		t = s + fy
	case fy >= s-1 && fx > 1:
		t = 2*s + s - fx
	default:
		t = 3*s + s - fy
	}
	return t / (4 * s)
}

// mbdtfWall draws the screen: the painting of the track blown up on the red canvas,
// falling in block by block, its frame gilding clockwise as the track plays, the
// wall label beside it.
func mbdtfWall(m *Model, f *fb, b mbdtfBox) {
	rows := m.h
	if m.typing {
		rows = m.h - 4
	}
	for py := 0; py < rows*2; py++ {
		for px := 0; px < m.w; px++ {
			f.pixel(px, py, mbdtfCanvas(m, b, px, py))
		}
	}

	if b.white {
		mbdtfDancers(m, f, b)
	}

	tk := m.curTrack()
	frac := 0.0
	if tk.Dur > 0 {
		frac = min(1, m.pos/tk.Dur)
	}
	for py := b.my - 2; py < b.my+b.ms+2; py++ {
		for px := b.mx - 2; px < b.mx+b.ms+2; px++ {
			in := px >= b.mx && px < b.mx+b.ms && py >= b.my && py < b.my+b.ms
			if in {
				continue
			}
			outer := px < b.mx-1 || px > b.mx+b.ms || py < b.my-1 || py > b.my+b.ms
			t := mbdtfAlong(b, px, py)
			c := mbdtfDull
			if outer {
				c = mbdtfDullLo
			}
			if t < frac {
				c = mbdtfGold
				if outer {
					c = mbdtfGoldLo
				}
			}
			if frac > 0 && frac < 1 && math.Abs(t-frac) < 0.006 {
				c = mbdtfGlint
			}
			f.pixel(px, py, c)
		}
	}

	name := mbdtfPainting(*tk)
	cg := mbdtfCanvasOf(m, name, b.ms)
	span := mbdtfMosaicU1 - mbdtfMosaicU0
	for j := range 8 {
		for i := range 8 {
			shown := m.trackT >= 0.15+hash(i, j, 71)
			u, v := mbdtfMosaicU0+(float64(i)+0.5)*span/8, mbdtfMosaicU0+(float64(j)+0.5)*span/8
			block := cg.at(int(u*200), int(v*200))
			for py := 0; py < b.cell; py++ {
				for px := 0; px < b.cell; px++ {
					x, y := i*b.cell+px, j*b.cell+py
					c := mbdtfDullLo.mul(0.6)
					switch {
					case !shown:
					case name == "":
						c = block
					default:
						c = cg.at(x, y)
					}
					f.pixel(b.mx+x, b.my+y, c)
				}
			}
		}
	}

	card, ink, dim, help := mbdtfCard, mbdtfCardInk, mbdtfCardDim, mbdtfCard
	if b.white {
		card, ink, dim, help = mbdtfTutu, mbdtfSet, hex(0x9a9a96), hex(0x6a6a66)
	}
	f.fill(b.lx, b.ly, b.lw, b.lh, card)
	tw := b.lw - 4
	title := trunc(tk.Title, tw)
	f.textS(b.lx+2, b.ly+1, title, ink, true, false)
	f.text(b.lx+2, b.ly+2, trunc(tk.Artist, tw), ink)
	f.textS(b.lx+2, b.ly+4, trunc(mbdtfCredit, tw), dim, false, true)
	f.text(b.lx+2, b.ly+5, fmtDur(m.pos)+" / "+fmtDur(tk.Dur), dim)
	m.titleBox = image.Rect(b.lx+2, (b.ly+1)*2, b.lx+2+len([]rune(title)), (b.ly+1)*2+2)
	m.barBox = image.Rect(b.lx+2, b.ly+5, b.lx+b.lw-2, b.ly+6)

	if !m.typing {
		keys := tr("? клавиши", "? keys")
		f.text(3, m.h-2, tr("/ сказать агенту", "/ talk to the agent"), help)
		f.text(m.w-3-len([]rune(keys)), m.h-2, keys, help)
	}
}

// mbdtfPoses are the ballerinas of the Runaway film in black tutus: an arabesque
// facing right and a turn on pointe with the arms overhead.
var mbdtfPoses = [2][]string{{
	"...........#......",
	"..........###.....",
	"..........###.....",
	"...........#......",
	".....#############",
	"..........###.....",
	"..........###.....",
	".........#####....",
	"......###########.",
	".....#############",
	"###########.......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
	"...........#......",
}, {
	"......###......",
	".....#...#.....",
	"....#..#..#....",
	"....#.###.#....",
	".....#####.....",
	"......###......",
	".......#.......",
	"......###......",
	"......###......",
	"......###......",
	".....#####.....",
	"..###########..",
	".#############.",
	"..###########..",
	"......#.#......",
	"......#.#......",
	"......#.#......",
	"......#.#......",
	"......#.#......",
	".......#.......",
}}

// mbdtfDancers: five ballerinas cross the set behind the painting, holding an
// arabesque, then turning on pointe; they move with the playback, so a pause freezes
// them like the end of the scene in the film.
func mbdtfDancers(m *Model, f *fb, b mbdtfBox) {
	floor := b.my + b.ms + 1
	span := float64(m.w + 30)
	for i := range 5 {
		x := int(math.Mod(hash(i, 1, 91)*span+float64(i)*span/5+m.pos*3, span)) - 15
		pose := mbdtfPoses[0]
		if math.Mod(m.pos/2.4+hash(i, 2, 91), 1) > 0.6 {
			pose = mbdtfPoses[1]
		}
		h := len(pose)
		for dx := -4; dx <= 18; dx++ {
			if math.Abs(float64(dx)-7) < 7 {
				f.pixel(x+dx, floor, mbdtfShadow)
			}
		}
		for py, row := range pose {
			for px, r := range row {
				if r == '#' {
					f.pixel(x+px, floor-h+py, mbdtfTutu)
				}
			}
		}
	}
}

func mbdtfOver(m *Model, f *fb, g playGeom) {
	if !g.noCover {
		mbdtfFrame(m, f, g)
		return
	}
	b, _ := mbdtfPlan(m)
	mbdtfWall(m, f, b)
}
