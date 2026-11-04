package stage

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// First run: a sound wave curls into a record, the record tilts into 3D, and each connected
// service brings it to life. The needle drop dives through the spindle hole into the app.
// The host does the sign-ins and the model list: the steps send actions and wait for StepDone and Models.

const (
	obReady   = 5.6  // the cinematic is over and the steps take keys
	obTilt    = 0.9  // radians
	obFocal   = 3.2  // perspective, in record radii
	obBrowser = 1.6  // simulated browser round-trip
	obDrop    = 0.95 // needle touches the record, from the start of the drop
	obDive    = 1.9  // dive through the hole starts
	obDiveLen = 1.3
)

type onboard struct {
	t       float64
	step    int // 0 Apple Music, 1 agent, 2 Last.fm, 3 needle
	stepT   float64
	waitT   float64
	replied bool // answered by the host: the browser or the model list
	errMsg  string
	list    []ModelInfo
	total   int
	done    [3]float64
	skipped bool
	errT    float64
	needleT float64
	spin    float64
	spinV   float64

	sub      agentSub
	subT     float64
	prov     int
	url      []rune
	key      []rune
	filter   []rune
	model    int
	effort   int
	picked   string
	langDone bool
	eggsAsk  bool // the eggs switch on screen: the settings, not the first run
	eggsOn   bool
	eggsDone bool
	lfKey    []rune
	lfSecret []rune
	lfSub    int // 0 API key field, 1 secret field
}

// agentSub walks the agent step: provider, address for a custom one, key, model list, effort.
type agentSub int

const (
	SubProvider = iota
	SubURL
	SubKey
	SubLoading
	SubModel
	SubEffort
)

type provider struct{ name, host, keys string }

// providers and efforts carry language data and are rebuilt by setLang.
var providers []provider

// ModelInfo is one model the provider offers.
type ModelInfo struct {
	ID        string
	Reasoning bool // the effort step is offered
	Rec       bool // marked as recommended
}

var efforts []string

func newOnboard() *onboard {
	return &onboard{waitT: -1, done: [3]float64{-1, -1, -1}, errT: -1, needleT: -1, stepT: obReady, effort: 1}
}

// prefsDone gates the connected steps: the language now, and the eggs in the settings.
func (o *onboard) prefsDone() bool { return o.langDone && (!o.eggsAsk || o.eggsDone) }

func (o *onboard) models() []ModelInfo {
	q := strings.ToLower(string(o.filter))
	var out []ModelInfo
	for _, mi := range o.list {
		if strings.Contains(strings.ToLower(mi.ID), q) {
			out = append(out, mi)
		}
	}
	return out
}

func (o *onboard) setSub(s agentSub) {
	o.sub, o.subT, o.errT = s, o.t, -1
	o.replied, o.errMsg = false, ""
}

func (m *Model) onboardStep(dt float64) {
	o := m.ob
	o.t += dt
	target := 0.0
	switch {
	case o.needleT >= 0 && o.t > o.needleT+obDrop:
		target = 2.6
	case o.done[0] >= 0:
		target = 1.4
	case o.t > 3.3:
		target = 0.35
	}
	o.spinV += (target - o.spinV) * (1 - math.Exp(-dt*1.5))
	o.spin += o.spinV * dt
	if o.step == 1 && o.sub == SubLoading && o.replied && o.t-o.subT > 1.2 {
		o.setSub(SubModel)
	}
	if o.waitT >= 0 && o.replied && o.t-o.waitT > obBrowser {
		o.waitT = -1
		m.completeStep()
	}
	if o.step < 3 && o.prefsDone() && o.t-o.stepT >= 0.45 && m.o.Connected[o.step] {
		m.completeStep()
	}
	if o.step == 3 && o.needleT < 0 && o.t-o.done[2] > 0.5 {
		o.needleT = o.t
	}
	if o.needleT >= 0 && o.t > o.needleT+obDrop && o.t-dt <= o.needleT+obDrop {
		m.bloom = 0.5
	}
	if o.needleT >= 0 && o.t > o.needleT+obDive+obDiveLen {
		m.ob = nil
		m.setPhase(phIntro)
		m.pt = 2.0
	}
}

func (m *Model) completeStep() {
	o := m.ob
	o.done[o.step] = o.t
	o.step++
	o.stepT = o.t
	o.replied, o.errMsg = false, ""
	m.bloom = 0.3
	if o.step == 3 {
		m.send(o.setup())
	}
}

func (m *Model) onboardKey(k, text string) {
	o := m.ob
	if o.t < obReady-0.4 || o.waitT >= 0 && (o.step != 2 || k != "tab") || o.step > 2 {
		return
	}
	o.errMsg = ""
	switch o.step {
	case 0:
		if !o.langDone {
			switch k {
			case "left", "right", "h", "l":
				if lang == "ru" {
					SetLang("en")
				} else {
					SetLang("ru")
				}
			case "enter":
				o.langDone, o.stepT = true, o.t
			}
			return
		}
		if o.eggsAsk && !o.eggsDone {
			switch k {
			case "left", "right", "h", "l":
				o.eggsOn = !o.eggsOn
			case "enter":
				o.eggsDone, o.stepT = true, o.t
				m.o.NoEggs = !o.eggsOn
				clear(m.themes)
				m.send(Eggs{On: o.eggsOn})
			}
			return
		}
		if k == "enter" {
			o.waitT = o.t
			m.send(ConnectApple{})
		}
	case 1:
		m.agentKey(k, text)
	case 2:
		if !m.o.LastfmAskKeys {
			switch k {
			case "enter":
				o.waitT = o.t
				m.send(ConnectLastfm{})
			case "tab":
				o.skipped, o.waitT = true, -1
				m.completeStep()
			}
			return
		}
		edit := func(buf *[]rune) {
			switch {
			case k == "backspace":
				if len(*buf) > 0 {
					*buf = (*buf)[:len(*buf)-1]
				}
			case len(text) > 0:
				*buf = append(*buf, []rune(text)...)
			}
		}
		switch {
		case k == "tab":
			o.skipped, o.waitT = true, -1
			m.completeStep()
		case k == "enter" && o.lfSub == 0:
			if len(o.lfKey) == 0 {
				o.errMsg = tr("нужен API key", "API key needed")
			} else {
				o.lfSub = 1
			}
		case k == "enter":
			if len(o.lfSecret) == 0 {
				o.errMsg = tr("нужен secret", "secret needed")
			} else {
				o.waitT = o.t
				m.o.LastfmKey = string(o.lfKey)
				m.send(ConnectLastfm{Key: string(o.lfKey), Secret: string(o.lfSecret)})
			}
		case k == "esc" && o.lfSub > 0:
			o.lfSub = 0
		default:
			if o.lfSub == 0 {
				edit(&o.lfKey)
			} else {
				edit(&o.lfSecret)
			}
		}
	}
}

func (m *Model) agentKey(k, text string) {
	o := m.ob
	edit := func(buf *[]rune) {
		switch {
		case k == "backspace":
			if len(*buf) > 0 {
				*buf = (*buf)[:len(*buf)-1]
			}
		case len(text) > 0:
			*buf = append(*buf, []rune(text)...)
		}
	}
	if k == "esc" {
		switch o.sub {
		case SubLoading:
			o.setSub(SubKey)
		case SubURL:
			o.setSub(SubProvider)
		case SubKey:
			if providers[o.prov].keys == "" {
				o.setSub(SubURL)
			} else {
				o.setSub(SubProvider)
			}
		case SubModel:
			o.setSub(SubKey)
		case SubEffort:
			o.setSub(SubModel)
		}
		return
	}
	switch o.sub {
	case SubProvider:
		switch k {
		case "up", "k":
			o.prov = max(0, o.prov-1)
		case "down", "j":
			o.prov = min(len(providers)-1, o.prov+1)
		case "enter":
			if providers[o.prov].keys == "" {
				o.setSub(SubURL)
			} else {
				o.setSub(SubKey)
			}
		}
	case SubURL:
		if k == "enter" {
			if len(o.url) == 0 {
				o.errT = o.t
			} else {
				o.setSub(SubKey)
			}
			return
		}
		edit(&o.url)
	case SubKey:
		if k == "enter" {
			if len(o.key) == 0 && providers[o.prov].keys != "" {
				o.errT = o.t
			} else {
				o.setSub(SubLoading)
				m.send(LoadModels{URL: o.agentURL(), Key: string(o.key)})
			}
			return
		}
		edit(&o.key)
	case SubModel:
		list := o.models()
		switch k {
		case "up":
			o.model = max(0, o.model-1)
		case "down":
			o.model = max(0, min(len(list)-1, o.model+1))
		case "enter":
			if len(list) == 0 {
				return
			}
			o.picked = list[o.model].ID
			if list[o.model].Reasoning {
				o.setSub(SubEffort)
			} else {
				m.completeStep()
			}
		default:
			edit(&o.filter)
			o.model = 0
		}
	case SubEffort:
		switch k {
		case "left", "h":
			o.effort = max(0, o.effort-1)
		case "right", "l":
			o.effort = min(len(efforts)-1, o.effort+1)
		case "enter":
			m.completeStep()
		}
	}
}

// agentURL is the API base of the chosen provider: scheme for the fixed ones, the
// entered address with a scheme added when the custom one lacks it.
func (o *onboard) agentURL() string {
	if p := providers[o.prov]; p.keys != "" {
		return "https://" + p.host
	}
	u := string(o.url)
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	return u
}

// pickedInfo is the model chosen on the model step.
func (o *onboard) pickedInfo() (ModelInfo, bool) {
	for _, mi := range o.list {
		if mi.ID == o.picked {
			return mi, true
		}
	}
	return ModelInfo{}, false
}

// setup is what the onboarding settled, scored over the chosen provider; the agent step
// closed by the host (Connected) carries no credential.
func (o *onboard) setup() Setup {
	url, key, model := "", "", ""
	if o.picked != "" {
		url, key, model = o.agentURL(), string(o.key), o.picked
	}
	effort := ""
	if mi, ok := o.pickedInfo(); ok && mi.Reasoning {
		effort = [...]string{"low", "medium", "high"}[o.effort]
	}
	return Setup{Lang: lang, URL: url, Key: key, Model: model, Effort: effort,
		Lastfm: o.done[2] >= 0 && !o.skipped}
}

// StepDone reports the browser sign-in finishing on the Apple Music or Last.fm step;
// err != "" means it failed and the step stays on screen with the message.
func (m *Model) StepDone(step int, err string) {
	o := m.ob
	if m.ob == nil || o.step != step || o.waitT < 0 {
		return
	}
	if err != "" {
		o.waitT, o.replied, o.errMsg = -1, false, err
		return
	}
	o.replied = true
	if step == 2 {
		m.o.LastfmAskKeys = false
	}
}

// Models replies to LoadModels: the models that support tools, how many the provider has
// in all, or the failure which puts the key substep back on screen with the message.
func (m *Model) Models(list []ModelInfo, total int, err string) {
	o := m.ob
	if m.ob == nil || o.step != 1 || o.sub != SubLoading {
		return
	}
	if err != "" {
		o.setSub(SubKey)
		o.errMsg = err
		return
	}
	o.list, o.total, o.replied = list, total, true
	o.model = max(0, slices.IndexFunc(o.models(), func(mi ModelInfo) bool { return mi.Rec }))
}

// Onboarding reports where the onboarding stands: the current step, the agent step's
// substep, the phase seconds and whether the onboarding is on screen.
func (m *Model) Onboarding() (step, sub int, t float64, ok bool) {
	if m.ob == nil {
		return 0, 0, 0, false
	}
	return m.ob.step, int(m.ob.sub), m.ob.t, true
}

// ---------- layout ----------

type obLayout struct {
	cx, cy, R  float64 // final record placement, pixels
	px, py, pw int     // steps panel, cells
}

func (m *Model) obLayout() obLayout {
	w, h := float64(m.w), float64(m.h)
	if m.w >= 2*m.h+10 {
		R := math.Max(8, math.Min(h-3, math.Min(w/5, (w-44)/2.4)))
		pw := min(56, m.w-int(R*2.4)-11)
		shift := math.Max(0, (w-(R*2.4+7+float64(pw)))/2-4)
		cx := 4 + R + shift
		return obLayout{cx: cx, cy: h - 1, R: R, px: int(cx+R*1.4) + 4, py: max(1, (m.h-22)/2), pw: pw}
	}
	R := math.Max(6, math.Min((w-6)/2.6, math.Min(h-3, (h-18)/0.75)))
	cx := 3 + R + math.Max(0, (w-6-R*2.6)/2)
	cy := 4 + R*math.Cos(obTilt)*1.1
	_, bottom := discView{cx: cx, cy: cy, R: R, tilt: obTilt}.toScreen(0, 1, 0)
	pw := min(56, m.w-6)
	return obLayout{cx: cx, cy: cy, R: R, px: max(3, (m.w-pw)/2), py: int(bottom/2) + 3, pw: pw}
}

type discView struct {
	cx, cy, R float64
	tilt      float64
	spin      float64
	focal     float64 // perspective, in record radii; 0 means obFocal
}

func (d discView) fl() float64 {
	if d.focal > 0 {
		return d.focal
	}
	return obFocal
}

// toDisc maps a screen pixel to the record plane, unit radius; the top edge leans away from the viewer.
func (d discView) toDisc(sx, sy float64) (float64, float64, bool) {
	u, v := (sx-d.cx)/d.R, (sy-d.cy)/d.R
	sn, cs := math.Sin(d.tilt), math.Cos(d.tilt)
	fl := d.fl()
	den := fl*cs + v*sn
	if den <= 0.05 {
		return 0, 0, false
	}
	y := v * fl / den
	s := fl / (fl - y*sn)
	return u / s, y, true
}

func (d discView) toScreen(x, y, z float64) (sx, sy float64) {
	sn, cs := math.Sin(d.tilt), math.Cos(d.tilt)
	s := d.fl() / (d.fl() - y*sn)
	return d.cx + x*s*d.R, d.cy + (y*cs*s-z*sn)*d.R
}

type discLook struct {
	fill    float64 // the record grows in from the rim
	light   float64
	accent  col
	label   col
	wipe    float64 // label takes the accent from the center out
	rainbow float64
	sweep   float64 // a light ring runs down the grooves, -1 when idle
	ripple  float64 // age of the needle ripple, -1 when idle
	rx, ry  float64
	print   bool // the label is big enough to carry lettering
}

func (m *Model) discState() (discView, *discLook) {
	o, lay := m.ob, m.obLayout()
	w, h := float64(m.w), float64(m.h)
	mv := easeInOut(prog(o.t, 3.3, 1.7))
	dv := discView{cx: lerpf(w/2, lay.cx, mv), cy: lerpf(h, lay.cy, mv), R: lay.R, tilt: obTilt * mv, spin: o.spin}
	lk := &discLook{fill: easeOut(prog(o.t, 2.5, 1.0)), light: 0.25 + 0.75*mv, accent: m.th.a1,
		label: hex(0x2a2a30), sweep: -1, ripple: -1, print: lay.R >= 40}
	if o.done[0] >= 0 {
		lk.wipe = easeOut(prog(o.t, o.done[0], 0.7))
	}
	if o.done[1] >= 0 {
		lk.rainbow = easeOut(prog(o.t, o.done[1], 1.2))
		if s := prog(o.t, o.done[1], 1.0); s < 1 {
			lk.sweep = s
		}
	}
	if o.needleT >= 0 {
		if a := o.t - o.needleT - obDrop; a >= 0 && a < 1.2 {
			lk.ripple = a
			lk.rx, lk.ry = armHead(108)
		}
		if z := prog(o.t, o.needleT+obDive, obDiveLen); z > 0 {
			e := easeInOut(z)
			dv.tilt *= 1 - easeInOut(math.Min(1, z*1.6))
			dv.cx, dv.cy = lerpf(dv.cx, w/2, e), lerpf(dv.cy, h, e)
			k := math.Hypot(w, 2*h) / 2 / (0.03 * dv.R) * 1.3
			dv.R *= math.Pow(k, easeIn(z))
			lk.print = lk.print || dv.R >= 40
		}
	}
	return dv, lk
}

// ---------- record ----------

func (lk *discLook) shade(x, y, spin float64) (col, float64) {
	r := math.Hypot(x, y)
	if r > 1 || r < 0.03 || r < 1-lk.fill {
		return col{}, 0
	}
	th := math.Atan2(y, x)
	var c col
	if r < 0.34 {
		c = lk.labelAt(x, y, r, spin)
	} else {
		c = lk.vinyl(r, th, spin)
	}
	if lk.fill < 1 {
		c = c.lerp(lk.accent.lerp(white, 0.4), math.Exp(-(r-(1-lk.fill))/0.03))
	}
	if lk.ripple >= 0 {
		d := math.Hypot(x-lk.rx, y-lk.ry)
		rr := lk.ripple * 1.4
		c = c.lerp(lk.accent.lerp(white, 0.3), 0.7*math.Exp(-(d-rr)*(d-rr)/0.0008)*(1-lk.ripple/1.2))
	}
	return c, 1
}

func (lk *discLook) labelAt(x, y, r, spin float64) col {
	gray := lk.label
	edge := 0.34 * lk.wipe
	c := gray.lerp(lk.accent.mul(0.9), smooth((edge-r)/0.04+0.5))
	if r > 0.325 {
		c = c.mul(0.75)
	}
	c = c.mul(0.92 - 0.12*y)
	cs, sn := math.Cos(spin), math.Sin(spin)
	lx, ly := x*cs+y*sn, -x*sn+y*cs
	const unit = 0.36 / 17
	gx, gy := int(math.Floor((lx+0.18)/unit)), int(math.Floor((ly+0.21)/unit))
	if lk.print && gx >= 0 && gx < 17 && gy >= 0 && gy < 5 && gx%6 < 5 {
		if font5[rune("AUX"[gx/6])][gy][gx%6] == '#' {
			c = c.mul(0.3)
		}
	}
	if math.Abs(r-0.24) < 0.004 {
		c = c.mul(0.8)
	}
	return c
}

func (lk *discLook) vinyl(r, th, spin float64) col {
	v := 0.045 * (0.8 + 0.4*hash(int(r*900), 0, 1))
	for _, g := range [...]float64{0.47, 0.6, 0.72, 0.84} {
		if math.Abs(r-g) < 0.006 {
			v = 0.085
		}
	}
	if r > 0.955 {
		v = 0.07
	}
	if r > 0.99 {
		v = 0.22
	}
	c := col{v, v, v * 1.15}

	a := math.Abs(math.Cos(th + 2.3))
	sh := (math.Pow(a, 30)*0.55 + math.Pow(a, 4)*0.07) * lk.light * (0.4 + 0.6*r)
	sc := white.lerp(lk.accent, 0.2)
	if lk.rainbow > 0 {
		sc = sc.lerp(hsv(r*720+th*20, 0.55, 1), lk.rainbow*0.8)
	}
	c = col{c.r + sc.r*sh, c.g + sc.g*sh, c.b + sc.b*sh}

	if hash(int(r*160), int(math.Mod(th-spin+40*math.Pi, 2*math.Pi)*90), 5) > 0.996 {
		c = c.lerp(white, 0.3)
	}
	if lk.sweep >= 0 {
		rs := 0.97 - lk.sweep*0.62
		c = c.lerp(lk.accent.lerp(white, 0.3), 0.8*math.Exp(-(r-rs)*(r-rs)/0.0005))
	}
	return c
}

func (m *Model) sampleDisc(sx, sy float64, dv discView, lk *discLook) (col, float64) {
	X, Y, ok := dv.toDisc(sx, sy)
	if ok {
		if c, a := lk.shade(X, Y, dv.spin); a > 0 {
			return c, a
		}
		if math.Hypot(X, Y) <= 1 {
			return col{}, 0
		}
	}
	if lk.fill < 1 {
		return col{}, 0
	}
	sn := math.Sin(dv.tilt)
	if X2, Y2, ok2 := dv.toDisc(sx, sy-0.045*dv.R*sn); ok2 && Y2 > 0 && math.Hypot(X2, Y2) <= 1 {
		return col{0.025, 0.025, 0.03}, 1
	}
	if ok {
		rs := math.Hypot(X, Y-0.1)
		if dark := (1 - smooth((rs-0.92)/0.4)) * 0.6 * sn / math.Sin(obTilt); dark > 0 {
			return col{}, dark
		}
	}
	return col{}, 0
}

func (m *Model) drawDisc(f *fb, dv discView, lk *discLook) {
	x0, x1 := max(0, int(dv.cx-dv.R*1.35)), min(f.w-1, int(dv.cx+dv.R*1.35)+1)
	y0, y1 := max(0, int(dv.cy-dv.R*1.3)), min(f.h*2-1, int(dv.cy+dv.R*1.45)+1)
	offs := [4][2]float64{{0.25, 0.25}, {0.75, 0.25}, {0.25, 0.75}, {0.75, 0.75}}
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			var s col
			a := 0.0
			for _, o := range offs {
				c, ca := m.sampleDisc(float64(px)+o[0], float64(py)+o[1], dv, lk)
				s = col{s.r + c.r*ca, s.g + c.g*ca, s.b + c.b*ca}
				a += ca
			}
			if a < 0.01 {
				continue
			}
			bg := f.pixelAt(px, py)
			k := 1 - a/4
			f.pixel(px, py, col{bg.r*k + s.r/4, bg.g*k + s.g/4, bg.b*k + s.b/4})
		}
	}
}

// ---------- tonearm ----------

var armPivot = [2]float64{1.22, -0.92}

func armHead(deg float64) (x, y float64) {
	a := deg * math.Pi / 180
	return armPivot[0] + 1.5*math.Cos(a), armPivot[1] + 1.5*math.Sin(a)
}

func (m *Model) drawArm(f *fb, dv discView) {
	o := m.ob
	vis := easeOut(prog(o.t, 4.9, 0.6))
	if vis <= 0 {
		return
	}
	deg, z := 93.0, 0.09
	if o.needleT >= 0 {
		nt := o.t - o.needleT
		deg = lerpf(93, 108, easeInOut(prog(nt, 0, 0.7)))
		z = lerpf(0.09, 0.012, easeInOut(prog(nt, 0.7, obDrop-0.7)))
	}
	a := deg * math.Pi / 180
	dx, dy := math.Cos(a), math.Sin(a)
	px, py := armPivot[0], armPivot[1]
	hx, hy := armHead(deg)
	hw := math.Max(0.6, dv.R*0.016)
	metal, dark := hex(0xb9b9c0), hex(0x26262b)

	ax, ay := dv.toScreen(px, py, 0)
	bx, by := dv.toScreen(hx, hy, 0)
	f.seg(ax+hw, ay+hw, bx+hw, by+hw, hw*1.2, col{}, 0.35*vis)

	cx, cy := dv.toScreen(px, py, 0.02)
	pr := dv.R * 0.09
	f.seg(cx, cy, cx, cy, pr, dark, vis)
	f.seg(cx-pr*0.3, cy-pr*0.3, cx-pr*0.3, cy-pr*0.3, pr*0.35, hex(0x4a4a52), vis)

	ax, ay = dv.toScreen(px, py, 0.09)
	wx, wy := dv.toScreen(px-0.26*dx, py-0.26*dy, 0.09)
	f.seg(ax, ay, wx, wy, hw*2.4, dark.lerp(metal, 0.3), vis)
	bx, by = dv.toScreen(hx-0.12*dx, hy-0.12*dy, z+0.01)
	f.seg(ax, ay, bx, by, hw, metal, vis)
	tx, ty := dv.toScreen(hx, hy, z)
	f.seg(bx, by, tx, ty, hw*2, dark, vis)
	f.seg(tx, ty, tx, ty, hw*0.9, m.th.a1, vis)
}

// seg draws an antialiased capsule in pixel space, shaded like a metal rod.
func (f *fb) seg(ax, ay, bx, by, hw float64, c col, a float64) {
	x0, x1 := int(math.Floor(math.Min(ax, bx)-hw-1)), int(math.Ceil(math.Max(ax, bx)+hw+1))
	y0, y1 := int(math.Floor(math.Min(ay, by)-hw-1)), int(math.Ceil(math.Max(ay, by)+hw+1))
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	for py := max(0, y0); py <= min(f.h*2-1, y1); py++ {
		for px := max(0, x0); px <= min(f.w-1, x1); px++ {
			qx, qy := float64(px)+0.5, float64(py)+0.5
			t := 0.0
			if l2 > 0 {
				t = clamp01(((qx-ax)*dx + (qy-ay)*dy) / l2)
			}
			d := math.Hypot(qx-ax-t*dx, qy-ay-t*dy)
			cov := clamp01(hw + 0.5 - d)
			if cov <= 0 {
				continue
			}
			k := d / math.Max(hw, 0.5)
			f.pixel(px, py, f.pixelAt(px, py).lerp(c.mul(1-0.45*k*k), cov*a))
		}
	}
}

// ---------- sound wave ----------

type lightbuf struct {
	w, h int
	v    []float64
}

func (l *lightbuf) add(x, y, a float64) {
	ix, iy := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(ix), y-float64(iy)
	for _, p := range [4]struct {
		dx, dy int
		k      float64
	}{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
		if x, y := ix+p.dx, iy+p.dy; x >= 0 && y >= 0 && x < l.w && y < l.h {
			l.v[y*l.w+x] += a * p.k
		}
	}
}

func (l *lightbuf) blur(passes int) {
	tmp := make([]float64, len(l.v))
	const r = 2
	for range passes {
		for y := 0; y < l.h; y++ {
			for x := 0; x < l.w; x++ {
				s := 0.0
				for k := -r; k <= r; k++ {
					if xx := x + k; xx >= 0 && xx < l.w {
						s += l.v[y*l.w+xx]
					}
				}
				tmp[y*l.w+x] = s / (2*r + 1)
			}
		}
		for y := 0; y < l.h; y++ {
			for x := 0; x < l.w; x++ {
				s := 0.0
				for k := -r; k <= r; k++ {
					if yy := y + k; yy >= 0 && yy < l.h {
						s += tmp[yy*l.w+x]
					}
				}
				l.v[y*l.w+x] = s / (2*r + 1)
			}
		}
	}
}

func (m *Model) drawWave(f *fb, r float64) {
	t := m.ob.t
	fade := 1 - prog(t, 2.6, 0.8)
	if fade <= 0 {
		return
	}
	lb := &lightbuf{w: f.w, h: f.h * 2, v: make([]float64, f.w*f.h*2)}
	cx, cy := float64(f.w)/2, float64(f.h)
	grow := easeInOut(prog(t, 0.45, 1.0))
	curl := easeInOut(prog(t, 1.5, 1.1))
	level := easeOut(prog(t, 0.5, 0.9))
	lw := math.Min(float64(f.w)-8, 2*math.Pi*r)
	dot := easeOut(prog(t, 0.1, 0.4)) * (1 - grow)
	lb.add(cx, cy, 6*dot)
	const n = 1400
	wgt := lerpf(lw, 2*math.Pi*r, curl) / n * 1.2
	core := make([][2]float64, 0, n+1)
	for i := 0; i <= n; i++ {
		u := float64(i) / n
		if math.Abs(u-0.5)*2 > grow {
			continue
		}
		env := lerpf(math.Pow(math.Sin(math.Pi*u), 1.5), 0.7, curl)
		amp := env * level * (0.55*math.Sin(u*38+t*7) + 0.3*math.Sin(u*91-t*11) + 0.15*math.Sin(u*173+t*17))
		lx, ly := cx+(u-0.5)*lw, cy+amp*r*0.4
		ang := -math.Pi/2 + u*2*math.Pi
		rad := r * (1 + amp*0.14)
		x, y := lerpf(lx, cx+math.Cos(ang)*rad, curl), lerpf(ly, cy+math.Sin(ang)*rad, curl)
		lb.add(x, y, wgt)
		core = append(core, [2]float64{x, y})
	}
	lb.blur(2)
	for py := 0; py < lb.h; py++ {
		for px := 0; px < lb.w; px++ {
			if v := lb.v[py*lb.w+px]; v > 0.004 {
				f.pixel(px, py, f.pixelAt(px, py).lerp(m.th.a1, math.Min(0.85, v*3)*fade))
			}
		}
	}
	hot := m.th.a1.lerp(white, 0.55)
	for _, p := range core {
		px, py := int(p[0]), int(p[1])
		f.pixel(px, py, f.pixelAt(px, py).lerp(hot, 0.9*fade))
	}
	if dot > 0 {
		f.pixel(int(cx), int(cy), f.pixelAt(int(cx), int(cy)).lerp(hot, dot))
	}
}

// ---------- frame ----------

func (m *Model) drawOnboard(f *fb) {
	o := m.ob
	dv, lk := m.discState()
	if o.t < 3.4 {
		m.drawWave(f, m.obLayout().R)
	}
	if o.t > 2.45 {
		m.drawDisc(f, dv, lk)
	}
	m.drawArm(f, dv)
	m.drawPanel(f)
	if o.needleT >= 0 && o.t > o.needleT+obDive {
		m.portal(f, dv)
	}
}

// portal shows the app through the spindle hole while the camera dives into it.
func (m *Model) portal(f *fb, dv discView) {
	in := newFB(f.w, f.h)
	saved := m.pt
	m.pt = 2.0
	m.backdrop(in)
	m.drawIntro(in)
	m.pt = saved
	hole := func(px, py int) bool {
		X, Y, ok := dv.toDisc(float64(px)+0.5, float64(py)+0.5)
		return ok && math.Hypot(X, Y) < 0.03
	}
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			top, bot := hole(x, y*2), hole(x, y*2+1)
			i := y*f.w + x
			switch {
			case top && bot:
				f.c[i] = in.c[i]
			case top:
				f.pixel(x, y*2, in.c[i].bg)
			case bot:
				f.pixel(x, y*2+1, in.c[i].bg)
			}
		}
	}
}

func (m *Model) drawPanel(f *fb) {
	o, lay := m.ob, m.obLayout()
	out := 1.0
	if o.needleT >= 0 {
		out = 1 - prog(o.t, o.needleT+obDive, 0.35)
	}
	if out <= 0 {
		return
	}
	in := func(i int) float64 { return easeOut(prog(o.t, 4.8+float64(i)*0.08, 0.5)) * out }
	x, y, pw := lay.px, lay.py, lay.pw

	sc := 1
	if pw >= 34 && m.h >= 28 {
		sc = 2
	}
	width := 3*5*sc + 2*3
	sweep := prog(o.t, 5.3, 0.9)*1.6 - 0.3
	for i, r := range "AUX" {
		for gy, row := range font5[r] {
			for gx, b := range row {
				if b != '#' {
					continue
				}
				col0 := i*(5*sc+3) + gx*sc
				u := float64(col0) / float64(width)
				k := easeOut(prog(o.t, 4.6+u*0.4, 0.4)) * out
				c := m.th.a1.lerp(m.th.a2, 0.25).lerp(white, 0.8*math.Exp(-(u-sweep)*(u-sweep)/0.006))
				for sy := 0; sy < sc; sy++ {
					for sx := 0; sx < sc; sx++ {
						px, py := x+col0+sx, y*2+gy*sc+sy
						f.pixel(px, py, f.pixelAt(px, py).lerp(c, k))
					}
				}
			}
		}
	}
	y += (5*sc+1)/2 + 1
	f.fade(x, y, trunc(tr("музыку подбирает агент", "music, picked by an agent"), pw), m.th.dim, in(1), false)
	y += 2

	{
		a := in(2)
		name := tr("Язык", "Language")
		if o.langDone {
			f.fade(x, y, "✓", m.th.a1, a, true)
			f.fade(x+2, y, name, m.th.text.lerp(m.th.dim, 0.3), a, false)
			status := tr("Русский", "English")
			f.fade(x+pw-len([]rune(status)), y, status, m.th.dim, a*0.8, false)
			y++
		} else {
			f.fade(x, y, "›", m.th.a1, a, true)
			f.fade(x+2, y, name, m.th.text, a, true)
			y++
			y += m.drawLangBody(f, x+2, y, pw-2, a*easeOut(prog(o.t, o.stepT, 0.35)))
		}
	}
	if o.eggsAsk {
		a := in(2)
		name := tr("Пасхалки", "Easter eggs")
		switch {
		case o.eggsDone:
			f.fade(x, y, "✓", m.th.a1, a, true)
			f.fade(x+2, y, name, m.th.text.lerp(m.th.dim, 0.3), a, false)
			status := tr("вкл", "on")
			if !o.eggsOn {
				status = tr("выкл", "off")
			}
			f.fade(x+pw-len([]rune(status)), y, status, m.th.dim, a*0.8, false)
			y++
		case o.langDone:
			f.fade(x, y, "›", m.th.a1, a, true)
			f.fade(x+2, y, name, m.th.text, a, true)
			y++
			cur := 0
			if !o.eggsOn {
				cur = 1
			}
			y += m.drawPick(f, x+2, y, []string{tr("вкл", "on"), tr("выкл", "off")}, cur, a*easeOut(prog(o.t, o.stepT, 0.35)))
		default:
			f.fade(x, y, "·", m.th.dim, a, true)
			f.fade(x+2, y, name, m.th.dim, a, false)
			y++
		}
	}

	names := [3]string{"Apple Music", tr("Агент", "Agent"), "Last.fm"}
	for i, name := range names {
		a := in(3 + i)
		mark, mc, nc := "·", m.th.dim, m.th.dim
		status := ""
		switch {
		case o.done[i] >= 0 && i == 2 && o.skipped:
			status = tr("пропущено", "skipped")
		case o.done[i] >= 0:
			mark, mc, nc, status = "✓", m.th.a1, m.th.text.lerp(m.th.dim, 0.3), tr("подключено", "connected")
			if i == 1 {
				status = trunc(m.agentStatus(), pw-len([]rune(name))-4)
			}
			fl := 0.25 * math.Exp(-(o.t-o.done[i])*3) * a
			for xx := x - 1; xx < x+pw; xx++ {
				f.tint(xx, y, m.th.a1, fl)
			}
		case i == o.step && o.prefsDone():
			mark, mc, nc = "›", m.th.a1, m.th.text
		}
		f.fade(x, y, mark, mc, a, true)
		f.fade(x+2, y, name, nc, a, i == o.step && o.prefsDone())
		if status != "" {
			f.fade(x+pw-len([]rune(status)), y, status, m.th.dim, a*0.8, false)
		}
		y++
		if i == o.step && o.prefsDone() {
			a := a * easeOut(prog(o.t, o.stepT, 0.35))
			y += m.drawStepBody(f, i, x+2, y, pw-2, a)
		}
	}
	if o.step == 3 {
		y++
		m.wave(f, x, y, tr("ставлю иглу", "dropping the needle"), m.th.a1)
		if out < 1 {
			for xx := x; xx < x+11; xx++ {
				if c := f.at(xx, y); c != nil {
					c.fg = c.bg.lerp(c.fg, out)
				}
			}
		}
	}
}

// drawPick lays a choice out in one line, underlines the current one and shows the key hints.
func (m *Model) drawPick(f *fb, x, y int, names []string, cur int, a float64) int {
	ux, sx := x, x
	for i, n := range names {
		if i == cur {
			sx = ux
			f.fade(ux, y, n, m.th.a1, a, true)
		} else {
			f.fade(ux, y, n, m.th.dim, a, false)
		}
		ux += len([]rune(n)) + 3
	}
	f.fade(sx, y+1, strings.Repeat("▔", len([]rune(names[cur]))), m.th.a1, a*0.8, false)
	key := func(x, y int, k, label string) int {
		n := f.fade(x, y, k, m.th.a1, a, true)
		return n + 1 + f.fade(x+n+1, y, label, m.th.dim, a, false) + 3
	}
	dx := key(x, y+2, "←→", tr("выбрать", "choose"))
	key(x+dx, y+2, "enter", tr("дальше", "continue"))
	return 4
}

func (m *Model) drawLangBody(f *fb, x, y, w int, a float64) int {
	cur := 1
	if lang == "en" {
		cur = 0
	}
	return m.drawPick(f, x, y, []string{"English", "Русский"}, cur, a)
}

func (m *Model) drawStepBody(f *fb, i, x, y, w int, a float64) int {
	o := m.ob
	if o.waitT >= 0 {
		m.wave(f, x, y, tr("жду ответ из браузера", "waiting for the browser"), m.th.a1)
		return 2
	}
	key := func(x, y int, k, label string) int {
		n := f.fade(x, y, k, m.th.a1, a, true)
		return n + 1 + f.fade(x+n+1, y, label, m.th.dim, a, false) + 3
	}
	switch i {
	case 0:
		f.fade(x, y, trunc(tr("откроется браузер, войди в Apple ID", "a browser will open, sign in to Apple ID"), w), m.th.dim, a, false)
		key(x, y+1, "enter", tr("войти", "sign in"))
		if o.errMsg != "" {
			f.fade(x, y+2, trunc(o.errMsg, w), m.th.bad, a, false)
		}
	case 1:
		return m.drawAgentBody(f, x, y, w, a*easeOut(prog(o.t, o.subT, 0.3)))

	case 2:
		if !m.o.LastfmAskKeys {
			f.fade(x, y, trunc(tr("скробблинг, по желанию", "scrobbling, optional"), w), m.th.dim, a, false)
			dx := key(x, y+1, "enter", tr("подключить", "connect"))
			key(x+dx, y+1, "tab", tr("пропустить", "skip"))
			if o.errMsg != "" {
				f.fade(x, y+2, trunc(o.errMsg, w), m.th.bad, a, false)
			}
			return 3
		}
		f.fade(x, y, trunc(tr("ключи: last.fm/api/account/create", "keys: last.fm/api/account/create"), w), m.th.dim, a, false)
		fields := [2]struct {
			label string
			val   []rune
			mask  bool
		}{{"API key", o.lfKey, false}, {"secret", o.lfSecret, true}}
		for i, fl := range fields {
			if i == o.lfSub {
				f.fade(x, y+1+i, "›", m.th.a1, a, true)
			}
			f.fade(x+2, y+1+i, fl.label, m.th.dim, a, false)
			text := string(fl.val)
			if fl.mask {
				text = strings.Repeat("•", len(fl.val))
			}
			if r, n := []rune(text), max(0, w-13); len(r) > n {
				text = string(r[len(r)-n:])
			}
			if i == o.lfSub {
				n := f.fade(x+12, y+1+i, text, m.th.text, a, false)
				f.fade(x+12+n, y+1+i, "▎", m.th.a1, a*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
			} else if text != "" {
				f.fade(x+12, y+1+i, text, m.th.text, a, false)
			}
		}
		next := tr("дальше", "next")
		if o.lfSub == 1 {
			next = tr("подключить", "connect")
		}
		dx := key(x, y+3, "enter", next)
		dx += key(x+dx, y+3, "tab", tr("пропустить", "skip"))
		if o.lfSub == 1 {
			key(x+dx, y+3, "esc", tr("назад", "back"))
		}
		if o.errMsg != "" {
			f.fade(x, y+4, trunc(o.errMsg, w), m.th.bad, a, false)
		}
		return 6
	}
	return 3
}

func (m *Model) drawAgentBody(f *fb, x, y, w int, a float64) int {
	o := m.ob
	key := func(x, y int, k, label string) int {
		n := f.fade(x, y, k, m.th.a1, a, true)
		return n + 1 + f.fade(x+n+1, y, label, m.th.dim, a, false) + 3
	}
	p := providers[o.prov]
	switch o.sub {
	case SubProvider:
		f.fade(x, y, tr("откуда брать модель", "where the model comes from"), m.th.dim, a, false)
		for i, pr := range providers {
			if i == o.prov {
				f.fade(x, y+1+i, "›", m.th.a1, a, true)
				f.fade(x+2, y+1+i, pr.name, m.th.text, a, false)
			} else {
				f.fade(x+2, y+1+i, pr.name, m.th.dim, a, false)
			}
			if len([]rune(pr.name))+len([]rune(pr.host))+4 <= w {
				f.fade(x+w-len([]rune(pr.host)), y+1+i, pr.host, m.th.dim, a*0.7, false)
			}
		}
		dx := key(x, y+1+len(providers), "↑↓", tr("выбрать", "choose"))
		key(x+dx, y+1+len(providers), "enter", tr("дальше", "continue"))
		return len(providers) + 3
	case SubURL:
		f.fade(x, y, tr("адрес OpenAI-совместимого API", "OpenAI-compatible API address"), m.th.dim, a, false)
		f.fade(x, y+1, "›", m.th.a1, a, true)
		url := o.url
		if n := max(0, w-6); len(url) > n {
			url = url[len(url)-n:]
		}
		n := f.fade(x+2, y+1, string(url), m.th.text, a, false)
		f.fade(x+2+n, y+1, "▎", m.th.a1, a*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		if o.errT >= 0 && o.t-o.errT < 1.6 && len(o.url) == 0 {
			need := tr("нужен адрес", "address needed")
			f.fade(x+w-len([]rune(need)), y+1, need, m.th.bad, a, false)
		}
		dx := key(x, y+2, "enter", tr("дальше", "continue"))
		key(x+dx, y+2, "esc", tr("назад", "back"))
		return 4
	case SubKey:
		title, right := tr("ключ ", "key ")+p.name, p.keys
		if p.keys == "" {
			title, right = tr("ключ, если нужен", "key, if needed"), ""
		}
		f.fade(x, y, title, m.th.dim, a, false)
		if len([]rune(title))+len([]rune(right))+2 <= w {
			f.fade(x+w-len([]rune(right)), y, right, m.th.dim, a*0.7, false)
		}
		f.fade(x, y+1, "›", m.th.a1, a, true)
		masked := []rune(string(o.key[:min(6, len(o.key))]) + strings.Repeat("•", max(0, len(o.key)-6)))
		if n := max(0, w-6); len(masked) > n {
			masked = masked[len(masked)-n:]
		}
		n := f.fade(x+2, y+1, string(masked), m.th.text, a, false)
		f.fade(x+2+n, y+1, "▎", m.th.a1, a*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		switch {
		case o.errMsg != "":
			f.fade(x+w-len([]rune(trunc(o.errMsg, w))), y+1, trunc(o.errMsg, w), m.th.bad, a, false)
		case o.errT >= 0 && o.t-o.errT < 1.6 && len(o.key) == 0:
			need := tr("нужен ключ", "key needed")
			f.fade(x+w-len([]rune(need)), y+1, need, m.th.bad, a, false)
		}
		dx := key(x, y+2, "enter", tr("проверить", "verify"))
		key(x+dx, y+2, "esc", tr("назад", "back"))
		return 4
	case SubLoading:
		m.wave(f, x, y, tr("спрашиваю список моделей", "fetching models"), m.th.a1)
		return 2
	case SubModel:
		list := o.models()
		f.fade(x, y, tr("модель", "model"), m.th.dim, a, false)
		cnt := fmt.Sprintf(tr("%d с инструментами из %d", "%d with tools of %d"), len(o.list), o.total)
		f.fade(x+w-len([]rune(cnt)), y, cnt, m.th.dim, a*0.7, false)
		f.fade(x, y+1, "/", m.th.a1, a, true)
		f.fade(x+1, y+1, string(o.filter), m.th.text, a, false)
		f.fade(x+1+len(o.filter), y+1, "▎", m.th.a1, a*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		if len(o.filter) == 0 {
			f.fade(x+2+len(o.filter), y+1, tr("поиск", "search"), m.th.dim, a*0.5, false)
		}
		rows := max(2, min(6, m.h-y-7))
		start := max(0, min(o.model-rows/2, len(list)-rows))
		if len(list) == 0 {
			f.fade(x, y+2, tr("ничего не нашлось", "nothing found"), m.th.dim, a, false)
		}
		for k := 0; k < rows && start+k < len(list); k++ {
			idx, mi := start+k, list[start+k]
			id, idc := mi.ID, m.th.dim
			if idx == o.model {
				f.fade(x, y+2+k, "›", m.th.a1, a, true)
				idc = m.th.text
			}
			if mi.Rec {
				mark := tr("рекомендуем", "recommended")
				id = trunc(mi.ID, w-2-len([]rune(mark))-2)
				f.fade(x+w-len([]rune(mark)), y+2+k, mark, m.th.a1, a*0.8, false)
			}
			f.fade(x+2, y+2+k, id, idc, a, false)
		}
		dx := key(x, y+2+rows, "enter", tr("выбрать", "choose"))
		key(x+dx, y+2+rows, "esc", tr("назад", "back"))
		return rows + 4
	case SubEffort:
		short := o.picked
		if i := strings.LastIndex(short, "/"); i >= 0 {
			short = short[i+1:]
		}
		f.fade(x, y, tr("рассуждение", "reasoning"), m.th.dim, a, false)
		f.fade(x+w-len([]rune(short)), y, short, m.th.dim, a*0.7, false)
		ux, sx := x, x
		for i, e := range efforts {
			if i == o.effort {
				sx = ux
				f.fade(ux, y+1, e, m.th.a1, a, true)
			} else {
				f.fade(ux, y+1, e, m.th.dim, a, false)
			}
			ux += len([]rune(e)) + 3
		}
		f.fade(sx, y+2, strings.Repeat("▔", len([]rune(efforts[o.effort]))), m.th.a1, a*0.8, false)
		ex := [...]string{
			tr("быстрее, подборки проще", "faster, simpler picks"),
			tr("баланс скорости и точности", "balanced"),
			tr("точнее, но дольше", "sharper, but slower"),
		}[o.effort]
		f.fade(x, y+3, ex, m.th.dim, a, false)
		dx := key(x, y+4, "←→", tr("выбрать", "choose"))
		key(x+dx, y+4, "enter", tr("готово", "done"))
		return 6
	}
	return 0
}

func (m *Model) agentStatus() string {
	o := m.ob
	p := providers[o.prov]
	if o.picked == "" {
		return p.name
	}
	short := o.picked
	if i := strings.LastIndex(short, "/"); i >= 0 {
		short = short[i+1:]
	}
	return p.name + " · " + short
}
