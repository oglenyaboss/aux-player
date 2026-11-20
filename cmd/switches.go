package cmd

import (
	"sync"

	"github.com/oglenyaboss/aux-player/internal/config"
	"github.com/oglenyaboss/aux-player/internal/discord"
	"github.com/oglenyaboss/aux-player/internal/lastfm"
	playerpkg "github.com/oglenyaboss/aux-player/internal/player"
)

type stateSource interface {
	Subscribe() <-chan playerpkg.State
}

type presence interface {
	Update(playerpkg.State)
	Close() error
}

type scrobble interface {
	Update(playerpkg.State)
}

type discordSwitch struct {
	mu       sync.Mutex
	on       bool
	svc      presence
	last     *playerpkg.State
	attached bool
	start    func() presence
}

func newDiscordSwitch(cfg *config.Config, log func(string)) *discordSwitch {
	clientID := cfg.DiscordClientID
	if clientID == "" {
		clientID = discord.DefaultClientID
	}
	return &discordSwitch{
		on: !noDiscord && cfg.DiscordRPCEnabled(),
		start: func() presence {
			svc := discord.NewService(clientID)
			if log != nil {
				svc.SetLogger(log)
			}
			return svc
		},
	}
}

func (d *discordSwitch) Attach(p stateSource) {
	d.mu.Lock()
	if d.attached {
		d.mu.Unlock()
		return
	}
	d.attached = true
	d.mu.Unlock()
	go func() {
		for st := range p.Subscribe() {
			d.mu.Lock()
			if d.on {
				if d.svc == nil {
					d.svc = d.start()
				}
				d.svc.Update(st)
			}
			last := st
			d.last = &last
			d.mu.Unlock()
		}
	}()
}

func (d *discordSwitch) Set(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.on = on
	if !on {
		if d.svc != nil {
			_ = d.svc.Close()
			d.svc = nil
		}
		return
	}
	if d.svc == nil {
		d.svc = d.start()
		if d.last != nil {
			d.svc.Update(*d.last)
		}
	}
}

func (d *discordSwitch) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.on = false
	if d.svc != nil {
		_ = d.svc.Close()
		d.svc = nil
	}
}

type lastfmSwitch struct {
	mu       sync.Mutex
	cfg      *config.Config
	log      func(string)
	cur      scrobble
	last     *playerpkg.State
	attached bool
	make     func(key, secret, session string) scrobble
}

var newLastfmScrobbler = func(key, secret, session string, log func(string)) scrobble {
	client := lastfm.NewClient(key, secret, session)
	s := lastfm.NewScrobbler(client)
	s.SetLogger(log)
	return s
}

func newLastfmSwitch(cfg *config.Config, log func(string)) *lastfmSwitch {
	l := &lastfmSwitch{
		cfg: cfg,
		log: log,
		make: func(key, secret, session string) scrobble {
			return newLastfmScrobbler(key, secret, session, log)
		},
	}
	if cfg.LastfmAPIKey != "" && cfg.LastfmAPISecret != "" && cfg.LastfmSessionKey != "" {
		l.cur = l.make(cfg.LastfmAPIKey, cfg.LastfmAPISecret, cfg.LastfmSessionKey)
	}
	return l
}

func (l *lastfmSwitch) Attach(p stateSource) {
	l.mu.Lock()
	if l.attached {
		l.mu.Unlock()
		return
	}
	l.attached = true
	l.mu.Unlock()
	go func() {
		for st := range p.Subscribe() {
			l.mu.Lock()
			if l.cur != nil {
				l.cur.Update(st)
			}
			last := st
			l.last = &last
			l.mu.Unlock()
		}
	}()
}

func (l *lastfmSwitch) Set(session string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if session == "" {
		l.cur = nil
		return
	}
	if l.cfg.LastfmAPIKey == "" || l.cfg.LastfmAPISecret == "" {
		l.cur = nil
		if l.log != nil {
			l.log("[lastfm] scrobbling off: no app key or secret configured")
		}
		return
	}
	l.cur = l.make(l.cfg.LastfmAPIKey, l.cfg.LastfmAPISecret, session)
}
