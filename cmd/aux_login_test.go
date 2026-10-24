package cmd

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/aux-player/internal/tui"
)

func TestAppleLoginLoopRetries(t *testing.T) {
	ask := make(chan struct{}, 2)
	ask <- struct{}{}
	ask <- struct{}{}
	var sent []tui.AppleLoginMsg
	calls := 0
	appleLoginLoop(ask, func() error {
		calls++
		if calls == 1 {
			return errors.New("первый заход сорвался")
		}
		return nil
	}, func(msg tea.Msg) {
		sent = append(sent, msg.(tui.AppleLoginMsg))
	})
	if len(sent) != 2 {
		t.Fatalf("sent = %d replies, want two", len(sent))
	}
	if sent[0].Err == nil {
		t.Error("the first reply must carry the failure")
	}
	if sent[1].Err != nil {
		t.Errorf("the second reply must be a success, got %v", sent[1].Err)
	}
}

func TestAppleLoginLoopWaits(t *testing.T) {
	ask := make(chan struct{}, 1)
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		appleLoginLoop(ask, func() error {
			calls.Add(1)
			return nil
		}, func(tea.Msg) {})
	}()
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("login ran %d times before anyone asked, want 0", got)
	}
	ask <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the loop must return after a successful sign-in")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("login ran %d times after one ask, want 1", got)
	}
}
