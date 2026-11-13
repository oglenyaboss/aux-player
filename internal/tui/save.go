package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/library"
	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
	"github.com/oglenyaboss/aux-player/internal/vibeai"
)

const recentLimit = 20

type SaveTrackMsg struct {
	Track    provider.Track
	Favorite bool
}

type saveTrackDoneMsg struct {
	key      string
	track    provider.Track
	favorite bool
	err      error
	songs    []provider.Track // the stored library after the save; nil when none is stored
}

type saveState int

const (
	savePending saveState = iota
	saveAccepted
)

type vibeAISavedSong struct {
	song     vibeai.Song
	favorite bool
}

func saveTrackID(t provider.Track) string {
	if t.CatalogID != "" {
		return t.CatalogID
	}
	return t.ID
}

func saveKey(t provider.Track, favorite bool) string {
	kind := "library"
	if favorite {
		kind = "favorites"
	}
	return saveTrackID(t) + "|" + kind
}

func saveLabel(favorite bool) string {
	if favorite {
		return "Favorites"
	}
	return stage.Tr("Библиотека", "Library")
}

func (m *Model) noteRecent(s player.State) {
	t := s.Track
	if t == nil || t.Title == "" {
		return
	}
	id := PlaybackID(*t)
	if len(m.recentTracks) > 0 {
		head := m.recentTracks[0]
		if PlaybackID(head) == id {
			if head.Title == t.Title && head.CatalogID == t.CatalogID {
				return
			}
			m.recentTracks[0] = *t
			m.pushRecent()
			return
		}
	}
	for i := 1; i < len(m.recentTracks); i++ {
		if PlaybackID(m.recentTracks[i]) == id {
			m.recentTracks = append(m.recentTracks[:i], m.recentTracks[i+1:]...)
			break
		}
	}
	m.recentTracks = append([]provider.Track{*t}, m.recentTracks...)
	if len(m.recentTracks) > recentLimit {
		m.recentTracks = m.recentTracks[:recentLimit]
	}
	m.pushRecent()
}

func (m *Model) pushRecent() {
	rows := make([]stage.Track, len(m.recentTracks))
	for i, t := range m.recentTracks {
		rows[i] = stage.Track{ID: PlaybackID(t), Artist: t.Artist, Title: t.Title}
	}
	m.vibeAIP.m.SetRecent(rows)
}

func (m *Model) handleSaveMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case SaveTrackMsg:
		key := saveKey(msg.Track, msg.Favorite)
		label := saveLabel(msg.Favorite)
		title := msg.Track.Title
		if state, ok := m.saves[key]; ok {
			switch state {
			case savePending:
				m.vibeAIP.m.SetNotice(fmt.Sprintf(stage.Tr("%s: уже отправляется «%s»", "%s: already on its way “%s”"), label, title))
				return nil
			case saveAccepted:
				m.vibeAIP.m.SetNotice(fmt.Sprintf(stage.Tr("%s: уже отправлен «%s»", "%s: already sent “%s”"), label, title))
				return nil
			}
		}
		m.saves[key] = savePending
		m.vibeAIP.m.SetNotice(fmt.Sprintf(stage.Tr("%s: сохраняю «%s»…", "%s: saving “%s”…"), label, title))
		prov := m.provider
		track := msg.Track
		favorite := msg.Favorite
		store := m.musicLibrary()
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if prov == nil {
				return saveTrackDoneMsg{key: key, track: track, favorite: favorite, err: fmt.Errorf("no provider")}
			}
			id := saveTrackID(track)
			var err error
			if favorite {
				err = prov.AddToFavorites(ctx, id)
			} else {
				err = prov.AddToLibrary(ctx, id)
			}
			var songs []provider.Track
			if err == nil && store != nil {
				songs = store.NoteSaved(track, favorite)
			}
			return saveTrackDoneMsg{key: key, track: track, favorite: favorite, err: err, songs: songs}
		}
	case saveTrackDoneMsg:
		if msg.err != nil {
			delete(m.saves, msg.key)
			m.appendLog(fmt.Sprintf("[save] %v", msg.err))
			m.vibeAIP.m.SetNotice(fmt.Sprintf(stage.Tr("%s ✗ %s · «%s»", "%s ✗ %s · “%s”"), saveLabel(msg.favorite), msg.err.Error(), msg.track.Title))
			return nil
		}
		m.saves[msg.key] = saveAccepted
		m.vibeAI.savedSongs = append(m.vibeAI.savedSongs, vibeAISavedSong{
			song:     vibeai.Song{ID: saveTrackID(msg.track), Artist: msg.track.Artist, Title: msg.track.Title, Album: msg.track.Album},
			favorite: msg.favorite,
		})
		if msg.songs != nil {
			m.musicShow(musicTarget{kind: library.Songs}, msg.songs, nil, nil)
		}
		m.vibeAIP.m.SetNotice(fmt.Sprintf(stage.Tr("%s ✓ Apple принял «%s»", "%s ✓ Apple accepted “%s”"), saveLabel(msg.favorite), msg.track.Title))
		return nil
	}
	return nil
}
