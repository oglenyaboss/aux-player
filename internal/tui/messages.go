package tui

import (
	"io"

	"github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
	"github.com/oglenyaboss/aux-player/internal/stage"
)

// Options configures optional TUI features at startup.
type Options struct {
	// AppleLogin, when set, holds the Apple Music sign-in until the onboarding asks for it.
	AppleLogin chan<- struct{}
	// LastfmConnected, when set, gets a signal each time the onboarding connects Last.fm,
	// so the scrobbler can start without a restart.
	LastfmConnected chan<- struct{}
	// IconPath is the absolute path of the installed app icon (may be empty).
	IconPath string
	// Backend is a human-readable description of the audio backend, logged on
	// startup. Leave empty if the engine starts asynchronously (CDP path uses
	// EngineReadyMsg.Backend instead).
	Backend string
	// InitialTracks pre-populates the queue on launch (used by local mode).
	InitialTracks []provider.Track
	// ScanNotice summarises what a local-mode directory scan left out. It is
	// recorded in the debug log on startup, and shown as an error when the
	// scan produced no tracks at all, because an empty library otherwise looks
	// identical to a mistyped path.
	ScanNotice string
	// UpdateNotice, when set, is a one-shot word about an update: the tag a
	// self-update installed before this start (Installed), or a newer release
	// that is out but was not installed. It waits for the player page and
	// lands as a stage toast.
	UpdateNotice *UpdateNoticeMsg
	// InitialLogs pre-populates the debug log on launch (e.g. broker token fetch logs).
	InitialLogs []string
	// DebugLog, when set, gets every debug log line with its date (aux --debug).
	DebugLog io.Writer
}

// InitStatusMsg updates the loading status; the AUX scene has no loading screen
// of its own, so the message is accepted and ignored.
type InitStatusMsg string

// EngineReadyMsg is sent when the audio engine and provider are both ready.
type EngineReadyMsg struct {
	Player      player.Player
	Provider    provider.Provider
	HelperPaths []string // absolute paths of the Chrome helper binary (for RSS tracking)
	Backend     string   // human-readable backend description, shown in the debug log
}

// InitErrMsg is sent when initialization fails fatally.
type InitErrMsg struct{ Err error }

// AppleLoginMsg is the outcome of the Apple Music sign-in the onboarding asked for.
type AppleLoginMsg struct{ Err error }

// RestartMsg is sent when AUX has self-updated and needs to re-exec.
// The model handles it by quitting cleanly so the caller can exec the new binary.
type RestartMsg struct{}

// UpdateNoticeMsg tells the listener what an update did or found: this build
// is itself a fresh install (Installed, with its Tag), or a newer release
// exists that this build did not install. It is drawn once, as a toast on the
// player page — the one surface the scene has for host lines.
type UpdateNoticeMsg struct {
	Installed bool
	Tag       string
}

// text is the localized toast line for the notice.
func (m UpdateNoticeMsg) text() string {
	if m.Installed {
		return stage.Tr("AUX обновился до "+m.Tag, "AUX updated to "+m.Tag)
	}
	return stage.Tr("вышел AUX "+m.Tag, "AUX "+m.Tag+" is available")
}

// DebugLogMsg appends a line to the TUI debug log.
// Use this to surface background-goroutine events (e.g. scrobbling) without
// printing to stderr.
type DebugLogMsg string
