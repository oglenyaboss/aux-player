package cmd

import (
	tea "charm.land/bubbletea/v2"
	"github.com/simone-vibes/vibez/internal/tui"
)

// appleLoginLoop waits for the onboarding to ask, signs in, reports the result; a failed sign-in waits for the next ask.
func appleLoginLoop(ask <-chan struct{}, login func() error, send func(tea.Msg)) {
	for range ask {
		err := login()
		send(tui.AppleLoginMsg{Err: err})
		if err == nil {
			return
		}
	}
}
