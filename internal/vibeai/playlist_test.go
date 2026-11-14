package vibeai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type capturedPost struct {
	method string
	path   string
	origin string
	ctype  string
	user   string
	body   string
}

func postServer(t *testing.T, status int, respBody string) (*appleAPI, *[]capturedPost) {
	t.Helper()
	var mu sync.Mutex
	reqs := make([]capturedPost, 0, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, capturedPost{
			method: r.Method,
			path:   r.URL.Path,
			origin: r.Header.Get("Origin"),
			ctype:  r.Header.Get("Content-Type"),
			user:   r.Header.Get("Music-User-Token"),
			body:   string(b),
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	api := &appleAPI{http: srv.Client(), dev: "dev-token", user: "secret-user-token", base: srv.URL}
	return api, &reqs
}

func TestCreatePlaylist(t *testing.T) {
	api, reqs := postServer(t, http.StatusCreated, `{"data":[{"id":"p.abc"}]}`)
	id, err := createPlaylist(context.Background(), api, "Порция 1", "AUX · «рейдж»", []string{"1", "2"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != "p.abc" {
		t.Errorf("id = %q, want p.abc", id)
	}
	if len(*reqs) != 1 {
		t.Fatalf("requests %d, want 1", len(*reqs))
	}
	r := (*reqs)[0]
	if r.method != http.MethodPost || r.path != "/me/library/playlists" {
		t.Errorf("request: %s %s, want POST /me/library/playlists", r.method, r.path)
	}
	if r.origin != "https://music.apple.com" {
		t.Errorf("Origin = %q", r.origin)
	}
	if r.ctype != "application/json" {
		t.Errorf("Content-Type = %q", r.ctype)
	}
	if r.user != "secret-user-token" {
		t.Errorf("Music-User-Token = %q", r.user)
	}
	if !strings.Contains(r.body, `"name":"Порция 1"`) || !strings.Contains(r.body, `"description":"AUX · «рейдж»"`) {
		t.Errorf("attributes: %s", r.body)
	}
	if n := strings.Count(r.body, `{"id":"1","type":"songs"}`) + strings.Count(r.body, `{"id":"2","type":"songs"}`); n != 2 {
		t.Errorf("body without the two tracks: %s", r.body)
	}
}

func TestCreatePlaylistNoID(t *testing.T) {
	api, _ := postServer(t, http.StatusCreated, "")
	_, err := createPlaylist(context.Background(), api, "X", "desc", []string{"1"})
	if err == nil || !strings.Contains(err.Error(), "no id returned") {
		t.Errorf("err = %v, want \"no id returned\"", err)
	}
}

func TestCreatePlaylistEmpty(t *testing.T) {
	api, reqs := postServer(t, http.StatusCreated, `{"data":[{"id":"p.1"}]}`)
	if _, err := createPlaylist(context.Background(), api, "X", "desc", nil); err == nil || err.Error() != "apple: empty playlist" {
		t.Errorf("err = %v, want «apple: empty playlist»", err)
	}
	if len(*reqs) != 0 {
		t.Errorf("an empty list must not go to the server")
	}
}

func TestAddToPlaylistBatches(t *testing.T) {
	api, reqs := postServer(t, http.StatusOK, "")
	ids := make([]string, 30)
	for i := range ids {
		ids[i] = "id" + string(rune('a'+i))
	}
	if err := addToPlaylist(context.Background(), api, "p.abc", ids); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(*reqs) != 2 {
		t.Fatalf("requests %d, want 2", len(*reqs))
	}
	for i, r := range *reqs {
		if r.method != http.MethodPost || r.path != "/me/library/playlists/p.abc/tracks" {
			t.Errorf("request %d: %s %s", i, r.method, r.path)
		}
	}
	if got := strings.Count((*reqs)[0].body, `"type":"songs"`); got != 25 {
		t.Errorf("%d tracks in the first batch, want 25", got)
	}
	if got := strings.Count((*reqs)[1].body, `"type":"songs"`); got != 5 {
		t.Errorf("%d tracks in the second batch, want 5", got)
	}
}

func TestAddToPlaylistEmpty(t *testing.T) {
	api, reqs := postServer(t, http.StatusOK, "")
	if err := addToPlaylist(context.Background(), api, "p.abc", nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(*reqs) != 0 {
		t.Errorf("an empty list must not go to the server")
	}
}

func TestPostErrorNoTokenLeak(t *testing.T) {
	api, _ := postServer(t, http.StatusUnauthorized, "unauthorized")
	_, _, err := api.post(context.Background(), "/me/library/playlists", map[string]string{"x": "y"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v, want HTTP 401", err)
	}
	if strings.Contains(err.Error(), "secret-user-token") {
		t.Errorf("the error exposes the user token: %v", err)
	}
}

func TestParseCreatedPlaylistID(t *testing.T) {
	if got := parseCreatedPlaylistID([]byte("")); got != "" {
		t.Errorf("empty body: got %q, want \"\"", got)
	}
	if got := parseCreatedPlaylistID([]byte(`{}`)); got != "" {
		t.Errorf("{}: got %q, want \"\"", got)
	}
	if got := parseCreatedPlaylistID([]byte(`{"data":[{"id":"p.x"}]}`)); got != "p.x" {
		t.Errorf("got %q, want p.x", got)
	}
}

func TestEngineSaveAndAppendPlaylist(t *testing.T) {
	api, reqs := postServer(t, http.StatusCreated, `{"data":[{"id":"p.abc"}]}`)
	e := &Engine{api: api, log: &runLog{}, query: "рейдж 2024"}
	tracks := []Track{{ID: "1", Artist: "A", Title: "T1"}, {ID: "2", Artist: "B", Title: "T2"}}
	id, err := e.SavePlaylist(context.Background(), "Порция 1", "", tracks)
	if err != nil || id != "p.abc" {
		t.Fatalf("SavePlaylist: (%q, %v)", id, err)
	}
	if !strings.Contains(e.log.b.String(), "playlist: Порция 1 (2 tracks)") {
		t.Errorf("log: %q", e.log.b.String())
	}
	if !strings.Contains((*reqs)[0].body, `"description":"AUX · «рейдж 2024»"`) {
		t.Errorf("description: %s", (*reqs)[0].body)
	}

	// appending to the same server: 201 with an empty body — the tracks are already added
	api2, reqs2 := postServer(t, http.StatusCreated, "")
	e2 := &Engine{api: api2, log: &runLog{}, query: "рейдж 2024"}
	if err := e2.AppendPlaylist(context.Background(), "p.abc", tracks); err != nil {
		t.Fatalf("AppendPlaylist: %v", err)
	}
	if !strings.Contains(e2.log.b.String(), "playlist +2") {
		t.Errorf("log: %q", e2.log.b.String())
	}
	if len(*reqs2) != 1 || (*reqs2)[0].path != "/me/library/playlists/p.abc/tracks" {
		t.Errorf("requests: %+v", *reqs2)
	}
}
