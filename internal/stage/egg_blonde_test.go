package stage

import "testing"

var blondeNikes = Track{Artist: "Frank Ocean", Title: "Nikes", Album: "Blonde", Dur: 314}

func TestBlondeRule(t *testing.T) {
	if e := New(Options{}).eggOf(blondeNikes); e == nil || e.theme != &blondeTheme || e.bg == nil || e.knob != '●' {
		t.Fatalf("eggOf(Nikes) = %+v, ждали тему, bg и ручку ●", e)
	}
	if o := New(Options{}).eggOf(Track{Artist: "Frank Ocean", Title: "Nikes", Album: "channel ORANGE", Dur: 314}); o != nil {
		t.Errorf("channel ORANGE ловит душ: %+v", o)
	}
	if New(Options{}).eggOf(Track{Artist: "Frank Sinatra", Title: "Nikes", Album: "Blonde", Dur: 314}) != nil {
		t.Errorf("Frank Sinatra — не тот Франк")
	}
}

func TestBlondeNightsOnTop(t *testing.T) {
	m := New(Options{})
	e := m.eggOf(Track{Artist: "Frank Ocean", Title: "Nights", Album: "Blonde", Dur: 310})
	if e == nil || e.switchAt != 210 || e.bg == nil || e.theme != &blondeTheme {
		t.Errorf("Nights на Blonde: %+v, ждали switchAt 210 поверх душа", e)
	}
}

func TestBlondeTile(t *testing.T) {
	_, f := eggTrackFrame(t, Options{}, blondeNikes, 0, 10)
	if lt, lu := f.at(6, 4).bg.lum(), f.at(8, 4).bg.lum(); lt >= 0.8*lu {
		t.Errorf("шов не темнее плитки: %v >= 0.8*%v", lt, lu)
	}
	if l0, l2 := f.at(0, 4).bg.lum(), f.at(2, 4).bg.lum(); l0 >= 0.8*l2 {
		t.Errorf("лесенка не играет: %v >= 0.8*%v", l0, l2)
	}
}

func TestBlondeBeforeReveal(t *testing.T) {
	_, f := eggTrackFrame(t, Options{}, blondeNikes, 0, 3)
	if lt, lu := f.at(6, 4).bg.lum(), f.at(8, 4).bg.lum(); lt <= 0.8*lu {
		t.Errorf("кафель показался до раскрытия: %v <= 0.8*%v", lt, lu)
	}
}

func TestBlondeSteamRises(t *testing.T) {
	_, late := eggTrackFrame(t, Options{}, blondeNikes, 270, 10)
	_, early := eggTrackFrame(t, Options{}, blondeNikes, 0, 10)
	mean := func(f *fb) float64 {
		s := 0.0
		for x := 0; x < 12; x++ {
			s += f.at(x, 22).bg.lum()
		}
		return s / 12
	}
	if d := mean(late) - mean(early); d <= 0.05 {
		t.Errorf("пар не растёт: прирост %.3f", d)
	}
}

func TestBlondeDrop(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, blondeNikes, 17, 10)
	if !blondeBrightRows(f) {
		t.Errorf("капля не светится на %.1f-й секунде", m.pos)
	}
	m, f = eggTrackFrame(t, Options{}, blondeNikes, 2, 10)
	if blondeBrightRows(f) {
		t.Errorf("капля светится на %.1f-й секунде без капель", m.pos)
	}
}

func blondeBrightRows(f *fb) bool {
	for y := 35; y <= 44; y++ {
		for x := 0; x < f.w; x++ {
			if f.at(x, y).bg.lum() > 0.35 {
				return true
			}
		}
	}
	return false
}

func blondeB(m *Model, f *fb) float64 {
	g := m.geom()
	gl := font3['B']
	sumDot, nDot, sumHash, nHash := 0.0, 0, 0.0, 0
	for dy := 0; dy < 5; dy++ {
		for dx := 0; dx < len(gl[0]); dx++ {
			v := f.at(g.cx+dx, g.cy+g.ch+1+dy).bg.lum()
			switch gl[dy][dx] {
			case '#':
				sumHash, nHash = sumHash+v, nHash+1
			case '.':
				sumDot, nDot = sumDot+v, nDot+1
			}
		}
	}
	return (sumHash / float64(nHash)) / (sumDot / float64(nDot))
}

func TestBlondeWord(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, blondeNikes, 136, 10)
	if r := blondeB(m, f); r >= 0.7 {
		t.Errorf("B не проступил: %.2f >= 0.7", r)
	}
	m, f = eggTrackFrame(t, Options{}, blondeNikes, 96, 10)
	if r := blondeB(m, f); r <= 0.85 {
		t.Errorf("без надписи темнит: %.2f <= 0.85", r)
	}
}

func TestBlondeWordFogsBack(t *testing.T) {
	m, f := eggTrackFrame(t, Options{}, blondeNikes, 196, 10)
	if r := blondeB(m, f); r <= 0.85 {
		t.Errorf("через 60 с слово не затянулось: %.2f <= 0.85", r)
	}
}

func TestBlondeNoEggs(t *testing.T) {
	_, f := eggTrackFrame(t, Options{NoEggs: true}, blondeNikes, 0, 10)
	if lt, lu := f.at(6, 4).bg.lum(), f.at(8, 4).bg.lum(); lt <= 0.8*lu {
		t.Errorf("NoEggs рисует кафель: %v <= 0.8*%v", lt, lu)
	}
}

func TestBlondeDeterminism(t *testing.T) {
	m, _ := eggTrackFrame(t, Options{}, blondeNikes, 20, 4)
	n, _ := eggTrackFrame(t, Options{}, blondeNikes, 20, 4)
	if m.View() != n.View() {
		t.Errorf("кадры одной сцены разошлись")
	}
}

func TestBlondeSurvives(t *testing.T) {
	for _, wh := range [][2]int{{160, 45}, {110, 32}, {80, 40}, {60, 12}} {
		for _, pos := range [4]float64{0, 7, 314 * 0.5, 314 - 15} {
			for _, secs := range [2]float64{1, 9} {
				m := New(Options{SelfPlay: true})
				m.SetSize(wh[0], wh[1])
				m.JumpPlay([]Track{blondeNikes}, 0, pos)
				steps(m, secs)
				m.render()
			}
		}
	}
}

func TestBlondeNightsLiveNoSwitch(t *testing.T) {
	e := New(Options{}).eggOf(Track{Artist: "Frank Ocean", Title: "Nights (Live)", Album: "Blonde"})
	if e == nil || e.switchAt != 0 {
		t.Errorf("Nights (Live): egg %+v, want the shower without a switch", e)
	}
}
