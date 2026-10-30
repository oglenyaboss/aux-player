package vibeai

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

func newTestEngineUI(t *testing.T, ch *fakeChatter, ui UI) *Engine {
	a := newTestAgent(t, ch, &fakeBackend{}, ui)
	sig, err := OpenSignals("")
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	sig.now = func() time.Time { return testNow }
	return &Engine{
		agent:     a,
		session:   a.Session,
		mem:       a.Memory,
		log:       &runLog{},
		sig:       sig,
		sessionID: "s1",
		start:     time.Now(),
	}
}

func newTestEngine(t *testing.T, ch *fakeChatter) *Engine {
	return newTestEngineUI(t, ch, nil)
}

func flowEngine(t *testing.T, ch *fakeChatter) *Engine {
	t.Helper()
	ch.resps = append(ch.resps,
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(call("c3", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	)
	e := newTestEngine(t, ch)
	p, err := e.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if len(p.Tracks) != 3 {
		t.Fatalf("ход 1: %d треков, want 3", len(p.Tracks))
	}
	err = e.Record([]Outcome{
		{ID: p.Tracks[0].ID, Artist: "A", Title: "T1", Kind: "played"},
		{ID: p.Tracks[1].ID, Artist: "B", Title: "T2", Kind: "skip_early", PosSec: 12},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	for i, want := range []string{"D", "E", "F"} {
		if p2.Tracks[i].Artist != want {
			t.Errorf("ход 2, трек %d: %q, want %q", i, p2.Tracks[i].Artist, want)
		}
	}
	return e
}

func TestTurnFreshContextEachTurn(t *testing.T) {
	ch := &fakeChatter{}
	flowEngine(t, ch)
	if len(ch.got) != 3 {
		t.Fatalf("Chat вызван %d раз, want 3", len(ch.got))
	}
	msgs := ch.got[2]
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("последний вызов: %d сообщений, want ровно [system user]", len(msgs))
	}
	user := msgs[1].Content
	for _, want := range []string{
		`Session: request "в духе X".`,
		"referents: X (and_similar)",
		"1. A — T1 — played through",
		"2. B — T2 — skipped at 0:12",
		"3. C — T3 — not listened",
		"Continue the stream",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user не содержит %q:\n%s", want, user)
		}
	}
}

func TestTurnReplyLifecycle(t *testing.T) {
	ch := &fakeChatter{}
	e := flowEngine(t, ch)
	ch.resps = append(ch.resps,
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"G","title":"T7","why":"w"},
			{"artist":"H","title":"T8","why":"w"},
			{"artist":"I","title":"T9","why":"w"}
		]}`)),
		toolResp(call("c5", "submit_candidates", `{"tracks":[
			{"artist":"J","title":"T10","why":"w"},
			{"artist":"K","title":"T11","why":"w"},
			{"artist":"L","title":"T12","why":"w"}
		]}`)),
	)
	if _, err := e.Turn(context.Background(), "меньше такого"); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	user := ch.got[3][1].Content
	for _, want := range []string{"Portion 2:", "4. D — ", `New reply from the listener: "меньше такого"`} {
		if !strings.Contains(user, want) {
			t.Errorf("ход 3: user не содержит %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Listener replies:") {
		t.Errorf("ход 3: user содержит «Реплики владельца:\n%s", user)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 4: %v", err)
	}
	user = ch.got[4][1].Content
	for _, want := range []string{"Listener replies:", `- after portion 2: "меньше такого"`} {
		if !strings.Contains(user, want) {
			t.Errorf("ход 4: user не содержит %q:\n%s", want, user)
		}
	}
	log := e.log.b.String()
	for _, want := range []string{"## listener: (stream)", "## listener: меньше такого"} {
		if !strings.Contains(log, want) {
			t.Errorf("лог не содержит %q:\n%s", want, log)
		}
	}
}

func TestTurnFirstTurnGlobalBlock(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	addSignals(t, e.sig,
		signalRec{Time: testNow, Kind: "skip_early", Session: "g1", Artist: "Ghostemane", Title: "X", Query: "q1"},
		signalRec{Time: testNow, Kind: "skip_early", Session: "g2", Artist: "Ghostemane", Title: "X", Query: "q2"},
	)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	key := songArtists(Song{Artist: "Ghostemane", Title: "X"})[0]
	user := ch.got[0][1].Content
	want := "в духе X\n\n" + globalBlock(e.sig.globalSkips()) + "\n\n" + skippedBlock(e.sig.skippedElsewhere("s1"))
	if user != want {
		t.Errorf("первое сообщение:\n%s\nwant:\n%s", user, want)
	}
	if !strings.Contains(user, "- "+key+": early skips 2 in 2 sessions, played through 0; queries: \"q1\", \"q2\"") {
		t.Errorf("нет строки статистики:\n%s", user)
	}
}

func TestSkippedBlock(t *testing.T) {
	if got := skippedBlock(nil); got != "" {
		t.Errorf("skippedBlock(nil) = %q, want \"\"", got)
	}
	got := skippedBlock([]Song{
		{Artist: "Ken Carson", Title: "Yale"},
		{Artist: "SoFaygo", Title: "Knock Knock"},
		{Artist: "Ken Carson", Title: "Freestyle 2"},
	})
	want := "In past sessions the listener dropped these before the middle (the code will reject them, do not offer them):\n" +
		"- Ken Carson: Yale, Freestyle 2\n" +
		"- SoFaygo: Knock Knock"
	if got != want {
		t.Errorf("skippedBlock:\n%s\nwant:\n%s", got, want)
	}
}

func TestTurnFirstTurnSkippedBlock(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	addSignals(t, e.sig, signalRec{Time: testNow, Kind: "skip_mid", Session: "other", Artist: "Ken Carson", Title: "Yale"})
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	user := ch.got[0][1].Content
	want := "в духе X\n\n" + skippedBlock(e.sig.skippedElsewhere("s1"))
	if user != want {
		t.Errorf("первое сообщение:\n%s\nwant:\n%s", user, want)
	}
}

func TestHistorySkippedBlock(t *testing.T) {
	e := newTestEngine(t, &fakeChatter{})
	if h := e.history(); strings.Contains(h, "In past sessions the listener dropped") {
		t.Fatalf("history без записей содержит блок:\n%s", h)
	}
	addSignals(t, e.sig, signalRec{Time: testNow, Kind: "skip_mid", Session: "other", Artist: "SoFaygo", Title: "Knock Knock"})
	h := e.history()
	want := skippedBlock(e.sig.skippedElsewhere("s1"))
	if want == "" {
		t.Fatal("skippedBlock пуст, тест бессмыслен")
	}
	if !strings.HasSuffix(h, want) {
		t.Errorf("history не заканчивается на блок:\n%s\nwant suffix:\n%s", h, want)
	}
}

func TestGlobalBlock(t *testing.T) {
	if got := globalBlock(nil); got != "" {
		t.Errorf("globalBlock(nil) = %q, want \"\"", got)
	}
	got := globalBlock([]skipStat{{Artist: "ghostemane", Early: 2, Sessions: 2, Played: 1}})
	want := "Lasting skips from past sessions (the code no longer lets these artists through):\n" +
		"- ghostemane: early skips 2 in 2 sessions, played through 1\n" +
		"If they share a sound or a direction — write it to memory as one entry (memory_add or memory_replace), evidence — these numbers. Do not write about single artists to memory."
	if got != want {
		t.Errorf("globalBlock:\n%s\nwant:\n%s", got, want)
	}
}

func TestPromptRulesReactionWording(t *testing.T) {
	if !strings.Contains(promptRules, "Step aside only from tracks played to the end") {
		t.Error("нет новой формулировки про шаг в сторону")
	}
	if strings.Contains(promptRules, "may drift a step") {
		t.Error("осталась старая формулировка про дрейф")
	}
}

func TestValidateNoAppleTokens(t *testing.T) {
	err := Config{}.Validate()
	if err == nil || !strings.Contains(err.Error(), "no Apple tokens") {
		t.Errorf("пустой конфиг: err = %v, want «no Apple tokens»", err)
	}
}

func TestValidateNoLLMKey(t *testing.T) {
	cfg := Config{AppleDeveloperToken: "dev", AppleUserToken: "user"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "no LLM key") {
		t.Errorf("без LLM-ключа: err = %v, want «no LLM key»", err)
	}
}

func TestCacheFresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if cacheFresh(dir, time.Hour, now) {
		t.Error("нет файлов: want false")
	}
	lib := filepath.Join(dir, "library.json")
	favs := filepath.Join(dir, "favorites.json")
	for _, p := range []string{lib, favs} {
		if err := os.WriteFile(p, []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !cacheFresh(dir, time.Hour, now) {
		t.Error("оба файла, mtime сейчас: want true")
	}
	old := now.Add(-2 * time.Hour)
	for _, p := range []string{lib, favs} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if cacheFresh(dir, time.Hour, now) {
		t.Error("mtime старше maxAge: want false")
	}
}

func TestLastfmClientNoKey(t *testing.T) {
	l := &lastfmClient{}
	_, err := l.call(context.Background(), "artist.getsimilar", url.Values{"artist": {"Yeat"}})
	if err == nil || err.Error() != "lastfm: no key" {
		t.Errorf("err = %v, want «lastfm: нет ключа»", err)
	}
}

func TestToTracks(t *testing.T) {
	entries := []Entry{
		{
			Song: Song{
				ID: "1", Artist: "Yeat", Title: "Monëy so big", Album: "Up 2 Më", Year: "2021",
				ArtworkURL: "https://example.com/a/{w}x{h}bb.jpg", DurationMs: 191000,
			},
			Why:      "рейдж",
			Favorite: true,
		},
		{Song: Song{ID: "2", Artist: "B", Title: "T2"}},
	}
	got := toTracks(entries)
	if len(got) != 2 {
		t.Fatalf("len(toTracks) = %d, want 2", len(got))
	}
	tr := got[0]
	if tr.ID != "1" || tr.Artist != "Yeat" || tr.Title != "Monëy so big" || tr.Album != "Up 2 Më" || tr.Year != "2021" {
		t.Errorf("поля из Song не перенеслись: %+v", tr)
	}
	if tr.ArtworkURL != "https://example.com/a/{w}x{h}bb.jpg" || tr.DurationMs != 191000 {
		t.Errorf("ArtworkURL/DurationMs: %q %d", tr.ArtworkURL, tr.DurationMs)
	}
	if tr.Why != "рейдж" || !tr.Favorite {
		t.Errorf("Why/Favorite: %q %v", tr.Why, tr.Favorite)
	}
	if got[1].Favorite || got[1].Why != "" {
		t.Errorf("пустой entry: %+v", got[1])
	}
}

func TestToTrackReleased(t *testing.T) {
	tr := toTrack(Entry{Song: Song{ID: "1", Released: "2020-12-25"}})
	if tr.Released != "2020-12-25" {
		t.Errorf("Released = %q, ждём 2020-12-25", tr.Released)
	}
}

func TestTurnPortionNoteAndLater(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`),
		),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		],"note":"без крика"}`)),
		toolResp(
			call("c3", "ask_later", `{"question":"убрать хоррор?","options":[{"n":1,"label":"да"},{"n":2,"label":"нет"}],"default":2}`),
		),
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	p, err := e.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if p.Note != "без крика" {
		t.Errorf("Note = %q, want «без крика»", p.Note)
	}
	if p.Later != nil {
		t.Fatalf("ход 1: Later: %+v, want пусто", p.Later)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if p2.Later == nil || p2.Later.Question != "убрать хоррор?" {
		t.Fatalf("Later: %+v", p2.Later)
	}
	if len(p2.Later.Tracks) != 2 || p2.Later.Tracks[0].Artist != "A" || p2.Later.Tracks[1].Artist != "B" {
		t.Errorf("Later.Tracks: %+v, want треки 1 и 2 первой порции", p2.Later.Tracks)
	}
	if len(p2.Tracks) != 3 {
		t.Fatalf("Tracks: %d, want 3", len(p2.Tracks))
	}
	log := e.log.b.String()
	for _, want := range []string{"note: без крика", "later question: убрать хоррор? [да нет]"} {
		if !strings.Contains(log, want) {
			t.Errorf("лог не содержит %q:\n%s", want, log)
		}
	}
}

func TestEngineAnswerQuestion(t *testing.T) {
	e := flowEngine(t, &fakeChatter{})
	q := Question{Question: "дальше?", Options: []string{"дрим-поп", "шугейз"},
		Tracks: []Track{{Artist: "A", Title: "T1"}, {Artist: "B", Title: "T2"}}, Default: "1"}
	e.Answer(q, "1")
	h := e.history()
	if !strings.Contains(h, `to the question "дальше?" — дрим-поп (A — T1)`) {
		t.Errorf("history не содержит ответ:\n%s", h)
	}
	if !strings.Contains(e.log.b.String(), `## listener answer: "дальше?" → дрим-поп (A — T1)`) {
		t.Errorf("лог не содержит ответ владельца:\n%s", e.log.b.String())
	}
}

func TestTurnWarmupReplyAndSecondTurnAskUserRejected(t *testing.T) {
	ui := &fakeUI{answers: []string{"2"}}
	askUser := `{"questions":[{"question":"какая эпоха?","options":[{"artist":"A","title":"T1","label":"ранний"},{"artist":"B","title":"T2","label":"поздний"}],"default":1}]}`
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("u1", "ask_user", askUser)),
		toolResp(call("p1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("s1", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(call("u2", "ask_user", askUser)),
		toolResp(call("s2", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
		toolResp(call("s3", "submit_candidates", `{"tracks":[
			{"artist":"G","title":"T7","why":"w"},
			{"artist":"H","title":"T8","why":"w"},
			{"artist":"I","title":"T9","why":"w"}
		]}`)),
	}}
	e := newTestEngineUI(t, ch, ui)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if ui.asks != 1 {
		t.Errorf("UI.Ask вызван %d раз, want 1", ui.asks)
	}
	user := ch.got[3][1].Content
	if !strings.Contains(user, `to the question "какая эпоха?" — поздний (B — T2)`) {
		t.Errorf("ход 2: user не содержит ответ разогрева:\n%s", user)
	}
	wantErr := "error: ask_user is allowed only in the first turn of the session; ask a fork in the skips through ask_later"
	found := false
	for _, m := range ch.got[4] {
		if m.Role == "tool" && strings.Contains(m.Content, wantErr) {
			found = true
		}
	}
	if !found {
		t.Errorf("ход 2: ошибка ask_user не видна в следующем вызове:\n%+v", ch.got[4])
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	user = ch.got[5][1].Content
	if n := strings.Count(user, `to the question "какая эпоха?" — поздний (B — T2)`); n != 1 {
		t.Errorf("ход 3: ответ разогрева встречается %d раз, want 1:\n%s", n, user)
	}
}

func TestTurnHistoryListsAskedLaterQuestions(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`),
		),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(
			call("c3", "ask_later", `{"question":"убрать хоррор?","options":[{"n":1,"label":"да"},{"n":2,"label":"нет"}],"default":2}`),
		),
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
		toolResp(call("c5", "submit_candidates", `{"tracks":[
			{"artist":"G","title":"T7","why":"w"},
			{"artist":"H","title":"T8","why":"w"},
			{"artist":"I","title":"T9","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	user := ch.got[4][1].Content
	if !strings.Contains(user, `Questions already asked (do not repeat): "убрать хоррор?"`) {
		t.Errorf("ход 3: user не содержит заданный вопрос:\n%s", user)
	}
}

func TestTurnHistoryForAskLater(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(
			call("c3", "ask_later", `{"question":"дальше?","options":[{"n":1,"label":"дрим-поп"},{"n":2,"label":"шугейз"}],"default":1}`),
		),
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if p2.Later == nil || len(p2.Later.Tracks) != 2 || p2.Later.Tracks[0].Artist != "A" || p2.Later.Tracks[1].Artist != "B" {
		t.Fatalf("Later.Tracks: %+v, want треки 1 и 2 первой порции", p2.Later)
	}
}

func TestHistoryTrackOutcomeWithRatings(t *testing.T) {
	e := newTestEngine(t, &fakeChatter{})
	e.portions = [][]Track{{
		{ID: "1", Artist: "A", Title: "T1"},
		{ID: "2", Artist: "B", Title: "T2"},
		{ID: "3", Artist: "C", Title: "T3"},
	}}
	err := e.Record([]Outcome{
		{ID: "1", Artist: "A", Title: "T1", Kind: "played"},
		{ID: "1", Artist: "A", Title: "T1", Kind: "more"},
		{ID: "2", Artist: "B", Title: "T2", Kind: "less"},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	h := e.history()
	for _, want := range []string{
		"1. A — T1 — played through, ▲ more of this",
		"2. B — T2 — not listened, ▼ less of this",
		"3. C — T3 — not listened",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("history не содержит %q:\n%s", want, h)
		}
	}
}

func TestTurnTailAndPromptMentionRatings(t *testing.T) {
	ch := &fakeChatter{}
	flowEngine(t, ch)
	if user := ch.got[2][1].Content; !strings.Contains(user, "▲ is the main referent") {
		t.Errorf("хвост потока не содержит «▲ is the main referent»:\n%s", user)
	}
	if !strings.Contains(promptRules, "explicit ratings from the listener") {
		t.Error("promptRules не содержит «explicit ratings from the listener»")
	}
}

func TestTurnReplyClearsReserve(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"},
			{"artist":"D","title":"T4","why":"w"}
		]}`)),
		toolResp(call("c3", "submit_candidates", `{"tracks":[
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"},
			{"artist":"G","title":"T7","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "запрос"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	start := len(ch.got)
	p, err := e.Turn(context.Background(), "чето мимо")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if len(p.Tracks) != 3 || p.Tracks[0].Artist != "E" {
		t.Errorf("порция 2: %+v, want E, F, G", p.Tracks)
	}
	if user := ch.got[start][1].Content; strings.Contains(user, "Candidates from the previous turn") {
		t.Errorf("после реплики запас не сброшен:\n%s", user)
	}
}

func skipNudgeEngine(t *testing.T, ch *fakeChatter, outs ...Outcome) *Engine {
	t.Helper()
	ch.resps = append(ch.resps,
		toolResp(
			call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`),
			call("c2", "submit_candidates", `{"tracks":[
				{"artist":"A","title":"T1","why":"w"},
				{"artist":"B","title":"T2","why":"w"},
				{"artist":"C","title":"T3","why":"w"}
			]}`),
		),
	)
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if err := e.Record(outs); err != nil {
		t.Fatalf("Record: %v", err)
	}
	return e
}

func TestTurnNudgesAskLater(t *testing.T) {
	ch := &fakeChatter{}
	e := skipNudgeEngine(t, ch,
		Outcome{Artist: "A", Title: "T1", Kind: "skip_early", PosSec: 10},
		Outcome{Artist: "B", Title: "T2", Kind: "skip_mid", PosSec: 90},
	)
	ch.resps = append(ch.resps, toolResp(call("c3", "submit_candidates", `{"tracks":[
		{"artist":"D","title":"T4","why":"w"},
		{"artist":"E","title":"T5","why":"w"},
		{"artist":"F","title":"T6","why":"w"}
	]}`)))
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	user := ch.got[start][1].Content
	for _, want := range []string{"Skipped since the last question: A — T1; B — T2.", "call ask_later"} {
		if !strings.Contains(user, want) {
			t.Errorf("ход 2: user не содержит %q:\n%s", want, user)
		}
	}
}

func TestTurnNoNudgeOneSkip(t *testing.T) {
	ch := &fakeChatter{}
	e := skipNudgeEngine(t, ch, Outcome{Artist: "A", Title: "T1", Kind: "skip_early", PosSec: 10})
	ch.resps = append(ch.resps, toolResp(call("c3", "submit_candidates", `{"tracks":[
		{"artist":"D","title":"T4","why":"w"},
		{"artist":"E","title":"T5","why":"w"},
		{"artist":"F","title":"T6","why":"w"}
	]}`)))
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if user := ch.got[start][1].Content; strings.Contains(user, "Skipped since the last question") {
		t.Errorf("ход 2: подсказка при одном скипе:\n%s", user)
	}
}

func TestTurnNudgeResetsAfterAskLater(t *testing.T) {
	ch := &fakeChatter{}
	e := skipNudgeEngine(t, ch,
		Outcome{Artist: "A", Title: "T1", Kind: "skip_early", PosSec: 10},
		Outcome{Artist: "B", Title: "T2", Kind: "skip_mid", PosSec: 90},
	)
	ch.resps = append(ch.resps,
		toolResp(call("c3", "ask_later", `{"question":"дальше?","options":[{"n":1,"label":"дрим-поп"},{"n":2,"label":"шугейз"}],"default":1}`)),
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
		toolResp(call("c5", "submit_candidates", `{"tracks":[
			{"artist":"G","title":"T7","why":"w"},
			{"artist":"H","title":"T8","why":"w"},
			{"artist":"I","title":"T9","why":"w"}
		]}`)),
	)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	if user := ch.got[start][1].Content; strings.Contains(user, "Skipped since the last question") {
		t.Errorf("ход 3: подсказка не сброшена после ask_later:\n%s", user)
	}
}

func TestTurnNoNudgeOnReply(t *testing.T) {
	ch := &fakeChatter{}
	e := skipNudgeEngine(t, ch,
		Outcome{Artist: "A", Title: "T1", Kind: "skip_early", PosSec: 10},
		Outcome{Artist: "B", Title: "T2", Kind: "skip_mid", PosSec: 90},
	)
	ch.resps = append(ch.resps, toolResp(call("c3", "submit_candidates", `{"tracks":[
		{"artist":"D","title":"T4","why":"w"},
		{"artist":"E","title":"T5","why":"w"},
		{"artist":"F","title":"T6","why":"w"}
	]}`)))
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), "потише"); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if user := ch.got[start][1].Content; strings.Contains(user, "Skipped since the last question") {
		t.Errorf("ход 2: подсказка при реплике:\n%s", user)
	}
}

func TestTurnWritesNoServed(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход: %v", err)
	}
	for _, r := range e.sig.recs {
		if r.Kind == "served" {
			t.Fatalf("порция не должна писать served: %+v", r)
		}
	}
}

func TestTurnStartsEmptyPortion(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`),
			call("c2", "submit_candidates", `{"tracks":[
				{"artist":"A","title":"T1","why":"w"},
				{"artist":"B","title":"T2","why":"w"},
				{"artist":"C","title":"T3","why":"w"}
			]}`),
		),
		toolResp(call("c3", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	p1, err := e.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if len(p1.Tracks) != 3 {
		t.Fatalf("ход 1: %d треков, want 3", len(p1.Tracks))
	}
	v := e.session.Offer(Suggestion{Artist: "Z", Title: "TZ", Why: "w"}, Song{ID: "z", Artist: "Z", Title: "TZ"}, true)
	if !v.Accepted {
		t.Fatalf("Z: rejected: %s", v.Reason)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if len(p2.Tracks) != 3 {
		t.Fatalf("ход 2: %d треков, want 3", len(p2.Tracks))
	}
	for i, want := range []string{"D", "E", "F"} {
		if p2.Tracks[i].Artist != want {
			t.Errorf("ход 2, трек %d: %q, want %q", i, p2.Tracks[i].Artist, want)
		}
	}
	for _, tr := range p2.Tracks {
		if tr.Artist == "Z" {
			t.Errorf("ход 2 содержит Z: %+v", p2.Tracks)
		}
	}
}

func TestEngineDropLast(t *testing.T) {
	ch := &fakeChatter{}
	e := flowEngine(t, ch)
	ch.resps = append(ch.resps, toolResp(call("c4", "submit_candidates", `{"tracks":[
		{"artist":"G","title":"T7","why":"w"},
		{"artist":"H","title":"T8","why":"w"},
		{"artist":"I","title":"T9","why":"w"}
	]}`)))
	e.DropLast()
	p3, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	if len(p3.Tracks) != 3 {
		t.Fatalf("ход 3: %d треков, want 3", len(p3.Tracks))
	}
	user := ch.got[3][1].Content
	if !strings.Contains(user, "A — T1") {
		t.Errorf("user не содержит трек порции 1:\n%s", user)
	}
	for _, gone := range []string{"D — T4", "E — T5", "F — T6"} {
		if strings.Contains(user, gone) {
			t.Errorf("user содержит трек отброшенной порции %q:\n%s", gone, user)
		}
	}
	if !strings.Contains(e.log.b.String(), "## portion 2 dropped: listener reply") {
		t.Errorf("дорог нет:\n%s", e.log.b.String())
	}
}

func TestEngineDropLastForgetsLater(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(call("c3", "ask_later", `{"question":"убрать хоррор?","options":[{"n":1,"label":"да"},{"n":2,"label":"нет"}],"default":2}`)),
		toolResp(call("c4", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
		toolResp(call("c5", "submit_candidates", `{"tracks":[
			{"artist":"G","title":"T7","why":"w"},
			{"artist":"H","title":"T8","why":"w"},
			{"artist":"I","title":"T9","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	e.DropLast()
	if _, err := e.Turn(context.Background(), "потише"); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	if user := ch.got[4][1].Content; strings.Contains(user, "убрать хоррор?") {
		t.Errorf("ход 3: вопрос отброшенной порции остался в заданных:\n%s", user)
	}
}
