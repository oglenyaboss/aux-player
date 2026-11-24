package vibeai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func sigTime(day int) time.Time { return time.Date(2026, 11, day, 12, 0, 0, 0, time.UTC) }

func TestSearchTrackMarks(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: sigTime(1), Kind: "more", Session: "s0", Artist: "R", Title: "Rated"},
		signalRec{Time: sigTime(2), Kind: "less", Session: "s0", Artist: "D", Title: "Disliked"},
		signalRec{Time: sigTime(18), Kind: "played", Session: "s1", Artist: "P", Title: "Played"},
		signalRec{Time: sigTime(19), Kind: "played", Session: "s2", Artist: "P", Title: "Played", Source: "manual"},
		signalRec{Time: sigTime(20), Kind: "played", Session: "s1", Artist: "P", Title: "Played"},
		signalRec{Time: sigTime(21), Kind: "skip_mid", Session: "s1", Artist: "P", Title: "Played", PosSec: 60},
	)
	s := NewSession(
		[]Song{{ID: "lib-1", Artist: "L", Title: "Lib Song"}},
		[]Song{{ID: "fav-1", Artist: "F", Title: "Fav Song"}},
		3,
	)
	s.SetSignals(sig, "s9")
	for _, c := range []struct {
		song Song
		want string
	}{
		{Song{ID: "cat-1", Artist: "L", Title: "Lib Song"}, "library"},
		{Song{ID: "cat-2", Artist: "F", Title: "Fav Song"}, "favorite"},
		{Song{ID: "cat-3", Artist: "R", Title: "Rated"}, "▲"},
		{Song{ID: "cat-4", Artist: "D", Title: "Disliked"}, "▼"},
		{Song{ID: "cat-5", Artist: "P", Title: "Played"}, "3× (last 2026-11-20)"},
		{Song{ID: "cat-6", Artist: "P", Title: "Played"}, "last: played, played, skip_mid"},
	} {
		if m := s.marks(c.song); !strings.Contains(m, c.want) {
			t.Errorf("marks(%s — %s) = %q, want to contain %q", c.song.Artist, c.song.Title, m, c.want)
		}
	}
	if m := s.marks(Song{ID: "cat-7", Artist: "Z", Title: "Brand New"}); m != "new" {
		t.Errorf("marks(unknown) = %q, want exactly %q", m, "new")
	}
}

func TestSearchTrackInStream(t *testing.T) {
	s := NewSession(nil, nil, 3)
	s.Give([]Song{{ID: "g-1", Artist: "G", Title: "Given"}})
	if m := s.marks(Song{ID: "cat-1", Artist: "G", Title: "Given"}); m != "in this stream" {
		t.Errorf("marks = %q, want %q", m, "in this stream")
	}
}

func TestSubmitByIDSkipsLookup(t *testing.T) {
	be := &fakeBackend{searches: map[string][]Song{
		"A|": {{ID: "x1", Artist: "A", Title: "Title", Album: "Album", Year: "2024"}},
	}}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	ctx := context.Background()
	if res := a.dispatch(ctx, call("s1", "search_tracks", `{"queries":[{"artist":"A"}]}`)); strings.HasPrefix(res, "error") {
		t.Fatalf("search_tracks: %s", res)
	}
	if res := a.dispatch(ctx, call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %s", res)
	}
	res := a.dispatch(ctx, call("s2", "submit_candidates", `{"tracks":[{"id":"x1","why":"w"}],"note":"n"}`))
	var out struct {
		Accepted []string `json:"accepted"`
		Rejected []struct {
			Track  string `json:"track"`
			Reason string `json:"reason"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, res)
	}
	if len(out.Accepted) != 1 || out.Accepted[0] != "A — Title" {
		t.Errorf("accepted: %v, want [A — Title]", out.Accepted)
	}
	be.mu.Lock()
	calls := len(be.findCalls)
	be.mu.Unlock()
	if calls != 0 {
		t.Errorf("findSong called %d times, want 0", calls)
	}
	res2 := a.dispatch(ctx, call("s3", "submit_candidates", `{"tracks":[{"id":"nope","why":"w"}],"note":"n"}`))
	var out2 struct {
		Rejected []struct {
			Track  string `json:"track"`
			Reason string `json:"reason"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal([]byte(res2), &out2); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, res2)
	}
	if len(out2.Rejected) != 1 || out2.Rejected[0].Reason != "unknown id: take ids from search_tracks" {
		t.Errorf("rejected: %+v, want the unknown id reason", out2.Rejected)
	}
}

func TestSubmitNotFoundShowsNearest(t *testing.T) {
	ctx := context.Background()
	be := &fakeBackend{searches: map[string][]Song{
		"Ghost|Missing": {
			{ID: "n1", Artist: "Ghost", Title: "Near1", Album: "Al1", Year: "2020"},
			{ID: "o1", Artist: "Other", Title: "X", Album: "Al", Year: "2021"},
			{ID: "n2", Artist: "Ghost", Title: "Near2", Album: "Al2", Year: "2022"},
		},
	}}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	if res := a.dispatch(ctx, call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %s", res)
	}
	res := a.dispatch(ctx, call("s", "submit_candidates", `{"tracks":[{"artist":"Ghost","title":"Missing","why":"w"}],"note":"n"}`))
	var out struct {
		Rejected []struct {
			Track  string `json:"track"`
			Reason string `json:"reason"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, res)
	}
	if len(out.Rejected) != 1 {
		t.Fatalf("rejected: %+v, want 1 row", out.Rejected)
	}
	r := out.Rejected[0].Reason
	if !strings.HasPrefix(r, "not in Apple Music; nearest by this artist: ") {
		t.Errorf("reason = %q, want the nearest-by-this-artist prefix", r)
	}
	if !strings.Contains(r, "Ghost — Near1") || !strings.Contains(r, "Ghost — Near2") {
		t.Errorf("reason = %q, want Ghost — Near1 and Ghost — Near2", r)
	}
	if strings.Contains(r, "Other — X") {
		t.Errorf("reason = %q, want no hits by other artists", r)
	}

	b2 := &fakeBackend{}
	a2 := newTestAgent(t, &fakeChatter{}, b2, &fakeUI{})
	if res := a2.dispatch(ctx, call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %s", res)
	}
	res2 := a2.dispatch(ctx, call("s", "submit_candidates", `{"tracks":[{"artist":"Ghost","title":"Missing","why":"w"}],"note":"n"}`))
	var out2 struct {
		Rejected []struct {
			Track  string `json:"track"`
			Reason string `json:"reason"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal([]byte(res2), &out2); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, res2)
	}
	if len(out2.Rejected) != 1 || out2.Rejected[0].Reason != "not in Apple Music; nothing by this artist found" {
		t.Errorf("rejected: %+v, want the nothing-found reason", out2.Rejected)
	}
}

const topSongsBody = `{"data":[{"id":"9070","type":"artists","attributes":{"name":"Yeat"},
"views":{"top-songs":{"data":[
  {"id":"t1","type":"songs","attributes":{"artistName":"Yeat","name":"Bigger Thën Everything","albumName":"bigger thën everything","releaseDate":"2023-06-02","artwork":{"url":"https://a/1.jpg"},"durationInMillis":200000}},
  {"id":"t2","type":"songs","attributes":{"artistName":"Drake","name":"IDGAF (feat. Yeat)","albumName":"For All The Dogs","releaseDate":"2023-10-06","artwork":{"url":"https://a/2.jpg"},"durationInMillis":180000}}
]}}}]}`

func TestParseTopSongs(t *testing.T) {
	songs, err := parseTopSongs([]byte(topSongsBody))
	if err != nil {
		t.Fatalf("parseTopSongs: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("songs: %d, want 2", len(songs))
	}
	want := Song{
		ID: "t1", Artist: "Yeat", Title: "Bigger Thën Everything", Album: "bigger thën everything",
		Year: "2023", Released: "2023-06-02", ArtworkURL: "https://a/1.jpg", DurationMs: 200000,
	}
	if songs[0] != want {
		t.Errorf("songs[0] = %+v, want %+v", songs[0], want)
	}
	if songs[1].ID != "t2" || songs[1].Artist != "Drake" || songs[1].Title != "IDGAF (feat. Yeat)" || songs[1].Year != "2023" {
		t.Errorf("songs[1] = %+v", songs[1])
	}
}

func TestSearchTracksBadInput(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	ctx := context.Background()
	if res := a.dispatch(ctx, call("s1", "search_tracks", `{"queries":[]}`)); res != "error: 1–10 queries" {
		t.Errorf("0 queries: got %q", res)
	}
	qs := make([]string, 0, 11)
	for i := range 11 {
		qs = append(qs, fmt.Sprintf(`{"artist":"A%d"}`, i))
	}
	if res := a.dispatch(ctx, call("s2", "search_tracks", `{"queries":[`+strings.Join(qs, ",")+`]}`)); res != "error: 1–10 queries" {
		t.Errorf("11 queries: got %q", res)
	}
}
