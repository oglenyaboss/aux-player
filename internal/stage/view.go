package stage

import (
	"fmt"
	"image"
	"math"
	"strings"
)

func (m *Model) render() *fb {
	f := newFB(m.w, m.h)
	m.backdrop(f)
	switch m.ph {
	case phIntro:
		m.drawIntro(f)
	case phLaunch:
		m.drawLaunch(f)
	case phAsk:
		m.drawChips(f)
		m.drawAsk(f)
	case phScan, phDone:
		m.drawChips(f)
		m.drawScan(f)
	case phPlay:
		m.drawPlay(f)
		m.drawChips(f)
	case phOnboard:
		m.drawOnboard(f)
	}
	m.drawParticles(f)
	if m.snap != nil && m.snap.w == f.w && m.snap.h == f.h {
		m.dissolve(f)
	}
	if m.help {
		m.drawHelp(f)
	}
	m.lastShot = f
	return f
}

// backdrop: a slow, barely-there tint of the accent, plus bloom; in play
// phase the whole field flows in the colors of the current track's cover.
func (m *Model) backdrop(f *fb) {
	th := m.th
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			fx, fy := float64(x), float64(y)*2
			var bg col
			if m.ph == phPlay && m.queue != nil {
				u, v := fx/float64(f.w), fy/float64(f.h*2)
				bg = m.coverField(m.queue[m.cur].Key(), u, v)
				if m.prev != m.cur {
					bg = m.coverField(m.queue[m.prev].Key(), u, v).lerp(bg, easeInOut(prog(m.trackT, 0, 1.4)))
				}
				bg = m.eggBackdrop(bg, x, y, float64(y)/float64(f.h)).mul(1 - m.pause)
			} else {
				n := math.Sin(fx*0.035+m.t*0.08) + math.Sin(fy*0.05-m.t*0.06) + math.Sin((fx+fy)*0.025+m.t*0.04)
				bg = th.bg.lerp(th.a1, smooth((n+1)/4)*0.07)
			}
			vx := math.Abs(fx/float64(f.w)-0.5) * 2
			vy := math.Abs(fy/float64(f.h*2)-0.4) * 2
			vig := 1 - 0.4*(vx*vx*0.6+vy*vy*0.8)
			bg = bg.lerp(th.a1, m.bloom*0.12).mul(vig * m.dim.x)
			c := &f.c[y*f.w+x]
			c.bg, c.ch = bg, ' '
		}
	}
}

// coverField: one bilinear sample of a small cover grid, warped slowly over time,
// tone-capped and blended into the theme background.
func (m *Model) coverField(key string, u, v float64) col {
	g := m.cover(key, 6, 6)
	uu := u + 0.06*math.Sin(m.t*0.11+v*4.0)
	vv := v + 0.06*math.Cos(m.t*0.09+u*3.5)
	fx, fy := uu*float64(g.w-1), vv*float64(g.h-1)
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := fx-float64(x0), fy-float64(y0)
	c := g.at(x0, y0).lerp(g.at(x0+1, y0), tx).lerp(g.at(x0, y0+1).lerp(g.at(x0+1, y0+1), tx), ty)
	if peak := math.Max(c.r, math.Max(c.g, c.b)); peak > 0.2 {
		c = c.mul(0.2 / peak)
	}
	return m.th.bg.lerp(c, 0.8)
}

func (m *Model) logoBounds() (x0, x1 float64) {
	x0, x1 = 1e9, -1e9
	for _, p := range m.logo {
		x0, x1 = math.Min(x0, p.tx), math.Max(x1, p.tx)
	}
	return
}

// logoColor: accent, with one highlight sweep right after the letters land.
func (m *Model) logoColor(tx, x0, x1 float64) col {
	u := (tx - x0) / math.Max(1, x1-x0)
	s := prog(m.pt, 1.5, 1.1)*1.6 - 0.3
	boost := math.Exp(-(u - s) * (u - s) / 0.006)
	if m.ph != phIntro {
		boost = 0
	}
	return m.th.a1.lerp(m.th.a2, 0.25).lerp(white, 0.8*boost)
}

func (m *Model) drawIntro(f *fb) {
	x0, x1 := m.logoBounds()
	ly := 0.0
	for _, p := range m.logo {
		ly = math.Max(ly, p.ty)
	}
	for _, p := range m.logo {
		k := easeOut(prog(m.pt, p.delay, p.life))
		x, y := lerpf(p.sx, p.tx, k), lerpf(p.sy, p.ty, k)
		c := m.logoColor(p.tx, x0, x1)
		if k < 1 {
			c = f.pixelAt(int(x), int(y)).lerp(c, 0.2+0.8*k)
		}
		f.pixel(int(math.Round(x)), int(math.Round(y)), c)
	}
	cx := m.w / 2
	tag := tr("скажи, что хочешь услышать", "say what you want to hear")
	f.fade(cx-len([]rune(tag))/2, int(ly/2)+2, tag, m.th.dim, easeOut(prog(m.pt, 1.5, 0.8)), false)

	bx, by, bw := m.inputBox()
	a := easeOut(prog(m.pt, 1.7, 0.7))
	m.glowBox(f, bx, by, bw, 3, a)
	f.fade(bx+2, by+1, "›", m.th.a1, a, true)
	if len(m.input) == 0 {
		f.fade(bx+4, by+1, tr("что хочется послушать?", "what do you feel like hearing?"), m.th.dim, a*0.8, false)
	} else {
		in := m.input
		if over := len(in) - (bw - 7); over > 0 {
			in = in[over:]
		}
		f.text(bx+4, by+1, string(in), m.th.text)
	}
	cur := bx + 4 + min(len(m.input), bw-7)
	blink := 0.5 + 0.5*math.Sin(m.t*6)
	f.fade(cur, by+1, "▎", m.th.a1, a*(0.35+0.65*blink), false)
	if m.err != "" {
		f.fade(bx+2, by+3, trunc(m.err, bw), m.th.bad, 1, false)
	}
	if m.o.Example != nil {
		if hint := trunc(tr("например: ", "e.g. ")+m.o.Example(), m.w-6); hint != "" {
			f.fade(cx-len([]rune(hint))/2, by+4, hint, m.th.dim, a*0.6, false)
		}
	}
	hint := tr("tab настройки", "tab settings")
	f.fade(m.w-3-len([]rune(hint)), m.h-2, hint, m.th.dim, a*0.7, false)
}

func (m *Model) drawLaunch(f *fb) {
	x0, x1 := m.logoBounds()
	for _, p := range m.logo {
		if p.age < p.life {
			c := m.logoColor(p.tx, x0, x1)
			px, py := int(math.Round(p.x)), int(math.Round(p.y))
			f.pixel(px, py, f.pixelAt(px, py).lerp(c, p.alpha()))
		}
	}
	bx, by, bw := m.inputBox()
	if a := 1 - prog(m.pt, 0, 0.35); a > 0 {
		m.glowBox(f, bx, by, bw, 3, a)
	}
	m.drawChips(f)
	if m.pt > 0.35 {
		shrink := 0.0
		if m.launchOut >= 0 {
			shrink = easeIn(prog(m.pt, m.launchOut, 0.25))
		}
		grow := easeOut(prog(m.pt, 0.35, 0.5)) * (1 - shrink)
		m.orb(f, float64(m.w)/2, float64(m.h), grow)
		if grow > 0.3 {
			s := tr("агент думает", "agent is thinking")
			if m.status != "" {
				s = trunc(m.status, m.w-6)
			}
			m.wave(f, m.w/2-len([]rune(s))/2, m.h/2+5, s, m.th.text)
		}
	}
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}

type pt struct{ x, y int }

func perim(x, y, w, h int) []pt {
	var p []pt
	for i := 0; i < w; i++ {
		p = append(p, pt{x + i, y})
	}
	for j := 1; j < h-1; j++ {
		p = append(p, pt{x + w - 1, y + j})
	}
	for i := w - 1; i >= 0; i-- {
		p = append(p, pt{x + i, y + h - 1})
	}
	for j := h - 2; j >= 1; j-- {
		p = append(p, pt{x, y + j})
	}
	return p
}

func boxRune(q pt, x, y, w, h int) rune {
	r, b := x+w-1, y+h-1
	switch {
	case q.x == x && q.y == y:
		return '╭'
	case q.x == r && q.y == y:
		return '╮'
	case q.x == x && q.y == b:
		return '╰'
	case q.x == r && q.y == b:
		return '╯'
	case q.y == y || q.y == b:
		return '─'
	}
	return '│'
}

func (m *Model) glowBox(f *fb, x, y, w, h int, a float64) {
	if a <= 0 {
		return
	}
	c := m.th.a1.lerp(m.th.bg, 0.4)
	for _, q := range perim(x, y, w, h) {
		if p := f.at(q.x, q.y); p != nil {
			f.put(q.x, q.y, boxRune(q, x, y, w, h), p.bg.lerp(c, a))
		}
	}
}

// drawChips: the plan as quiet words; a fresh one lights up in the accent and settles.
func (m *Model) drawChips(f *fb) {
	right := "aux"
	if m.ph == phPlay {
		right = fmt.Sprintf("%d / %d   aux", m.cur+1, len(m.queue))
	}
	rx := m.w - 3 - len([]rune(right))
	f.fade(rx, 1, right, m.th.dim, 0.8, false)
	for i, c := range m.chips {
		if m.t < c.born {
			continue
		}
		x := m.chipX(i) + 1
		n := len([]rune(c.label))
		if x+n > rx-2 {
			break
		}
		fresh := math.Exp(-(m.t - c.born) * 1.5)
		f.text(x, 1, c.label, m.th.dim.lerp(m.th.text, 0.45).lerp(m.th.a1, fresh))
	}
}

func (m *Model) drawAsk(f *fb) {
	q := m.warmups[m.qi]
	x, y, w, h := m.card()
	in := easeOut(prog(m.qT, 0, 0.5))
	out := 0.0
	if m.answered {
		out = easeIn(prog(m.ansT, 0.25, 0.4))
	}
	if out >= 1 {
		return
	}
	if out > 0 {
		nh := max(1, int(float64(h)*(1-out)))
		y += (h - nh) / 2
		h = nh
	}
	a := in * (1 - out)
	cardBg := m.th.bg.lerp(m.th.text, 0.05)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if p := f.at(xx, yy); p != nil {
				p.bg, p.ch, p.pix = p.bg.lerp(cardBg, a), ' ', false
			}
		}
	}
	per := perim(x, y, w, h)
	n := int(float64(len(per)) * easeOut(prog(m.qT, 0, 0.4)))
	border := m.th.a1.lerp(cardBg, 0.35)
	for k := 0; k < n && k < len(per); k++ {
		q := per[k]
		if p := f.at(q.x, q.y); p != nil {
			f.put(q.x, q.y, boxRune(q, x, y, w, h), p.bg.lerp(border, 1-out))
		}
	}
	if out > 0.3 {
		return
	}
	ca := prog(m.qT, 0.35, 0.3) * (1 - out*3)
	if len(m.warmups) > 1 {
		f.fade(x+w-7, y+1, fmt.Sprintf("%d / %d", m.qi+1, len(m.warmups)), m.th.dim, ca, false)
	}
	f.fade(x+3, y+2, trunc(q.Title, w-6), m.th.text, easeOut(prog(m.qT, 0.25, 0.35)), true)
	if q.Scale {
		m.drawScale(f, q, x, y, w, ca)
		f.fade(x+3, y+h-2, tr("←→ степень   enter ответ", "←→ scale   enter answer"), m.th.dim, ca*0.8, false)
		return
	}
	if q.trackCard() {
		m.drawTrackOptions(f, q, x, y, w, ca)
		if m.cardTyping {
			tail := m.cardText
			if n := max(1, w-9); len(tail) > n {
				tail = tail[len(tail)-n:]
			}
			f.textS(x+3, y+h-2, "›", m.th.a1, true, false)
			f.text(x+5, y+h-2, string(tail), m.th.text)
			f.put(x+5+len(tail), y+h-2, '▎', m.th.a1.mul(0.6+0.4*math.Sin(m.t*6)))
		} else {
			f.fade(x+3, y+h-2, trunc(tr("←→ выбор  enter вот это  a всё вместе  / своё  esc мимо", "←→ choose  enter this one  a all  / own words  esc none"), w-6), m.th.dim, ca*0.8, false)
		}
		return
	}
	for i, o := range q.Options {
		p := easeOut(prog(m.qT, 0.4+0.05*float64(i), 0.35))
		if p <= 0 {
			continue
		}
		ry := y + 4 + i
		hl := math.Max(0, 1-math.Abs(float64(i)-m.sel.x))
		chosen := m.answered && i == m.selIdx
		fl := 0.0
		if chosen {
			fl = 1 - prog(m.ansT, 0, 0.35)
		}
		oa := clamp01(p)
		if m.answered && !chosen {
			oa *= 1 - prog(m.ansT, 0, 0.2)
		}
		for xx := x + 2; xx < x+w-2; xx++ {
			u := float64(xx-x) / float64(w)
			if pp := f.at(xx, ry); pp != nil {
				pp.bg = pp.bg.lerp(m.th.a1, 0.22*hl*(1-u*0.85)*oa).lerp(white, 0.5*fl)
			}
		}
		if hl > 0.05 {
			f.fade(x+2, ry, "▌", m.th.a1, hl*oa, false)
		}
		dx := int((1 - p) * 3)
		f.fade(x+4+dx, ry, fmt.Sprint(i+1), m.th.dim, oa, false)
		f.fade(x+7+dx, ry, trunc(o, w-10), m.th.dim.lerp(m.th.text, 0.55+0.45*hl), oa, hl > 0.5)
	}
	f.fade(x+3, y+h-2, tr("↑↓ выбрать   enter ответ   1-4 сразу", "↑↓ choose   enter answer   1-4 quick"), m.th.dim, ca*0.8, false)
}

func (m *Model) drawScale(f *fb, q Question, x, y, w int, a float64) {
	steps := q.scaleSteps()
	tx, ty, tw := m.track(x, y, w)
	f.fade(tx, ty-1, q.Left, m.th.dim, a, false)
	f.fade(tx+tw-len([]rune(q.Right)), ty-1, q.Right, m.th.dim, a, false)
	kx := m.knob.x / float64(steps-1)
	reveal := easeOut(prog(m.qT, 0.55, 0.5))
	kpx := tx + int(math.Round(kx*float64(tw-1)))
	step := max(1, (tw-1)/(steps-1))
	for i := 0; i < tw; i++ {
		u := float64(i) / float64(tw-1)
		if u > reveal {
			break
		}
		ch, c := '─', m.th.dim.mul(0.8)
		if u <= kx {
			ch, c = '━', m.th.a1
		}
		if i%step == 0 {
			ch = '┼'
			if u <= kx {
				ch = '┿'
			}
		}
		if p := f.at(tx+i, ty); p != nil {
			f.put(tx+i, ty, ch, p.bg.lerp(c, a))
		}
	}
	if reveal >= 1 {
		for d := -3; d <= 3; d++ {
			f.tint(kpx+d, ty, m.th.a1, 0.2*math.Exp(-math.Abs(float64(d))/1.5)*a)
		}
		f.style(kpx, ty, '●', white, true, false)
	}
}

func (m *Model) drawTrackOptions(f *fb, q Question, x, y, w int, ca float64) {
	colW, cw, ch := trackCols(w, len(q.Tracks))
	if colW < 8 {
		m.drawTrackList(f, q, x, y, w)
		return
	}
	for i, tk := range q.Tracks {
		p := easeOut(prog(m.qT, 0.4+0.05*float64(i), 0.35))
		if p <= 0 {
			continue
		}
		hl := math.Max(0, 1-math.Abs(float64(i)-m.sel.x))
		chosen := m.answered && !m.ansFree && i == m.selIdx
		fl := 0.0
		if chosen {
			fl = 1 - prog(m.ansT, 0, 0.35)
		}
		oa := clamp01(p)
		if m.answered && !chosen {
			oa *= 1 - prog(m.ansT, 0, 0.2)
		}
		cx := x + 3 + i*(colW+2)
		ox := cx + (colW-cw)/2
		g := m.cover(tk.Key(), cw, 2*ch)
		for py := 0; py < 2*ch; py++ {
			for px := 0; px < cw; px++ {
				f.pixel(ox+px, (y+4)*2+py, g.at(px, py).mul(oa*(0.5+0.5*hl)))
			}
		}
		ly := y + 4 + ch
		for yy := ly; yy <= ly+2; yy++ {
			for xx := cx; xx < cx+colW; xx++ {
				if pp := f.at(xx, yy); pp != nil {
					pp.bg = pp.bg.lerp(m.th.a1, 0.22*hl*oa).lerp(white, 0.5*fl)
				}
			}
		}
		lc := m.th.dim.lerp(m.th.text, 0.55+0.45*hl)
		f.fade(cx, ly, trunc(q.Options[i], colW), lc, oa, hl > 0.5)
		f.fade(cx, ly+1, trunc(tk.Artist, colW), m.th.dim, oa, false)
		f.fade(cx, ly+2, trunc(tk.Title, colW), m.th.dim, oa, false)
	}
}

func (m *Model) drawTrackList(f *fb, q Question, x, y, w int) {
	for i, tk := range q.Tracks {
		p := easeOut(prog(m.qT, 0.4+0.05*float64(i), 0.35))
		if p <= 0 {
			continue
		}
		hl := math.Max(0, 1-math.Abs(float64(i)-m.sel.x))
		chosen := m.answered && !m.ansFree && i == m.selIdx
		fl := 0.0
		if chosen {
			fl = 1 - prog(m.ansT, 0, 0.35)
		}
		oa := clamp01(p)
		if m.answered && !chosen {
			oa *= 1 - prog(m.ansT, 0, 0.2)
		}
		ry := y + 4 + 2*i
		for yy := ry; yy <= ry+1; yy++ {
			for xx := x + 2; xx < x+w-2; xx++ {
				u := float64(xx-x) / float64(w)
				if pp := f.at(xx, yy); pp != nil {
					pp.bg = pp.bg.lerp(m.th.a1, 0.22*hl*(1-u*0.85)*oa).lerp(white, 0.5*fl)
				}
			}
		}
		if hl > 0.05 {
			f.fade(x+2, ry, "▌", m.th.a1, hl*oa, false)
		}
		g := m.cover(tk.Key(), 4, 4)
		for py := 0; py < 4; py++ {
			for px := 0; px < 4; px++ {
				f.pixel(x+3+px, ry*2+py, g.at(px, py).mul(oa*(0.5+0.5*hl)))
			}
		}
		f.fade(x+9, ry, trunc(q.Options[i], w-12), m.th.dim.lerp(m.th.text, 0.55+0.45*hl), oa, hl > 0.5)
		f.fade(x+9, ry+1, trunc(tk.Artist+" — "+tk.Title, w-12), m.th.dim, oa, false)
	}
}

// candLine renders one stream row: pending ones decode, accepted settle, rejected break apart.
func (m *Model) candLine(f *fb, c *cand, i, x, y, w int, la float64) {
	title := trunc(c.tr.Title, w-4)
	artist := c.tr.Artist
	age := m.t - c.born
	switch {
	case c.decided < 0:
		a := la * easeOut(prog(age, 0, 0.25))
		f.fade(x, y, "›", m.th.a1, a, false)
		f.fade(x+2, y, title, m.th.text.lerp(m.th.a1, 0.4), a, false)
	case c.ok:
		d := m.t - c.decided
		fl := math.Exp(-d * 4)
		for xx := x - 1; xx < x+w; xx++ {
			f.tint(xx, y, m.th.a1, 0.15*fl)
		}
		f.style(x, y, '✓', m.th.a1.lerp(white, fl), true, false)
		n := f.fade(x+2, y, title, m.th.text.lerp(m.th.dim, clamp01((d-1.2)/1.5)*0.5).lerp(white, fl), la, false)
		f.fade(x+4+n, y, trunc(artist, w-6-n), m.th.dim, la*0.8, false)
	default:
		ra := easeOut(clamp01((m.t - c.decided) / 0.5))
		reason := c.reason
		f.fade(x, y, "✕", m.th.bad, la*0.7, false)
		f.fade(x+2, y, trunc(c.tr.Title, w-6-len([]rune(reason))), m.th.bad.lerp(m.th.dim, ra), la*(1-0.6*ra), false)
		f.fade(x+w-len([]rune(reason)), y, reason, m.th.bad, la*0.7*ra, false)
	}
}

func (m *Model) drawScan(f *fb) {
	sx, y0, y1, sw, show := m.streamGeom()
	_, _, cols, cw, cards := m.gridGeom()
	f.textS(3, 3, fmt.Sprintf(tr("Порция %d", "Batch %d"), m.portionNo), m.th.text, true, false)
	cnt := fmt.Sprintf(tr("%d из %d", "%d of %d"), m.filled, len(m.slots))
	f.textS(m.w-3-len([]rune(cnt)), 3, cnt, m.th.a1, true, false)
	if m.rejected > 0 && m.w >= 70 {
		miss := fmt.Sprintf(tr("мимо %d", "passed %d"), m.rejected)
		f.text(m.w-6-len([]rune(cnt))-len([]rune(miss)), 3, miss, m.th.dim)
	}
	switch {
	case m.ph == PhDone:
		note := []rune(m.note)
		n := int(prog(m.pt, 0.5, 1.2) * float64(len(note)))
		f.textS(3, 4, trunc(string(note[:n]), m.w-6), m.th.a2.lerp(m.th.text, 0.3), false, true)
	case m.thinking:
		if s := m.status; s != "" {
			m.wave(f, 3, 4, trunc(s, m.w-6), m.th.a1)
		} else {
			m.wave(f, 3, 4, tr("агент думает, что ещё подходит", "agent is looking for more fits"), m.th.a1)
		}
	default:
		m.wave(f, 3, 4, tr("проверяю в Apple Music и по библиотеке", "checking Apple Music and the library"), m.th.a1)
	}

	first := m.visible()
	for i := first; i < len(m.cands); i++ {
		y := y0 + i - first
		if y >= y1 {
			break
		}
		la := 1 - 0.65*clamp01(float64(len(m.cands)-1-i-6)/8)
		if !show {
			la = 1
		}
		m.candLine(f, m.cands[i], i, sx, y, sw, la)
	}

	for i, s := range m.slots {
		px, py := m.slotPos(i)
		if py+1 >= m.h {
			break
		}
		wave := 0.0
		if m.ph == phDone {
			e := (m.pt - 0.15 - float64(i%cols+i/cols)*0.09) * 5
			wave = math.Exp(-e * e)
		}
		base := m.th.bg.lerp(m.th.text, 0.04)
		for yy := py; yy < py+2; yy++ {
			for xx := px; xx < px+cw; xx++ {
				if p := f.at(xx, yy); p != nil {
					p.bg, p.ch, p.pix = p.bg.lerp(base, 0.9).lerp(m.th.a1, 0.3*wave), ' ', false
				}
			}
		}
		switch {
		case s.fillAt < 0:
			b := 0.0
			if i == m.filled && m.ph == phScan {
				b = 0.05 + 0.05*math.Sin(m.t*4)
			}
			for yy := py; yy < py+2; yy++ {
				for xx := px; xx < px+cw; xx++ {
					f.tint(xx, yy, m.th.a1, b)
				}
			}
			f.put(px+min(cw/2, 2), py, '·', m.th.dim)
		default:
			age := m.t - s.fillAt
			fl := math.Exp(-age * 4)
			for yy := py; yy < py+2; yy++ {
				for xx := px; xx < px+cw; xx++ {
					f.tint(xx, yy, m.th.a1, 0.18*fl)
				}
			}
			g := m.cover(s.tr.Key(), 4, 4)
			r := easeOut(age / 0.35)
			for yy := 0; yy < 4; yy++ {
				for xx := 0; xx < 4; xx++ {
					f.pixel(px+xx, py*2+yy, f.pixelAt(px+xx, py*2+yy).lerp(g.at(xx, yy).lerp(white, 0.4*wave), r))
				}
			}
			if !cards {
				continue
			}
			tw := cw - 6
			f.fade(px+5, py, trunc(s.tr.Title, tw), m.th.text.lerp(white, wave), r, true)
			f.fade(px+5, py+1, trunc(s.tr.Artist, tw), m.th.dim, r, false)
		}
	}
}

func fmtDur(s float64) string { return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60) }

type playGeom struct {
	cx, cy, cw, ch int
	px, py, pw     int
	side, mini     bool
	split          bool
	noCover        bool // an egg layout draws its own centrepiece in the cover box
	noQueue        bool
}

// geom is the play layout: the window's, reshaped by the egg of the current track.
func (m *Model) geom() playGeom {
	g := m.baseGeom()
	if m.queue == nil {
		return g
	}
	if e := m.eggOf(*m.curTrack()); e != nil && e.layout != nil {
		g = e.layout(m, g)
	}
	return g
}

// baseGeom: landscape windows put the cover beside the text, portrait ones stack it on top.
func (m *Model) baseGeom() playGeom {
	w, h := m.w, m.h
	if h < 14 {
		ch := max(2, h-4)
		return playGeom{cx: 3, cy: 3, cw: ch * 2, ch: ch, px: 3 + ch*2 + 3, py: 3, pw: w - ch*2 - 9, side: true, mini: true}
	}
	if w >= 2*h+10 {
		ch := max(3, min(h-7, int(float64(w)*0.4/2)))
		ch = min(ch, 26)
		pw := min(72, w-ch*2-10)
		cx := max(3, (w-ch*2-4-pw)/2)
		cy := max(3, (h-ch)/2)
		px := cx + ch*2 + 4
		return playGeom{cx: cx, cy: cy, cw: ch * 2, ch: ch, px: px, py: cy, pw: pw, side: true}
	}
	if w >= 50 {
		ch := max(6, min(20, min((h-10)/2, (w-33)/2)))
		return playGeom{cx: 3, cy: 3, cw: ch * 2, ch: ch, px: 3 + ch*2 + 3, py: 3, pw: w - 9 - ch*2, side: true, split: true}
	}
	ch := max(3, min(20, min(h-13, (w-6)/2)))
	return playGeom{cx: 3, cy: 3, cw: ch * 2, ch: ch, px: 3, py: 3 + ch + 1, pw: w - 6}
}

func (m *Model) drawPlay(f *fb) {
	tk := m.curTrack()
	g := m.geom()
	reveal := easeOut(prog(m.trackT, 0, 1.1))

	e := m.eggOf(*tk)
	if !g.noCover {
		m.drawCover(f, g, tk, e, reveal)
	}

	x, y, pw := g.px, g.py, g.pw
	lf := 0.0
	if m.likeT > 0 {
		lf = math.Exp(-(m.t - m.likeT) * 3)
	}
	rows := 0
	if !g.mini && (g.side && g.ch >= 12 || !g.side && m.h-y >= 12) {
		rows, m.titleBox = m.bigText(f, x, y, tk.Title, pw, easeOut(prog(m.titleT(), 0.1, 0.8)), lf, m.cur+100, e)
	}
	if rows == 0 {
		rows = 1
		title := trunc(tk.Title, pw)
		for i, r := range scramble(title, prog(m.titleT(), 0.15, 0.55), m.t, m.cur) {
			if r != 0 {
				f.style(x+i, y, r, m.th.a1.lerp(m.th.text, 0.3).lerp(white, 0.6*lf), true, false)
			}
		}
		m.titleBox = image.Rect(x, y*2, x+len(title), y*2+2)
	}
	gap := 1
	if g.mini {
		gap = 0
	}
	y += rows + gap
	an := f.fade(x, y, trunc(tk.Artist, pw), m.th.text, easeOut(prog(m.trackT, 0.35, 0.5)), true)
	if e != nil && e.credit != "" {
		m.drawTag(f, x+an+3, y, pw-an-3, e)
	}
	y += 1 + gap
	if !g.mini || m.h >= 8 {
		wa := easeOut(prog(m.trackT, 0.6, 0.5))
		for i, r := range []rune(trunc(tk.Why, pw)) {
			if c := f.at(x+i, y); c != nil && r != ' ' {
				f.style(x+i, y, r, c.bg.lerp(m.th.a2.lerp(m.th.text, 0.4), wa), false, true)
			}
		}
		y += 1 + gap*2
	}

	time := fmtDur(m.pos) + " / " + fmtDur(tk.Dur)
	bw := min(56, pw-len(time)-2)
	m.barBox = image.Rect(x, y, x+bw, y+1)
	frac := 0.0
	if tk.Dur > 0 {
		frac = min(1, m.pos/tk.Dur)
	}
	knob, kc, fill := '●', m.th.text, m.th.a1
	if e != nil && e.knob != 0 {
		k := easeInOut(prog(m.trackT, eggKnobAt, 1.5))
		fill, kc = fill.lerp(e.knobCol, k), kc.lerp(e.knobCol, k)
		if k > 0.5 {
			knob = e.knob
		}
		kc = kc.lerp(white, 0.7*math.Exp(-(m.trackT-eggKnobAt-0.75)*(m.trackT-eggKnobAt-0.75)/0.04))
	}
	mark := -1
	if e != nil && e.switchAt > 0 && m.pos < e.switchAt {
		mark = int(e.switchAt / tk.Dur * float64(bw))
	}
	for i := 0; i < bw; i++ {
		var r rune
		var c col
		switch {
		case float64(i)/float64(bw) < frac:
			r, c = '━', fill
		case i == mark:
			r, c = '┼', m.eggMark(e)
		default:
			r, c = '─', m.th.dim.mul(0.55)
		}
		if e != nil && e.bar != nil {
			r, c = e.bar(m, i, bw, frac, r, c)
		}
		f.put(x+i, y, r, c)
	}
	f.style(x+int(frac*float64(bw)), y, knob, kc, true, false)
	f.text(x+bw+2, y, time, m.th.dim)

	y += 2

	m.drawNextLine(f, x, y, pw)

	if !g.mini && !g.noQueue {
		bottom := m.h - 3
		if !g.side {
			bottom = m.h - 2
		}
		if g.split {
			x, pw = 3, m.w-6
			y = max(y, g.cy+g.ch+1)
			bottom = m.h - 2
		}
		strip := m.nextStrip()
		limit := bottom
		if strip {
			limit = bottom - 2
		}
		for k := 1; y+1 < limit && m.cur+k < len(m.queue); k++ {
			next := m.queue[m.cur+k]
			rp := 1.0
			if m.queueT > 0 {
				rp = prog(m.t-m.queueT, 0.08*float64(k), 0.4)
			}
			tg := m.cover(next.Key(), 4, 4)
			for yy := 0; yy < 4; yy++ {
				for xx := 0; xx < 4; xx++ {
					if hash(xx, yy, k) < rp {
						f.pixel(x+xx, y*2+yy, tg.at(xx, yy).mul(0.85*(1-0.7*m.pause)))
					}
				}
			}
			fade := max(0.4, 1-0.12*float64(k))
			f.fade(x+6, y, trunc(next.Title, pw-6), m.th.dim.lerp(m.th.text, 0.6*fade), easeOut(rp), false)
			f.fade(x+6, y+1, trunc(next.Artist, pw-6), m.th.dim, rp*0.7*fade, false)
			y += 2
		}
		if strip && y+1 < bottom {
			m.drawNextStrip(f, x, y, pw)
		}
	}

	if m.typing {
		bx, by, bw := 3, m.h-4, m.w-6
		for yy := by; yy < by+3; yy++ {
			for xx := bx; xx < bx+bw; xx++ {
				if p := f.at(xx, yy); p != nil {
					p.bg, p.ch, p.pix = p.bg.lerp(m.th.bg, 0.8), ' ', false
				}
			}
		}
		m.glowBox(f, bx, by, bw, 3, 1)
		f.textS(bx+2, by+1, "›", m.th.a1, true, false)
		f.text(bx+4, by+1, string(m.reply), m.th.text)
		f.put(bx+4+len(m.reply), by+1, '▎', m.th.a1.mul(0.6+0.4*math.Sin(m.t*6)))
	} else if !g.mini {
		help := tr("? клавиши", "? keys")
		f.fade(3, m.h-2, tr("/ сказать агенту", "/ talk to the agent"), m.th.dim, 0.7, false)
		f.fade(m.w-3-len([]rune(help)), m.h-2, help, m.th.dim, 0.7, false)
	}
	m.eggOverlay(f, g)
	m.drawToasts(f)
}

// drawNextLine is the play-phase line under the progress bar: what the agent is doing
// with the next portion.
func (m *Model) drawNextLine(f *fb, x, y, pw int) {
	if m.next.state == nextOff || y-1 >= m.h-2 {
		return
	}
	sy := y - 1
	switch m.next.state {
	case nextGathering:
		var s string
		if m.next.reply == "" {
			s = fmt.Sprintf(tr("собираю следующие %d", "picking the next %d"), m.portionSize())
			if m.status != "" {
				s += " · " + m.status
			}
		} else {
			s = fmt.Sprintf(tr("учитываю «%s»", "taking in \"%s\""), trunc(m.next.reply, 24))
			if m.status != "" {
				s += " · " + m.status
			} else {
				s += fmt.Sprintf(tr(", пересобираю следующие %d", ", rebuilding the next %d"), m.portionSize())
			}
		}
		m.wave(f, x, sy, trunc(s, pw), m.th.a1)
	case nextReady:
		s := fmt.Sprintf(tr("✓ следующие %d готовы", "✓ next %d ready"), len(m.next.tracks))
		f.fade(x, sy, trunc(s, pw), m.th.a1, easeOut(prog(m.t-m.next.at, 0, 0.3)), false)
	case nextFailed:
		if a := 0.9 * (1 - prog(m.t-m.next.at, 5, 1)); a > 0 {
			f.fade(x, sy, trunc(tr("✗ следующие не собрались", "✗ couldn't pick the next ones"), pw), m.th.bad, a, false)
		}
	}
}

// nextStrip: the tail of the list gives room to the next portion while it is gathered,
// ready, or still dissolving away.
func (m *Model) nextStrip() bool {
	return m.next.state == nextGathering || m.next.state == nextReady ||
		m.t-m.next.goneAt < 0.4 && len(m.next.gone) > 0
}

// drawNextStrip draws the portion's thumbnails in a row of slots under the queue.
func (m *Model) drawNextStrip(f *fb, x, y, pw int) {
	n := m.next
	d := prog(m.t-n.goneAt, 0, 0.4)
	k := min(m.portionSize(), (pw+1)/5)
	for i := 0; i < k; i++ {
		sx := x + 5*i
		if n.state == nextGathering {
			b := 0.18 + 0.12*(0.5+0.5*math.Sin(m.t*3-float64(i)*0.8))
			for yy := 0; yy < 4; yy++ {
				for xx := 0; xx < 4; xx++ {
					f.pixel(sx+xx, y*2+yy, m.th.dim.mul(b))
				}
			}
		}
		if i < len(n.gone) && d < 1 {
			g := m.cover(n.gone[i].Key(), 4, 4)
			for yy := 0; yy < 4; yy++ {
				for xx := 0; xx < 4; xx++ {
					if hash(xx, yy, i+23) >= d {
						f.pixel(sx+xx, y*2+yy, g.at(xx, yy).mul(0.7*(1-0.7*m.pause)))
					}
				}
			}
		}
		if i < len(n.tracks) {
			b := 0.7
			if n.state == nextReady {
				b = 0.7 + 0.15*easeOut(prog(m.t-n.at, 0, 0.4))
			}
			g := m.cover(n.tracks[i].Key(), 4, 4)
			rp := prog(m.t-n.born[i], 0, 0.5)
			for yy := 0; yy < 4; yy++ {
				for xx := 0; xx < 4; xx++ {
					if hash(xx, yy, i+11) < rp {
						f.pixel(sx+xx, y*2+yy, g.at(xx, yy).mul(b*(1-0.7*m.pause)))
					}
				}
			}
		}
	}
	if n.state != nextGathering {
		return
	}
	s := fmt.Sprintf(tr("%d из %d", "%d of %d"), len(n.tracks), m.portionSize())
	if x+5*k+1+len([]rune(s)) < x+pw {
		f.fade(x+5*k+1, y, s, m.th.dim, 0.8, false)
	}
}

// drawCover lays the cover into its box over a glow of its own colours.
func (m *Model) drawCover(f *fb, g playGeom, tk *Track, e *egg, reveal float64) {
	glow := m.cover(tk.Key(), 12, 12)
	breathe := 0.9 + 0.1*math.Sin(m.t*1.6)
	sc := float64(g.ch) / 22
	rx, ry := int(18*sc)+2, int(8*sc)+1
	for y := g.cy - ry; y < g.cy+g.ch+ry; y++ {
		for x := g.cx - rx; x < g.cx+g.cw+rx; x++ {
			if x >= g.cx && x < g.cx+g.cw && y >= g.cy && y < g.cy+g.ch {
				continue
			}
			nx := math.Max(float64(g.cx), math.Min(float64(g.cx+g.cw-1), float64(x)))
			ny := math.Max(float64(g.cy), math.Min(float64(g.cy+g.ch-1), float64(y)))
			d := math.Hypot((float64(x)-nx)*0.5, float64(y)-ny)
			c := glow.at(int((nx-float64(g.cx))/float64(g.cw)*11), int((ny-float64(g.cy))/float64(g.ch)*11))
			lit := smooth((c.lum() - 0.12) * 4)
			f.tint(x, y, c, 0.55*lit*math.Exp(-d/(1+2.2*sc))*breathe*(0.4+0.6*reveal)*(1-m.pause))
		}
	}

	cov := m.cover(tk.Key(), g.cw, g.ch*2)
	var prev *pixgrid
	if m.prev != m.cur {
		prev = m.cover(m.queue[m.prev].Key(), g.cw, g.ch*2)
	}
	skip := 0.0
	if m.skipT > 0 {
		skip = easeIn(prog(m.t-m.skipT, 0, 0.35))
	}
	for py := 0; py < g.ch*2; py++ {
		for px := 0; px < g.cw; px++ {
			thr := hash(px/2, py/2, m.cur)*0.65 + float64(py)/float64(g.ch*2)*0.35
			var c col
			if reveal >= thr || prev == nil {
				c = cov.at(px, py)
				if e != nil && e.cover != nil {
					c = e.cover(m, cov.at, px, py)
				}
				if es := reveal - thr; es >= 0 && es < 0.05 && reveal < 1 {
					c = c.lerp(m.th.a2.lerp(white, 0.5), 1-es/0.05)
				}
			} else {
				c = prev.at(px, py).mul(0.6)
			}
			f.pixel(g.cx+px, g.cy*2+py, m.eggCover(c, float64(py)/float64(g.ch*2)).mul((1-0.75*skip)*(1-0.7*m.pause)))
		}
	}
}

func (m *Model) drawToasts(f *fb) {
	w := min(46, m.w-6)
	base := m.h - 4
	if m.typing {
		base = m.h - 6
	}
	for i := len(m.toasts) - 1; i >= 0; i-- {
		t := m.toasts[i]
		h := 3
		if t.q != nil {
			h = 6
			if t.q.trackCard() {
				h = 8
			}
		}
		x := m.w - w - 3 + int(t.x.x)
		y := base - h + 1
		base -= h + 1
		bg := m.th.bg.lerp(m.th.text, 0.07)
		for yy := y; yy < y+h; yy++ {
			for xx := x; xx < x+w; xx++ {
				if p := f.at(xx, yy); p != nil {
					p.bg, p.ch, p.pix = bg, ' ', false
				}
			}
		}
		m.glowBox(f, x, y, w, h, 1)
		if t.q == nil {
			f.fade(x+2, y+1, trunc(t.text, w-4), m.th.text, easeOut(prog(m.t-t.born, 0.1, 0.3)), false)
			continue
		}
		f.text(x+2, y+1, tr("вопрос, музыка не прервётся", "a question, the music keeps playing"), m.th.dim)
		f.textS(x+2, y+2, trunc(t.q.Title, w-4), m.th.text, true, false)
		if t.q.trackCard() {
			for k, tk := range t.q.Tracks {
				ox := x + 2 + k*14
				fl := 0.0
				if t.picked == k {
					fl = 1 - prog(m.t-t.pickT, 0, 0.5)
					for xx := ox; xx < ox+14; xx++ {
						f.tint(xx, y+6, m.th.a1, 0.45*fl)
					}
				}
				g := m.cover(tk.Key(), 6, 6)
				for py := 0; py < 6; py++ {
					for px := 0; px < 6; px++ {
						c := g.at(px, py)
						if t.picked >= 0 && t.picked != k {
							c = c.mul(0.5)
						}
						f.pixel(ox+px, (y+3)*2+py, c)
					}
				}
				oc := m.th.text
				if t.picked >= 0 && t.picked != k {
					oc = m.th.dim
				}
				f.style(ox, y+6, rune('1'+k), m.th.a1.lerp(white, fl), true, false)
				f.text(ox+2, y+6, trunc(t.q.Options[k], 11), oc.lerp(white, fl))
			}
			continue
		}
		ox := x + 2
		for k, o := range t.q.Options {
			fl := 0.0
			if t.picked == k {
				fl = 1 - prog(m.t-t.pickT, 0, 0.5)
				for xx := ox - 1; xx < ox+len([]rune(o))+3; xx++ {
					f.tint(xx, y+4, m.th.a1, 0.45*fl)
				}
			}
			oc := m.th.text
			if t.picked >= 0 && t.picked != k {
				oc = m.th.dim
			}
			f.style(ox, y+4, rune('1'+k), m.th.a1.lerp(white, fl), true, false)
			ox += 2 + f.text(ox+2, y+4, o, oc.lerp(white, fl)) + 3
		}
	}
}

func (m *Model) drawHelp(f *fb) {
	keys := [][2]string{
		{"/", tr("сказать агенту", "talk to the agent")},
		{"+", tr("больше такого", "more like this")},
		{"-", tr("меньше такого", "less like this")},
		{"n", tr("следующий", "next")},
		{"w", tr("в плейлист", "save to playlist")},
		{"r", tr("заново", "start over")},
		{"q", tr("выход", "quit")},
	}
	w, h := 30, len(keys)+4
	x, y := (m.w-w)/2, (m.h-h)/2
	bg := m.th.bg.lerp(m.th.text, 0.06)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if p := f.at(xx, yy); p != nil {
				p.bg, p.ch, p.pix = bg, ' ', false
			}
		}
	}
	m.glowBox(f, x, y, w, h, 1)
	for i, k := range keys {
		f.textS(x+3, y+2+i, k[0], m.th.a1, true, false)
		f.text(x+6, y+2+i, k[1], m.th.text)
	}
}

func (m *Model) orb(f *fb, cx, cy, grow float64) {
	r := (5 + 0.5*math.Sin(m.t*2.4)) * grow
	if r <= 0.2 {
		return
	}
	for y := int(cy/2 - r - 6); y <= int(cy/2+r+6); y++ {
		for x := int(cx - r*2 - 10); x <= int(cx+r*2+10); x++ {
			d := math.Hypot((float64(x)-cx)*0.5, float64(y)-cy/2)
			f.tint(x, y, m.th.a1, 0.35*math.Exp(-math.Max(0, d-r/2)/2.5)*grow)
		}
	}
	for py := int(cy - r - 1); py <= int(cy+r+1); py++ {
		for px := int(cx - r - 1); px <= int(cx+r+1); px++ {
			dx, dy := float64(px)-cx, float64(py)-cy
			d := math.Hypot(dx, dy)
			if d > r {
				continue
			}
			ang := math.Atan2(dy, dx)/(2*math.Pi) + m.t*0.35
			c := m.th.grad(ang).lerp(white, 0.75*math.Pow(1-d/r, 2))
			f.pixel(px, py, f.pixelAt(px, py).lerp(c, clamp01(r-d)))
		}
	}
}

func (m *Model) wave(f *fb, x, y int, s string, base col) {
	rs := []rune(s)
	n := float64(len(rs))
	pos := math.Mod(m.t*16, n+14) - 7
	for i, r := range rs {
		d := float64(i) - pos
		b := 0.3 + 0.7*math.Exp(-d*d/8)
		f.put(x+i, y, r, m.th.dim.lerp(base, b).lerp(white, 0.4*math.Exp(-d*d/3)))
	}
}

func (m *Model) bigText(f *fb, x, y int, s string, maxW int, reveal, flash float64, seed int, e *egg) (int, image.Rectangle) {
	drips := e != nil && e.drips
	rs := []rune(strings.ToUpper(s))
	for _, r := range rs {
		if _, ok := font3[r]; !ok {
			return 0, image.Rectangle{}
		}
	}
	cols := -1
	for _, r := range rs {
		cols += len(font3[r][0]) + 1
	}
	sc := 2
	if e != nil && e.big && cols*3 <= maxW {
		sc = 3
	}
	if cols*sc > maxW {
		sc = 1
	}
	if cols*sc > maxW {
		return 0, image.Rectangle{}
	}
	wpx := float64(cols * sc)
	sweep := prog(m.titleT(), 0.9, 1.2)*1.6 - 0.3
	gx0 := 0
	for _, r := range rs {
		g := font3[r]
		gw := len(g[0])
		for gy := 0; gy < 5; gy++ {
			for gx := 0; gx < gw; gx++ {
				if g[gy][gx] != '#' {
					continue
				}
				for sy := 0; sy < sc; sy++ {
					for sx := 0; sx < sc; sx++ {
						px := x + (gx0+gx)*sc + sx
						py := y*2 + gy*sc + sy
						if hash(px, py, seed) > reveal {
							continue
						}
						if e != nil && e.ragged && gy == 4 && sy == sc-1 && hash(px, 3, seed) < 0.4 {
							continue
						}
						u := float64(px-x) / wpx
						b := math.Exp(-(u - sweep) * (u - sweep) / 0.004)
						c := m.th.grad(u*0.5+m.t*0.03).lerp(white, 0.7*b+0.6*flash)
						if e != nil && e.ink != nil {
							c = e.ink(m, c, u, float64(py-y*2)/float64(5*sc))
						}
						f.pixel(px, py, c)
					}
				}
			}
		}
		if drips {
			m.drip(f, x+gx0*sc, y*2+5*sc, g[4], sc, seed+gx0)
		}
		gx0 += gw + 1
	}
	if drips {
		return (5*sc+1)/2 + (dripMax*sc+1)/2, image.Rect(x, y*2, x+cols*sc, y*2+5*sc)
	}
	return (5*sc + 1) / 2, image.Rect(x, y*2, x+cols*sc, y*2+5*sc)
}

func (m *Model) drawParticles(f *fb) {
	for _, p := range m.parts {
		a := p.alpha()
		if a <= 0 || p.age < 0 {
			continue
		}
		if p.pixel {
			px, py := int(math.Round(p.x)), int(math.Round(p.y))
			f.pixel(px, py, f.pixelAt(px, py).lerp(p.c, a))
			continue
		}
		x, y := int(math.Round(p.x)), int(math.Round(p.y))
		c := f.at(x, y)
		if c == nil {
			continue
		}
		ch := p.ch
		if p.home && prog(p.age, p.delay, p.life) > 0.6 {
			ch = p.tch
		}
		bg := c.bg
		f.put(x, y, ch, bg.lerp(p.c.lerp(white, 0.3), a))
	}
}

// dissolve reveals the new frame over the previous one along a noisy sweep with a glowing edge.
func (m *Model) dissolve(f *fb) {
	p := m.transT / 1.0
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			thr := hash(x/3, y, 11)*0.18 + float64(x)/float64(f.w)*0.82
			i := y*f.w + x
			switch {
			case p < thr:
				f.c[i] = m.snap.c[i]
			case p < thr+0.07:
				k := 1 - (p-thr)/0.07
				f.c[i].bg = f.c[i].bg.lerp(m.th.a2.lerp(white, 0.3), 0.7*k)
				if f.c[i].pix {
					f.c[i].fg = f.c[i].fg.lerp(m.th.a2.lerp(white, 0.3), 0.7*k)
				}
			}
		}
	}
}
