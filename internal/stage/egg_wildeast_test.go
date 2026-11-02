package stage

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func weT(title string, dur float64) Track {
	return Track{Artist: "SALUKI", Title: title, Album: "WILD EA$T", Dur: dur}
}

var (
	weOgney  = weT("ОГНЕЙ", 182)
	weHaha   = weT("HAHAHA", 116)
	weTitle  = weT("WILD EA$T", 266)
	weKreem  = weT("ЗВУЧИТ КАК ПЛАН (Kreem's Intro)", 75)
	weCold   = weT("ТАК ЛЕГКО ЗАМЁРЗНУТЬ", 81)
	weGlugi  = weT("GLUGICIR (feat. SENSEE)", 215)
	weMoskva = weT("MOSKVICHKA (feat. Найк Борзов)", 148)
	weWitchT = weT("ВЕДЬМА", 227)
	weHomeT  = weT("HOME (feat. Basta)", 212)
)

// weCrowdCover is a field of grey bodies: a pattern with structure for the crowd to move.
func weCrowdCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := range 120 {
		for x := range 120 {
			v := uint8(40 + 180*hash(x/4, y/3, 9))
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

// weCrowdImg is the field of grey bodies drawn once: every frame test reuses it.
var weCrowdImg = weCrowdCover()

func weFrame(t *testing.T, o Options, tr Track, at, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.SetCover(tr.Key(), weCrowdImg)
	m.JumpPlay([]Track{tr, hvCreepin}, 0, at)
	steps(m, secs)
	return m, m.render()
}

func weChroma(c col) float64 { return max(c.r, c.g, c.b) - min(c.r, c.g, c.b) }

func TestWERule(t *testing.T) {
	m := New(Options{})
	for _, tr := range []Track{weOgney, weTitle, weGlugi, {Artist: "SALUKI", Title: "THE GOOD, THE BAD & THE UGLY", Album: "Wild Ea$T"}} {
		if e := m.eggOf(tr); e == nil || e.theme != &weTheme {
			t.Errorf("%s: egg %+v", tr.Title, e)
		}
	}
	if e := m.eggOf(Track{Artist: "SALUKI", Title: "SORRY I'M WORKING ON THE WILD EA$T", Album: "Sorry I'M Working On the Wild Ea$T - Single"}); e != nil {
		t.Errorf("сингл поймал пасхалку")
	}
}

// Titles fold to the table whatever Apple does with case, commas and ё.
func TestWEKeys(t *testing.T) {
	for title, mode := range map[string]weMode{
		"THE GOOD, THE BAD & THE UGLY":    weBang,
		"The Good the Bad & the Ugly":     weBang,
		"Так Легко Замерзнуть":            weSnow,
		"ЗВУЧИТ КАК ПЛАН (Kreem’s Intro)": weWake,
		"Pixies & Fairies":                weBang,
		"Moskvichka (feat. Найк Борзов)":  weDrive,
		"Wild Ea$T": weStill,
	} {
		if got := weTracks[weKey(title)].mode; got != mode {
			t.Errorf("%q: режим %d, ждём %d", title, got, mode)
		}
	}
}

// The film is black and white outside the cover on a still track.
func TestWEFilm(t *testing.T) {
	m, f := weFrame(t, Options{}, weTitle, 40, 6)
	if n := count(m, f, func(c col) bool { return weChroma(c) > 0.03 }); n > 0 {
		t.Errorf("%d цветных пикселей на ч/б плёнке", n)
	}
}

// The camera drifts over the field with the playback and holds on pause.
func TestWECamera(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{weTitle}, 0, 40)
	m.Now(weTitle.Key(), 40, true)
	steps(m, 2)
	a := weCamera(m, weNow(m))
	steps(m, 2)
	if weCamera(m, weNow(m)) <= a {
		t.Fatalf("камера стоит во время игры")
	}
	m.Now(weTitle.Key(), m.pos, false)
	steps(m, 0.2)
	b := weCamera(m, weNow(m))
	steps(m, 2)
	if weCamera(m, weNow(m)) != b {
		t.Errorf("на паузе камера плывёт")
	}
}

// On a banger the crowd jumps to the measured tempo: the beat clock runs at bpm/60,
// and a frame on the beat differs from a frame half a beat later.
func TestWEBang(t *testing.T) {
	m, _ := weFrame(t, Options{}, weHaha, 30, 4)
	phi, amp := weBeat(m, weNow(m))
	if amp < 0.9 {
		t.Fatalf("толпа не качает: amp %.2f", amp)
	}
	if want := m.pos * 95.3 / 60; phi != want {
		t.Errorf("бит %.3f, ждём %.3f", phi, want)
	}
	m, _ = weFrame(t, Options{}, weTitle, 30, 4)
	if _, amp := weBeat(m, weNow(m)); amp != 0 {
		t.Errorf("на спокойном треке толпа качает")
	}
}

// On a banger the crowd is on its feet: dark silhouettes along the bottom against the
// haze of the stage, and they jump: a frame half a beat later differs.
func TestWEStage(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{weHaha}, 0, 30)
	steps(m, 3)
	tr := weNow(m)
	H := 2 * m.h
	band := func(y0, y1 int) (dark int) {
		for py := y0; py < y1; py++ {
			for px := 0; px < m.w; px++ {
				if weStage(m, tr, px, py).lum() < 0.04 {
					dark++
				}
			}
		}
		return dark
	}
	if top, bottom := band(0, H/4), band(H*3/4, H); top > 0 || bottom < m.w*(H/4)/3 {
		t.Errorf("тёмных пикселей вверху %d, внизу %d; ждём силуэты толпы внизу", top, bottom)
	}
	frame := func() (s float64) {
		for py := H / 2; py < H; py++ {
			for px := 0; px < m.w; px++ {
				s += weStage(m, tr, px, py).lum()
			}
		}
		return s
	}
	a := frame()
	m.pos += 30 / tr.bpm
	if frame() == a {
		t.Errorf("толпа не прыгает")
	}
	m = New(Options{})
	m.JumpPlay([]Track{weTitle}, 0, 30)
	if weStanding(weNow(m)) {
		t.Errorf("на спокойном треке толпа стоит")
	}
}

// Lighters on Огней: flames on the raised hands, the only colour on the film.
func TestWELighters(t *testing.T) {
	m, f := weFrame(t, Options{}, weOgney, 60, 4)
	if warm := count(m, f, func(c col) bool { return c.r > c.b+0.25 && c.r > 0.4 }); warm < 20 {
		t.Errorf("%d пикселей огоньков", warm)
	}
	m, f = weFrame(t, Options{}, weHaha, 60, 4)
	if n := count(m, f, func(c col) bool { return weChroma(c) > 0.03 }); n > 0 {
		t.Errorf("на HAHAHA %d цветных пикселей", n)
	}
}

// Kreem's intro: the crowd gets up over the track, rising from below the screen.
func TestWEWake(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{weKreem}, 0, 2)
	tr := weNow(m)
	dark := func() (n int) {
		for px := 0; px < m.w; px++ {
			if weStage(m, tr, px, 2*m.h*9/10).lum() < 0.04 {
				n++
			}
		}
		return n
	}
	early := dark()
	m.JumpPlay([]Track{weKreem}, 0, 70)
	if late := dark(); early > 0 || late < m.w/4 {
		t.Errorf("силуэтов внизу: в начале %d, к концу %d", early, late)
	}
}

// Snow on the cold songs: flakes over the backdrop, never over letters.
func TestWESnow(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{weCold}, 0, 30)
	flakes := func() (*fb, int) {
		f := newFB(m.w, m.h)
		f.fill(0, 0, m.w, m.h, col{})
		f.text(10, 5, "буквы", white)
		weSnowfall(m, f, m.h)
		n := 0
		for i := range f.c {
			if f.c[i].pix {
				n++
			}
		}
		return f, n
	}
	f, n := flakes()
	if n < 60 {
		t.Errorf("%d снежинок", n)
	}
	if s := stripANSI(f.String()); !strings.Contains(s, "буквы") {
		t.Errorf("снег лёг на буквы")
	}
	if mode := weNow(m).mode; mode != weSnow {
		t.Errorf("режим %d, ждём снег", mode)
	}
}

// Glugicir: the mantra under the cover while it is sung, the last words after.
func TestWEGlugicir(t *testing.T) {
	_, f := weFrame(t, Options{}, weGlugi, 150, 3)
	if s := stripANSI(f.String()); strings.Contains(s, "Д И К О Й") {
		t.Errorf("мантра до 2:41")
	}
	_, f = weFrame(t, Options{}, weGlugi, 165, 3)
	if s := stripANSI(f.String()); !strings.Contains(s, "О С Т А В А Й С Я   Д И К О Й") {
		t.Errorf("нет мантры на 2:48")
	}
	_, f = weFrame(t, Options{}, weGlugi, 195, 2)
	if s := stripANSI(f.String()); !strings.Contains(s, "Д И К И Й   В О С Т О К") {
		t.Errorf("нет последних слов на 3:17")
	}
	m, _ := weFrame(t, Options{}, weGlugi, 140, 3)
	if g := weGrain(m, weNow(m)); g > 0.2 {
		t.Errorf("после бриджа зерно не улеглось: %.2f", g)
	}
	a := weCamera(m, weNow(m))
	steps(m, 3)
	if weCamera(m, weNow(m)) != a {
		t.Errorf("после бриджа камера плывёт")
	}
}

// The finale: headlights over a dark field, a cold moonlit city, warm sepia.
func TestWEFinale(t *testing.T) {
	m, f := weFrame(t, Options{}, weWitchT, 40, 6)
	if n := count(m, f, func(c col) bool { return c.b > c.r+0.02 }); n < 4000 {
		t.Errorf("Ведьма: %d холодных пикселей", n)
	}
	m, f = weFrame(t, Options{}, weHomeT, 40, 8)
	if n := count(m, f, func(c col) bool { return c.r > c.b+0.05 }); n < 6000 {
		t.Errorf("Home: %d тёплых пикселей", n)
	}
	m = New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{weMoskva}, 0, 40)
	lit := 0
	for px := 0; px < m.w; px++ {
		if weHeadlights(m, px, int(float64(2*m.h)*0.62)) > 0.3 {
			lit++
		}
	}
	if lit < 10 || lit > m.w/2 {
		t.Errorf("Moskvichka: фары освещают %d столбцов", lit)
	}
}

func TestWEReadable(t *testing.T) {
	_, f := weFrame(t, Options{}, weHaha, 30, 6)
	if !strings.Contains(stripANSI(f.String()), "Creepin'") {
		t.Errorf("очередь потеряла трек")
	}
}

func TestWENoEggs(t *testing.T) {
	_, f := weFrame(t, Options{NoEggs: true}, weGlugi, 165, 3)
	if strings.Contains(stripANSI(f.String()), "Д И К О Й") {
		t.Errorf("NoEggs печатает мантру")
	}
}

// G3 (TestWEDeterminism) lives in determinism_norace_test.go: it sits out of the
// -race run and runs in the plain, pre-commit and CI runs.

// G4: every track of the album survives a wide and a narrow window.
func TestWESizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {60, 12}} {
		for _, tr := range []Track{weOgney, weHaha, weKreem, weCold, weGlugi, weMoskva, weWitchT, weHomeT} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{tr}, 0, 165)
			steps(m, 3)
			m.render()
		}
	}
}
