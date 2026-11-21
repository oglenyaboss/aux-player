package stage

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// memSetup opens the settings on the memory row and enters the memory sub-screen.
func memSetup(t *testing.T) (*Model, *recorder) {
	t.Helper()
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 9)
	m.Key("enter", "")
	return m, r
}

// ratedSetup opens the settings on the ratings row and enters the ratings sub-screen.
func ratedSetup(t *testing.T) (*Model, *recorder) {
	t.Helper()
	m, r := settingsSetup(t)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 10)
	m.Key("enter", "")
	return m, r
}

// T1: the taste group lists the memory and ratings rows with their counts, and enter
// on them asks for the data.
func TestSettingsTasteRows(t *testing.T) {
	m, r := settingsSetup(t)
	info := settingsInfo()
	info.MemoryCount, info.RatedCount = 3, 5
	m.OpenSettings(info)
	frame := settingsFrame(m)
	for _, want := range []string{"taste", "memory", "ratings", "3", "5"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the settings frame lacks %q:\n%s", want, frame)
		}
	}
	settingsDown(m, 9)
	m.Key("enter", "")
	if got := recorded[LoadMemory](r); len(got) != 1 {
		t.Fatalf("enter on memory sent %v, want one LoadMemory", r.acts)
	}
	if m.settings.mem == nil || !m.settings.mem.loading {
		t.Fatalf("the memory sub-screen did not open loading: %+v", m.settings.mem)
	}
	m.Key("esc", "")
	settingsDown(m, 10)
	m.Key("enter", "")
	if got := recorded[LoadRated](r); len(got) != 1 {
		t.Fatalf("enter on ratings sent %v, want one LoadRated", r.acts)
	}
}

// T2: the memory sub-screen adds, edits and deletes entries, with armed deletes and
// validation on empty text.
func TestMemUIFlow(t *testing.T) {
	m, r := memSetup(t)
	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}, {Text: "no metal"}}, "")
	frame := settingsFrame(m)
	for _, want := range []string{"likes jazz", "no metal"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the memory frame lacks %q:\n%s", want, frame)
		}
	}

	m.Key("a", "")
	m.Paste("likes bossa")
	m.Key("enter", "")
	adds := recorded[MemoryAdd](r)
	if len(adds) != 1 || adds[0].Text != "likes bossa" {
		t.Fatalf("MemoryAdd = %+v, want likes bossa", adds)
	}

	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}}, "")
	m.Key("enter", "")
	m.Key("backspace", "")
	m.Key("", "!")
	m.Key("enter", "")
	edits := recorded[MemoryEdit](r)
	if len(edits) != 1 || edits[0].Index != 0 || edits[0].Text != "likes jaz!" {
		t.Fatalf("MemoryEdit = %+v, want index 0 of likes jaz!", edits)
	}

	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}, {Text: "no metal"}}, "")
	m.Key("d", "")
	if got := recorded[MemoryDelete](r); len(got) != 0 {
		t.Fatalf("the first d deleted: %v", got)
	}
	if !m.settings.mem.confirm {
		t.Fatal("the first d did not arm the delete")
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "again to delete") {
		t.Fatalf("the frame does not ask to confirm:\n%s", frame)
	}
	m.Key("down", "")
	if m.settings.mem.confirm {
		t.Fatal("down did not disarm the delete")
	}
	m.Key("up", "")
	m.Key("d", "")
	m.Key("enter", "")
	dels := recorded[MemoryDelete](r)
	if len(dels) != 1 || dels[0].Index != 0 {
		t.Fatalf("MemoryDelete = %+v, want index 0", dels)
	}

	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}, {Text: "no metal"}}, "")
	m.Key("d", "")
	m.Key("d", "")
	dels = recorded[MemoryDelete](r)
	if len(dels) != 2 || dels[1].Index != 0 {
		t.Fatalf("MemoryDelete = %+v, want a second index 0", dels)
	}

	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}}, "")
	m.Key("a", "")
	m.Key("enter", "")
	if got := recorded[MemoryAdd](r); len(got) != 1 {
		t.Fatalf("the empty submit sent %v, want nothing new", got)
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "enter text") {
		t.Fatalf("the frame does not ask for text:\n%s", frame)
	}
	m.Key("esc", "")
	if m.settings.mem == nil || m.settings.mem.editing {
		t.Fatalf("esc did not return to the list: %+v", m.settings.mem)
	}
	m.Key("esc", "")
	if m.settings.mem != nil || !m.SettingsOpen() {
		t.Fatal("esc did not return to the settings")
	}
}

// T3: while the memory loads, only esc is taken.
func TestMemUILoading(t *testing.T) {
	m, r := memSetup(t)
	m.Key("a", "")
	m.Key("d", "")
	m.Key("down", "")
	if len(r.acts) != 1 || m.settings.mem == nil || m.settings.mem.editing || m.settings.mem.confirm {
		t.Fatalf("keys while loading broke the sub-screen: %+v %v", m.settings.mem, r.acts)
	}
	m.Key("esc", "")
	if m.settings.mem != nil {
		t.Fatal("esc did not close the loading sub-screen")
	}
}

// T4: d on an empty memory does not arm the delete.
func TestMemDeleteEmptyList(t *testing.T) {
	m, r := memSetup(t)
	m.SetMemory(nil, "")
	m.Key("d", "")
	if m.settings.mem.confirm {
		t.Fatal("d armed the delete on an empty list")
	}
	if len(r.acts) != 1 {
		t.Fatalf("an empty list sent %v, want nothing", r.acts)
	}
}

// T5: the edit buffer stops at 300 runes, counting runes and not bytes.
func TestMemEditCap(t *testing.T) {
	m, r := memSetup(t)
	m.SetMemory(nil, "")
	m.Key("a", "")
	m.Key("", strings.Repeat("н", 200))
	m.Key("", strings.Repeat("н", 200))
	m.Key("", "tail")
	m.Key("enter", "")
	adds := recorded[MemoryAdd](r)
	if len(adds) != 1 || len([]rune(adds[0].Text)) != 300 || strings.Contains(adds[0].Text, "tail") {
		t.Fatalf("MemoryAdd = %d runes, want 300 without the tail", len([]rune(adds[0].Text)))
	}
}

// T6: the ratings sub-screen flips and clears ratings on the two tabs.
func TestRatedUIFlow(t *testing.T) {
	m, r := ratedSetup(t)
	if !m.settings.rated.loading {
		t.Fatal("the ratings sub-screen did not open loading")
	}
	m.Key("tab", "")
	if m.settings.rated.tab != "more" {
		t.Fatal("tab moved while loading")
	}
	songs := []RatedSong{
		{ID: "1", Artist: "A", Title: "T1", Kind: "more"},
		{ID: "2", Artist: "B", Title: "T2", Kind: "more"},
		{ID: "3", Artist: "C", Title: "T3", Kind: "less"},
	}
	m.SetRated(songs, "")
	frame := settingsFrame(m)
	for _, want := range []string{"liked (2)", "disliked (1)", "A — T1", "B — T2"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the ratings frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "C — T3") {
		t.Errorf("the liked tab shows a disliked track:\n%s", frame)
	}
	m.Key("down", "")
	m.Key("enter", "")
	sets := recorded[RatedSet](r)
	if len(sets) != 1 || sets[0] != (RatedSet{ID: "2", Artist: "B", Title: "T2", Kind: "less"}) {
		t.Fatalf("RatedSet = %+v, want B T2 flipped to less", sets)
	}
	m.SetRated(songs, "")
	m.Key("d", "")
	sets = recorded[RatedSet](r)
	if len(sets) != 2 || sets[1] != (RatedSet{ID: "2", Artist: "B", Title: "T2", Kind: "rate_clear"}) {
		t.Fatalf("RatedSet = %+v, want B T2 cleared", sets)
	}
	m.SetRated(songs, "")
	m.Key("tab", "")
	frame = settingsFrame(m)
	if !strings.Contains(frame, "C — T3") {
		t.Errorf("the disliked tab lacks its track:\n%s", frame)
	}
	if strings.Contains(frame, "A — T1") {
		t.Errorf("the disliked tab shows a liked track:\n%s", frame)
	}
	m.Key("space", "")
	sets = recorded[RatedSet](r)
	if len(sets) != 3 || sets[2] != (RatedSet{ID: "3", Artist: "C", Title: "T3", Kind: "more"}) {
		t.Fatalf("RatedSet = %+v, want C T3 flipped to more", sets)
	}
	m.Key("esc", "")
	if m.settings.rated != nil || !m.SettingsOpen() {
		t.Fatal("esc did not return to the settings")
	}
}

// T7: an empty tab sends nothing and says so, the other tab still lists its track.
func TestRatedUIEmptyTab(t *testing.T) {
	m, r := ratedSetup(t)
	m.SetRated([]RatedSong{{ID: "3", Artist: "C", Title: "T3", Kind: "less"}}, "")
	m.Key("enter", "")
	m.Key("d", "")
	m.Key("down", "")
	if len(r.acts) != 1 {
		t.Fatalf("an empty tab sent %v, want nothing", r.acts)
	}
	if frame := settingsFrame(m); !strings.Contains(frame, "empty") {
		t.Errorf("the frame does not say empty:\n%s", frame)
	}
	m.Key("tab", "")
	if frame := settingsFrame(m); strings.Contains(frame, "empty") {
		t.Errorf("the other tab still says empty:\n%s", frame)
	}
}

// T8: an error keeps the old memory list and the count, and clears the wait.
func TestSetMemoryError(t *testing.T) {
	m, _ := memSetup(t)
	m.SetMemory([]MemoryEntry{{Text: "likes jazz"}}, "")
	m.SetMemory(nil, "memory did not answer")
	if m.settings.info.MemoryCount != 1 {
		t.Fatalf("MemoryCount = %d, want 1", m.settings.info.MemoryCount)
	}
	frame := settingsFrame(m)
	if !strings.Contains(frame, "likes jazz") || !strings.Contains(frame, "memory did not answer") {
		t.Errorf("the error frame lost the entries or the error:\n%s", frame)
	}
	if m.settings.mem.loading {
		t.Error("the error left the sub-screen loading")
	}
}

// T9: an error keeps the old ratings list and the count.
func TestSetRatedError(t *testing.T) {
	m, _ := ratedSetup(t)
	m.SetRated([]RatedSong{{ID: "1", Artist: "A", Title: "T1", Kind: "more"}}, "")
	m.SetRated(nil, "ratings did not answer")
	if m.settings.info.RatedCount != 1 {
		t.Fatalf("RatedCount = %d, want 1", m.settings.info.RatedCount)
	}
	frame := settingsFrame(m)
	if !strings.Contains(frame, "A — T1") || !strings.Contains(frame, "ratings did not answer") {
		t.Errorf("the error frame lost the tracks or the error:\n%s", frame)
	}
}

// T10: the answers are safe without the overlay or the sub-screen, and a refresh of the
// settings keeps an open sub-screen.
func TestSetTasteClosed(t *testing.T) {
	m, _ := settingsSetup(t)
	m.SetMemory([]MemoryEntry{{Text: "x"}}, "")
	m.SetRated([]RatedSong{{ID: "1", Kind: "more"}}, "")
	if m.SettingsOpen() {
		t.Fatal("SetMemory opened the overlay")
	}
	m.OpenSettings(settingsInfo())
	m.SetMemory([]MemoryEntry{{Text: "x"}}, "")
	m.SetRated([]RatedSong{{ID: "1", Kind: "more"}}, "")
	if m.settings.info.MemoryCount != 1 || m.settings.info.RatedCount != 1 {
		t.Fatalf("counts = %d/%d, want 1/1", m.settings.info.MemoryCount, m.settings.info.RatedCount)
	}
	if m.settings.mem != nil || m.settings.rated != nil {
		t.Fatal("the answers opened a sub-screen")
	}
	m.SetMemory(nil, "boom")
	m.SetRated(nil, "boom")

	m2, _ := memSetup(t)
	m2.SetMemory([]MemoryEntry{{Text: "x"}}, "")
	m2.OpenSettings(settingsInfo())
	if m2.settings.mem == nil {
		t.Fatal("the refresh closed the memory sub-screen")
	}
}

// T12: the editor wraps its buffer at word boundaries and maps the cursor onto the
// wrapped lines.
func TestWrapEdit(t *testing.T) {
	cases := []struct {
		text string
		w    int
		want []editLine
	}{
		{"", 10, []editLine{{0, 0}}},
		{"hello world", 10, []editLine{{0, 6}, {6, 11}}},
		{"abcdefghij", 10, []editLine{{0, 10}, {10, 10}}},
		{"abcdefghijkl", 5, []editLine{{0, 5}, {5, 10}, {10, 12}}},
		{"a   b", 10, []editLine{{0, 5}}},
	}
	for _, c := range cases {
		if got := wrapEdit([]rune(c.text), c.w); !slices.Equal(got, c.want) {
			t.Errorf("wrapEdit(%q, %d) = %v, want %v", c.text, c.w, got, c.want)
		}
	}
	if row, col := editCursorPos(wrapEdit([]rune(""), 10), 0); row != 0 || col != 0 {
		t.Errorf("editCursorPos(empty, 0) = (%d, %d), want (0, 0)", row, col)
	}
	lines := wrapEdit([]rune("abcdefghij"), 10)
	if row, col := editCursorPos(lines, 10); row != 1 || col != 0 {
		t.Errorf("editCursorPos(full last line, 10) = (%d, %d), want (1, 0)", row, col)
	}
	if row, col := editCursorPos(lines, 9); row != 0 || col != 9 {
		t.Errorf("editCursorPos(full last line, 9) = (%d, %d), want (0, 9)", row, col)
	}
}

// T13: the editor moves its cursor and edits in place at it, word by word included.
func TestMemEditCursorAndOps(t *testing.T) {
	m, r := memSetup(t)
	lastEdit := func() MemoryEdit {
		t.Helper()
		edits := recorded[MemoryEdit](r)
		if len(edits) == 0 {
			t.Fatal("no MemoryEdit recorded")
		}
		return edits[len(edits)-1]
	}

	m.SetMemory([]MemoryEntry{{Text: "hello world"}}, "")
	m.Key("enter", "")
	for range 6 {
		m.Key("left", "")
	}
	if s := m.settings.mem; s.cur != 5 {
		t.Fatalf("after six lefts cur = %d, want 5", s.cur)
	}
	m.Key("", ",")
	m.Key("enter", "")
	if e := lastEdit(); e.Index != 0 || e.Text != "hello, world" {
		t.Fatalf("MemoryEdit = %+v, want hello, world", e)
	}

	m.SetMemory([]MemoryEntry{{Text: "hello world"}}, "")
	m.Key("enter", "")
	m.Key("home", "")
	m.Key("", "! ")
	m.Key("end", "")
	m.Key("", "?")
	m.Key("enter", "")
	if e := lastEdit(); e.Text != "! hello world?" {
		t.Fatalf("home/end edit = %q, want ! hello world?", e.Text)
	}

	m.SetMemory([]MemoryEntry{{Text: "hello world"}}, "")
	m.Key("enter", "")
	m.Key("ctrl+a", "")
	m.Key("", "[")
	m.Key("ctrl+e", "")
	m.Key("", "]")
	m.Key("enter", "")
	if e := lastEdit(); e.Text != "[hello world]" {
		t.Fatalf("ctrl+a/ctrl+e edit = %q, want [hello world]", e.Text)
	}

	m.SetMemory([]MemoryEntry{{Text: "hello world"}}, "")
	m.Key("enter", "")
	m.Key("home", "")
	m.Key("delete", "")
	m.Key("ctrl+d", "")
	if s := m.settings.mem; string(s.buf) != "llo world" || s.cur != 0 {
		t.Fatalf("after deletes buf = %q cur = %d, want llo world at 0", string(s.buf), s.cur)
	}

	m.SetMemory([]MemoryEntry{{Text: "любит джаз и соул"}}, "")
	m.Key("enter", "")
	m.Key("ctrl+w", "")
	if s := m.settings.mem; string(s.buf) != "любит джаз и " || s.cur != 13 {
		t.Fatalf("after ctrl+w buf = %q cur = %d", string(s.buf), s.cur)
	}
	m.Key("alt+backspace", "")
	if s := m.settings.mem; string(s.buf) != "любит джаз " || s.cur != 11 {
		t.Fatalf("after alt+backspace buf = %q cur = %d", string(s.buf), s.cur)
	}

	m.SetMemory([]MemoryEntry{{Text: "любит джаз и соул"}}, "")
	m.Key("enter", "")
	s := m.settings.mem
	jump := func(k string, want int) {
		t.Helper()
		m.Key(k, "")
		if s.cur != want {
			t.Fatalf("after %s cur = %d, want %d", k, s.cur, want)
		}
	}
	jump("alt+left", 13)
	jump("ctrl+left", 11)
	jump("alt+b", 6)
	jump("alt+right", 10)
	jump("ctrl+right", 12)
	jump("alt+f", 17)
	m.Key("", "!")
	m.Key("enter", "")
	if e := lastEdit(); e.Text != "любит джаз и соул!" {
		t.Fatalf("word-jump edit = %q, want любит джаз и соул!", e.Text)
	}

	m.SetMemory([]MemoryEntry{{Text: "hello world"}}, "")
	m.Key("enter", "")
	m.Key("ctrl+a", "")
	for range 5 {
		m.Key("right", "")
	}
	m.Key("ctrl+u", "")
	if s := m.settings.mem; string(s.buf) != " world" || s.cur != 0 {
		t.Fatalf("after ctrl+u buf = %q cur = %d", string(s.buf), s.cur)
	}
	m.Key("right", "")
	m.Key("right", "")
	m.Key("ctrl+k", "")
	if s := m.settings.mem; string(s.buf) != " w" || s.cur != 2 {
		t.Fatalf("after ctrl+k buf = %q cur = %d", string(s.buf), s.cur)
	}

	m.SetMemory([]MemoryEntry{{Text: strings.Repeat("a", 295)}}, "")
	m.Key("enter", "")
	m.Key("home", "")
	m.Paste(strings.Repeat("b", 10))
	if s := m.settings.mem; len(s.buf) != 300 || s.cur != 5 || string(s.buf[:5]) != "bbbbb" || string(s.buf[5:]) != strings.Repeat("a", 295) {
		t.Fatalf("cap insert: len = %d cur = %d head = %q", len(s.buf), s.cur, string(s.buf[:min(10, len(s.buf))]))
	}
}

// T14: long entries wrap across the frame and up/down move between the wrapped lines.
func TestMemEditUpDownAndWrapRender(t *testing.T) {
	m, r := memSetup(t)
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "w%03d ", i)
	}
	m.SetMemory([]MemoryEntry{{Text: b.String()}}, "")
	m.Key("enter", "")
	frame := settingsFrame(m)
	if !strings.Contains(frame, "w000") || !strings.Contains(frame, "w029") {
		t.Errorf("the wrapped frame lost the start or the end of the entry:\n%s", frame)
	}
	if !strings.Contains(frame, "150/300") {
		t.Errorf("the frame lacks the 150/300 counter:\n%s", frame)
	}

	m.Key("up", "")
	if s := m.settings.mem; s.cur >= 130 {
		t.Fatalf("up from the last line kept cur = %d, want a line above", s.cur)
	}
	m.Key("", "X")
	m.Key("down", "")
	m.Key("", "Y")
	m.Key("enter", "")
	edits := recorded[MemoryEdit](r)
	if len(edits) != 1 {
		t.Fatalf("MemoryEdit = %+v, want one", edits)
	}
	x := strings.Index(edits[0].Text, "X")
	y := strings.Index(edits[0].Text, "Y")
	if x < 65 || x >= 130 {
		t.Errorf("X landed at %d, want on the middle wrapped line [65,130)", x)
	}
	if y < 130 {
		t.Errorf("Y landed at %d, want on the last wrapped line", y)
	}
}

// T15: the narrow screen scrolls the wrapped entry to keep the cursor visible.
func TestMemEditScrollNarrow(t *testing.T) {
	m, _ := settingsSetup(t)
	m.SetSize(40, 15)
	m.OpenSettings(settingsInfo())
	settingsDown(m, 10)
	m.Key("up", "")
	m.Key("enter", "")
	var b strings.Builder
	for i := range 52 {
		fmt.Fprintf(&b, "w%03d ", i)
	}
	m.SetMemory([]MemoryEntry{{Text: b.String()}}, "")
	m.Key("enter", "")

	m.Key("home", "")
	top := settingsFrame(m)
	if s := m.settings.mem; s.scroll != 0 {
		t.Fatalf("scroll at the top = %d, want 0", s.scroll)
	}
	if !strings.Contains(top, "w000") {
		t.Errorf("the top of the entry left the frame:\n%s", top)
	}

	m.Key("end", "")
	bottom := settingsFrame(m)
	if s := m.settings.mem; s.scroll <= 0 {
		t.Fatalf("scroll at the end = %d, want past the top", s.scroll)
	}
	if !strings.Contains(bottom, "w051") {
		t.Errorf("the end of the entry left the frame:\n%s", bottom)
	}
	if strings.Contains(bottom, "w000") {
		t.Errorf("the top stayed visible after scrolling to the end:\n%s", bottom)
	}
}

// T11: both sub-screens render at every size and language without panicking.
func TestTasteRenderSmoke(t *testing.T) {
	long := strings.Repeat("x", 280)
	for _, sz := range [][2]int{{100, 35}, {40, 15}} {
		for _, l := range []string{"en", "ru"} {
			m, _ := settingsSetup(t)
			SetLang(l)
			m.SetSize(sz[0], sz[1])
			m.OpenSettings(settingsInfo())
			downs := 9
			taste := "taste"
			if l == "ru" {
				taste = "вкус"
			}
			if sz[0] < 80 {
				// the short screen scrolls the group in with the selection
				downs, taste = 10, "memory"
				if l == "ru" {
					taste = "память"
				}
			}
			settingsDown(m, downs)
			if frame := settingsFrame(m); !strings.Contains(frame, taste) {
				t.Errorf("%dx%d %s: the frame lacks the taste group:\n%s", sz[0], sz[1], l, frame)
			}
			if sz[0] < 80 {
				m.Key("up", "")
			}
			m.Key("enter", "")
			m.SetMemory(nil, "")
			m.SetMemory([]MemoryEntry{{Text: long}, {Text: "b"}}, "")
			m.Key("d", "")
			m.Key("enter", "")
			m.SetMemory([]MemoryEntry{{Text: long}}, "")
			m.Key("a", "")
			m.Key("", strings.Repeat("y", 350))
			_ = settingsFrame(m)
			m.Key("esc", "")
			m.Key("esc", "")
			settingsDown(m, 1)
			m.Key("enter", "")
			liked := "liked"
			if l == "ru" {
				liked = "нравится"
			}
			m.SetRated(nil, "")
			if frame := settingsFrame(m); !strings.Contains(frame, liked) {
				t.Errorf("%dx%d %s: the ratings frame lacks its tab:\n%s", sz[0], sz[1], l, frame)
			}
			m.SetRated([]RatedSong{{ID: "1", Artist: "A", Title: "T", Kind: "less"}}, "")
			m.SetRated(nil, "boom")
			m.Key("tab", "")
			_ = settingsFrame(m)
		}
	}
}
