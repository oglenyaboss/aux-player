package cmd

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
	playerpkg "github.com/oglenyaboss/aux-player/internal/player"
	"github.com/oglenyaboss/aux-player/internal/provider"
)

type fakeSource struct {
	subs atomic.Int32
	ch   chan playerpkg.State
}

func newFakeSource() *fakeSource {
	return &fakeSource{ch: make(chan playerpkg.State)}
}

func (s *fakeSource) Subscribe() <-chan playerpkg.State {
	s.subs.Add(1)
	return s.ch
}

type fakePresence struct {
	updates chan playerpkg.State
	closed  chan struct{}
	once    sync.Once
}

func newFakePresence() *fakePresence {
	return &fakePresence{updates: make(chan playerpkg.State, 16), closed: make(chan struct{})}
}

func (p *fakePresence) Update(st playerpkg.State) { p.updates <- st }

func (p *fakePresence) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

type fakeScrobble struct {
	updates chan playerpkg.State
}

func (s *fakeScrobble) Update(st playerpkg.State) { s.updates <- st }

func stateFor(title string) playerpkg.State {
	return playerpkg.State{Track: &provider.Track{ID: title, Title: title}, Playing: true}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func awaitPresence(t *testing.T, created <-chan *fakePresence) *fakePresence {
	t.Helper()
	select {
	case p := <-created:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("no presence service was created")
		return nil
	}
}

func recvState(t *testing.T, ch <-chan playerpkg.State, title string) {
	t.Helper()
	select {
	case st := <-ch:
		if st.Track == nil || st.Track.Title != title {
			t.Fatalf("got track %+v, want %q", st.Track, title)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no state %q reached the service", title)
	}
}

func assertNoState(t *testing.T, ch <-chan playerpkg.State) {
	t.Helper()
	select {
	case st := <-ch:
		t.Fatalf("service received track %+v, want none", st.Track)
	default:
	}
}

func withNoDiscord(t *testing.T, on bool) {
	t.Helper()
	old := noDiscord
	noDiscord = on
	t.Cleanup(func() { noDiscord = old })
}

func discordFactory(created chan<- *fakePresence) func() presence {
	return func() presence {
		p := newFakePresence()
		created <- p
		return p
	}
}

func waitDiscordSeen(t *testing.T, d *discordSwitch, title string) {
	t.Helper()
	waitFor(t, "the Discord switch to see "+title, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.last != nil && d.last.Track != nil && d.last.Track.Title == title
	})
}

func waitLastfmSeen(t *testing.T, l *lastfmSwitch, title string) {
	t.Helper()
	waitFor(t, "the Last.fm switch to see "+title, func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.last != nil && l.last.Track != nil && l.last.Track.Title == title
	})
}

func TestDiscordSwitchTogglesLive(t *testing.T) {
	withNoDiscord(t, false)
	d := newDiscordSwitch(&config.Config{}, nil)
	created := make(chan *fakePresence, 4)
	d.start = discordFactory(created)
	src := newFakeSource()
	defer close(src.ch)
	d.Attach(src)

	src.ch <- stateFor("one")
	p1 := awaitPresence(t, created)
	recvState(t, p1.updates, "one")

	d.Set(false)
	select {
	case <-p1.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Set(false) must close the presence service")
	}

	src.ch <- stateFor("two")
	waitDiscordSeen(t, d, "two")
	assertNoState(t, p1.updates)
	select {
	case <-created:
		t.Fatal("a service was created while Discord was off")
	default:
	}

	d.Set(true)
	p2 := awaitPresence(t, created)
	recvState(t, p2.updates, "two")

	src.ch <- stateFor("three")
	recvState(t, p2.updates, "three")

	select {
	case <-created:
		t.Fatal("a third service was created")
	default:
	}
	if got := src.subs.Load(); got != 1 {
		t.Fatalf("Subscribe called %d times, want 1", got)
	}
}

func TestDiscordSwitchSetTwiceAndClose(t *testing.T) {
	withNoDiscord(t, false)
	d := newDiscordSwitch(&config.Config{}, nil)
	created := make(chan *fakePresence, 4)
	d.start = discordFactory(created)
	d.Set(true)
	p := awaitPresence(t, created)
	d.Set(true)
	select {
	case <-created:
		t.Fatal("Set(true) twice created a second service")
	default:
	}
	d.Close()
	select {
	case <-p.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close must close the service")
	}
	d.Close()
	newDiscordSwitch(&config.Config{}, nil).Close()
}

func TestDiscordSwitchStartsOff(t *testing.T) {
	off := false
	cases := []struct {
		name  string
		setup func(*testing.T)
		cfg   *config.Config
	}{
		{"--no-discord", func(t *testing.T) { withNoDiscord(t, true) }, &config.Config{}},
		{"config off", func(*testing.T) {}, &config.Config{DiscordRPC: &off}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			d := newDiscordSwitch(tc.cfg, nil)
			created := make(chan *fakePresence, 4)
			d.start = discordFactory(created)
			src := newFakeSource()
			defer close(src.ch)
			d.Attach(src)

			src.ch <- stateFor("one")
			waitDiscordSeen(t, d, "one")
			select {
			case <-created:
				t.Fatal("a service was created at launch with Discord off")
			default:
			}

			d.Set(true)
			p := awaitPresence(t, created)
			recvState(t, p.updates, "one")
		})
	}
}

func TestLastfmSwitchStoredSession(t *testing.T) {
	orig := newLastfmScrobbler
	t.Cleanup(func() { newLastfmScrobbler = orig })
	updates := make(chan playerpkg.State, 16)
	var gotKey, gotSecret, gotSession string
	newLastfmScrobbler = func(key, secret, session string, _ func(string)) scrobble {
		gotKey, gotSecret, gotSession = key, secret, session
		return &fakeScrobble{updates: updates}
	}

	cfg := &config.Config{LastfmAPIKey: "key", LastfmAPISecret: "secret", LastfmSessionKey: "s0"}
	l := newLastfmSwitch(cfg, nil)
	if gotKey != "key" || gotSecret != "secret" || gotSession != "s0" {
		t.Fatalf("scrobbler made with (%q, %q, %q), want (key, secret, s0)", gotKey, gotSecret, gotSession)
	}

	src := newFakeSource()
	defer close(src.ch)
	l.Attach(src)
	src.ch <- stateFor("one")
	recvState(t, updates, "one")
	if got := src.subs.Load(); got != 1 {
		t.Fatalf("Subscribe called %d times, want 1", got)
	}
}

func TestLastfmSwitchSessionChanges(t *testing.T) {
	cfg := &config.Config{LastfmAPIKey: "key", LastfmAPISecret: "secret"}
	l := newLastfmSwitch(cfg, nil)
	var calls []string
	var got *fakeScrobble
	l.make = func(key, secret, session string) scrobble {
		calls = append(calls, key+" "+secret+" "+session)
		got = &fakeScrobble{updates: make(chan playerpkg.State, 16)}
		return got
	}
	src := newFakeSource()
	defer close(src.ch)
	l.Attach(src)

	src.ch <- stateFor("one")
	waitLastfmSeen(t, l, "one")
	if len(calls) != 0 {
		t.Fatalf("a scrobbler was made before a session: %v", calls)
	}

	l.Set("s1")
	if len(calls) != 1 || calls[0] != "key secret s1" {
		t.Fatalf("scrobbler made with %v, want [key secret s1]", calls)
	}
	src.ch <- stateFor("two")
	recvState(t, got.updates, "two")

	l.Set("")
	src.ch <- stateFor("three")
	waitLastfmSeen(t, l, "three")
	assertNoState(t, got.updates)

	if got := src.subs.Load(); got != 1 {
		t.Fatalf("Subscribe called %d times, want 1", got)
	}
}

func TestLastfmSwitchMissingAppKey(t *testing.T) {
	var logs []string
	l := newLastfmSwitch(&config.Config{}, func(msg string) { logs = append(logs, msg) })
	l.Set("s1")
	if l.cur != nil {
		t.Fatal("a scrobbler was made without an app key")
	}
	if len(logs) == 0 {
		t.Fatal("no log line about the missing app key")
	}
}
