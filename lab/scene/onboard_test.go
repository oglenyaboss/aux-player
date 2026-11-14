package main

import (
	"strings"
	"testing"
)

func TestOnboardProviderFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 9.5)
	for _, s := range []string{"откуда брать модель", "OpenRouter", "OpenCode Go", "свой адрес"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the frame 9.5 is without %q", s)
		}
	}
	if row := rowWith(frame, "OpenRouter"); !strings.Contains(row, "›") {
		t.Errorf("the row with OpenRouter has no ›: %q", row)
	}
}

func TestOnboardKeyFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 11.0)
	for _, s := range []string{"ключ OpenRouter", "sk-or-", "•"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the frame 11.0 is without %q", s)
		}
	}
	if strings.Contains(frame, "4f9a2c71") {
		t.Errorf("the frame 11.0 shows an unmasked key tail")
	}
}

func TestOnboardLoadingFrame(t *testing.T) {
	if frame := frameText(t, 110, 32, 12.0); !strings.Contains(frame, "спрашиваю список моделей") {
		t.Errorf("the frame 12.0 is without loading:\n%s", frame)
	}
}

func TestOnboardModelFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 13.6)
	for _, s := range []string{"/flash", "deepseek/deepseek-v4.1-flash", "рекомендуем", "с инструментами из 312"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the frame 13.6 is without %q", s)
		}
	}
	if strings.Contains(frame, "llama") {
		t.Errorf("the frame 13.6 shows llama outside the flash filter")
	}
}

func TestOnboardEffortFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 15.5)
	for _, s := range []string{"рассуждение", "низкий", "средний", "высокий", "быстрее, подборки проще", "▔", "deepseek-v4.1-flash"} {
		if !strings.Contains(frame, s) {
			t.Errorf("the frame 15.5 is without %q", s)
		}
	}
}

func TestOnboardPanelStatus(t *testing.T) {
	frame := frameText(t, 110, 32, 16.8)
	if row := rowWith(frame, "Агент"); !strings.Contains(row, "OpenRouter · deepseek-v4.1-flash") {
		t.Errorf("the agent row has no status: %q", row)
	}
}

func TestOnboardSmallScreens(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 24}} {
		for at := 8.0; at <= 17.0; at += 0.5 {
			frameText(t, size[0], size[1], at)
		}
	}
}
