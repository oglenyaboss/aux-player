package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/oglenyaboss/aux-player/internal/provider"
)

// PlaybackID returns the best ID to use for MusicKit queue descriptors.
// Library tracks (IDs prefixed with "i.") must use their library ID directly
// so MusicKit never encounters a CONTENT_RESTRICTED error — the catalog copy
// of a track may be region-locked even when the user already owns it in their
// library. Catalog IDs are used for tracks not present in the user's library.
func PlaybackID(t provider.Track) string {
	// Library tracks must use their library ID directly — never CONTENT_RESTRICTED.
	if strings.HasPrefix(t.ID, "i.") {
		return t.ID
	}
	if t.CatalogID != "" {
		return t.CatalogID
	}
	return t.ID
}

// FormatDuration formats a duration as m:ss.
func FormatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	mins := secs / 60
	secs %= 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

type VibeAIQuestion struct {
	Question    string
	Kind        string
	Options     []string
	Left, Right string
	Steps       int
	Default     string
}

type VibeAITrack struct {
	ID                              string
	Artist, Title, Album, Year, Why string
	Favorite                        bool
	ArtworkURL                      string
	Dur                             time.Duration
}

// VibeAISubmitMsg asks the model to run an agent turn. Fresh starts a new
// session (a new request); otherwise Text is a reaction within the session.
type VibeAISubmitMsg struct {
	Text  string
	Fresh bool
}

// VibeAIAnswersMsg carries raw answers in the agent's format:
// "" = default, "N" = option number, anything else = the user's own answer.
type VibeAIAnswersMsg struct{ Answers []string }

// VibeAILaterMsg reports an answer to a mid-stream question: Raw is the option
// number the user typed ("1".."N"); esc dismisses the question without a msg.
type VibeAILaterMsg struct{ Raw string }

type VibeAICancelMsg struct{}

type VibeAIAction int

const (
	VibeAIPlay VibeAIAction = iota
	VibeAIQueue
	VibeAISave
	VibeAIStream
)

// VibeAIActionMsg carries play/queue/save/stream commands. For Play, Index is
// the ordinal of a feed track (0-based across every portion).
type VibeAIActionMsg struct {
	Action VibeAIAction
	Index  int
}

// VibeAIRateMsg carries the owner's rating of a feed track: Index is its
// ordinal across every portion, Rating is 1 (more), -1 (less) or 0 (cleared).
type VibeAIRateMsg struct {
	Index, Rating int
}
