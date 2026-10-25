package stage

import (
	"image"
	"strings"
	"testing"
)

// setRules swaps the egg rules for one test and puts them back afterwards.
func setRules(t *testing.T, rs ...eggRule) {
	t.Helper()
	old := eggRules
	eggRules = rs
	t.Cleanup(func() { eggRules = old })
}

// playFrame opens the player on one track, runs secs of playback and renders the frame.
func playFrame(t *testing.T, o Options, title string, e egg, secs float64) (*Model, *fb) {
	t.Helper()
	o.SelfPlay = true
	m := New(o)
	m.SetSize(110, 32)
	setRules(t, eggRule{artist: "A", e: e})
	m.JumpPlay([]Track{{Artist: "A", Title: title}}, 0, 0)
	steps(m, secs)
	return m, m.render()
}

func TestEggLayersAlbumReplacesArtist(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", e: egg{knob: 'a', jewel: true}},
		eggRule{artist: "A", album: "X", e: egg{knob: 'x'}})
	e := m.eggOf(Track{Artist: "A", Title: "T", Album: "X"})
	if e == nil {
		t.Fatal("egg nil")
	}
	if e.knob != 'x' {
		t.Errorf("knob %q, want 'x'", e.knob)
	}
	if e.jewel {
		t.Errorf("jewel должен уйти вместе со слоем артиста")
	}
}

func TestEggLayersTitleOverAlbum(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", album: "X", e: egg{knob: 'x', grain: true}},
		eggRule{artist: "A", title: "T", e: egg{switchAt: 100}})
	e := m.eggOf(Track{Artist: "A", Title: "T", Album: "X"})
	if e == nil {
		t.Fatal("egg nil")
	}
	if e.knob != 'x' || !e.grain || e.switchAt != 100 {
		t.Errorf("knob %q grain %v switchAt %v, want 'x' true 100", e.knob, e.grain, e.switchAt)
	}
}

func TestEggLayersTitleOverArtist(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", e: egg{jewel: true}},
		eggRule{artist: "A", title: "T", e: egg{switchAt: 100}})
	e := m.eggOf(Track{Artist: "A", Title: "T"})
	if e == nil {
		t.Fatal("egg nil")
	}
	if !e.jewel || e.switchAt != 100 {
		t.Errorf("jewel %v switchAt %v, want true 100", e.jewel, e.switchAt)
	}
}

func TestEggLayersHookBg(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", album: "X", e: egg{
			bg: func(m *Model, c col, x, y int) col { return col{1, 0, 0} }}},
		eggRule{artist: "A", title: "T", e: egg{
			bg: func(m *Model, c col, x, y int) col { return c.mul(0.5) }}})
	e := m.eggOf(Track{Artist: "A", Title: "T", Album: "X"})
	if c := e.bg(m, col{}, 0, 0); c != (col{0.5, 0, 0}) {
		t.Errorf("bg %v, want {0.5 0 0}", c)
	}
}

func TestEggLayersHookInkBar(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", album: "X", e: egg{
			ink: func(m *Model, c col, u, v float64) col { return col{1, 0, 0} },
			bar: func(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) { return 'a', col{1, 0, 0} }}},
		eggRule{artist: "A", title: "T", e: egg{
			ink: func(m *Model, c col, u, v float64) col { return c.mul(0.5) },
			bar: func(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) { return r + 1, c.mul(0.5) }}})
	e := m.eggOf(Track{Artist: "A", Title: "T", Album: "X"})
	if c := e.ink(m, col{}, 0, 0); c != (col{0.5, 0, 0}) {
		t.Errorf("ink %v, want {0.5 0 0}", c)
	}
	if r, c := e.bar(m, 0, 10, 0.5, '─', col{}); r != 'b' || c != (col{0.5, 0, 0}) {
		t.Errorf("bar %q %v, want 'b' {0.5 0 0}", r, c)
	}
}

func TestEggLayersHookCover(t *testing.T) {
	m := New(Options{})
	setRules(t,
		eggRule{artist: "A", album: "X", e: egg{
			cover: func(m *Model, at func(px, py int) col, px, py int) col { return col{0, 0, 1} }}},
		eggRule{artist: "A", title: "T", e: egg{
			cover: func(m *Model, at func(px, py int) col, px, py int) col { return at(px, py).mul(0.5) }}})
	e := m.eggOf(Track{Artist: "A", Title: "T", Album: "X"})
	if c := e.cover(m, func(px, py int) col { return col{} }, 0, 0); c != (col{0, 0, 0.5}) {
		t.Errorf("cover %v, want {0 0 0.5}", c)
	}
}

func TestEggLayersHookOverStep(t *testing.T) {
	m := New(Options{})
	var ov, st []string
	setRules(t,
		eggRule{artist: "A", e: egg{
			over: func(m *Model, f *fb, g playGeom) { ov = append(ov, "a") },
			step: func(m *Model) { st = append(st, "a") }}},
		eggRule{artist: "A", title: "T", e: egg{
			over: func(m *Model, f *fb, g playGeom) { ov = append(ov, "b") },
			step: func(m *Model) { st = append(st, "b") }}})
	e := m.eggOf(Track{Artist: "A", Title: "T"})
	e.over(m, newFB(4, 4), playGeom{})
	e.step(m)
	if len(ov) != 2 || ov[0] != "a" || ov[1] != "b" || len(st) != 2 || st[0] != "a" || st[1] != "b" {
		t.Errorf("порядок слоёв: over %v step %v", ov, st)
	}
}

func TestEggFrameHookBg(t *testing.T) {
	_, f := playFrame(t, Options{}, "T", egg{
		bg: func(m *Model, c col, x, y int) col { return col{0, 1, 0} }}, 2)
	if p := f.at(1, 16); p.bg.g <= p.bg.r*2 || p.bg.g <= p.bg.b*2 {
		t.Errorf("bg hook не пробился: %+v", p.bg)
	}
	_, f2 := playFrame(t, Options{NoEggs: true}, "T", egg{
		bg: func(m *Model, c col, x, y int) col { return col{0, 1, 0} }}, 2)
	if q := f2.at(1, 16); q.bg.g > q.bg.r*2 && q.bg.g > q.bg.b*2 {
		t.Errorf("NoEggs пропускает bg hook: %+v", q.bg)
	}
}

func TestEggFrameHookCover(t *testing.T) {
	m, f := playFrame(t, Options{}, "T", egg{
		cover: func(m *Model, at func(px, py int) col, px, py int) col { return col{0, 0, 1} }}, 3)
	g := m.geom()
	if c := f.pixelAt(g.cx+g.cw/2, g.cy*2+g.ch); c.b <= 0.5 || c.r >= 0.2 || c.g >= 0.2 {
		t.Errorf("cover hook не пробился: %+v (geom %+v)", c, g)
	}
}

func TestEggFrameHookInk(t *testing.T) {
	m, f := playFrame(t, Options{}, "TT", egg{
		ink: func(m *Model, c col, u, v float64) col { return col{1, 0, 0} }}, 2)
	red := false
	for py := m.titleBox.Min.Y; py < m.titleBox.Max.Y && !red; py++ {
		for px := m.titleBox.Min.X; px < m.titleBox.Max.X; px++ {
			if c := f.pixelAt(px, py); c.r > 0.9 && c.g < 0.1 && c.b < 0.1 {
				red = true
			}
		}
	}
	if !red {
		t.Errorf("ink hook не пробился в %v", m.titleBox)
	}
}

func TestEggFrameHookBar(t *testing.T) {
	m, _ := playFrame(t, Options{}, "T", egg{
		bar: func(m *Model, i, bw int, frac float64, r rune, c col) (rune, col) { return '▲', white }}, 2)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "▲") {
		t.Errorf("bar hook не пробился:\n%s", frame)
	}
}

func TestEggFrameHookOver(t *testing.T) {
	m, _ := playFrame(t, Options{}, "T", egg{
		over: func(m *Model, f *fb, g playGeom) { f.put(0, 0, '@', white) }}, 2)
	lines := strings.Split(stripANSI(m.View()), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "@") {
		t.Errorf("over hook не пробился: первая строка %q", lines[0])
	}
}

func TestEggFrameHookStep(t *testing.T) {
	n := 0
	m := New(Options{})
	m.SetSize(110, 32)
	setRules(t, eggRule{artist: "A", title: "T", e: egg{step: func(m *Model) { n++ }}})
	m.JumpPlay([]Track{{Artist: "A", Title: "T"}}, 0, 0)
	steps(m, 1)
	if n == 0 {
		t.Fatal("step hook не вызван ни разу")
	}
}

func TestTitleBoxFrame(t *testing.T) {
	m, _ := playFrame(t, Options{}, "TT", egg{}, 2)
	if m.titleBox == (image.Rectangle{}) {
		t.Fatal("titleBox пустой")
	}
	if px := m.geom().px; m.titleBox.Min.X != px {
		t.Errorf("titleBox.Min.X %d, geom px %d", m.titleBox.Min.X, px)
	}
}

func TestEggOfAlbum(t *testing.T) {
	tests := []struct {
		name   string
		noEggs bool
		t      Track
		knob   rune // '†' rules on the Whole Lotta Red album
		cold   bool // Magnolia's producer tag
		jewel  bool // Uzi's diamond
		swAt   float64
		none   bool
	}{
		{"WLR on another album track", false, Track{Artist: "Playboi Carti", Title: "Stop Breathing", Album: "Whole Lotta Red"}, '†', false, false, 0, false},
		{"WLR on the deluxe album", false, Track{Artist: "Playboi Carti", Title: "Sky", Album: "Whole Lotta Red (Deluxe)"}, '†', false, false, 0, false},
		{"WLR folded over case", false, Track{Artist: "PLAYBOI CARTI", Title: "Vamp Anthem", Album: "WHOLE LOTTA RED"}, '†', false, false, 0, false},
		{"other album stays plain", false, Track{Artist: "Playboi Carti", Title: "Shoota", Album: "Die Lit"}, 0, false, false, 0, true},
		{"Magnolia on its album", false, Track{Artist: "Playboi Carti", Title: "Magnolia", Album: "Playboi Carti"}, 0, true, false, 0, false},
		{"Nights switches", false, Track{Artist: "Frank Ocean", Title: "Nights", Album: "Blonde"}, 0, false, false, 210, false},
		{"Nights live is not Nights", false, Track{Artist: "Frank Ocean", Title: "Nights (Live)", Album: "Blonde"}, 0, false, false, 0, true},
		{"feat bracket stripped", false, Track{Artist: "Playboi Carti", Title: "Magnolia (feat. Someone)", Album: "Playboi Carti"}, 0, true, false, 0, false},
		{"Uzi in the duo track", false, Track{Artist: "Future & Lil Uzi Vert", Title: "Patek", Album: "Pluto x Baby Pluto"}, 0, false, true, 0, false},
		{"Uzi Vertigo is not Uzi", false, Track{Artist: "Lil Uzi Vertigo", Title: "X"}, 0, false, false, 0, true},
		{"NoEggs kills everything", true, Track{Artist: "Playboi Carti", Title: "Sky", Album: "Whole Lotta Red"}, 0, false, false, 0, true},
		{"Sky without an album is plain", false, Track{Artist: "Playboi Carti", Title: "Sky"}, 0, false, false, 0, true},
	}
	for _, tt := range tests {
		m := New(Options{NoEggs: tt.noEggs})
		e := m.eggOf(tt.t)
		switch {
		case tt.none:
			if e != nil {
				t.Errorf("%s: egg %+v, want nil", tt.name, e)
			}
			continue
		case e == nil:
			t.Errorf("%s: egg nil, want one", tt.name)
			continue
		}
		if tt.knob != 0 && e.knob != tt.knob {
			t.Errorf("%s: knob %q, want %q", tt.name, e.knob, tt.knob)
		}
		if tt.cold && e.cold == nil {
			t.Errorf("%s: cold open missing", tt.name)
		}
		if tt.jewel && !e.jewel {
			t.Errorf("%s: jewel missing", tt.name)
		}
		if tt.swAt != 0 && e.switchAt != tt.swAt {
			t.Errorf("%s: switchAt %v, want %v", tt.name, e.switchAt, tt.swAt)
		}
	}
}

func TestEggOfMemo(t *testing.T) {
	m := New(Options{})
	tr := Track{Artist: "Playboi Carti", Title: "Stop Breathing", Album: "Whole Lotta Red"}
	a, b := m.eggOf(tr), m.eggOf(tr)
	if a == nil {
		t.Fatal("egg nil")
	}
	if a != b {
		t.Fatalf("два вызова eggOf вернули разные указатели: %p и %p", a, b)
	}
}

func TestFoldHelpers(t *testing.T) {
	if got := fold("The Life of Pi’erre 4"); got != "the life of pi'erre 4" {
		t.Errorf("fold апострофа: %q", got)
	}
	if got := foldAlbum("Heroes & Villains"); got != "heroes & villains" {
		t.Errorf("foldAlbum Heroes: %q", got)
	}
	if got := foldAlbum("The Life of Pi’erre 4"); got != "the life of pi'erre 4" {
		t.Errorf("foldAlbum портит Pi’erre: %q", got)
	}
	if got := foldAlbum("X - Single"); got != "x" {
		t.Errorf("foldAlbum сингла: %q", got)
	}
	if got := foldTitle("Sky (feat. X)"); got != "sky" {
		t.Errorf("foldTitle фита: %q", got)
	}
	if got := foldTitle("Nights (Live)"); got != "nights (live)" {
		t.Errorf("foldTitle живой: %q", got)
	}
}

func TestEggFrameOnAlbumTrack(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.JumpPlay([]Track{{Artist: "Playboi Carti", Title: "Stop Breathing", Album: "Whole Lotta Red", Dur: 190}}, 0, 0)
	steps(m, 8)
	if frame := stripANSI(m.View()); !strings.Contains(frame, "†") {
		t.Errorf("кадр пасхалки альбома без ползунка †:\n%s", frame)
	}
}
