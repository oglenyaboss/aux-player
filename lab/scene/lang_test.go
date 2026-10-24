package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

func TestMain(m *testing.M) {
	stage.SetLang("ru")
	os.Exit(m.Run())
}

// langEN switches to English and restores Russian when the test ends.
func langEN(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { stage.SetLang("ru") })
	stage.SetLang("en")
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// frameText renders the demo frame at `at` seconds into plain letters, like the old
// frameAt over the model internals: fixed 30 fps tick, a render on every step.
func frameText(t *testing.T, w, h int, at float64) string {
	t.Helper()
	s := newScene(true, true)
	s.m.SetSize(w, h)
	const dt = 1.0 / 30
	var clock float64
	for clock+dt <= at {
		s.m.Step(dt)
		s.m.View()
		clock += dt
	}
	return stripANSI(s.m.View())
}

func rowWith(frame, sub string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestLangEnFramesHaveNoCyrillic(t *testing.T) {
	langEN(t)
	for _, at := range []float64{5.8, 9.5, 13.6, 15.5, 16.8, 25, 35, 40, 44, 47.5, 52, 57, 60} {
		frame := frameText(t, 110, 32, at)
		if hasCyrillic(strings.ReplaceAll(frame, "Русский", "")) {
			t.Errorf("кадр %v на английском содержит кириллицу:\n%s", at, frame)
		}
	}
}

func TestLangEnScanAndPlayText(t *testing.T) {
	langEN(t)
	frame := frameText(t, 110, 32, 40)
	for _, s := range []string{"Batch", "of 20", "passed"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр сканирования 40 без %q:\n%s", s, frame)
		}
	}
	if frame := frameText(t, 110, 32, 57); !strings.Contains(frame, "noted: more like this") {
		t.Errorf("кадр плеера 57 без тоста:\n%s", frame)
	}
}

// The help overlay is toggled by "?" in the player now: the demo frames render it there.
func TestLangEnHelpFrame(t *testing.T) {
	langEN(t)
	s := newScene(false, false)
	s.m.SetSize(110, 32)
	s.m.JumpPlay(demoTracks(), 0, 0)
	s.m.Key("?", "")
	frame := stripANSI(s.m.View())
	for _, k := range []string{"talk to the agent", "more like this", "less like this", "next", "save to playlist", "start over", "quit"} {
		if !strings.Contains(frame, k) {
			t.Errorf("справка без %q:\n%s", k, frame)
		}
	}
	if hasCyrillic(frame) {
		t.Errorf("справка на английском содержит кириллицу:\n%s", frame)
	}
}

func TestLangPickerEnFrame(t *testing.T) {
	langEN(t)
	frame := frameText(t, 110, 32, 5.8)
	for _, s := range []string{"English", "Русский"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр выбора языка без %q:\n%s", s, frame)
		}
	}
}

func TestLangModelStepEn(t *testing.T) {
	langEN(t)
	frame := frameText(t, 110, 32, 13.6)
	if !strings.Contains(frame, "recommended") {
		t.Errorf("кадр 13.6 без recommended:\n%s", frame)
	}
	if strings.Contains(frame, "fetching models") {
		t.Errorf("кадр 13.6 всё ещё показывает загрузку:\n%s", frame)
	}
	if frame := frameText(t, 110, 32, 12.0); !strings.Contains(frame, "fetching models") {
		t.Errorf("кадр 12.0 без fetching models:\n%s", frame)
	}
}

func TestLangEnQueryAndHint(t *testing.T) {
	langEN(t)
	// the old test ran the intro without onboarding; replay the same 4 seconds
	s := newScene(false, true)
	s.m.SetSize(110, 32)
	const dt = 1.0 / 30
	var clock float64
	for clock+dt <= 4.0 {
		s.m.Step(dt)
		s.m.View()
		clock += dt
	}
	frame := stripANSI(s.m.View())
	for _, s := range []string{"say what you want to hear", "like Don Toliver but not him, late night drive"} {
		if !strings.Contains(frame, s) {
			t.Errorf("интро без %q:\n%s", s, frame)
		}
	}
	if hasCyrillic(frame) {
		t.Errorf("интро на английском содержит кириллицу:\n%s", frame)
	}
}

func TestLangEnSmallScreensNoPanic(t *testing.T) {
	langEN(t)
	for at := 5.5; at <= 17.0; at += 0.5 {
		frameText(t, 60, 16, at)
	}
}
