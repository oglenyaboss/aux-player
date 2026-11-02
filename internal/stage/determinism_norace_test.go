//go:build !race

// G3 for every egg: the same track, position and clock must render the same
// frame twice. These are full-frame renders that only check pixel-level
// determinism, so they sit out of the `-race` run (the race detector multiplies
// a full-frame render several-fold) and run everywhere else: plain `go test`,
// pre-commit and CI — ci.yml has no -race job. The race run still covers every
// egg scene through the bespoke tests and the G4 size walks in the egg files.
package stage

import "testing"

func TestAllaDeterminism(t *testing.T) {
	_, a := allaFrame(t, Options{}, allaEvery, 5)
	_, b := allaFrame(t, Options{}, allaEvery, 5)
	if a.String() != b.String() {
		t.Errorf("кадры разъехались")
	}
}

func TestAtakeDeterminism(t *testing.T) {
	m1, f1 := atakeTrackFrame(t, Options{}, atakePluto, 0, 95)
	m2, f2 := atakeTrackFrame(t, Options{}, atakePluto, 0, 95)
	if f1.String() != f2.String() {
		t.Error("кадры на pluto не совпали")
	}
	_ = m1
	_ = m2
}

func TestBlondeDeterminism(t *testing.T) {
	_, a := blondeFrame(t, Options{}, blondeNights, 205, 7)
	_, b := blondeFrame(t, Options{}, blondeNights, 205, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestDsotmDeterminism(t *testing.T) {
	a, _ := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	b, _ := eggTrackFrame(t, Options{}, dsotmMoney, 100, 10)
	if a.View() != b.View() {
		t.Errorf("два кадра одного трека разошлись")
	}
}

func TestDragonSteady(t *testing.T) {
	m1, _ := eggTrackFrame(t, Options{}, dragonborn, 30, 8)
	m2, _ := eggTrackFrame(t, Options{}, dragonborn, 30, 8)
	if stripANSI(m1.View()) != stripANSI(m2.View()) {
		t.Error("the frames diverged")
	}
}

func TestHVDeterminism(t *testing.T) {
	_, a := hvFrame(t, Options{}, hvSuperhero, 7)
	_, b := hvFrame(t, Options{}, hvSuperhero, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestMbdtfDeterminism(t *testing.T) {
	m1, _ := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	m2, _ := eggTrackFrame(t, Options{}, mbdtfFantasy, 20, 9)
	if m1.View() != m2.View() {
		t.Error("два одинаковых прогона дают разные кадры")
	}
}

func TestPulsarDeterminism(t *testing.T) {
	disorder := Track{Artist: "Joy Division", Title: "Disorder", Album: "Unknown Pleasures (2019 Digital Master)", Dur: 209}
	run := func() string {
		m := New(Options{SelfPlay: true})
		m.SetSize(160, 45)
		m.JumpPlay([]Track{disorder}, 0, 50)
		steps(m, 10)
		return m.View()
	}
	first, second := run(), run()
	if first != second {
		t.Error("два прогона дают разные кадры")
	}
}

func TestUtopiaDeterminism(t *testing.T) {
	_, a := utopiaFrame(t, Options{}, 7)
	_, b := utopiaFrame(t, Options{}, 7)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}

func TestWEDeterminism(t *testing.T) {
	for _, tr := range []Track{weHaha, weOgney, weCold} {
		_, a := weFrame(t, Options{}, tr, 30, 5)
		_, b := weFrame(t, Options{}, tr, 30, 5)
		if a.String() != b.String() {
			t.Errorf("%s: кадры разъехались", tr.Title)
		}
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

func TestYeezusDeterminism(t *testing.T) {
	_, a := yeezusFrame(t, Options{}, 25, 9)
	_, b := yeezusFrame(t, Options{}, 25, 9)
	if a.String() != b.String() {
		t.Errorf("два одинаковых кадра разъехались")
	}
}
