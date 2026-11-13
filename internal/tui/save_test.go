package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai"
)

type recordingProvider struct {
	mockProvider
	library     []string
	favorites   []string
	libraryErr  error
	favoriteErr error
}

func (p *recordingProvider) AddToLibrary(_ context.Context, id string) error {
	p.library = append(p.library, id)
	return p.libraryErr
}

func (p *recordingProvider) AddToFavorites(_ context.Context, id string) error {
	p.favorites = append(p.favorites, id)
	return p.favoriteErr
}

func saveTestModel(t *testing.T, prov provider.Provider, plyr player.Player) *Model {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	m := newModel(plyr)
	m.provider = prov
	m.stateCh = nil
	stageIntoPlayer(m)
	return m
}

func playTrack(t *testing.T, m *Model, tr provider.Track) {
	t.Helper()
	_, _ = m.Update(playerStateMsg(player.State{Playing: true, Track: &tr}))
}

func saveRequest(t *testing.T, m *Model, k string) tea.Cmd {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range stageMsgs(t, press(m, k)) {
		if _, ok := msg.(stageTickMsg); ok {
			continue
		}
		_, next := m.Update(msg)
		if next != nil {
			cmd = next
		}
	}
	return cmd
}

func runSave(t *testing.T, m *Model, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	var done []tea.Msg
	for _, msg := range stageMsgs(t, cmd) {
		if _, ok := msg.(stageTickMsg); ok {
			continue
		}
		done = append(done, msg)
		_, _ = m.Update(msg)
	}
	return done
}

func pressSave(t *testing.T, m *Model, k string) {
	t.Helper()
	runSave(t, m, saveRequest(t, m, k))
}

func saveFrame(m *Model) string {
	for range 10 {
		m.vibeAIP.m.st.Step(0.033)
	}
	return ansi.Strip(m.vibeAIP.m.Frame(110, 32))
}

func TestSavePlayingKeys(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	playTrack(t, m, a)

	pressSave(t, m, "l")
	pressSave(t, m, "f")
	if len(rp.library) != 1 || rp.library[0] != "10" {
		t.Fatalf("library calls = %v, want one for 10", rp.library)
	}
	if len(rp.favorites) != 1 || rp.favorites[0] != "10" {
		t.Fatalf("favorites calls = %v, want one for 10", rp.favorites)
	}
}

func TestSaveTargetFixedAtPress(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	b := provider.Track{ID: "i.b", CatalogID: "20", Artist: "B", Title: "Beta"}
	playTrack(t, m, a)
	cmd := saveRequest(t, m, "f")
	playTrack(t, m, b)
	runSave(t, m, cmd)
	if len(rp.favorites) != 1 || rp.favorites[0] != "10" {
		t.Fatalf("favorites calls = %v, want A's 10", rp.favorites)
	}
	if frame := saveFrame(m); !strings.Contains(frame, a.Title) {
		t.Errorf("accepted toast does not name %q:\n%s", a.Title, frame)
	}
}

func TestSaveRepeatsOneRequest(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})

	cmd := saveRequest(t, m, "l")
	if cmd == nil {
		t.Fatal("the first l must start a save")
	}
	if len(rp.library) != 0 {
		t.Fatalf("provider called before the cmd ran: %v", rp.library)
	}
	if again := saveRequest(t, m, "l"); again != nil {
		t.Fatal("a repeated l while pending must not send another request")
	}
	if frame := saveFrame(m); !strings.Contains(frame, stage.Tr("уже отправляется", "already on its way")) {
		t.Errorf("pending repeat toast missing:\n%s", frame)
	}
	runSave(t, m, cmd)
	if len(rp.library) != 1 {
		t.Fatalf("library calls = %v, want one", rp.library)
	}
	pressSave(t, m, "l")
	if len(rp.library) != 1 {
		t.Fatalf("accepted repeat made another call: %v", rp.library)
	}
	if frame := saveFrame(m); !strings.Contains(frame, stage.Tr("уже отправлен", "already sent")) {
		t.Errorf("accepted repeat toast missing:\n%s", frame)
	}
}

func TestSaveFailureCanRetry(t *testing.T) {
	rp := &recordingProvider{libraryErr: errors.New("denied")}
	m := saveTestModel(t, rp, newMockPlayer())
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	playTrack(t, m, a)

	pressSave(t, m, "l")
	if frame := saveFrame(m); !strings.Contains(frame, "✗ denied") {
		t.Errorf("failed toast missing:\n%s", frame)
	}
	if _, ok := m.saves[saveKey(a, false)]; ok {
		t.Fatal("a failed save must clear its state")
	}
	cmd := saveRequest(t, m, "l")
	if cmd == nil {
		t.Fatal("a failed save must be retryable")
	}
	runSave(t, m, cmd)
	if len(rp.library) != 2 {
		t.Fatalf("library calls = %v, want a retry", rp.library)
	}
}

func TestSaveLibraryAndFavoritesIndependent(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})
	library := saveRequest(t, m, "l")
	favorites := saveRequest(t, m, "f")
	runSave(t, m, library)
	runSave(t, m, favorites)
	if len(rp.library) != 1 || len(rp.favorites) != 1 {
		t.Fatalf("library = %v, favorites = %v, want one each", rp.library, rp.favorites)
	}
}

func TestSaveCache(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	dir := vibeAIConfig(m.cfg).CacheDir
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	libraryPath := filepath.Join(dir, "library.json")
	favoritesPath := filepath.Join(dir, "favorites.json")
	for _, path := range []string{libraryPath, favoritesPath} {
		if err := os.WriteFile(path, []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha", Album: "LP"})
	pressSave(t, m, "l")
	data, err := os.ReadFile(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	var songs []vibeai.Song
	if err := json.Unmarshal(data, &songs); err != nil {
		t.Fatal(err)
	}
	if len(songs) != 1 || songs[0].ID != "10" || songs[0].Title != "Alpha" {
		t.Fatalf("library cache = %+v, want the accepted song", songs)
	}

	beforeLibrary, err := os.ReadFile(libraryPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeFavorites, err := os.ReadFile(favoritesPath)
	if err != nil {
		t.Fatal(err)
	}
	rp.libraryErr = errors.New("denied")
	playTrack(t, m, provider.Track{ID: "i.c", CatalogID: "30", Artist: "C", Title: "Gamma"})
	pressSave(t, m, "l")
	afterLibrary, _ := os.ReadFile(libraryPath)
	afterFavorites, _ := os.ReadFile(favoritesPath)
	if !bytes.Equal(beforeLibrary, afterLibrary) || !bytes.Equal(beforeFavorites, afterFavorites) {
		t.Fatal("a failed save changed the cache files")
	}
}

type savedRecorder struct {
	songs     []vibeai.Song
	favorites []bool
}

func (s *savedRecorder) NoteSaved(song vibeai.Song, favorite bool) {
	s.songs = append(s.songs, song)
	s.favorites = append(s.favorites, favorite)
}

func TestSaveHandsSongToAgent(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha", Album: "LP"})
	pressSave(t, m, "f")
	r := m.vibeAI
	if len(r.savedSongs) != 1 {
		t.Fatalf("savedSongs = %+v, want one song", r.savedSongs)
	}
	saved := append([]vibeAISavedSong(nil), r.savedSongs...)
	if got := saved[0]; got.song.ID != "10" || !got.favorite {
		t.Errorf("saved song = %+v, want catalog 10 as a favorite", got)
	}
	if len(r.pending) != 0 {
		t.Errorf("pending = %+v, a save must not add an outcome", r.pending)
	}
	m.vibeAITurnCmd("", false, false)
	t.Cleanup(func() {
		if r.cancel != nil {
			r.cancel()
		}
	})
	if r.savedSongs != nil {
		t.Errorf("savedSongs = %+v, want drained into the turn", r.savedSongs)
	}
	rec := &savedRecorder{}
	noteSavedSongs(rec, saved)
	if len(rec.songs) != 1 {
		t.Fatalf("NotSaved calls = %+v, want one", rec.songs)
	}
	if rec.songs[0].ID != "10" || !rec.favorites[0] {
		t.Errorf("NoteSaved(%+v, %v), want catalog 10 as a favorite", rec.songs[0], rec.favorites[0])
	}
}

func TestSavePlusStaysLocal(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})
	for _, msg := range stageMsgs(t, press(m, "+")) {
		if _, ok := msg.(stageTickMsg); ok {
			continue
		}
		_, _ = m.Update(msg)
	}
	if len(rp.library) != 0 || len(rp.favorites) != 0 {
		t.Fatalf("+ reached the provider: library = %v, favorites = %v", rp.library, rp.favorites)
	}
}

func TestSaveLeavesPlaybackAlone(t *testing.T) {
	rp := &recordingProvider{}
	mp := newMockPlayer()
	m := saveTestModel(t, rp, mp)
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})
	pressSave(t, m, "l")
	pressSave(t, m, "f")
	press(m, "h")
	if mp.playCalled || mp.pauseCalled || mp.nextCalled || mp.prevCalled {
		t.Error("a save key touched playback")
	}
	if len(mp.setQueueIDs) != 0 || len(mp.appendQueueIDs) != 0 {
		t.Errorf("a save key changed the queue: %v %v", mp.setQueueIDs, mp.appendQueueIDs)
	}
}

func TestNoteRecent(t *testing.T) {
	m := newModel(nil)
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	b := provider.Track{ID: "i.b", CatalogID: "20", Artist: "B", Title: "Beta"}
	for _, tr := range []provider.Track{a, a, a, b, a} {
		m.noteRecent(player.State{Track: &tr})
	}
	if len(m.recentTracks) != 2 || m.recentTracks[0].ID != a.ID || m.recentTracks[1].ID != b.ID {
		t.Fatalf("recent = %+v, want [A B]", m.recentTracks)
	}

	empty := provider.Track{ID: "i.c", CatalogID: "30"}
	m.noteRecent(player.State{Track: &empty})
	if len(m.recentTracks) != 2 {
		t.Fatalf("a titleless state added a row: %+v", m.recentTracks)
	}
	filled := empty
	filled.Title = "One"
	m.noteRecent(player.State{Track: &filled})
	if len(m.recentTracks) != 3 {
		t.Fatalf("recent = %+v, want one row for the filled track", m.recentTracks)
	}

	for i := 0; i < 25; i++ {
		tr := provider.Track{ID: fmt.Sprintf("i.%d", i), CatalogID: fmt.Sprintf("%d", i), Artist: "A", Title: fmt.Sprintf("T%d", i)}
		m.noteRecent(player.State{Track: &tr})
	}
	if len(m.recentTracks) != recentLimit {
		t.Fatalf("recent = %d, want %d", len(m.recentTracks), recentLimit)
	}
	if got := m.recentTracks[0].ID; got != "i.24" {
		t.Errorf("newest = %q, want i.24", got)
	}
	if got := m.recentTracks[len(m.recentTracks)-1].ID; got != "i.5" {
		t.Errorf("oldest kept = %q, want i.5", got)
	}
}

func TestRecentRowSavesCatalogID(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	b := provider.Track{ID: "i.b", CatalogID: "20", Artist: "B", Title: "Beta"}
	playTrack(t, m, a)
	m.noteRecent(player.State{Track: &b})
	press(m, "h")
	pressSave(t, m, "l")
	if len(rp.library) != 1 || rp.library[0] != "20" {
		t.Fatalf("library calls = %v, want the selected row's 20", rp.library)
	}
	if m.playerState.Track == nil || m.playerState.Track.ID != a.ID {
		t.Fatalf("playing track changed: %+v", m.playerState.Track)
	}
}

func TestSaveWithoutProvider(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	m.provider = nil
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})
	pressSave(t, m, "l")
	if frame := saveFrame(m); !strings.Contains(frame, "✗ no provider") {
		t.Errorf("failed toast missing without a provider:\n%s", frame)
	}
	if len(m.saves) != 0 {
		t.Errorf("saves = %v, want cleared", m.saves)
	}
}

func TestSaveNothingPlaying(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	m.playerState.Track = nil
	if cmd := press(m, "l"); cmd != nil {
		t.Fatalf("l without a track produced %#v", cmd())
	}
	if frame := saveFrame(m); !strings.Contains(frame, stage.Tr("ничего не играет", "nothing is playing")) {
		t.Errorf("toast missing without a track:\n%s", frame)
	}
	if len(rp.library) != 0 || len(rp.favorites) != 0 {
		t.Fatalf("provider called without a track: %v %v", rp.library, rp.favorites)
	}
}

func TestSaveFailureShowsCause(t *testing.T) {
	rp := &recordingProvider{libraryErr: errors.New("status 401")}
	m := saveTestModel(t, rp, newMockPlayer())
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"})

	pressSave(t, m, "l")
	if len(rp.library) != 1 {
		t.Fatalf("library calls = %v, want one", rp.library)
	}
	frame := saveFrame(m)
	if !strings.Contains(frame, "✗ status 401") {
		t.Errorf("failed toast hides the cause:\n%s", frame)
	}
	if !strings.Contains(frame, stage.Tr("Библиотека", "Library")) {
		t.Errorf("failed toast lacks the destination:\n%s", frame)
	}
}

func TestSaveLongTitleKeepsVerdict(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	title := "A Very Long Title That Goes On And On And On Forever Okay Yes"
	playTrack(t, m, provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: title})

	pressSave(t, m, "f")
	if len(rp.favorites) != 1 {
		t.Fatalf("favorites calls = %v, want one", rp.favorites)
	}
	if frame := saveFrame(m); !strings.Contains(frame, "Favorites ✓") {
		t.Errorf("accepted toast lacks the verdict:\n%s", frame)
	}

	pressSave(t, m, "f")
	if len(rp.favorites) != 1 {
		t.Fatalf("accepted repeat made another call: %v", rp.favorites)
	}
	frame := saveFrame(m)
	if !strings.Contains(frame, stage.Tr("уже отправлен", "already sent")) {
		t.Errorf("repeat toast lacks the verdict:\n%s", frame)
	}
	if !strings.Contains(frame, "Favorites:") {
		t.Errorf("repeat toast lacks the destination:\n%s", frame)
	}
}

func TestSaveEvictedRecentRow(t *testing.T) {
	rp := &recordingProvider{}
	m := saveTestModel(t, rp, newMockPlayer())
	a := provider.Track{ID: "i.a", CatalogID: "10", Artist: "A", Title: "Alpha"}
	b := provider.Track{ID: "i.b", CatalogID: "20", Artist: "B", Title: "Beta"}
	playTrack(t, m, a)
	m.recentTracks = []provider.Track{b}

	press(m, "h")
	pressSave(t, m, "l")
	if len(rp.library) != 0 {
		t.Fatalf("an evicted row reached the provider: %v", rp.library)
	}
	frame := saveFrame(m)
	if !strings.Contains(frame, stage.Tr("выпал из списка недавних", "left the recent list")) {
		t.Errorf("evicted-row toast missing:\n%s", frame)
	}
	if strings.Contains(frame, stage.Tr("ничего не играет", "nothing is playing")) {
		t.Errorf("evicted row toasted as a missing track:\n%s", frame)
	}
}
