package stage

// The drawer: the music list on the right, the stage laid out in the columns the
// drawer leaves free.
const (
	musicDrawerMin = 44 // the narrowest the drawer gets
	musicDrawerMax = 72 // and the widest
)

// musicDrawerWidth is the columns the drawer takes from the stage, 0 when it is shut.
func (m *Model) musicDrawerWidth() int {
	if !m.mu.open || m.musicStyle() != musicStyleDrawer {
		return 0
	}
	dw := min(max(m.w*2/5, musicDrawerMin), musicDrawerMax)
	return min(dw, max(0, m.w-12))
}

// drawMusicDrawer paints the panel on the right edge over the narrowed stage.
func (m *Model) drawMusicDrawer(f *fb) {
	dw := m.musicDrawerWidth()
	if dw <= 0 || dw >= m.w {
		return
	}
	x := m.w - dw
	bg := m.th.bg.lerp(m.th.text, 0.07)
	for y := range m.h {
		for i := range dw {
			if c := f.at(x+i, y); c != nil {
				c.ch, c.pix, c.bold, c.ital, c.bg = ' ', false, false, false, bg
			}
		}
	}
	for y := range m.h {
		if c := f.at(x, y); c != nil {
			c.bg = bg.lerp(m.th.a1, 0.55)
		}
		if c := f.at(x+1, y); c != nil {
			c.bg = bg.lerp(m.th.a1, 0.12)
		}
	}
	inner := dw - 2
	m.mlDrawInput(f, x+1, 1, inner)
	m.mlRule(f, x+1, 2, inner)
	m.mlDrawBody(f, x+1, 3, inner, max(1, m.h-6), m.mlLines(), m.mu.list.sel)
	m.mlDrawBar(f, x+1, m.h-2, inner)
}
