package stage

import (
	"testing"
)

const eventDT = 1.0 / 30

type recorder struct{ acts []Action }

func (r *recorder) on(a Action) { r.acts = append(r.acts, a) }

func recorded[T Action](r *recorder) []T {
	var out []T
	for _, a := range r.acts {
		if v, ok := a.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// steps runs the given seconds of wall clock at the 30 fps tick.
func steps(m *Model, seconds float64) {
	for i := 0; i < int(seconds/eventDT); i++ {
		m.Step(eventDT)
	}
}

func threeTracks() []Track {
	return []Track{
		{Artist: "Alpha", Title: "One", Dur: 120},
		{Artist: "Beta", Title: "Two", Dur: 120},
		{Artist: "Gamma", Title: "Three", Dur: 120},
	}
}

// R1/R4: typing on the intro and enter send Query with the entered line.
func TestQueryAction(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	qs := recorded[Query](r)
	if len(qs) != 1 || qs[0].Text != "abc" {
		t.Fatalf("ждём один Query{Text:\"abc\"}, записано %v", r.acts)
	}
}

// D3: with nothing but a query, enter reaches PhScan on its own (thinking, no cards asked).
func TestNoQuestionsGoesToScan(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	steps(m, 2.5)
	if p := m.Phase(); p != PhScan {
		t.Fatalf("фаза %v, ждём PhScan", p)
	}
}

// R4: each warm-up card lands one entry in Raw, 1-based, and the flow closes with Answers.
func TestAnswersRaw(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	choice := Question{Title: "q1", Options: []string{"a", "b", "c"}, Chip: "vibe"}
	scale := Question{Title: "q2", Scale: true, Left: "l", Right: "r", Def: 2, Chip: "energy"}
	m.Ask([]Question{choice, scale})
	for i := 0; i < 120 && m.Phase() != PhAsk; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhAsk {
		t.Fatalf("фаза %v, ждём PhAsk", m.Phase())
	}
	steps(m, 0.8)
	m.Key("down", "")
	m.Key("enter", "")
	for i := 0; i < 60; i++ {
		if _, answered := m.Asking(); !answered {
			break
		}
		m.Step(eventDT)
	}
	steps(m, 0.8)
	m.Key("right", "")
	m.Key("enter", "")
	for i := 0; i < 60 && m.Phase() == PhAsk; i++ {
		m.Step(eventDT)
	}
	ans := recorded[Answers](r)
	if len(ans) != 1 || !(len(ans[0].Raw) == 2 && ans[0].Raw[0] == "2" && ans[0].Raw[1] == "4") {
		t.Fatalf("ждём Answers{Raw:[2 4]}, записано %v", r.acts)
	}
	if p := m.Phase(); p != PhScan {
		t.Fatalf("фаза %v, ждём PhScan", p)
	}
}

// D3: PortionDone closes the portion: the grid shrinks and the flow passes through PhDone.
func TestPortionShrinks(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	for i := 0; i < 120 && m.Phase() != PhScan; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhScan {
		t.Fatalf("фаза %v, ждём PhScan", m.Phase())
	}
	tracks := threeTracks()
	m.Candidate(tracks[0], true, "")
	m.Candidate(tracks[1], true, "")
	m.Candidate(tracks[2], true, "")
	m.Candidate(Track{Artist: "Delta", Title: "Four"}, false, "no")
	m.PortionDone(tracks, "n")
	sawDone := false
	for i := 0; i < int(5/eventDT); i++ {
		m.Step(eventDT)
		if m.Phase() == PhDone {
			sawDone = true
		}
	}
	if !sawDone || (m.Phase() != PhDone && m.Phase() != PhPlay) {
		t.Fatalf("sawDone=%v, фаза %v; ждём прохождение PhDone и финиш в PhDone или PhPlay", sawDone, m.Phase())
	}
}

// R1: with SelfPlay false n passes through to the host: the key is not taken, nothing
// is sent, and the queue stays put.
func TestNoSelfPlayPassesThrough(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.Step(eventDT)
	if m.Key("n", "") {
		t.Errorf("n без SelfPlay отдан хосту, а экран его взял")
	}
	if len(r.acts) != 0 {
		t.Fatalf("ждём пустой набор действий, записано %v", r.acts)
	}
	want := []string{tracks[1].Key(), tracks[2].Key()}
	if got := m.Ahead(); !eqStrings(got, want) {
		t.Fatalf("Ahead()=%v, ждём %v (трек не должен смениться)", got, want)
	}
}

// R5: with SelfPlay false the down arrow only sends Rate after the 0.35s gesture window.
func TestRateDownAfterGesture(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.Step(eventDT)
	m.Key("-", "")
	steps(m, 0.2)
	if rs := recorded[Rate](r); len(rs) != 0 {
		t.Fatalf("через 0.2с уже есть Rate: %v", r.acts)
	}
	steps(m, 0.3)
	rs := recorded[Rate](r)
	if len(rs) != 1 || rs[0].Up {
		t.Fatalf("через 0.5с ждём Rate{Up:false}, записано %v", r.acts)
	}
	want := []string{tracks[1].Key(), tracks[2].Key()}
	if got := m.Ahead(); !eqStrings(got, want) {
		t.Fatalf("Ahead()=%v, ждём %v (трек не должен смениться)", got, want)
	}
}

// D1: with SelfPlay true n moves to the next track and stays on the screen.
func TestSelfPlaySkip(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on, SelfPlay: true})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.Key("n", "")
	if len(r.acts) != 0 {
		t.Fatalf("при SelfPlay действий не ждём: %v", r.acts)
	}
	want := []string{tracks[2].Key()}
	if got := m.Ahead(); !eqStrings(got, want) {
		t.Fatalf("Ahead()=%v, ждём %v", got, want)
	}
}

// D3: SetAhead puts the tracks in the given order after the current one.
func TestSetAheadOrder(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.SetAhead([]string{tracks[2].Key(), tracks[1].Key()})
	want := []string{tracks[2].Key(), tracks[1].Key()}
	if got := m.Ahead(); !eqStrings(got, want) {
		t.Fatalf("Ahead()=%v, ждём %v", got, want)
	}
}

// D4: enter on the player input sends Say with the typed line.
func TestSayAction(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.Key("/", "")
	m.Key("", "x")
	m.Key("enter", "")
	says := recorded[Say](r)
	if len(says) != 1 || says[0].Text != "x" {
		t.Fatalf(" ждём Say{Text:\"x\"}, записано %v", r.acts)
	}
}

// D4: a digit picks the toast question option; a second AskLater while the first
// toast is still up adds nothing new.
func TestLaterPick(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	tracks := threeTracks()
	m.JumpPlay(tracks, 0, 0)
	m.AskLater(Question{Title: "q1", Options: []string{"early", "late"}})
	m.Key("2", "")
	m.AskLater(Question{Title: "q2", Options: []string{"x", "y"}})
	m.Key("2", "")
	picks := recorded[LaterPick](r)
	if len(picks) != 1 || picks[0].N != 2 {
		t.Fatalf("ждём один LaterPick{2}, записано %v", r.acts)
	}
}

// The terminal turns mouse-wheel scrolling into arrow keys: arrows must not rate.
func TestArrowsDoNotRate(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Step(eventDT)
	for _, k := range []string{"up", "down", "up", "down"} {
		m.Key(k, "")
	}
	steps(m, 1)
	if rs := recorded[Rate](r); len(rs) != 0 {
		t.Fatalf("стрелки оценили трек: %v", r.acts)
	}
}

func TestPasteReply(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Step(eventDT)
	m.Paste("без вокала")
	m.Key("/", "/")
	m.Paste("без вокала\n")
	m.Key("enter", "")
	if got := recorded[Say](r); len(got) != 1 || got[0].Text != "без вокала" {
		t.Fatalf("ждём один Say «без вокала», записано %v", r.acts)
	}
}
