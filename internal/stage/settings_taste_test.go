package stage

import (
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
