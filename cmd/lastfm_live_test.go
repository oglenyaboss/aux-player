package cmd

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/simone-vibes/vibez/internal/config"
	playerpkg "github.com/simone-vibes/vibez/internal/player"
)

type countingPlayer struct {
	subs atomic.Int32
}

func (p *countingPlayer) Subscribe() <-chan playerpkg.State {
	p.subs.Add(1)
	return make(chan playerpkg.State)
}

func TestScrobbleLoopStartsOnSignal(t *testing.T) {
	cfg := &config.Config{LastfmAPIKey: "key", LastfmAPISecret: "secret"}
	p := &countingPlayer{}
	ch := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		lastfmScrobbleLoop(ch, cfg, p, func(string) {})
	}()
	time.Sleep(20 * time.Millisecond)
	if got := p.subs.Load(); got != 0 {
		t.Fatalf("scrobbler subscribed %d times before any session, want 0", got)
	}
	cfg.LastfmSessionKey = "s"
	ch <- struct{}{}
	if !waitSubs(p, 1) {
		t.Fatal("scrobbler did not subscribe after the session signal")
	}
	ch <- struct{}{}
	close(ch)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the loop must return once the channel is closed")
	}
	if got := p.subs.Load(); got != 1 {
		t.Fatalf("scrobbler subscribed %d times, want exactly 1", got)
	}
}

func TestScrobbleLoopStartsAtOnce(t *testing.T) {
	cfg := &config.Config{LastfmAPIKey: "key", LastfmAPISecret: "secret", LastfmSessionKey: "s"}
	p := &countingPlayer{}
	ch := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		lastfmScrobbleLoop(ch, cfg, p, func(string) {})
	}()
	if !waitSubs(p, 1) {
		t.Fatal("the scrobbler must start at once with a stored session")
	}
	ch <- struct{}{}
	ch <- struct{}{}
	close(ch)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the loop must return once the channel is closed")
	}
	if got := p.subs.Load(); got != 1 {
		t.Fatalf("scrobbler subscribed %d times, want exactly 1", got)
	}
}

func waitSubs(p *countingPlayer, want int32) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p.subs.Load() >= want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
