package stage

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// runTo steps until the phase becomes p or the step budget runs out.
func runTo(m *Model, p Phase, maxSteps int) bool {
	for range maxSteps {
		if m.Phase() == p {
			return true
		}
		m.Step(eventDT)
	}
	return m.Phase() == p
}

// toScan walks the usual first turn: enter, one card answered, back to the scan.
func toScan(t *testing.T, m *Model) {
	t.Helper()
	m.Key("", "abc")
	m.Key("enter", "")
	m.Ask([]Question{{Title: "q", Options: []string{"a", "b"}}})
	if !runTo(m, PhAsk, 300) {
		t.Fatalf("phase %v, want PhAsk", m.Phase())
	}
	steps(m, 0.8)
	m.Key("enter", "")
	if !runTo(m, PhScan, 300) {
		t.Fatalf("phase %v, want PhScan", m.Phase())
	}
}

// R1: in the player without SelfPlay the queue keys pass through to the host: the key
// is not taken, nothing is sent, the queue is untouched. Service keys stay on the screen.
func TestKeyPassThrough(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	if m.Key("n", "") {
		t.Errorf("n without SelfPlay went to the host, but the screen took it")
	}
	if m.Key("space", "") {
		t.Errorf("space must pass through")
	}
	if m.Key("left", "") {
		t.Errorf("left must pass through")
	}
	if !m.Key("+", "") {
		t.Errorf("+ must be taken")
	}
	if !m.Key("w", "") {
		t.Errorf("w must be taken")
	}
	if saves := recorded[Save](r); len(saves) != 1 {
		t.Fatalf("want one Save, recorded %v", r.acts)
	}
	if m.Phase() != PhPlay {
		t.Errorf("the phase changed: %v", m.Phase())
	}
	tr := threeTracks()
	if got := m.Ahead(); !eqStrings(got, []string{tr[1].Key(), tr[2].Key()}) {
		t.Errorf("Ahead()=%v, want the queue untouched", got)
	}
}

// R1: with SelfPlay n stays on the screen and moves to the next track.
func TestKeySelfPlay(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on, SelfPlay: true})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	if !m.Key("n", "") {
		t.Errorf("n with SelfPlay must be taken")
	}
	tr := threeTracks()
	if got := m.Ahead(); !eqStrings(got, []string{tr[2].Key()}) {
		t.Fatalf("Ahead()=%v, want %v", got, []string{tr[2].Key()})
	}
}

// R2: Progress shows the line the agent passed in, on the scan status row.
func TestProgressStatus(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	toScan(t, m)
	m.Progress("ищу свежие релизы")
	if frame := stripANSI(m.View()); !strings.Contains(frame, "ищу свежие релизы") {
		t.Errorf("frame without the status:\n%s", frame)
	}
}

// R3: a batch of candidates auditions one at a time: the first steps on stage at once,
// the rest wait behind it and all of them are through within about fifteen seconds.
func TestCandidatePacing(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	toScan(t, m)
	tr := Track{Artist: "Solo", Title: "One"}
	for range 5 {
		m.Candidate(tr, true, "")
	}
	steps(m, 0.1)
	if len(m.cands) != 1 {
		t.Fatalf("%d on stage 0.1 s after the batch, want 1", len(m.cands))
	}
	steps(m, 0.5)
	if len(m.cands) != 1 {
		t.Fatalf("%d on stage after 0.6 s, want 1: the next one waits for the first to leave", len(m.cands))
	}
	steps(m, 14)
	if len(m.cands) != 5 || len(m.inbox) != 0 {
		t.Fatalf("%d passed, %d in the queue after 14.6 s; want 5 and 0", len(m.cands), len(m.inbox))
	}
}

// R4: the next portion in the player tops the queue up without duplicates.
func TestPortionInPlay(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	a := Track{Artist: "Aa", Title: "A"}
	b := Track{Artist: "Bb", Title: "B"}
	c := Track{Artist: "Cc", Title: "C"}
	d := Track{Artist: "Dd", Title: "D"}
	m.JumpPlay([]Track{a, b}, 0, 0)
	m.PortionDone([]Track{b, c, d}, "")
	if got := m.Ahead(); !eqStrings(got, []string{b.Key(), c.Key(), d.Key()}) {
		t.Fatalf("Ahead()=%v, want %v", got, []string{b.Key(), c.Key(), d.Key()})
	}
}

// R5: an empty portion in the scan sends the stage back to the intro with the error.
func TestEmptyPortionFails(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("ru")
	m := New(Options{})
	m.SetSize(110, 32)
	toScan(t, m)
	m.PortionDone(nil, "")
	steps(m, 1)
	if m.Phase() != PhIntro {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "ничего не нашлось") {
		t.Errorf("frame without the error:\n%s", frame)
	}
}

// R6: questions in the player are answered with the defaults, one record with all of them.
func TestAskInPlay(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks()[:2], 0, 0)
	m.Ask([]Question{
		{Title: "q1", Options: []string{"a", "b"}},
		{Title: "q2", Scale: true, Left: "l", Right: "r", Def: 2},
	})
	ans := recorded[Answers](r)
	if len(ans) != 1 || !eqStrings(ans[0].Raw, []string{"", ""}) {
		t.Fatalf("want Answers{Raw:[ ]}, recorded %v", r.acts)
	}
}

// R7: the stage follows the track and position of the real player.
func TestNowFollowsPlayer(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tr := threeTracks()
	m.JumpPlay(tr, 0, 0)
	m.Now(tr[2].Key(), 42, true)
	if got := m.Ahead(); len(got) != 0 {
		t.Fatalf("Ahead()=%v, want the current C", got)
	}
	if m.pos != 42 {
		t.Fatalf("pos=%v, want 42", m.pos)
	}
}

// R7: the player state is remembered before the flow reaches the player, and the queue
// opens on the track the player is at.
func TestNowBeforePlay(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	tr := threeTracks()
	toScan(t, m)
	m.Now(tr[1].Key(), 10, true)
	m.PortionDone(tr, "")
	m.Candidate(tr[0], true, "")
	m.Candidate(tr[1], true, "")
	m.Candidate(tr[2], true, "")
	if !runTo(m, PhPlay, 1500) {
		t.Fatalf("phase %v, want PhPlay", m.Phase())
	}
	if m.cur != 1 || m.queue[m.cur].Key() != tr[1].Key() {
		t.Fatalf("current track %d %q, want B", m.cur, m.queue[m.cur].Key())
	}
	if m.pos != 10 {
		t.Errorf("pos=%v, want 10", m.pos)
	}
}

// R8: while the real player is paused the stage's own position is frozen.
func TestPausedHoldsPos(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	solo := []Track{{Artist: "Solo", Title: "One", Dur: 120}}
	m.JumpPlay(solo, 0, 0)
	m.Now(solo[0].Key(), 5, false)
	steps(m, 2)
	if m.pos != 5 {
		t.Fatalf("pos=%v, want 5", m.pos)
	}
}

// R9: a failure outside the player returns to the intro, the query preserved, and the
// first key clears the error.
func TestFailRestoresQuery(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	m.Fail("сеть упала")
	if m.Phase() != PhIntro {
		t.Fatalf("phase %v, want PhIntro", m.Phase())
	}
	if m.Input() != "abc" {
		t.Fatalf("Input()=%q, want abc", m.Input())
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "сеть упала") {
		t.Errorf("frame without the error:\n%s", frame)
	}
	m.Key("x", "")
	if frame := stripANSI(m.View()); strings.Contains(frame, "сеть упала") {
		t.Errorf("the error stayed after a key:\n%s", frame)
	}
}

// R10: the scale of any length: Steps=3 limits the knob at the last stop on the right,
// the answer stays 1-based.
func TestScaleSteps(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	m.Ask([]Question{{Title: "scale", Scale: true, Steps: 3, Left: "l", Right: "r", Def: 1}})
	if !runTo(m, PhAsk, 300) {
		t.Fatalf("phase %v, want PhAsk", m.Phase())
	}
	steps(m, 1)
	m.Key("right", "")
	m.Key("right", "")
	m.Key("enter", "")
	for i := 0; i < 120 && len(recorded[Answers](r)) == 0; i++ {
		m.Step(eventDT)
	}
	ans := recorded[Answers](r)
	if len(ans) != 1 || !eqStrings(ans[0].Raw, []string{"3"}) {
		t.Fatalf("want Answers{Raw:[3]}, recorded %v", r.acts)
	}
}

// R11: a same-size SetSize leaves the logo untouched.
func TestSetSizeSame(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	n1 := len(m.logo)
	ptr1 := &m.logo[0]
	m.SetSize(110, 32)
	m.SetSize(110, 32)
	if len(m.logo) != n1 || &m.logo[0] != ptr1 {
		t.Errorf("a same-size SetSize rebuilt the logo")
	}
}
