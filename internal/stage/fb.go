package stage

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type col struct{ r, g, b float64 }

func hex(h uint32) col {
	return col{float64(h>>16&255) / 255, float64(h>>8&255) / 255, float64(h&255) / 255}
}

func (a col) lerp(b col, t float64) col {
	t = clamp01(t)
	return col{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func (a col) mul(k float64) col     { return col{a.r * k, a.g * k, a.b * k} }
func (a col) lum() float64          { return 0.2126*a.r + 0.7152*a.g + 0.0722*a.b }
func (a col) css() string           { return fmt.Sprintf("#%02x%02x%02x", q8(a.r), q8(a.g), q8(a.b)) }
func (a col) q() [3]int             { return [3]int{q8(a.r), q8(a.g), q8(a.b)} }
func q8(x float64) int              { return int(clamp01(x)*255 + 0.5) }
func smooth(x float64) float64      { x = clamp01(x); return x * x * (3 - 2*x) }
func lerpf(a, b, t float64) float64 { return a + (b-a)*t }

func clamp01(x float64) float64 {
	if x < 0 || math.IsNaN(x) {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

var white = col{1, 1, 1}

type cell struct {
	ch         rune
	fg, bg     col
	bold, ital bool
	pix        bool
}

type fb struct {
	w, h int
	c    []cell
}

func newFB(w, h int) *fb { return &fb{w: w, h: h, c: make([]cell, w*h)} }

func (f *fb) clone() *fb {
	g := *f
	g.c = append([]cell(nil), f.c...)
	return &g
}

func (f *fb) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= f.w || y >= f.h {
		return nil
	}
	return &f.c[y*f.w+x]
}

func (f *fb) put(x, y int, ch rune, fg col) { f.style(x, y, ch, fg, false, false) }

func (f *fb) style(x, y int, ch rune, fg col, bold, ital bool) {
	if c := f.at(x, y); c != nil {
		c.ch, c.fg, c.bold, c.ital, c.pix = ch, fg, bold, ital, false
	}
}

func (f *fb) tint(x, y int, to col, k float64) {
	if c := f.at(x, y); c != nil {
		c.bg = c.bg.lerp(to, k)
		if c.pix {
			c.fg = c.fg.lerp(to, k)
		}
	}
}

func (f *fb) fill(x, y, w, h int, bg col) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if c := f.at(xx, yy); c != nil {
				*c = cell{ch: ' ', bg: bg}
			}
		}
	}
}

// blank wipes the glyphs of w cells from (x, y); the backgrounds stay.
func (f *fb) blank(x, y, w int) {
	for i := 0; i < w; i++ {
		if c := f.at(x+i, y); c != nil {
			c.ch, c.pix, c.bold, c.ital = ' ', false, false, false
		}
	}
}

func (f *fb) text(x, y int, s string, fg col) int { return f.textS(x, y, s, fg, false, false) }

func (f *fb) textS(x, y int, s string, fg col, bold, ital bool) int {
	n := 0
	for _, r := range s {
		f.style(x+n, y, r, fg, bold, ital)
		n++
	}
	return n
}

// fade draws text blended into the cell background by alpha.
func (f *fb) fade(x, y int, s string, fg col, a float64, bold bool) int {
	n := 0
	for _, r := range s {
		if c := f.at(x+n, y); c != nil && r != ' ' {
			f.style(x+n, y, r, c.bg.lerp(fg, a), bold, false)
		}
		n++
	}
	return n
}

// pixel paints one half-block pixel: cell (px, py/2), top or bottom half.
func (f *fb) pixel(px, py int, c col) {
	if py < 0 {
		return
	}
	p := f.at(px, py/2)
	if p == nil {
		return
	}
	if !p.pix {
		p.pix, p.ch, p.fg, p.bold, p.ital = true, '▀', p.bg, false, false
	}
	if py%2 == 0 {
		p.fg = c
	} else {
		p.bg = c
	}
}

func (f *fb) pixelAt(px, py int) col {
	p := f.at(px, py/2)
	if p == nil {
		return col{}
	}
	if p.pix && py%2 == 0 {
		return p.fg
	}
	return p.bg
}

func (f *fb) String() string {
	var b strings.Builder
	b.Grow(f.w * f.h * 20)
	num := func(pre string, v [3]int) {
		b.WriteString(pre)
		b.WriteString(strconv.Itoa(v[0]))
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(v[1]))
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(v[2]))
		b.WriteByte('m')
	}
	for y := 0; y < f.h; y++ {
		lf, lb := [3]int{-1}, [3]int{-1}
		bold, ital := false, false
		for x := 0; x < f.w; x++ {
			c := &f.c[y*f.w+x]
			ch := c.ch
			if ch == 0 {
				ch = ' '
			}
			if c.bold != bold {
				if c.bold {
					b.WriteString("\x1b[1m")
				} else {
					b.WriteString("\x1b[22m")
				}
				bold = c.bold
			}
			if c.ital != ital {
				if c.ital {
					b.WriteString("\x1b[3m")
				} else {
					b.WriteString("\x1b[23m")
				}
				ital = c.ital
			}
			if ch != ' ' {
				if fg := c.fg.q(); fg != lf {
					num("\x1b[38;2;", fg)
					lf = fg
				}
			}
			if bg := c.bg.q(); bg != lb {
				num("\x1b[48;2;", bg)
				lb = bg
			}
			b.WriteRune(ch)
		}
		b.WriteString("\x1b[0m")
		if y < f.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// png renders the frame through SVG for offline review of the prototype.
func (f *fb) png(path string) error {
	const cw, ch = 9, 18
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" font-family="FiraCode Nerd Font Mono" font-size="15">`, f.w*cw, f.h*ch)
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; {
			c := f.c[y*f.w+x]
			run := 1
			for x+run < f.w && f.c[y*f.w+x+run].bg.q() == c.bg.q() {
				run++
			}
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x*cw, y*ch, run*cw, ch, c.bg.css())
			x += run
		}
	}
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			c := f.c[y*f.w+x]
			switch {
			case c.pix:
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x*cw, y*ch, cw, ch/2, c.fg.css())
			case c.ch != 0 && c.ch != ' ':
				w := ""
				if c.bold {
					w = ` font-weight="bold"`
				}
				if c.ital {
					w += ` font-style="italic"`
				}
				s := string(c.ch)
				s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
				fmt.Fprintf(&b, `<text x="%d" y="%d" fill="%s"%s>%s</text>`, x*cw, y*ch+14, c.fg.css(), w, s)
			}
		}
	}
	b.WriteString("</svg>")
	svg := strings.TrimSuffix(path, ".png") + ".svg"
	if err := os.WriteFile(svg, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return exec.Command("magick", svg, path).Run()
}
