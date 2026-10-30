package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func trackQ(def int) Question {
	return Question{
		Title:   "какое настроение?",
		Options: []string{"мечтательное", "уютное", "меланхолия", "плавное"},
		Tracks: []Track{
			{ID: "t1", Artist: "Mazzy Star", Title: "Fade Into You"},
			{ID: "t2", Artist: "Beach House", Title: "Space Song"},
			{ID: "t3", Artist: "Frank Ocean", Title: "Self Control"},
			{ID: "t4", Artist: "Slowdive", Title: "Alison"},
		},
		Def: def,
	}
}

func redCover(t *testing.T, m *Model, key string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 255, A: 255}
	for yy := range 8 {
		for xx := range 8 {
			img.Set(xx, yy, red)
		}
	}
	m.SetCover(key, img)
}

// R1: the track card draws each option's cover: the set image lands in the cover rectangle.
func TestTrackCardCovers(t *testing.T) {
	m := New(Options{})
	askCard(t, m, []Question{trackQ(2)})
	redCover(t, m, "t3")
	steps(m, 2)
	f := m.render()
	w := min(64, m.w-6)
	colW, cw, ch := trackCols(w, 4)
	x, y, _, _ := m.card()
	ox := x + 3 + 2*(colW+2) + (colW-cw)/2
	hit, total := 0, 0
	for yy := y + 4; yy < y+4+ch; yy++ {
		for xx := ox; xx < ox+cw; xx++ {
			total++
			c := f.at(xx, yy)
			if c == nil || !c.pix {
				continue
			}
			if c.fg.r > 0.8 && c.fg.g < 0.2 && c.bg.r > 0.8 && c.bg.g < 0.2 {
				hit++
			}
		}
	}
	if total == 0 || hit*2 < total {
		t.Fatalf("обложка варианта 2: красных пикселей %d из %d", hit, total)
	}
}

// R2: under each cover the label, the artist and the title are visible.
func TestTrackCardLabels(t *testing.T) {
	m := New(Options{})
	askCard(t, m, []Question{trackQ(2)})
	steps(m, 1.5)
	frame := stripANSI(m.View())
	for _, s := range []string{"мечтательное", "уютное", "меланхолия", "плавное", "Frank Ocean", "Self Control", "a всё вместе"} {
		if !strings.Contains(frame, s) {
			t.Fatalf("кадр без %q:\n%s", s, frame)
		}
	}
}

// R3: right moves the selection, enter answers.
func TestTrackCardRightEnter(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1)
	m.Key("right", "")
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"2"}) {
		t.Fatalf("ждали Raw [2], записано %v", r.acts)
	}
}

// R3: a digit answers straight away.
func TestTrackCardDigit(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1)
	m.Key("3", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"3"}) {
		t.Fatalf("ждали Raw [3], записано %v", r.acts)
	}
}

// R3: a answers "all of them" as one raw entry.
func TestTrackCardAll(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1)
	m.Key("a", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"all"}) {
		t.Fatalf("ждали Raw [all], записано %v", r.acts)
	}
}

// R4: without key presses the default option answers on enter.
func TestTrackCardDefault(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(2)})
	steps(m, 1)
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"3"}) {
		t.Fatalf("ждали Raw [3], записано %v", r.acts)
	}
}

// R5: a narrow window falls back to the list of covers, the answer still works.
func TestTrackCardNarrow(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	m.SetSize(46, 60)
	steps(m, 1.5)
	frame := stripANSI(m.View())
	for _, s := range []string{"мечтательное", "уютное", "меланхолия", "плавное"} {
		if !strings.Contains(frame, s) {
			t.Fatalf("кадр без %q:\n%s", s, frame)
		}
	}
	for line := range strings.SplitSeq(frame, "\n") {
		if strings.Contains(line, "←→") && !strings.HasSuffix(strings.TrimRight(line, " "), "│") {
			t.Fatalf("подсказка рвёт рамку карточки: %q", line)
		}
	}
	m.Key("2", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"2"}) {
		t.Fatalf("ждали Raw [2], записано %v", r.acts)
	}
}

// R6: a on a text question does nothing.
func TestTextCardIgnoresA(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{{Title: "q", Options: []string{"a", "b", "c"}}})
	m.Key("a", "")
	steps(m, 2)
	if n := len(recorded[Answers](r)); n != 0 {
		t.Fatalf("`a` на текстовом вопросе отправил ответ: %v", r.acts)
	}
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"1"}) {
		t.Fatalf("ждали Raw [1], записано %v", r.acts)
	}
}

// R7: the toast question draws covers and labels, a digit picks as before.
func TestLaterTrackToast(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	m.SetSize(110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.AskLater(Question{
		Title:   "дальше?",
		Options: []string{"дрим-поп", "соул"},
		Tracks: []Track{
			{ID: "d1", Artist: "Cocteau Twins", Title: "Cherry-Coloured Funk"},
			{ID: "d2", Artist: "Al Green", Title: "Simply Beautiful"},
		},
	})
	steps(m, 1)
	f := m.render()
	frame := stripANSI(f.String())
	if !strings.Contains(frame, "дрим-поп") || !strings.Contains(frame, "соул") {
		t.Fatalf("кадр без подписей вариантов:\n%s", frame)
	}
	tt := m.laterToast()
	if tt == nil {
		t.Fatalf("вопрос-тост не найден")
	}
	w := min(46, m.w-6)
	x := m.w - w - 3 + int(tt.x.x)
	hit, total := 0, 0
	for k := range 2 {
		ox := x + 2 + k*14
		for yy := yBase(m) - 8 + 1 + 3; yy < yBase(m)-8+1+6; yy++ {
			for xx := ox; xx < ox+6; xx++ {
				total++
				if c := f.at(xx, yy); c != nil && c.pix {
					hit++
				}
			}
		}
	}
	if total == 0 || hit*2 < total {
		t.Fatalf("обложки в тосте: пиксельных ячеек %d из %d", hit, total)
	}
	m.Key("2", "")
	picks := recorded[LaterPick](r)
	if len(picks) != 1 || picks[0].N != 2 {
		t.Fatalf("ждём один LaterPick{2}, записано %v", r.acts)
	}
}

func yBase(m *Model) int {
	if m.typing {
		return m.h - 6
	}
	return m.h - 4
}

// R8: Options.Slots sets the portion size; the portion ends and flows into PhDone.
func TestScanSlotsOption(t *testing.T) {
	m := New(Options{Slots: 5})
	m.SetSize(110, 32)
	toScan(t, m)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "0 из 5") {
		t.Fatalf("кадр без «0 из 5»:\n%s", frame)
	}
	tracks := []Track{
		{ID: "p1", Artist: "A", Title: "One", Dur: 120},
		{ID: "p2", Artist: "B", Title: "Two", Dur: 120},
		{ID: "p3", Artist: "C", Title: "Three", Dur: 120},
		{ID: "p4", Artist: "D", Title: "Four", Dur: 120},
		{ID: "p5", Artist: "E", Title: "Five", Dur: 120},
	}
	for _, tk := range tracks {
		m.Candidate(tk, true, "")
	}
	m.PortionDone(tracks, "")
	sawDone := false
	for range int(10 / eventDT) {
		m.Step(eventDT)
		if m.Phase() == PhDone {
			sawDone = true
		}
	}
	if !sawDone || (m.Phase() != PhDone && m.Phase() != PhPlay) {
		t.Fatalf("sawDone=%v фаза %v; ждём прохождение PhDone", sawDone, m.Phase())
	}
}

// R8: without the option the scan keeps the 20 slots.
func TestScanSlotsDefault(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	toScan(t, m)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "0 из 20") {
		t.Fatalf("кадр без «0 из 20»:\n%s", frame)
	}
}

// R1: esc on a track card answers "none of these" and leaves the chip.
func TestTrackCardEscNone(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("esc", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"none"}) {
		t.Fatalf("ждали Raw [none], записано %v", r.acts)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "мимо") {
		t.Fatalf("кадр без «мимо»:\n%s", frame)
	}
}

// R2: / then own words then enter answers with that text and leaves the chip.
func TestTrackCardOwnWords(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Key("", "потише")
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"потише"}) {
		t.Fatalf("ждали Raw [потише], записано %v", r.acts)
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "своё: потише") {
		t.Fatalf("кадр без «своё: потише»:\n%s", frame)
	}
}

// R3: esc during typing goes back to the choice without answering.
func TestTrackCardTypingEscBack(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Key("", "x")
	m.Key("esc", "")
	steps(m, 2)
	if n := len(recorded[Answers](r)); n != 0 {
		t.Fatalf("esc в вводе ответил: %v", r.acts)
	}
	m.Key("2", "2")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"2"}) {
		t.Fatalf("ждали Raw [2], записано %v", r.acts)
	}
}

// R4: a, digits and arrows typed into the input are text, not choices.
func TestTrackCardTypingKeysAreText(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Key("a", "a")
	m.Key("2", "2")
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"a2"}) {
		t.Fatalf("ждали Raw [a2], записано %v", r.acts)
	}
}

// R5: enter on an empty input closes it without answering.
func TestTrackCardTypingEmptyEnter(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Key("enter", "")
	steps(m, 2)
	if n := len(recorded[Answers](r)); n != 0 {
		t.Fatalf("пустой enter ответил: %v", r.acts)
	}
	m.Key("1", "1")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"1"}) {
		t.Fatalf("ждали Raw [1], записано %v", r.acts)
	}
}

// R6: a paste lands in the open input.
func TestTrackCardTypingPaste(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Paste("  тише\nи мягче ")
	m.Key("enter", "")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"тише и мягче"}) {
		t.Fatalf("ждали Raw [тише и мягче], записано %v", r.acts)
	}
}

// R7: two cards in a row collect their answers in order.
func TestTrackCardTwoCardsNone(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0), trackQ(1)})
	steps(m, 1.5)
	m.Key("esc", "")
	for i := 0; i < 300 && m.qi == 0; i++ {
		m.Step(eventDT)
	}
	if m.qi != 1 {
		t.Fatalf("вторая карточка не открылась: qi %d", m.qi)
	}
	steps(m, 1)
	m.Key("1", "1")
	ans := untilAnswers(t, m, r)
	if !eqStrings(ans[len(ans)-1].Raw, []string{"none", "1"}) {
		t.Fatalf("ждали Raw [none 1], записано %v", r.acts)
	}
}

// R8: the hint names both keys; typing replaces it with the input line.
func TestTrackCardHint(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	askCard(t, m, []Question{trackQ(0)})
	steps(m, 1.5)
	frame := stripANSI(m.View())
	for _, s := range []string{"/ своё", "esc мимо"} {
		if !strings.Contains(frame, s) {
			t.Fatalf("кадр без %q:\n%s", s, frame)
		}
	}
	m.Key("/", "/")
	m.Key("", "тише")
	frame = stripANSI(m.View())
	if !strings.Contains(frame, "› тише") {
		t.Fatalf("кадр без «› тише»:\n%s", frame)
	}
	if strings.Contains(frame, "esc мимо") {
		t.Fatalf("подсказка осталась во время ввода:\n%s", frame)
	}
}

// R9: "none of these" leaves no option chosen, the next card starts clean.
func TestTrackCardNoneNoChosen(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0), trackQ(1)})
	steps(m, 1.5)
	m.Key("/", "/")
	m.Key("esc", "")
	m.Key("esc", "")
	steps(m, 0.1)
	if !m.ansFree {
		t.Fatalf("мимо не выставил ansFree")
	}
	for i := 0; i < 300 && m.qi == 0; i++ {
		m.Step(eventDT)
	}
	if m.qi != 1 || m.ansFree || m.cardTyping {
		t.Fatalf("вторая карточка: qi %d ansFree %v cardTyping %v", m.qi, m.ansFree, m.cardTyping)
	}
}

// R1: esc right after the card appears does nothing, like enter.
func TestTrackCardEscTooEarly(t *testing.T) {
	r := &recorder{}
	m := New(Options{OnAction: r.on})
	askCard(t, m, []Question{trackQ(0)})
	m.Key("esc", "")
	steps(m, 2)
	if n := len(recorded[Answers](r)); n != 0 {
		t.Fatalf("esc до 0.7с ответил: %v", r.acts)
	}
}
