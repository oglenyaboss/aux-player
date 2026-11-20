package chatgpt

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
)

const (
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	Issuer   = "https://auth.openai.com"
)

var ErrSignedOut = errors.New("chatgpt: signed out, sign in again")

var errNotSignedIn = errors.New("chatgpt: not signed in")

var loginPorts = []int{1455, 1457}

const (
	authFileName    = "chatgpt-auth.json"
	callbackPath    = "/auth/callback"
	authScope       = "openid profile email offline_access"
	refreshWindow   = 5 * time.Minute
	fallbackExpiry  = time.Hour
	maxTokenPayload = 1 << 20
	successPage     = `<!doctype html><html><head><meta charset="utf-8"><title>aux</title></head><body style="font-family:sans-serif;text-align:center;margin-top:20vh"><h1>aux: signed in — you can close this tab</h1></body></html>`
	failurePage     = `<!doctype html><html><head><meta charset="utf-8"><title>aux</title></head><body style="font-family:sans-serif;text-align:center;margin-top:20vh"><h1>aux: sign-in failed — you can close this tab</h1></body></html>`
)

type Tokens struct {
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	Expires      time.Time `json:"expires"`
}

type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Store struct {
	path   string
	issuer string
	http   *http.Client
	now    func() time.Time
	mu     sync.Mutex
	tok    *Tokens
}

func Path() (string, error) {
	cfgPath, err := config.ConfigPath("")
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), authFileName), nil
}

func Open(path string, h *http.Client) (*Store, error) {
	if h == nil {
		h = http.DefaultClient
	}
	s := &Store{
		path:   path,
		issuer: Issuer,
		http:   h,
		now:    time.Now,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("chatgpt: reading credentials: %w", err)
	}
	var tok Tokens
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("chatgpt: parsing credentials: %w", err)
	}
	s.tok = &tok
	return s, nil
}

func (s *Store) LoggedIn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tok != nil && s.tok.AccessToken != ""
}

func (s *Store) Access(ctx context.Context, force bool) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok == nil || s.tok.AccessToken == "" {
		return "", "", errNotSignedIn
	}
	if force || !s.now().Before(s.tok.Expires.Add(-refreshWindow)) {
		if err := s.refresh(ctx); err != nil {
			return "", "", err
		}
	}
	return s.tok.AccessToken, s.tok.AccountID, nil
}

func (s *Store) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("chatgpt: removing credentials: %w", err)
	}
	s.tok = nil
	return nil
}

func (s *Store) refresh(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     ClientID,
		"refresh_token": s.tok.RefreshToken,
	})
	if err != nil {
		return fmt.Errorf("chatgpt: encoding refresh request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.issuer+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("chatgpt: building refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("chatgpt: refreshing token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized:
		return fmt.Errorf("chatgpt: refresh rejected: %w", ErrSignedOut)
	case http.StatusOK:
	default:
		return fmt.Errorf("chatgpt: refresh failed: %s", resp.Status)
	}
	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenPayload)).Decode(&tr); err != nil {
		return fmt.Errorf("chatgpt: decoding refresh response: %w", err)
	}
	if tr.AccessToken == "" {
		return errors.New("chatgpt: refresh response has no access token")
	}
	next := *s.tok
	next.AccessToken = tr.AccessToken
	if tr.IDToken != "" {
		next.IDToken = tr.IDToken
	}
	if tr.RefreshToken != "" {
		next.RefreshToken = tr.RefreshToken
	}
	if id := accountIDFromIDToken(next.IDToken); id != "" {
		next.AccountID = id
	}
	next.Expires = expiry(s.now(), tr, next.AccessToken)
	if err := writeTokens(s.path, &next); err != nil {
		return err
	}
	s.tok = &next
	return nil
}

func (s *Store) exchange(ctx context.Context, code, verifier, redirectURI string) (*Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("chatgpt: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chatgpt: exchanging code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chatgpt: code exchange failed: %s", resp.Status)
	}
	var tr tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenPayload)).Decode(&tr); err != nil {
		return nil, fmt.Errorf("chatgpt: decoding token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("chatgpt: token response has no access token")
	}
	tok := &Tokens{
		IDToken:      tr.IDToken,
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		AccountID:    accountIDFromIDToken(tr.IDToken),
	}
	tok.Expires = expiry(s.now(), tr, tok.AccessToken)
	return tok, nil
}

type loginResult struct {
	tokens *Tokens
	err    error
}

func Login(ctx context.Context, s *Store, open func(url string) error) error {
	verifier, err := randomURL(64)
	if err != nil {
		return fmt.Errorf("chatgpt: generating code verifier: %w", err)
	}
	state, err := randomURL(32)
	if err != nil {
		return fmt.Errorf("chatgpt: generating state: %w", err)
	}
	challenge := codeChallenge(verifier)

	ln, port, err := listenCallback()
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	redirectURI := fmt.Sprintf("http://localhost:%d%s", port, callbackPath)
	authURL := authorizeURL(s.issuer, redirectURI, challenge, state)

	results := make(chan loginResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		if !stateMatches(state, q.Get("state")) {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		if authErr := q.Get("error"); authErr != "" {
			writePage(w, http.StatusBadRequest, failurePage)
			sendResult(results, loginResult{err: fmt.Errorf("chatgpt: authorization failed: %s", authErr)})
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		tok, err := s.exchange(ctx, code, verifier, redirectURI)
		if err != nil {
			writePage(w, http.StatusBadGateway, failurePage)
			sendResult(results, loginResult{err: err})
			return
		}
		writePage(w, http.StatusOK, successPage)
		sendResult(results, loginResult{tokens: tok})
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := open(authURL); err != nil {
		return fmt.Errorf("chatgpt: opening browser: %w", err)
	}

	select {
	case res := <-results:
		if res.err != nil {
			return res.err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := writeTokens(s.path, res.tokens); err != nil {
			return err
		}
		s.tok = res.tokens
		return nil
	case err := <-serveErr:
		return fmt.Errorf("chatgpt: callback server: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sendResult(results chan<- loginResult, res loginResult) {
	select {
	case results <- res:
	default:
	}
}

func authorizeURL(issuer, redirectURI, challenge, state string) string {
	q := url.Values{
		"response_type":              {"code"},
		"client_id":                  {ClientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {authScope},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {"codex_cli_rs"},
	}
	return issuer + "/oauth/authorize?" + q.Encode()
}

func listenCallback() (net.Listener, int, error) {
	var lastErr error
	for _, port := range loginPorts {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no callback ports configured")
	}
	return nil, 0, fmt.Errorf("chatgpt: starting callback server: %w", lastErr)
}

func stateMatches(want, got string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func randomURL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func writePage(w http.ResponseWriter, status int, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func expiry(now time.Time, tr tokenResponse, accessToken string) time.Time {
	if tr.ExpiresIn > 0 {
		return now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	if exp, ok := jwtExp(accessToken); ok {
		return exp
	}
	return now.Add(fallbackExpiry)
}

func jwtPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 || parts[1] == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func accountIDFromIDToken(idToken string) string {
	payload := jwtPayload(idToken)
	auth, ok := payload["https://api.openai.com/auth"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := auth["chatgpt_account_id"].(string)
	return id
}

func jwtExp(token string) (time.Time, bool) {
	payload := jwtPayload(token)
	exp, ok := payload["exp"].(float64)
	if !ok || exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(exp), 0), true
}

func writeTokens(path string, tok *Tokens) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("chatgpt: creating auth dir: %w", err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("chatgpt: encoding credentials: %w", err)
	}
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("chatgpt: writing credentials: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("chatgpt: writing credentials: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chatgpt: writing credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chatgpt: saving credentials: %w", err)
	}
	return nil
}
