package main

import (
	"fmt"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

const (
	needleLand = 2.2
	prompt     = "~ ❯ "
	shellRow   = 1
	shellCol   = 2
)

var shellBg = [3]uint8{10, 13, 14}

// preroll is everything before the needle drop: the shell, the query, the agent's
// question and the scan, quantized to the beat of the first track. The scan runs at
// the rate that puts the needle on the record exactly at the drop.
type preroll struct {
	f     *film
	m     *stage.Model
	rate  float64
	agent *fakeAgent
	start int
	enter int
	asked int
	typed int
}

type fakeAgent struct {
	clock    float64
	thinking bool
	done     bool
}

func newPreroll(f *film) *preroll {
	p := &preroll{f: f, start: f.beatFrame(2), enter: f.beatFrame(6), asked: -1}
	if f.e.ScanRate > 0 {
		p.rate = f.e.ScanRate
		p.reset()
		return p
	}
	lo, hi := 1.0, 6.0
	target := f.drop - frameOf(needleLand)
	for range 12 {
		mid := (lo + hi) / 2
		got := p.dry(mid)
		fmt.Printf("scan rate %.4f lands at %d, wants %d\n", mid, got, target)
		if got > target {
			lo = mid
		} else {
			hi = mid
		}
	}
	p.rate = hi
	p.reset()
	return p
}

func (p *preroll) reset() {
	p.agent = &fakeAgent{}
	p.asked, p.typed = -1, 0
	ts := p.f.tracks
	var m *stage.Model
	m = stage.New(stage.Options{
		SelfPlay: true, Slots: 5,
		OnAction: func(a stage.Action) {
			switch a.(type) {
			case stage.Query:
				m.Ask([]stage.Question{{Title: "От чего оттолкнуться?",
					Options: []string{"SALUKI · HAHAHA", "Kanye West · Black Skinhead", "Travis Scott · FE!N"},
					Tracks:  []stage.Track{ts[0], ts[1], ts[3]}, Chip: "ориентир"}})
			case stage.Answers:
				p.agent.thinking = true
			}
		},
	})
	m.SetSize(cols, rows)
	stock(m, p.f.es)
	p.m = m
}

// dry runs the preroll with no pictures and tells the frame PhDone came.
func (p *preroll) dry(rate float64) int {
	p.reset()
	p.rate = rate
	log := p.f.log
	var c [rows * cols]cell
	for n := 0; n < p.f.drop+fps; n++ {
		p.frame(n, &c)
		if p.m.Phase() == stage.PhDone {
			p.f.log = log
			return n
		}
	}
	p.f.log = log
	return p.f.drop + fps
}

func (p *preroll) frame(n int, c *[rows * cols]cell) {
	if n < p.start {
		p.shell(n, c)
		return
	}
	rate := 1.0
	switch p.m.Phase() {
	case stage.PhIntro:
		if p.m.PhaseTime() < 2 {
			rate = 1.7
		}
		p.typeQuery(n)
	case stage.PhLaunch:
		rate = 1.5
	case stage.PhAsk:
		p.pick(n)
		if _, answered := p.m.Asking(); answered {
			rate = 2
		}
	case stage.PhScan:
		rate = p.rate
		p.think(rate / fps)
	}
	p.m.Step(rate / fps)
	parseANSI(p.m.View(), c)
}

func (p *preroll) typeQuery(n int) {
	rs := []rune(p.f.e.Query)
	from := p.start + frameOf(1.18)
	enter := p.f.beatFrame(7)
	for p.typed < len(rs) && n >= from+p.typed*2 {
		p.m.Key("", string(rs[p.typed]))
		p.typed++
	}
	if n == enter {
		p.m.Key("enter", "")
	}
}

func (p *preroll) pick(n int) {
	if p.asked < 0 {
		p.asked = n
	}
	switch n - p.asked {
	case p.f.beatFrame(1):
		p.m.Key("right", "")
	case p.f.beatFrame(2):
		p.m.Key("left", "")
	case p.f.beatFrame(3):
		p.m.Key("enter", "")
	}
}

func (p *preroll) think(dt float64) {
	a := p.agent
	if !a.thinking || a.done {
		return
	}
	a.clock += dt
	lines := []string{"читаю про SALUKI", "смотрю соседей Kanye West", "проверяю в Apple Music…"}
	p.m.Progress(lines[min(len(lines)-1, int(a.clock/0.4))])
	if a.clock < 1.2 {
		return
	}
	a.done = true
	portion := p.f.tracks[:5]
	for _, t := range portion {
		p.m.Candidate(t, true, "")
	}
	p.m.PortionDone(portion, "держу напор, дальше психоделика")
}

// land starts the player at the needle: the rest of the drop's wait is skipped.
func (p *preroll) land(s segment) {
	p.m.Now(p.f.tracks[0].Key(), s.Pos, true)
	p.m.Step(3.3 - p.m.PhaseTime())
	p.f.sound(s.Start, "needle")
}

func (p *preroll) shell(n int, c *[rows * cols]cell) {
	for i := range c {
		c[i] = cell{' ', shellBg, shellBg, false}
	}
	typed := 0
	for _, at := range []float64{0.5, 0.62, 0.74} {
		if n >= frameOf(at) {
			typed++
		}
	}
	line := prompt + "aux"[:typed]
	y, x := shellRow, shellCol
	for _, r := range line {
		fg := [3]uint8{226, 232, 232}
		if r == '❯' || r == '~' {
			fg = [3]uint8{125, 226, 200}
		}
		c[y*cols+x] = cell{r, fg, shellBg, true}
		x++
	}
	c[y*cols+x] = cell{'█', [3]uint8{226, 232, 232}, shellBg, false}
}
