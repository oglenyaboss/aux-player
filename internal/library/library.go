// Package library keeps the user's Apple Music library on disk and serves it
// without a refetch, deciding per entry when Apple must be asked again.
package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/oglenyaboss/aux-player/internal/provider"
)

// Kind identifies one piece of the library served by the store.
type Kind string

const (
	Songs     Kind = "songs"     // the whole library, Favorites flagged; id ""
	Albums    Kind = "albums"    // library albums; id ""
	Playlists Kind = "playlists" // library playlists without the Favorites entry; id ""
	Album     Kind = "album"     // tracks of one album; id "l.…" (library) or catalog numeric
	Playlist  Kind = "playlist"  // tracks of one playlist; id "p.…" (library) or "pl.…" (catalog)
)

// Result is the data the store serves for one entry.
type Result struct {
	Songs     []provider.Track
	Albums    []provider.Album
	Playlists []provider.Playlist
	Fetched   time.Time // when Apple last answered for this entry
	Due       bool      // a Refresh should run; the data may be old
}

const (
	fileVersion      = 1
	countProbeWindow = 6 * time.Hour
	recheckInterval  = 15 * time.Minute
	catalogAlbumTTL  = 30 * 24 * time.Hour
	hashLen          = 16
)

// counter is the optional provider capability the store type-asserts.
type counter interface {
	LibrarySongCount(ctx context.Context) (int, error)
}

// entry is the JSON body of every file the store writes.
type entry struct {
	Version   int                 `json:"v"`
	Fetched   time.Time           `json:"fetched"`
	Checked   time.Time           `json:"checked"`
	Songs     []provider.Track    `json:"songs,omitempty"`
	Albums    []provider.Album    `json:"albums,omitempty"`
	Playlists []provider.Playlist `json:"playlists,omitempty"`
}

// flight is one in-progress refresh that concurrent callers share.
type flight struct {
	done    chan struct{}
	r       Result
	changed bool
	err     error
}

// Store caches library data on disk, one file per kind and id, scoped to the
// user token so accounts never see each other's libraries.
type Store struct {
	dir     string
	owner   string
	prov    provider.Provider
	counter counter
	now     func() time.Time

	mu      sync.Mutex
	mem     map[string]*entry
	checked map[string]bool
	flights map[string]*flight
}

// Open returns a store rooted at dir for the user behind userToken, deleting
// the on-disk data of other users found under dir.
func Open(dir string, prov provider.Provider, userToken string) *Store {
	s := &Store{
		dir:     dir,
		owner:   hashPrefix(userToken),
		prov:    prov,
		now:     time.Now,
		mem:     make(map[string]*entry),
		checked: make(map[string]bool),
		flights: make(map[string]*flight),
	}
	if c, ok := prov.(counter); ok {
		s.counter = c
	}
	s.removeOtherOwners()
	return s
}

func (s *Store) removeOtherOwners() {
	if s.dir == "" {
		return
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == s.owner {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.dir, e.Name()))
	}
}

// hashPrefix returns the first hashLen hex digits of the sha256 of v.
func hashPrefix(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:hashLen/2])
}

func (s *Store) path(k Kind, id string) string {
	name := string(k)
	if k == Album || k == Playlist {
		name += "-" + hashPrefix(id)
	}
	return filepath.Join(s.dir, s.owner, name+".json")
}

func entryKey(k Kind, id string) string {
	return string(k) + "\x00" + id
}

func validate(k Kind, id string) error {
	switch k {
	case Songs, Albums, Playlists:
		return nil
	case Album, Playlist:
		if id == "" {
			return fmt.Errorf("library: kind %q requires an id", k)
		}
		return nil
	default:
		return fmt.Errorf("library: unknown kind %q", k)
	}
}

// Load returns the cached data for k, refetching it on a miss. With a cache it
// never touches the network; Due reports whether a Refresh is wanted.
func (s *Store) Load(ctx context.Context, k Kind, id string) (Result, error) {
	if err := validate(k, id); err != nil {
		return Result{}, err
	}
	key := entryKey(k, id)
	s.mu.Lock()
	e := s.mem[key]
	if e == nil {
		e = s.readDisk(k, id)
		if e != nil {
			s.mem[key] = e
		}
	}
	var due bool
	if e != nil {
		due = s.due(k, id, e)
	}
	s.mu.Unlock()
	if e != nil {
		r := result(k, e)
		r.Due = due
		return r, nil
	}
	r, _, err := s.Refresh(ctx, k, id, true)
	if err != nil {
		return Result{}, err
	}
	r.Due = false
	return r, nil
}

// Refresh refetches k from the provider and stores it, returning the fresh
// result and whether the data changed. Concurrent refreshes of the same entry
// share one fetch. On error the stored data stays untouched.
func (s *Store) Refresh(ctx context.Context, k Kind, id string, force bool) (r Result, changed bool, err error) {
	if err := validate(k, id); err != nil {
		return Result{}, false, err
	}
	key := entryKey(k, id)
	s.mu.Lock()
	if fl, ok := s.flights[key]; ok {
		s.mu.Unlock()
		<-fl.done
		return fl.r, fl.changed, fl.err
	}
	fl := &flight{done: make(chan struct{})}
	s.flights[key] = fl
	s.mu.Unlock()

	r, changed, err = s.refresh(ctx, k, id, key, force)
	fl.r, fl.changed, fl.err = r, changed, err
	close(fl.done)
	s.mu.Lock()
	delete(s.flights, key)
	s.mu.Unlock()
	return r, changed, err
}

// refresh does the real work of Refresh; the caller owns the flight.
func (s *Store) refresh(ctx context.Context, k Kind, id, key string, force bool) (Result, bool, error) {
	now := s.now()
	s.mu.Lock()
	stored := s.mem[key]
	if stored == nil {
		stored = s.readDisk(k, id)
	}
	s.mu.Unlock()

	if s.counter != nil && !force && k == Songs && stored != nil && now.Sub(stored.Fetched) < countProbeWindow {
		count, err := s.counter.LibrarySongCount(ctx)
		if err == nil && count != 0 && count == len(stored.Songs) {
			e := *stored
			e.Checked = now
			if err := s.writeEntry(k, id, &e); err != nil {
				return Result{}, false, err
			}
			s.mu.Lock()
			s.mem[key] = &e
			s.checked[key] = true
			s.mu.Unlock()
			return result(k, &e), false, nil
		}
	}

	var (
		songs     []provider.Track
		albums    []provider.Album
		playlists []provider.Playlist
		err       error
	)
	switch k {
	case Songs:
		songs, err = s.prov.GetLibraryTracks(ctx)
	case Albums:
		albums, err = s.prov.GetLibraryAlbums(ctx)
	case Playlists:
		var all []provider.Playlist
		all, err = s.prov.GetLibraryPlaylists(ctx)
		if err == nil {
			playlists = make([]provider.Playlist, 0, len(all))
			for _, p := range all {
				if p.ID != provider.FavoritesPlaylistID {
					playlists = append(playlists, p)
				}
			}
		}
	case Album:
		if strings.HasPrefix(id, "l.") {
			songs, err = s.prov.GetLibraryAlbumTracks(ctx, id)
		} else {
			songs, err = s.prov.GetAlbumTracks(ctx, id)
		}
	case Playlist:
		if strings.HasPrefix(id, "pl.") {
			songs, err = s.prov.GetCatalogPlaylistTracks(ctx, id)
		} else {
			songs, err = s.prov.GetPlaylistTracks(ctx, id)
		}
	}
	if err != nil {
		return Result{}, false, err
	}

	changed := false
	switch k {
	case Songs, Album, Playlist:
		changed = stored == nil || !reflect.DeepEqual(stored.Songs, songs)
	case Albums:
		changed = stored == nil || !reflect.DeepEqual(stored.Albums, albums)
	case Playlists:
		changed = stored == nil || !reflect.DeepEqual(stored.Playlists, playlists)
	}

	e := &entry{
		Version:   fileVersion,
		Fetched:   now,
		Checked:   now,
		Songs:     songs,
		Albums:    albums,
		Playlists: playlists,
	}
	if err := s.writeEntry(k, id, e); err != nil {
		return Result{}, false, err
	}
	s.mu.Lock()
	s.mem[key] = e
	s.checked[key] = true
	s.mu.Unlock()
	return result(k, e), changed, nil
}

// due reports whether the entry should be refreshed before it is served.
// Callers hold s.mu. Catalog albums are immutable, so they go stale only after
// catalogAlbumTTL; everything else is verified once per process and then at
// most every recheckInterval.
func (s *Store) due(k Kind, id string, e *entry) bool {
	now := s.now()
	if k == Album && !strings.HasPrefix(id, "l.") {
		return now.Sub(e.Fetched) > catalogAlbumTTL
	}
	key := entryKey(k, id)
	if !s.checked[key] {
		return true
	}
	return now.Sub(e.Checked) > recheckInterval
}

// NoteSaved folds a track AUX itself saved into the stored songs, so the next
// Load reflects it without a refetch. A stored song with the same ID or the same
// catalog song is the one saved. favorite additionally flags the track
// as being in Favorites. It returns the stored songs after the save; without a
// stored songs entry it does nothing and returns nil.
func (s *Store) NoteSaved(t provider.Track, favorite bool) []provider.Track {
	k, id := Songs, ""
	key := entryKey(k, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.mem[key]
	if e == nil {
		e = s.readDisk(k, id)
		if e == nil {
			return nil
		}
	}
	songs := make([]provider.Track, len(e.Songs))
	copy(songs, e.Songs)
	at := len(songs)
	for i := range songs {
		if songs[i].ID == t.ID || (songs[i].CatalogID != "" && songs[i].CatalogID == catalogID(t)) {
			at = i
			break
		}
	}
	if at == len(songs) {
		songs = append(songs, t)
	}
	if favorite {
		songs[at].Favorite = true
	}
	ne := *e
	ne.Songs = songs
	if err := s.writeEntry(k, id, &ne); err != nil {
		return nil
	}
	s.mem[key] = &ne
	return songs
}

// catalogID is the catalog song behind t: its CatalogID, or its ID when it came
// from the catalog itself.
func catalogID(t provider.Track) string {
	if t.CatalogID != "" {
		return t.CatalogID
	}
	return t.ID
}

// Forget drops the memory entry, the file and the checked mark of k.
func (s *Store) Forget(k Kind, id string) {
	if validate(k, id) != nil {
		return
	}
	key := entryKey(k, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mem, key)
	delete(s.checked, key)
	_ = os.Remove(s.path(k, id))
}

// Favorites returns the tracks flagged as Favorites, in their order.
func Favorites(songs []provider.Track) []provider.Track {
	var out []provider.Track
	for _, t := range songs {
		if t.Favorite {
			out = append(out, t)
		}
	}
	return out
}

func result(k Kind, e *entry) Result {
	r := Result{Fetched: e.Fetched}
	switch k {
	case Songs, Album, Playlist:
		r.Songs = e.Songs
	case Albums:
		r.Albums = e.Albums
	case Playlists:
		r.Playlists = e.Playlists
	}
	return r
}

// readDisk loads the entry file, treating a missing, undecodable or
// wrong-version file as a miss. Callers hold s.mu.
func (s *Store) readDisk(k Kind, id string) *entry {
	data, err := os.ReadFile(s.path(k, id)) //nolint:gosec // G304: the path is built from the store dir and hashed ids
	if err != nil {
		return nil
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil || e.Version != fileVersion {
		return nil
	}
	return &e
}

// writeEntry stores e atomically: a temp file renamed over the target, so a
// crash never leaves a half-written entry behind.
func (s *Store) writeEntry(k Kind, id string, e *entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.dir, s.owner), 0o700); err != nil {
		return err
	}
	path := s.path(k, id)
	tmp, err := os.CreateTemp(filepath.Dir(path), "*.json.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
