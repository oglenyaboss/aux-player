package vibeai

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSimilar(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		want  []similar
		isErr bool
	}{
		{
			name: "array of artists",
			body: `{"similarartists":{"artist":[{"name":"Pond","match":"1"},{"name":"GUM","match":"0.65"}]}}`,
			want: []similar{{Name: "Pond", Match: 1}, {Name: "GUM", Match: 0.65}},
		},
		{
			name: "single artist object",
			body: `{"similarartists":{"artist":{"name":"Solo","match":"0.5"}}}`,
			want: []similar{{Name: "Solo", Match: 0.5}},
		},
		{
			name:  "lastfm error",
			body:  `{"error":6,"message":"The artist you supplied could not be found"}`,
			isErr: true,
		},
	}
	for _, tt := range tests {
		got, err := parseSimilar([]byte(tt.body))
		if tt.isErr {
			if err == nil || !strings.Contains(err.Error(), "6") || !strings.Contains(err.Error(), "could not be found") {
				t.Errorf("%s: err = %v, want text with 6 and %q", tt.name, err, "could not be found")
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected err %v", tt.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseArtistSearch(t *testing.T) {
	id, name, err := parseArtistSearch([]byte(`{"results":{"artists":{"data":[{"id":"123","attributes":{"name":"Pond"}}]}}}`))
	if err != nil || id != "123" || name != "Pond" {
		t.Errorf("got (%q, %q, %v), want (\"123\", \"Pond\", nil)", id, name, err)
	}
	id, name, err = parseArtistSearch([]byte(`{"results":{}}`))
	if err != nil || id != "" || name != "" {
		t.Errorf("got (%q, %q, %v), want empty nil", id, name, err)
	}
}

func TestParseLibraryPage(t *testing.T) {
	songs, libIDs, next, err := parseLibraryPage([]byte(`{"data":[{"id":"i.abc","attributes":{"artistName":"Artist A","name":"Title A","albumName":"Album A","playParams":{"catalogId":"123"}}},{"id":"i.def","attributes":{"artistName":"Artist B","name":"Title B"}}],"next":"/v1/me/library/songs?offset=100"}`))
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	want := []Song{
		{ID: "123", Artist: "Artist A", Title: "Title A", Album: "Album A"},
		{ID: "i.def", Artist: "Artist B", Title: "Title B"},
	}
	if !reflect.DeepEqual(songs, want) {
		t.Errorf("songs = %+v, want %+v", songs, want)
	}
	wantIDs := []string{"i.abc", "i.def"}
	if !reflect.DeepEqual(libIDs, wantIDs) {
		t.Errorf("libIDs = %v, want %v", libIDs, wantIDs)
	}
	if next != "/v1/me/library/songs?offset=100" {
		t.Errorf("next = %q, want %q", next, "/v1/me/library/songs?offset=100")
	}

	_, _, next, err = parseLibraryPage([]byte(`{"data":[]}`))
	if err != nil || next != "" {
		t.Errorf("empty page: (%v, %q), want nil err and empty next", err, next)
	}
}

func TestParseRatings(t *testing.T) {
	tests := []struct {
		name string
		body string
		want map[string]bool
	}{
		{"prefixed id, negative dropped", `{"data":[{"id":"r.1","attributes":{"value":1}},{"id":"2","attributes":{"value":-1}}]}`, map[string]bool{"1": true}},
		{"library ids", `{"data":[{"id":"i.abc","attributes":{"value":1}}]}`, map[string]bool{"i.abc": true}},
	}
	for _, tt := range tests {
		got, err := parseRatings([]byte(tt.body))
		if err != nil {
			t.Errorf("%s: unexpected err %v", tt.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
