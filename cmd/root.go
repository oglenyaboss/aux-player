package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/assets"
	"github.com/oglenyaboss/aux-player/internal/auth"
	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/lastfm"
	demoPlayer "github.com/oglenyaboss/aux-player/internal/player/demo"
	localPlayer "github.com/oglenyaboss/aux-player/internal/player/local"
	demoProvider "github.com/oglenyaboss/aux-player/internal/provider/demo"
	localProvider "github.com/oglenyaboss/aux-player/internal/provider/local"
	"github.com/oglenyaboss/aux-player/internal/tui"
	"github.com/spf13/cobra"
)

var cfgFile string
var debug bool
var demo bool
var noUpdate bool
var local bool
var musicDir string
var noDiscord bool

var rootCmd = &cobra.Command{
	Use:   "aux",
	Short: "AUX — a vibe-driven music player for your terminal",
	Long:  "AUX — a vibe-driven music player for your terminal.\n\nRun without arguments to open the TUI.",
	RunE:  runTUI,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/aux/config.json; Windows: %APPDATA%\\aux\\config.json)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&demo, "demo", false, "run with built-in fake data — no Apple account or internet required")
	rootCmd.PersistentFlags().BoolVar(&noUpdate, "no-update", false, "skip automatic update check on startup")
	rootCmd.PersistentFlags().BoolVar(&local, "local", false, "run with local music files (no Apple account required)")
	rootCmd.PersistentFlags().StringVar(&musicDir, "music-dir", "", "path to your music directory (saved to config)")
	rootCmd.PersistentFlags().BoolVar(&noDiscord, "no-discord", false, "disable Discord Rich Presence")
}

func runTUI(_ *cobra.Command, _ []string) error {
	cfgPath, err := config.ConfigPath(cfgFile)
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	audioBitrateKbps, err := cfg.AudioBitrateKbps()
	if err != nil {
		return err
	}

	opts := tui.Options{}
	if debug {
		logFile, err := openDebugLog()
		if err != nil {
			return err
		}
		defer func() { _ = logFile.Close() }()
		opts.DebugLog = logFile
		fmt.Fprintf(os.Stderr, "debug log: %s\n", logFile.Name())
	}

	if local {
		// if --music-dir was passed, it will save to config.
		if musicDir != "" {
			cfg.MusicDir = musicDir
			if saveErr := cfg.Save(cfgFile); saveErr != nil && debug {
				fmt.Fprintf(os.Stderr, "debug: saving music dir: %v\n", saveErr)
			}
		}

		// If musicDir is still empty, prompt the user on the command line
		// before launching the TUI.
		if cfg.MusicDir == "" {
			fmt.Print("Enter path to your music directory: ")
			dir, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			dir = strings.TrimSpace(dir)
			if dir == "" {
				return fmt.Errorf("no music directory provided")
			}
			cfg.MusicDir = dir
			if saveErr := cfg.Save(cfgFile); saveErr != nil && debug {
				fmt.Fprintf(os.Stderr, "debug: saving music dir: %v\n", saveErr)
			}
			fmt.Println("Music directory saved. You can change it anytime in Settings.")
		}

		iconPath := assets.InstallIcon()
		assets.InstallDesktopEntry()
		prov, err := localProvider.New(cfg.MusicDir)
		if err != nil {
			return fmt.Errorf("local provider: %w", err)
		}
		plyr, err := localPlayer.New(func(msg string) {
			fmt.Fprintln(os.Stderr, msg)
		})
		if err != nil {
			return err
		}
		defer func() { _ = plyr.Close() }()
		tracks, err := prov.GetLibraryTracks(context.Background())
		if err != nil {
			return fmt.Errorf("loading local tracks: %w", err)
		}
		plyr.LoadTracks(tracks)
		opts.IconPath = iconPath
		opts.InitialTracks = tracks
		opts.Backend = "Local mode · playing from " + cfg.MusicDir
		opts.ScanNotice = prov.ScanNotice()
		prog := tea.NewProgram(tui.New(cfg, prov, plyr, opts))
		stopRPC := startDiscordRPC(cfg, plyr, func(msg string) { prog.Send(tui.DebugLogMsg(msg)) })
		defer stopRPC()
		_, err = prog.Run()
		return err
	}

	if demo {
		iconPath := assets.InstallIcon()
		assets.InstallDesktopEntry()
		p := demoPlayer.New()
		dp := demoProvider.Provider{}
		opts.IconPath = iconPath
		opts.Backend = "Demo mode · built-in fake tracks, no credentials required"
		prog := tea.NewProgram(tui.New(cfg, dp, p, opts))
		_, err = prog.Run()
		return err
	}

	tokenErr := auth.ResolveDeveloperToken(cfg)
	lastfm.ApplyEmbedded(cfg)

	tokenLogs := auth.TokenLogs()
	if debug {
		for _, msg := range tokenLogs {
			fmt.Fprintf(os.Stderr, "debug: %s\n", msg)
		}
	}
	opts.InitialLogs = tokenLogs

	if tokenErr != nil && cfg.AppleDeveloperToken == "" {
		return fmt.Errorf("could not get an Apple Music developer token from music.apple.com.\n\nCheck your internet connection, or set apple_developer_token in %s", cfgPath)
	}

	// Make sure the desktop can resolve the MPRIS DesktopEntry property to an
	// icon (idempotent, best-effort). An install that brought its own entry and
	// icon, such as a package or the install script, is left alone; otherwise a
	// hidden fallback entry is written once.
	iconPath := assets.InstallIcon()
	assets.InstallDesktopEntry()
	opts.IconPath = iconPath

	onUserToken := func(token string) {
		cfg.AppleUserToken = token
		if saveErr := cfg.Save(""); saveErr != nil && debug {
			fmt.Fprintf(os.Stderr, "debug: saving user token: %v\n", saveErr)
		}
	}
	onStorefront := func(sf string) {
		if sf != "" && sf != cfg.StoreFront {
			cfg.StoreFront = sf
			if saveErr := cfg.Save(""); saveErr != nil && debug {
				fmt.Fprintf(os.Stderr, "debug: saving storefront: %v\n", saveErr)
			}
		}
	}

	return runPlatform(cfg, iconPath, opts, onUserToken, onStorefront, audioBitrateKbps)

}

// openDebugLog opens ~/.cache/aux/debug.log for appending, so the lines of a run
// that went wrong survive the next start.
func openDebugLog() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("debug log: %w", err)
	}
	dir = filepath.Join(dir, "aux")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("debug log: %w", err)
	}
	return os.OpenFile(filepath.Join(dir, "debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: the path is the user cache dir plus a fixed name
}
