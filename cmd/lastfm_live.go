//go:build linux || darwin || windows

package cmd

import (
	"github.com/oglenyaboss/aux-player/internal/config"
	playerpkg "github.com/oglenyaboss/aux-player/internal/player"
)

// lastfmScrobbleLoop starts the scrobbler on the first signal after the onboarding
// connects Last.fm, and never twice. started reports whether the caller already did
// it for a session stored at startup: the config is read here only after a signal,
// so the onboarding's write of the session key is ordered before it.
func lastfmScrobbleLoop(connected <-chan struct{}, cfg *config.Config, started bool, player interface{ Subscribe() <-chan playerpkg.State }, log func(string)) {
	for range connected {
		if !started {
			started = startLastfmScrobbler(cfg, player, log)
		}
	}
}
