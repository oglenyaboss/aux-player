package vibeai

import (
	"fmt"
	"strings"
	"testing"
)

// A1: every taste rule is the model's call; the code only checks the facts.
func TestOfferAcceptsTasteCases(t *testing.T) {
	lib := []Song{{ID: "l", Artist: "L", Title: "Lib"}}
	favs := make([]Song, 0, 5)
	for i := 1; i <= 5; i++ {
		favs = append(favs, Song{ID: fmt.Sprintf("f%d", i), Artist: "F", Title: fmt.Sprintf("F%d", i)})
	}
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_early", Session: "past", Artist: "P", Title: "Past"})
	s := NewSession(lib, favs, 10)
	s.SetSignals(sig, "cur")
	for i := 1; i <= 5; i++ {
		song := Song{ID: fmt.Sprintf("f%d", i), Artist: "F", Title: fmt.Sprintf("F%d", i)}
		if v := s.Offer(Suggestion{Artist: "F", Title: song.Title}, song, true); !v.Accepted {
			t.Fatalf("F%d: rejected: %s", i, v.Reason)
		}
	}
	s.ClosePortion()
	for _, title := range []string{"T1", "T2"} {
		song := Song{ID: title, Artist: "T", Title: title}
		if v := s.Offer(Suggestion{Artist: "T", Title: title}, song, true); !v.Accepted {
			t.Fatalf("T %s: rejected: %s", title, v.Reason)
		}
	}
	s.ClosePortion()
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "N", Mode: "not_it"}}, Exclusions: []string{"Ex"}})
	for _, song := range []Song{
		{ID: "l2", Artist: "L", Title: "Lib"},
		{ID: "f6", Artist: "F", Title: "F6"},
		{ID: "t3", Artist: "T", Title: "T3"},
		{ID: "n1", Artist: "N", Title: "Song"},
		{ID: "e1", Artist: "Ex", Title: "Song"},
		{ID: "p1", Artist: "P", Title: "Past"},
	} {
		if v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true); !v.Accepted {
			t.Errorf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
}

// A2: Again replays a track served in an earlier portion, but never twice in one portion.
func TestOfferAlreadyServedAgain(t *testing.T) {
	s := NewSession(nil, nil, 5)
	if v := s.Offer(Suggestion{Artist: "A", Title: "T"}, Song{ID: "1", Artist: "A", Title: "T"}, true); !v.Accepted {
		t.Fatalf("first: rejected: %s", v.Reason)
	}
	s.ClosePortion()
	v := s.Offer(Suggestion{Artist: "A", Title: "T"}, Song{ID: "2", Artist: "A", Title: "T"}, true)
	if v.Accepted || v.Reason != "already served" {
		t.Errorf("repeat: got (%v, %q), want (false, «already served»)", v.Accepted, v.Reason)
	}
	if v := s.Offer(Suggestion{Artist: "A", Title: "T", Again: true}, Song{ID: "2", Artist: "A", Title: "T"}, true); !v.Accepted {
		t.Fatalf("again: rejected: %s", v.Reason)
	}
	w := s.Offer(Suggestion{Artist: "A", Title: "T", Again: true}, Song{ID: "3", Artist: "A", Title: "T"}, true)
	if w.Accepted || w.Reason != "already served" {
		t.Errorf("again in the same portion: got (%v, %q), want (false, «already served»)", w.Accepted, w.Reason)
	}
}

// A3: the two factual checks stay — existence and the portion.
func TestOfferFactsStay(t *testing.T) {
	s := NewSession(nil, nil, 1)
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	if v := s.Offer(Suggestion{Artist: "G", Title: "T"}, Song{}, false); v.Accepted || v.Reason != "not in Apple" {
		t.Errorf("not found: got (%v, %q), want (false, «not in Apple»)", v.Accepted, v.Reason)
	}
	v := s.Offer(Suggestion{Artist: "B", Title: "T2"}, Song{ID: "2", Artist: "B", Title: "T2"}, true)
	if v.Accepted || v.Reason != "portion is full" {
		t.Fatalf("full: got (%v, %q), want (false, «portion is full»)", v.Accepted, v.Reason)
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 1 || names[0] != "B — T2" {
		t.Fatalf("reserve: %v, want [B — T2]", names)
	}
}

func TestOfferNotFound(t *testing.T) {
	s := NewSession(nil, nil, 20)
	v := s.Offer(Suggestion{Artist: "A", Title: "T"}, Song{}, false)
	if v.Accepted || v.Reason != "not in Apple" || v.Song != (Song{}) {
		t.Errorf("got (%v, %q, %+v), want (false, \"not in Apple\", empty song)", v.Accepted, v.Reason, v.Song)
	}
}

func TestOfferPortionFull(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "Yeat", Title: "B"}, Song{ID: "2", Artist: "Yeat", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "portion is full" {
		t.Errorf("second: got (%v, %q), want (false, \"portion is full\")", v2.Accepted, v2.Reason)
	}
}

func TestOfferDuplicateAfterClose(t *testing.T) {
	lib := []Song{{ID: "l1", Artist: "F", Title: "T2"}}
	s := NewSession(lib, lib, 20)
	v := s.Offer(Suggestion{Artist: "F", Title: "T2", Why: "w"}, Song{ID: "1", Artist: "F", Title: "T2"}, true)
	if !v.Accepted {
		t.Fatalf("first: rejected: %s", v.Reason)
	}
	out := s.ClosePortion()
	if len(out) != 1 || out[0].Song.ID != "1" {
		t.Fatalf("ClosePortion: got %+v", out)
	}
	if s.Need() != 20 {
		t.Errorf("after close: Need = %d, want 20", s.Need())
	}
	if len(s.Portion()) != 0 {
		t.Errorf("after close: portion not empty")
	}
	v2 := s.Offer(Suggestion{Artist: "F", Title: "T2"}, Song{ID: "2", Artist: "F", Title: "T2"}, true)
	if v2.Accepted || v2.Reason != "already served" {
		t.Errorf("repeat: got (%v, %q), want (false, \"already served\")", v2.Accepted, v2.Reason)
	}
}

func TestOfferDuplicateInPortion(t *testing.T) {
	s := NewSession(nil, nil, 20)
	first := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !first.Accepted {
		t.Fatalf("first: rejected: %s", first.Reason)
	}
	v := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "2", Artist: "Yeat", Title: "A"}, true)
	if v.Accepted || v.Reason != "already served" {
		t.Errorf("dup in portion: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
}

func TestOfferLibraryAndFavorite(t *testing.T) {
	lib := []Song{{ID: "1", Artist: "L", Title: "T1"}}
	favs := []Song{{ID: "2", Artist: "F", Title: "T2"}}
	s := NewSession(lib, favs, 20)
	v1 := s.Offer(Suggestion{Artist: "L", Title: "T1"}, lib[0], true)
	if !v1.Accepted {
		t.Fatalf("library track: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "F", Title: "T2", Why: "потому что"}, favs[0], true)
	if !v2.Accepted {
		t.Fatalf("favorite track: rejected: %s", v2.Reason)
	}
	e := s.Portion()[1]
	if !e.Favorite || e.Why != "потому что" {
		t.Errorf("entry: got %+v, want Favorite=true with Why set", e)
	}
}

func TestTakeBypassesRules(t *testing.T) {
	lib := []Song{{ID: "l1", Artist: "L", Title: "TL"}}
	s := NewSession(lib, nil, 5)
	s.SetPlan(Plan{Exclusions: []string{"Xeno"}})
	l := Song{ID: "n1", Artist: "L", Title: "TL"}
	v := s.Take(Suggestion{Artist: "L", Title: "TL", Why: "w"}, l)
	if !v.Accepted || v.Reason != "" || v.Song != l {
		t.Errorf("take L: got (%v, %q, %+v), want (true, \"\", %+v)", v.Accepted, v.Reason, v.Song, l)
	}
	x := Song{ID: "n2", Artist: "Xeno", Title: "TX"}
	if v := s.Take(Suggestion{Artist: "Xeno", Title: "TX", Why: "w"}, x); !v.Accepted {
		t.Errorf("take Xeno: rejected: %s", v.Reason)
	}
	part := s.Portion()
	if len(part) != 2 || part[0].Why != "w" || part[0].Song.ID != "n1" {
		t.Errorf("portion: %+v, want two entries with Why=\"w\"", part)
	}
	if v := s.Offer(Suggestion{Artist: "L", Title: "TL"}, Song{ID: "n3", Artist: "L", Title: "TL"}, true); v.Accepted || v.Reason != "already served" {
		t.Errorf("offer L: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
}

func TestTakeFullPortion(t *testing.T) {
	s := NewSession(nil, nil, 2)
	for i, id := range []string{"1", "2"} {
		if v := s.Take(Suggestion{Artist: "A" + id, Title: "T"}, Song{ID: id, Artist: "A" + id, Title: "T"}); !v.Accepted {
			t.Fatalf("take %d: rejected: %s", i, v.Reason)
		}
	}
	v := s.Take(Suggestion{Artist: "A3", Title: "T"}, Song{ID: "3", Artist: "A3", Title: "T"})
	if v.Accepted || v.Reason != "portion is full" {
		t.Errorf("take 3: got (%v, %q), want (false, \"portion is full\")", v.Accepted, v.Reason)
	}
	if n := len(s.Portion()); n != 2 {
		t.Errorf("portion = %d, want 2", n)
	}
}

func TestTakeAlreadyGiven(t *testing.T) {
	s := NewSession(nil, nil, 5)
	if v := s.Offer(Suggestion{Artist: "P", Title: "TP"}, Song{ID: "1", Artist: "P", Title: "TP"}, true); !v.Accepted {
		t.Fatalf("offer: rejected: %s", v.Reason)
	}
	s.ClosePortion()
	v := s.Take(Suggestion{Artist: "P", Title: "TP"}, Song{ID: "2", Artist: "P", Title: "TP"})
	if v.Accepted || v.Reason != "already served" {
		t.Errorf("take given: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
	if v := s.Take(Suggestion{Artist: "Q", Title: "TQ"}, Song{ID: "3", Artist: "Q", Title: "TQ"}); !v.Accepted {
		t.Fatalf("take Q: rejected: %s", v.Reason)
	}
	if v := s.Take(Suggestion{Artist: "Q", Title: "TQ"}, Song{ID: "4", Artist: "Q", Title: "TQ"}); v.Accepted || v.Reason != "already served" {
		t.Errorf("take twice: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
}

func TestResetPortion(t *testing.T) {
	s := NewSession(nil, nil, 5)
	if v := s.Offer(Suggestion{Artist: "S", Title: "TS"}, Song{ID: "1", Artist: "S", Title: "TS"}, true); !v.Accepted {
		t.Fatalf("first: rejected: %s", v.Reason)
	}
	s.ResetPortion()
	if got := s.Portion(); len(got) != 0 {
		t.Fatalf("portion after reset: %+v, want empty", got)
	}
	if got := s.Need(); got != 5 {
		t.Fatalf("Need after reset = %d, want 5", got)
	}
	if v := s.Offer(Suggestion{Artist: "S", Title: "TS"}, Song{ID: "1", Artist: "S", Title: "TS"}, true); !v.Accepted {
		t.Errorf("second: rejected: %s", v.Reason)
	}
}

func TestDropLast(t *testing.T) {
	s := NewSession(nil, nil, 5)
	if v := s.Offer(Suggestion{Artist: "P1", Title: "T1"}, Song{ID: "1", Artist: "P1", Title: "T1"}, true); !v.Accepted {
		t.Fatalf("P1: rejected: %s", v.Reason)
	}
	s.ClosePortion()
	if v := s.Offer(Suggestion{Artist: "P2", Title: "T2"}, Song{ID: "2", Artist: "P2", Title: "T2"}, true); !v.Accepted {
		t.Fatalf("P2: rejected: %s", v.Reason)
	}
	s.ClosePortion()
	s.DropLast()
	if v := s.Offer(Suggestion{Artist: "P2", Title: "T2"}, Song{ID: "3", Artist: "P2", Title: "T2"}, true); !v.Accepted {
		t.Errorf("P2: rejected: %s", v.Reason)
	}
	if v := s.Offer(Suggestion{Artist: "P1", Title: "T1"}, Song{ID: "4", Artist: "P1", Title: "T1"}, true); v.Accepted || v.Reason != "already served" {
		t.Errorf("P1: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
	s.DropLast()
	if v := s.Offer(Suggestion{Artist: "P1", Title: "T1"}, Song{ID: "5", Artist: "P1", Title: "T1"}, true); v.Accepted || v.Reason != "already served" {
		t.Errorf("second drop: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
}

func TestReserveOverflowNames(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "Don Toliver", Title: "B"}, Song{ID: "2", Artist: "Don Toliver", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "portion is full" {
		t.Fatalf("second: got (%v, %q), want (false, \"portion is full\")", v2.Accepted, v2.Reason)
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 1 || names[0] != "Don Toliver — B" {
		t.Fatalf("reserve: %v, want [Don Toliver — B]", names)
	}
	if got := s.Portion(); len(got) != 0 {
		t.Errorf("portion after the reserve: %+v, want empty", got)
	}
}

func TestReserveSkipsNotFoundAndOtherRejects(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{}, false)
	if v2.Accepted || v2.Reason != "not in Apple" {
		t.Fatalf("not found: got (%v, %q), want (false, \"not in Apple\")", v2.Accepted, v2.Reason)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Fatalf("reserve: %d names, want 0: %v", len(names), names)
	}
	if got := s.Portion(); len(got) != 0 {
		t.Errorf("portion after the reserve: %+v, want empty", got)
	}

	w := NewSession(nil, nil, 2)
	if v := w.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	if v := w.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1x", Artist: "A", Title: "T1"}, true); v.Accepted || v.Reason != "already served" {
		t.Fatalf("A again: got (%v, %q), want (false, \"already served\")", v.Accepted, v.Reason)
	}
	if v := w.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{}, false); v.Reason != "not in Apple" {
		t.Fatalf("C: got %q", v.Reason)
	}
	if v := w.Offer(Suggestion{Artist: "B", Title: "T2"}, Song{ID: "2", Artist: "B", Title: "T2"}, true); !v.Accepted {
		t.Fatalf("B: rejected: %s", v.Reason)
	}
	w.ClosePortion()
	if names := w.TakeReserve(); len(names) != 0 {
		t.Errorf("reserve (already served and not in Apple): %d names, want 0: %v", len(names), names)
	}
}

func TestReserveDedupAndCap(t *testing.T) {
	s := NewSession(nil, nil, 2)
	for i, want := range []string{"A", "B"} {
		v := s.Offer(Suggestion{Artist: want, Title: "T"}, Song{ID: fmt.Sprintf("%d", i), Artist: want, Title: "T"}, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", want, v.Reason)
		}
	}
	for i := range 2 {
		v := s.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{ID: "c", Artist: "C", Title: "T3"}, true)
		if v.Accepted || v.Reason != "portion is full" {
			t.Fatalf("C #%d: got (%v, %q)", i, v.Accepted, v.Reason)
		}
	}
	for i, want := range []string{"D", "E"} {
		v := s.Offer(Suggestion{Artist: want, Title: fmt.Sprintf("T%d", 4+i)}, Song{ID: want, Artist: want, Title: fmt.Sprintf("T%d", 4+i)}, true)
		if v.Accepted || v.Reason != "portion is full" {
			t.Fatalf("%s: got (%v, %q)", want, v.Accepted, v.Reason)
		}
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 2 {
		t.Fatalf("reserve: %d names, want 2 (C, D, no duplicate C and no E): %v", len(names), names)
	}
	for i, want := range []string{"C — T3", "D — T4"} {
		if names[i] != want {
			t.Errorf("reserve[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestReserveCapAtSize(t *testing.T) {
	s := NewSession(nil, nil, 1)
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	for i, want := range []Song{{ID: "2", Artist: "B", Title: "T2"}, {ID: "3", Artist: "C", Title: "T3"}} {
		v := s.Offer(Suggestion{Artist: want.Artist, Title: want.Title}, want, true)
		if v.Accepted || v.Reason != "portion is full" {
			t.Fatalf("%d: got (%v, %q)", i, v.Accepted, v.Reason)
		}
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 1 || names[0] != "B — T2" {
		t.Fatalf("reserve: %v, want [B — T2]", names)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("second reserve: %d names, want 0", len(names))
	}
}

func TestClearReserve(t *testing.T) {
	s := NewSession(nil, nil, 1)
	s.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true)
	s.Offer(Suggestion{Artist: "B", Title: "T2"}, Song{ID: "2", Artist: "B", Title: "T2"}, true)
	s.ClosePortion()
	s.ClearReserve()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("after ClearReserve: %d names, want 0", len(names))
	}
}

func TestSessionDone(t *testing.T) {
	fill := func(s *Session, n int) {
		t.Helper()
		for i := range n {
			artist, title := fmt.Sprintf("A%d", i), fmt.Sprintf("T%d", i)
			v := s.Offer(Suggestion{Artist: artist, Title: title}, Song{ID: fmt.Sprintf("%d", i), Artist: artist, Title: title}, true)
			if !v.Accepted {
				t.Fatalf("%s: rejected: %s", artist, v.Reason)
			}
		}
	}
	s := NewSession(nil, nil, 20)
	fill(s, 18)
	if !s.Done() {
		t.Errorf("18 of 20: Done = false, want true")
	}
	w := NewSession(nil, nil, 20)
	fill(w, 17)
	if w.Done() {
		t.Errorf("17 of 20: Done = true, want false")
	}
	th := NewSession(nil, nil, 3)
	fill(th, 2)
	if th.Done() {
		t.Errorf("2 of 3: Done = true, want false")
	}
	if !th.Offer(Suggestion{Artist: "A100", Title: "T100"}, Song{ID: "100", Artist: "A100", Title: "T100"}, true).Accepted {
		t.Fatal("A100: rejected")
	}
	if !th.Done() {
		t.Errorf("3 of 3: Done = false, want true")
	}
}

func TestReserveSkipsRejectsWhenFull(t *testing.T) {
	s := NewSession(nil, nil, 1)
	a := Song{ID: "1", Artist: "A", Title: "T1"}
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, a, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, a, true); v.Reason != "already served" {
		t.Errorf("A again: %q, want «already served»", v.Reason)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("reserve: %v, want empty", names)
	}
}

func TestSessionNoteSaved(t *testing.T) {
	song := Song{ID: "1", Artist: "New Artist", Title: "Saved Song"}
	artist := splitArtists(song.Artist)[0]

	t.Run("library", func(t *testing.T) {
		s := NewSession(nil, nil, 3)
		s.NoteSaved(song, false)
		if !s.lib.Has(song) {
			t.Error("library save is missing from s.lib")
		}
		if s.favs.Has(song) {
			t.Error("library save leaked into s.favs")
		}
		if s.known[artist] {
			t.Error("library save added the artist to s.known")
		}
	})

	t.Run("favorite", func(t *testing.T) {
		s := NewSession(nil, nil, 3)
		s.NoteSaved(song, true)
		if !s.favs.Has(song) {
			t.Error("favorite save is missing from s.favs")
		}
		if s.lib.Has(song) {
			t.Error("favorite save leaked into s.lib")
		}
		if !s.known[artist] {
			t.Error("favorite save did not add the artist to s.known")
		}
	})
}

// F8/R3: favouriting a track makes its artist known to the tracks already in the
// portion, so the known budget drops with it.
// R1/R2/R3: playlist mode fills one portion of the target size, keeps the artists
// the listener named, and is done only when full.
func TestPlaylistModeLetsNamedArtistsThrough(t *testing.T) {
	s := NewSession(nil, nil, 5)
	s.SetPlan(Plan{
		Seeds: []Seed{
			{"Westside Gunn", "want_it"},
			{"Earl Sweatshirt", "and_similar"},
			{"Armand Hammer", "and_similar"},
		},
		Exclusions: []string{"Kendrick Lamar", "21 Savage"},
		Playlist:   &PlaylistPlan{Size: 30, Name: "Drumless haze"},
	})
	for i, line := range []string{
		"Westside Gunn & Joey Bada$$ — 327 (feat. Tyler, The Creator & Billie Essco)",
		"Westside Gunn — George Bondo (feat. Conway the Machine & BENNY the BUTCHER)",
		"Earl Sweatshirt — Shattered Dreams",
		"Armand Hammer & The Alchemist — Falling out the Sky (feat. Earl Sweatshirt)",
		"Benny the Butcher — Crowns for Kings (Feat. Black Thought)",
		"Westside Gunn — Allah Sent Me (feat. Conway the Machine & Benny the Butcher)",
		"Earl Sweatshirt — Nowhere2go",
	} {
		artist, title, _ := strings.Cut(line, " — ")
		song := Song{ID: fmt.Sprintf("%d", i+1), Artist: artist, Title: title}
		v := s.Offer(Suggestion{Artist: artist, Title: title}, song, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", line, v.Reason)
		}
	}
	if s.Need() != 23 {
		t.Errorf("Need = %d, want 23", s.Need())
	}
	if s.Done() {
		t.Error("Done = true, want false")
	}
}

func TestPlaylistModeDoneOnlyWhenFull(t *testing.T) {
	s := NewSession(nil, nil, 5)
	s.SetPlan(Plan{Playlist: &PlaylistPlan{Size: 10, Name: "P"}})
	for i := range 9 {
		artist := fmt.Sprintf("A%d", i)
		song := Song{ID: fmt.Sprintf("%d", i), Artist: artist, Title: fmt.Sprintf("T%d", i)}
		if v := s.Offer(Suggestion{Artist: artist, Title: song.Title}, song, true); !v.Accepted {
			t.Fatalf("%s: rejected: %s", artist, v.Reason)
		}
	}
	if s.Done() {
		t.Fatal("9 of 10: Done = true, want false")
	}
	song := Song{ID: "10", Artist: "B", Title: "T10"}
	if v := s.Offer(Suggestion{Artist: "B", Title: "T10"}, song, true); !v.Accepted {
		t.Fatalf("10th: rejected: %s", v.Reason)
	}
	if !s.Done() {
		t.Error("full: Done = false, want true")
	}
}

func TestSetPlanKeepsPlaylistWithinSession(t *testing.T) {
	s := NewSession(nil, nil, 5)
	s.SetPlan(Plan{Playlist: &PlaylistPlan{Size: 30, Name: "P"}})
	if s.Need() != 30 {
		t.Fatalf("playlist: Need = %d, want 30", s.Need())
	}
	s.SetPlan(Plan{})
	if s.Need() != 30 {
		t.Errorf("second plan: Need = %d, want 30", s.Need())
	}
	if p := s.Playlist(); p == nil || p.Size != 30 {
		t.Errorf("second plan: playlist = %+v, want the 30-track playlist kept", p)
	}
}

func TestStreamSessionStaysStream(t *testing.T) {
	s := NewSession(nil, nil, 5)
	s.SetPlan(Plan{})
	s.SetPlan(Plan{})
	if s.Need() != 5 {
		t.Errorf("stream: Need = %d, want 5", s.Need())
	}
	if p := s.Playlist(); p != nil {
		t.Errorf("stream: playlist = %+v, want nil", p)
	}
}
