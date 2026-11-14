package art

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Cache is the on-disk artwork store: a fetched cover survives the run that
// downloaded it, one file per URL under dir.
type Cache struct {
	dir      string
	maxBytes int64
}

// NewCache builds the cache over dir; a cache with no directory set (a nil
// *Cache) fetches without storing.
func NewCache(dir string, maxBytes int64) *Cache {
	return &Cache{dir: dir, maxBytes: maxBytes}
}

// Fetch returns the artwork for url, serving it from dir when it holds it and
// downloading it through client otherwise. A nil cache and a data: URL go
// straight to FetchAndDecode, uncached.
func (c *Cache) Fetch(ctx context.Context, client *http.Client, url string, maxBytes int64) (image.Image, error) {
	if c == nil || strings.HasPrefix(url, "data:") {
		return FetchAndDecode(ctx, client, url, maxBytes)
	}
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
	case "":
		return nil, fmt.Errorf("fetch artwork: missing URL scheme")
	default:
		return nil, fmt.Errorf("fetch artwork: unsupported URL scheme %q", parsed.Scheme)
	}
	if client == nil {
		return nil, fmt.Errorf("fetch artwork: nil http client")
	}
	if img, ok := c.cached(url, maxBytes); ok {
		return img, nil
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
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > maxBytes {
		return nil, fmt.Errorf("fetch artwork: exceeds max bytes %d", maxBytes)
	}
	c.store(url, buf)
	return decodeBounded(bytes.NewReader(buf), maxBytes)
}

// cached reads the file behind url; a file that fails to decode is removed and
// reported as a miss.
func (c *Cache) cached(url string, maxBytes int64) (image.Image, bool) {
	path := c.path(url)
	buf, err := os.ReadFile(path) //nolint:gosec // G304: the name is the hex SHA-256 of the URL, written by this cache
	if err != nil {
		return nil, false
	}
	_ = os.Chtimes(path, time.Now(), time.Now())
	img, err := decodeBounded(bytes.NewReader(buf), maxBytes)
	if err != nil {
		_ = os.Remove(path)
		return nil, false
	}
	return img, true
}

// store lands buf under its URL's name through a temp file, so a reader never
// sees a half-written image; a write failure only costs the cache.
func (c *Cache) store(url string, buf []byte) {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, "")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, werr := tmp.Write(buf)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(name)
		return
	}
	if err := os.Rename(name, c.path(url)); err != nil {
		_ = os.Remove(name)
	}
}

func (c *Cache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".img")
}

// Prune deletes the oldest cached files by mtime until their total size fits
// 90% of the cap; a total within the cap deletes nothing.
func (c *Cache) Prune() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type file struct {
		name string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".img") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, file{e.Name(), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= c.maxBytes {
		return nil
	}
	limit := c.maxBytes * 9 / 10
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= limit {
			break
		}
		if err := os.Remove(filepath.Join(c.dir, f.name)); err != nil {
			continue
		}
		total -= f.size
	}
	return nil
}
