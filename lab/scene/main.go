// The AUX screen with a demo agent: a promo and a test bench for internal/stage.
//
//	go run ./lab/scene          interactive
//	go run ./lab/scene -demo    plays itself
//	go run ./lab/scene -onboard=false   skips the first-run onboarding
//	go run ./lab/scene -shots 3,6.5 -size 160x45   renders frames of the demo to /tmp/scene-*.png
//	go run ./lab/scene -demo -wait 45   the agent thinks as long as the real one
//	go run ./lab/scene -egg "Playboi Carti Stop Breathing@20"   opens the player on any catalog track to look at its egg; [ ] seek
//	go run ./lab/scene -music   the player and the music screen on the demo crate
//	go run ./lab/scene -music -musicstyle drawer|full   the other looks of that screen
//	go run ./lab/scene -music -script "1:right,2:enter" -shots 3   keys fed at simulated seconds
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// sceneUI is the thin tea model around the screen: it feeds the tick and the keys.
type sceneUI struct {
	s      *scene
	last   time.Time
	start  time.Time
	script []scriptKey
}

func (u *sceneUI) Init() tea.Cmd { return tick() }

func (u *sceneUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.s.m.SetSize(msg.Width, msg.Height)
	case tickMsg:
		now := time.Time(msg)
		dt := now.Sub(u.last).Seconds()
		u.last = now
		if len(u.script) > 0 {
			el := now.Sub(u.start).Seconds()
			for len(u.script) > 0 && u.script[0].at <= el {
				u.s.m.Key(u.script[0].k, u.script[0].text)
				u.script = u.script[1:]
			}
		}
		u.s.m.Step(min(dt, 0.1))
		return u, tick()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return u, tea.Quit
		}
		u.s.m.Key(msg.String(), msg.Text)
		if u.s.quit {
			return u, tea.Quit
		}
	}
	return u, nil
}

func (u *sceneUI) View() tea.View {
	v := tea.NewView(u.s.m.View())
	v.AltScreen = true
	return v
}

func main() {
	demo := flag.Bool("demo", false, "play the flow by itself")
	shots := flag.String("shots", "", "comma-separated demo seconds to render as PNG, then exit")
	out := flag.String("out", "/tmp/scene", "PNG prefix for -shots")
	size := flag.String("size", "160x45", "frame size for -shots")
	firstRun := flag.Bool("onboard", true, "start with the first-run onboarding")
	eggAt := flag.String("egg", "", `open the player on "artist title[@seconds]" to look at its egg`)
	musicFlag := flag.Bool("music", false, "open the player and the music screen on the demo crate")
	musicStyle := flag.String("musicstyle", "palette", "the look of the music screen: palette, drawer or full")
	scriptFlag := flag.String("script", "", `keys fed at simulated seconds: "1:right,2:enter"`)
	langFlag := flag.String("lang", "", "interface language: en or ru, empty picks from the environment")
	wait := flag.Float64("wait", 9, "seconds the fake agent thinks before its batch; the real one takes 40–70")
	flag.Parse()

	switch l := *langFlag; {
	case l == "":
		stage.SetLang(stage.DefaultLang())
	case strings.ToLower(l) == "ru":
		stage.SetLang("ru")
	default:
		stage.SetLang("en")
	}

	fmt.Fprint(os.Stderr, stage.Tr("обложки…", "covers…"))
	s := newScene(*firstRun && !*musicFlag, *demo && *eggAt == "", *musicStyle)
	s.wait = *wait
	coverTracks := demoTracks()
	if *musicFlag {
		coverTracks = append(coverTracks, s.mus.crate.coverTracks()...)
	}
	covers := loadCovers(coverTracks)
	fmt.Fprintf(os.Stderr, " %d\n", len(covers))
	for key, img := range covers {
		s.m.SetCover(key, img)
	}
	if *eggAt != "" {
		playEgg(s, *eggAt)
	}
	if *musicFlag {
		s.m.JumpPlay(demoTracks(), 0, 0)
		s.m.SetRecent(demoTracks())
		s.m.OpenMusic()
	}
	script := parseScript(*scriptFlag)

	if *shots != "" {
		var w, h int
		_, _ = fmt.Sscanf(*size, "%dx%d", &w, &h)
		s.m.SetSize(w, h)
		var times []float64
		for v := range strings.SplitSeq(*shots, ",") {
			f, _ := strconv.ParseFloat(v, 64)
			times = append(times, f)
		}
		const dt = 1.0 / 60
		var t float64
		for _, at := range times {
			for t+dt <= at {
				for len(script) > 0 && script[0].at <= t {
					s.m.Key(script[0].k, script[0].text)
					script = script[1:]
				}
				s.m.Step(dt)
				s.m.View()
				t += dt
			}
			for len(script) > 0 && script[0].at <= at {
				s.m.Key(script[0].k, script[0].text)
				script = script[1:]
			}
			p := fmt.Sprintf("%s-%08.3f.png", *out, at)
			if err := s.m.PNG(p); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println(p, s.m.Phase())
		}
		return
	}
	if _, err := tea.NewProgram(&sceneUI{s: s, last: time.Now(), start: time.Now(), script: script}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// scriptKey is one key fed at a simulated second: a name the stage knows and the text
// a single-rune key types.
type scriptKey struct {
	at   float64
	k    string
	text string
}

// parseScript reads "1.5:down,2:enter,2.5:a" in time order.
func parseScript(s string) []scriptKey {
	var out []scriptKey
	for _, part := range strings.Split(s, ",") {
		at, name, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			continue
		}
		sec, err := strconv.ParseFloat(strings.TrimSpace(at), 64)
		if err != nil {
			continue
		}
		name = strings.TrimSpace(name)
		k := scriptKey{at: sec, k: name}
		if len([]rune(name)) == 1 {
			k.text = name
		}
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at < out[j].at })
	return out
}

// playEgg opens the player: the named term searched in the Apple catalog as the
// first track in queue, the demo queue behind; without the network the demo
// track with the matching name plays, a key that matches nothing opens the
// player at the very start.
func playEgg(s *scene, spec string) {
	key, at, _ := strings.Cut(spec, "@")
	pos, _ := strconv.ParseFloat(at, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if t, art, err := lookupTrack(ctx, key); err == nil {
		tracks := append([]stage.Track{t}, demoTracks()...)
		if img, err := fetchImage(ctx, art); err == nil {
			s.m.SetCover(t.Key(), img)
		}
		fmt.Fprintf(os.Stderr, "egg: %s — %s · %s · %s · %.0fs\n", t.Artist, t.Title, t.Album, t.Released, t.Dur)
		s.m.JumpPlay(tracks, 0, pos)
		return
	}
	fmt.Fprintln(os.Stderr, "egg: offline, demo track")
	tracks := demoTracks()
	i := 0
	for j, tr := range tracks {
		if strings.EqualFold(tr.Artist+" "+tr.Title, key) {
			i = j
			break
		}
	}
	s.m.JumpPlay(tracks, i, pos)
}
