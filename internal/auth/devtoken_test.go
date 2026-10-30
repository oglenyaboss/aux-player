package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
)

const (
	testBundlePath = "/assets/index~abc123.js"
	testPage       = `<html><script type="module" crossorigin src="/assets/index~abc123.js"></script></html>`
	cfgToken       = "eyJ.cfg.tok" //nolint:gosec // G101: test value, not a real credential
)

// testJWT builds a token shaped like the ones the web player bundle carries:
// a base64url header naming the kid, a payload carrying exp, and a signature.
func testJWT(t *testing.T, kid string, exp time.Time) string {
	t.Helper()
	header := testSegment(t, map[string]any{"typ": "JWT", "alg": "ES256", "kid": kid})
	payload := testSegment(t, map[string]any{
		"iss":               "https://music.apple.com",
		"iat":               exp.Add(-70 * 24 * time.Hour).Unix(),
		"exp":               exp.Unix(),
		"root_https_origin": []string{"music.apple.com"},
	})
	return header + "." + payload + "." + strings.Repeat("sig", 15)
}

func testSegment(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshalling test JWT claims: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// newWebPlayerServer serves the page and the bundle it names, counting every
// request it receives.
func newWebPlayerServer(t *testing.T, page, bundle string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case webPlayerPage:
			_, _ = w.Write([]byte(page))
		case testBundlePath:
			_, _ = w.Write([]byte(bundle))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// isolateCache points the developer token cache at a fresh directory, so a test
// neither reads nor leaves a cached token.
func isolateCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
}

// stubWebPlayer points the web player at a local server that serves a
// WebPlayKid token, leaves the token cache empty and returns that token.
func stubWebPlayer(t *testing.T, exp time.Time) string {
	t.Helper()
	isolateCache(t)
	tok := testJWT(t, webPlayKid, exp)
	srv, _ := newWebPlayerServer(t, testPage, bundleOf(tok))
	orig := webPlayerURL
	webPlayerURL = srv.URL
	t.Cleanup(func() { webPlayerURL = orig })
	return tok
}

// stubWebPlayerDown points the web player at a server that answers 404 and
// leaves the token cache empty, so resolving a developer token fails.
func stubWebPlayerDown(t *testing.T) {
	t.Helper()
	isolateCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	orig := webPlayerURL
	webPlayerURL = srv.URL
	t.Cleanup(func() { webPlayerURL = orig })
}

// bundleOf wraps tokens in JS so they appear where the real bundle keeps them.
func bundleOf(tokens ...string) string {
	quoted := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		quoted = append(quoted, `"`+tok+`"`)
	}
	return "var a=[" + strings.Join(quoted, ",") + "];"
}

func TestWebPlayerToken_FetchesWebPlayTokenAndCaches(t *testing.T) {
	now := time.Now()
	ltExp := now.Add(30 * 24 * time.Hour)
	webExp := now.Add(70 * 24 * time.Hour)
	lt := testJWT(t, "LT", ltExp)
	web := testJWT(t, webPlayKid, webExp)

	srv, hits := newWebPlayerServer(t, testPage, bundleOf(lt, web))
	cache := filepath.Join(t.TempDir(), "sub", "devtoken.json")

	got, err := webPlayerToken(srv.URL, cache, now)
	if err != nil || got != web {
		t.Fatalf("webPlayerToken = %q, %v; want the WebPlayKid token", got, err)
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2 (page and bundle)", hits.Load())
	}

	cached, err := readCache(cache)
	if err != nil {
		t.Fatalf("reading cache: %v", err)
	}
	if cached.Token != web {
		t.Errorf("cached token is not the WebPlayKid token")
	}
	if cached.ExpiresAt != webExp.Unix() {
		t.Errorf("cached expires_at = %d, want %d", cached.ExpiresAt, webExp.Unix())
	}

	for _, line := range TokenLogs() {
		if strings.Contains(line, web) {
			t.Errorf("log line leaks the token: %q", line)
		}
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(cache)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("cache mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestWebPlayerToken_UsesFreshCacheWithoutNetwork(t *testing.T) {
	now := time.Now()
	exp := now.Add(10 * 24 * time.Hour)
	web := testJWT(t, webPlayKid, exp)

	cache := filepath.Join(t.TempDir(), "devtoken.json")
	if err := writeCache(cache, cachedToken{Token: web, ExpiresAt: exp.Unix()}); err != nil {
		t.Fatal(err)
	}

	srv, hits := newWebPlayerServer(t, testPage, "")
	got, err := webPlayerToken(srv.URL, cache, now)
	if err != nil || got != web {
		t.Fatalf("webPlayerToken = %q, %v; want the cached token", got, err)
	}
	if hits.Load() != 0 {
		t.Errorf("hits = %d, want 0 for a cache with 10 days left", hits.Load())
	}
}

func TestWebPlayerToken_FallsBackToUnexpiredCache(t *testing.T) {
	now := time.Now()
	exp := now.Add(24 * time.Hour)
	web := testJWT(t, webPlayKid, exp)

	cache := filepath.Join(t.TempDir(), "devtoken.json")
	if err := writeCache(cache, cachedToken{Token: web, ExpiresAt: exp.Unix()}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	got, err := webPlayerToken(srv.URL, cache, now)
	if err != nil || got != web {
		t.Fatalf("webPlayerToken = %q, %v; want the cached token despite the 500", got, err)
	}
}

func TestWebPlayerToken_RejectsPageWithoutBundle(t *testing.T) {
	srv, _ := newWebPlayerServer(t, "<html><body>no script here</body></html>", "")
	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), time.Now()); err == nil {
		t.Error("webPlayerToken accepted a page without a script bundle")
	}
}

func TestWebPlayerToken_RejectsBundleWithoutWebPlayKid(t *testing.T) {
	now := time.Now()
	srv, _ := newWebPlayerServer(t, testPage, bundleOf(testJWT(t, "LT", now.Add(30*24*time.Hour))))
	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), now); err == nil {
		t.Error("webPlayerToken accepted a bundle without a WebPlayKid token")
	}
}

func TestWebPlayerToken_RejectsExpiredToken(t *testing.T) {
	now := time.Now()
	srv, _ := newWebPlayerServer(t, testPage, bundleOf(testJWT(t, webPlayKid, now.Add(-time.Hour))))
	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), now); err == nil {
		t.Error("webPlayerToken accepted an expired WebPlayKid token")
	}
}

func TestWebPlayerToken_RefusesForeignBundle(t *testing.T) {
	page := `<html><script type="module" crossorigin src="https://evil.example/assets/index~x.js"></script></html>`
	srv, hits := newWebPlayerServer(t, page, "")
	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), time.Now()); err == nil {
		t.Error("webPlayerToken accepted a bundle hosted on another host")
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1 (the page only)", hits.Load())
	}
}

func TestWebPlayerToken_FollowsSameHostRedirect(t *testing.T) {
	now := time.Now()
	web := testJWT(t, webPlayKid, now.Add(70*24*time.Hour))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case webPlayerPage:
			http.Redirect(w, r, "/us/new2", http.StatusMovedPermanently)
		case "/us/new2":
			_, _ = w.Write([]byte(testPage))
		case testBundlePath:
			_, _ = w.Write([]byte(bundleOf(web)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	got, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), now)
	if err != nil || got != web {
		t.Fatalf("webPlayerToken = %q, %v; want the token behind the redirect", got, err)
	}
}

func TestWebPlayerToken_RefusesForeignRedirect(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		leaked.Add(1)
	}))
	defer other.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+webPlayerPage, http.StatusMovedPermanently)
	}))
	defer srv.Close()

	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), time.Now()); err == nil {
		t.Error("webPlayerToken followed a redirect to another host")
	}
	if leaked.Load() != 0 {
		t.Error("the request reached the redirect target")
	}
}

func TestWebPlayerToken_RetriesTransientErrors(t *testing.T) {
	origBackoff := retryBackoff
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })

	now := time.Now()
	web := testJWT(t, webPlayKid, now.Add(70*24*time.Hour))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case webPlayerPage:
			_, _ = w.Write([]byte(testPage))
		case testBundlePath:
			_, _ = w.Write([]byte(bundleOf(web)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	tok, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), now)
	if err != nil || tok != web {
		t.Fatalf("webPlayerToken = %q, %v; want a token after the retry", tok, err)
	}
	if hits.Load() != 3 {
		t.Errorf("hits = %d, want 3 (failed page, page, bundle)", hits.Load())
	}

	logs := strings.Join(TokenLogs(), "\n")
	if !strings.Contains(logs, "[devtoken] attempt 1/3 failed") || !strings.Contains(logs, "retrying in") {
		t.Errorf("missing retry log messages in %q", logs)
	}
}

func TestWebPlayerToken_DoesNotRetryClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := webPlayerToken(srv.URL, filepath.Join(t.TempDir(), "devtoken.json"), time.Now()); err == nil {
		t.Error("webPlayerToken accepted a 404 page")
	}
}

func TestWebPlayerToken_RefusesPlainHTTPToRemoteHost(t *testing.T) {
	if _, err := fetchToken("http://example.com", time.Now()); err == nil {
		t.Error("fetchToken accepted http to a non-loopback host")
	}
}

func TestResolveDeveloperToken_UsesWebPlayerToken(t *testing.T) {
	orig := webPlayerURL
	t.Cleanup(func() { webPlayerURL = orig })
	isolateCache(t)

	now := time.Now()
	web := testJWT(t, webPlayKid, now.Add(70*24*time.Hour))
	srv, _ := newWebPlayerServer(t, testPage, bundleOf(web))
	webPlayerURL = srv.URL

	cfg := &config.Config{AppleDeveloperToken: "stale.tok"}
	if err := ResolveDeveloperToken(cfg); err != nil || cfg.AppleDeveloperToken != web {
		t.Fatalf("ResolveDeveloperToken = %v, token %q; want the fetched token", err, cfg.AppleDeveloperToken)
	}
	if _, err := readCache(cachePath()); err != nil {
		t.Errorf("cache not written to %s: %v", cachePath(), err)
	}
}

func TestResolveDeveloperToken_KeepsConfigTokenOnFetchFailure(t *testing.T) {
	orig := webPlayerURL
	t.Cleanup(func() { webPlayerURL = orig })
	origBackoff := retryBackoff
	retryBackoff = time.Millisecond
	t.Cleanup(func() { retryBackoff = origBackoff })
	isolateCache(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()
	webPlayerURL = srv.URL

	cfg := &config.Config{AppleDeveloperToken: cfgToken}
	if err := ResolveDeveloperToken(cfg); err == nil {
		t.Fatal("want an error when the web player is unreachable and nothing is cached")
	}
	if cfg.AppleDeveloperToken != cfgToken {
		t.Errorf("config token clobbered: %q", cfg.AppleDeveloperToken)
	}
}

func TestCheckTokensRefreshing_RefreshesRejectedDeveloperToken(t *testing.T) {
	cfg := &config.Config{AppleDeveloperToken: "stale.tok", AppleUserToken: "user.tok"}
	statuses := []TokenStatus{DeveloperTokenRejected, TokensValid}
	var checks, refreshes int

	check := func(string, string) (TokenStatus, string) {
		status := statuses[checks]
		checks++
		return status, "us"
	}
	refresh := func() error {
		refreshes++
		cfg.AppleDeveloperToken = "fresh.tok"
		return nil
	}

	status, sf := checkTokensRefreshing(check, refresh, cfg)
	if status != TokensValid || sf != "us" {
		t.Errorf("checkTokensRefreshing = %v, %q; want TokensValid, us", status, sf)
	}
	if checks != 2 {
		t.Errorf("checks = %d, want 2", checks)
	}
	if refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", refreshes)
	}
}

func TestCheckTokensRefreshing_KeepsRejectionWhenRefreshFails(t *testing.T) {
	cfg := &config.Config{AppleDeveloperToken: "stale.tok", AppleUserToken: "user.tok"}
	checks := 0

	check := func(string, string) (TokenStatus, string) {
		checks++
		return DeveloperTokenRejected, ""
	}
	refresh := func() error { return errors.New("offline") }

	status, _ := checkTokensRefreshing(check, refresh, cfg)
	if status != DeveloperTokenRejected {
		t.Errorf("checkTokensRefreshing = %v, want DeveloperTokenRejected", status)
	}
	if checks != 1 {
		t.Errorf("checks = %d, want 1 when the refresh fails", checks)
	}
}

// TestLiveWebPlayerToken fetches the token from the real music.apple.com. It
// is skipped unless AUX_LIVE=1, because it needs the network and Apple's
// current bundle layout.
func TestLiveWebPlayerToken(t *testing.T) {
	if os.Getenv("AUX_LIVE") != "1" {
		t.Skip("set AUX_LIVE=1 to run the live music.apple.com test")
	}

	now := time.Now()
	tok, err := webPlayerToken(webPlayerURL, filepath.Join(t.TempDir(), "devtoken.json"), now)
	if err != nil {
		t.Fatalf("webPlayerToken: %v", err)
	}
	kid, ok := jwtKid(tok)
	if !ok || kid != webPlayKid {
		t.Fatalf("token kid = %q, want %q", kid, webPlayKid)
	}
	exp, ok := jwtExp(tok)
	if !ok {
		t.Fatal("token has no exp claim")
	}
	if !time.Unix(exp, 0).After(now.Add(24 * time.Hour)) {
		t.Errorf("token expires in less than a day: %s", time.Unix(exp, 0))
	}
	t.Logf("web player token: %d bytes, %d days to expiry", len(tok), int(time.Until(time.Unix(exp, 0)).Hours()/24))
}
