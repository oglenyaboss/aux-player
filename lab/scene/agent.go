package main

import (
	"math/rand"

	"github.com/simone-vibes/vibez/internal/stage"
)

// scene owns the screen and the fake agent pretending to be a picker.
type scene struct {
	m    *stage.Model
	quit bool

	pilot bool // old drive(): the demo walks its own flow, only with -demo

	// the fake agent: it streams candidates in batches after the wake-up; its timings carry
	// over unchanged from the old scanStep
	r        *rand.Rand
	clock    float64
	next     int
	inBatch  int
	batch    int
	emit     bool
	accepted []stage.Track

	keyTyped    int // how many key runes the onboarding autopilot typed in
	filterTyped int // how many filter runes the onboarding autopilot typed in
}

func newScene(firstRun, pilot bool) *scene {
	s := &scene{pilot: pilot, r: rand.New(rand.NewSource(7))}
	s.m = stage.New(stage.Options{
		Onboard:  firstRun,
		SelfPlay: true,
		Hook:     s.hook,
		OnAction: s.onAction,
		Example:  func() string { return demoQuery() },
		Chips:    demoChips,
	})
	return s
}

// hook runs where drive() used to run: the demo walk first, then the fake agent's ticker.
func (s *scene) hook(dt float64) {
	if s.pilot {
		s.drive()
	}
	s.stream(dt)
}

func (s *scene) onAction(a stage.Action) {
	switch a.(type) {
	case stage.Query:
		s.m.Ask(questions())
	case stage.Answers:
		s.next, s.batch, s.inBatch, s.accepted = 0, 0, 0, nil
		s.clock, s.emit = 0.9, true
	case stage.Say:
		s.rebuild()
	case stage.LaterPick:
		s.rebuild()
	case stage.ConnectApple:
		s.m.StepDone(0, "")
	case stage.ConnectLastfm:
		s.m.StepDone(2, "")
	case stage.LoadModels:
		s.m.Models(demoModels, demoModelTotal, "")
	case stage.Setup:
		// the demo backend has nothing to add once the needle drops
	case stage.Quit:
		s.quit = true
	case stage.Restart:
		s.next, s.batch, s.inBatch, s.clock = 0, 0, 0, 0
		s.emit, s.accepted = false, nil
		s.r = rand.New(rand.NewSource(7))
	}
}

// rebuild is the old reshuffle: the tracks still ahead of the current one get reordered.
func (s *scene) rebuild() {
	keys := s.m.Ahead()
	s.r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	s.m.SetAhead(keys)
}

// stream is the timer part of the old scanStep, moved out of the screen: every dt it
// may feed one candidate, keep the batch gaps and close the portion after the last one.
func (s *scene) stream(dt float64) {
	if !s.emit {
		return
	}
	s.clock -= dt
	cands := demoCandidates()
	if s.next < len(cands) && s.clock <= 0 {
		c := cands[s.next]
		t := stage.Track{Artist: c.artist, Title: c.title, Why: c.why, Dur: c.dur}
		s.m.Candidate(t, c.ok, c.reason)
		if c.ok {
			s.accepted = append(s.accepted, t)
		}
		s.next++
		s.inBatch++
		s.clock = 0.24
		if s.batch < len(batches()) && s.inBatch >= batches()[s.batch] {
			s.batch++
			s.inBatch = 0
			s.clock = 1.5
			s.m.SetThinking(true)
		}
		if s.next == len(cands) {
			s.m.PortionDone(s.accepted, portionNote())
		}
	}
}
