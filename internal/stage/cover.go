package stage

import (
	"fmt"
	"image"
	"math"
	"sort"
)

// hashStr is the FNV-1a of the track key: seeds the procedural covers and the cover cache names.
func hashStr(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h = (h ^ uint32(s[i])) * 16777619
	}
	return h
}

type pixgrid struct {
	w, h int
	p    []col
}

func (g *pixgrid) at(x, y int) col {
	x = max(0, min(g.w-1, x))
	y = max(0, min(g.h-1, y))
	return g.p[y*g.w+x]
}

func sample(img image.Image, w, h int) *pixgrid {
	g := &pixgrid{w: w, h: h, p: make([]col, w*h)}
	b := img.Bounds()
	for y := range h {
		for x := range w {
			x0 := b.Min.X + x*b.Dx()/w
			x1 := b.Min.X + (x+1)*b.Dx()/w
			y0 := b.Min.Y + y*b.Dy()/h
			y1 := b.Min.Y + (y+1)*b.Dy()/h
			var r, gg, bb, n float64
			step := max(1, (x1-x0)/4)
			for yy := y0; yy < max(y1, y0+1); yy += step {
				for xx := x0; xx < max(x1, x0+1); xx += step {
					cr, cg, cb, _ := img.At(xx, yy).RGBA()
					r += float64(cr)
					gg += float64(cg)
					bb += float64(cb)
					n++
				}
			}
			g.p[y*w+x] = col{r / n / 65535, gg / n / 65535, bb / n / 65535}
		}
	}
	return g
}

func procedural(key string, w, h int) *pixgrid {
	s := float64(hashStr(key) % 360)
	c1, c2 := hsv(s, 0.7, 0.85), hsv(s+70, 0.8, 0.35)
	g := &pixgrid{w: w, h: h, p: make([]col, w*h)}
	for y := range h {
		for x := range w {
			u, v := float64(x)/float64(w), float64(y)/float64(h)
			c := c2.lerp(c1, (u+v)/2)
			d := math.Hypot(u-0.62, v-0.38)
			if d < 0.22 {
				c = c.lerp(hsv(s+180, 0.5, 0.95), smooth((0.22-d)*12))
			}
			g.p[y*w+x] = c
		}
	}
	return g
}

func (m *Model) cover(key string, w, h int) *pixgrid {
	ck := fmt.Sprintf("%s|%dx%d", key, w, h)
	if g, ok := m.pix[ck]; ok {
		return g
	}
	var g *pixgrid
	if img, ok := m.covers[key]; ok {
		g = sample(img, w, h)
	} else {
		g = procedural(key, w, h)
	}
	m.pix[ck] = g
	return g
}

// themeOf derives the UI palette from a cover: dominant hues become accents, the darkest tone the backdrop.
func themeOf(g *pixgrid) theme {
	type bucket struct {
		n float64
		c col
	}
	var bs [12]bucket
	var avg col
	for _, c := range g.p {
		avg = col{avg.r + c.r, avg.g + c.g, avg.b + c.b}
		h, s, v := toHSV(c)
		if s < 0.18 || v < 0.15 {
			continue
		}
		i := int(h/30) % 12
		w := s * v
		bs[i].n += w
		bs[i].c = col{bs[i].c.r + c.r*w, bs[i].c.g + c.g*w, bs[i].c.b + c.b*w}
	}
	n := float64(len(g.p))
	avg = avg.mul(1 / n)
	idx := []int{}
	for i := range bs {
		if bs[i].n > 0 {
			bs[i].c = bs[i].c.mul(1 / bs[i].n)
			idx = append(idx, i)
		}
	}
	sort.Slice(idx, func(a, b int) bool { return bs[idx[a]].n > bs[idx[b]].n })
	ah, _, _ := toHSV(avg)
	hues := []float64{}
	for _, i := range idx {
		h, _, _ := toHSV(bs[i].c)
		far := true
		for _, o := range hues {
			if d := math.Abs(math.Mod(h-o+540, 360) - 180); d < 40 {
				far = false
			}
		}
		if far {
			hues = append(hues, h)
		}
	}
	if len(hues) == 0 {
		hues = []float64{ah}
	}
	for len(hues) < 3 {
		hues = append(hues, hues[0]+float64(len(hues))*48)
	}
	h := hues[0]
	return theme{
		bg:   hsv(h, 0.35, 0.065),
		a1:   hsv(h, 0.6, 1),
		a2:   hsv(h, 0.22, 1),
		a3:   hsv(h, 0.6, 1),
		text: hsv(h, 0.04, 0.95),
		dim:  hsv(h, 0.1, 0.55),
		bad:  hex(0xff5c7a),
	}
}
