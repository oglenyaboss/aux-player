package stage

import (
	"strings"
	"testing"
)

var (
	curLetIt   = Track{Artist: "Tame Impala", Title: "Let It Happen", Album: "Currents", Dur: 467}
	igorQuake  = Track{Artist: "Tyler, The Creator", Title: "EARFQUAKE", Album: "IGOR", Dur: 190}
	igorFriend = Track{Artist: "Tyler, The Creator", Title: "ARE WE STILL FRIENDS?", Album: "IGOR", Dur: 265}
)

func wave3Frame(t *testing.T, o Options, tr Track, at, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.JumpPlay([]Track{tr, hvCreepin}, 0, at)
	steps(m, secs)
	return m, m.render()
}

// count counts pixels of the screen outside the cover that pass ok.
func count(m *Model, f *fb, ok func(c col) bool) int {
	g := m.geom()
	n := 0
	for py := 0; py < 2*m.h; py++ {
		for px := 0; px < m.w; px++ {
			if px >= g.cx && px < g.cx+g.cw && py >= g.cy*2 && py < g.cy*2+g.ch*2 {
				continue
			}
			if ok(f.pixelAt(px, py)) {
				n++
			}
		}
	}
	return n
}

func TestWave3Rules(t *testing.T) {
	m := New(Options{})
	for _, c := range []struct {
		tr Track
		th *theme
	}{{curLetIt, &currentsTheme}, {igorQuake, &igorTheme}} {
		if e := m.eggOf(c.tr); e == nil || e.theme != c.th {
			t.Errorf("%s: egg %+v", c.tr.Title, e)
		}
	}
	for _, tr := range []Track{
		{Artist: "Tame Impala", Title: "Borderline", Album: "The Slow Rush"},
		{Artist: "Tyler, The Creator", Title: "See You Again", Album: "Flower Boy"},
	} {
		if e := m.eggOf(tr); e != nil {
			t.Errorf("%s поймал пасхалку", tr.Album)
		}
	}
}

func currentsLilac(c col) bool { return c.b > 0.25 && c.b > c.g+0.12 && c.r > c.g+0.08 }

// The stream fills the backdrop once it has come in, not before.
func TestCurrentsStream(t *testing.T) {
	m, f := wave3Frame(t, Options{}, curLetIt, 60, 0.3)
	if n := count(m, f, currentsLilac); n > 200 {
		t.Errorf("на 0.3 с уже %d пикселей потока", n)
	}
	m, f = wave3Frame(t, Options{}, curLetIt, 60, 8)
	if n := count(m, f, currentsLilac); n < 3000 {
		t.Errorf("%d пикселей потока, ждём линии по всему фону", n)
	}
	if n := count(m, f, func(c col) bool { return c.r > 0.85 && c.g < 0.45 && c.b < 0.3 }); n < 30 {
		t.Errorf("%d пикселей красной линии", n)
	}
}

// The lines move with time.
func TestCurrentsFlows(t *testing.T) {
	m, f := wave3Frame(t, Options{}, curLetIt, 60, 8)
	a := f.String()
	steps(m, 1)
	if m.render().String() == a {
		t.Errorf("поток стоит")
	}
}

func TestCurrentsReadable(t *testing.T) {
	_, f := wave3Frame(t, Options{}, curLetIt, 60, 8)
	if s := stripANSI(f.String()); !strings.Contains(s, "Creepin'") {
		t.Errorf("очередь потеряла трек")
	}
}

func igorPinkish(c col) bool { return c.r > 0.45 && c.r > c.g+0.1 && c.b > c.g+0.03 }

// The pink is bright at the start of the album and nearly gone at its end.
func TestIgorDrain(t *testing.T) {
	m, f := wave3Frame(t, Options{}, igorQuake, 20, 3)
	if n := count(m, f, igorPinkish); n < 6000 {
		t.Errorf("EARFQUAKE: %d розовых пикселей", n)
	}
	m, f = wave3Frame(t, Options{}, igorFriend, 20, 3)
	if n := count(m, f, igorPinkish); n > 500 {
		t.Errorf("ARE WE STILL FRIENDS?: %d розовых пикселей, ждём выцветший", n)
	}
}

// The credit types itself under the cover.
func TestIgorCredit(t *testing.T) {
	_, f := wave3Frame(t, Options{}, igorQuake, 20, 1.6)
	if strings.Contains(stripANSI(f.String()), "ALL SONGS") {
		t.Errorf("строка появилась целиком сразу")
	}
	_, f = wave3Frame(t, Options{}, igorQuake, 20, 8)
	s := stripANSI(f.String())
	for _, w := range igorCredit {
		if !strings.Contains(s, w) {
			t.Errorf("нет %q", w)
		}
	}
}

func TestWave3NoEggs(t *testing.T) {
	m, f := wave3Frame(t, Options{NoEggs: true}, curLetIt, 60, 8)
	if n := count(m, f, currentsLilac); n > 1500 {
		t.Errorf("Currents NoEggs: %d", n)
	}
	_, f = wave3Frame(t, Options{NoEggs: true}, igorQuake, 20, 8)
	if strings.Contains(stripANSI(f.String()), "TYLER OKONMA") {
		t.Errorf("IGOR NoEggs печатает строку")
	}
}

func TestWave3Determinism(t *testing.T) {
	for _, tr := range []Track{curLetIt, igorQuake} {
		_, a := wave3Frame(t, Options{}, tr, 30, 5)
		_, b := wave3Frame(t, Options{}, tr, 30, 5)
		if a.String() != b.String() {
			t.Errorf("%s: кадры разъехались", tr.Title)
		}
	}
}

func TestWave3Sizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}, {300, 80}, {20, 6}} {
		for _, tr := range []Track{curLetIt, igorQuake} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{tr}, 0, 30)
			steps(m, 5)
			m.render()
		}
	}
}
