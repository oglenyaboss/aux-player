package main

import (
	"math/rand"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

// scene owns the screen and the fake agent pretending to be a picker.
type scene struct {
	m    *stage.Model
	quit bool

	pilot bool // old drive(): the demo walks its own flow, only with -demo

	// the fake agent paced like the real one: a long think with progress lines, then one
	// submit that lands the whole batch at once
	r     *rand.Rand
	clock float64
	said  int
	emit  bool
	wait  float64 // seconds the agent thinks before its submit

	keyTyped    int // how many key runes the onboarding autopilot typed in
	filterTyped int // how many filter runes the onboarding autopilot typed in
}

func newScene(firstRun, pilot bool) *scene {
	s := &scene{pilot: pilot, r: rand.New(rand.NewSource(7))} //nolint:gosec // G404: the demo narrates the same takes on every run
	s.m = stage.New(stage.Options{
		Onboard:  firstRun,
		SelfPlay: true,
		Hook:     s.hook,
		OnAction: s.onAction,
		Example:  demoQuery,
		Chips:    demoChips,
		Slots:    portionSize,
	})
	s.wait = 9
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
		s.clock, s.said, s.emit = 0, 0, true
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
		s.clock, s.said, s.emit = 0, 0, false
		s.r = rand.New(rand.NewSource(7)) //nolint:gosec // G404: the demo narrates the same takes on every run
	}
}

// rebuild is the old reshuffle: the tracks still ahead of the current one get reordered.
func (s *scene) rebuild() {
	keys := s.m.Ahead()
	s.r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	s.m.SetAhead(keys)
}

// stream plays the agent's turn: its steps spread over the wait, then the submit lands
// every candidate of the batch in one go and closes the portion.
func (s *scene) stream(dt float64) {
	if !s.emit {
		return
	}
	s.clock += dt
	steps := demoSteps()
	for s.said < len(steps) && s.clock >= s.wait*float64(s.said)/float64(len(steps)) {
		s.m.Progress(steps[s.said])
		s.said++
	}
	if s.clock < s.wait {
		return
	}
	s.emit = false
	var accepted []stage.Track
	for _, c := range demoCandidates() {
		if len(accepted) == portionSize {
			break
		}
		t := stage.Track{Artist: c.artist, Title: c.title, Why: c.why,
			Album: demoAlbums[c.artist+" "+c.title], Dur: c.dur}
		s.m.Candidate(t, c.ok, c.reason)
		if c.ok {
			accepted = append(accepted, t)
		}
	}
	s.m.PortionDone(accepted, portionNote())
}
