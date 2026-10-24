package vibeai

import (
	"reflect"
	"testing"
)

func TestLibraryHas(t *testing.T) {
	lib := NewLibrary([]Song{{Artist: "Travis Scott", Title: "goosebumps"}})
	tests := []struct {
		name string
		song Song
		want bool
	}{
		{"feat version of known song", Song{Artist: "Travis Scott", Title: "goosebumps (feat. Kendrick Lamar)"}, true},
		{"reordered artists", Song{Artist: "Travis Scott & Kendrick Lamar", Title: "goosebumps"}, true},
		{"same title other artist", Song{Artist: "Other Artist", Title: "goosebumps"}, false},
	}
	for _, tt := range tests {
		if got := lib.Has(tt.song); got != tt.want {
			t.Errorf("%s: Has(%+v) = %v, want %v", tt.name, tt.song, got, tt.want)
		}
	}
}

func TestKnownArtists(t *testing.T) {
	got := KnownArtists([]Song{
		{Artist: "Travis Scott feat. Kendrick Lamar", Title: "t1"},
		{Artist: "Offset & Metro Boomin", Title: "t2"},
	})
	want := map[string]bool{"kendricklamar": true, "metroboomin": true, "offset": true, "travisscott": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KnownArtists = %v, want %v", got, want)
	}
}
