package stage

import (
	"fmt"
	"math"
	"testing"
)

func dist(a, b col) float64 {
	return math.Abs(a.r-b.r) + math.Abs(a.g-b.g) + math.Abs(a.b-b.b)
}

func backdropFrame(m *Model) *fb {
	f := newFB(m.w, m.h)
	m.backdrop(f)
	return f
}

func cellCenter(f *fb) col { return f.c[(f.h/2)*f.w+f.w/2].bg }

// backdropModel: play phase with 20 own tracks and no real covers, so m.cover falls back to procedural grids.
func backdropModel(t *testing.T) *Model {
	t.Helper()
	m := New(Options{})
	m.SetSize(110, 32)
	tracks := make([]Track, 20)
	for i := range tracks {
		tracks[i] = Track{Artist: fmt.Sprintf("Artist%d", i), Title: fmt.Sprintf("Title%d", i), Dur: float64(90 + i)}
	}
	for i, s := range m.slots {
		s.tr = tracks[i%len(tracks)]
		s.fillAt = 0
	}
	m.bloom = 0
	m.toPlay()
	return m
}

// R1: the play backdrop follows the current track's cover.
func TestBackdropInPlayUsesCover(t *testing.T) {
	m := backdropModel(t)
	m.trackT, m.t = 5, 3
	a := cellCenter(backdropFrame(m))
	m.startTrack(1)
	m.trackT, m.t = 5, 3
	b := cellCenter(backdropFrame(m))
	if !(dist(a, b) > 0.02) {
		t.Errorf("центр при треке 1 совпал с треком 0: dist=%v a=%v b=%v", dist(a, b), a, b)
	}
}

// R3: brightness of the play backdrop is capped.
func TestBackdropBrightnessBounded(t *testing.T) {
	m := backdropModel(t)
	m.trackT, m.t = 5, 3
	f := backdropFrame(m)
	for i, c := range f.c {
		if p := math.Max(c.bg.r, math.Max(c.bg.g, c.bg.b)); p > 0.21 {
			t.Fatalf("ячейка %d: канал %v > 0.21 (bg %v)", i, p, c.bg)
		}
	}
}

// R2: the play backdrop drifts slowly over time.
func TestBackdropDriftsOverTime(t *testing.T) {
	m := backdropModel(t)
	m.trackT, m.t = 5, 0
	a := backdropFrame(m)
	m.t = 10
	b := backdropFrame(m)
	best := 0.0
	for i := range a.c {
		best = math.Max(best, dist(a.c[i].bg, b.c[i].bg))
	}
	if !(best > 0.003) {
		t.Errorf("фон статичен во времени: лучшая дистанция по ячейкам %v", best)
	}
}

// R4: after a track change the backdrop flows from the old cover to the new over 1.4s.
func TestBackdropTrackTransition(t *testing.T) {
	m := backdropModel(t)
	m.trackT, m.t = 5, 3
	a := cellCenter(backdropFrame(m))
	m.startTrack(1)
	m.trackT, m.t = 5, 3
	end := cellCenter(backdropFrame(m))
	m.trackT = 0
	start := cellCenter(backdropFrame(m))
	if !(dist(a, start) < 1e-6) {
		t.Errorf("при trackT=0 центр ушёл от фона старого трека: dist=%v", dist(a, start))
	}
	m.trackT = 1.4
	done := cellCenter(backdropFrame(m))
	if !(dist(done, end) < 1e-6) {
		t.Errorf("при trackT=1.4 центр не дошёл до фона нового трека: dist=%v", dist(done, end))
	}
}

// R5: outside play phase the backdrop ignores the queue and the current track.
func TestBackdropOutsidePlay(t *testing.T) {
	m := New(Options{})
	m.SetSize(110, 32)
	m.setPhase(PhIntro)
	if m.queue != nil {
		t.Fatalf("queue != nil до toPlay")
	}
	base := cellCenter(backdropFrame(m))
	m.cur = 7
	g := backdropFrame(m)
	if d := dist(base, cellCenter(g)); d > 1e-12 {
		t.Errorf("вне плеера фон меняется от m.cur: dist=%v", d)
	}
}
