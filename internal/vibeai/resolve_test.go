package vibeai

import (
	"reflect"
	"testing"
)

func TestMatchSong(t *testing.T) {
	tests := []struct {
		name    string
		sug     Suggestion
		results []Song
		wantID  string
		wantOK  bool
	}{
		{"clean beats live", Suggestion{Artist: "Rhye", Title: "The Fall"}, []Song{
			{ID: "1", Artist: "Rhye", Title: "The Fall (Live)"},
			{ID: "2", Artist: "Rhye", Title: "The Fall"},
		}, "2", true},
		{"all versioned", Suggestion{Artist: "Rhye", Title: "The Fall"}, []Song{
			{ID: "1", Artist: "Rhye", Title: "The Fall (Live)"},
		}, "1", true},
		{"main artist", Suggestion{Artist: "Chance the Rapper", Title: "How Great"}, []Song{
			{ID: "3", Artist: "Chance the Rapper", Title: "How Great (feat. Jay Electronica & My cousin Nicole)"},
		}, "3", true},
		{"feat artist", Suggestion{Artist: "Jay Electronica", Title: "How Great"}, []Song{
			{ID: "3", Artist: "Chance the Rapper", Title: "How Great (feat. Jay Electronica & My cousin Nicole)"},
		}, "3", true},
		{"wrong artist", Suggestion{Artist: "Travis Scott", Title: "Sky"}, []Song{
			{ID: "4", Artist: "Playboi Carti", Title: "Sky"},
		}, "", false},
		{"exact beats remix", Suggestion{Artist: "Kanye West", Title: "Ultralight Beam"}, []Song{
			{ID: "5", Artist: "Kanye West", Title: "Ultralight Beam (Remix)"},
			{ID: "6", Artist: "Kanye West", Title: "Ultralight Beam"},
		}, "6", true},
	}
	for _, tt := range tests {
		got, ok := matchSong(tt.sug, tt.results)
		if ok != tt.wantOK || got.ID != tt.wantID {
			t.Errorf("%s: got (%+v, %v), want ID %q ok %v", tt.name, got, ok, tt.wantID, tt.wantOK)
		}
	}
}

func TestParseSongSearch(t *testing.T) {
	body := `{"results":{"songs":{"data":[` +
		`{"id":"s1","attributes":{"artistName":"Kanye West","name":"Ultralight Beam","albumName":"The Life of Pablo","releaseDate":"2016-02-14","artwork":{"url":"https://is1-ssl.mzstatic.com/image/{w}x{h}bb.jpg"},"durationInMillis":363245}},` +
		`{"id":"s2","attributes":{"artistName":"Kanye West","name":"Father Stretch My Hands","albumName":"The Life of Pablo"}}]}}}`
	got, err := parseSongSearch([]byte(body))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	want := []Song{
		{ID: "s1", Artist: "Kanye West", Title: "Ultralight Beam", Album: "The Life of Pablo", Year: "2016",
			ArtworkURL: "https://is1-ssl.mzstatic.com/image/{w}x{h}bb.jpg", DurationMs: 363245},
		{ID: "s2", Artist: "Kanye West", Title: "Father Stretch My Hands", Album: "The Life of Pablo", Year: "?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSongArtists(t *testing.T) {
	got := songArtists(Song{Artist: "Metro Boomin & John Legend", Title: "On Time (feat. A, B & C) [with D]"})
	want := []string{"a", "b", "c", "d", "johnlegend", "metroboomin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got = songArtists(Song{Artist: "Yeat", Title: "Track (prod. X)"})
	want = []string{"yeat"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("prod: got %v, want %v", got, want)
	}
}
