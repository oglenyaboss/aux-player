package stage

import (
	"strings"
	"testing"
)

func askCard(t *testing.T, m *Model, qs []Question) {
	t.Helper()
	m.SetSize(110, 32)
	m.Key("", "abc")
	m.Key("enter", "")
	m.Ask(qs)
	for i := 0; i < 120 && m.Phase() != PhAsk; i++ {
		m.Step(eventDT)
	}
	if m.Phase() != PhAsk {
		t.Fatalf("фаза %v, ждём PhAsk", m.Phase())
	}
}

func untilAnswers(t *testing.T, m *Model, r *recorder) []Answers {
	t.Helper()
	for i := 0; i < 120 && len(recorded[Answers](r)) == 0; i++ {
		m.Step(eventDT)
	}
	ans := recorded[Answers](r)
	if len(ans) == 0 {
		t.Fatalf("Answers не отправлены, записано %v", r.acts)
	}
	return ans
}

// R1: the card opens on the option the agent proposed as default.
func TestAskDefaultOption(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{{Title: "q", Options: []string{"a", "b", "c"}, Def: 2}})
	steps(m, 1)
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	last := ans[len(ans)-1]
	if !eqStrings(last.Raw, []string{"3"}) {
		t.Fatalf("ждали Raw [3], записано %v", last.Raw)
	}
}

// R1: a default past the last option clamps to it.
func TestAskDefaultClamp(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{{Title: "q", Options: []string{"a", "b", "c"}, Def: 7}})
	steps(m, 1)
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	last := ans[len(ans)-1]
	if !eqStrings(last.Raw, []string{"3"}) {
		t.Fatalf("ждали Raw [3], записано %v", last.Raw)
	}
}

// R2: a long title and a long option are cut, the card border stays in place.
func TestAskLongTitle(t *testing.T) {
	m := New(Options{})
	askCard(t, m, []Question{{
		Title:   strings.Repeat("я", 90),
		Options: []string{strings.Repeat("ж", 80)},
	}})
	steps(m, 1.5)
	frame := stripANSI(m.View())
	var line string
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, strings.Repeat("я", 10)) {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("строка с вопросом не найдена:\n%s", frame)
	}
	last := strings.LastIndex(line, "я")
	if last < 0 {
		t.Fatalf("в строке нет вопроса: %q", line)
	}
	if !strings.Contains(line[last:], "│") {
		t.Fatalf("справа от вопроса нет границы карточки: %q", line)
	}
	if n := strings.Count(line, "я"); n > 58 {
		t.Fatalf("вопрос не обрезан: %d символов я", n)
	}
}

// R3: the second round sends only its own answer.
func TestAskSecondRound(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{{Title: "q1", Options: []string{"a", "b", "c"}}})
	steps(m, 1)
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[0].Raw, []string{"1"}) {
		t.Fatalf("первый раунд: ждали Raw [1], записано %v", ans[0].Raw)
	}
	m.Ask([]Question{{Title: "q2", Options: []string{"a", "b", "c"}}})
	steps(m, 1)
	m.Key("3", "")
	for i := 0; i < 120 && len(recorded[Answers](r)) < 2; i++ {
		m.Step(eventDT)
	}
	ans = recorded[Answers](r)
	if len(ans) != 2 {
		t.Fatalf("ждали два Answers, записано %v", r.acts)
	}
	last := ans[len(ans)-1]
	if !eqStrings(last.Raw, []string{"3"}) || len(last.Raw) != 1 {
		t.Fatalf("второй раунд: ждали Raw [3], записано %v", last.Raw)
	}
}

// R4: an unanswered toast question leaves after 45 s.
func TestLaterExpires(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.AskLater(Question{Title: "first", Options: []string{"x", "y"}})
	steps(m, 47)
	if m.laterToast() != nil {
		t.Fatalf("вопрос-тост всё ещё висит")
	}
}

// R4: a newer toast question replaces an unanswered one.
func TestLaterReplaces(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.AskLater(Question{Title: "first", Options: []string{"x", "y"}})
	steps(m, 1)
	m.AskLater(Question{Title: "second", Options: []string{"x", "y"}})
	if got := m.laterToast(); got == nil || got.q.Title != "second" {
		t.Fatalf("ждали вопрос second, записано %v", got)
	}
}
