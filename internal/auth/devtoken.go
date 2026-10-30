package auth

import (
	"context"
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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/oglenyaboss/aux-player/internal/config"
)

// webPlayerURL is the origin of the Apple Music web player. Its pages and
// scripts carry the public developer token Apple ships to every browser, so
// AUX reads the token from there instead of signing one or asking a broker.
// Tests point this at a local server.
var webPlayerURL = "https://music.apple.com" //nolint:gochecknoglobals // overridden in tests

const (
	// webPlayerUserAgent is the desktop Chrome user agent the web player is
	// requested with.
	webPlayerUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36"
	// webPlayerPage is the path whose HTML names the current script bundle.
	webPlayerPage = "/us/new"
	// webPlayKid identifies the token MusicKit uses in the browser. The bundle
	// carries other JWTs whose kid Apple rejects for catalog requests.
	webPlayKid = "WebPlayKid"

	// refreshBefore is how close to expiry a cached token stops being reused
	// without trying to fetch a fresh one. Web player tokens live ~70 days, so
	// refreshing at 3 days guarantees every session starts with a usable one.
	refreshBefore = 72 * time.Hour
	// fetchTimeout bounds a cold-start page and bundle fetch when no valid
	// cached token exists.
	fetchTimeout = 15 * time.Second
	// refreshTimeout bounds an opportunistic refresh when a valid cached token
	// is available to fall back on, keeping offline/slow startup fast.
	refreshTimeout = 5 * time.Second
	// maxHTMLBytes bounds the web player page body.
	maxHTMLBytes = 8 << 20
	// maxBundleBytes bounds the script bundle body; it is ~3.3 MiB today.
	maxBundleBytes = 16 << 20
	// maxRedirects bounds same-host redirects followed while fetching.
	maxRedirects = 3
	// maxFetchAttempts is the maximum number of attempts for cold-start fetches.
	maxFetchAttempts = 3
)

var (
	retryBackoff  = 300 * time.Millisecond //nolint:gochecknoglobals // modified in tests
	tokenLogsMu   sync.Mutex               //nolint:gochecknoglobals
	lastTokenLogs []string                 //nolint:gochecknoglobals

	// bundleRe matches the script tag that names the web player bundle.
	bundleRe = regexp.MustCompile(`<script[^>]+src="(/assets/index[^"]*\.js)"`) //nolint:gochecknoglobals
	// tokenRe matches every JWT-shaped string in a bundle. The bundle is one
	// huge line, so it is searched as bytes rather than line by line.
	tokenRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{40,}\.[A-Za-z0-9_-]{40,}`) //nolint:gochecknoglobals
)

func logToken(msg string) {
	tokenLogsMu.Lock()
	defer tokenLogsMu.Unlock()
	lastTokenLogs = append(lastTokenLogs, msg)
}

func resetTokenLogs() {
	tokenLogsMu.Lock()
	defer tokenLogsMu.Unlock()
	lastTokenLogs = nil
}

// TokenLogs returns a copy of the log messages produced during the most
// recent developer token resolution.
func TokenLogs() []string {
	tokenLogsMu.Lock()
	defer tokenLogsMu.Unlock()
	out := make([]string, len(lastTokenLogs))
	copy(out, lastTokenLogs)
	return out
}

// ResolveDeveloperToken sets cfg.AppleDeveloperToken from the web player token,
// or from the cache left by an earlier run. The result takes priority over any
// value in the config file.
//
// A fetch error is returned only when no usable token was found: a fetch that
// failed but left a still-valid cached token succeeds. On error the configured
// token is left untouched, so the caller can carry on with it.
func ResolveDeveloperToken(cfg *config.Config) error {
	resetTokenLogs()
	tok, err := webPlayerToken(webPlayerURL, cachePath(), time.Now())
	if err != nil {
		if cfg.AppleDeveloperToken != "" {
			logToken(fmt.Sprintf("[devtoken] falling back to configured apple_developer_token after fetch error: %v", err))
		}
		return err
	}
	cfg.AppleDeveloperToken = tok
	return nil
}

// RefreshDeveloperToken fetches a fresh web player token, ignoring the cache,
// stores it in the cache and sets cfg.AppleDeveloperToken.
func RefreshDeveloperToken(cfg *config.Config) error {
	now := time.Now()
	tok, err := fetchTokenWithRetry(webPlayerURL, now)
	if err != nil {
		return err
	}
	validity := time.Unix(tok.ExpiresAt, 0).Sub(now).Round(time.Minute)
	logToken(fmt.Sprintf("[devtoken] fresh developer token received (valid for %v)", validity))
	_ = writeCache(cachePath(), tok)
	cfg.AppleDeveloperToken = tok.Token
	return nil
}

// CheckTokensRefreshing asks Apple Music which credential still works, and
// when the developer token is the one being rejected, fetches a fresh web
// player token once and asks again.
func CheckTokensRefreshing(cfg *config.Config) (TokenStatus, string) {
	return checkTokensRefreshing(CheckTokens, func() error { return RefreshDeveloperToken(cfg) }, cfg)
}

// checkTokensRefreshing is the testable core of CheckTokensRefreshing: the
// check and the refresh are parameters so tests can drive both outcomes.
func checkTokensRefreshing(check func(dev, user string) (TokenStatus, string), refresh func() error, cfg *config.Config) (TokenStatus, string) {
	status, sf := check(cfg.AppleDeveloperToken, cfg.AppleUserToken)
	if status != DeveloperTokenRejected {
		return status, sf
	}
	logToken("[devtoken] developer token rejected, fetching a fresh one")
	if err := refresh(); err != nil {
		logToken(fmt.Sprintf("[devtoken] refresh after rejection failed: %v", err))
		return DeveloperTokenRejected, ""
	}
	return check(cfg.AppleDeveloperToken, cfg.AppleUserToken)
}

type cachedToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func (c cachedToken) validFor(now time.Time, d time.Duration) bool {
	return isValidJWT(c.Token) && time.Unix(c.ExpiresAt, 0).After(now.Add(d))
}

// isValidJWT checks that tok has three non-empty base64url segments.
// Because the developer token is rendered into musickit.html via text/template,
// restricting it to the base64url alphabet prevents template/JS injection.
func isValidJWT(tok string) bool {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i := 0; i < len(p); i++ {
			c := p[i]
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

type nonRetryableError struct{ error }

func (e nonRetryableError) Unwrap() error { return e.error }

func isPermanentError(err error) bool {
	var nonRetryable nonRetryableError
	return errors.As(err, &nonRetryable)
}

// webPlayerToken returns a developer token from the on-disk cache when it has
// time left, otherwise from the web player. If the fetch fails it falls back
// to a cached token that has not yet expired.
func webPlayerToken(base, cache string, now time.Time) (string, error) {
	cached, _ := readCache(cache)
	if cached.validFor(now, refreshBefore) {
		remaining := time.Unix(cached.ExpiresAt, 0).Sub(now).Round(time.Minute)
		logToken(fmt.Sprintf("[devtoken] using cached developer token (valid for %v)", remaining))
		return cached.Token, nil
	}

	hasFallback := cached.validFor(now, 0)
	if hasFallback {
		remaining := time.Unix(cached.ExpiresAt, 0).Sub(now).Round(time.Minute)
		logToken(fmt.Sprintf("[devtoken] cached token expiring soon (%v remaining), fetching fresh token from %s", remaining, base))
	} else {
		logToken(fmt.Sprintf("[devtoken] fetching fresh developer token from %s", base))
	}

	var (
		fetched cachedToken
		err     error
	)
	if hasFallback {
		// When a working cached token is available, make a single fast attempt
		// rather than blocking startup across multiple retries and backoffs.
		fetched, err = fetchWebPlayerToken(base, now, refreshTimeout)
		if err != nil {
			logToken(fmt.Sprintf("[devtoken] refresh attempt failed: %v", err))
		}
	} else {
		fetched, err = fetchTokenWithRetry(base, now)
	}
	if err == nil {
		validity := time.Unix(fetched.ExpiresAt, 0).Sub(now).Round(time.Minute)
		logToken(fmt.Sprintf("[devtoken] fresh developer token received (valid for %v)", validity))
		_ = writeCache(cache, fetched)
		return fetched.Token, nil
	}

	if hasFallback {
		remaining := time.Unix(cached.ExpiresAt, 0).Sub(now).Round(time.Minute)
		logToken(fmt.Sprintf("[devtoken] fetch failed, falling back to unexpired cached token (valid for %v)", remaining))
		return cached.Token, nil
	}

	logToken(fmt.Sprintf("[devtoken] all fetch attempts failed and no valid cache available: %v", err))
	return "", fmt.Errorf("fetching developer token from the web player: %w", err)
}

func fetchTokenWithRetry(base string, now time.Time) (cachedToken, error) {
	var lastErr error
	backoff := retryBackoff

	for attempt := 1; attempt <= maxFetchAttempts; attempt++ {
		tok, err := fetchToken(base, now)
		if err == nil {
			return tok, nil
		}
		lastErr = err
		logToken(fmt.Sprintf("[devtoken] attempt %d/%d failed: %v", attempt, maxFetchAttempts, err))

		if isPermanentError(err) || attempt == maxFetchAttempts {
			break
		}

		logToken(fmt.Sprintf("[devtoken] retrying in %v...", backoff))
		time.Sleep(backoff)
		backoff *= 2
	}
	return cachedToken{}, lastErr
}

// fetchToken reads a token from the web player using the default cold-start
// timeout.
func fetchToken(base string, now time.Time) (cachedToken, error) {
	return fetchWebPlayerToken(base, now, fetchTimeout)
}

// fetchWebPlayerToken reads the web player page, follows it to the script
// bundle and returns the WebPlayKid token found there. The base must be HTTPS,
// or HTTP on a loopback host so tests can use httptest. Redirects are followed
// only within the same host, so the page cannot send AUX somewhere else.
func fetchWebPlayerToken(base string, now time.Time, timeout time.Duration) (cachedToken, error) {
	u, err := url.Parse(base)
	if err != nil {
		return cachedToken{}, nonRetryableError{fmt.Errorf("web player url: %w", err)}
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname())) {
		return cachedToken{}, nonRetryableError{errors.New("web player url must be https")}
	}
	origin := strings.TrimSuffix(base, "/")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{
		Timeout:       timeout,
		CheckRedirect: sameHostRedirect(u),
	}

	page, err := get(ctx, client, origin+webPlayerPage, maxHTMLBytes)
	if err != nil {
		return cachedToken{}, err
	}
	m := bundleRe.FindSubmatch(page)
	if m == nil {
		return cachedToken{}, nonRetryableError{fmt.Errorf("no script bundle in %s%s", origin, webPlayerPage)}
	}
	path := string(m[1])
	if !strings.HasPrefix(path, "/assets/") {
		return cachedToken{}, nonRetryableError{fmt.Errorf("refusing to fetch bundle from %q", path)}
	}
	bundle, err := get(ctx, client, origin+path, maxBundleBytes)
	if err != nil {
		return cachedToken{}, err
	}

	tok, exp, err := webPlayTokenIn(bundle, now)
	if err != nil {
		return cachedToken{}, nonRetryableError{fmt.Errorf("%s%s: %w", origin, path, err)}
	}
	return cachedToken{Token: tok, ExpiresAt: exp}, nil
}

// sameHostRedirect keeps the fetch on the web player's own host.
func sameHostRedirect(base *url.URL) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return nonRetryableError{fmt.Errorf("stopped after %d redirects", maxRedirects)}
		}
		if req.URL.Host != base.Host {
			return nonRetryableError{fmt.Errorf("refusing redirect to %s", req.URL.Host)}
		}
		return nil
	}
}

// get fetches one web player resource with the headers the site expects and
// returns at most limit bytes of it.
func get(ctx context.Context, client *http.Client, target string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webPlayerUserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, nonRetryableError{fmt.Errorf("%s returned %s", target, resp.Status)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", target, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", target, err)
	}
	if int64(len(body)) > limit {
		return nil, nonRetryableError{fmt.Errorf("%s is larger than %d bytes", target, limit)}
	}
	return body, nil
}

// webPlayTokenIn returns the WebPlayKid token in a script bundle and its
// expiry. The bundle carries several JWTs, so the kid is read from each
// token's header rather than trusting the order or the encoded header string.
func webPlayTokenIn(bundle []byte, now time.Time) (string, int64, error) {
	seen := 0
	for _, m := range tokenRe.FindAll(bundle, -1) {
		seen++
		tok := string(m)
		kid, ok := jwtKid(tok)
		if !ok || kid != webPlayKid {
			continue
		}
		exp, ok := jwtExp(tok)
		if !ok {
			return "", 0, errors.New("WebPlayKid token has no exp")
		}
		if !isValidJWT(tok) {
			return "", 0, errors.New("WebPlayKid token is malformed")
		}
		if !time.Unix(exp, 0).After(now) {
			return "", 0, errors.New("WebPlayKid token has expired")
		}
		return tok, exp, nil
	}
	return "", 0, fmt.Errorf("no WebPlayKid token among %d JWTs", seen)
}

// jwtClaims decodes one JWT segment into its JSON claims.
func jwtClaims(tok string, segment int) (map[string]any, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[segment])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}

// jwtKid returns the kid of a JWT header.
func jwtKid(tok string) (string, bool) {
	claims, ok := jwtClaims(tok, 0)
	if !ok {
		return "", false
	}
	kid, ok := claims["kid"].(string)
	return kid, ok
}

// jwtExp returns the exp claim of a JWT payload, in unix seconds.
func jwtExp(tok string) (int64, bool) {
	claims, ok := jwtClaims(tok, 1)
	if !ok {
		return 0, false
	}
	exp, ok := claims["exp"].(float64)
	return int64(exp), ok
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// cachePath is where the fetched token is kept between runs, or "" when the
// platform has no user cache directory.
func cachePath() string {
	d, err := os.UserCacheDir()
	if err != nil || d == "" {
		return ""
	}
	return filepath.Join(d, "aux", "devtoken.json")
}

func readCache(path string) (cachedToken, error) {
	var c cachedToken
	if path == "" {
		return c, errors.New("no cache dir")
	}
	b, err := os.ReadFile(path) //nolint:gosec // G304: fixed path under the user cache dir
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

func writeCache(path string, c cachedToken) error {
	if path == "" {
		return errors.New("no cache dir")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "devtoken-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
