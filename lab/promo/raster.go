package main

import (
	"encoding/binary"
	"io"
	"os"
	"sync"
)

const (
	cols, rows = 160, 45
	cellW      = 24
	cellH      = 48
	srcW       = cols * cellW
	srcH       = rows * cellH
	frameW     = srcW / 2
	frameH     = srcH / 2
)

type glyphKey struct {
	r    rune
	bold bool
}

type atlas map[glyphKey]*[cellW * cellH]byte

func loadAtlas(path string) (atlas, error) {
	f, err := os.Open(path) //nolint:gosec // G304: paths come from the flags of a local tool
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var n uint32
	if err := binary.Read(f, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	a := make(atlas, n)
	head := make([]byte, 5)
	for range n {
		if _, err := io.ReadFull(f, head); err != nil {
			return nil, err
		}
		g := new([cellW * cellH]byte)
		if _, err := io.ReadFull(f, g[:]); err != nil {
			return nil, err
		}
		a[glyphKey{rune(binary.LittleEndian.Uint32(head)), head[4] != 0}] = g //nolint:gosec // G115: the atlas stores code points
	}
	return a, nil
}

type cell struct {
	ch     rune
	fg, bg [3]uint8
	bold   bool
}

// parseANSI reads what the stage writes to a terminal: the same bytes a user's
// terminal would draw, so the video shows the real output.
func parseANSI(s string, out *[rows * cols]cell) {
	var fg, bg [3]uint8
	var bold bool
	x, y := 0, 0
	rs := []byte(s)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch c {
		case 0x1b:
			i += 2
			var p [8]int
			n := 0
			for i < len(rs) && rs[i] != 'm' {
				if rs[i] == ';' {
					n++
				} else if n < len(p) {
					p[n] = p[n]*10 + int(rs[i]-'0')
				}
				i++
			}
			i++
			switch {
			case p[0] == 38 && p[1] == 2:
				fg = [3]uint8{uint8(p[2]), uint8(p[3]), uint8(p[4])} //nolint:gosec // G115: SGR colours are bytes
			case p[0] == 48 && p[1] == 2:
				bg = [3]uint8{uint8(p[2]), uint8(p[3]), uint8(p[4])} //nolint:gosec // G115: SGR colours are bytes
			case p[0] == 1:
				bold = true
			case p[0] == 22:
				bold = false
			case p[0] == 0:
				bold = false
			}
		case '\n':
			x, y = 0, y+1
			i++
		default:
			r, w := decodeRune(rs[i:])
			i += w
			if x < cols && y < rows {
				out[y*cols+x] = cell{r, fg, bg, bold}
			}
			x++
		}
	}
}

func decodeRune(b []byte) (rune, int) {
	c := b[0]
	switch {
	case c < 0x80:
		return rune(c), 1
	case c < 0xE0:
		return rune(c&0x1F)<<6 | rune(b[1]&0x3F), 2
	case c < 0xF0:
		return rune(c&0x0F)<<12 | rune(b[1]&0x3F)<<6 | rune(b[2]&0x3F), 3
	}
	return rune(c&0x07)<<18 | rune(b[1]&0x3F)<<12 | rune(b[2]&0x3F)<<6 | rune(b[3]&0x3F), 4
}

// raster paints the cells into an RGB frame, one goroutine per terminal row.
func raster(cells *[rows * cols]cell, a atlas, rgb []byte) {
	var wg sync.WaitGroup
	for y := range rows {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			for x := range cols {
				c := &cells[y*cols+x]
				var g *[cellW * cellH]byte
				if c.ch != ' ' && c.ch != 0 {
					g = a[glyphKey{c.ch, c.bold}]
				}
				for py := range cellH {
					o := ((y*cellH+py)*srcW + x*cellW) * 3
					for px := range cellW {
						r, gr, b := c.bg[0], c.bg[1], c.bg[2]
						if g != nil {
							if al := uint32(g[py*cellW+px]); al != 0 {
								r = uint8((uint32(c.fg[0])*al + uint32(c.bg[0])*(255-al)) / 255)  //nolint:gosec // G115: a blend of two bytes is a byte
								gr = uint8((uint32(c.fg[1])*al + uint32(c.bg[1])*(255-al)) / 255) //nolint:gosec // G115: a blend of two bytes is a byte
								b = uint8((uint32(c.fg[2])*al + uint32(c.bg[2])*(255-al)) / 255)  //nolint:gosec // G115: a blend of two bytes is a byte
							}
						}
						rgb[o], rgb[o+1], rgb[o+2] = r, gr, b
						o += 3
					}
				}
			}
		}(y)
	}
	wg.Wait()
}
