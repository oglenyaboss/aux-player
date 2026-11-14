package stage

// The cover grid of the Albums and Playlists sections: square covers with the title
// and the artist under each, walked in two dimensions.
const (
	mlTileW    = 12 // the cover, in cells
	mlTileH    = 6  // half-block rows: 12 pixels tall
	mlTileGap  = 2
	mlTileRows = mlTileH + 2 // the cover and the two text rows under it
)

// mlGridCols is how many tiles fit the list area, the one-cell air on both sides included.
func (m *Model) mlGridCols(w int) int {
	return max(1, (w-2+mlTileGap)/(mlTileW+mlTileGap))
}

// mlGridColsNow is the column count of the palette at its current width.
func (m *Model) mlGridColsNow() int {
	inner := m.mlPaletteW() - 2
	return m.mlGridCols(m.mlListW(inner))
}

// mlGridShown reports whether the list area is the cover grid right now.
func (m *Model) mlGridShown() bool {
	if m.musicStyle() != musicStylePalette || m.mu.mode != mlModeMusic {
		return false
	}
	c := m.mlState()
	return c.grid && len(c.items) > 0
}

// mlClampGrid keeps the highlight inside the grid when a resize changes the columns
// or the rows arrive shorter than before.
func (m *Model) mlClampGrid() {
	if !m.mlGridShown() {
		return
	}
	n := len(m.mlState().items)
	if n == 0 {
		m.mu.list.sel = 0
		return
	}
	m.mu.list.sel = max(0, min(m.mu.list.sel, n-1))
}

// mlGridKey walks the tiles: left and right wrap inside a row, down from a column
// with no tile under it lands on the last tile, resizing clamps the highlight.
func (m *Model) mlGridKey(k string) bool {
	n := len(m.mlState().items)
	if n == 0 {
		return true
	}
	cols := m.mlGridColsNow()
	i := max(0, min(m.mu.list.sel, n-1))
	switch k {
	case "left":
		row := i / cols * cols
		if i == row {
			i = min(n, row+cols) - 1
		} else {
			i--
		}
	case "right":
		row := i / cols * cols
		if i+1 < min(n, row+cols) {
			i++
		} else {
			i = row
		}
	case "up":
		if i-cols >= 0 {
			i -= cols
		}
	case "down":
		if i+cols < n {
			i += cols
		} else {
			i = n - 1
		}
	case "home":
		i = 0
	case "end":
		i = n - 1
	case "pgup":
		i = max(0, i-cols*(m.mlPage()-1))
	case "pgdown":
		i = min(n-1, i+cols*(m.mlPage()-1))
	default:
		return false
	}
	if i != m.mu.list.sel {
		m.mu.list.sel = i
		m.mu.selT = m.t
	}
	return true
}

// mlDrawGrid paints the tiles of the list into the w×h area at (x, y), scrolled so the
// selected tile stays in view and the rest faded behind it.
func (m *Model) mlDrawGrid(f *fb, x, y, w, h int, lines []mlLine, sel int) {
	cols := m.mlGridCols(w)
	vis := max(1, h/mlTileRows)
	top := m.mu.gridTop
	if sel >= 0 && sel < len(lines) {
		row := sel / cols
		if row < top {
			top = row
		}
		if row >= top+vis {
			top = row - vis + 1
		}
	}
	top = max(0, min(top, max(0, (len(lines)-1)/cols-vis+1)))
	m.mu.gridTop = top
	gx := x + 1
	for i, l := range lines {
		if l.kind != mlTile {
			continue
		}
		row := i / cols
		if row < top {
			continue
		}
		tx, ty := gx+(i%cols)*(mlTileW+mlTileGap), y+(row-top)*mlTileRows
		if ty >= y+h {
			break
		}
		g := m.cover(l.it.Key(), mlTileW, mlTileH*2)
		for py := range mlTileH * 2 {
			for px := range mlTileW {
				f.pixel(tx+px, ty*2+py, g.at(px, py))
			}
		}
		if i == sel {
			m.glowBox(f, tx, ty, mlTileW, mlTileH, 1)
			m.mu.rowY, m.mu.rowX = ty, tx+mlTileW
			m.mu.tileX, m.mu.tileY = tx, ty
		}
		fg := m.th.text
		if i == sel {
			fg = m.th.a1
		}
		if ty+mlTileH < y+h {
			f.textS(tx, ty+mlTileH, trunc(l.it.Title, mlTileW), fg, i == sel, false)
		}
		if ty+mlTileH+1 < y+h {
			mark, mc, egg := m.mlEggMark(l.it)
			aw := mlTileW
			if egg {
				aw -= 2
				f.put(tx+mlTileW-1, ty+mlTileH+1, mark, mc)
			}
			f.text(tx, ty+mlTileH+1, trunc(l.it.Artist, aw), m.th.dim)
		}
	}
	bg := m.th.bg.lerp(m.th.text, 0.07)
	for i, l := range lines {
		if l.kind != mlTile || i == sel {
			continue
		}
		row := i / cols
		if row < top {
			continue
		}
		tx, ty := gx+(i%cols)*(mlTileW+mlTileGap), y+(row-top)*mlTileRows
		if ty >= y+h {
			break
		}
		fadeRect(f, tx, ty, mlTileW, mlTileH, bg, 0.35)
		if ty+mlTileH < y+h {
			fadeRect(f, tx, ty+mlTileH, mlTileW, 1, bg, 0.35)
		}
		if ty+mlTileH+1 < y+h {
			fadeRect(f, tx, ty+mlTileH+1, mlTileW, 1, bg, 0.35)
		}
	}
	for i := range lines {
		row := i / cols
		if row < top {
			continue
		}
		tx, ty := gx+(i%cols)*(mlTileW+mlTileGap), y+(row-top)*mlTileRows
		if ty >= y+h {
			break
		}
		m.mlCascade(f, tx, ty, mlTileW, mlTileH+2, (row-top)*cols+i%cols)
	}
}
