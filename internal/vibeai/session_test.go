package vibeai

import (
	"fmt"
	"testing"
)

func favArtists(t *testing.T, n int) []Song {
	t.Helper()
	favs := make([]Song, 0, n)
	for i := 1; i <= n; i++ {
		favs = append(favs, Song{ID: fmt.Sprintf("f%d", i), Artist: fmt.Sprintf("k%d", i), Title: fmt.Sprintf("fav%d", i)})
	}
	return favs
}

func TestOfferNotFound(t *testing.T) {
	s := NewSession(nil, nil, 20)
	v := s.Offer(Suggestion{Artist: "A", Title: "T"}, Song{}, false)
	if v.Accepted || v.Reason != "нет в Apple" || v.Song != (Song{}) {
		t.Errorf("got (%v, %q, %+v), want (false, \"нет в Apple\", empty song)", v.Accepted, v.Reason, v.Song)
	}
}

func TestOfferPortionFull(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "Yeat", Title: "B"}, Song{ID: "2", Artist: "Yeat", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "порция заполнена" {
		t.Errorf("second: got (%v, %q), want (false, \"порция заполнена\")", v2.Accepted, v2.Reason)
	}
}

func TestOfferExclusion(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{Exclusions: []string{"Scally Milano"}})
	song := Song{ID: "1", Artist: "Scally Milano & X", Title: "T"}
	v := s.Offer(Suggestion{Artist: "Scally Milano & X", Title: "T"}, song, true)
	if v.Accepted || v.Reason != "исключение: Scally Milano" {
		t.Errorf("got (%v, %q), want (false, \"исключение: Scally Milano\")", v.Accepted, v.Reason)
	}
}

func TestOfferSeedNotIt(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Don Toliver", Mode: "not_it"}}})
	song := Song{ID: "1", Artist: "Travis Scott", Title: "T (feat. Don Toliver)"}
	v := s.Offer(Suggestion{Artist: "Travis Scott", Title: "T (feat. Don Toliver)"}, song, true)
	if v.Accepted || v.Reason != "сам ориентир (не он)" {
		t.Errorf("got (%v, %q), want (false, \"сам ориентир (не он)\")", v.Accepted, v.Reason)
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
	if v2.Accepted || v2.Reason != "уже было" {
		t.Errorf("repeat: got (%v, %q), want (false, \"уже было\")", v2.Accepted, v2.Reason)
	}
}

func TestOfferDuplicateInPortion(t *testing.T) {
	s := NewSession(nil, nil, 20)
	first := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !first.Accepted {
		t.Fatalf("first: rejected: %s", first.Reason)
	}
	v := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "2", Artist: "Yeat", Title: "A"}, true)
	if v.Accepted || v.Reason != "уже было" {
		t.Errorf("dup in portion: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
	}
}

func TestOfferLibraryAndFavorite(t *testing.T) {
	lib := []Song{
		{ID: "1", Artist: "L", Title: "T1"},
		{ID: "2", Artist: "F", Title: "T2"},
	}
	favs := []Song{{ID: "2", Artist: "F", Title: "T2"}}
	s := NewSession(lib, favs, 20)
	v1 := s.Offer(Suggestion{Artist: "L", Title: "T1"}, lib[0], true)
	if v1.Accepted || v1.Reason != "известный трек (библиотека)" {
		t.Errorf("library track: got (%v, %q), want (false, \"известный трек (библиотека)\")", v1.Accepted, v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "F", Title: "T2", Why: "потому что"}, favs[0], true)
	if !v2.Accepted {
		t.Fatalf("favorite track: rejected: %s", v2.Reason)
	}
	e := s.Portion()[0]
	if !e.Favorite || !e.Known || e.Why != "потому что" {
		t.Errorf("entry: got %+v, want Favorite=true Known=true Why=\"потому что\"", e)
	}
}

func TestOfferSeedLibraryTrack(t *testing.T) {
	lib := []Song{
		{ID: "1", Artist: "Don Toliver", Title: "BANDIT"},
		{ID: "2", Artist: "Travis Scott", Title: "FE!N (feat. Playboi Carti)"},
		{ID: "3", Artist: "Yeat", Title: "Talk"},
	}
	s := NewSession(lib, nil, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Don Toliver", Mode: "and_similar"}, {Artist: "Playboi Carti", Mode: "want_it"}}})
	for _, song := range lib[:2] {
		if v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true); !v.Accepted {
			t.Errorf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
	v := s.Offer(Suggestion{Artist: "Yeat", Title: "Talk"}, lib[2], true)
	if v.Accepted || v.Reason != "известный трек (библиотека)" {
		t.Errorf("non-seed library track: got (%v, %q), want (false, \"известный трек (библиотека)\")", v.Accepted, v.Reason)
	}
}

func TestOfferArtistLimit(t *testing.T) {
	s := NewSession(nil, nil, 20)
	for i, title := range []string{"A", "B"} {
		v := s.Offer(Suggestion{Artist: "Yeat", Title: title}, Song{ID: fmt.Sprintf("%d", i), Artist: "Yeat", Title: title}, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", title, v.Reason)
		}
	}
	song := Song{ID: "3", Artist: "Lil Uzi Vert", Title: "C (feat. Yeat)"}
	v := s.Offer(Suggestion{Artist: "Lil Uzi Vert", Title: "C (feat. Yeat)"}, song, true)
	if v.Accepted || v.Reason != "лимит артиста: yeat" {
		t.Errorf("got (%v, %q), want (false, \"лимит артиста: yeat\")", v.Accepted, v.Reason)
	}
}

func TestOfferWantItSeed(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Westside Gunn", Mode: "want_it"}}})
	for i := 1; i <= 5; i++ {
		title := fmt.Sprintf("S%d", i)
		v := s.Offer(Suggestion{Artist: "Westside Gunn", Title: title}, Song{ID: fmt.Sprintf("%d", i), Artist: "Westside Gunn", Title: title}, true)
		if !v.Accepted {
			t.Fatalf("song %d: rejected: %s", i, v.Reason)
		}
	}
}

func TestOfferArtistLimitBeforeSimilar(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Westside Gunn", Mode: "and_similar"}}})
	for i, title := range []string{"A", "B"} {
		v := s.Offer(Suggestion{Artist: "Westside Gunn", Title: title}, Song{ID: fmt.Sprintf("%d", i), Artist: "Westside Gunn", Title: title}, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", title, v.Reason)
		}
	}
	v := s.Offer(Suggestion{Artist: "Westside Gunn", Title: "C"}, Song{ID: "3", Artist: "Westside Gunn", Title: "C"}, true)
	if v.Accepted || v.Reason != "лимит артиста: westsidegunn" {
		t.Errorf("got (%v, %q), want (false, \"лимит артиста: westsidegunn\")", v.Accepted, v.Reason)
	}
}

func TestOfferSimilarLimit(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.PerArtist = 5
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Westside Gunn", Mode: "and_similar"}}})
	for i, title := range []string{"A", "B", "C"} {
		v := s.Offer(Suggestion{Artist: "Westside Gunn", Title: title}, Song{ID: fmt.Sprintf("%d", i), Artist: "Westside Gunn", Title: title}, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", title, v.Reason)
		}
	}
	v4 := s.Offer(Suggestion{Artist: "Westside Gunn", Title: "D"}, Song{ID: "3", Artist: "Westside Gunn", Title: "D"}, true)
	if v4.Accepted || v4.Reason != "лимит ориентира" {
		t.Errorf("D: got (%v, %q), want (false, \"лимит ориентира\")", v4.Accepted, v4.Reason)
	}
}

func TestWindowArtistLimit(t *testing.T) {
	s := NewSession(nil, nil, 5)
	for _, song := range []Song{
		{ID: "1", Artist: "A", Title: "T1"},
		{ID: "2", Artist: "A", Title: "T2"},
		{ID: "3", Artist: "B", Title: "T3"},
		{ID: "4", Artist: "C", Title: "T4"},
		{ID: "5", Artist: "D", Title: "T5"},
	} {
		v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true)
		if !v.Accepted {
			t.Fatalf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
	s.ClosePortion()
	v := s.Offer(Suggestion{Artist: "A", Title: "T6"}, Song{ID: "6", Artist: "A", Title: "T6"}, true)
	if v.Accepted || v.Reason != "лимит артиста: a" {
		t.Errorf("got (%v, %q), want (false, \"лимит артиста: a\")", v.Accepted, v.Reason)
	}
}

func TestWindowArtistLimitExpires(t *testing.T) {
	s := NewSession(nil, nil, 5)
	for _, song := range []Song{
		{ID: "1", Artist: "A", Title: "T1"},
		{ID: "2", Artist: "A", Title: "T2"},
		{ID: "3", Artist: "B", Title: "T3"},
		{ID: "4", Artist: "C", Title: "T4"},
		{ID: "5", Artist: "D", Title: "T5"},
	} {
		v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true)
		if !v.Accepted {
			t.Fatalf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
	s.ClosePortion()
	for p := 2; p <= 4; p++ {
		for i := 0; i < 5; i++ {
			artist, title := fmt.Sprintf("P%dE%d", p, i), fmt.Sprintf("Q%d", i)
			song := Song{ID: fmt.Sprintf("%d-%d", p, i), Artist: artist, Title: title}
			v := s.Offer(Suggestion{Artist: artist, Title: title}, song, true)
			if !v.Accepted {
				t.Fatalf("порция %d, %s: rejected: %s", p, artist, v.Reason)
			}
		}
		s.ClosePortion()
	}
	v := s.Offer(Suggestion{Artist: "A", Title: "T6"}, Song{ID: "6", Artist: "A", Title: "T6"}, true)
	if !v.Accepted {
		t.Errorf("T6: rejected: %s", v.Reason)
	}
}

func TestWindowKnownLimit(t *testing.T) {
	favs := favArtists(t, 6)
	s := NewSession(favs, favs, 5)
	for i := 1; i <= 3; i++ {
		artist, title := fmt.Sprintf("k%d", i), fmt.Sprintf("t%d", i)
		v := s.Offer(Suggestion{Artist: artist, Title: title}, Song{ID: fmt.Sprintf("o%d", i), Artist: artist, Title: title}, true)
		if !v.Accepted {
			t.Fatalf("k%d: rejected: %s", i, v.Reason)
		}
	}
	for i := 0; i < 2; i++ {
		artist, title := fmt.Sprintf("n%d", i), fmt.Sprintf("u%d", i)
		v := s.Offer(Suggestion{Artist: artist, Title: title}, Song{ID: title, Artist: artist, Title: title}, true)
		if !v.Accepted {
			t.Fatalf("%s: rejected: %s", artist, v.Reason)
		}
	}
	s.ClosePortion()
	v := s.Offer(Suggestion{Artist: "k4", Title: "t4"}, Song{ID: "o4", Artist: "k4", Title: "t4"}, true)
	if !v.Accepted {
		t.Fatalf("k4: rejected: %s", v.Reason)
	}
	if got := s.KnownLeft(); got != 0 {
		t.Errorf("KnownLeft = %d, want 0", got)
	}
	w := s.Offer(Suggestion{Artist: "k5", Title: "t5"}, Song{ID: "o5", Artist: "k5", Title: "t5"}, true)
	if w.Accepted || w.Reason != "лимит известных" {
		t.Errorf("k5: got (%v, %q), want (false, \"лимит известных\")", w.Accepted, w.Reason)
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
		t.Errorf("portion: %+v, want два entry с Why=\"w\"", part)
	}
	if v := s.Offer(Suggestion{Artist: "L", Title: "TL"}, Song{ID: "n3", Artist: "L", Title: "TL"}, true); v.Accepted || v.Reason != "уже было" {
		t.Errorf("offer L: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
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
	if v.Accepted || v.Reason != "порция полна" {
		t.Errorf("take 3: got (%v, %q), want (false, \"порция полна\")", v.Accepted, v.Reason)
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
	if v.Accepted || v.Reason != "уже было" {
		t.Errorf("take given: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
	}
	if v := s.Take(Suggestion{Artist: "Q", Title: "TQ"}, Song{ID: "3", Artist: "Q", Title: "TQ"}); !v.Accepted {
		t.Fatalf("take Q: rejected: %s", v.Reason)
	}
	if v := s.Take(Suggestion{Artist: "Q", Title: "TQ"}, Song{ID: "4", Artist: "Q", Title: "TQ"}); v.Accepted || v.Reason != "уже было" {
		t.Errorf("take twice: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
	}
}

func TestResetPortion(t *testing.T) {
	s := NewSession(nil, nil, 5)
	if v := s.Offer(Suggestion{Artist: "S", Title: "TS"}, Song{ID: "1", Artist: "S", Title: "TS"}, true); !v.Accepted {
		t.Fatalf("first: rejected: %s", v.Reason)
	}
	s.ResetPortion()
	if got := s.Portion(); len(got) != 0 {
		t.Fatalf("portion после сброса: %+v, want пусто", got)
	}
	if got := s.Need(); got != 5 {
		t.Fatalf("Need после сброса = %d, want 5", got)
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
	if v := s.Offer(Suggestion{Artist: "P1", Title: "T1"}, Song{ID: "4", Artist: "P1", Title: "T1"}, true); v.Accepted || v.Reason != "уже было" {
		t.Errorf("P1: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
	}
	s.DropLast()
	if v := s.Offer(Suggestion{Artist: "P1", Title: "T1"}, Song{ID: "5", Artist: "P1", Title: "T1"}, true); v.Accepted || v.Reason != "уже было" {
		t.Errorf("второй drop: got (%v, %q), want (false, \"уже было\")", v.Accepted, v.Reason)
	}
}

func TestDropLastFreesWindow(t *testing.T) {
	s := NewSession(nil, nil, 5)
	for _, song := range []Song{
		{ID: "1", Artist: "A", Title: "T1"},
		{ID: "2", Artist: "A", Title: "T2"},
		{ID: "3", Artist: "B", Title: "T3"},
		{ID: "4", Artist: "C", Title: "T4"},
		{ID: "5", Artist: "D", Title: "T5"},
	} {
		v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true)
		if !v.Accepted {
			t.Fatalf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
	s.ClosePortion()
	s.DropLast()
	v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true)
	if !v.Accepted {
		t.Errorf("A—T1: rejected: %s", v.Reason)
	}
}

func TestOfferKnownLimit(t *testing.T) {
	favs := favArtists(t, 5)
	s := NewSession(favs, favs, 20)
	for i := 1; i <= 4; i++ {
		artist, title := fmt.Sprintf("k%d", i), fmt.Sprintf("t%d", i)
		v := s.Offer(Suggestion{Artist: artist, Title: title}, Song{ID: fmt.Sprintf("o%d", i), Artist: artist, Title: title}, true)
		if !v.Accepted {
			t.Fatalf("k%d: rejected: %s", i, v.Reason)
		}
		if !s.Portion()[len(s.Portion())-1].Known {
			t.Errorf("k%d: entry Known = false, want true", i)
		}
	}
	v := s.Offer(Suggestion{Artist: "k5", Title: "t5"}, Song{ID: "o5", Artist: "k5", Title: "t5"}, true)
	if v.Accepted || v.Reason != "лимит известных" {
		t.Errorf("k5: got (%v, %q), want (false, \"лимит известных\")", v.Accepted, v.Reason)
	}
	if got := s.KnownLeft(); got != 0 {
		t.Errorf("KnownLeft = %d, want 0", got)
	}
	s.ClosePortion()
	if got := s.KnownLeft(); got != 4 {
		t.Errorf("KnownLeft after ClosePortion = %d, want 4", got)
	}
}

func TestOfferSeedKnownNotCounted(t *testing.T) {
	favs := favArtists(t, 5)
	s := NewSession(favs, favs, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "k1", Mode: "and_similar"}}})
	for i := 2; i <= 5; i++ {
		artist, title := fmt.Sprintf("k%d", i), fmt.Sprintf("t%d", i)
		v := s.Offer(Suggestion{Artist: artist, Title: title}, Song{ID: fmt.Sprintf("o%d", i), Artist: artist, Title: title}, true)
		if !v.Accepted {
			t.Fatalf("k%d: rejected: %s", i, v.Reason)
		}
	}
	v := s.Offer(Suggestion{Artist: "k1", Title: "X"}, Song{ID: "x", Artist: "k1", Title: "X"}, true)
	if !v.Accepted {
		t.Errorf("k1: rejected: %s", v.Reason)
	}
}

func TestOfferSignalsSkippedBefore(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_mid", Session: "s1", Artist: "Ken Carson", Title: "Yale"})
	s := NewSession(nil, nil, 20)
	s.SetSignals(sig, "s2")
	v := s.Offer(Suggestion{Artist: "Ken Carson", Title: "Yale"}, Song{ID: "1", Artist: "Ken Carson", Title: "Yale"}, true)
	if v.Accepted || v.Reason != "бросал раньше, не дослушав" {
		t.Errorf("got (%v, %q), want (false, \"бросал раньше, не дослушав\")", v.Accepted, v.Reason)
	}
}

func TestOfferSignalsGlobalArtistBlock(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	song := Song{ID: "1", Artist: "Ghostemane", Title: "Venom"}
	s := NewSession(nil, nil, 20)
	s.SetSignals(sig, "s3")
	v := s.Offer(Suggestion{Artist: "Ghostemane", Title: "Venom"}, song, true)
	if v.Accepted || v.Reason != "владелец скипает (2 сессий)" {
		t.Errorf("без сидов: got (%v, %q), want (false, \"владелец скипает (2 сессий)\")", v.Accepted, v.Reason)
	}
	w := NewSession(nil, nil, 20)
	w.SetPlan(Plan{Seeds: []Seed{{Artist: "Ghostemane", Mode: "and_similar"}}})
	w.SetSignals(sig, "s3")
	v2 := w.Offer(Suggestion{Artist: "Ghostemane", Title: "Venom"}, song, true)
	if !v2.Accepted {
		t.Errorf("seed and_similar: rejected: %s", v2.Reason)
	}
}

func TestOfferSignalsSessionSkipBeatsWantIt(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_early", Session: "s3", Artist: "Ghostemane", Title: "Mercury: Retrograde"})
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Ghostemane", Mode: "want_it"}}})
	s.SetSignals(sig, "s3")
	v := s.Offer(Suggestion{Artist: "Ghostemane", Title: "Mercury: Retrograde"}, Song{ID: "1", Artist: "Ghostemane", Title: "Mercury: Retrograde"}, true)
	if v.Accepted || v.Reason != "скипнут в этой сессии" {
		t.Errorf("got (%v, %q), want (false, \"скипнут в этой сессии\")", v.Accepted, v.Reason)
	}
}

func TestOfferSignalsBlockedViaFeatArtist(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "skip_early", Session: "s3", Artist: "$NOT", Title: "GOSHA"})
	s := NewSession(nil, nil, 20)
	s.SetSignals(sig, "s3")
	song := Song{ID: "1", Artist: "Cochise & $NOT", Title: "Tell Em"}
	v := s.Offer(Suggestion{Artist: "Cochise & $NOT", Title: "Tell Em"}, song, true)
	if v.Accepted || v.Reason != "скипнут в этой сессии" {
		t.Errorf("got (%v, %q), want (false, \"скипнут в этой сессии\")", v.Accepted, v.Reason)
	}
}

func TestReserveOverflowNames(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "Don Toliver", Title: "B"}, Song{ID: "2", Artist: "Don Toliver", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "порция заполнена" {
		t.Fatalf("second: got (%v, %q), want (false, \"порция заполнена\")", v2.Accepted, v2.Reason)
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 1 || names[0] != "Don Toliver — B" {
		t.Fatalf("запас: %v, want [Don Toliver — B]", names)
	}
	if got := s.Portion(); len(got) != 0 {
		t.Errorf("portion после запаса: %+v, want пусто", got)
	}
}

func TestReserveSkipsNotFoundAndOtherRejects(t *testing.T) {
	s := NewSession(nil, nil, 1)
	v1 := s.Offer(Suggestion{Artist: "Yeat", Title: "A"}, Song{ID: "1", Artist: "Yeat", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("first: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{}, false)
	if v2.Accepted || v2.Reason != "нет в Apple" {
		t.Fatalf("not found: got (%v, %q), want (false, \"нет в Apple\")", v2.Accepted, v2.Reason)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Fatalf("запас: %d имён, want 0: %v", len(names), names)
	}
	if got := s.Portion(); len(got) != 0 {
		t.Errorf("portion после запаса: %+v, want пусто", got)
	}

	w := NewSession(nil, nil, 2)
	w.SetPlan(Plan{Exclusions: []string{"Xeno"}})
	if v := w.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	xeno := Song{ID: "x", Artist: "Xeno", Title: "X"}
	if v := w.Offer(Suggestion{Artist: "Xeno", Title: "X"}, xeno, true); v.Accepted || v.Reason != "исключение: Xeno" {
		t.Fatalf("xeno: got (%v, %q), want (false, \"исключение: Xeno\")", v.Accepted, v.Reason)
	}
	if v := w.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{}, false); v.Reason != "нет в Apple" {
		t.Fatalf("C: got %q", v.Reason)
	}
	if v := w.Offer(Suggestion{Artist: "B", Title: "T2"}, Song{ID: "2", Artist: "B", Title: "T2"}, true); !v.Accepted {
		t.Fatalf("B: rejected: %s", v.Reason)
	}
	w.ClosePortion()
	if names := w.TakeReserve(); len(names) != 0 {
		t.Errorf("запас (исключение и нет в Apple): %d имён, want 0: %v", len(names), names)
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
	for i := 0; i < 2; i++ {
		v := s.Offer(Suggestion{Artist: "C", Title: "T3"}, Song{ID: "c", Artist: "C", Title: "T3"}, true)
		if v.Accepted || v.Reason != "порция заполнена" {
			t.Fatalf("C %d-й: got (%v, %q)", i, v.Accepted, v.Reason)
		}
	}
	for i, want := range []string{"D", "E"} {
		v := s.Offer(Suggestion{Artist: want, Title: fmt.Sprintf("T%d", 4+i)}, Song{ID: want, Artist: want, Title: fmt.Sprintf("T%d", 4+i)}, true)
		if v.Accepted || v.Reason != "порция заполнена" {
			t.Fatalf("%s: got (%v, %q)", want, v.Accepted, v.Reason)
		}
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 2 {
		t.Fatalf("запас: %d имён, want 2 (C, D, без дубля C и без E): %v", len(names), names)
	}
	for i, want := range []string{"C — T3", "D — T4"} {
		if names[i] != want {
			t.Errorf("запас[%d] = %q, want %q", i, names[i], want)
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
		if v.Accepted || v.Reason != "порция заполнена" {
			t.Fatalf("%d: got (%v, %q)", i, v.Accepted, v.Reason)
		}
	}
	s.ClosePortion()
	names := s.TakeReserve()
	if len(names) != 1 || names[0] != "B — T2" {
		t.Fatalf("запас: %v, want [B — T2]", names)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("второй запас: %d имён, want 0", len(names))
	}
}

func TestClearReserve(t *testing.T) {
	s := NewSession(nil, nil, 1)
	s.Offer(Suggestion{Artist: "A", Title: "T1"}, Song{ID: "1", Artist: "A", Title: "T1"}, true)
	s.Offer(Suggestion{Artist: "B", Title: "T2"}, Song{ID: "2", Artist: "B", Title: "T2"}, true)
	s.ClosePortion()
	s.ClearReserve()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("после ClearReserve: %d имён, want 0", len(names))
	}
}

func TestSessionDone(t *testing.T) {
	fill := func(s *Session, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
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
		t.Errorf("18 из 20: Done = false, want true")
	}
	w := NewSession(nil, nil, 20)
	fill(w, 17)
	if w.Done() {
		t.Errorf("17 из 20: Done = true, want false")
	}
	th := NewSession(nil, nil, 3)
	fill(th, 2)
	if th.Done() {
		t.Errorf("2 из 3: Done = true, want false")
	}
	if !th.Offer(Suggestion{Artist: "A100", Title: "T100"}, Song{ID: "100", Artist: "A100", Title: "T100"}, true).Accepted {
		t.Fatal("A100: rejected")
	}
	if !th.Done() {
		t.Errorf("3 из 3: Done = false, want true")
	}
}

func TestOfferReplan(t *testing.T) {
	s := NewSession(nil, nil, 20)
	s.SetPlan(Plan{})
	v1 := s.Offer(Suggestion{Artist: "Scally Milano", Title: "A"}, Song{ID: "1", Artist: "Scally Milano", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("before replan: rejected: %s", v1.Reason)
	}
	s.SetPlan(Plan{Exclusions: []string{"Scally Milano"}})
	if len(s.Portion()) != 1 || s.Portion()[0].Song.ID != "1" {
		t.Fatalf("after replan: portion = %+v, want old entry kept", s.Portion())
	}
	v2 := s.Offer(Suggestion{Artist: "Scally Milano", Title: "B"}, Song{ID: "2", Artist: "Scally Milano", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "исключение: Scally Milano" {
		t.Errorf("after replan: got (%v, %q), want (false, \"исключение: Scally Milano\")", v2.Accepted, v2.Reason)
	}
}

func TestReserveSkipsRejectsWhenFull(t *testing.T) {
	s := NewSession(nil, nil, 1)
	s.SetPlan(Plan{Exclusions: []string{"Xeno"}})
	a := Song{ID: "1", Artist: "A", Title: "T1"}
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, a, true); !v.Accepted {
		t.Fatalf("A: rejected: %s", v.Reason)
	}
	if v := s.Offer(Suggestion{Artist: "A", Title: "T1"}, a, true); v.Reason != "уже было" {
		t.Errorf("A again: %q, want «уже было»", v.Reason)
	}
	xeno := Song{ID: "x", Artist: "Xeno", Title: "X"}
	if v := s.Offer(Suggestion{Artist: "Xeno", Title: "X"}, xeno, true); v.Reason != "исключение: Xeno" {
		t.Errorf("xeno: %q, want «исключение: Xeno»", v.Reason)
	}
	s.ClosePortion()
	if names := s.TakeReserve(); len(names) != 0 {
		t.Errorf("запас: %v, want пусто", names)
	}
}
