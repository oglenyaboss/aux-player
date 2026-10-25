//go:build linux

package cmd

import (
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/tui"
)

func runPlatform(cfg *config.Config, _ string, opts tui.Options, onUserToken, onStorefront func(string), audioBitrateKbps int) error {
	return runBrowserlessFlow(cfg, opts, onUserToken, onStorefront, audioBitrateKbps)
}
