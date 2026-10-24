package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/simone-vibes/vibez/internal/config"
	"github.com/simone-vibes/vibez/internal/player"
	"github.com/simone-vibes/vibez/internal/provider"
	"github.com/simone-vibes/vibez/internal/tui/views"
	"github.com/simone-vibes/vibez/internal/vibeai"
)

func press(m *Model, k string) tea.Cmd {
	r := []rune(k)
	msg := tea.KeyPressMsg{Code: r[0], Text: k}
	switch k {
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEsc}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return m.handleNormalKey(msg, msg.String())
}

func vibeTracks() []vibeai.Track {
	return []vibeai.Track{
		{ID: "1", Artist: "A", Title: "One", ArtworkURL: "https://x/{w}x{h}bb.jpg", DurationMs: 61000},
		{ID: "2", Artist: "B", Title: "Two", Favorite: true},
		{ID: "3", Artist: "C", Title: "Three"},
	}
}

func openVibeWithPortion(t *testing.T) *Model {
	t.Helper()
	m := newModel(nil)
	press(m, "v")
	vibeAITypeText(m, "первый запрос")
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks(), elapsed: time.Second})
	return m
}

func startVibeTurn(m *Model) int {
	m.vibeAITurnCmd("", false, false)
	return m.vibeAI.gen
}

func acceptPick(m *Model, gen int, id, artist, title string) {
	m.handleVibeAIMsg(vibeAIPickMsg{gen: gen, pick: vibeai.Pick{
		Track:    vibeai.Track{ID: id, Artist: artist, Title: title},
		Name:     artist + " — " + title,
		Accepted: true,
	}})
}

// vibeAITypeText types a request and submits it, driving the panel through its
// real intro→feed flow.
func vibeAITypeText(m *Model, text string) {
	for _, k := range text {
		press(m, string(k))
	}
	if cmd := press(m, "enter"); cmd != nil {
		m.handleVibeAIMsg(cmd())
	}
}

func TestVibeAIConfigSignalsPath(t *testing.T) {
	cfg := vibeAIConfig(&config.Config{})
	if filepath.Base(cfg.SignalsPath) != "vibe-signals.jsonl" {
		t.Errorf("SignalsPath = %q, want …/vibe-signals.jsonl", cfg.SignalsPath)
	}
	if filepath.Dir(cfg.SignalsPath) != filepath.Dir(cfg.MemoryPath) {
		t.Errorf("SignalsPath dir %q != MemoryPath dir %q", filepath.Dir(cfg.SignalsPath), filepath.Dir(cfg.MemoryPath))
	}
}

func TestVibeAI_TypingDoesNotTriggerGlobals(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	for _, k := range []string{"q", "l", "v", "/", ":"} {
		press(m, k)
	}
	if !m.vibeAIActive() || m.mode != modeNormal {
		t.Fatalf("typing in the vibe input leaked to globals: active=%v mode=%v", m.vibeAIActive(), m.mode)
	}
	press(m, "esc")
	if m.vibeAIActive() {
		t.Error("esc on the empty prompt should close the vibe screen")
	}
}

func TestVibeAI_SubmitStartsFreshTurn(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	for _, k := range []string{"j", "a", "z", "z"} {
		press(m, k)
	}
	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter should submit the request")
	}
	sub, ok := cmd().(views.VibeAISubmitMsg)
	if !ok || sub.Text != "jazz" || !sub.Fresh {
		t.Fatalf("got %#v, want fresh submit of %q", sub, "jazz")
	}
	m.handleVibeAIMsg(sub)
	if !m.vibeAIP.m.Busy() {
		t.Error("the screen should switch to working")
	}
}

func TestVibeAI_StaleDoneIgnored(t *testing.T) {
	m := newModel(nil)
	m.vibeAI.gen = 2
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: 1, tracks: vibeTracks()})
	if len(m.vibeAI.tracks) != 0 {
		t.Error("a cancelled turn's result must not land")
	}
}

func TestVibeAI_StreamOnByDefault(t *testing.T) {
	m := newModel(nil)
	if !m.vibeAI.stream {
		t.Error("stream mode должен быть включён по умолчанию")
	}
	press(m, "v")
	vibeAITypeText(m, "q")
	m.vibeAIP.m.SetSize(120, 30)
	if !strings.Contains(m.vibeAIP.m.View(), "● поток") {
		t.Error("экран должен показывать включённый поток")
	}
}

func TestVibeAI_AutoplayFromFirstPick(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAITurnCmd("что-нибудь", true, false)
	gen := m.vibeAI.gen
	acceptPick(m, gen, "1", "A", "One")
	if len(m.queueIDs) != 1 || m.queueIDs[0] != "1" {
		t.Fatalf("queue = %v, want auto-start of the first pick", m.queueIDs)
	}
	if m.playerState.Track == nil || m.playerState.Track.ID != "1" {
		t.Fatalf("playing = %+v, want track 1", m.playerState.Track)
	}
	acceptPick(m, gen, "2", "B", "Two")
	if len(m.queueIDs) != 1 || m.queueIDs[0] != "1" {
		t.Errorf("a second autoplayer pick must not touch the queue, got %v", m.queueIDs)
	}
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: gen, tracks: vibeTracks(), elapsed: time.Second})
	if want := []string{"1", "2", "3"}; len(m.queueIDs) != len(want) || !vibeAIEqual(want, m.queueIDs) {
		t.Errorf("queue = %v, want %v", m.queueIDs, want)
	}
	if len(m.vibeAI.feed) != 3 {
		t.Errorf("feed = %d tracks, want 3", len(m.vibeAI.feed))
	}
	if m.vibeAI.autoplay {
		t.Error("autoplay must be consumed by the first portion")
	}
}

func TestVibeAI_AutoplayWithoutPicks(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAITurnCmd("что-нибудь", true, false)
	gen := m.vibeAI.gen
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: gen, tracks: vibeTracks(), elapsed: time.Second})
	if want := []string{"1", "2", "3"}; len(m.queueIDs) != len(want) || !vibeAIEqual(want, m.queueIDs) {
		t.Fatalf("queue = %v, want %v — первый трек должен играть", m.queueIDs, want)
	}
	if m.playerState.Track == nil || m.playerState.Track.ID != "1" {
		t.Errorf("playing = %+v, want track 1", m.playerState.Track)
	}
}

func TestVibeAI_StalePickIgnored(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAITurnCmd("что-нибудь", true, false)
	if cmd := m.handleVibeAIMsg(vibeAIPickMsg{gen: m.vibeAI.gen - 1, pick: vibeai.Pick{
		Track:    vibeai.Track{ID: "1", Artist: "A", Title: "One"},
		Name:     "A — One",
		Accepted: true,
	}}); cmd == nil {
		t.Error("a stale pick must keep reading events, so waitVibeAI is expected")
	}
	if len(m.vibeAI.feed) != 0 {
		t.Errorf("feed = %d tracks, want 0: stale pick must not land", len(m.vibeAI.feed))
	}
	if len(m.queueIDs) != 0 {
		t.Errorf("queue = %v, want empty", m.queueIDs)
	}
}

func TestVibeAI_RejectedPick(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAITurnCmd("что-нибудь", true, false)
	m.handleVibeAIMsg(vibeAIPickMsg{gen: m.vibeAI.gen, pick: vibeai.Pick{Name: "X — Y", Reason: "не нашлось в Apple Music"}})
	if len(m.vibeAI.feed) != 0 {
		t.Errorf("feed = %d tracks, want 0 after a rejected pick", len(m.vibeAI.feed))
	}
}

func TestVibeAILatePickAfterDoneIgnored(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	vibeAITypeText(m, "первый запрос")
	gen := m.vibeAI.gen
	acceptPick(m, gen, "1", "A", "One")
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: gen, tracks: vibeTracks(), elapsed: time.Second})
	if cmd := m.handleVibeAIMsg(vibeAIPickMsg{gen: gen, pick: vibeai.Pick{
		Track:    vibeai.Track{ID: "3", Artist: "C", Title: "Three"},
		Name:     "C — Three",
		Accepted: true,
	}}); cmd == nil {
		t.Fatal("поздний пик должен вернуть waitVibeAI, иначе чтение канала остановится")
	}
	if len(m.vibeAI.feed) != 3 {
		t.Fatalf("feed = %d треков, want 3: поздний пик не должен менять ленту", len(m.vibeAI.feed))
	}
	if want := []string{"1", "2", "3"}; !vibeAIEqual(want, m.queueIDs) {
		t.Fatalf("queue = %v, want %v", m.queueIDs, want)
	}
	m.vibeAIP.m.SetSize(100, 40)
	if n := strings.Count(m.vibeAIP.m.View(), "Three"); n != 1 {
		t.Errorf("Three в ленте %d раз, want 1 (поздний пик дублирует трек)", n)
	}
}

func TestVibeAILateAskAfterDoneIgnored(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	vibeAITypeText(m, "первый запрос")
	gen := m.vibeAI.gen
	acceptPick(m, gen, "1", "A", "One")
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: gen, tracks: vibeTracks(), elapsed: time.Second})
	if cmd := m.handleVibeAIMsg(vibeAIAskMsg{gen: gen, qs: []vibeai.Question{{Question: "ещё?", Options: []string{"да", "нет"}}}}); cmd == nil {
		t.Fatal("поздний вопрос должен вернуть waitVibeAI, иначе чтение канала остановится")
	}
	if m.vibeAIP.m.Capturing() {
		t.Error("поздний вопрос не должен захватывать клавиши")
	}
	m.vibeAIP.m.SetSize(100, 40)
	if strings.Contains(m.vibeAIP.m.View(), "ещё?") {
		t.Error("поздний вопрос не должен попадать в ленту")
	}
}

func TestVibeAI_StreamAppendsWhileOn(t *testing.T) {
	m := openVibeWithPortion(t)
	next := []vibeai.Track{{ID: "4", Artist: "D", Title: "Four"}, {ID: "5", Artist: "E", Title: "Five"}}
	done := func() vibeAIDoneMsg {
		return vibeAIDoneMsg{gen: startVibeTurn(m), tracks: next, elapsed: time.Second}
	}
	m.handleVibeAIMsg(done())
	before := len(m.queueIDs)
	if before != 5 {
		t.Fatalf("поток должен был добавить 2 трека в очередь, got %d", before)
	}
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIStream})
	m.handleVibeAIMsg(done())
	if len(m.queueIDs) != before {
		t.Errorf("очередь = %v, при выключенном потоке она должна оставаться неизменной", m.queueIDs)
	}
}

func TestVibeAI_PlayFromCursor(t *testing.T) {
	m := openVibeWithPortion(t)
	press(m, "j")
	cmd := press(m, "enter")
	act, ok := cmd().(views.VibeAIActionMsg)
	if !ok || act.Action != views.VibeAIPlay || act.Index != 1 {
		t.Fatalf("got %#v, want play from 1", act)
	}
	m.handleVibeAIMsg(act)
	if len(m.queueTracks) != 2 || m.queueIDs[0] != "2" || m.playerState.Track.Title != "Two" {
		t.Fatalf("queue = %v, now playing %v", m.queueIDs, m.playerState.Track)
	}
}

func TestVibeAI_QueueAppends(t *testing.T) {
	m := openVibeWithPortion(t)
	m.queueTracks = []provider.Track{{ID: "x"}}
	m.queueIDs = []string{"x"}
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIQueue})
	if len(m.queueIDs) != 4 || m.queueIDs[3] != "3" {
		t.Fatalf("queue = %v", m.queueIDs)
	}
}

func TestVibeAI_ProviderTracks(t *testing.T) {
	got := vibeAIProviderTracks(vibeTracks())[0]
	if got.CatalogID != "1" || got.ArtworkURL != "https://x/300x300bb.jpg" || got.Duration != 61*time.Second {
		t.Fatalf("got %+v", got)
	}
	if views.PlaybackID(got) != "1" {
		t.Error("playback id should be the catalog id")
	}
}

func TestVibeAI_SaveTwiceSamePortion(t *testing.T) {
	m := openVibeWithPortion(t)
	if m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAISave}) == nil {
		t.Fatal("first save should run")
	}
	if m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAISave}) != nil {
		t.Error("saving the same portion twice should be a no-op")
	}
	m.handleVibeAIMsg(vibeAISavedMsg{id: "p.1", name: "Vibes AI · x", n: 3, created: true})
	m.vibeAITurnCmd("", false, false)
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks(), elapsed: time.Second})
	if m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAISave}) == nil {
		t.Error("a new portion should append to the linked playlist")
	}
}

func TestVibeAI_Outcome(t *testing.T) {
	long := vibeai.Track{ID: "1", Artist: "A", Title: "T", DurationMs: 200000}
	cases := []struct {
		pos  time.Duration
		want vibeai.Outcome
	}{
		{150 * time.Second, vibeai.Outcome{ID: "1", Artist: "A", Title: "T", Kind: "played", PosSec: 150, DurSec: 200}},
		{139 * time.Second, vibeai.Outcome{ID: "1", Artist: "A", Title: "T", Kind: "skip_mid", PosSec: 139, DurSec: 200}},
		{12 * time.Second, vibeai.Outcome{ID: "1", Artist: "A", Title: "T", Kind: "skip_early", PosSec: 12, DurSec: 200}},
	}
	for _, c := range cases {
		if got := vibeAIOutcome(long, c.pos); got != c.want {
			t.Errorf("vibeAIOutcome(pos %s) = %+v, want %+v", c.pos, got, c.want)
		}
	}
	unknown := vibeai.Track{ID: "2", Artist: "B", Title: "U"}
	if got := vibeAIOutcome(unknown, 125*time.Second); got.Kind != "played" {
		t.Errorf("unknown duration, pos 125s: kind = %q, want played", got.Kind)
	}
	if got := vibeAIOutcome(unknown, 60*time.Second); got.Kind != "skip_mid" {
		t.Errorf("unknown duration, pos 60s: kind = %q, want skip_mid", got.Kind)
	}
}

func TestVibeAI_FeedPlayFromIndex(t *testing.T) {
	m := openVibeWithPortion(t)
	next := []vibeai.Track{{ID: "4", Artist: "D", Title: "Four"}, {ID: "5", Artist: "E", Title: "Five"}}
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: startVibeTurn(m), tracks: next, elapsed: time.Second})
	if len(m.vibeAI.feed) != 5 {
		t.Fatalf("feed = %d tracks, want 5", len(m.vibeAI.feed))
	}
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 3})
	if len(m.queueIDs) != 2 || m.queueIDs[0] != "4" || m.queueIDs[1] != "5" {
		t.Fatalf("queue = %v, want [4 5]", m.queueIDs)
	}
}

func TestVibeAI_PlayIndexOutOfBounds(t *testing.T) {
	m := openVibeWithPortion(t)
	n := len(m.queueIDs)
	if n == 0 {
		t.Fatal("fixture broken: queue must hold the portion")
	}
	for _, idx := range []int{-1, n + 99} {
		if cmd := m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: idx}); cmd != nil {
			t.Errorf("play at index %d должен быть no-op, got cmd", idx)
		}
	}
	if len(m.queueIDs) != n {
		t.Errorf("queue = %v, очередь не должна была измениться", m.queueIDs)
	}
}

func TestVibeAINoteLeavePending(t *testing.T) {
	m := openVibeWithPortion(t)
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	t1 := &provider.Track{ID: "1", Title: "One"}
	t2 := &provider.Track{ID: "2", Title: "Two"}
	m.Update(playerStateMsg{Track: t1, Position: 15 * time.Second})
	m.Update(playerStateMsg{Track: t2, Position: 0})
	if len(m.vibeAI.pending) != 1 {
		t.Fatalf("pending = %+v, want one outcome", m.vibeAI.pending)
	}
	o := m.vibeAI.pending[0]
	if o.ID != "1" || o.Kind != "skip_early" || o.PosSec != 15 || o.DurSec != 61 {
		t.Errorf("pending[0] = %+v, want skip_early ID 1 at 15s", o)
	}
	m.Update(playerStateMsg{Track: t1, Position: 500 * time.Millisecond})
	m.Update(playerStateMsg{Track: t2, Position: 0})
	if len(m.vibeAI.pending) != 1 {
		t.Errorf("pos < 1s must be ignored, pending = %+v", m.vibeAI.pending)
	}
	m.Update(playerStateMsg{Track: &provider.Track{ID: "x"}, Position: 30 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "3", Title: "Three"}, Position: 0})
	if len(m.vibeAI.pending) != 1 {
		t.Errorf("non-vibe track must be ignored, pending = %+v", m.vibeAI.pending)
	}
}

func TestVibeAINoteLeaveViaStateMsg(t *testing.T) {
	m := openVibeWithPortion(t)
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	m.Update(playerStateMsg{Track: m.playerState.Track, Position: 40 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2", Title: "Two"}, Position: 0})
	found := false
	for _, o := range m.vibeAI.pending {
		if o.ID == "1" && o.Kind == "skip_mid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pending = %+v, want skip_mid for ID 1", m.vibeAI.pending)
	}
}

func TestVibeAI_TrackStatesInFeed(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	vibeAITypeText(m, "хочу нового")
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks(), elapsed: time.Second})
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	m.Update(playerStateMsg{Track: m.playerState.Track, Position: 15 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2", Title: "Two"}, Position: 0})
	m.vibeAIP.m.SetSize(100, 30)
	view := m.vibeAIP.m.View()
	if !strings.Contains(view, "скип на 0:15") {
		t.Errorf("скип должен быть виден в ленте:\n%s", view)
	}
}
func TestVibeAIOutcomesAcrossNilTrack(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 40 * time.Second})
	m.Update(playerStateMsg{Position: 0})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	want := []vibeai.Outcome{{ID: "1", Artist: "A", Title: "One", Kind: "skip_mid", PosSec: 40, DurSec: 61}}
	if !reflect.DeepEqual(m.vibeAI.pending, want) {
		t.Errorf("pending = %+v, want %+v (один исход с позицией до перехода)", m.vibeAI.pending, want)
	}
}

func TestVibeAIOutcomesSameTrackAtZero(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 40 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 0})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	want := []vibeai.Outcome{{ID: "1", Artist: "A", Title: "One", Kind: "skip_mid", PosSec: 40, DurSec: 61}}
	if !reflect.DeepEqual(m.vibeAI.pending, want) {
		t.Errorf("pending = %+v, want %+v (Position 0 не должен затирать heardPos)", m.vibeAI.pending, want)
	}
}

func TestVibeAIOutcomesRepeatStatesNoDouble(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 15 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 3 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 5 * time.Second})
	want := []vibeai.Outcome{{ID: "1", Artist: "A", Title: "One", Kind: "skip_early", PosSec: 15, DurSec: 61}}
	if !reflect.DeepEqual(m.vibeAI.pending, want) {
		t.Errorf("pending = %+v, want ровно %+v", m.vibeAI.pending, want)
	}
}

func TestVibeAIOutcomesPlayedNoSkipInView(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 60 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	if len(m.vibeAI.pending) == 0 {
		t.Fatal("pending пуст, want записанный исход")
	}
	if m.vibeAI.pending[0].Kind != "played" {
		t.Errorf("pending[0].Kind = %q, want played", m.vibeAI.pending[0].Kind)
	}
	m.vibeAIP.m.SetSize(100, 30)
	view := m.vibeAIP.m.View()
	if strings.Contains(view, "скип на") {
		t.Errorf("дослушанный трек не должен показывать скип:\n%s", view)
	}
}

func TestVibeAIOutcomesSubSecondIgnored(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 500 * time.Millisecond})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	if len(m.vibeAI.pending) != 0 {
		t.Errorf("pending = %+v, want пусто: позиция < 1s не даёт исхода", m.vibeAI.pending)
	}
}

func TestVibeAIOutcomesNonVibeTrackIgnored(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "x"}, Position: 30 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	if len(m.vibeAI.pending) != 0 {
		t.Errorf("pending = %+v, want пусто: трек вне vibe-подборки не даёт исхода", m.vibeAI.pending)
	}
}

func TestVibeAIOutcomesPlayActionNoDouble(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 20 * time.Second})
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 1})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 20 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 0})
	n := 0
	for _, o := range m.vibeAI.pending {
		if o.ID == "1" {
			n++
		}
		if o.ID == "1" && (o.Kind != "skip_early" || o.PosSec != 20) {
			t.Errorf("pending %+v, want skip_early at 20s", o)
		}
	}
	if n != 1 {
		t.Errorf("исходов для трека 1 = %d, want 1 (play не должен давать двойной исход), pending = %+v", n, m.vibeAI.pending)
	}
}

func TestVibeAIOutcomesTwoTransitions(t *testing.T) {
	m := openVibeWithPortion(t)
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1"}, Position: 40 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2"}, Position: 130 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "3"}, Position: 0})
	want := []vibeai.Outcome{
		{ID: "1", Artist: "A", Title: "One", Kind: "skip_mid", PosSec: 40, DurSec: 61},
		{ID: "2", Artist: "B", Title: "Two", Kind: "played", PosSec: 130},
	}
	if !reflect.DeepEqual(m.vibeAI.pending, want) {
		t.Errorf("pending = %+v, want %+v", m.vibeAI.pending, want)
	}
}

func TestVibeAITurnCmdTakesPending(t *testing.T) {
	m := openVibeWithPortion(t)
	m.vibeAI.pending = []vibeai.Outcome{{ID: "1", Kind: "skip_early", PosSec: 15}}
	if m.vibeAITurnCmd("", false, true) == nil {
		t.Fatal("turn cmd expected")
	}
	if len(m.vibeAI.pending) != 0 {
		t.Errorf("pending = %+v, want moved out", m.vibeAI.pending)
	}
}

func TestVibeAI_LaterQuestionAndAnswer(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAITurnCmd("что-нибудь", true, false)
	gen := m.vibeAI.gen
	m.handleVibeAIMsg(vibeAIDoneMsg{
		gen:    gen,
		tracks: vibeTracks(),
		later:  &vibeai.Question{Question: "убрать хоррор?", Options: []string{"да", "нет"}},
	})
	if m.vibeAI.later == nil {
		t.Fatal("отложенный вопрос не попал в состояние")
	}
	m.handleVibeAIMsg(views.VibeAILaterMsg{Raw: "1"})
	ans := m.vibeAI.laterAnswers
	if len(ans) != 1 || ans[0].q.Question != "убрать хоррор?" || ans[0].raw != "1" || m.vibeAI.later != nil {
		t.Fatalf("laterAnswers = %+v, later = %v", ans, m.vibeAI.later)
	}
	next := startVibeTurn(m)
	if len(m.vibeAI.laterAnswers) != 0 {
		t.Errorf("laterAnswers должны уйти в ход и очиститься, got %+v", m.vibeAI.laterAnswers)
	}
	if next != gen+1 || m.vibeAI.gen != next {
		t.Fatalf("turn gen: got %d (model %d), want %d", next, m.vibeAI.gen, gen+1)
	}
}

func TestVibeAI_AbortedTurnTrimsPicks(t *testing.T) {
	make := func(t *testing.T) *Model {
		t.Helper()
		m := newModel(nil)
		press(m, "v")
		m.vibeAITurnCmd("что-нибудь", true, false)
		acceptPick(m, m.vibeAI.gen, "1", "A", "One")
		acceptPick(m, m.vibeAI.gen, "2", "B", "Two")
		if len(m.vibeAI.feed) != 2 {
			t.Fatalf("fixture broken: feed = %d tracks, want 2", len(m.vibeAI.feed))
		}
		return m
	}
	m := make(t)
	m.cancelVibeAI()
	if len(m.vibeAI.feed) != 0 {
		t.Errorf("cancel: feed = %d tracks, want 0", len(m.vibeAI.feed))
	}
	if m.vibeAI.autoplay {
		t.Error("cancel должен выключить автостарт")
	}
	failed := make(t)
	failed.handleVibeAIMsg(vibeAIDoneMsg{gen: failed.vibeAI.gen, err: errors.New("бум")})
	if len(failed.vibeAI.feed) != 0 {
		t.Errorf("error: feed = %d tracks, want 0", len(failed.vibeAI.feed))
	}
	if failed.vibeAI.autoplay {
		t.Error("ошибка хода должна выключить автостарт")
	}
}

func TestVibeAI_StreamRefillAhead(t *testing.T) {
	m := openVibeWithPortion(t)
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	s := player.State{Track: &provider.Track{ID: "1"}}
	if m.vibeAIStreamTick(s) == nil {
		t.Fatal("remaining 2 ≤ 4: stream should top up")
	}
	m.vibeAI.busy = false
	if m.vibeAIStreamTick(s) != nil {
		t.Error("the same track must not trigger a second top-up")
	}
	if m.vibeAIStreamTick(player.State{Track: &provider.Track{ID: "x"}}) != nil {
		t.Error("a non-vibe track must not trigger a top-up")
	}
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: m.vibeAI.gen, tracks: vibeTracks(), refill: true})
	if len(m.queueIDs) != 6 {
		t.Errorf("refill should append to the queue, got %v", m.queueIDs)
	}
}

func TestVibeAI_StreamAheadThreshold(t *testing.T) {
	m := newModel(nil)
	press(m, "v")
	m.vibeAI.gen = 1
	tracks := make([]vibeai.Track, 10)
	for i := range tracks {
		tracks[i] = vibeai.Track{ID: fmt.Sprintf("%d", i+1), Artist: "A", Title: fmt.Sprintf("T%d", i+1)}
	}
	m.handleVibeAIMsg(vibeAIDoneMsg{gen: 1, tracks: tracks, elapsed: time.Second})
	m.queueTracks = vibeAIProviderTracks(tracks)
	m.queueIDs = make([]string, len(m.queueTracks))
	for i, tr := range m.queueTracks {
		m.queueIDs[i] = views.PlaybackID(tr)
	}
	if m.vibeAIStreamTick(player.State{Track: &m.queueTracks[2]}) != nil {
		t.Error("remaining 7 > 4: must not top up")
	}
	if m.vibeAIStreamTick(player.State{Track: &m.queueTracks[6]}) == nil {
		t.Error("remaining 3 ≤ 4: must top up")
	}
}

func TestVibeAIPlaylistName(t *testing.T) {
	if got := vibeAIPlaylistName("  джаз "); got != "Vibes AI · джаз" {
		t.Errorf("got %q", got)
	}
	long := vibeAIPlaylistName("абвгдеёжзийклмнопрстуфхцчшщъыьэюя абвгдеёжз")
	if n := len([]rune(long)); n != len([]rune("Vibes AI · "))+40 {
		t.Errorf("long name has %d runes: %q", n, long)
	}
}

func vibeAIEqual(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}

func TestVibeAICloseWaitsForCancelledTurn(t *testing.T) {
	r := newVibeAIRun()
	r.mu.Lock()
	released := false
	go func() {
		time.Sleep(100 * time.Millisecond)
		released = true
		r.mu.Unlock()
	}()
	r.close()
	if !released {
		t.Fatal("close вернулся, не дождавшись отменённого хода: лог сессии потерян")
	}
}

func ratedIndex(t *testing.T, m *Model, k string) int {
	t.Helper()
	cmd := press(m, k)
	if cmd == nil {
		t.Fatalf("%q produced no cmd", k)
	}
	got := cmd()
	msg, ok := got.(views.VibeAIRateMsg)
	if !ok {
		t.Fatalf("%q: msg = %#v, want views.VibeAIRateMsg", k, got)
	}
	return msg.Index
}

func TestVibeAI_FollowPlayingOnChange(t *testing.T) {
	m := openVibeWithPortion(t)
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "1", Title: "One"}, Position: 20 * time.Second})
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2", Title: "Two"}, Position: 0})
	if idx := ratedIndex(t, m, "]"); idx != 1 {
		t.Fatalf("cursor = %d, want 1 after the playing track changed", idx)
	}
	press(m, "k")
	if idx := ratedIndex(t, m, "]"); idx != 0 {
		t.Fatalf("cursor = %d, want 0 after k", idx)
	}
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2", Title: "Two"}, Position: 5 * time.Second})
	if idx := ratedIndex(t, m, "]"); idx != 0 {
		t.Fatalf("cursor = %d, want 0: a repeated state tick must not move it", idx)
	}
}

func TestVibeAI_RateMsgPending(t *testing.T) {
	m := openVibeWithPortion(t)
	cmd := m.handleVibeAIMsg(views.VibeAIRateMsg{Index: 1, Rating: 1})
	if cmd != nil {
		t.Errorf("rating a non-playing track must not return a cmd")
	}
	want := vibeai.Outcome{ID: m.vibeAI.feed[1].ID, Artist: "B", Title: "Two", Kind: "more"}
	if len(m.vibeAI.pending) != 1 || m.vibeAI.pending[0] != want {
		t.Fatalf("pending = %+v, want %+v", m.vibeAI.pending, want)
	}
}

func TestVibeAI_LessSkipsPlayingTrack(t *testing.T) {
	m := openVibeWithPortion(t)
	m.handleVibeAIMsg(views.VibeAIActionMsg{Action: views.VibeAIPlay, Index: 0})
	m.Update(playerStateMsg{Track: m.playerState.Track, Position: 10 * time.Second})
	if cmd := m.handleVibeAIMsg(views.VibeAIRateMsg{Index: 0, Rating: -1}); cmd == nil {
		t.Fatal("less on the playing track must skip to the next one")
	}
	if o := m.vibeAI.pending[0]; o.ID != "1" || o.Kind != "less" {
		t.Fatalf("pending[0] = %+v, want less for ID 1", o)
	}
	m.Update(playerStateMsg{Track: &provider.Track{ID: "2", Title: "Two"}, Position: 0})
	if len(m.vibeAI.pending) != 1 {
		t.Fatalf("pending = %+v, want only less: the skip by [ must not add skip_early", m.vibeAI.pending)
	}
	if cmd := m.handleVibeAIMsg(views.VibeAIRateMsg{Index: 0, Rating: 0}); cmd != nil {
		t.Error("clearing the rating must not skip")
	}
	if o := m.vibeAI.pending[1]; o.ID != "1" || o.Kind != "rate_clear" {
		t.Fatalf("pending[1] = %+v, want rate_clear for ID 1", o)
	}
	if cmd := m.handleVibeAIMsg(views.VibeAIRateMsg{Index: 2, Rating: -1}); cmd != nil {
		t.Error("less on another track must not skip")
	}
}

func TestVibeAI_RateIndexOutOfBounds(t *testing.T) {
	m := openVibeWithPortion(t)
	before := len(m.vibeAI.pending)
	if cmd := m.handleVibeAIMsg(views.VibeAIRateMsg{Index: 99, Rating: 1}); cmd != nil {
		t.Errorf("out-of-range rating must be a no-op, got cmd")
	}
	if len(m.vibeAI.pending) != before {
		t.Errorf("pending = %+v, want unchanged", m.vibeAI.pending)
	}
}
