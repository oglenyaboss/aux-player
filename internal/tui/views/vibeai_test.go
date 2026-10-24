package views

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

func key(v *VibeAIModel, k string) (tea.Cmd, bool) {
	return v.HandleKey(keyMsg(k))
}

func typeText(v *VibeAIModel, s string) {
	for _, r := range s {
		v.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func vibeTrack(id, artist, title, why string) VibeAITrack {
	return VibeAITrack{ID: id, Artist: artist, Title: title, Album: "Album", Year: "2019", Why: why}
}

func trackIDs(v *VibeAIModel) []string {
	var ids []string
	for _, r := range v.feed {
		if r.kind == rowTrack {
			ids = append(ids, r.track.ID)
		}
	}
	return ids
}

func dividerCount(v *VibeAIModel) int {
	n := 0
	for _, r := range v.feed {
		if r.kind == rowDivider {
			n++
		}
	}
	return n
}

func firstDone(v *VibeAIModel) string {
	for _, r := range v.feed {
		if r.kind == rowDone {
			return r.text
		}
	}
	return ""
}

func TestVibeAISubmitStartsFeed(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	typeText(v, "в духе X")
	cmd, handled := key(v, "enter")
	if !handled || cmd == nil {
		t.Fatal("enter on the intro must submit")
	}
	sub, ok := cmd().(VibeAISubmitMsg)
	if !ok || sub.Text != "в духе X" || !sub.Fresh {
		t.Fatalf("submit = %#v", sub)
	}
	if v.screen != aiFeed {
		t.Fatalf("screen = %d, want aiFeed", v.screen)
	}
	users := 0
	for _, r := range v.feed {
		if r.kind == rowUser {
			users++
			if r.text != "в духе X" {
				t.Fatalf("user row = %q", r.text)
			}
		}
	}
	if users != 1 {
		t.Fatalf("user rows = %d, want 1", users)
	}
}

func TestVibeAIWarmupRound(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.Ask([]VibeAIQuestion{
		{Question: "основа", Kind: "choice", Options: []string{"a", "b", "c"}, Default: "b"},
		{Question: "энергия", Kind: "scale", Steps: 5, Left: "фон", Right: "разъёб", Default: "3"},
	})
	if !v.Capturing() {
		t.Fatal("warmup question must capture keys")
	}
	if _, handled := key(v, "right"); !handled {
		t.Fatal("right must drive the question")
	}
	if r := v.feed[v.warmRow()]; r.qCur != 2 {
		t.Fatalf("qCur = %d, want 2", r.qCur)
	}
	if cmd, _ := key(v, "enter"); cmd != nil {
		t.Fatalf("first answer must not finish the round: %#v", cmd)
	}
	if got := firstDone(v); got != "основа: c" {
		t.Fatalf("rowDone = %q, want %q", got, "основа: c")
	}
	if _, handled := key(v, "left"); !handled {
		t.Fatal("left must drive the scale")
	}
	if r := v.feed[v.warmRow()]; r.qCur != 2 {
		t.Fatalf("scale qCur = %d, want 2", r.qCur)
	}
	cmd, _ := key(v, "enter")
	msg, ok := cmd().(VibeAIAnswersMsg)
	if !ok {
		t.Fatal("last answer must finish the round")
	}
	if strings.Join(msg.Answers, "|") != "3|2" {
		t.Fatalf("answers = %v, want [3 2]", msg.Answers)
	}
	if v.Capturing() {
		t.Fatal("keys must be released after the round")
	}
}

func TestVibeAIWarmupEscapeAndOwn(t *testing.T) {
	v := NewVibeAI()
	v.SetWorking("")
	v.Ask([]VibeAIQuestion{{Question: "?", Kind: "choice", Options: []string{"a", "b"}, Default: "b"}})
	cmd, handled := key(v, "esc")
	if !handled || cmd == nil {
		t.Fatal("esc must answer with the default")
	}
	msg, ok := cmd().(VibeAIAnswersMsg)
	if !ok || len(msg.Answers) != 1 || msg.Answers[0] != "" {
		t.Fatalf("esc answers = %v", msg.Answers)
	}

	v = NewVibeAI()
	v.SetWorking("")
	v.Ask([]VibeAIQuestion{{Question: "громкость", Kind: "choice", Options: []string{"низкая", "высокая"}}})
	if _, handled := key(v, "t"); !handled {
		t.Fatal("t must open own-answer input")
	}
	if !v.Capturing() {
		t.Fatal("typing must capture keys")
	}
	typeText(v, "громче")
	cmd, _ = key(v, "enter")
	msg, ok = cmd().(VibeAIAnswersMsg)
	if !ok || len(msg.Answers) != 1 || msg.Answers[0] != "громче" {
		t.Fatalf("own answer = %v, want [громче]", msg.Answers)
	}
}

func TestVibeAIPicksAndPortion(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	t1 := vibeTrack("1", "A", "T1", "")
	t2 := vibeTrack("2", "B", "T2", "")
	t3 := vibeTrack("3", "C", "T3", "")
	v.AddPick(t1, "", true, "")
	v.AddPick(VibeAITrack{}, "Ghost — X", false, "нет в Apple")
	v.AddPick(t2, "", true, "")
	if dividers := dividerCount(v); dividers != 1 {
		t.Fatalf("dividers = %d, want 1", dividers)
	}
	if v.trackCount() != 2 {
		t.Fatalf("tracks = %d, want 2", v.trackCount())
	}
	strip := ansi.Strip(v.View())
	if !strings.Contains(strip, "+ A") {
		t.Errorf("ticker misses + A:\n%s", strings.ReplaceAll(strip, "\n", " | "))
	}
	if !strings.Contains(strip, "− Ghost — X: нет в Apple") {
		t.Errorf("ticker misses the rejected pick")
	}
	v.PortionDone([]VibeAITrack{t2, t1, t3}, "держу рэйдж")
	want := []string{"2", "1", "3"}
	if got := trackIDs(v); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("track order = %v, want %v", got, want)
	}
	if dividers := dividerCount(v); dividers != 1 {
		t.Fatalf("dividers after done = %d, want 1", dividers)
	}
	if strip = ansi.Strip(v.View()); !strings.Contains(strip, "держу рэйдж") {
		t.Errorf("divider note missing")
	}
	if v.Busy() {
		t.Fatal("portion done must release busy")
	}
}

func TestVibeAISecondPortion(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}, "первая")
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("4", "D", "T4", ""),
		vibeTrack("5", "E", "T5", ""),
	}, "второй заход")
	if dividers := dividerCount(v); dividers != 2 {
		t.Fatalf("dividers = %d, want 2", dividers)
	}
	if v.trackCount() != 5 {
		t.Fatalf("tracks = %d, want 5", v.trackCount())
	}
	if v.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 (unchanged)", v.cursor)
	}
	if strip := ansi.Strip(v.View()); !strings.Contains(strip, "── 2") {
		t.Errorf("second divider missing")
	}
}

func TestVibeAICursorPlay(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
		vibeTrack("4", "D", "T4", ""),
		vibeTrack("5", "E", "T5", ""),
	}, "порция")
	for range 3 {
		key(v, "j")
	}
	if v.cursor != 3 {
		t.Fatalf("cursor = %d, want 3", v.cursor)
	}
	cmd, handled := key(v, "enter")
	if !handled {
		t.Fatal("enter must play from cursor")
	}
	msg, ok := cmd().(VibeAIActionMsg)
	if !ok || msg.Action != VibeAIPlay || msg.Index != 3 {
		t.Fatalf("action = %#v, want play index 3", msg)
	}
}

func TestVibeAITrackStates(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}, "порция")
	v.SetPlaying("1")
	if r := v.feed[v.lastTrack("1")]; r.state != trackPlaying {
		t.Fatalf("state = %d, want playing", r.state)
	}
	v.SetSkipped("1", 8)
	strip := ansi.Strip(v.View())
	if !strings.Contains(strip, "скип на 0:08") {
		t.Errorf("skipped row missing skip mark")
	}
	if !strings.Contains(strip, "↷1") {
		t.Errorf("chips missing ↷1")
	}
	v.SetPlaying("3")
	if r := v.feed[v.lastTrack("3")]; r.state != trackPlaying {
		t.Fatalf("state = %d, want playing", r.state)
	}
	v.SetPlaying("2")
	if r := v.feed[v.lastTrack("3")]; r.state != trackUpcoming {
		t.Fatalf("state = %d, want upcoming after switching", r.state)
	}
	if r := v.feed[v.lastTrack("2")]; r.state != trackPlaying {
		t.Fatalf("state = %d, want playing", r.state)
	}
}

func TestVibeAILaterQuestion(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}, "порция")
	v.SetPlaying("2")
	v.AskLater(VibeAIQuestion{Question: "убрать хоррор?", Options: []string{"да", "нет"}})
	pi := v.lastTrack("2")
	if r := v.feed[pi+1]; r.kind != rowQuestion || !r.later {
		t.Fatal("later question must follow the playing track")
	}
	if v.Capturing() {
		t.Fatal("later question must not capture keys")
	}
	cmd, handled := key(v, "2")
	if !handled || cmd == nil {
		t.Fatal("digit must answer the later question")
	}
	msg, ok := cmd().(VibeAILaterMsg)
	if !ok || msg.Raw != "2" {
		t.Fatalf("later = %#v, want Raw 2", msg)
	}
	if r := v.feed[pi+1]; r.kind != rowDone || r.text != "убрать хоррор?: нет" {
		t.Fatalf("done row = %q", r.text)
	}
	if _, handled := key(v, "2"); handled {
		t.Fatal("digits must pass through without a later question")
	}
	if _, handled := key(v, "n"); handled {
		t.Fatal("n must never be intercepted")
	}
	if _, handled := key(v, "space"); handled {
		t.Fatal("space must never be intercepted")
	}
}

func TestVibeAILaterDismiss(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
	}, "порция")
	v.AskLater(VibeAIQuestion{Question: "убрать хоррор?", Options: []string{"да", "нет"}})
	if v.laterIdx() < 0 {
		t.Fatal("later question missing")
	}
	if _, handled := key(v, "esc"); !handled {
		t.Fatal("esc must dismiss the later question")
	}
	if v.laterIdx() >= 0 {
		t.Fatal("later row must be gone")
	}
	for _, r := range v.feed {
		if r.kind == rowQuestion && r.later {
			t.Fatal("later rows leaked")
		}
	}
	if _, handled := key(v, "esc"); handled {
		t.Fatal("second esc must reach global keys")
	}
}

func TestVibeAICancelAndError(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.AddPick(vibeTrack("1", "A", "T1", ""), "", true, "")
	v.Cancelled()
	if v.trackCount() != 0 {
		t.Fatalf("tracks = %d, want 0", v.trackCount())
	}
	if v.screen != aiIntro {
		t.Fatalf("screen = %d, want aiIntro", v.screen)
	}
	if v.input.Value() != v.query {
		t.Fatalf("input = %q, want query %q", v.input.Value(), v.query)
	}

	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}, "первая")
	v.SetWorking("")
	v.AddPick(vibeTrack("9", "Z", "T9", ""), "", true, "")
	v.SetError(errors.New("boom"))
	if v.trackCount() != 3 {
		t.Fatalf("tracks = %d, want 3", v.trackCount())
	}
	if dividers := dividerCount(v); dividers != 1 {
		t.Fatalf("dividers = %d, want 1", dividers)
	}
	errRow := false
	for _, r := range v.feed {
		if r.kind == rowError && r.text == "boom" {
			errRow = true
		}
	}
	if !errRow {
		t.Fatal("error row missing")
	}
	if v.screen != aiFeed {
		t.Fatalf("screen = %d, want aiFeed", v.screen)
	}
}

func TestVibeAIMoreKey(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	cmd, handled := key(v, "m")
	if !handled || cmd != nil {
		t.Fatalf("m while building: cmd=%v handled=%v", cmd, handled)
	}
	if v.notice != "уже подбираю" {
		t.Fatalf("notice = %q", v.notice)
	}
	v.PortionDone([]VibeAITrack{vibeTrack("1", "A", "T1", "")}, "порция")
	cmd, handled = key(v, "m")
	if !handled {
		t.Fatal("m must be handled in the feed")
	}
	sub, ok := cmd().(VibeAISubmitMsg)
	if !ok || sub.Fresh || sub.Text != "" {
		t.Fatalf("more submit = %#v", sub)
	}
}

func TestVibeAIIntroMemoryAndHistory(t *testing.T) {
	v := NewVibeAI()
	v.SetMemory([]string{"любит рэйдж"})
	v.SetSize(80, 30)
	strip := ansi.Strip(v.View())
	if !strings.Contains(strip, "что я о тебе знаю") {
		t.Errorf("memory heading missing")
	}
	if !strings.Contains(strip, "любит рэйдж") {
		t.Errorf("memory line missing")
	}
	ask := func(q string) {
		typeText(v, q)
		key(v, "enter")
	}
	ask("q1")
	if _, handled := key(v, "x"); !handled {
		t.Fatal("x must return to the intro")
	}
	ask("q2")
	if strings.Join(v.history, ",") != "q2,q1" {
		t.Fatalf("history = %v, want [q2 q1]", v.history)
	}
	key(v, "x")
	key(v, "down")
	if v.input.Value() != "q2" {
		t.Fatalf("input = %q, want q2", v.input.Value())
	}
	key(v, "down")
	if v.input.Value() != "q1" {
		t.Fatalf("input = %q, want q1", v.input.Value())
	}
	key(v, "up")
	if v.input.Value() != "q2" {
		t.Fatalf("input = %q, want q2", v.input.Value())
	}
}

func TestVibeAIFeedFooterWraps(t *testing.T) {
	v := NewVibeAI()
	v.SetSize(60, 30)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
	}, "порция")
	strip := ansi.Strip(v.View())
	for _, want := range []string{"enter играть отсюда", "n скип", "space пауза", "t сказать", "m ещё", "a в очередь", "w в плейлист", "o выкл поток", "x новый"} {
		if !strings.Contains(strip, want) {
			t.Errorf("footer missing %q", want)
		}
	}
}

func TestVibeAIViewFits(t *testing.T) {
	t1 := vibeTrack("1", "A", "T1", "")
	t2 := vibeTrack("2", "B", "T2", "")
	t3 := vibeTrack("3", "C", "T3", "")
	t4 := vibeTrack("4", "D", "T4", "")
	t5 := vibeTrack("5", "E", "T5", "")
	setups := map[string]func(*VibeAIModel){
		"intro": func(v *VibeAIModel) {},
		"question": func(v *VibeAIModel) {
			v.SetWorking("")
			v.Ask([]VibeAIQuestion{{
				Question: "насколько энергично?",
				Kind:     "scale",
				Steps:    5,
				Left:     "фон",
				Right:    "разъёб",
				Default:  "3",
			}})
		},
		"building": func(v *VibeAIModel) {
			v.SetWorking("")
			v.AddProgress("· similar_artists Tame Impala")
			v.AddPick(vibeTrack("6", "Alpha", "Один", "цепляет"), "", true, "")
			v.AddPick(VibeAITrack{}, "Ghost — X", false, "нет в Apple")
		},
		"feed": func(v *VibeAIModel) {
			v.SetWorking("")
			v.PortionDone([]VibeAITrack{t1, t2, t3}, "первая порция")
			v.SetPlayed("1")
			v.SetSkipped("2", 45)
			v.SetPlaying("3")
			v.SetWorking("")
			v.PortionDone([]VibeAITrack{t4, t5}, "вторая порция")
			v.AskLater(VibeAIQuestion{Question: "скипаешь хоррор — убрать совсем?", Options: []string{"да, убирай", "нет, просто не сейчас"}})
		},
		"failed": func(v *VibeAIModel) {
			v.SetError(errors.New(strings.Repeat("boom ", 40)))
		},
	}
	for _, w := range []int{40, 80, 120, 160} {
		for _, h := range []int{10, 20, 40} {
			for name, setup := range setups {
				v := NewVibeAI()
				v.SetSize(w, h)
				setup(v)
				lines := strings.Split(v.View(), "\n")
				if len(lines) != h {
					t.Errorf("%s w=%d h=%d: %d lines", name, w, h, len(lines))
					continue
				}
				for i, l := range lines {
					if lw := ansi.StringWidth(l); lw > w {
						t.Errorf("%s w=%d h=%d: line %d width %d: %q", name, w, h, i, lw, ansi.Strip(l))
					}
				}
			}
		}
	}
}

func vibeRateFeed(t *testing.T) *VibeAIModel {
	t.Helper()
	v := NewVibeAI()
	v.SetSize(80, 24)
	v.SetWorking("")
	v.PortionDone([]VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}, "порция")
	return v
}

func rateKey(t *testing.T, v *VibeAIModel, k string) VibeAIRateMsg {
	t.Helper()
	cmd, handled := key(v, k)
	if !handled {
		t.Fatalf("%q must be handled in the feed", k)
	}
	if cmd == nil {
		t.Fatalf("%q must emit a rating msg", k)
	}
	got := cmd()
	msg, ok := got.(VibeAIRateMsg)
	if !ok {
		t.Fatalf("%q: msg = %#v, want VibeAIRateMsg", k, got)
	}
	return msg
}

func TestVibeAIRateKeys(t *testing.T) {
	v := vibeRateFeed(t)
	key(v, "j")
	if v.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", v.cursor)
	}
	if msg := rateKey(t, v, "]"); msg.Index != 1 || msg.Rating != 1 {
		t.Fatalf("] = %+v, want index 1 rating 1", msg)
	}
	if strip := ansi.Strip(v.View()); !strings.Contains(strip, "▲") {
		t.Errorf("view missing ▲ after ]:\n%s", strip)
	}

	if msg := rateKey(t, v, "]"); msg.Index != 1 || msg.Rating != 0 {
		t.Fatalf("second ] = %+v, want toggle to 0", msg)
	}
	if strip := ansi.Strip(v.View()); strings.Contains(strip, "▲") {
		t.Errorf("view still has ▲ after toggle:\n%s", strip)
	}

	if msg := rateKey(t, v, "["); msg.Index != 1 || msg.Rating != -1 {
		t.Fatalf("[ = %+v, want index 1 rating -1", msg)
	}
	if strip := ansi.Strip(v.View()); !strings.Contains(strip, "▼") {
		t.Errorf("view missing ▼ after [:\n%s", strip)
	}
	if msg := rateKey(t, v, "]"); msg.Index != 1 || msg.Rating != 1 {
		t.Fatalf("] after [ = %+v, want switch to 1", msg)
	}

	tracks := []VibeAITrack{
		vibeTrack("1", "A", "T1", ""),
		vibeTrack("2", "B", "T2", ""),
		vibeTrack("3", "C", "T3", ""),
	}
	settling := NewVibeAI()
	settling.SetSize(80, 24)
	settling.SetWorking("")
	for _, tr := range tracks {
		settling.AddPick(tr, "", true, "")
	}
	key(settling, "j")
	rateKey(t, settling, "]")
	settling.PortionDone(tracks, "порция")
	if r := settling.feed[settling.lastTrack("2")]; r.rating != 1 {
		t.Fatalf("rating after PortionDone = %d, want 1", r.rating)
	}
	if strip := ansi.Strip(settling.View()); !strings.Contains(strip, "▲") {
		t.Errorf("rating must survive PortionDone:\n%s", strip)
	}

	v.SetWorking("")
	v.PortionDone(tracks, "порция 2")
	if strip := ansi.Strip(v.View()); !strings.Contains(strip, "▲") {
		t.Errorf("rating must survive a new portion:\n%s", strip)
	}

	strip := ansi.Strip(v.View())
	for _, want := range []string{"больше", "меньше"} {
		if !strings.Contains(strip, want) {
			t.Errorf("footer missing %q:\n%s", want, strip)
		}
	}
}

func TestVibeAIFollowPlaying(t *testing.T) {
	v := vibeRateFeed(t)
	if v.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", v.cursor)
	}
	v.FollowPlaying("3")
	if v.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", v.cursor)
	}
	v.FollowPlaying("nope")
	if v.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 after unknown id", v.cursor)
	}
}
