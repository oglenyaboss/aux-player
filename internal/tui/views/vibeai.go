package views

import "time"

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
