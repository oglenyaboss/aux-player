//go:build linux

package cmd

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/auth"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/player/cdp"
	"github.com/oglenyaboss/aux-player/internal/player/mpris"
	"github.com/oglenyaboss/aux-player/internal/player/webkit"
	"github.com/oglenyaboss/aux-player/internal/provider/apple"
	"github.com/oglenyaboss/aux-player/internal/tui"
	"github.com/oglenyaboss/aux-player/internal/updater"
	"github.com/oglenyaboss/aux-player/internal/version"
)

func runPlatform(cfg *config.Config, iconPath string, opts tui.Options, onUserToken, onStorefront func(string), audioBitrateKbps int) error {
	wantBrowserless := browserlessFlag || os.Getenv("AUX_BROWSERLESS") == "1"
	if wantBrowserless {
		return runBrowserlessFlow(cfg, opts, onUserToken, onStorefront, audioBitrateKbps)
	}
	if cdp.Available() {
		return runCDPFlow(cfg, opts, onUserToken, onStorefront, audioBitrateKbps, cdpPlatformHooks{
			initStatus: "Initializing AUX...",
			afterReady: func(cdpPlayer *cdp.Player) {
				if srv, mprisErr := mpris.NewServer(cdpPlayer); mprisErr == nil {
					go func() {
						for st := range cdpPlayer.Subscribe() {
							srv.Update(st)
						}
					}()
				}
			},
			helperPaths: func() []string {
				return []string{cdp.HelperPath(), cdp.ChromePath()}
			},
			backend: func(audioBitrateKbps int) string {
				return fmt.Sprintf("Chrome/CDP · provider: Apple Music · %d kbps AAC · helper: %s", audioBitrateKbps, cdp.HelperPath())
			},
		})
	}
	return runWebKitFlow(cfg, iconPath, opts, onUserToken, onStorefront, audioBitrateKbps)
}

// runWebKitFlow is the legacy path: GTK must own the main OS thread so auth
// and engine setup happen before the TUI, and BubbleTea runs in a goroutine.
func runWebKitFlow(cfg *config.Config, iconPath string, opts tui.Options, onUserToken, onStorefront func(string), audioBitrateKbps int) error {
	fmt.Fprintln(os.Stderr, "Engine: WebKit + GStreamer (30 s preview, bitrate changes unsupported)")

	if cfg.AppleUserToken != "" {
		fmt.Fprintln(os.Stderr, "Checking Apple Music session...")
		switch status, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); status {
		case auth.UserTokenRejected:
			fmt.Fprintln(os.Stderr, "Session expired - re-authenticating...")
			cfg.AppleUserToken = ""
			_ = cfg.Save("")
		case auth.DeveloperTokenRejected:
			// Re-authenticating cannot mint a working developer token, so keep
			// the user token: it is still good once the build is. This path
			// runs no update check of its own, so without asking here a stale
			// build has no way to fix itself.
			res := updater.UpdateNow(version.Version, noUpdate, func(msg string) {
				fmt.Fprintln(os.Stderr, msg)
			})
			if res.Outcome == updater.OutcomeInstalled {
				// Nothing owns the terminal or the main thread yet on this
				// path, so the new binary can take over directly.
				return syscall.Exec(res.Exe, os.Args, os.Environ()) //nolint:gosec // exe is the binary we just verified and installed
			}
			fmt.Fprintln(os.Stderr, auth.DeveloperTokenHelp)
			if advice := res.Advice(); advice != "" {
				fmt.Fprintln(os.Stderr, advice)
			}
		case auth.TokensValid:
			// The same request reported the account's storefront. Save it before
			// the player page renders: on a saved token the page's own value is
			// MusicKit's default, not the account's (#163).
			onStorefront(sf)
		}
	}
	if cfg.AppleUserToken == "" {
		if err := auth.Login(cfg); err != nil {
			return fmt.Errorf("authentication: %w", err)
		}
		// A fresh session has no storefront in config yet. The check that
		// validates returning sessions reports it.
		if _, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); sf != "" {
			onStorefront(sf)
		}
	}

	wkPlayer, err := webkit.New(cfg.AppleDeveloperToken, cfg.AppleUserToken, cfg.StoreFront, audioBitrateKbps)
	if err != nil {
		return fmt.Errorf("creating audio engine: %w", err)
	}
	wkPlayer.OnUserToken = onUserToken
	wkPlayer.OnStorefront = onStorefront

	prov := apple.New(cfg)
	tuiErr := make(chan error, 1)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if waitErr := wkPlayer.WaitReady(ctx); waitErr != nil {
			tuiErr <- fmt.Errorf("audio engine init: %w", waitErr)
			wkPlayer.Terminate()
			return
		}

		if srv, mprisErr := mpris.NewServer(wkPlayer); mprisErr == nil {
			defer func() { _ = srv.Close() }()
			go func() {
				for st := range wkPlayer.Subscribe() {
					srv.Update(st)
				}
			}()
		}

		opts.IconPath = iconPath
		opts.Backend = "WebKit/GStreamer · fixed 30s previews · bitrate changes unsupported · provider: Apple Music"
		lfm := make(chan struct{}, 1)
		opts.LastfmConnected = lfm
		m := tui.New(cfg, prov, wkPlayer, opts)
		p := tea.NewProgram(m)

		go lastfmScrobbleLoop(lfm, cfg, wkPlayer, func(msg string) { p.Send(tui.DebugLogMsg(msg)) })
		stopRPC := startDiscordRPC(cfg, wkPlayer, func(msg string) { p.Send(tui.DebugLogMsg(msg)) })
		defer stopRPC()

		_, runErr := p.Run()
		tuiErr <- runErr
		wkPlayer.Terminate()
	}()

	wkPlayer.Run()
	return <-tuiErr
}
