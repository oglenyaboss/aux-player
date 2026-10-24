package tui

import (
	"context"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
)

// --- mock player ---

type mockPlayer struct {
	state              player.State
	playCalled         bool
	pauseCalled        bool
	nextCalled         bool
	prevCalled         bool
	closeCalled        bool
	seekCalled         bool
	seekPos            time.Duration
	bitrateKbps        int
	setQueueIDs        []string   // last IDs passed to SetQueue
	appendQueueIDs     [][]string // all calls to AppendQueue (each call appended)
	moveInQueueCalls   []struct{ From, To int }
	removeFromQueueIdx []int // all calls to RemoveFromQueue, in call order
	err                error
	stateCh            chan player.State
}

func newMockPlayer() *mockPlayer {
	return &mockPlayer{stateCh: make(chan player.State, 4)}
}

func (m *mockPlayer) Play() error     { m.playCalled = true; return m.err }
func (m *mockPlayer) Pause() error    { m.pauseCalled = true; return m.err }
func (m *mockPlayer) Stop() error     { return m.err }
func (m *mockPlayer) Next() error     { m.nextCalled = true; return m.err }
func (m *mockPlayer) Previous() error { m.prevCalled = true; return m.err }
func (m *mockPlayer) Seek(pos time.Duration) error {
	m.seekCalled = true
	m.seekPos = pos
	return m.err
}
func (m *mockPlayer) SetVolume(_ float64) error { return m.err }
func (m *mockPlayer) SetAudioBitrate(kbps int) error {
	m.bitrateKbps = kbps
	return m.err
}
func (m *mockPlayer) SetRepeat(_ int) error                { return m.err }
func (m *mockPlayer) SetShuffle(_ bool) error              { return m.err }
func (m *mockPlayer) SetEqualizer(_ []player.EQBand) error { return m.err }
func (m *mockPlayer) SetQueue(ids []string) error          { m.setQueueIDs = ids; return m.err }
func (m *mockPlayer) SetPlaylist(_ string, _ int) error    { return m.err }
func (m *mockPlayer) AppendQueue(ids []string) error {
	m.appendQueueIDs = append(m.appendQueueIDs, ids)
	return m.err
}
func (m *mockPlayer) RemoveFromQueue(idx int) error {
	m.removeFromQueueIdx = append(m.removeFromQueueIdx, idx)
	return m.err
}
func (m *mockPlayer) MoveInQueue(from, to int) error {
	m.moveInQueueCalls = append(m.moveInQueueCalls, struct{ From, To int }{from, to})
	return m.err
}
func (m *mockPlayer) ClearQueue() error { return m.err }
func (m *mockPlayer) GetState() (*player.State, error) {
	s := m.state
	return &s, m.err
}
func (m *mockPlayer) Subscribe() <-chan player.State { return m.stateCh }
func (m *mockPlayer) Close() error                   { m.closeCalled = true; return m.err }

// --- mock provider ---

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Search(_ context.Context, _ string) (*provider.SearchResult, error) {
	return &provider.SearchResult{}, nil
}
func (m *mockProvider) GetLibraryTracks(_ context.Context) ([]provider.Track, error) {
	return nil, nil
}
func (m *mockProvider) GetLibraryPlaylists(_ context.Context) ([]provider.Playlist, error) {
	return nil, nil
}
func (m *mockProvider) GetPlaylistTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}
func (m *mockProvider) GetAlbumTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}

func (m *mockProvider) GetLibraryAlbumTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}
func (m *mockProvider) GetCatalogPlaylistTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}
func (m *mockProvider) GetArtistAlbums(_ context.Context, _ string) ([]provider.Album, error) {
	return nil, nil
}
func (m *mockProvider) CreatePlaylist(_ context.Context, _ string, _ []string) (provider.Playlist, error) {
	return provider.Playlist{}, nil
}
func (m *mockProvider) LoveSong(_ context.Context, _ string, _ bool) error      { return nil }
func (m *mockProvider) GetSongRating(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *mockProvider) IsAuthenticated() bool                                   { return true }
func (m *mockProvider) AddToPlaylist(_ context.Context, _, _ string) error      { return nil }
func (m *mockProvider) GetRecommendations(_ context.Context) ([]provider.RecommendationGroup, error) {
	return nil, nil
}
func (m *mockProvider) GetStationTracks(_ context.Context, _ string) ([]provider.Track, error) {
	return nil, nil
}

// --- helpers ---

func testCfg() *config.Config {
	return &config.Config{
		StoreFront:    "us",
		AuthPort:      7777,
		Provider:      "apple",
		Theme:         "default",
		VibeLLMAPIKey: "test-key",
	}
}

func newModel(plyr player.Player) *Model {
	return New(testCfg(), &mockProvider{}, plyr, Options{})
}
