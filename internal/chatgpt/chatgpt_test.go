package chatgpt

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening for a free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func useLoginPorts(t *testing.T, ports ...int) {
	t.Helper()
	previous := loginPorts
	loginPorts = ports
	t.Cleanup(func() { loginPorts = previous })
}

func idTokenWithAccount(accountID string) string {
	payload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID},
	})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

type capturedRequest struct {
	contentType string
	form        url.Values
	body        map[string]any
}

type tokenServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

func newTokenServer(t *testing.T, respond func(w http.ResponseWriter, n int)) *tokenServer {
	t.Helper()
	ts := &tokenServer{}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		req := capturedRequest{contentType: r.Header.Get("Content-Type")}
		if strings.HasPrefix(req.contentType, "application/json") {
			if err := json.NewDecoder(r.Body).Decode(&req.body); err != nil {
				t.Errorf("decoding JSON token request: %v", err)
			}
		} else if err := r.ParseForm(); err != nil { //nolint:gosec // G120: the test server's own form
			t.Errorf("parsing form token request: %v", err)
		} else {
			req.form = r.PostForm
		}
		ts.mu.Lock()
		ts.requests = append(ts.requests, req)
		n := len(ts.requests)
		ts.mu.Unlock()
		respond(w, n)
	}))
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *tokenServer) count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.requests)
}

func (ts *tokenServer) request(i int) capturedRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requests[i]
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writing token response: %v", err)
	}
}

func newEmptyStore(t *testing.T, issuerURL string) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), authFileName), nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.issuer = issuerURL
	return s
}

func newStoredStore(t *testing.T, tok *Tokens, issuerURL string, now func() time.Time) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), authFileName)
	data, err := json.Marshal(tok) //nolint:gosec // G117: the test tokens
	if err != nil {
		t.Fatalf("marshalling test tokens: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing test tokens: %v", err)
	}
	s, err := Open(path, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.issuer = issuerURL
	if now != nil {
		s.now = now
	}
	return s
}

func readTokensFile(t *testing.T, path string) Tokens {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: the test store's own path
	if err != nil {
		t.Fatalf("reading credentials: %v", err)
	}
	var tok Tokens
	if err := json.Unmarshal(data, &tok); err != nil {
		t.Fatalf("unmarshalling credentials: %v", err)
	}
	return tok
}

func TestLoginStoresTokens(t *testing.T) {
	useLoginPorts(t, freePort(t))
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		writeJSON(t, w, map[string]any{
			"id_token":      idTokenWithAccount("acc-1"),
			"access_token":  "a1",
			"refresh_token": "r1",
			"expires_in":    3600,
		})
	})
	s := newEmptyStore(t, ts.srv.URL)

	var authz url.Values
	var redirectURI string
	open := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		if got, want := u.Scheme+"://"+u.Host+u.Path, ts.srv.URL+"/oauth/authorize"; got != want {
			return fmt.Errorf("authorize URL = %q, want %q", got, want)
		}
		authz = u.Query()
		redirectURI = authz.Get("redirect_uri")
		state := authz.Get("state")
		resp, err := http.Get(redirectURI + "?code=c1&state=" + url.QueryEscape(state)) //nolint:gosec // G107: localhost URL built by Login
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("callback status = %d, body = %s", resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "signed in") {
			return fmt.Errorf("callback body = %q", body)
		}
		return nil
	}
	if err := Login(context.Background(), s, open); err != nil {
		t.Fatalf("Login: %v", err)
	}

	for key, want := range map[string]string{
		"response_type":              "code",
		"client_id":                  ClientID,
		"scope":                      "openid profile email offline_access",
		"code_challenge_method":      "S256",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"originator":                 "codex_cli_rs",
	} {
		if got := authz.Get(key); got != want {
			t.Errorf("authorize %s = %q, want %q", key, got, want)
		}
	}
	if authz.Get("code_challenge") == "" {
		t.Error("authorize URL has no code_challenge")
	}
	if authz.Get("state") == "" {
		t.Error("authorize URL has no state")
	}
	ru, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatalf("parsing redirect_uri: %v", err)
	}
	if ru.Scheme != "http" || ru.Hostname() != "localhost" || ru.Path != "/auth/callback" || ru.Port() == "" {
		t.Errorf("redirect_uri = %q, want http://localhost:<port>/auth/callback", redirectURI)
	}

	if got := ts.count(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
	req := ts.request(0)
	if req.contentType != "application/x-www-form-urlencoded" {
		t.Errorf("token Content-Type = %q, want application/x-www-form-urlencoded", req.contentType)
	}
	for key, want := range map[string]string{
		"grant_type":   "authorization_code",
		"client_id":    ClientID,
		"code":         "c1",
		"redirect_uri": redirectURI,
	} {
		if got := req.form.Get(key); got != want {
			t.Errorf("token %s = %q, want %q", key, got, want)
		}
	}
	sum := sha256.Sum256([]byte(req.form.Get("code_verifier")))
	if got := base64.RawURLEncoding.EncodeToString(sum[:]); got != authz.Get("code_challenge") {
		t.Errorf("code_verifier sha256 = %q, want code_challenge %q", got, authz.Get("code_challenge"))
	}

	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("credentials mode = %o, want 600", got)
	}
	disk := readTokensFile(t, s.path)
	if disk.AccountID != "acc-1" {
		t.Errorf("account_id = %q, want acc-1", disk.AccountID)
	}
	if disk.AccessToken != "a1" || disk.RefreshToken != "r1" {
		t.Errorf("tokens = %+v, want access a1 and refresh r1", disk)
	}
	if !s.LoggedIn() {
		t.Error("LoggedIn = false after Login")
	}
}

func TestLoginIgnoresWrongState(t *testing.T) {
	useLoginPorts(t, freePort(t))
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		writeJSON(t, w, map[string]any{
			"id_token":      idTokenWithAccount("acc-1"),
			"access_token":  "a1",
			"refresh_token": "r1",
			"expires_in":    3600,
		})
	})
	s := newEmptyStore(t, ts.srv.URL)

	open := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		redirectURI := q.Get("redirect_uri")

		wrong, err := http.Get(redirectURI + "?code=stale&state=wrong-state") //nolint:gosec // G107: localhost URL built by Login
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, wrong.Body)
		_ = wrong.Body.Close()
		if wrong.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("wrong-state callback status = %d, want 400", wrong.StatusCode)
		}

		right, err := http.Get(redirectURI + "?code=c2&state=" + url.QueryEscape(q.Get("state"))) //nolint:gosec // G107: localhost URL built by Login
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, right.Body)
		_ = right.Body.Close()
		if right.StatusCode != http.StatusOK {
			return fmt.Errorf("right-state callback status = %d, want 200", right.StatusCode)
		}
		return nil
	}
	if err := Login(context.Background(), s, open); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got := ts.count(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
	if code := ts.request(0).form.Get("code"); code != "c2" {
		t.Errorf("exchanged code = %q, want c2", code)
	}
	if !s.LoggedIn() {
		t.Error("LoggedIn = false after Login")
	}
}

func TestLoginReturnsErrorParam(t *testing.T) {
	useLoginPorts(t, freePort(t))
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s := newEmptyStore(t, ts.srv.URL)

	open := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		resp, err := http.Get(q.Get("redirect_uri") + "?error=access_denied&state=" + url.QueryEscape(q.Get("state"))) //nolint:gosec // G107: localhost URL built by Login
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	err := Login(context.Background(), s, open)
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("Login error = %v, want one containing access_denied", err)
	}
	if got := ts.count(); got != 0 {
		t.Errorf("token requests = %d, want 0", got)
	}
}

func TestAccessUsesFreshToken(t *testing.T) {
	now := time.Now()
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s := newStoredStore(t, &Tokens{
		AccessToken:  "a1",
		RefreshToken: "r1",
		AccountID:    "acc-1",
		Expires:      now.Add(time.Hour),
	}, ts.srv.URL, func() time.Time { return now })

	token, account, err := s.Access(context.Background(), false)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if token != "a1" || account != "acc-1" {
		t.Errorf("Access = (%q, %q), want (a1, acc-1)", token, account)
	}
	if got := ts.count(); got != 0 {
		t.Errorf("issuer requests = %d, want 0", got)
	}
}

func TestAccessRefreshesNearExpiry(t *testing.T) {
	now := time.Now()
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		writeJSON(t, w, map[string]any{
			"access_token":  "a2",
			"refresh_token": "r2",
			"expires_in":    3600,
		})
	})
	s := newStoredStore(t, &Tokens{
		AccessToken:  "a1",
		RefreshToken: "r1",
		AccountID:    "acc-1",
		Expires:      now.Add(time.Minute),
	}, ts.srv.URL, func() time.Time { return now })

	token, account, err := s.Access(context.Background(), false)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if token != "a2" || account != "acc-1" {
		t.Errorf("Access = (%q, %q), want (a2, acc-1)", token, account)
	}
	if got := ts.count(); got != 1 {
		t.Fatalf("issuer requests = %d, want 1", got)
	}
	req := ts.request(0)
	if req.contentType != "application/json" {
		t.Errorf("refresh Content-Type = %q, want application/json", req.contentType)
	}
	for key, want := range map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     ClientID,
		"refresh_token": "r1",
	} {
		if got, _ := req.body[key].(string); got != want {
			t.Errorf("refresh %s = %q, want %q", key, got, want)
		}
	}
	disk := readTokensFile(t, s.path)
	if disk.RefreshToken != "r2" {
		t.Errorf("stored refresh token = %q, want r2", disk.RefreshToken)
	}
	if disk.AccountID != "acc-1" {
		t.Errorf("stored account id = %q, want acc-1", disk.AccountID)
	}
	if disk.AccessToken != "a2" {
		t.Errorf("stored access token = %q, want a2", disk.AccessToken)
	}
}

func TestAccessForceKeepsRefreshToken(t *testing.T) {
	now := time.Now()
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		writeJSON(t, w, map[string]any{
			"access_token": "a2",
			"expires_in":   3600,
		})
	})
	s := newStoredStore(t, &Tokens{
		AccessToken:  "a1",
		RefreshToken: "r1",
		AccountID:    "acc-1",
		Expires:      now.Add(time.Hour),
	}, ts.srv.URL, func() time.Time { return now })

	token, account, err := s.Access(context.Background(), true)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if token != "a2" || account != "acc-1" {
		t.Errorf("Access = (%q, %q), want (a2, acc-1)", token, account)
	}
	if got := ts.count(); got != 1 {
		t.Fatalf("issuer requests = %d, want 1", got)
	}
	disk := readTokensFile(t, s.path)
	if disk.RefreshToken != "r1" {
		t.Errorf("stored refresh token = %q, want r1", disk.RefreshToken)
	}
	if disk.AccountID != "acc-1" {
		t.Errorf("stored account id = %q, want acc-1", disk.AccountID)
	}
}

func TestAccessRefreshRejected(t *testing.T) {
	now := time.Now()
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	s := newStoredStore(t, &Tokens{
		AccessToken:  "a1",
		RefreshToken: "r1",
		AccountID:    "acc-1",
		Expires:      now.Add(-time.Minute),
	}, ts.srv.URL, func() time.Time { return now })

	_, _, err := s.Access(context.Background(), false)
	if !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Access error = %v, want ErrSignedOut", err)
	}
}

func TestAccessConcurrentRefresh(t *testing.T) {
	now := time.Now()
	ts := newTokenServer(t, func(w http.ResponseWriter, n int) {
		writeJSON(t, w, map[string]any{
			"access_token":  "a2",
			"refresh_token": "r2",
			"expires_in":    3600,
		})
	})
	s := newStoredStore(t, &Tokens{
		AccessToken:  "a1",
		RefreshToken: "r1",
		AccountID:    "acc-1",
		Expires:      now.Add(-time.Minute),
	}, ts.srv.URL, func() time.Time { return now })

	type accessResult struct {
		token   string
		account string
		err     error
	}
	results := make([]accessResult, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i].token, results[i].account, results[i].err = s.Access(context.Background(), false)
		}(i)
	}
	close(start)
	wg.Wait()

	if got := ts.count(); got != 1 {
		t.Fatalf("issuer requests = %d, want 1", got)
	}
	for i, res := range results {
		if res.err != nil {
			t.Errorf("Access[%d] error: %v", i, res.err)
			continue
		}
		if res.token != "a2" || res.account != "acc-1" {
			t.Errorf("Access[%d] = (%q, %q), want (a2, acc-1)", i, res.token, res.account)
		}
	}
}

func TestOpenMissingAndLogout(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		s := newEmptyStore(t, "http://127.0.0.1:1")
		if s.LoggedIn() {
			t.Error("LoggedIn = true for a missing file")
		}
		if _, _, err := s.Access(context.Background(), false); err == nil || !strings.Contains(err.Error(), "not signed in") {
			t.Errorf("Access error = %v, want one containing not signed in", err)
		}
	})

	t.Run("logout removes the file", func(t *testing.T) {
		s := newStoredStore(t, &Tokens{
			AccessToken:  "a1",
			RefreshToken: "r1",
			AccountID:    "acc-1",
			Expires:      time.Now().Add(time.Hour),
		}, "http://127.0.0.1:1", nil)
		if !s.LoggedIn() {
			t.Fatal("LoggedIn = false for a stored token")
		}
		if err := s.Logout(); err != nil {
			t.Fatalf("Logout: %v", err)
		}
		if _, err := os.Stat(s.path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat after Logout = %v, want not-exist", err)
		}
		if s.LoggedIn() {
			t.Error("LoggedIn = true after Logout")
		}
	})

	t.Run("logout without a file", func(t *testing.T) {
		s := newEmptyStore(t, "http://127.0.0.1:1")
		if err := s.Logout(); err != nil {
			t.Fatalf("Logout on a missing file: %v", err)
		}
	})
}
