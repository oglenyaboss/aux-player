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
	s := NewSession(nil, nil, 6)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "Westside Gunn", Mode: "and_similar"}}})
	v1 := s.Offer(Suggestion{Artist: "Westside Gunn", Title: "A"}, Song{ID: "1", Artist: "Westside Gunn", Title: "A"}, true)
	if !v1.Accepted {
		t.Fatalf("A: rejected: %s", v1.Reason)
	}
	v2 := s.Offer(Suggestion{Artist: "Westside Gunn", Title: "B"}, Song{ID: "2", Artist: "Westside Gunn", Title: "B"}, true)
	if v2.Accepted || v2.Reason != "лимит ориентира" {
		t.Errorf("B: got (%v, %q), want (false, \"лимит ориентира\")", v2.Accepted, v2.Reason)
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

func TestOfferSignalsServedBefore(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig, signalRec{Time: testNow, Kind: "served", Session: "s1", Artist: "Ken Carson", Title: "Yale"})
	s := NewSession(nil, nil, 20)
	s.SetSignals(sig, "s2")
	v := s.Offer(Suggestion{Artist: "Ken Carson", Title: "Yale"}, Song{ID: "1", Artist: "Ken Carson", Title: "Yale"}, true)
	if v.Accepted || v.Reason != "уже выдавали раньше" {
		t.Errorf("got (%v, %q), want (false, \"уже выдавали раньше\")", v.Accepted, v.Reason)
	}
}

func TestOfferSignalsGlobalArtistBlock(t *testing.T) {
	sig := testSignals(t, "")
	addSignals(t, sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "s1", Artist: "Ghostemane", Title: "Mercury: Retrograde"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "s2", Artist: "Ghostemane", Title: "Rapture"},
	)
	song := Song{ID: "1", Artist: "Ghostemane", Title: "Mercury: Retrograde"}
	s := NewSession(nil, nil, 20)
	s.SetSignals(sig, "s3")
	v := s.Offer(Suggestion{Artist: "Ghostemane", Title: "Mercury: Retrograde"}, song, true)
	if v.Accepted || v.Reason != "владелец скипает (2 сессий)" {
		t.Errorf("без сидов: got (%v, %q), want (false, \"владелец скипает (2 сессий)\")", v.Accepted, v.Reason)
	}
	w := NewSession(nil, nil, 20)
	w.SetPlan(Plan{Seeds: []Seed{{Artist: "Ghostemane", Mode: "and_similar"}}})
	w.SetSignals(sig, "s3")
	v2 := w.Offer(Suggestion{Artist: "Ghostemane", Title: "Mercury: Retrograde"}, song, true)
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
