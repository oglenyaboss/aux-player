package main

import "github.com/simone-vibes/vibez/internal/stage"

// drive is the old autopilot of the demo, rewritten over the screen's public API:
// it walks intro, warm-up cards and the player by typed keys, minute by minute.

func (s *scene) drive() {
	m := s.m
	switch m.Phase() {
	case stage.PhOnboard:
		s.onboardWalk()
	case stage.PhIntro:
		if m.PhaseTime() > 2.2 {
			rs := []rune(demoQuery())
			want := min(len(rs), int((m.PhaseTime()-2.2)/0.05))
			for len([]rune(m.Input())) < want {
				m.Key("", string(rs[len([]rune(m.Input()))]))
			}
			if len([]rune(m.Input())) == len(rs) {
				m.At(0, 2.2+float64(len(rs))*0.05+0.7, func() { m.Key("enter", "") })
			}
		}
	case stage.PhAsk:
		q, answered := m.Asking()
		if answered {
			return
		}
		if q.Scale {
			m.At(0, 1.3, func() { m.Key("right", "") })
			m.At(1, 1.7, func() { m.Key("right", "") })
			m.At(2, 2.1, func() { m.Key("left", "") })
			m.At(3, 2.8, func() { m.Key("enter", "") })
		} else {
			m.At(0, 1.3, func() { m.Key("down", "") })
			m.At(1, 1.7, func() { m.Key("down", "") })
			m.At(2, 2.1, func() { m.Key("up", "") })
			m.At(3, 2.5, func() { m.Key("up", "") })
			m.At(4, 3.1, func() { m.Key("enter", "") })
		}
	case stage.PhPlay:
		m.At(0, 5, func() { m.Key("+", "") })
		m.At(1, 8, func() { m.AskLater(laterQuestion()) })
		m.At(2, 10.5, func() { m.Key("2", "") })
		m.At(3, 14, func() { m.Key("n", "") })
		m.At(4, 19, func() { m.Key("-", "") })
		m.At(5, 25, func() { m.Key("n", "") })
	}
}

// onboardWalk types the same key presses the old DemoOnboard did, second by second.
func (s *scene) onboardWalk() {
	m := s.m
	m.At(0, 6.0, func() { m.Key("enter", "") })
	m.At(1, 6.4, func() { m.Key("enter", "") })
	m.At(2, 8.9, func() { m.Key("down", "") })
	m.At(3, 9.3, func() { m.Key("up", "") })
	m.At(4, 9.8, func() { m.Key("enter", "") })
	if step, sub, t, ok := m.Onboarding(); ok && step == 1 && sub == stage.SubKey && t > 10.2 {
		rs := []rune(demoKey)
		want := min(len(rs), int((t-10.2)/0.025))
		for s.keyTyped < want {
			m.Key("", string(rs[s.keyTyped]))
			s.keyTyped++
		}
	}
	m.At(5, 11.2, func() { m.Key("enter", "") })
	if step, sub, t, ok := m.Onboarding(); ok && step == 1 && sub == stage.SubModel && t > 13.0 {
		rs := []rune("flash")
		want := min(len(rs), int((t-13.0)/0.09))
		for s.filterTyped < want {
			m.Key("", string(rs[s.filterTyped]))
			s.filterTyped++
		}
	}
	m.At(6, 13.9, func() { m.Key("down", "") })
	m.At(7, 14.3, func() { m.Key("up", "") })
	m.At(8, 14.8, func() { m.Key("enter", "") })
	m.At(9, 15.3, func() { m.Key("left", "") })
	m.At(10, 15.7, func() { m.Key("right", "") })
	m.At(11, 16.2, func() { m.Key("enter", "") })
	m.At(12, 17.0, func() { m.Key("enter", "") })
}
