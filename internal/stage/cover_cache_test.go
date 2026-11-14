package stage

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// plainCover is a small solid image to stock covers with.
func plainCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	return img
}

func countKey(keys []string, key string) int {
	n := 0
	for _, k := range keys {
		if k == key {
			n++
		}
	}
	return n
}

// A7: covers past the cap leave, oldest first.
func TestCoverCapEvictsOldest(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	for i := range 301 {
		m.SetCover(fmt.Sprintf("k%d", i), plainCover())
	}
	if got := len(m.covers); got != 300 {
		t.Fatalf("covers = %d, want 300", got)
	}
	if _, ok := m.covers["k0"]; ok {
		t.Fatal("the oldest cover k0 survived the cap")
	}
	if _, ok := m.covers["k300"]; !ok {
		t.Fatal("the newest cover k300 left with the cap")
	}
}

// A8: the key of a track in the queue is never evicted.
func TestCoverCapKeepsQueue(t *testing.T) {
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay([]Track{{ID: "k0", Artist: "A", Title: "One", Dur: 60}}, 0, 0)
	m.SetCover("k0", plainCover())
	for i := 1; i <= 301; i++ {
		m.SetCover(fmt.Sprintf("k%d", i), plainCover())
	}
	if _, ok := m.covers["k0"]; !ok {
		t.Fatal("the queued track's cover was evicted")
	}
	if got := len(m.covers); got != 300 {
		t.Fatalf("covers = %d, want 300", got)
	}
}

// A9: a key drawn without a cover is asked for once, and asked for again after
// eviction brought it back to missing.
func TestCoverWantedOnDraw(t *testing.T) {
	tracks := []Track{{ID: "a", Artist: "A", Title: "One", Dur: 60},
		{ID: "b", Artist: "B", Title: "Two", Dur: 60},
		{ID: "c", Artist: "C", Title: "Three", Dur: 60}}
	m := New(Options{})
	m.SetSize(160, 45)
	m.JumpPlay(tracks, 0, 0)
	steps(m, 1)
	m.render()
	if got := countKey(m.WantedCovers(), "a"); got != 1 {
		t.Fatalf("the drawn key is asked for %d times, want 1", got)
	}
	if got := len(m.WantedCovers()); got != 0 {
		t.Fatalf("the second drain reported %d keys, want none", got)
	}
	m.SetCover("a", plainCover())
	m.JumpPlay([]Track{{ID: "z", Artist: "Z", Title: "Zed", Dur: 60}}, 0, 0)
	for i := range 301 {
		m.SetCover(fmt.Sprintf("e%d", i), plainCover())
	}
	if _, ok := m.covers["a"]; ok {
		t.Fatal("a stayed past the cap")
	}
	m.JumpPlay(tracks, 0, 0)
	steps(m, 1)
	m.render()
	if got := countKey(m.WantedCovers(), "a"); got != 1 {
		t.Fatalf("after eviction the drawn key is asked for %d times, want 1", got)
	}
}
