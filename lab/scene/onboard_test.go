package main

import (
	"strings"
	"testing"
)

func TestOnboardProviderFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 9.5)
	for _, s := range []string{"откуда брать модель", "OpenRouter", "OpenCode Go", "свой адрес"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр 9.5 без %q", s)
		}
	}
	if row := rowWith(frame, "OpenRouter"); !strings.Contains(row, "›") {
		t.Errorf("строка с OpenRouter без ›: %q", row)
	}
}

func TestOnboardKeyFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 11.0)
	for _, s := range []string{"ключ OpenRouter", "sk-or-", "•"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр 11.0 без %q", s)
		}
	}
	if strings.Contains(frame, "4f9a2c71") {
		t.Errorf("кадр 11.0 показывает незамаскированный хвост ключа")
	}
}

func TestOnboardLoadingFrame(t *testing.T) {
	if frame := frameText(t, 110, 32, 12.0); !strings.Contains(frame, "спрашиваю список моделей") {
		t.Errorf("кадр 12.0 без загрузки:\n%s", frame)
	}
}

func TestOnboardModelFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 13.6)
	for _, s := range []string{"/flash", "deepseek/deepseek-v4.1-flash", "рекомендуем", "с инструментами из 312"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр 13.6 без %q", s)
		}
	}
	if strings.Contains(frame, "llama") {
		t.Errorf("кадр 13.6 показывает llama вне фильтра flash")
	}
}

func TestOnboardEffortFrame(t *testing.T) {
	frame := frameText(t, 110, 32, 15.5)
	for _, s := range []string{"рассуждение", "низкий", "средний", "высокий", "быстрее, подборки проще", "▔", "deepseek-v4.1-flash"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр 15.5 без %q", s)
		}
	}
}

func TestOnboardPanelStatus(t *testing.T) {
	frame := frameText(t, 110, 32, 16.8)
	if row := rowWith(frame, "Агент"); !strings.Contains(row, "OpenRouter · deepseek-v4.1-flash") {
		t.Errorf("строка с Агент без статуса: %q", row)
	}
}

func TestOnboardSmallScreens(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 24}} {
		for at := 8.0; at <= 17.0; at += 0.5 {
			frameText(t, size[0], size[1], at)
		}
	}
}
