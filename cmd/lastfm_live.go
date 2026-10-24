//go:build linux || darwin || windows

package cmd

import (
	"github.com/simone-vibes/vibez/internal/config"
	playerpkg "github.com/simone-vibes/vibez/internal/player"
)

// lastfmScrobbleLoop starts the scrobbler now if Last.fm is already connected, and
// otherwise on the first signal after the onboarding connects it. Never twice.
func lastfmScrobbleLoop(connected <-chan struct{}, cfg *config.Config, player interface{ Subscribe() <-chan playerpkg.State }, log func(string)) {
	started := startLastfmScrobbler(cfg, player, log)
	for range connected {
		if !started {
			started = startLastfmScrobbler(cfg, player, log)
		}
	}
}
