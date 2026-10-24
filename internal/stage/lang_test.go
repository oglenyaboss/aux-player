package stage

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	SetLang("ru")
	os.Exit(m.Run())
}

// langEN switches to English and restores Russian when the test ends,
// so the untouched onboard_test.go and backdrop_test.go keep running in Russian.
func langEN(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
}

func TestLangPickerToggle(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("ru")
	m := New(Options{Onboard: true})
	m.SetSize(110, 32)
	o := m.ob
	o.t = 6
	for want := true; ; want = !want {
		m.onboardKey("right", "")
		if want && lang != "en" {
			t.Fatalf("right: lang == %q, ждём en", lang)
		}
		m.onboardKey("left", "")
		if want && lang != "ru" {
			t.Fatalf("left: lang == %q, ждём ru", lang)
		}
		if !want {
			break
		}
	}
	m.onboardKey("enter", "")
	if !o.langDone {
		t.Errorf("enter не завершил выбор языка")
	}
	if lang != "ru" {
		t.Errorf("enter переключил язык на %q", lang)
	}
}

func TestDefaultLang(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	if got := DefaultLang(); got != "ru" {
		t.Errorf("DefaultLang() == %q, ждём ru", got)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if got := DefaultLang(); got != "en" {
		t.Errorf("DefaultLang() == %q, ждём en", got)
	}
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	if got := DefaultLang(); got != "ru" {
		t.Errorf("DefaultLang() с пустым LANG == %q, ждём ru", got)
	}
}
