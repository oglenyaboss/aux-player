package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
)

// tasteModel is an English model over an isolated home, so the memory and
// signals files the settings edit land in the test's own config directory.
func tasteModel(t *testing.T) *Model {
	t.Helper()
	isolatedHome(t)
	return newEnglishModel(t)
}

// seedMemory adds entries to the isolated home's memory file.
func seedMemory(t *testing.T, texts ...string) {
	t.Helper()
	st, err := memory.Open(vibeMemoryPath(), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	for _, text := range texts {
		if err := st.Add(text, "seeded by the test"); err != nil {
			t.Fatalf("seeding %q: %v", text, err)
		}
	}
}

// seedRating lands one rating in the isolated home's signals file.
func seedRating(t *testing.T, id, artist, title, kind string) {
	t.Helper()
	sig, err := vibeai.OpenSignals(vibeSignalsPath())
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if err := sig.SetRating(id, artist, title, kind); err != nil {
		t.Fatalf("seeding %s: %v", title, err)
	}
}

// diskMemory reads the memory file back for the assertions.
func diskMemory(t *testing.T) []memory.Entry {
	t.Helper()
	st, err := memory.Open(vibeMemoryPath(), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	return st.Entries()
}

// diskRated reads the signals file back for the assertions.
func diskRated(t *testing.T) []stage.RatedSong {
	t.Helper()
	sig, err := vibeai.OpenSignals(vibeSignalsPath())
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	rated := sig.Rated()
	out := make([]stage.RatedSong, len(rated))
	for i, rt := range rated {
		out[i] = stage.RatedSong{ID: rt.ID, Artist: rt.Artist, Title: rt.Title, Kind: rt.Kind}
	}
	return out
}

// tasteFlush runs every command the screen collected and feeds each message back
// into the model, the way the program loop would.
func tasteFlush(t *testing.T, m *Model) []tea.Msg {
	t.Helper()
	msgs := stageMsgs(t, m.vibeAIP.m.flush())
	if len(msgs) == 0 {
		t.Fatal("the taste action produced no message")
	}
	for _, msg := range msgs {
		m.Update(msg)
	}
	return msgs
}

// tasteAct pushes one taste action through the screen and answers its message.
func tasteAct[T any](t *testing.T, m *Model, a stage.Action) T {
	t.Helper()
	m.vibeAIP.m.act(a)
	msgs := tasteFlush(t, m)
	if len(msgs) != 1 {
		t.Fatalf("action %T produced %#v, want one message", a, msgs)
	}
	msg, ok := msgs[0].(T)
	if !ok {
		t.Fatalf("action %T answered %T, want %T", a, msgs[0], msg)
	}
	return msg
}

// openTasteRow opens the settings, moves onto the nth row and presses enter,
// running the load command the enter produced.
func openTasteRow(t *testing.T, m *Model, row int) []tea.Msg {
	t.Helper()
	s := m.vibeAIP.m
	s.st.OpenSettings(m.settingsInfo())
	for range row {
		s.st.Key("down", "")
	}
	s.st.Key("enter", "")
	return tasteFlush(t, m)
}

// ratedKind reports the standing of one title in a list, "" when absent.
func ratedKind(songs []stage.RatedSong, title string) string {
	for _, sg := range songs {
		if sg.Title == title {
			return sg.Kind
		}
	}
	return ""
}

// R1: settingsInfo counts the memory lines and the rated tracks beside the
// config, zero when the files do not exist yet.
func TestSettingsInfoTasteCounts(t *testing.T) {
	m := tasteModel(t)
	info := m.settingsInfo()
	if info.MemoryCount != 0 || info.RatedCount != 0 {
		t.Fatalf("counts = %d/%d, want 0/0 with no taste files", info.MemoryCount, info.RatedCount)
	}
	seedMemory(t, "likes ambient", "no morning shows")
	seedRating(t, "1", "A", "One", "more")
	seedRating(t, "2", "B", "Two", "less")
	info = m.settingsInfo()
	if info.MemoryCount != 2 || info.RatedCount != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", info.MemoryCount, info.RatedCount)
	}
}

// R2: the memory actions round-trip through the screen, the command and the
// model, persisting to vibe-memory.md and keeping the evidence on edit.
func TestTasteMemoryRoundTrip(t *testing.T) {
	m := tasteModel(t)
	s := m.vibeAIP.m
	seedMemory(t, "likes jazz")

	msgs := openTasteRow(t, m, 9)
	load, ok := msgs[0].(stageMemoryMsg)
	if !ok {
		t.Fatalf("the memory row answered %T, want stageMemoryMsg", msgs[0])
	}
	if load.err != "" || len(load.entries) != 1 || load.entries[0].Text != "likes jazz" {
		t.Fatalf("load = %+v, want the seeded entry without error", load)
	}
	if frame := ansi.Strip(s.Frame(100, 40)); !strings.Contains(frame, "likes jazz") {
		t.Fatalf("the memory sub-screen lacks the loaded entry:\n%s", frame)
	}

	add := tasteAct[stageMemoryMsg](t, m, stage.MemoryAdd{Text: "likes ambient"})
	if add.err != "" || len(add.entries) != 2 {
		t.Fatalf("add = %+v, want two entries without error", add)
	}
	if entries := diskMemory(t); len(entries) != 2 || entries[1].Text != "likes ambient" {
		t.Fatalf("disk entries = %+v, want likes ambient appended", entries)
	}

	edit := tasteAct[stageMemoryMsg](t, m, stage.MemoryEdit{Index: 0, Text: "likes dark ambient"})
	if edit.err != "" || len(edit.entries) != 2 {
		t.Fatalf("edit = %+v, want two entries without error", edit)
	}
	entries := diskMemory(t)
	if entries[0].Text != "likes dark ambient" {
		t.Fatalf("disk entry 0 = %+v, want the edited text", entries[0])
	}
	if entries[0].Evidence != "seeded by the test" {
		t.Fatalf("disk evidence = %q, want the original preserved", entries[0].Evidence)
	}

	del := tasteAct[stageMemoryMsg](t, m, stage.MemoryDelete{Index: 0})
	if del.err != "" || len(del.entries) != 1 || del.entries[0].Text != "likes ambient" {
		t.Fatalf("delete = %+v, want likes ambient left", del)
	}
	if entries = diskMemory(t); len(entries) != 1 || entries[0].Text != "likes ambient" {
		t.Fatalf("disk entries = %+v, want likes ambient left", entries)
	}
}

// R5: a memory line that would corrupt the file is rejected and changes nothing.
func TestTasteMemoryRejectsDelimiter(t *testing.T) {
	m := tasteModel(t)
	seedMemory(t, "likes jazz")
	before, err := os.ReadFile(vibeMemoryPath())
	if err != nil {
		t.Fatalf("reading the memory file: %v", err)
	}
	bad := tasteAct[stageMemoryMsg](t, m, stage.MemoryAdd{Text: "bad\n§\nline"})
	if bad.err == "" {
		t.Fatal("the § line was accepted")
	}
	after, err := os.ReadFile(vibeMemoryPath())
	if err != nil {
		t.Fatalf("reading the memory file: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("the rejected add changed the file:\n%s", after)
	}
}

// R3: the ratings actions round-trip through the screen, the command and the
// model, persisting to vibe-signals.jsonl.
func TestTasteRatedRoundTrip(t *testing.T) {
	m := tasteModel(t)
	s := m.vibeAIP.m
	seedRating(t, "1", "A", "One", "more")
	seedRating(t, "2", "B", "Two", "less")

	msgs := openTasteRow(t, m, 10)
	load, ok := msgs[0].(stageRatedMsg)
	if !ok {
		t.Fatalf("the ratings row answered %T, want stageRatedMsg", msgs[0])
	}
	if load.err != "" || ratedKind(load.songs, "One") != "more" || ratedKind(load.songs, "Two") != "less" {
		t.Fatalf("load = %+v, want One more and Two less", load)
	}
	if frame := ansi.Strip(s.Frame(100, 40)); !strings.Contains(frame, "A — One") {
		t.Fatalf("the ratings sub-screen lacks the loaded track:\n%s", frame)
	}

	flip := tasteAct[stageRatedMsg](t, m, stage.RatedSet{ID: "1", Artist: "A", Title: "One", Kind: "less"})
	if flip.err != "" || ratedKind(flip.songs, "One") != "less" {
		t.Fatalf("flip = %+v, want One less without error", flip)
	}
	if kind := ratedKind(diskRated(t), "One"); kind != "less" {
		t.Fatalf("disk rating of One = %q, want less", kind)
	}

	clear := tasteAct[stageRatedMsg](t, m, stage.RatedSet{ID: "1", Artist: "A", Title: "One", Kind: "rate_clear"})
	if clear.err != "" || ratedKind(clear.songs, "One") != "" || len(clear.songs) != 1 {
		t.Fatalf("clear = %+v, want One gone and one song left", clear)
	}
	if rated := diskRated(t); len(rated) != 1 || rated[0].Title != "Two" {
		t.Fatalf("disk rated = %+v, want only Two", rated)
	}
}

// R5: invalid taste changes answer with an error and leave the files alone.
func TestTasteValidation(t *testing.T) {
	m := tasteModel(t)
	seedMemory(t, "likes jazz")
	if msg := tasteAct[stageMemoryMsg](t, m, stage.MemoryEdit{Index: 5, Text: "x"}); msg.err == "" {
		t.Fatal("an out-of-range edit was accepted")
	}
	if msg := tasteAct[stageMemoryMsg](t, m, stage.MemoryDelete{Index: -1}); msg.err == "" {
		t.Fatal("an out-of-range delete was accepted")
	}
	if msg := tasteAct[stageMemoryMsg](t, m, stage.MemoryAdd{Text: "likes jazz again"}); msg.err == "" {
		t.Fatal("a duplicate add was accepted")
	}
	if msg := tasteAct[stageRatedMsg](t, m, stage.RatedSet{ID: "1", Artist: "A", Title: "One", Kind: "nope"}); msg.err == "" {
		t.Fatal("an unknown rating kind was accepted")
	}
	if entries := diskMemory(t); len(entries) != 1 {
		t.Fatalf("disk entries = %+v, want the seed untouched", entries)
	}
}

// R4: the taste commands take the agent run's mutex for their file work.
func TestTasteCmdsTakeRunMutex(t *testing.T) {
	m := tasteModel(t)
	seedMemory(t, "likes jazz")
	seedRating(t, "1", "A", "One", "more")
	cases := []struct {
		name string
		cmd  tea.Cmd
	}{
		{"loadMemory", m.loadMemoryCmd()},
		{"mutateMemory", m.mutateMemoryCmd(func(*memory.Store) error { return nil })},
		{"loadRated", m.loadRatedCmd()},
		{"setRated", m.setRatedCmd(stage.RatedSet{ID: "1", Artist: "A", Title: "One", Kind: "less"})},
	}
	r := m.vibeAI
	for _, tc := range cases {
		r.mu.Lock()
		done := make(chan struct{})
		go func(cmd tea.Cmd) {
			defer close(done)
			cmd()
		}(tc.cmd)
		select {
		case <-done:
			r.mu.Unlock()
			t.Fatalf("%s finished while the run mutex was held", tc.name)
		case <-time.After(50 * time.Millisecond):
		}
		r.mu.Unlock()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s never finished after the release", tc.name)
		}
	}
}
