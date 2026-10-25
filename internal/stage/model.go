// Package stage draws the AUX screen: onboarding, the agent at work and the player, as a scene with motion.
package stage

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"unicode"
)

// Track is one song on the screen; Key is its identity for covers, themes and the queue.
type Track struct {
	ID, Artist, Title, Why string
	Album                  string
	Released               string  // release date as Apple gives it: "2020-12-25"
	Dur                    float64 // seconds
}

// Key: ID when set, otherwise Artist + " " + Title. Covers, themes and queue slots are keyed by it.
func (t Track) Key() string {
	if t.ID != "" {
		return t.ID
	}
	return t.Artist + " " + t.Title
}

// Question is a warm-up card or a mid-stream toast question.
type Question struct {
	Title       string
	Options     []string // choice question
	Scale       bool
	Left, Right string // scale ends
	Def         int    // default option index, or default scale step
	Steps       int    // scale length, 0 = 5
	Chip        string // short label of the answer chip
}

// scaleSteps is the length of the scale: 5 when Steps is unset or unusable.
func (q Question) scaleSteps() int {
	if q.Steps < 2 {
		return 5
	}
	return q.Steps
}

// Chip is one plan word on the top line: Token is the query word it grew from, Label what is drawn.
type Chip struct{ Token, Label string }

// Action is what the screen sends to the host through Options.OnAction.
type Action interface{}

// Query is the line typed on the intro input, sent on enter.
type Query struct{ Text string }

// Answers carries one entry per warm-up card: the option or scale step picked, 1..N;
// an empty string means the default answer.
type Answers struct{ Raw []string }

// Say is a line typed after "/" in the player.
type Say struct{ Text string }

// LaterPick is the 1-based option picked in the mid-stream question.
type LaterPick struct{ N int }

// Rate is the + / - gesture; - sends it after the 0.35s gesture window.
type Rate struct{ Up bool }

// Save is the w key: the host writes the queue as a playlist and answers via Toast.
type Save struct{}

// ConnectApple is enter on the Apple Music step: the host signs the user in in the browser.
type ConnectApple struct{}

// ConnectLastfm is enter on the Last.fm step; Key and Secret are set when the step asked for them.
type ConnectLastfm struct{ Key, Secret string }

// LoadModels is the agent key confirmed: the host fetches the provider's models.
type LoadModels struct{ URL, Key string }

// Setup is what the onboarding settled, sent once when the needle drops.
type Setup struct {
	Lang   string // "ru" or "en"
	URL    string // agent API base URL with scheme
	Key    string
	Model  string
	Effort string // "low", "medium", "high"; "" when the model has no effort step
	Lastfm bool   // Last.fm connected (not skipped)
}

// Quit is esc / q where the screen used to quit the program.
type Quit struct{}

// Restart is the r key.
type Restart struct{}

// Settings is tab on the intro: the host reopens the onboarding with Onboard.
type Settings struct{}

// Eggs is the easter-eggs switch in the settings: On false turns every egg off.
type Eggs struct{ On bool }

// Phase is what the screen currently draws.
type Phase int

const (
	PhIntro   Phase = iota // the AUX logo and the clear input
	PhLaunch               // the input scatters into chips, the agent orb appears
	PhAsk                  // warm-up cards
	PhScan                 // the candidate stream and the slots
	PhDone                 // the portion is ready, on the way to the player
	PhPlay                 // the player
	PhOnboard              // the first-run onboarding
)

const (
	phIntro   = PhIntro
	phLaunch  = PhLaunch
	phAsk     = PhAsk
	phScan    = PhScan
	phDone    = PhDone
	phPlay    = PhPlay
	phOnboard = PhOnboard
)

// Options wires the screen to its host.
type Options struct {
	Onboard       bool                      // start with the first-run onboarding
	SelfPlay      bool                      // no player behind the screen: the stage itself skips, seeks and moves on at track end
	Hook          func(dt float64)          // called at the start of every Step, where drive() used to be called
	OnAction      func(Action)              // called synchronously, inside Key or Step
	Example       func() string             // hint under the input; enter on an empty input sends it as the query
	Chips         func(query string) []Chip // plan chips for a query; nil result → words of the query
	Connected     [3]bool                   // steps already connected: 0 Apple Music, 1 agent, 2 Last.fm
	LastfmAskKeys bool                      // the Last.fm app key or secret is missing: the Last.fm step asks for both
	LastfmKey     string                    // the Last.fm API key already known, prefilled in that field
	NoEggs        bool                      // no easter eggs
}

type chip struct {
	label string
	born  float64
}

type cand struct {
	tr      Track
	ok      bool
	reason  string
	born    float64
	decided float64
}

type slot struct {
	tr     Track
	fillAt float64
}

type toast struct {
	text   string
	q      *Question
	born   float64
	x      spring
	picked int
	pickT  float64
	ttl    float64
}

type Model struct {
	w, h   int
	t, pt  float64
	dt     float64
	ph     Phase
	th, to theme
	dim    spring
	bloom  float64

	input []rune
	query string
	err   string
	chips []chip
	parts []*particle
	logo  []*particle
	ob    *onboard
	o     Options

	qi            int
	qT            float64
	sel, knob     spring
	selIdx, knobI int
	answered      bool
	ansT          float64
	warmups       []Question
	raw           []string

	cands     []*cand
	inbox     []*cand
	lastCand  float64
	status    string
	thinking  bool
	launchOut float64
	slots     []*slot
	filled    int
	checked   int
	rejected  int
	doneAt    float64
	portionNo int

	note    string
	portion []Track
	pDone   bool

	snap   *fb
	transT float64

	queue    []Track
	cur      int
	prev     int
	trackT   float64
	pos      float64
	playing  bool
	pause    float64 // 0 playing … 1 paused: the play screen fades to black
	idleT    float64 // seconds the player has not been playing; a gap between tracks stays lit
	nowKey   string
	nowPos   float64
	skipT    float64
	eggDone  bool
	likeT    float64
	queueT   float64
	toasts   []*toast
	typing   bool
	reply    []rune
	lastShot *fb
	help     bool

	covers map[string]image.Image
	pix    map[string]*pixgrid
	themes map[string]theme

	eggMemo map[string]*egg // eggOf runs the egg rules once per track key

	titleBox image.Rectangle // where the title sits on the player, in pixels; set by every play frame
	barBox   image.Rectangle // where the progress bar sits, in cells; set by every play frame

	dstep int
}

// New builds the screen; call SetCover for each cover before the first frame.
func New(o Options) *Model {
	m := &Model{w: 160, h: 45, th: night, to: night, o: o,
		covers: map[string]image.Image{}, pix: map[string]*pixgrid{}, themes: map[string]theme{},
		eggMemo: map[string]*egg{}}
	m.dim = spring{x: 1, to: 1}
	m.reset()
	if o.Onboard {
		m.ph, m.ob = PhOnboard, newOnboard()
		m.ob.lfKey = []rune(m.o.LastfmKey)
	}
	return m
}

// Onboard reopens the onboarding over the intro; connected marks the steps it closes by itself.
func (m *Model) Onboard(connected [3]bool) {
	m.reset()
	m.o.Connected = connected
	m.ph, m.ob = PhOnboard, newOnboard()
	m.ob.lfKey = []rune(m.o.LastfmKey)
	m.ob.eggsAsk, m.ob.eggsOn = true, !m.o.NoEggs
}

func (m *Model) reset() {
	*m = Model{w: m.w, h: m.h, th: night, to: night, o: m.o, covers: m.covers,
		pix: m.pix, themes: m.themes, eggMemo: m.eggMemo, dim: spring{x: 1, to: 1}, portionNo: 1}
	m.slots = make([]*slot, 20)
	for i := range m.slots {
		m.slots[i] = &slot{fillAt: -1}
	}
	m.buildLogo()
}

func (m *Model) setPhase(p Phase) { m.ph, m.pt, m.dstep = p, 0, 0 }

func (m *Model) send(a Action) {
	if m.o.OnAction != nil {
		m.o.OnAction(a)
	}
}

// SetSize is the window resize: new size and the logo rebuilt for it; a same-size
// call does nothing, a host may call it on every frame.
func (m *Model) SetSize(w, h int) {
	if w == m.w && h == m.h {
		return
	}
	m.w, m.h = w, h
	m.buildLogo()
}

// Step moves the scene one tick; the host's Hook runs where drive() used to run.
func (m *Model) Step(dt float64) {
	m.dt = dt
	m.t += dt
	m.pt += dt
	if m.o.Hook != nil {
		m.o.Hook(dt)
	}
	m.th = m.th.lerp(m.to, 1-math.Exp(-dt*2.2))
	m.dim.step(dt)
	m.bloom *= math.Exp(-dt * 4)
	m.sel.step(dt)
	m.knob.step(dt)
	for _, t := range m.toasts {
		t.x.step(dt)
	}
	alive := m.parts[:0]
	for _, p := range m.parts {
		if p.step(dt) {
			alive = append(alive, p)
		}
	}
	m.parts = alive
	for _, p := range m.logo {
		if !p.home {
			p.step(dt)
		}
	}
	if m.snap != nil {
		m.transT += dt
		if m.transT > 1.1 {
			m.snap = nil
		}
	}

	switch m.ph {
	case PhOnboard:
		m.onboardStep(dt)
	case PhLaunch:
		if m.launchOut < 0 && m.pt > 1.85 && (len(m.warmups) > 0 || len(m.inbox) > 0 || m.pDone) {
			m.launchOut = m.pt
		}
		if m.launchOut >= 0 && m.pt > m.launchOut+0.25 {
			if len(m.warmups) > 0 {
				m.startAsk(0)
			} else {
				m.dim.to = 1
				m.setPhase(PhScan)
				m.thinking = true
			}
		}
	case PhAsk:
		m.qT += dt
		if m.answered {
			m.ansT += dt
			if m.ansT > 0.8 {
				if m.qi+1 < len(m.warmups) {
					m.startAsk(m.qi + 1)
				} else {
					m.send(Answers{Raw: m.raw})
					m.dim.to = 1
					m.setPhase(PhScan)
					m.thinking = true
				}
			}
		}
	case PhScan:
		m.scanStep(dt)
	case PhDone:
		if m.pt > 3.2 {
			m.toPlay()
		}
	case PhPlay:
		m.playStep(dt)
	}
}

// ---------- host side ----------

// SetCover stocks a cover for the track key and invalidates its thumbnails and theme;
// for the track that is playing the theme is derived again at once.
func (m *Model) SetCover(key string, img image.Image) {
	m.covers[key] = img
	for k := range m.pix {
		if k == key || strings.HasPrefix(k, key+"|") {
			delete(m.pix, k)
		}
	}
	delete(m.themes, key)
	if m.ph == PhPlay && len(m.queue) > 0 && key == m.curTrack().Key() {
		m.to = m.themeFor(*m.curTrack())
	}
}

// Ask hands over the warm-up cards; the flow asks them when it reaches that point,
// and straight away if the stream is already running. In the player the defaults
// answer for the agent, and the host learns about it from the empty Answers.
func (m *Model) Ask(qs []Question) {
	if len(qs) == 0 {
		return
	}
	if m.ph == PhPlay {
		m.send(Answers{Raw: make([]string, len(qs))})
		m.say(tr("агент спросил — оставил по умолчанию", "the agent asked — kept the defaults"))
		return
	}
	m.warmups = qs
	m.raw = nil
	if m.ph == PhScan {
		m.startAsk(0)
	}
}

// Candidate adds one stream row; a rejected one only visits with its reason. In the
// player phase the rows of the coming portions are ignored — they land via
// PortionDone. Rows arriving faster than the stream paces out wait in the inbox.
func (m *Model) Candidate(t Track, ok bool, reason string) {
	if m.ph == PhPlay {
		return
	}
	if m.ph == PhScan && len(m.inbox) == 0 && m.t-m.lastCand >= candGap {
		m.lastCand = m.t
		m.thinking = false
		m.cands = append(m.cands, &cand{tr: t, ok: ok, reason: reason, born: m.t, decided: -1})
		return
	}
	m.inbox = append(m.inbox, &cand{tr: t, ok: ok, reason: reason, decided: -1})
}

// SetThinking shows or hides the "agent is looking" line.
func (m *Model) SetThinking(on bool) { m.thinking = on }

// Progress shows what the agent is busy with: the line replaces the standing hint
// while the screen waits.
func (m *Model) Progress(line string) {
	m.status = strings.TrimSpace(line)
	m.thinking = true
}

// PortionDone ends the portion: note is spoken when it lands, tracks becomes the player
// queue. In the player it only tops the queue up with the tracks it doesn't have yet.
func (m *Model) PortionDone(tracks []Track, note string) {
	if m.ph == PhIntro {
		return
	}
	if m.ph == PhPlay {
		if n := m.queueAdditions(tracks); n > 0 {
			m.queueT = m.t
			m.say(fmt.Sprintf(tr("+%d в очереди", "+%d queued"), n))
		}
		return
	}
	m.note = note
	m.portion = append([]Track(nil), tracks...)
	m.pDone = true
}

// queueAdditions appends the tracks the queue doesn't have yet, by key, and reports
// how many landed.
func (m *Model) queueAdditions(tracks []Track) int {
	n := 0
	for _, tr := range tracks {
		seen := false
		for _, q := range m.queue {
			if q.Key() == tr.Key() {
				seen = true
				break
			}
		}
		if !seen {
			m.queue = append(m.queue, tr)
			n++
		}
	}
	return n
}

// AskLater pops a toast question; a newer question replaces an unanswered one;
// an unanswered question leaves after 45 s.
func (m *Model) AskLater(q Question) {
	if t := m.laterToast(); t != nil {
		if t.picked < 0 {
			t.x.to = 80
		} else {
			return
		}
	}
	m.toasts = append(m.toasts, &toast{q: &q, born: m.t, x: spring{x: 70}, picked: -1, ttl: 45})
}

// Ahead lists the keys of the tracks queued after the current one.
func (m *Model) Ahead() []string {
	var out []string
	for i := m.cur + 1; i < len(m.queue); i++ {
		out = append(out, m.queue[i].Key())
	}
	return out
}

// SetAhead puts those tracks after the current one, in this order, and starts the queue ripple.
func (m *Model) SetAhead(keys []string) {
	rest := append([]Track(nil), m.queue[m.cur+1:]...)
	used := make([]bool, len(rest))
	out := make([]Track, 0, len(rest))
	for _, k := range keys {
		for j, tr := range rest {
			if !used[j] && tr.Key() == k {
				used[j] = true
				out = append(out, tr)
				break
			}
		}
	}
	for j, tr := range rest {
		if !used[j] {
			out = append(out, tr)
		}
	}
	copy(m.queue[m.cur+1:], out)
	m.queueT = m.t
}

// JumpPlay opens the player on track i seeked to pos, without the scan in between.
func (m *Model) JumpPlay(tracks []Track, i int, pos float64) {
	m.ob = nil
	n := 0
	for n < len(tracks) && n < len(m.slots) {
		m.slots[n].tr, m.slots[n].fillAt = tracks[n], 0
		n++
	}
	m.toPlay()
	m.snap = nil
	m.startTrack(i)
	m.prev = m.cur
	m.th = m.to
	m.pos = pos
}

// Phase is what the screen draws right now.
func (m *Model) Phase() Phase { return m.ph }

// PhaseTime is the time since the phase started.
func (m *Model) PhaseTime() float64 { return m.pt }

// Input is the current line typed.
func (m *Model) Input() string { return string(m.input) }

// Asking is the warm-up card on screen and whether it has been answered.
func (m *Model) Asking() (Question, bool) {
	if m.qi < len(m.warmups) {
		return m.warmups[m.qi], m.answered
	}
	return Question{}, false
}

// At runs f once when the demo walk reaches second n of the phase.
func (m *Model) At(n int, t float64, f func()) {
	if m.dstep == n && m.pt >= t {
		m.dstep++
		f()
	}
}

// View renders the frame with its ANSI colours.
func (m *Model) View() string { return m.render().String() }

// PNG renders the frame and writes it as a file.
func (m *Model) PNG(path string) error { return m.render().png(path) }

// ---------- intro & launch ----------

func (m *Model) buildLogo() {
	if m.ph != PhIntro && m.ph != PhOnboard {
		return
	}
	word := "AUX"
	sc := 2
	if m.h < 26 || m.w < 60 {
		sc = 1
	}
	width := len(word)*5*sc + (len(word)-1)*3
	x0 := (m.w - width) / 2
	y0 := (m.h/2 - 4 - 5*sc/2) * 2
	m.logo = m.logo[:0]
	rng := rand.New(rand.NewSource(3))
	for i, r := range word {
		g := font5[r]
		for gy, row := range g {
			for gx, b := range row {
				if b != '#' {
					continue
				}
				for sy := 0; sy < sc; sy++ {
					for sx := 0; sx < sc; sx++ {
						tx := float64(x0 + i*(5*sc+3) + gx*sc + sx)
						ty := float64(y0 + gy*sc + sy)
						m.logo = append(m.logo, &particle{home: true, pixel: true, sx: tx, sy: ty + 2, tx: tx, ty: ty,
							delay: 0.1 + rng.Float64()*0.25 + (tx-float64(x0))/float64(width)*0.3, life: 0.55, x: tx, y: ty})
					}
				}
			}
		}
	}
}

func (m *Model) inputBox() (x, y, w int) {
	w = min(64, m.w-6)
	return (m.w - w) / 2, m.h/2 + 1, w
}

func (m *Model) typeRune(r rune) { m.input = append(m.input, r) }

func (m *Model) chipX(i int) int {
	x := 3
	for j := 0; j < i && j < len(m.chips); j++ {
		x += len([]rune(m.chips[j].label)) + 3
	}
	return x
}

// wordChips splits the query into words and keeps up to three long ones as plan chips.
func (m *Model) wordChips(query string) []Chip {
	var out []Chip
	for _, w := range strings.FieldsFunc(query, func(r rune) bool { return strings.ContainsRune(" ,.—-", r) }) {
		if len([]rune(w)) >= 4 && len(out) < 3 {
			out = append(out, Chip{w, w})
		}
	}
	return out
}

func (m *Model) launch() {
	m.query = strings.TrimSpace(string(m.input))
	if m.query == "" && m.o.Example != nil {
		if x := m.o.Example(); x != "" {
			m.query = x
			m.input = []rune(x)
		}
	}
	if m.query == "" {
		return
	}
	srcs := []Chip(nil)
	if m.o.Chips != nil {
		srcs = m.o.Chips(m.query)
	}
	if srcs == nil {
		srcs = m.wordChips(m.query)
	}
	m.chips = nil
	for _, s := range srcs {
		m.chips = append(m.chips, chip{label: s.Label, born: m.t + 1.1})
	}
	bx, by, _ := m.inputBox()
	qr := []rune(m.query)
	owner := make([]int, len(qr))
	offset := make([]int, len(qr))
	for i := range owner {
		owner[i] = -1
	}
	for ci, s := range srcs {
		if at := strings.Index(m.query, s.Token); at >= 0 {
			start := len([]rune(m.query[:at]))
			n := len([]rune(s.Token))
			for k := 0; k < n; k++ {
				owner[start+k] = ci
				offset[start+k] = k * len([]rune(s.Label)) / n
			}
		}
	}
	for i, r := range qr {
		x, y := float64(bx+3+i), float64(by+1)
		if r == ' ' {
			continue
		}
		if ci := owner[i]; ci >= 0 {
			lab := []rune(m.chips[ci].label)
			m.parts = append(m.parts, &particle{home: true, x: x, y: y, sx: x, sy: y,
				tx: float64(m.chipX(ci) + 1 + offset[i]), ty: 1, delay: 0.1 + float64(i)*0.01, life: 0.8,
				arc: 0.8, ch: r, tch: lab[min(offset[i], len(lab)-1)], c: m.th.a2})
			continue
		}
		m.parts = append(m.parts, &particle{x: x, y: y, ch: r, c: m.th.text, life: 0.35})
	}
	cx, cy := float64(m.w)/2, float64(m.h)
	for _, p := range m.logo {
		p.home = false
		p.x, p.y = p.tx, p.ty
		p.vx, p.vy = (cx-p.tx)*2.7, (cy-p.ty)*2.7
		p.life, p.drag, p.grav, p.age = 0.75, 0.9, 0, 0
	}
	m.bloom = 0.35
	m.launchOut = -1
	m.setPhase(PhLaunch)
	m.send(Query{Text: m.query})
}

// ---------- warm-up cards ----------

func (m *Model) startAsk(i int) {
	m.setPhase(PhAsk)
	m.qi, m.qT, m.answered, m.ansT = i, 0, false, 0
	m.dim.to = 0.45
	q := m.warmups[i]
	if !q.Scale && len(q.Options) > 0 {
		d := min(max(q.Def, 0), len(q.Options)-1)
		m.selIdx = d
		m.sel = spring{x: float64(d), to: float64(d)}
	} else {
		m.selIdx = 0
		m.sel = spring{}
	}
	m.knobI = q.Def
	m.knob = spring{x: float64(q.Def), to: float64(q.Def)}
}

func (m *Model) card() (x, y, w, h int) {
	q := m.warmups[m.qi]
	w = min(64, m.w-6)
	h = 8 + len(q.Options)
	if q.Scale {
		h = 9
	}
	return (m.w - w) / 2, (m.h - h) / 2, w, h
}

func (m *Model) answer() {
	if m.answered || m.qT < 0.7 {
		return
	}
	q := m.warmups[m.qi]
	m.answered, m.ansT = true, 0
	var label string
	if q.Scale {
		m.raw = append(m.raw, strconv.Itoa(m.knobI+1))
		label = strings.Repeat("▮", m.knobI+1) + strings.Repeat("▯", q.scaleSteps()-1-m.knobI)
	} else {
		m.raw = append(m.raw, strconv.Itoa(m.selIdx+1))
		label = q.Options[m.selIdx]
	}
	if q.Chip != "" && q.Scale {
		label = q.Chip + " " + label
	} else if q.Chip != "" {
		label = q.Chip + ": " + label
	}
	m.chips = append(m.chips, chip{label: label, born: m.t + 0.3})
	m.bloom = 0.25
}

func (m *Model) track(x, y, w int) (tx, ty, tw int) { return x + 5, y + 5, w - 10 }

// ---------- scan ----------

// streamGeom: the candidate stream gets its own column only on wide screens.
func (m *Model) streamGeom() (x, y0, y1, w int, show bool) {
	if m.w < 120 {
		return 3, 6, 7, m.w - 6, false
	}
	return 3, 7, m.h - 2, max(30, min(46, m.w/3)), true
}

// gridGeom picks full cards when 20 of them fit, thumbnails otherwise.
func (m *Model) gridGeom() (x, y, cols, cw int, cards bool) {
	sx, _, _, sw, show := m.streamGeom()
	x, y = 3, 8
	if show {
		x, y = sx+sw+4, 7
	}
	avail := m.w - x - 2
	cols = max(1, min(5, (avail+2)/24))
	if rows := (20 + cols - 1) / cols; rows*3-1 <= m.h-y-1 {
		return x, y, cols, 22, true
	}
	cols = max(1, min(10, (avail+1)/5))
	return x, y, cols, 4, false
}

func (m *Model) slotPos(i int) (x, y int) {
	gx, gy, cols, cw, _ := m.gridGeom()
	return gx + (i%cols)*(cw+2), gy + (i/cols)*3
}

func (m *Model) visible() (first int) {
	_, y0, y1, _, _ := m.streamGeom()
	return max(0, len(m.cands)-(y1-y0))
}

// candGap is the stream tempo: one candidate lands at most once in candGap seconds.
const candGap = 0.12

func (m *Model) scanStep(dt float64) {
	if len(m.inbox) > 0 && m.t-m.lastCand >= candGap {
		c := m.inbox[0]
		m.inbox = m.inbox[1:]
		m.lastCand = min(m.lastCand+candGap, m.t)
		c.born, m.thinking = m.t, false
		m.cands = append(m.cands, c)
	}
	for _, c := range m.cands {
		if c.decided >= 0 || m.t-c.born < 0.5 {
			continue
		}
		c.decided = m.t
		m.checked++
		if !c.ok {
			m.rejected++
			continue
		}
		if m.filled < len(m.slots) {
			m.slots[m.filled].tr = c.tr
			m.slots[m.filled].fillAt = m.t
			m.filled++
		}
	}
	if m.pDone {
		solved := len(m.inbox) == 0
		for _, c := range m.cands {
			if c.decided < 0 {
				solved = false
				break
			}
		}
		if solved && m.filled == 0 {
			m.Fail(tr("ничего не нашлось — попробуй сказать иначе", "found nothing — try saying it another way"))
			return
		}
		if solved && m.filled > 0 && m.filled < len(m.slots) {
			m.slots = m.slots[:m.filled]
		}
		if m.filled == len(m.slots) {
			landed := true
			for _, s := range m.slots {
				if s.fillAt < 0 {
					landed = false
				}
			}
			if landed {
				m.setPhase(PhDone)
				m.doneAt = m.t
			}
		}
	}
}

// ---------- play ----------

func (m *Model) toPlay() {
	m.snap = m.lastShot
	m.transT = 0
	if m.portion != nil {
		m.queue = m.portion
	} else {
		for _, s := range m.slots {
			if s.fillAt >= 0 {
				m.queue = append(m.queue, s.tr)
			}
		}
	}
	start, found := 0, false
	for i, tk := range m.queue {
		if tk.Key() == m.nowKey {
			start, found = i, true
			break
		}
	}
	m.setPhase(PhPlay)
	m.startTrack(start)
	if found {
		m.pos = m.nowPos
	}
}

func (m *Model) curTrack() *Track { return &m.queue[m.cur] }

// themeFor derives the track's palette once and caches it; SetCover drops the cache.
func (m *Model) themeFor(t Track) theme {
	if th, ok := m.themes[t.Key()]; ok {
		return th
	}
	th := themeOf(m.cover(t.Key(), 24, 24))
	if e := m.eggOf(t); e != nil && e.theme != nil {
		th = *e.theme
	}
	m.themes[t.Key()] = th
	return th
}

func (m *Model) startTrack(i int) {
	m.prev = m.cur
	m.cur = i % len(m.queue)
	m.trackT, m.pos, m.eggDone = 0, 0, false
	m.to = m.themeFor(*m.curTrack())
}

func (m *Model) playStep(dt float64) {
	m.trackT += dt
	if m.o.SelfPlay || m.playing {
		m.pos += dt
	}
	m.idleT += dt
	if m.o.SelfPlay || m.playing {
		m.idleT = 0
	}
	want := 0.0
	if m.idleT > 0.4 {
		want = 1
	}
	m.pause += (want - m.pause) * (1 - math.Exp(-dt*5))
	if d := m.curTrack().Dur; d > 0 {
		m.pos = min(m.pos, d)
	}
	m.eggStep()
	if d := m.curTrack().Dur; d > 0 && m.pos >= d && m.o.SelfPlay {
		m.startTrack(m.cur + 1)
	}
	if m.skipT > 0 && m.t-m.skipT > 0.35 {
		m.skipT = 0
		m.send(Rate{Up: false})
		if m.o.SelfPlay {
			m.startTrack(m.cur + 1)
		}
	}
	kept := m.toasts[:0]
	for _, t := range m.toasts {
		age := m.t - t.born
		if t.ttl > 0 && age > t.ttl {
			t.x.to = 80
		}
		if t.picked >= 0 && t.pickT > 0 && m.t-t.pickT > 0.6 {
			t.x.to = 80
		}
		if !(t.x.to == 80 && t.x.x > 70) {
			kept = append(kept, t)
		}
	}
	m.toasts = kept
}

func (m *Model) say(s string) {
	m.toasts = append(m.toasts, &toast{text: s, born: m.t, x: spring{x: 70}, picked: -1, ttl: 2.6})
}

func (m *Model) like(up bool) {
	if up {
		m.likeT = m.t
		m.bloom = 0.25
		m.say(tr("запомню: больше такого", "noted: more like this"))
		return
	}
	m.skipT = m.t
	m.say(tr("ухожу от этого звучания", "moving away from this sound"))
}

// Now is the state of the real player: the stage follows its track, corrects its own
// position when the player reports drift and freezes while it is paused.
func (m *Model) Now(key string, pos float64, playing bool) {
	m.playing = playing
	if m.ph != PhPlay {
		m.nowKey, m.nowPos = key, pos
		return
	}
	if key == m.curTrack().Key() {
		if math.Abs(pos-m.pos) > 1.5 {
			m.pos = pos
		}
		return
	}
	for i, tk := range m.queue {
		if tk.Key() == key {
			m.startTrack(i)
			m.pos = pos
			return
		}
	}
}

// Fail reports a host-side failure: in the player it becomes a toast, anywhere else
// the stage returns to the intro with the error on screen and the query preserved.
func (m *Model) Fail(msg string) {
	if m.ph == PhPlay {
		m.say(msg)
		return
	}
	q := m.query
	m.reset()
	m.input = []rune(q)
	m.err = msg
}

// Toast shows a host line; only the player draws it.
func (m *Model) Toast(s string) {
	if m.ph == PhPlay {
		m.say(s)
	}
}

// ---------- input ----------

// Paste types pasted text into the field in focus as one line: the edges are trimmed,
// line breaks and tabs become spaces, other control characters are dropped.
func (m *Model) Paste(text string) {
	line := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, strings.TrimSpace(text))
	if line != "" {
		m.Key("", line)
	}
}

// Key handles one key press and reports whether the screen took it: a host with its
// own player passes the untaken keys through. Quitting became OnAction(Quit{}); esc
// never quits, it steps back or clears.
func (m *Model) Key(k, text string) bool {
	switch m.ph {
	case PhOnboard:
		m.onboardKey(k, text)
		return true
	case PhIntro:
		m.err = ""
		switch k {
		case "esc":
			m.input = nil
		case "enter":
			m.launch()
		case "tab":
			m.send(Settings{})
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			for _, r := range text {
				m.typeRune(r)
			}
		}
		return true
	case PhAsk:
		q := m.warmups[m.qi]
		switch k {
		case "up", "k":
			if !q.Scale && m.selIdx > 0 {
				m.selIdx--
			}
		case "down", "j":
			if !q.Scale && m.selIdx < len(q.Options)-1 {
				m.selIdx++
			}
		case "left", "h":
			if q.Scale && m.knobI > 0 {
				m.knobI--
			}
		case "right", "l":
			if q.Scale && m.knobI < q.scaleSteps()-1 {
				m.knobI++
			}
		case "enter", "space":
			m.answer()
		default:
			if n, err := strconv.Atoi(k); err == nil && !q.Scale && n >= 1 && n <= len(q.Options) {
				m.selIdx = n - 1
				m.answer()
			}
		}
		m.sel.to = float64(m.selIdx)
		m.knob.to = float64(m.knobI)
		return true
	case PhPlay:
		return m.playKey(k, text)
	default:
		switch k {
		case "q":
			m.send(Quit{})
		case "r":
			m.reset()
			m.send(Restart{})
		case "+", "=", "up", "-", "down", "/", "?", "w":
			return true
		default:
			return false
		}
		return true
	}
}

// playKey is the player phase: the input after "/", toast answers and the rest of the
// keys; n/right/left/]/[ belong to the real player and pass through when it is there.
func (m *Model) playKey(k, text string) bool {
	if m.typing {
		switch k {
		case "esc":
			m.typing, m.reply = false, nil
		case "enter":
			if s := strings.TrimSpace(string(m.reply)); s != "" {
				m.say(tr("понял: «"+s+"», перестраиваю дальше", "got it: \""+s+"\", rebuilding the queue"))
				m.send(Say{Text: s})
			}
			m.typing, m.reply = false, nil
		case "backspace":
			if len(m.reply) > 0 {
				m.reply = m.reply[:len(m.reply)-1]
			}
		default:
			m.reply = append(m.reply, []rune(text)...)
		}
		return true
	}
	if t := m.laterToast(); t != nil {
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(t.q.Options) && t.picked < 0 {
			t.picked, t.pickT = n-1, m.t
			m.say(tr("▸ "+t.q.Options[n-1]+", перестраиваю дальше", "▸ "+t.q.Options[n-1]+", rebuilding the queue"))
			m.send(LaterPick{N: n})
			return true
		}
	}
	switch k {
	case "q":
		m.send(Quit{})
	case "esc":
		m.help = false
	case "r":
		m.reset()
		m.send(Restart{})
	case "n", "right":
		if m.o.SelfPlay {
			m.startTrack(m.cur + 1)
			return true
		}
		return false
	case "]":
		if m.o.SelfPlay {
			m.pos = min(m.pos+10, m.curTrack().Dur-1)
			return true
		}
		return false
	case "[":
		if m.o.SelfPlay {
			m.pos = max(0, m.pos-10)
			return true
		}
		return false
	case "+", "=":
		m.like(true)
		m.send(Rate{Up: true})
	case "-":
		m.like(false)
	case "/":
		m.typing = true
	case "?":
		m.help = !m.help
	case "w":
		m.send(Save{})
	default:
		return false
	}
	return true
}

func (m *Model) laterToast() *toast {
	for _, t := range m.toasts {
		if t.q != nil && t.x.to != 80 {
			return t
		}
	}
	return nil
}
