package vibeai

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("прочитать %s: %v", name, err)
	}
	return body
}

func TestParseArtistFacts(t *testing.T) {
	f, err := parseArtistFacts(loadFixture(t, "apple-artist.json"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.Name != "Yeat" {
		t.Errorf("Name = %q, want Yeat", f.Name)
	}
	if want := []string{"Hip-Hop/Rap"}; !reflect.DeepEqual(f.Genres, want) {
		t.Errorf("Genres = %v, want %v", f.Genres, want)
	}
	wantRelease := Release{Name: "ADL", Date: "2026-03-27", Tracks: 21}
	if len(f.Albums) == 0 || f.Albums[0] != wantRelease {
		t.Errorf("Albums[0] = %+v, want %+v", f.Albums, wantRelease)
	}
	if f.Latest == nil || f.Latest.Name != "COCOON" {
		t.Errorf("Latest = %+v, want name COCOON", f.Latest)
	}
	if len(f.TopSongs) < 2 || f.TopSongs[1] != "Drake — IDGAF (feat. Yeat)" {
		t.Errorf("TopSongs = %q, want [1] = %q", f.TopSongs, "Drake — IDGAF (feat. Yeat)")
	}
	if f.Editorial != "Rage-rap auteur." {
		t.Errorf("Editorial = %q, want %q", f.Editorial, "Rage-rap auteur.")
	}
}

func TestParseAlbumFacts(t *testing.T) {
	f, err := parseAlbumFacts(loadFixture(t, "apple-album.json"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.Artist != "Frank Ocean" {
		t.Errorf("Artist = %q, want Frank Ocean", f.Artist)
	}
	if f.Name != "Blonde" {
		t.Errorf("Name = %q, want Blonde", f.Name)
	}
	if f.Date != "2016-08-20" {
		t.Errorf("Date = %q, want 2016-08-20", f.Date)
	}
	if want := []string{"Pop", "R&B/Soul"}; !reflect.DeepEqual(f.Genres, want) {
		t.Errorf("Genres = %v, want %v", f.Genres, want)
	}
	if want := []string{"Nikes", "Ivy", "Pink + White", "Be Yourself"}; !reflect.DeepEqual(f.Tracks, want) {
		t.Errorf("Tracks = %v, want %v", f.Tracks, want)
	}
	if strings.Contains(f.Editorial, "<") {
		t.Errorf("Editorial содержит HTML: %q", f.Editorial[:80])
	}
	if !strings.HasPrefix(f.Editorial, "100 Best Albums") {
		t.Errorf("Editorial = %q, want prefix %q", f.Editorial[:80], "100 Best Albums")
	}
}

func TestParseSongFacts(t *testing.T) {
	f, err := parseSongFacts(loadFixture(t, "apple-song.json"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if f.Title != "Closed on Sunday" {
		t.Errorf("Title = %q, want Closed on Sunday", f.Title)
	}
	if f.Album != "JESUS IS KING" {
		t.Errorf("Album = %q, want JESUS IS KING", f.Album)
	}
	if f.Date != "2019-10-25" {
		t.Errorf("Date = %q, want 2019-10-25", f.Date)
	}
	if !strings.HasPrefix(f.AlbumEditorial, "Ye looks to his faith") {
		t.Errorf("AlbumEditorial = %q, want prefix %q", f.AlbumEditorial, "Ye looks to his faith")
	}
}

func TestCleanEditorial(t *testing.T) {
	if got, want := cleanEditorial("<b>A</b>  &amp;\n B"), "A & B"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	long := strings.Repeat("а", 1300)
	want := strings.Repeat("а", 1200) + "…"
	if got := cleanEditorial(long); got != want {
		t.Errorf("len = %d рун, want %d рун + …", len([]rune(got)), len([]rune(want)))
	}
}

func TestParseArtistFactsEmpty(t *testing.T) {
	_, err := parseArtistFacts([]byte(`{"data":[]}`))
	if err == nil || !strings.Contains(err.Error(), "artist not found") {
		t.Errorf("err = %v, want «артист не найден»", err)
	}
}
