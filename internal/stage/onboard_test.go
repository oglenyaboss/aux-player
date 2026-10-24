package stage

import (
	"strings"
	"testing"
)

func dump(f *fb) string {
	var b strings.Builder
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			b.WriteRune(f.c[y*f.w+x].ch)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestOnboardURLStep(t *testing.T) {
	m := New(Options{Onboard: true})
	m.SetSize(110, 32)
	o := m.ob
	o.t, o.stepT, o.step, o.sub = 10, 10, 1, SubURL
	o.langDone = true
	o.errT = o.t - 0.4
	frame := dump(m.render())
	for _, s := range []string{"адрес OpenAI-совместимого API", "›", "▎", "нужен адрес", "enter дальше", "esc назад"} {
		if !strings.Contains(frame, s) {
			t.Errorf("кадр subURL без %q", s)
		}
	}
	o.url = []rune(strings.Repeat("а", 45))
	frame = dump(m.render())
	run, best := 0, 0
	for _, r := range frame {
		if r == 'а' {
			run++
			best = max(best, run)
		} else {
			run = 0
		}
	}
	if best != 39 {
		t.Errorf("subURL показывает %d подряд рун адреса, ждём последние w-6 = 39", best)
	}
}

func TestOnboardKeyVariants(t *testing.T) {
	m := New(Options{Onboard: true})
	m.SetSize(110, 32)
	o := m.ob
	o.t, o.stepT, o.step, o.sub = 10, 10, 1, SubKey
	o.langDone = true
	o.prov, o.errT = 5, o.t-0.4
	frame := dump(m.render())
	for _, s := range []string{"ключ, если нужен", "нужен ключ"} {
		if !strings.Contains(frame, s) {
			t.Errorf("подшаг ключа без %q (провайдер без ключей)", s)
		}
	}
	o.prov, o.errT = 0, -1
	o.key = []rune("sk-or-v1-4f9a2c71e0b8d3" + strings.Repeat("б", 45-len("sk-or-v1-4f9a2c71e0b8d3")))
	frame = dump(m.render())
	if strings.Contains(frame, "sk-or-") || !strings.Contains(frame, "•") {
		t.Errorf("маскированная строка длиннее w-6 не обрезана до последних рун")
	}
}

func TestOnboardEscFromKey(t *testing.T) {
	m := New(Options{Onboard: true})
	m.ob.t = 10
	m.ob.step = 1
	m.ob.langDone = true
	m.ob.sub = SubKey
	m.onboardKey("esc", "")
	if m.ob.sub != SubProvider {
		t.Errorf("esc из ключа должен вернуть SubProvider, стало %v", m.ob.sub)
	}
}

func TestOnboardEmptyFilter(t *testing.T) {
	m := New(Options{Onboard: true})
	m.SetSize(110, 32)
	o := m.ob
	o.t, o.step, o.sub = 10, 1, SubModel
	o.langDone = true
	o.filter = []rune("zzz")
	frame := dump(m.render())
	if !strings.Contains(frame, "ничего не нашлось") {
		t.Errorf("пустой фильтр без «ничего не нашлось»:\n%s", frame)
	}
}
