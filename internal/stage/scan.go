package stage

import (
	"fmt"
	"math"
	"strings"
)

// The scan is the agent pulling records for side A on a turntable, an automatic belt-drive
// deck of the current kind. Each keeper comes in as a sleeve with its real cover; the record,
// pressed in the cover's colour, slides out of it and is laid on the platter, on top of the
// ones before it, and its row is written into side A. A pass only shows its sleeve and its
// reason. When the side is full, START spins the stack up and the arm drops the needle into
// the player.

// A candidate's timeline, in seconds at pace 1, from the moment it steps on stage.
const (
	sleeveIn  = 0.5  // the sleeve fades in
	verdictIn = 0.6  // the verdict shows
	slideAt   = 0.8  // a keeper's record starts out of its sleeve
	slideLen  = 1.0  // its slide out
	carryLen  = 0.9  // from the sleeve's mouth onto the stack
	slideOut  = 0.62 // how far out of the sleeve it slides, in sleeve widths
	keepLen   = 2.9  // a keeper's whole stay
	passLen   = 1.4  // a pass's whole stay
	auditionH = 7    // rows the audition holds, however small its sleeve
	stackStep = 0.03 // each record on the stack lies this much higher, in record radii

	doneStart = 0.2 // PhDone: START
)

// The deck in record radii, on the plinth's top plane; y grows toward the viewer.
const (
	ttTilt   = 0.62
	ttFocal  = 5.0
	plX0     = -1.17
	plX1     = 1.72
	plY0     = -1.12
	plY1     = 1.14
	plCorner = 0.07
	plH      = 0.16 // plinth thickness
	platR    = 1.0
	platZ    = 0.05
	recR     = 0.98
	recZ     = 0.058
	labelR   = 0.33
	pivotX   = 1.32
	pivotY   = -0.72
	armL     = 1.5  // pivot to stylus
	armBend  = 0.13 // the J: the main tube runs this far outside the pivot-stylus line
	zPivot   = 0.17
	zRest    = 0.11
	btnY     = 0.99
)

var (
	armRest = math.Pi / 2
	armLead = leadAngle(0.955)
	btnX    = [3]float64{-0.99, -0.85, -0.71} // START, STOP, SPEED
)

// leadAngle is the arm angle that puts the stylus at radius r.
func leadAngle(r float64) float64 {
	d := math.Hypot(pivotX, pivotY)
	th := math.Atan2(-pivotY, -pivotX)
	return th - math.Acos((d*d+armL*armL-r*r)/(2*d*armL))
}

type journalLine struct {
	text string
	at   float64
}

type scanGeom struct {
	dv         discView
	px, py, pw int
	side       bool
}

// scanGeom fits the deck in its box: wide windows put the panel beside it, narrow ones
// stack the panel under it.
func (m *Model) scanGeom() scanGeom {
	unit := discView{R: 1, tilt: ttTilt, focal: ttFocal}
	ex0, ex1, ey0, ey1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range [][3]float64{{plX0, plY0, zPivot + 0.04}, {plX1, plY0, zPivot + 0.04}, {plX0, plY1, -plH}, {plX1, plY1, -plH}} {
		x, y := unit.toScreen(p[0], p[1], p[2])
		ex0, ex1, ey0, ey1 = math.Min(ex0, x), math.Max(ex1, x), math.Min(ey0, y), math.Max(ey1, y)
	}
	side := m.w >= 2*m.h+10
	bx0, by0 := 3.0, 5.0
	bx1, by1 := float64(m.w-3), float64(2*m.h-3)
	pw := m.w - 6
	if side {
		pw = min(60, max(30, m.w*36/100))
		bx1 = float64(m.w - pw - 6)
	} else {
		by1 = math.Max(by0+12, float64(2*m.h)*0.5)
	}
	R := math.Max(4, math.Min((bx1-bx0)/(ex1-ex0), (by1-by0)/(ey1-ey0)))
	cx := (bx0+bx1)/2 - (ex0+ex1)/2*R
	cy := (by0+by1)/2 - (ey0+ey1)/2*R
	dv := discView{cx: cx, cy: cy, R: R, tilt: ttTilt, spin: m.spin, focal: ttFocal}
	if side {
		return scanGeom{dv: dv, px: m.w - pw - 3, py: 4, pw: pw, side: true}
	}
	return scanGeom{dv: dv, px: 3, py: int((cy+ey1*R)/2) + 2, pw: pw}
}

// auditionPace speeds the stage up a little while candidates queue behind the one on it.
func auditionPace(waiting int) float64 { return math.Max(0.75, 1-0.06*float64(waiting)) }

func (m *Model) onStage() *cand {
	if len(m.cands) == 0 {
		return nil
	}
	if c := m.cands[len(m.cands)-1]; m.t < c.leave {
		return c
	}
	return nil
}

// rel is seconds since start on c's timeline, at its pace.
func (c *cand) rel(t, start float64) float64 { return (t - start) / c.pace }

func (m *Model) scanStep(dt float64) {
	if m.onStage() == nil && len(m.inbox) > 0 && m.ph == PhScan {
		c := m.inbox[0]
		m.inbox = m.inbox[1:]
		c.pace = auditionPace(len(m.inbox))
		c.born, c.verdict = m.t, verdictIn*c.pace
		c.leave = m.t + passLen*c.pace
		if c.ok && m.filled < len(m.slots) {
			c.leave = m.t + keepLen*c.pace
		}
		m.thinking = false
		m.cands = append(m.cands, c)
	}
	for _, c := range m.cands {
		if c.decided >= 0 || m.t-c.born < c.verdict {
			continue
		}
		c.decided = m.t
		m.checked++
		if !c.ok {
			m.rejected++
			continue
		}
		if m.filled < len(m.slots) {
			s := m.slots[m.filled]
			s.tr, s.fillAt = c.tr, m.t
			s.landAt = c.born + (slideAt+slideLen+carryLen)*c.pace
			c.slot = m.filled
			m.filled++
		}
	}
	if n := m.stacked(); n > 0 && m.slots[n-1].tr.Key() != m.seatKey {
		m.seatKey, m.seatSpin = m.slots[n-1].tr.Key(), m.spin
	}
	if !m.pDone || m.ph != PhScan {
		return
	}
	solved := len(m.inbox) == 0 && m.onStage() == nil
	for _, c := range m.cands {
		if c.decided < 0 {
			solved = false
			break
		}
	}
	if !solved {
		return
	}
	if m.filled == 0 {
		m.Fail(tr("ничего не нашлось — попробуй сказать иначе", "found nothing — try saying it another way"))
		return
	}
	if m.filled < len(m.slots) {
		m.slots = m.slots[:m.filled]
	}
	for _, s := range m.slots {
		if s.fillAt < 0 || m.t < s.landAt {
			return
		}
	}
	m.setPhase(PhDone)
	m.doneAt = m.t
}

// stacked counts the records lying on the platter.
func (m *Model) stacked() int {
	n := 0
	for _, s := range m.slots {
		if s.fillAt < 0 || m.t < s.landAt {
			break
		}
		n++
	}
	return n
}

// stackZ is the height of the top of record i on the stack.
func stackZ(i int) float64 { return recZ + float64(i)*stackStep }

// spinStep turns the platter: still while the stack grows, the belt brings it up to 33⅓
// once START is pressed.
func (m *Model) spinStep(dt float64) {
	target := 0.0
	if m.ph == PhDone && m.pt >= doneStart {
		target = 33.3 / 60 * 2 * math.Pi
	}
	m.spinV += (target - m.spinV) * (1 - math.Exp(-dt*1.4))
	m.spin += m.spinV * dt
}

// armPose is the automatic arm: it rests through the scan and, after START, rises, swings
// over the lead-in and settles. swing runs from the rest (0) to the lead-in (1), lift from
// resting (0) to raised (1).
func (m *Model) armPose() (ang, swing, lift float64) {
	if m.ph == PhDone {
		u := m.pt - doneStart
		lift = smooth(prog(u, 0.1, 0.25)) * (1 - smooth(prog(u, 1.55, 0.5)))
		swing = easeInOut(prog(u, 0.35, 1.2))
	}
	return lerpf(armRest, armLead, swing), swing, lift
}

// startPress is how far START is down: pressed once, when the side is done.
func (m *Model) startPress() float64 {
	since := m.pt - doneStart
	if m.ph != PhDone || since < 0 || since > 0.35 {
		return 0
	}
	return 1 - smooth(prog(since, 0.15, 0.2))
}

// auditionBox is the sleeve of the candidate on stage: cells, square in pixels.
func (m *Model) auditionBox(g scanGeom) (x, y, cs int) {
	cs = min(26, g.pw*2/5) &^ 1
	if !g.side {
		cs = min(16, g.pw/4) &^ 1
	}
	return g.px, g.py + 2, max(4, cs)
}

// sideTop is the row of the side A header.
func (m *Model) sideTop(g scanGeom) int {
	_, ay, cs := m.auditionBox(g)
	return ay + max(cs/2, auditionH) + 1
}

// labelKeys: the backdrop takes the colours of the record that went on the stack last.
func (m *Model) labelKeys() (cur, prev string, wipe float64) {
	wipe = 1
	for _, s := range m.slots {
		if s.fillAt < 0 || m.t < s.landAt {
			continue
		}
		prev, cur = cur, s.tr.Key()
		wipe = easeOut(prog(m.t, s.landAt, 0.6))
	}
	return
}

// ---------- drawing ----------

func (m *Model) drawScan(f *fb) {
	g := m.scanGeom()
	m.drawDeck(f, g.dv)
	for i := range m.stacked() {
		top := g.dv
		top.cy -= stackZ(i) * math.Sin(top.tilt) * top.R
		m.drawVinyl(f, top, m.slots[i].tr.Key(), m.spin-m.seatSpin, 1, 0)
	}
	m.drawTonearm(f, g.dv)
	m.drawScanPanel(f, g)
	m.drawCarry(f, g)
}

func plinthSDF(x, y float64) float64 {
	hx, hy := (plX1-plX0)/2, (plY1-plY0)/2
	qx := math.Abs(x-(plX0+plX1)/2) - hx + plCorner
	qy := math.Abs(y-(plY0+plY1)/2) - hy + plCorner
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - plCorner
}

// samples are the four subpixel offsets every surface pass antialiases with.
var samples = [4][2]float64{{0.25, 0.25}, {0.75, 0.25}, {0.25, 0.75}, {0.75, 0.75}}

// lit is the room light on a flat surface: a soft sheen and a hot spot that stay put.
func lit(x, y float64) (sheen, spec float64) {
	l := math.Abs(math.Cos(math.Atan2(y, x) - 0.55))
	return math.Pow(l, 16), math.Pow(l, 120)
}

// drawDeck paints the plinth with its shadow and front, the platter with its rim and mat,
// and the three buttons.
func (m *Model) drawDeck(f *fb, dv discView) {
	sn := math.Sin(dv.tilt)
	face := plH * sn * dv.R
	rimH := platZ * sn * dv.R
	plat := dv
	plat.cy -= rimH
	x0, x1 := max(0, int(dv.cx+plX0*1.3*dv.R)-2), min(f.w-1, int(dv.cx+plX1*1.3*dv.R)+2)
	y0, y1 := max(0, int(dv.cy-1.4*dv.R)), min(f.h*2-1, int(dv.cy+1.6*dv.R))
	top := col{0.1, 0.1, 0.108}
	edge := col{0.2, 0.2, 0.21}
	front := col{0.045, 0.045, 0.05}
	mat := col{0.06, 0.06, 0.065}
	alu := col{0.62, 0.62, 0.66}
	side := col{0.2, 0.2, 0.22}
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			bg := f.pixelAt(px, py)
			var s col
			n, shadow := 0.0, 0.0
			for _, o := range samples {
				sx, sy := float64(px)+o[0], float64(py)+o[1]
				var c col
				hit := true
				X, Y, ok := plat.toDisc(sx, sy)
				P, Q, okP := dv.toDisc(sx, sy)
				switch r := math.Hypot(X, Y); {
				case ok && r <= platR:
					sheen, spec := lit(X, Y)
					if r > platR-0.025 {
						c = alu.lerp(white, 0.4*sheen+0.4*spec)
					} else {
						c = mat.mul(0.9+0.2*hash(int(X*90), int(Y*90), 3)).lerp(white, 0.03*sheen)
					}
				case okP && Q > 0 && math.Hypot(P, Q) <= platR:
					c = side.mul(0.7 + 0.3*math.Max(0, -P))
				case okP && plinthSDF(P, Q) <= 0:
					d := -plinthSDF(P, Q)
					c = top.mul(1 + 0.12*(-Q*0.6-P*0.2))
					if d < 0.022 {
						c = edge
					}
				default:
					if X2, Y2, ok2 := dv.toDisc(sx, sy-face); ok2 && plinthSDF(X2, Y2) <= 0 {
						c = front.mul(1 - 0.3*clamp01((sy-face-plat.cy)/(dv.R*2)))
						break
					}
					hit = false
					if okP {
						d := plinthSDF(P-0.05, Q-0.09)
						shadow += (1 - smooth(d/0.22)) * 0.6 / 4
					}
				}
				if hit {
					s = col{s.r + c.r, s.g + c.g, s.b + c.b}
					n++
				}
			}
			if n == 0 {
				if shadow > 0 {
					f.pixel(px, py, bg.mul(1-shadow))
				}
				continue
			}
			k := 1 - n/4
			bg = bg.mul(1 - shadow)
			f.pixel(px, py, col{bg.r*k + s.r/4, bg.g*k + s.g/4, bg.b*k + s.b/4})
		}
	}
	sx, sy := plat.toScreen(0, 0, 0)
	f.seg(sx, sy, sx, sy-dv.R*0.03, math.Max(0.5, dv.R*0.018), alu, 1)

	start := m.startPress()
	bw := math.Max(0.7, dv.R*0.026)
	for i, x := range btnX {
		down := [3]float64{start, 0, 0}[i]
		ax, ay := dv.toScreen(x-0.035, btnY, 0)
		bx, by := dv.toScreen(x+0.035, btnY, 0)
		f.seg(ax+0.4, ay+0.5, bx+0.4, by+0.5, bw, col{}, 0.5)
		z := 0.02 * (1 - 0.8*down)
		ax, ay = dv.toScreen(x-0.035, btnY, z)
		bx, by = dv.toScreen(x+0.035, btnY, z)
		c := col{0.42, 0.42, 0.45}.lerp(col{0.2, 0.2, 0.22}, down)
		f.seg(ax, ay, bx, by, bw, c, 1)
	}
}

// vinylCol is the colour a record is pressed in: its cover's accent, kept bright enough to
// read as coloured vinyl.
func (m *Model) vinylCol(key string) col {
	var t Track
	for _, c := range m.cands {
		if c.tr.Key() == key {
			t = c.tr
		}
	}
	for _, s := range m.slots {
		if s.tr.Key() == key {
			t = s.tr
		}
	}
	h, sat, v := toHSV(m.themeFor(t).a1)
	return hsv(h, math.Min(1, sat*1.1), math.Max(0.7, v))
}

// drawVinyl draws a record: coloured translucent vinyl with its grooves, the cover on the
// label turned by rot, the light fixed in the room. grey drains its colour.
func (m *Model) drawVinyl(f *fb, dv discView, key string, rot, a, grey float64) {
	if a <= 0 || dv.R < 0.5 {
		return
	}
	base := m.vinylCol(key)
	cov := m.cover(key, 24, 24)
	cs, sn := math.Cos(rot), math.Sin(rot)
	grooves := math.Max(40, dv.R*3)
	x0, x1 := max(0, int(dv.cx-dv.R*1.3)-1), min(f.w-1, int(dv.cx+dv.R*1.3)+1)
	y0, y1 := max(0, int(dv.cy-dv.R*1.2)-1), min(f.h*2-1, int(dv.cy+dv.R*1.3)+1)
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			var s col
			n := 0.0
			for _, o := range samples {
				X, Y, ok := dv.toDisc(float64(px)+o[0], float64(py)+o[1])
				r := math.Hypot(X, Y)
				if !ok || r > recR {
					continue
				}
				var c col
				sheen, spec := lit(X, Y)
				switch {
				case r < 0.022:
					c = col{0.7, 0.7, 0.74}
				case r < labelR:
					lx, ly := X*cs+Y*sn, -X*sn+Y*cs
					c = cov.at(int((lx/labelR+1)/2*24), int((ly/labelR+1)/2*24))
					if r > labelR-0.015 {
						c = c.mul(0.55)
					}
					c = c.lerp(white, 0.12*spec)
				case r < 0.42 || r > 0.94:
					c = base.mul(0.5).lerp(base.lerp(white, 0.5), 0.35*sheen+0.5*spec)
				default:
					g := 0.75 + 0.25*math.Sin(r*grooves)
					c = base.mul(0.4).lerp(base.lerp(white, 0.55), (0.45*sheen+0.55*spec)*g)
				}
				if r > recR-0.012 {
					c = base.mul(0.75)
				}
				if grey > 0 {
					l := c.lum() * 0.7
					c = c.lerp(col{l, l, l}, grey)
				}
				s = col{s.r + c.r, s.g + c.g, s.b + c.b}
				n++
			}
			if n == 0 {
				continue
			}
			k := n / 4 * a
			f.pixel(px, py, f.pixelAt(px, py).lerp(col{s.r / n, s.g / n, s.b / n}, k))
		}
	}
}

// drawTonearm is the J-shaped arm on its pivot: counterweight, tube, headshell, the rest it
// lies on, and their shadows, which meet the stylus as it touches down.
func (m *Model) drawTonearm(f *fb, dv discView) {
	ang, swing, lift := m.armPose()
	ux, uy := math.Cos(ang), math.Sin(ang)
	bx, by := math.Cos(ang-armBend), math.Sin(ang-armBend)
	sx, sy := pivotX+armL*ux, pivotY+armL*uy
	ex, ey := pivotX+0.72*armL*bx, pivotY+0.72*armL*by
	n := m.stacked()
	zs := lerpf(zRest, stackZ(max(0, n-1)), swing) + 0.06*lift
	zh := zs + 0.035
	ze := lerpf(zPivot, zh, 0.72)

	rx, ry := pivotX+0.62*armL*math.Cos(armRest-armBend), pivotY+0.62*armL*math.Sin(armRest-armBend)
	hd := ang + 0.45
	hx, hy := math.Cos(hd), math.Sin(hd)
	h0x, h0y := sx-0.07*hx, sy-0.07*hy
	h1x, h1y := sx+0.05*hx, sy+0.05*hy

	surface := func(x, y float64) float64 {
		switch r := math.Hypot(x, y); {
		case r <= recR && n > 0:
			return stackZ(n - 1)
		case r <= platR:
			return platZ
		}
		return 0
	}
	shadowAt := func(x, y, z float64) (float64, float64) {
		b := surface(x, y)
		return dv.toScreen(x+(z-b)*0.45, y+(z-b)*0.3, b)
	}
	shade := func(ax, ay, az, bx, by, bz, w, a float64) {
		x0, y0 := shadowAt(ax, ay, az)
		x1, y1 := shadowAt(bx, by, bz)
		f.seg(x0, y0, x1, y1, w, col{}, a)
	}
	seg := func(ax, ay, az, bx, by, bz, w float64, c col) {
		x0, y0 := dv.toScreen(ax, ay, az)
		x1, y1 := dv.toScreen(bx, by, bz)
		f.seg(x0, y0, x1, y1, w, c, 1)
	}

	R := dv.R
	tube := math.Max(0.6, R*0.017)
	metal, dark, black := hex(0xc6c6cc), hex(0x3a3a40), hex(0x18181c)
	cwx, cwy := pivotX-0.1*bx, pivotY-0.1*by
	cw2x, cw2y := pivotX-0.3*bx, pivotY-0.3*by

	shade(rx, ry, 0, rx, ry, zRest, R*0.02, 0.4)
	shade(pivotX, pivotY, 0, pivotX, pivotY, zPivot*0.6, R*0.12, 0.35)
	shade(cwx, cwy, zPivot, cw2x, cw2y, zPivot, R*0.055, 0.4)
	shade(pivotX, pivotY, zPivot, ex, ey, ze, tube, 0.45)
	shade(ex, ey, ze, sx, sy, zh, tube, 0.45)
	shade(h0x, h0y, zh, h1x, h1y, zh, R*0.03, 0.45)

	seg(rx, ry, 0, rx, ry, zRest-0.01, math.Max(0.6, R*0.02), dark)
	seg(rx-0.04*by, ry+0.04*bx, zRest-0.005, rx+0.04*by, ry-0.04*bx, zRest-0.005, math.Max(0.5, R*0.012), dark.lerp(metal, 0.3))

	puck(f, dv, pivotX, pivotY, 0.12, 0, zPivot*0.6, dark)
	puck(f, dv, pivotX, pivotY, 0.045, zPivot*0.6, zPivot, dark.lerp(metal, 0.5))
	lvx, lvy := pivotX-0.2, pivotY+0.16
	seg(lvx, lvy, 0, lvx, lvy, 0.05, math.Max(0.5, R*0.014), dark)
	seg(lvx, lvy, 0.05, lvx-0.06, lvy+0.05, 0.05, math.Max(0.5, R*0.012), metal.mul(0.8))

	seg(cwx, cwy, zPivot, cw2x, cw2y, zPivot, R*0.055, black.lerp(metal, 0.25))
	seg(pivotX, pivotY, zPivot, ex, ey, ze, tube, metal)
	seg(ex, ey, ze, sx, sy, zh, tube, metal)
	seg(h0x, h0y, zh, h1x, h1y, zh, math.Max(0.8, R*0.03), black)
	fx, fy := h1x-0.05*hy, h1y+0.05*hx
	seg(h1x, h1y, zh, fx, fy, zh+0.01, math.Max(0.5, R*0.01), metal)
	seg(sx, sy, zh, sx, sy, zs, math.Max(0.5, R*0.012), dark)
}

// puck is a short upright cylinder standing at x, y on the deck, from z0 to z1.
func puck(f *fb, dv discView, x, y, r, z0, z1 float64, c col) {
	sn := math.Sin(dv.tilt)
	cx, cy := dv.toScreen(x, y, (z0+z1)/2)
	ext := r*dv.R*1.3 + (z1-z0)*sn*dv.R
	for py := int(cy - ext - 1); py <= int(cy+ext+1); py++ {
		for px := int(cx - ext - 1); px <= int(cx+ext+1); px++ {
			n, topHit := 0.0, 0.0
			for _, o := range samples {
				sx, sy := float64(px)+o[0], float64(py)+o[1]
				for i := range 7 {
					z := z1 - (z1-z0)*float64(i)/6
					X, Y, ok := dv.toDisc(sx, sy+z*sn*dv.R)
					if ok && math.Hypot(X-x, Y-y) <= r {
						n++
						if i == 0 {
							topHit++
						} else {
							topHit += 0.45 + 0.25*clamp01((X-x)/r+0.5)
						}
						break
					}
				}
			}
			if n > 0 {
				f.pixel(px, py, f.pixelAt(px, py).lerp(c.mul(topHit/n*1.4), n/4))
			}
		}
	}
}

// sleeveDisc is the keeper's record at slide p out of its sleeve, flat and upright: it
// slides out toward the deck, left beside it or up above it.
func (m *Model) sleeveDisc(g scanGeom, p float64) discView {
	ax, ay, cs := m.auditionBox(g)
	size := float64(cs)
	dx, dy := -slideOut*size*easeInOut(p), 0.0
	if !g.side {
		dx, dy = 0, dx
	}
	return discView{cx: float64(ax) + size/2 + dx, cy: float64(ay*2) + size/2 + dy,
		R: size / 2 * 0.96 / recR, focal: ttFocal}
}

// drawCarry lays the keeper from the sleeve's mouth onto the stack: it travels in one slow
// arc and lies down into the deck's perspective as it goes.
func (m *Model) drawCarry(f *fb, g scanGeom) {
	c := m.onStage()
	if m.ph != PhScan || c == nil || !c.ok || c.slot < 0 {
		return
	}
	p := prog(c.rel(m.t, c.born), slideAt+slideLen, carryLen)
	if p <= 0 || p >= 1 {
		return
	}
	from := m.sleeveDisc(g, 1)
	to := g.dv
	to.cy -= stackZ(c.slot) * math.Sin(to.tilt) * to.R
	e := easeInOut(p)
	v := discView{cx: lerpf(from.cx, to.cx, e), cy: lerpf(from.cy, to.cy, e) - math.Sin(math.Pi*e)*to.R*0.12,
		R: lerpf(from.R, to.R, e), tilt: lerpf(0, to.tilt, smooth(e)), focal: ttFocal}
	m.drawVinyl(f, v, c.tr.Key(), 0, 1, 0)
}

func (m *Model) drawScanPanel(f *fb, g scanGeom) {
	x, w := g.px, g.pw
	if w >= 30 {
		f.textS(x, g.py-1, fmt.Sprintf(tr("Порция %d", "Batch %d"), m.portionNo), m.th.text, true, false)
	}
	cnt := fmt.Sprintf(tr("%d из %d", "%d of %d"), m.filled, len(m.slots))
	f.textS(m.w-3-len([]rune(cnt)), g.py-1, cnt, m.th.a1, true, false)
	if m.rejected > 0 && w >= 30 {
		miss := fmt.Sprintf(tr("мимо %d", "passed %d"), m.rejected)
		f.text(m.w-6-len([]rune(cnt))-len([]rune(miss)), g.py-1, miss, m.th.dim)
	}

	ax, ay, cs := m.auditionBox(g)
	switch c := m.onStage(); {
	case m.ph == PhDone:
		m.drawNote(f, ax, ay, w)
	case c != nil:
		m.drawAudition(f, c, ax, ay, w, cs, g)
	default:
		m.drawWaiting(f, ax, ay, w)
	}

	limit := m.h - 1
	y := m.drawSide(f, x, m.sideTop(g), w, limit)
	m.drawJournal(f, x, y+1, w, limit)
}

// drawWaiting fills the audition spot while the agent thinks: what it is doing right now.
func (m *Model) drawWaiting(f *fb, x, y, w int) {
	if m.pDone {
		m.wave(f, x, y+1, trunc(tr("собираю сторону A", "putting side A together"), w), m.th.a1)
		return
	}
	m.wave(f, x, y+1, trunc(tr("агент ищет, что подходит", "the agent is looking for fits"), w), m.th.a1)
	hint := tr("каждую находку он поставит на проигрыватель", "each find goes on the turntable for a listen")
	if m.status != "" {
		hint = m.status
	}
	for i, line := range wrapWords(hint, w, 2) {
		f.fade(x, y+3+i, line, m.th.text, 0.85, false)
	}
}

// drawAudition is the candidate on stage: its sleeve, with a keeper's record sliding out of
// it, then title, artist, why and the verdict.
func (m *Model) drawAudition(f *fb, c *cand, x, y, w, cs int, g scanGeom) {
	u := c.rel(m.t, c.born)
	out := 1 - prog(m.t, c.leave-0.3, 0.3)
	decided := c.decided >= 0
	since := m.t - c.decided
	keeper := c.ok && c.slot >= 0

	a := smooth(prog(u, 0, sleeveIn)) * out
	if keeper {
		a *= 1 - smooth(prog(u, slideAt+slideLen, 0.5))
		if p := prog(u, slideAt, slideLen); p > 0 && p < 1 {
			m.drawVinyl(f, m.sleeveDisc(g, p), c.tr.Key(), 0, 1, 0)
		}
	}
	if a > 0 {
		cov := m.cover(c.tr.Key(), cs, cs)
		_, real := m.covers[c.tr.Key()]
		for py := range cs {
			f.pixel(x+cs, y*2+py+1, f.pixelAt(x+cs, y*2+py+1).mul(1-0.45*a))
		}
		for px := range cs {
			f.pixel(x+px+1, y*2+cs, f.pixelAt(x+px+1, y*2+cs).mul(1-0.45*a))
		}
		for py := range cs {
			for px := range cs {
				cc := cov.at(px, py)
				if !real && !c.ok {
					v := 0.1 + 0.08*hash(px/2, py/2, 5)
					cc = col{v, v, v * 1.05}
				}
				if decided && !c.ok {
					l := cc.lum()
					cc = cc.lerp(col{l, l, l}, smooth(prog(since, 0, 0.4)))
				}
				f.pixel(x+px, y*2+py, f.pixelAt(x+px, y*2+py).lerp(cc, a))
			}
		}
	}

	tx, tw := x+cs+3, w-cs-3
	ty := y
	f.fade(tx, ty, trunc(c.tr.Title, tw), m.th.text, smooth(prog(u, 0.1, 0.5))*out, true)
	ty += 2
	f.fade(tx, ty, trunc(c.tr.Artist, tw), m.th.text, smooth(prog(u, 0.2, 0.5))*out, false)
	ty++
	if c.tr.Why != "" {
		for _, line := range wrapWords(c.tr.Why, tw, 2) {
			for i, r := range []rune(line) {
				if cl := f.at(tx+i, ty); cl != nil && r != ' ' {
					f.style(tx+i, ty, r, cl.bg.lerp(m.th.a2.lerp(m.th.text, 0.4), smooth(prog(u, 0.3, 0.5))*out), false, true)
				}
			}
			ty++
		}
	}
	if ty+1 < y+max(cs/2, auditionH)-1 {
		ty++
	}
	switch {
	case !decided:
	case c.ok && c.slot >= 0:
		a := smooth(prog(since, 0, 0.4)) * out
		f.fade(tx, ty, "✓", m.th.a1, a, true)
		n := f.fade(tx+2, ty, fmt.Sprintf("A%d", c.slot+1), m.th.a1, a, true)
		f.fade(tx+3+n, ty, trunc(tr("на сторону A", "onto side A"), tw-3-n), m.th.dim, a, false)
	case c.ok:
		f.fade(tx, ty, "✓", m.th.a1, out, true)
	default:
		a := smooth(prog(since, 0, 0.4)) * out
		f.fade(tx, ty, "✕", m.th.bad, a, true)
		f.fade(tx+2, ty, trunc(c.reason, tw-2), m.th.bad, a, false)
	}
}

// drawNote: the side is together; the agent's line about it types out where the auditions were.
func (m *Model) drawNote(f *fb, x, y, w int) {
	note := []rune(m.note)
	k := int(prog(m.pt, 0.5, 1.2) * float64(len(note)))
	lines := wrapWords(string(note[:k]), w, 3)
	for i, line := range lines {
		f.textS(x, y+1+i, line, m.th.a2.lerp(m.th.text, 0.3), false, true)
	}
	s := tr("сторона A готова", "side A is ready")
	f.fade(x, y+2+max(1, len(lines)), s, m.th.a1, easeOut(prog(m.pt, 0.2, 0.4)), true)
}

// drawSide is the tracklist of side A: a row per keeper, written in as its record lands.
func (m *Model) drawSide(f *fb, x, y, w, limit int) int {
	if y >= limit {
		return y
	}
	f.text(x, y, tr("сторона A", "side A"), m.th.dim)
	y++
	for i, s := range m.slots {
		if y >= limit {
			break
		}
		tag := fmt.Sprintf("A%d", i+1)
		if s.fillAt < 0 || m.t < s.landAt {
			f.text(x+3, y, tag, m.th.dim.mul(0.7))
			f.text(x+7, y, strings.Repeat("· ", max(0, min(12, (w-7)/2))), m.th.dim.mul(0.4))
			y++
			continue
		}
		g := m.cover(s.tr.Key(), 2, 2)
		for py := range 2 {
			for px := range 2 {
				f.pixel(x+px, y*2+py, g.at(px, py))
			}
		}
		f.text(x+3, y, tag, m.th.a1)
		p := prog(m.t, s.landAt, 0.4)
		title := []rune(trunc(s.tr.Title, w-8))
		n := int(math.Ceil(p * float64(len(title))))
		f.text(x+7, y, string(title[:n]), m.th.text)
		if rest := w - 9 - len(title); rest > 3 {
			f.fade(x+9+len(title), y, trunc(s.tr.Artist, rest), m.th.dim, smooth(prog(p, 0.6, 0.4)), false)
		}
		y++
	}
	return y
}

// drawJournal is the agent's earlier steps, newest at the bottom and brightest; the current
// one stands in the audition spot while nothing is on stage.
func (m *Model) drawJournal(f *fb, x, y, w, limit int) {
	lines := m.journal
	if m.ph == PhScan && !m.pDone && m.onStage() == nil && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	room := limit - y
	if room < 2 || len(lines) == 0 {
		return
	}
	lines = lines[max(0, len(lines)-room):]
	for i, l := range lines {
		k := float64(len(lines) - 1 - i)
		a := math.Max(0.25, 0.7-0.12*k) * easeOut(prog(m.t-l.at, 0, 0.3))
		f.fade(x, y+i, "·", m.th.a1, a, false)
		f.fade(x+2, y+i, trunc(l.text, w-2), m.th.text, a, false)
	}
}

// wrapWords breaks s into at most n lines of width w, the last one truncated.
func wrapWords(s string, w, n int) []string {
	if w <= 0 {
		return nil
	}
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case len([]rune(line))+1+len([]rune(word)) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	if len(out) > n {
		last := strings.Join(out[n-1:], " ")
		out = append(out[:n-1], trunc(last, w))
	}
	for i := range out {
		out[i] = trunc(out[i], w)
	}
	return out
}
