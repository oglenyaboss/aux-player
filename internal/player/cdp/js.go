package cdp

import (
	"encoding/json"
	"fmt"

	"github.com/oglenyaboss/aux-player/internal/audioquality"
	"github.com/oglenyaboss/aux-player/internal/player"
)

// buildSetQueueJS returns the JS expression that calls auxSetQueue.
// The IDs are JSON-encoded and passed as a JSON string literal so the
// browser side can JSON.parse() them without a second round-trip.
func buildSetQueueJS(ids []string) (string, error) {
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("cdp: marshal queue ids: %w", err)
	}
	js, err := json.Marshal(string(b))
	if err != nil {
		return "", fmt.Errorf("cdp: marshal queue json string: %w", err)
	}
	return fmt.Sprintf(`window.auxSetQueue && window.auxSetQueue(%s)`, js), nil
}

// buildSetPlaylistJS returns the JS expression that calls auxSetPlaylist.
func buildSetPlaylistJS(playlistID string, startIdx int) (string, error) {
	js, err := json.Marshal(playlistID)
	if err != nil {
		return "", fmt.Errorf("cdp: marshal playlist id: %w", err)
	}
	return fmt.Sprintf(`window.auxSetPlaylist && window.auxSetPlaylist(%s,%d)`, js, startIdx), nil
}

// buildAppendQueueJS returns the JS expression that calls auxAppendQueue.
func buildAppendQueueJS(ids []string) (string, error) {
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("cdp: marshal append ids: %w", err)
	}
	js, err := json.Marshal(string(b))
	if err != nil {
		return "", fmt.Errorf("cdp: marshal append json string: %w", err)
	}
	return fmt.Sprintf(`window.auxAppendQueue && window.auxAppendQueue(%s)`, js), nil
}

// buildPlayNextJS returns the JS expression that calls auxQueuePlayNext.
func buildPlayNextJS(ids []string) (string, error) {
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("cdp: marshal play-next ids: %w", err)
	}
	js, err := json.Marshal(string(b))
	if err != nil {
		return "", fmt.Errorf("cdp: marshal play-next json string: %w", err)
	}
	return fmt.Sprintf(`window.auxQueuePlayNext && window.auxQueuePlayNext(%s)`, js), nil
}

func buildSetAudioBitrateJS(kbps int) (string, error) {
	if err := audioquality.Validate(kbps); err != nil {
		return "", err
	}
	return fmt.Sprintf(`window.auxSetAudioBitrate && window.auxSetAudioBitrate(%d)`, kbps), nil
}

func buildSetEqualizerJS(bands []player.EQBand) (string, error) {
	b, err := json.Marshal(bands)
	if err != nil {
		return "", fmt.Errorf("cdp: marshal eq bands: %w", err)
	}
	js, err := json.Marshal(string(b))
	if err != nil {
		return "", fmt.Errorf("cdp: marshal eq json string: %w", err)
	}
	return fmt.Sprintf(`window.auxSetEqualizer && window.auxSetEqualizer(%s)`, js), nil
}
