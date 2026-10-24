package vibeai

import (
	"testing"
)

func TestMatchAlbum(t *testing.T) {
	tests := []struct {
		name    string
		artist  string
		album   string
		results []appleAlbum
		wantID  string
		wantOK  bool
	}{
		{"expanded edition variant", "Travis Scott", "Rodeo", []appleAlbum{{ID: "10", Name: "Rodeo (Expanded Edition)", Artist: "Travis Scott"}}, "10", true},
		{"skip wrong artist", "Travis Scott", "Rodeo", []appleAlbum{{ID: "11", Name: "Rodeo", Artist: "Other Artist"}, {ID: "12", Name: "Rodeo", Artist: "Travis Scott"}}, "12", true},
		{"single suffix", "Lil Yachty", "Poland", []appleAlbum{{ID: "13", Name: "Poland - Single", Artist: "Lil Yachty"}}, "13", true},
		{"dollar artist not matched", "Travi$ Scott", "Rodeo", []appleAlbum{{ID: "14", Name: "Rodeo", Artist: "Travis Scott"}}, "", false},
		{"deluxe edition", "Don Toliver", "Life of a DON", []appleAlbum{{ID: "15", Name: "Life of a DON (Deluxe)", Artist: "Don Toliver"}}, "15", true},
		{"exact beats earlier prefix", "BROCKHAMPTON", "SATURATION", []appleAlbum{{ID: "16", Name: "SATURATION II", Artist: "BROCKHAMPTON"}, {ID: "17", Name: "SATURATION", Artist: "BROCKHAMPTON"}}, "17", true},
	}
	for _, tt := range tests {
		got, ok := matchAlbum(tt.artist, tt.album, tt.results)
		if ok != tt.wantOK || got.ID != tt.wantID {
			t.Errorf("%s: got (%+v, %v), want (ID %q, %v)", tt.name, got, ok, tt.wantID, tt.wantOK)
		}
	}
}
