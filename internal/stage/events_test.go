package stage

import (
	"strings"
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
		if a[i] != b[i] { //nolint:gosec // G602: the lengths are compared above
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

// #3: while the agent says nothing, the orb stays up with "агент думает".
func TestOrbHoldsUntilAgentSpeaks(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("ru")
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	steps(m, 10)
	if p := m.Phase(); p != PhLaunch {
		t.Fatalf("фаза %v, ждём PhLaunch", p)
	}
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "агент думает") || strings.Contains(frame, "Порция 1") {
		t.Errorf("ждём орб «агент думает» без сетки скана:\n%s", frame)
	}
}

// #3: the orb shows what the agent is busy with.
func TestOrbShowsProgress(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	steps(m, 3)
	m.Progress("читаю про Blonde")
	steps(m, 0.5)
	if p := m.Phase(); p != PhLaunch {
		t.Fatalf("фаза %v, ждём PhLaunch", p)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "читаю про Blonde") {
		t.Errorf("кадр без статуса:\n%s", frame)
	}
}

// #3: a late first question takes the orb down in a quarter second and opens the card.
func TestOrbLeavesForQuestion(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	steps(m, 5)
	m.Ask([]Question{{Title: "q1", Options: []string{"a", "b"}}})
	steps(m, 0.15)
	if p := m.Phase(); p != PhLaunch {
		t.Fatalf("фаза %v, ждём PhLaunch, пока орб гаснет", p)
	}
	if !runTo(m, PhAsk, 15) {
		t.Fatalf("фаза %v, ждём PhAsk", m.Phase())
	}
}

// #3: an early question still waits out the launch intro.
func TestOrbKeepsIntroForEarlyQuestion(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	m.Ask([]Question{{Title: "q1", Options: []string{"a", "b"}}})
	steps(m, 1.8)
	if p := m.Phase(); p != PhLaunch {
		t.Fatalf("фаза %v, ждём PhLaunch до конца вступления", p)
	}
	if !runTo(m, PhAsk, 30) {
		t.Fatalf("фаза %v, ждём PhAsk", m.Phase())
	}
}

// #3: without questions the first pick takes the orb down and starts the scan.
func TestOrbLeavesForPick(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	steps(m, 3)
	m.Candidate(Track{Artist: "Solo", Title: "One"}, true, "")
	if !runTo(m, PhScan, 15) {
		t.Fatalf("фаза %v, ждём PhScan", m.Phase())
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
	for range 60 {
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
	if len(ans) != 1 || len(ans[0].Raw) != 2 || ans[0].Raw[0] != "2" || ans[0].Raw[1] != "4" {
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
	toScan(t, m)
	tracks := threeTracks()
	m.Candidate(tracks[0], true, "")
	m.Candidate(tracks[1], true, "")
	m.Candidate(tracks[2], true, "")
	m.Candidate(Track{Artist: "Delta", Title: "Four"}, false, "no")
	m.PortionDone(tracks, "n")
	sawDone := false
	for range int(5 / eventDT) {
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

func TestEscNeverQuits(t *testing.T) {
	r := &recorder{}
	m := liveOnboard(t, Options{}, r)
	m.Key("esc", "")
	if _, _, _, ok := m.Onboarding(); !ok {
		t.Fatal("esc закрыл онбординг")
	}

	m = New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	if !runTo(m, PhIntro, 300) {
		t.Fatalf("фаза %v, ждём PhIntro", m.Phase())
	}
	m.Key("", "lofi")
	m.Key("esc", "")
	if m.Input() != "" {
		t.Fatalf("esc на стартовом экране не очистил поле: %q", m.Input())
	}

	m.JumpPlay(threeTracks(), 0, 0)
	m.Step(eventDT)
	m.Key("?", "?")
	m.Key("esc", "")
	if m.help {
		t.Fatal("esc в плеере не закрыл подсказку")
	}
	if q := recorded[Quit](r); len(q) != 0 {
		t.Fatalf("esc вышел из приложения: %v", r.acts)
	}
	m.Key("q", "q")
	if q := recorded[Quit](r); len(q) != 1 {
		t.Fatalf("q в плеере должен выходить: %v", r.acts)
	}
}
