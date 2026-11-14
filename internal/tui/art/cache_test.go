package art

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// png300 is a 300×300 PNG with a gradient, so a decode of it can be told apart
// from a decode of anything else.
func png300(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	for y := range 300 {
		for x := range 300 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 77, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// coverServer serves data on every path and counts the hits.
func coverServer(t *testing.T, data []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// A1: the second fetch of a URL reads the file, and a fresh cache over the same
// directory does too; the server sees the URL once.
func TestCacheFetchHitsDisk(t *testing.T) {
	srv, hits := coverServer(t, png300(t))
	dir := t.TempDir()
	url := srv.URL + "/a.jpg"
	for range 2 {
		img, err := NewCache(dir, 1<<20).Fetch(t.Context(), srv.Client(), url, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 300 || img.Bounds().Dy() != 300 {
			t.Fatalf("decoded bounds = %v, want 300×300", img.Bounds())
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
	if _, err := NewCache(dir, 1<<20).Fetch(t.Context(), srv.Client(), url, 1<<20); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits after a fresh cache = %d, want 1", got)
	}
}

// A2: a cached file that no longer decodes is dropped and fetched again.
func TestCacheFetchDropsBrokenFile(t *testing.T) {
	srv, hits := coverServer(t, png300(t))
	dir := t.TempDir()
	url := srv.URL + "/a.jpg"
	if _, err := NewCache(dir, 1<<20).Fetch(t.Context(), srv.Client(), url, 1<<20); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.img"))
	if err != nil || len(files) != 1 {
		t.Fatalf("cache files = %v (%v), want one .img", files, err)
	}
	if err := os.WriteFile(files[0], []byte("not an image anymore"), 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := NewCache(dir, 1<<20).Fetch(t.Context(), srv.Client(), url, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 300 || img.Bounds().Dy() != 300 {
		t.Fatalf("decoded bounds = %v, want 300×300", img.Bounds())
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("server hits = %d, want 2", got)
	}
}

// A3: a body over the caller's byte cap is an error and writes nothing.
func TestCacheFetchRejectsOversize(t *testing.T) {
	srv, hits := coverServer(t, bytes.Repeat([]byte("x"), 2048))
	dir := t.TempDir()
	if _, err := NewCache(dir, 1<<20).Fetch(t.Context(), srv.Client(), srv.URL+"/big.jpg", 1024); err == nil {
		t.Fatal("Fetch over the byte cap = nil error, want error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache dir holds %d files after the rejected fetch, want none", len(entries))
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1", got)
	}
}

// A4: past the cap the oldest files leave until the total fits 90% of it; within
// the cap nothing is deleted.
func TestCachePrune(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-3 * time.Hour)
	for i, name := range []string{"a.img", "b.img", "c.img"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1000), 0o600); err != nil {
			t.Fatal(err)
		}
		at := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	c := NewCache(dir, 2500)
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.img")); !os.IsNotExist(err) {
		t.Fatalf("the oldest file survived: %v", err)
	}
	for _, name := range []string{"b.img", "c.img"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s did not survive the prune: %v", name, err)
		}
	}
	if err := c.Prune(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.img", "c.img"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("a prune within the cap deleted %s: %v", name, err)
		}
	}
}

// A5: a data: URL decodes without a cache behind it and writes nothing.
func TestCacheFetchDataURLUncached(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(1, 1, color.NRGBA{R: 200, A: 255})
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	var none *Cache
	decoded, err := none.Fetch(t.Context(), nil, url, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 4 || decoded.Bounds().Dy() != 4 {
		t.Fatalf("decoded bounds = %v, want 4×4", decoded.Bounds())
	}
	dir := t.TempDir()
	if _, err := NewCache(dir, 1<<20).Fetch(context.Background(), nil, url, 1<<20); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a data: URL wrote %d files, want none", len(entries))
	}
}
