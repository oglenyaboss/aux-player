//go:build linux || darwin || windows

package browserless

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestSegKeyIgnoresSignedQueryAndCoversRangeKidAndBase(t *testing.T) {
	kid := []byte("0123456789abcdef")
	seg := segmentRef{uri: "https://cdn.example.com/media/audio.mp4?Expires=1&token=abc#frag", offset: 100, length: 50}

	sameBase := segmentRef{uri: "https://cdn.example.com/media/audio.mp4?Expires=999&token=zzz", offset: 100, length: 50}
	if segKey(kid, seg) != segKey(kid, sameBase) {
		t.Error("keys differ for URIs that differ only in query string and fragment")
	}

	diffOffset := seg
	diffOffset.offset = 101
	if segKey(kid, seg) == segKey(kid, diffOffset) {
		t.Error("keys match for different offsets")
	}

	diffLength := seg
	diffLength.length = 51
	if segKey(kid, seg) == segKey(kid, diffLength) {
		t.Error("keys match for different lengths")
	}

	diffKID := []byte("ffffffffffffffff")
	if segKey(kid, seg) == segKey(diffKID, seg) {
		t.Error("keys match for different KIDs")
	}

	diffBase := seg
	diffBase.uri = "https://cdn.example.com/media/other.mp4?Expires=1&token=abc"
	if segKey(kid, seg) == segKey(kid, diffBase) {
		t.Error("keys match for different basenames")
	}
}

func TestSegCachePutGetRoundTrip(t *testing.T) {
	c := newSegCache(filepath.Join(t.TempDir(), "segs"), 1<<20)
	key := segKey([]byte("0123456789abcdef"), segmentRef{uri: "seg.mp4", offset: 10, length: 4})
	data := []byte("abcd")

	c.put(key, data)
	got, ok := c.get(key, 4)
	if !ok || !bytes.Equal(got, data) {
		t.Errorf("get after put = (%q, %v), want (%q, true)", got, ok, data)
	}

	if _, ok := c.get(key, 5); ok {
		t.Error("get with wrong wantLen hit, want miss")
	}
	if _, err := os.Stat(c.filePath(key)); !os.IsNotExist(err) {
		t.Error("mismatched entry still on disk, want it dropped")
	}

	c.put(key, data)
	if err := os.WriteFile(c.filePath(key), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.get(key, 4); ok {
		t.Error("get on an empty file hit, want miss")
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	c.put(key, data)
	info, err := os.Stat(c.filePath(key))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSegCachePruneDeletesOldestDownTo90Percent(t *testing.T) {
	c := newSegCache(t.TempDir(), 2500)
	now := time.Now()
	for i, key := range []string{"oldest", "middle", "newest"} {
		c.put(key, make([]byte, 1000))
		stamp := now.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(c.filePath(key), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.prune(); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := os.Stat(c.filePath("oldest")); !os.IsNotExist(err) {
		t.Error("oldest entry survived prune")
	}
	for _, key := range []string{"middle", "newest"} {
		if _, err := os.Stat(c.filePath(key)); err != nil {
			t.Errorf("entry %q pruned, want kept: %v", key, err)
		}
	}
}

func TestNilSegCacheIsANoOp(t *testing.T) {
	var c *segCache
	if _, ok := c.get("k", 1); ok {
		t.Error("nil cache get hit, want miss")
	}
	c.put("k", []byte("x"))
	c.drop("k")
	if err := c.prune(); err != nil {
		t.Errorf("nil cache prune = %v, want nil", err)
	}
}

// segCacheFixture wires fetchAndDecryptSegment to a stub decrypt and an
// upstream that always returns the same ciphertext.
type segCacheFixture struct {
	s        *StreamServer
	upstream *httptest.Server
	hits     *atomic.Int32
	cache    *segCache
	cipher   []byte
	plain    []byte
	st       *trackStream
	seg      segmentRef
}

func newSegCacheFixture(t *testing.T) *segCacheFixture {
	t.Helper()

	cipher := []byte("ENCRYPTED-SEGMENT")
	plain := append([]byte("PLAINTEXT|"), cipher...)
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(cipher)
	}))
	t.Cleanup(upstream.Close)

	seg := segmentRef{uri: "media.mp4", offset: 4096, length: int64(len(cipher))}
	st := &trackStream{
		trackID:  "123",
		kid:      []byte("0123456789abcdef"),
		baseURI:  upstream.URL + "/",
		segments: []segmentRef{seg},
	}
	s := &StreamServer{
		authToken: "tok",
		segs:      newSegCache(filepath.Join(t.TempDir(), "segs"), 1<<20),
		decrypt: func(segData []byte, _ *trackStream) ([]byte, error) {
			if !bytes.Equal(segData, cipher) {
				return nil, errors.New("stub decrypt failed")
			}
			return append([]byte("PLAINTEXT|"), segData...), nil
		},
	}
	return &segCacheFixture{s: s, upstream: upstream, hits: &hits, cache: s.segs, cipher: cipher, plain: plain, st: st, seg: seg}
}

func (f *segCacheFixture) key() string { return segKey(f.st.kid, f.seg) }

func (f *segCacheFixture) fetch(t *testing.T) []byte {
	t.Helper()
	aac, stage, err := f.s.fetchAndDecryptSegment(context.Background(), f.upstream.Client(), f.st, f.seg)
	if err != nil {
		t.Fatalf("fetchAndDecryptSegment: stage=%q err=%v", stage, err)
	}
	if !bytes.Equal(aac, f.plain) {
		t.Fatalf("decrypted bytes = %q, want %q", aac, f.plain)
	}
	return aac
}

func TestFetchAndDecryptSegmentServesSecondCallFromDiskCache(t *testing.T) {
	f := newSegCacheFixture(t)

	f.fetch(t)
	if got := f.hits.Load(); got != 1 {
		t.Fatalf("upstream hits after first call = %d, want 1", got)
	}
	if _, err := os.Stat(f.cache.filePath(f.key())); err != nil {
		t.Fatalf("no cached file after first call: %v", err)
	}

	f.fetch(t)
	if got := f.hits.Load(); got != 1 {
		t.Errorf("upstream hits after second call = %d, want 1 (served from disk)", got)
	}
}

func TestFetchAndDecryptSegmentWithoutCacheFetchesEveryTime(t *testing.T) {
	f := newSegCacheFixture(t)
	f.s.segs = nil

	f.fetch(t)
	f.fetch(t)
	if got := f.hits.Load(); got != 2 {
		t.Errorf("upstream hits = %d, want 2 without a cache", got)
	}
}

func TestFetchAndDecryptSegmentRecoversFromCorruptCacheEntry(t *testing.T) {
	f := newSegCacheFixture(t)
	f.fetch(t)

	key := f.key()
	if err := os.WriteFile(f.cache.filePath(key), bytes.Repeat([]byte{0xAA}, len(f.cipher)), 0o600); err != nil {
		t.Fatal(err)
	}

	f.fetch(t)
	if got := f.hits.Load(); got != 2 {
		t.Errorf("upstream hits = %d, want 2 after refetching a corrupt entry", got)
	}
	disk, err := os.ReadFile(f.cache.filePath(key))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, f.cipher) {
		t.Errorf("cached entry = %q, want the fresh upstream ciphertext %q", disk, f.cipher)
	}
}

func TestDiskCacheHoldsOnlyCiphertext(t *testing.T) {
	f := newSegCacheFixture(t)
	f.fetch(t)

	disk, err := os.ReadFile(f.cache.filePath(f.key()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk, f.cipher) {
		t.Errorf("cached entry = %q, want the upstream ciphertext %q", disk, f.cipher)
	}
	if bytes.Contains(disk, []byte("PLAINTEXT|")) {
		t.Error("cached entry contains decrypted output")
	}
}
