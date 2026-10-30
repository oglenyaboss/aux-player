//go:build linux

package mpris

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
)

// ─── MPRIS Server tests ───────────────────────────────────────────────────────

// mockController implements the Controller interface for testing.
type mockController struct {
	playCalled       bool
	pauseCalled      bool
	nextCalled       bool
	prevCalled       bool
	seekDuration     time.Duration
	repeatMode       int
	shuffleMode      bool
	setRepeatCalled  bool
	setShuffleCalled bool
}

func (m *mockController) Play() error                { m.playCalled = true; return nil }
func (m *mockController) Pause() error               { m.pauseCalled = true; return nil }
func (m *mockController) Next() error                { m.nextCalled = true; return nil }
func (m *mockController) Previous() error            { m.prevCalled = true; return nil }
func (m *mockController) Seek(d time.Duration) error { m.seekDuration = d; return nil }
func (m *mockController) SetRepeat(mode int) error {
	m.setRepeatCalled = true
	m.repeatMode = mode
	return nil
}
func (m *mockController) SetShuffle(on bool) error {
	m.setShuffleCalled = true
	m.shuffleMode = on
	return nil
}

func TestNewServer_Success(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		// The service name "org.mpris.MediaPlayer2.aux" might already be taken in CI.
		// Accept that as a non-fatal skip.
		t.Skipf("NewServer failed (name may be taken): %v", err)
	}
	defer srv.Close() //nolint:errcheck

	if srv == nil {
		t.Fatal("NewServer returned nil server")
	}
}

func TestServer_Update_Playing(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	track := &provider.Track{
		ID:     "track1",
		Title:  "Test Track",
		Artist: "Test Artist",
		Album:  "Test Album",
	}
	st := player.State{
		Playing:  true,
		Track:    track,
		Volume:   0.8,
		Position: 45 * time.Second,
	}
	// Should not panic.
	srv.Update(st)
}

func TestServer_Update_Paused(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	srv.Update(player.State{Playing: false, Volume: 0.5})
}

func TestServer_Update_RepeatMode(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	srv.Update(player.State{RepeatMode: player.RepeatModeAll, ShuffleMode: true})
	srv.Update(player.State{RepeatMode: player.RepeatModeOne})
	srv.Update(player.State{RepeatMode: player.RepeatModeOff})
}

func TestServer_Update_TrackWithArtwork(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	track := &provider.Track{
		ID:         "art-track",
		Title:      "Artwork Song",
		Artist:     "Artwork Artist",
		ArtworkURL: "https://example.com/art.jpg",
	}
	srv.Update(player.State{Playing: true, Track: track})
}

func TestServer_Update_TrackWithoutID(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	track := &provider.Track{Title: "Partial Track"}
	srv.Update(player.State{Playing: true, Track: track})
	srv.flush()

	metadata, ok := srv.props.get(mprisPlayerIface, "Metadata").(map[string]dbus.Variant)
	if !ok {
		t.Fatal("Metadata property has unexpected type")
	}
	trackID, ok := metadata["mpris:trackid"].Value().(dbus.ObjectPath)
	if !ok {
		t.Fatal("mpris:trackid has unexpected type")
	}
	want := trackObjectPath(track)
	if trackID != want {
		t.Fatalf("mpris:trackid = %q, want %q", trackID, want)
	}
}

func TestServer_Close(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	srv.Update(player.State{Playing: true, Track: &provider.Track{ID: "pending"}})

	// Holding flushMu models a callback already publishing. Close must mark the
	// server closed immediately, then wait before closing the D-Bus connection.
	srv.flushMu.Lock()
	closeDone := make(chan error, 1)
	go func() { closeDone <- srv.Close() }()
	deadline := time.Now().Add(time.Second)
	for {
		srv.mu.Lock()
		closed := srv.closed
		srv.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			srv.flushMu.Unlock()
			t.Fatal("Close did not mark the server closed")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-closeDone:
		srv.flushMu.Unlock()
		t.Fatalf("Close returned before the in-flight flush completed: %v", err)
	default:
	}
	srv.flushMu.Unlock()
	if err := <-closeDone; err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Pending and post-close updates must not flush against the closed D-Bus connection.
	srv.Update(player.State{Playing: true, Track: &provider.Track{ID: "closed"}})
	time.Sleep(200 * time.Millisecond)
}

func TestServer_Update_DistinguishesCollidingTrackIDs(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	first := &provider.Track{ID: "i.track/1", Title: "First"}
	srv.Update(player.State{Playing: true, Track: first})
	srv.flush()

	second := &provider.Track{ID: "i_track_1", Title: "Second"}
	srv.Update(player.State{Playing: true, Track: second})
	srv.flush()

	metadata, ok := srv.props.get(mprisPlayerIface, "Metadata").(map[string]dbus.Variant)
	if !ok {
		t.Fatal("Metadata property has unexpected type")
	}
	title, ok := metadata["xesam:title"].Value().(string)
	if !ok {
		t.Fatal("xesam:title has unexpected type")
	}
	if title != second.Title {
		t.Fatalf("xesam:title = %q, want %q", title, second.Title)
	}
}

func TestServer_UpdateAfterBusLossDoesNotPanic(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}

	srv.Update(player.State{Playing: true, Track: &provider.Track{ID: "track"}})
	if err := srv.conn.Close(); err != nil {
		t.Fatalf("close D-Bus connection: %v", err)
	}

	// flush runs synchronously so any panic fails this test directly.
	srv.flush()

	srv.mu.Lock()
	unavailable := srv.unavailable
	srv.mu.Unlock()
	if !unavailable {
		t.Fatal("server remained available after losing its D-Bus connection")
	}

	srv.Update(player.State{Playing: true, Track: &provider.Track{ID: "ignored"}})
	srv.mu.Lock()
	pending := srv.pending
	srv.mu.Unlock()
	if pending != nil {
		t.Fatal("server accepted an update after losing its D-Bus connection")
	}
	_ = srv.Close()
}

// --- metadata key lifetime ---

func currentMetadata(t *testing.T, srv *Server) map[string]dbus.Variant {
	t.Helper()
	meta, ok := srv.props.get(mprisPlayerIface, "Metadata").(map[string]dbus.Variant)
	if !ok {
		t.Fatal("Metadata property is not a map[string]dbus.Variant")
	}
	return meta
}

// godbus merges each update into the map it already holds, so a key left out of
// one update keeps the previous track's value instead of disappearing.
func TestServer_Update_ClearsStaleMetadataKeys(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	withArt := &provider.Track{
		ID:         "with-art",
		Title:      "Has Art",
		Artist:     "First Artist",
		Album:      "First Album",
		Duration:   3 * time.Minute,
		ArtworkURL: "https://example.com/cover.jpg",
	}
	srv.Update(player.State{Playing: true, Track: withArt, Position: time.Second})
	srv.flush()

	if got := currentMetadata(t, srv)["mpris:artUrl"].Value(); got != withArt.ArtworkURL {
		t.Fatalf("mpris:artUrl = %v, want %q", got, withArt.ArtworkURL)
	}

	noArt := &provider.Track{
		ID:       "no-art",
		Title:    "No Art",
		Artist:   "Second Artist",
		Duration: 2 * time.Minute,
	}
	srv.Update(player.State{Playing: true, Track: noArt, Position: time.Second})
	srv.flush()

	meta := currentMetadata(t, srv)
	if got := meta["mpris:artUrl"].Value(); got != "" {
		t.Errorf("mpris:artUrl = %v, want empty for a track without artwork", got)
	}
	if got := meta["xesam:album"].Value(); got != "" {
		t.Errorf("xesam:album = %v, want empty for a track without an album", got)
	}
}

func TestServer_Update_ClearsMetadataWhenTrackGoesAway(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	srv.Update(player.State{Playing: true, Position: time.Second, Track: &provider.Track{
		ID:       "going-away",
		Title:    "Departing",
		Artist:   "Departing Artist",
		Album:    "Departing Album",
		Duration: 4 * time.Minute,
	}})
	srv.flush()

	srv.Update(player.State{Playing: false})
	srv.flush()

	meta := currentMetadata(t, srv)
	if got := meta["mpris:trackid"].Value(); got != noTrackPath {
		t.Errorf("mpris:trackid = %v, want %v", got, noTrackPath)
	}
	for _, key := range []string{"xesam:title", "xesam:album"} {
		if got := meta[key].Value(); got != "" {
			t.Errorf("%s = %v, want empty once no track is playing", key, got)
		}
	}
	if got := meta["mpris:length"].Value(); got != int64(0) {
		t.Errorf("mpris:length = %v, want 0 once no track is playing", got)
	}
	if got, ok := meta["xesam:artist"].Value().([]string); !ok || len(got) != 0 {
		t.Errorf("xesam:artist = %v, want empty once no track is playing", meta["xesam:artist"].Value())
	}
}

// Regression test for #110, meaningful only under -race.
//
// A client calling GetAll while a track change was in flight raced godbus's
// retained Metadata map: prop.GetAll handed back the very map dbus.Store was
// merging the next update into, and the reply was encoded in the connection's
// output goroutine after GetAll had already released the lock. Emit settings
// could not fix that leg, because no PropertiesChanged was involved.
func TestServer_GetAllDuringFlushIsRaceFree(t *testing.T) {
	ctrl := &mockController{}
	srv, err := NewServer(ctrl)
	if err != nil {
		t.Skipf("NewServer failed: %v", err)
	}
	defer srv.Close() //nolint:errcheck

	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("D-Bus unavailable:", err)
	}
	defer client.Close() //nolint:errcheck
	obj := client.Object(mprisServiceName, mprisObjectPath)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			var all map[string]dbus.Variant
			if err := obj.Call(propertiesIface+".GetAll", 0, mprisPlayerIface).Store(&all); err != nil {
				continue
			}
			// Touch the metadata map so a racing write is actually observed
			// rather than sitting undetected in the reply body.
			if meta, ok := all["Metadata"].Value().(map[string]dbus.Variant); ok {
				for k := range meta {
					_ = k
				}
			}
		}
	})

	for i := range 60 {
		srv.Update(player.State{
			Playing:  true,
			Track:    &provider.Track{ID: strconv.Itoa(i), Title: "T" + strconv.Itoa(i), Artist: "A", Duration: time.Minute},
			Position: time.Duration(i) * time.Second,
		})
		srv.flush()
	}
	close(stop)
	wg.Wait()
}
