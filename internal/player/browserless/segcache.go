//go:build linux || darwin || windows

package browserless

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// segCache stores encrypted HLS segments on disk so replays do not hit the
// Apple CDN again. Only ciphertext is ever written; decryption stays in
// memory. A nil *segCache is a valid disabled cache: get misses and put,
// drop and prune do nothing.
type segCache struct {
	dir string
	max int64
}

func newSegCache(dir string, maxBytes int64) *segCache {
	return &segCache{dir: dir, max: maxBytes}
}

// segKey identifies one encrypted segment by KID, media file name and byte
// range. Signed query parameters are stripped from the URI because they
// change between runs, while byte-range segments sharing one URI are told
// apart by offset and length.
func segKey(kid []byte, seg segmentRef) string {
	base := seg.uri
	if i := strings.IndexAny(base, "?#"); i != -1 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, "/"); i != -1 {
		base = base[i+1:]
	}
	preimage := strings.Join([]string{
		hex.EncodeToString(kid),
		base,
		strconv.FormatInt(seg.offset, 10),
		strconv.FormatInt(seg.length, 10),
	}, "|")
	sum := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(sum[:])
}

func (c *segCache) filePath(key string) string {
	return filepath.Join(c.dir, key+".seg")
}

// get returns the cached ciphertext for key, dropping entries that do not
// hold wantLen bytes (wantLen <= 0 skips the length check).
func (c *segCache) get(key string, wantLen int64) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	data, err := os.ReadFile(c.filePath(key))
	if err != nil {
		return nil, false
	}
	if len(data) == 0 || (wantLen > 0 && int64(len(data)) != wantLen) {
		c.drop(key)
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(c.filePath(key), now, now)
	return data, true
}

// put stores ciphertext atomically: a unique temp file per write is renamed
// into place, so concurrent writers and torn writes never surface a partial
// segment.
func (c *segCache) put(key string, data []byte) {
	if c == nil {
		return
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.dir, "*.seg.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(name)
		return
	}
	if err := os.Rename(name, c.filePath(key)); err != nil {
		_ = os.Remove(name)
	}
}

func (c *segCache) drop(key string) {
	if c == nil {
		return
	}
	_ = os.Remove(c.filePath(key))
}

// prune deletes the oldest .seg files by mtime until the total size drops
// to 90% of max, so the cache is not pruned again after every put.
func (c *segCache) prune() error {
	if c == nil {
		return nil
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var files []entry
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".seg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entry{path: filepath.Join(c.dir, e.Name()), size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	if total <= c.max {
		return nil
	}
	limit := c.max * 9 / 10
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= limit {
			break
		}
		if err := os.Remove(f.path); err == nil {
			total -= f.size
		}
	}
	return nil
}
