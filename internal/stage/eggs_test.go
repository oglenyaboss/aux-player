package stage

import (
	"strings"
	"testing"
)

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
