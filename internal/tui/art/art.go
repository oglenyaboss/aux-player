// Package art fetches and decodes album artwork for the scene.
package art

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

const (
	MaxArtworkWidth  = 4096
	MaxArtworkHeight = 4096
	MaxArtworkPixels = 16_000_000
)

// SupportsColor reports whether the terminal has enough colours for album art
// to look reasonable (at least 256). Colours are emitted through lipgloss, so
// truecolor output is down-sampled to the terminal's profile automatically;
// below 256 colours the result is unrecognisable and we skip the artwork (and
// its download) entirely.
func SupportsColor() bool {
	return colorprofile.Detect(os.Stdout, os.Environ()) >= colorprofile.ANSI256
}

func Decode(r io.Reader) (image.Image, error) {
	if r == nil {
		return nil, fmt.Errorf("decode artwork: nil reader")
	}
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func FetchAndDecode(ctx context.Context, client *http.Client, url string, maxBytes int64) (image.Image, error) {
	if ctx == nil {
		return nil, fmt.Errorf("fetch artwork: nil context")
	}
	if url == "" {
		return nil, fmt.Errorf("fetch artwork: empty url")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("fetch artwork: non-positive max bytes %d", maxBytes)
	}

	parsed, err := urlpkg.Parse(url)
	if err != nil {
		return nil, err
	}
	switch parsed.Scheme {
	case "http", "https":
	case "data":
		return decodeDataURL(url, maxBytes)
	case "":
		return nil, fmt.Errorf("fetch artwork: missing URL scheme")
	default:
		return nil, fmt.Errorf("fetch artwork: unsupported URL scheme %q", parsed.Scheme)
	}

	if client == nil {
		return nil, fmt.Errorf("fetch artwork: nil http client")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // G107: URL scheme is explicitly limited to http/https above before fetching artwork
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Response body close errors are not actionable after the body is read.
			return
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch artwork: status %d", resp.StatusCode)
	}
	return decodeBounded(resp.Body, maxBytes)
}

func decodeDataURL(raw string, maxBytes int64) (image.Image, error) {
	meta, payload, ok := strings.Cut(raw, ",")
	if !ok {
		return nil, fmt.Errorf("decode artwork: invalid data URL")
	}
	mediaType := strings.ToLower(strings.TrimPrefix(meta, "data:"))
	if !strings.HasPrefix(mediaType, "image/png;") && !strings.HasPrefix(mediaType, "image/jpeg;") {
		return nil, fmt.Errorf("decode artwork: unsupported data media type %q", mediaType)
	}
	if !strings.Contains(mediaType, ";base64") {
		return nil, fmt.Errorf("decode artwork: unsupported data encoding")
	}
	return decodeBounded(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)), maxBytes)
}

func decodeBounded(r io.Reader, maxBytes int64) (image.Image, error) {
	if r == nil {
		return nil, fmt.Errorf("decode artwork: nil reader")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("decode artwork: non-positive max bytes %d", maxBytes)
	}
	buf, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > maxBytes {
		return nil, fmt.Errorf("decode artwork: exceeds max bytes %d", maxBytes)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("decode artwork: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width > MaxArtworkWidth || cfg.Height > MaxArtworkHeight {
		return nil, fmt.Errorf("decode artwork: dimensions %dx%d exceed max %dx%d", cfg.Width, cfg.Height, MaxArtworkWidth, MaxArtworkHeight)
	}
	if cfg.Width > MaxArtworkPixels/cfg.Height {
		return nil, fmt.Errorf("decode artwork: pixels %dx%d exceed max %d", cfg.Width, cfg.Height, MaxArtworkPixels)
	}
	return Decode(bytes.NewReader(buf))
}
