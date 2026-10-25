package stage

import "testing"

var utopiaFein = Track{Artist: "Travis Scott", Title: "FE!N (feat. Playboi Carti)", Album: "UTOPIA", Dur: 191}

func utopiaFrame(t *testing.T, o Options, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(160, 45)
	m.JumpPlay([]Track{utopiaFein}, 0, 40)
	steps(m, secs)
	return m, m.render()
}

// utopiaLit counts cells whose backdrop is lit and cells left in the dark.
func utopiaLit(m *Model, f *fb) (lit, dark int) {
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			switch l := f.at(x, y).bg.lum(); {
			case l > 0.12:
				lit++
			case l < 0.03:
				dark++
			}
		}
	}
	return lit, dark
}

func TestUtopiaRule(t *testing.T) {
	m := New(Options{})
	if e := m.eggOf(utopiaFein); e == nil || e.theme != &utopiaTheme || e.over == nil {
		t.Fatalf("egg %+v, ждём UTOPIA", e)
	}
	if e := m.eggOf(Track{Artist: "Travis Scott", Title: "SICKO MODE", Album: "ASTROWORLD"}); e != nil {
		t.Errorf("ASTROWORLD поймал пасхалку: %+v", e)
	}
}

// The track opens in the void; the light switches on a moment later.
func TestUtopiaDarkStart(t *testing.T) {
	m, f := utopiaFrame(t, Options{}, 0.5)
	if lit, _ := utopiaLit(m, f); lit > 0 {
		t.Errorf("на 0.5 с освещено %d ячеек, ждём темноту", lit)
	}
}

// One hard beam: a lit band, the rest of the player in the dark.
func TestUtopiaBeam(t *testing.T) {
	m, f := utopiaFrame(t, Options{}, 9)
	lit, dark := utopiaLit(m, f)
	if lit < 250 || dark < 2000 {
		t.Errorf("освещено %d, в темноте %d ячеек; ждём луч и пустоту вокруг", lit, dark)
	}
}

// The beam swings with the playback and holds still on pause.
func TestUtopiaSweep(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{utopiaFein}, 0, 40)
	m.Now(utopiaFein.Key(), 40, true)
	steps(m, 3)
	at := func() (s float64) {
		for x := 0; x < m.w; x++ {
			s += utopiaLight(m, x, 60) * float64(x)
		}
		return s
	}
	a := at()
	steps(m, 6)
	b := at()
	if a == b {
		t.Fatalf("луч не движется во время игры, pos %.1f", m.pos)
	}
	m.Now(utopiaFein.Key(), m.pos, false)
	steps(m, 0.2)
	c := at()
	steps(m, 3)
	if at() != c {
		t.Errorf("на паузе луч ходит")
	}
}

// Letters out of the light keep a trace, the title can still be found in the dark.
func TestUtopiaLettersTrace(t *testing.T) {
	m, f := utopiaFrame(t, Options{}, 9)
	g := m.geom()
	seen := false
	for x := g.px; x < g.px+g.pw; x++ {
		c := f.at(x, g.py)
		if c.ch != ' ' && !c.pix && utopiaLight(m, x, 2*g.py+1) == 0 {
			seen = true
			if c.fg.lum() < 0.15 {
				t.Errorf("буква в темноте почти не видна: %s", c.fg.css())
			}
		}
	}
	if !seen {
		t.Skip("заголовок целиком в луче")
	}
}

func TestUtopiaNoEggs(t *testing.T) {
	m, f := utopiaFrame(t, Options{NoEggs: true}, 9)
	if _, dark := utopiaLit(m, f); dark > 2000 {
		t.Errorf("NoEggs: %d ячеек в темноте", dark)
	}
}

func TestUtopiaDeterminism(t *testing.T) {
	_, a := utopiaFrame(t, Options{}, 7)
	_, b := utopiaFrame(t, Options{}, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestUtopiaSizes(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}, {300, 80}} {
		for _, secs := range []float64{0.5, 1, 9} {
			m := New(Options{SelfPlay: true})
			m.SetSize(sz[0], sz[1])
			m.JumpPlay([]Track{utopiaFein}, 0, 30)
			steps(m, secs)
			m.render()
		}
	}
}
