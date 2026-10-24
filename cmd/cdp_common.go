//go:build linux || darwin || windows

package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/auth"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/lastfm"
	playerpkg "github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/player/cdp"
	"github.com/oglenyaboss/aux-player/internal/provider/apple"
	"github.com/oglenyaboss/aux-player/internal/tui"
	"github.com/oglenyaboss/aux-player/internal/updater"
	"github.com/oglenyaboss/aux-player/internal/version"
)

type cdpPlatformHooks struct {
	initStatus  string
	afterReady  func(*cdp.Player)
	helperPaths func() []string
	backend     func(audioBitrateKbps int) string
}

func runCDPFlow(cfg *config.Config, opts tui.Options, onUserToken, onStorefront func(string), audioBitrateKbps int, hooks cdpPlatformHooks) error {
	// Without a stored user token the sign-in waits for the AUX onboarding to ask.
	var ask chan struct{}
	if cfg.AppleUserToken == "" {
		ask = make(chan struct{}, 1)
		opts.AppleLogin = ask
	}
	lfm := make(chan struct{}, 1)
	opts.LastfmConnected = lfm
	prog := tea.NewProgram(tui.New(cfg, nil, nil, opts))
	playerCh := make(chan *cdp.Player, 1)
	runDone := make(chan struct{})
	restartExe := make(chan string, 1)
	var stopRPC func()

	go func() {
		if exe := updater.CheckAndUpdate(version.Version, noUpdate, func(msg string) {
			prog.Send(tui.InitStatusMsg(msg))
		}); exe != "" {
			restartExe <- exe
			prog.Send(tui.RestartMsg{})
			return
		}

		devTokenRejected := false
		if cfg.AppleUserToken != "" {
			prog.Send(tui.InitStatusMsg("Checking Apple Music session..."))
			switch status, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); status {
			case auth.UserTokenRejected:
				prog.Send(tui.InitStatusMsg("Session expired - re-authenticating..."))
				cfg.AppleUserToken = ""
				_ = cfg.Save("")
			case auth.DeveloperTokenRejected:
				// Re-authenticating cannot mint a working developer token, so
				// keep the user token: it is still good once the build is.
				devTokenRejected = true
				// The update check above may have been inside its once-a-day
				// window and returned without asking, which is exactly what
				// would leave the user reading "update aux" on a build that
				// could have updated itself. Ask again, and say what came back.
				res := updater.UpdateNow(version.Version, noUpdate, func(msg string) {
					prog.Send(tui.InitStatusMsg(msg))
				})
				if res.Outcome == updater.OutcomeInstalled {
					restartExe <- res.Exe
					prog.Send(tui.RestartMsg{})
					return
				}
				status := auth.DeveloperTokenStatus
				if advice := res.Advice(); advice != "" {
					status = auth.DeveloperTokenPrefix + " - " + advice
				}
				prog.Send(tui.InitStatusMsg(status))
			case auth.TokensValid:
				// The same request reported the account's storefront. Save it before
				// the player page renders: on a saved token the page's own value is
				// MusicKit's default, not the account's (#163).
				onStorefront(sf)
			}
		}

		// The generic progress line would replace the warning above before it
		// could be read, and it carries less information than the warning does.
		if !devTokenRejected {
			status := hooks.initStatus
			if status == "" {
				status = "Initializing AUX..."
			}
			prog.Send(tui.InitStatusMsg(status))
		}
		if err := cdp.EnsureBrowser(func(msg string) {
			prog.Send(tui.InitStatusMsg(msg))
		}); err != nil {
			prog.Send(tui.InitErrMsg{Err: fmt.Errorf("browser setup: %w", err)})
			return
		}

		if cfg.AppleUserToken == "" {
			prog.Send(tui.InitStatusMsg("Authorizing with Apple Music..."))
			if ask != nil {
				appleLoginLoop(ask, func() error { return auth.Login(cfg) }, prog.Send)
			} else {
				// A token that existed at startup died during the check: the
				// AUX screen is already up, sign in without an ask.
				if err := auth.Login(cfg); err != nil {
					prog.Send(tui.InitErrMsg{Err: fmt.Errorf("authentication: %w", err)})
					return
				}
			}
			// A fresh session has no storefront in config yet. The check that
			// validates returning sessions reports it.
			if _, sf := auth.CheckTokens(cfg.AppleDeveloperToken, cfg.AppleUserToken); sf != "" {
				onStorefront(sf)
			}
		}

		prog.Send(tui.InitStatusMsg("Starting audio engine..."))
		cdpPlayer, err := cdp.New(cfg.AppleDeveloperToken, cfg.AppleUserToken, cfg.StoreFront, cfg.WSL, audioBitrateKbps)
		if err != nil {
			prog.Send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
			return
		}
		cdpPlayer.OnUserToken = onUserToken
		cdpPlayer.OnStorefront = onStorefront
		cdpPlayer.OnSessionExpired = func() {
			prog.Send(tui.SessionExpiredMsg{})
			go func() {
				if err := auth.Login(cfg); err != nil {
					prog.Send(tui.InitErrMsg{Err: fmt.Errorf("re-auth: %w", err)})
					return
				}
				if err := cdpPlayer.SetUserToken(cfg.AppleUserToken); err != nil {
					prog.Send(tui.InitErrMsg{Err: fmt.Errorf("re-auth token update: %w", err)})
					return
				}
				cdpPlayer.ResetSessionExpired()
				prog.Send(tui.SessionRestoredMsg{})
			}()
		}

		playerCh <- cdpPlayer
		go func() {
			defer close(runDone)
			cdpPlayer.Run()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if waitErr := cdpPlayer.WaitReady(ctx); waitErr != nil {
			prog.Send(tui.InitErrMsg{Err: fmt.Errorf("audio engine init: %w", waitErr)})
			cdpPlayer.Terminate()
			return
		}

		if hooks.afterReady != nil {
			hooks.afterReady(cdpPlayer)
		}
		go lastfmScrobbleLoop(lfm, cfg, cdpPlayer, func(msg string) { prog.Send(tui.DebugLogMsg(msg)) })
		stopRPC = startDiscordRPC(cfg, cdpPlayer, func(msg string) { prog.Send(tui.DebugLogMsg(msg)) })

		prog.Send(tui.EngineReadyMsg{
			Player:      cdpPlayer,
			Provider:    apple.New(cfg),
			HelperPaths: hooks.helperPaths(),
			Backend:     hooks.backend(audioBitrateKbps),
		})
	}()

	_, err := prog.Run()
	if stopRPC != nil {
		stopRPC()
	}

	select {
	case p := <-playerCh:
		p.Terminate()
		<-runDone
	default:
	}

	select {
	case exe := <-restartExe:
		if restartErr := restartProcess(exe); restartErr != nil {
			return errors.Join(err, fmt.Errorf("restarting into the update at %s: %w", exe, restartErr))
		}
	default:
	}

	return err
}

func startLastfmScrobbler(cfg *config.Config, player interface {
	Subscribe() <-chan playerpkg.State
}, log func(string)) bool {
	if cfg.LastfmAPIKey == "" || cfg.LastfmAPISecret == "" || cfg.LastfmSessionKey == "" {
		return false
	}
	lfmClient := lastfm.NewClient(cfg.LastfmAPIKey, cfg.LastfmAPISecret, cfg.LastfmSessionKey)
	scrobbler := lastfm.NewScrobbler(lfmClient)
	scrobbler.SetLogger(log)
	go func() {
		for st := range player.Subscribe() {
			scrobbler.Update(st)
		}
	}()
	return true
}
