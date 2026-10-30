package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
)

var cyrillic = regexp.MustCompile(`\p{Cyrillic}`)

type fakeChatter struct {
	resps []llm.Response
	got   [][]llm.Message
}

func (f *fakeChatter) Chat(ctx context.Context, msgs []llm.Message, tools []llm.Tool, jsonMode bool) (llm.Response, error) {
	f.got = append(f.got, append([]llm.Message(nil), msgs...))
	if len(f.resps) == 0 {
		return llm.Response{}, errors.New("фейк: ответы кончились")
	}
	r := f.resps[0]
	f.resps = f.resps[1:]
	return r, nil
}

func textResp(s string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", Content: s}}
}

func toolResp(calls ...llm.ToolCall) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", ToolCalls: calls}}
}

type fakeBackend struct {
	mu            sync.Mutex
	describeCalls []string
	findCalls     []string
}

func (f *fakeBackend) describe(ctx context.Context, kind, artist, name string) (any, error) {
	f.describeCalls = append(f.describeCalls, kind+"|"+artist+"|"+name)
	return map[string]string{"kind": kind, "artist": artist, "name": name}, nil
}

func (f *fakeBackend) similarArtists(ctx context.Context, artist string) ([]similar, error) {
	return []similar{{Name: "Neighbor", Match: 0.5}}, nil
}

func (f *fakeBackend) findSong(ctx context.Context, sug Suggestion) (Song, bool, error) {
	f.mu.Lock()
	f.findCalls = append(f.findCalls, sug.Artist+"|"+sug.Title)
	f.mu.Unlock()
	if sug.Artist == "Ghost" {
		return Song{}, false, nil
	}
	return Song{ID: sug.Artist + "/" + sug.Title, Artist: sug.Artist, Title: sug.Title, Album: "Album", Year: "2024",
		ArtworkURL: "https://example.com/" + sug.Artist + ".jpg"}, true, nil
}

type fakeUI struct {
	answers  []string
	asks     int
	asked    [][]Question
	progress []string
	picks    []Pick
}

func (f *fakeUI) Ask(qs []Question) []string {
	f.asks++
	f.asked = append(f.asked, qs)
	return f.answers
}
func (f *fakeUI) Progress(line string) { f.progress = append(f.progress, line) }
func (f *fakeUI) Picked(p Pick)        { f.picks = append(f.picks, p) }

func newTestAgent(t *testing.T, ch chatter, be backend, ui UI) *Agent {
	t.Helper()
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	return NewAgent(ch, be, NewSession(nil, nil, 3), st, ui, nil, nil, "en")
}

func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Arguments: args}
}

func TestTurnPlanDescribeSubmit(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("c1", "set_plan", `{"seeds":[{"artist":"Don Toliver","mode":"not_it"}],"exclusions":[]}`),
			call("c2", "describe", `{"kind":"album","artist":"Don Toliver","name":"HARDSTONE PSYCHO"}`),
		),
		toolResp(call("c3", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w1"},
			{"artist":"B","title":"T2","why":"w2"},
			{"artist":"C","title":"T3","why":"w3"}
		]}`)),
	}}
	be := &fakeBackend{}
	a := newTestAgent(t, ch, be, &fakeUI{})

	entries, err := a.Turn(context.Background(), "что-то в духе Don Toliver")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: %d, want 3", len(entries))
	}
	for i, want := range []string{"A", "B", "C"} {
		if entries[i].Song.Artist != want {
			t.Errorf("entries[%d].Artist = %q, want %q", i, entries[i].Song.Artist, want)
		}
	}
	if len(ch.got) != 2 {
		t.Fatalf("Chat вызван %d раз, want 2", len(ch.got))
	}
	tail := ch.got[1][len(ch.got[1])-2:]
	if len(tail) != 2 || tail[0].Role != "tool" || tail[1].Role != "tool" ||
		tail[0].ToolCallID != "c1" || tail[1].ToolCallID != "c2" {
		t.Fatalf("второй вызов должен заканчиваться tool-сообщениями c1 и c2, got %+v", tail)
	}
	if ch.got[0][0].Role != "system" {
		t.Errorf("первое сообщение не system: %q", ch.got[0][0].Role)
	}
	found := false
	for _, d := range be.describeCalls {
		if d == "album|Don Toliver|HARDSTONE PSYCHO" {
			found = true
		}
	}
	if !found {
		t.Errorf("describe не получил album/Don Toliver/HARDSTONE PSYCHO: %v", be.describeCalls)
	}
}

func TestSubmitBeforePlan(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	res := a.dispatch(context.Background(), call("x", "submit_candidates",
		`{"tracks":[{"artist":"A","title":"T","why":"w"}]}`))
	if !strings.Contains(res, "call set_plan first") {
		t.Errorf("got %q, want подстрока «сначала set_plan»", res)
	}
}

func TestTurnTextOnlyTwoHints(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{textResp("думаю"), textResp("ещё"), textResp("всё")}}
	a := newTestAgent(t, ch, &fakeBackend{}, &fakeUI{})

	entries, err := a.Turn(context.Background(), "запрос")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries: %d, want 0", len(entries))
	}
	if len(ch.got) != 3 {
		t.Fatalf("Chat вызван %d раз, want 3", len(ch.got))
	}
	hints := 0
	for _, m := range ch.got[2] {
		if m.Role == "user" && strings.HasPrefix(m.Content, "Portion is not full") {
			hints++
		}
	}
	if hints != 2 {
		t.Errorf("подсказок в msgs: %d, want 2", hints)
	}
}

func TestAskUserOnceAndResolveAnswer(t *testing.T) {
	ui := &fakeUI{answers: []string{"", "2"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[
		{"question":"q1","options":[{"artist":"A","title":"T1","label":"o1"},{"artist":"A","title":"T2","label":"o2"}],"default":1},
		{"question":"q2","options":[{"artist":"X","title":"T1","label":"x1"},{"artist":"X","title":"T2","label":"x2"}],"default":1}
	]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	var out struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	want := []answerRow{{Question: "q1", Answer: "o1 (A — T1)"}, {Question: "q2", Answer: "x2 (X — T2)"}}
	if len(out.Answers) != 2 || out.Answers[0] != want[0] || out.Answers[1] != want[1] {
		t.Errorf("answers: %+v, want %+v", out.Answers, want)
	}
	if len(out.Taken) != 2 || out.Taken[0] != "A — T1" || out.Taken[1] != "X — T2" {
		t.Errorf("taken: %v, want [A — T1 X — T2]", out.Taken)
	}
	res2 := a.dispatch(context.Background(), call("q2", "ask_user", args))
	if !strings.Contains(res2, "questions were already asked") {
		t.Errorf("повтор: got %q", res2)
	}
	q := Question{Question: "q", Options: []string{"o1", "o2"},
		Tracks: []Track{{Artist: "A", Title: "T1"}, {Artist: "B", Title: "T2"}}, Default: "1"}
	if got := resolveAnswer(q, ""); got != "o1 (A — T1)" {
		t.Errorf("resolveAnswer(пусто) = %q", got)
	}
	if got := resolveAnswer(q, "2"); got != "o2 (B — T2)" {
		t.Errorf("resolveAnswer(2) = %q", got)
	}
	if got := resolveAnswer(q, "своё"); got != "my own pick: своё" {
		t.Errorf("resolveAnswer(своё) = %q", got)
	}
	if got := resolveAnswer(q, "9"); got != "my own pick: 9" {
		t.Errorf("resolveAnswer(9) = %q", got)
	}
	if got := resolveAnswer(q, " 1 "); got != "o1 (A — T1)" {
		t.Errorf("resolveAnswer(« 1 ») = %q", got)
	}
}

func TestResolveAnswerNone(t *testing.T) {
	q := Question{Question: "q", Options: []string{"o1", "o2"},
		Tracks: []Track{{Artist: "A", Title: "T1"}, {Artist: "B", Title: "T2"}}, Default: "1"}
	if got := resolveAnswer(q, " none "); got != "missed: no option fits" {
		t.Errorf("resolveAnswer(« none ») = %q", got)
	}
	if got := pickedOptions(q, "none"); len(got) != 0 {
		t.Errorf("pickedOptions(none) = %v, want пусто", got)
	}
	if got := resolveAnswer(q, ""); got != "o1 (A — T1)" {
		t.Errorf("resolveAnswer(пусто) = %q, want вариант по умолчанию", got)
	}
}

func TestMemoryAddDuplicate(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	args := `{"text":"не нравится Gunna","evidence":"«Gunna нет»"}`
	res := a.dispatch(context.Background(), call("m1", "memory_add", args))
	if !strings.HasPrefix(res, "ok: memory") {
		t.Errorf("первый add: got %q", res)
	}
	res2 := a.dispatch(context.Background(), call("m2", "memory_add", args))
	if !strings.HasPrefix(res2, "error: a similar entry already exists") {
		t.Errorf("повтор: got %q", res2)
	}
}

func TestTurnStopsAtMaxSteps(t *testing.T) {
	describe := toolResp(call("d", "describe", `{"kind":"artist","artist":"Yeat"}`))
	ch := &fakeChatter{resps: []llm.Response{describe, describe, describe, describe}}
	be := &fakeBackend{}
	a := newTestAgent(t, ch, be, &fakeUI{})
	a.MaxSteps = 4

	entries, err := a.Turn(context.Background(), "запрос")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries: %d, want 0", len(entries))
	}
	if len(ch.got) != 4 {
		t.Fatalf("Chat вызван %d раз, want MaxSteps=4", len(ch.got))
	}
}

func TestDispatchUnknownToolAndBadArgs(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	if res := a.dispatch(context.Background(), call("u", "foo", `{}`)); res != "error: no such tool foo" {
		t.Errorf("неизвестный инструмент: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("u2", "set_plan", `{`)); res != "error: arguments are not JSON" {
		t.Errorf("плохие аргументы: got %q", res)
	}
}

func TestSubmitAcceptedRejectedOrder(t *testing.T) {
	be := &fakeBackend{}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %q", res)
	}
	res := a.dispatch(context.Background(), call("s", "submit_candidates",
		`{"tracks":[{"artist":"Ghost","title":"X","why":"w"},{"artist":"A","title":"Y","why":"w"}]}`))
	var out struct {
		Accepted []string `json:"accepted"`
		Rejected []struct {
			Track  string `json:"track"`
			Reason string `json:"reason"`
		} `json:"rejected"`
		Portion string `json:"portion"`
		Need    int    `json:"need"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Accepted) != 1 || out.Accepted[0] != "A — Y" {
		t.Errorf("accepted: %v, want [A — Y]", out.Accepted)
	}
	if len(out.Rejected) != 1 || out.Rejected[0].Track != "Ghost — X" || out.Rejected[0].Reason != "not in Apple" {
		t.Errorf("rejected: %+v", out.Rejected)
	}
	if out.Portion != "1/3" || out.Need != 2 {
		t.Errorf("portion=%q need=%d, want 1/3 и 2", out.Portion, out.Need)
	}
	if len(be.findCalls) != 2 {
		t.Errorf("findSong вызван %d раз, want 2", len(be.findCalls))
	}
}

func TestSystemPromptSections(t *testing.T) {
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	sp := systemPrompt(st, []Song{{Artist: "Yeat", Title: "bigger thën everything", Album: "bigger thën everything"}}, "en")
	for _, want := range []string{
		"You are the music picker for AUX",
		"## Memory about the listener",
		"- Yeat — bigger thën everything (bigger thën everything)",
		"in English.",
	} {
		if !strings.Contains(sp, want) {
			t.Errorf("systemPrompt не содержит %q", want)
		}
	}
	if before, _, ok := strings.Cut(sp, "## Memory about the listener"); ok {
		if cyrillic.MatchString(before) {
			t.Errorf("кириллица в systemPrompt до секции памяти:\n%s", before)
		}
	}
	ru := systemPrompt(st, nil, "ru")
	if !strings.Contains(ru, "in Russian.") {
		t.Error("systemPrompt с lang ru не содержит «in Russian.»")
	}
}

func TestPromptRulesNoCyrillic(t *testing.T) {
	if m := cyrillic.FindString(promptRules); m != "" {
		t.Errorf("promptRules содержит кириллицу: %q", m)
	}
}

func TestToolResultsNoCyrillic(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	ctx := context.Background()
	for _, c := range []llm.ToolCall{
		call("u1", "foo", `{}`),
		call("u2", "set_plan", `{`),
		call("u3", "submit_candidates", `{"tracks":[],"note":""}`),
		call("u4", "describe", `{"kind":"song","artist":"X"}`),
		call("u5", "ask_later", `{"question":"q","options":[{"n":1,"label":"l"}],"default":1}`),
		call("u6", "memory_add", `{"text":"","evidence":"e"}`),
		call("u7", "ask_user", `{"questions":[]}`),
		call("u8", "set_plan", `{"seeds":[{"artist":"X","mode":"bad"}],"exclusions":[]}`),
	} {
		if res := a.dispatch(ctx, c); cyrillic.MatchString(res) {
			t.Errorf("%s: кириллица в результате: %q", c.Name, res)
		}
	}
	if res := a.dispatch(ctx, call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); cyrillic.MatchString(res) {
		t.Errorf("set_plan: кириллица в результате: %q", res)
	}
	if res := a.dispatch(ctx, call("s", "submit_candidates", `{"tracks":[{"artist":"Ghost","title":"T","why":"w"}],"note":""}`)); cyrillic.MatchString(res) {
		t.Errorf("submit_candidates: кириллица в результате: %q", res)
	}
}

func TestAgentToolsNineValidSchemas(t *testing.T) {
	tools := agentTools()
	want := []string{"ask_user", "ask_later", "set_plan", "describe", "similar_artists", "submit_candidates",
		"memory_add", "memory_replace", "memory_remove", "memory_consolidate"}
	if len(tools) != len(want) {
		t.Fatalf("инструментов %d, want %d", len(tools), len(want))
	}
	for i, tool := range tools {
		if tool.Name != want[i] {
			t.Errorf("инструмент %d: %q, want %q", i, tool.Name, want[i])
		}
		var m map[string]any
		if err := json.Unmarshal(tool.Parameters, &m); err != nil {
			t.Fatalf("%s: параметры не JSON: %v", tool.Name, err)
		}
		if m["type"] != "object" {
			t.Errorf("%s: type = %v, want object", tool.Name, m["type"])
		}
	}
}

func TestDescribeValidation(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	if res := a.dispatch(context.Background(), call("d1", "describe", `{"kind":"song","artist":"X"}`)); !strings.Contains(res, "error: unknown kind") {
		t.Errorf("kind song: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("d2", "describe", `{"kind":"album","artist":"X"}`)); res != "error: name is required" {
		t.Errorf("album без name: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("d3", "describe", `{"kind":"artist","artist":"X"}`)); strings.HasPrefix(res, "error") {
		t.Errorf("artist без name: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("d4", "describe", `{"kind":"artist"}`)); res != "error: artist is required" {
		t.Errorf("artist без artist и name: got %q", res)
	}
}

func TestDescribeArtistFromName(t *testing.T) {
	be := &fakeBackend{}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	if res := a.dispatch(context.Background(), call("d", "describe", `{"kind":"artist","name":"Westside Gunn"}`)); strings.HasPrefix(res, "error") {
		t.Fatalf("artist в name: got %q", res)
	}
	if len(be.describeCalls) != 1 || be.describeCalls[0] != "artist|Westside Gunn|" {
		t.Errorf("describeCalls = %v, want [artist|Westside Gunn|]", be.describeCalls)
	}
}

func TestSimilarArtistsCappedAt20(t *testing.T) {
	be := &fakeBackend{}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	res := a.dispatch(context.Background(), call("s", "similar_artists", `{"artist":"Yeat"}`))
	var got []similarOut
	if err := json.Unmarshal([]byte(res), &got); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(got) != 1 || got[0].Name != "Neighbor" || got[0].Match != 50 {
		t.Errorf("similar: %+v, want [{Neighbor 50}]", got)
	}
}

func TestTurnDispatchesAllCallsEvenWhenFull(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(
			call("s1", "submit_candidates", `{"tracks":[
				{"artist":"A","title":"T1","why":"w"},
				{"artist":"B","title":"T2","why":"w"},
				{"artist":"C","title":"T3","why":"w"}
			]}`),
			call("d1", "describe", `{"kind":"artist","artist":"Yeat"}`),
		),
	}}
	ui := &fakeUI{}
	a := newTestAgent(t, ch, &fakeBackend{}, ui)

	entries, err := a.Turn(context.Background(), "запрос")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: %d, want 3", len(entries))
	}
	if len(ch.got) != 2 {
		t.Fatalf("Chat вызван %d раз, want 2 — порция заполнилась на первом вызове шага 2", len(ch.got))
	}
	toolMsgs := 0
	for _, m := range a.msgs {
		if m.Role == "tool" {
			toolMsgs++
		}
	}
	if toolMsgs != 3 {
		t.Errorf("tool-сообщений %d, want 3 — каждый вызов должен получить ответ", toolMsgs)
	}
	want := "· submit 3 → accepted 3, rejected 0 (portion 3/3)"
	found := false
	for _, p := range ui.progress {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Errorf("progress: %v, want %q", ui.progress, want)
	}
}

func TestTurnOffersReserve(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("p1", "set_plan", `{"seeds":[],"exclusions":[]}`),
			call("s1", "submit_candidates", `{"tracks":[
				{"artist":"A","title":"T1","why":"w"},
				{"artist":"B","title":"T2","why":"w"},
				{"artist":"C","title":"T3","why":"w"}
			]}`),
		),
		toolResp(call("s2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"}
		]}`)),
		toolResp(call("s3", "submit_candidates", `{"tracks":[
			{"artist":"C","title":"T3","why":"w"},
			{"artist":"D","title":"T4","why":"w"}
		]}`)),
		toolResp(call("s4", "submit_candidates", `{"tracks":[
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	}}
	be := &fakeBackend{}
	ui := &fakeUI{}
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	a := NewAgent(ch, be, NewSession(nil, nil, 2), st, ui, nil, nil, "en")

	entries, err := a.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if len(entries) != 2 || entries[0].Song.Artist != "A" || entries[1].Song.Artist != "B" {
		t.Fatalf("ход 1: %+v, want [A, B]", entries)
	}

	start := len(ch.got)
	entries2, err := a.Turn(context.Background(), "ещё")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if len(entries2) != 2 || entries2[0].Song.Artist != "C" || entries2[1].Song.Artist != "D" {
		t.Fatalf("ход 2: %+v, want [C, D]", entries2)
	}
	user := ch.got[start][len(ch.got[start])-1]
	if user.Role != "user" {
		t.Fatalf("ход 2: последнее сообщение %q, want user", user.Role)
	}
	if !strings.Contains(user.Content, "Candidates from the previous turn that did not make it into the portion: C — T3.") {
		t.Errorf("ход 2: user не содержит список запаса:\n%s", user.Content)
	}
	empty := false
	for _, msgs := range ch.got[start:] {
		for _, m := range msgs {
			if m.Role == "tool" && strings.Contains(m.Content, `"portion":"0/2"`) {
				empty = true
			}
		}
	}
	if !empty {
		t.Errorf("ход 2: запас попал в порцию до submit_candidates:\n%+v", ch.got[start:])
	}

	start = len(ch.got)
	if _, err := a.Turn(context.Background(), "ещё"); err != nil {
		t.Fatalf("ход 3: %v", err)
	}
	if user := ch.got[start][len(ch.got[start])-1].Content; strings.Contains(user, "Candidates from the previous turn") {
		t.Errorf("ход 3: список запаса повторился:\n%s", user)
	}
}

func TestTurnStopsAtSlack(t *testing.T) {
	tracks := make([]string, 0, 18)
	for i := range 18 {
		tracks = append(tracks, fmt.Sprintf(`{"artist":"A%d","title":"T%d","why":"w"}`, i, i))
	}
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)),
		toolResp(call("s", "submit_candidates", `{"tracks":[`+strings.Join(tracks, ",")+`]}`)),
	}}
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	a := NewAgent(ch, &fakeBackend{}, NewSession(nil, nil, 20), st, &fakeUI{}, nil, nil, "en")

	entries, err := a.Turn(context.Background(), "запрос")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if len(entries) != 18 {
		t.Fatalf("entries: %d, want 18", len(entries))
	}
	if len(ch.got) != 2 {
		t.Fatalf("Chat вызван %d раз, want 2", len(ch.got))
	}
	for _, msgs := range ch.got {
		for _, m := range msgs {
			if m.Role == "user" && strings.HasPrefix(m.Content, "Portion is not full") {
				t.Error("есть подсказка «Порция не заполнена», ход должен закончиться на недоборе ≤10%")
			}
		}
	}
}

func TestRunLogStepsAndCalls(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("d", "describe", `{"kind":"artist","artist":"Yeat"}`)),
		textResp("готово"), textResp("готово"), textResp("готово"),
	}}
	log := &runLog{}
	a := newTestAgent(t, ch, &fakeBackend{}, &fakeUI{})
	a.Log = log
	if _, err := a.Turn(context.Background(), "запрос"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	out := log.b.String()
	for _, want := range []string{"### step 1 (", "### step 2 (", "готово", "→ describe {\"kind\":\"artist\",\"artist\":\"Yeat\"}", "← {"} {
		if !strings.Contains(out, want) {
			t.Errorf("лог не содержит %q:\n%s", want, out)
		}
	}
}

func TestSubmitOverLimitAndBadMode(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %q", res)
	}
	var tracks strings.Builder
	tracks.WriteString(`{"tracks":[`)
	for i := range 31 {
		if i > 0 {
			tracks.WriteString(",")
		}
		_, _ = fmt.Fprintf(&tracks, `{"artist":"A%d","title":"T%d","why":"w"}`, i, i)
	}
	tracks.WriteString(`]}`)
	res := a.dispatch(context.Background(), call("s", "submit_candidates", tracks.String()))
	if res != "error: 1–30 tracks" {
		t.Errorf("31 трек: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("p2", "set_plan", `{"seeds":[{"artist":"X","mode":"bad"}],"exclusions":[]}`)); res != "error: unknown mode bad" {
		t.Errorf("mode: got %q", res)
	}
}

func TestAskUserTracksPick(t *testing.T) {
	ui := &fakeUI{answers: []string{"2"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"куда ведём?","options":[
		{"artist":"A","title":"T1","label":"тёплый дом"},
		{"artist":"B","title":"T2","label":"ночной драйв"},
		{"artist":"C","title":"T3","label":"дрим-поп"},
		{"artist":"D","title":"T4","label":"шугейз"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	if ui.asks != 1 {
		t.Fatalf("UI.Ask вызван %d раз, want 1", ui.asks)
	}
	if len(ui.asked) != 1 || len(ui.asked[0]) != 1 {
		t.Fatalf("asked: %+v", ui.asked)
	}
	q := ui.asked[0][0]
	if len(q.Tracks) != 4 || len(q.Options) != 4 {
		t.Fatalf("вопрос: %d треков, %d вариантов, want по 4", len(q.Tracks), len(q.Options))
	}
	if q.Tracks[1].ID != "B/T2" || q.Tracks[1].ArtworkURL != "https://example.com/B.jpg" || q.Tracks[1].Artist != "B" || q.Tracks[1].Title != "T2" {
		t.Errorf("Tracks[1]: %+v", q.Tracks[1])
	}
	if q.Options[2] != "дрим-поп" || q.Default != "1" {
		t.Errorf("Options/Default: %v %q", q.Options, q.Default)
	}
	portion := a.Session.Portion()
	if len(portion) != 1 || portion[0].Song.Artist != "B" || portion[0].Song.Title != "T2" {
		t.Fatalf("порция: %+v, want ровно B — T2", portion)
	}
	if len(ui.picks) != 1 || !ui.picks[0].Accepted || ui.picks[0].Name != "B — T2" || ui.picks[0].Track.ID != "B/T2" {
		t.Fatalf("picks: %+v", ui.picks)
	}
	var out struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
		Portion string      `json:"portion"`
		Need    int         `json:"need"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Taken) != 1 || out.Taken[0] != "B — T2" {
		t.Errorf("taken: %v, want [B — T2]", out.Taken)
	}
	if out.Portion != "1/3" || out.Need != 2 {
		t.Errorf("portion=%q need=%d, want 1/3 и 2", out.Portion, out.Need)
	}
	if len(out.Answers) != 1 || out.Answers[0].Answer != "ночной драйв (B — T2)" {
		t.Errorf("answers: %+v", out.Answers)
	}
}

func TestAskUserTracksDefault(t *testing.T) {
	ui := &fakeUI{answers: []string{""}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"},
		{"artist":"C","title":"T3","label":"o3"}
	],"default":3}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	portion := a.Session.Portion()
	if len(portion) != 1 || portion[0].Song.Artist != "C" || portion[0].Song.Title != "T3" {
		t.Fatalf("порция: %+v, want ровно C — T3", portion)
	}
	var out struct {
		Taken []string `json:"taken"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Taken) != 1 || out.Taken[0] != "C — T3" {
		t.Errorf("taken: %v, want [C — T3]", out.Taken)
	}
}

func TestAskUserTracksAll(t *testing.T) {
	ui := &fakeUI{answers: []string{"all"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"L1"},
		{"artist":"B","title":"T2","label":"L2"},
		{"artist":"C","title":"T3","label":"L3"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	portion := a.Session.Portion()
	if len(portion) != 3 {
		t.Fatalf("порция: %d треков, want 3", len(portion))
	}
	for i, want := range []string{"A", "B", "C"} {
		if portion[i].Song.Artist != want {
			t.Errorf("порция[%d]: %q, want %q", i, portion[i].Song.Artist, want)
		}
	}
	var out struct {
		Answers []answerRow `json:"answers"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	want := "all together: A — T1; B — T2; C — T3"
	if len(out.Answers) != 1 || !strings.HasPrefix(out.Answers[0].Answer, want) {
		t.Errorf("answer: %q, want префикс %q", out.Answers[0].Answer, want)
	}
}

func TestAskUserTracksOwnText(t *testing.T) {
	ui := &fakeUI{answers: []string{"потише"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	if n := len(a.Session.Portion()); n != 0 {
		t.Fatalf("порция: %d треков, want 0", n)
	}
	if len(ui.picks) != 0 {
		t.Errorf("picks: %+v, want пусто", ui.picks)
	}
	var out struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Taken) != 0 {
		t.Errorf("taken: %v, want пусто", out.Taken)
	}
	if len(out.Answers) != 1 || out.Answers[0].Answer != "my own pick: потише" {
		t.Errorf("answers: %+v", out.Answers)
	}
}

func TestAskUserMissReasks(t *testing.T) {
	ui := &fakeUI{answers: []string{"none"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	var out struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
		Next    string      `json:"next"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Answers) != 1 || out.Answers[0].Answer != "missed: no option fits" {
		t.Errorf("answers: %+v", out.Answers)
	}
	if len(out.Taken) != 0 {
		t.Errorf("taken: %v, want пусто", out.Taken)
	}
	if !strings.HasPrefix(out.Next, "The listener answered \"missed\"") {
		t.Errorf("next = %q", out.Next)
	}
	args2 := `{"questions":[{"question":"q2","options":[
		{"artist":"C","title":"T3","label":"o3"},
		{"artist":"D","title":"T4","label":"o4"}
	],"default":1}]}`
	if res2 := a.dispatch(context.Background(), call("q2", "ask_user", args2)); strings.Contains(res2, "questions were already asked in this turn") {
		t.Errorf("второй ask_user отклонён: %s", res2)
	}
	if len(a.Warmup) != 2 || a.Warmup[0].Question != "q" || a.Warmup[1].Question != "q2" {
		t.Errorf("Warmup: %+v, want ответы на обе карточки", a.Warmup)
	}
}

func TestAskUserSecondMissStops(t *testing.T) {
	ui := &fakeUI{answers: []string{"none"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	first := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	if res := a.dispatch(context.Background(), call("q", "ask_user", first)); strings.HasPrefix(res, "error") {
		t.Fatalf("первый ask_user: %s", res)
	}
	second := `{"questions":[{"question":"q2","options":[
		{"artist":"C","title":"T3","label":"o3"},
		{"artist":"D","title":"T4","label":"o4"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q2", "ask_user", second))
	var out struct {
		Next string `json:"next"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if !strings.HasPrefix(out.Next, "The listener answered \"missed\" again") {
		t.Errorf("next = %q", out.Next)
	}
	if res3 := a.dispatch(context.Background(), call("q3", "ask_user", first)); !strings.Contains(res3, "error: questions were already asked in this turn") {
		t.Errorf("третий ask_user: got %q", res3)
	}
}

func TestAskUserPickHasNoNext(t *testing.T) {
	ui := &fakeUI{answers: []string{"2"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(res), &fields); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if _, ok := fields["next"]; ok {
		t.Errorf("в ответе есть next: %s", res)
	}
	if res2 := a.dispatch(context.Background(), call("q2", "ask_user", args)); !strings.Contains(res2, "error: questions were already asked in this turn") {
		t.Errorf("повтор ask_user: got %q", res2)
	}

	own := &fakeUI{answers: []string{"потише"}}
	b := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, own)
	res = b.dispatch(context.Background(), call("q", "ask_user", args))
	var out struct {
		Answers []answerRow `json:"answers"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if len(out.Answers) != 1 || out.Answers[0].Answer != "my own pick: потише" {
		t.Errorf("answers: %+v", out.Answers)
	}
	if strings.Contains(res, `"next"`) {
		t.Errorf("в ответе на свой текст есть next: %s", res)
	}
}

func TestAskUserMixedMissNoReask(t *testing.T) {
	ui := &fakeUI{answers: []string{"none", "1"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[
		{"question":"q1","options":[{"artist":"A","title":"T1","label":"o1"},{"artist":"A","title":"T2","label":"o2"}],"default":1},
		{"question":"q2","options":[{"artist":"X","title":"T1","label":"x1"},{"artist":"X","title":"T2","label":"x2"}],"default":1}
	]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	var out struct {
		Answers []answerRow `json:"answers"`
		Taken   []string    `json:"taken"`
	}
	if err := json.Unmarshal([]byte(res), &out); err != nil {
		t.Fatalf("результат не JSON: %v\n%s", err, res)
	}
	if strings.Contains(res, `"next"`) {
		t.Errorf("в ответе есть next: %s", res)
	}
	if len(out.Answers) != 2 || !strings.HasPrefix(out.Answers[0].Answer, "missed") {
		t.Errorf("answers: %+v", out.Answers)
	}
	if len(out.Taken) != 1 || out.Taken[0] != "X — T1" {
		t.Errorf("taken: %v, want [X — T1]", out.Taken)
	}
}

func TestAskUserNotFound(t *testing.T) {
	ui := &fakeUI{answers: []string{"1"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[
		{"artist":"Ghost","title":"X","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if !strings.Contains(res, "Ghost — X") {
		t.Errorf("ошибка без Ghost — X: %q", res)
	}
	if !strings.Contains(res, "not found in Apple Music") {
		t.Errorf("ошибка без «не нашлись»: %q", res)
	}
	if ui.asks != 0 {
		t.Errorf("UI.Ask вызван %d раз, want 0", ui.asks)
	}
	fixed := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res2 := a.dispatch(context.Background(), call("q2", "ask_user", fixed))
	if strings.HasPrefix(res2, "error") {
		t.Fatalf("повторный ask_user: %s", res2)
	}
	if ui.asks != 1 {
		t.Errorf("UI.Ask вызван %d раз, want 1", ui.asks)
	}
}

func TestAskUserShapeValidation(t *testing.T) {
	cases := []struct {
		name string
		args string
	}{
		{"один вариант", `{"questions":[{"question":"q","options":[{"artist":"A","title":"T","label":"l"}],"default":1}]}`},
		{"пять вариантов", `{"questions":[{"question":"q","options":[
			{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"},
			{"artist":"C","title":"T3","label":"l"},{"artist":"D","title":"T4","label":"l"},
			{"artist":"E","title":"T5","label":"l"}],"default":1}]}`},
		{"три вопроса", `{"questions":[
			{"question":"q1","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":1},
			{"question":"q2","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":1},
			{"question":"q3","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":1}
		]}`},
		{"default 0", `{"questions":[{"question":"q","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":0}]}`},
		{"default за диапазоном", `{"questions":[{"question":"q","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":3}]}`},
		{"пустой label", `{"questions":[{"question":"q","options":[{"artist":"A","title":"T1","label":""},{"artist":"B","title":"T2","label":"l"}],"default":1}]}`},
		{"пустой вопрос", `{"questions":[{"question":"","options":[{"artist":"A","title":"T1","label":"l"},{"artist":"B","title":"T2","label":"l"}],"default":1}]}`},
	}
	for _, tc := range cases {
		a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
		res := a.dispatch(context.Background(), call("q", "ask_user", tc.args))
		if res != askUserErr {
			t.Errorf("%s: got %q, want %q", tc.name, res, askUserErr)
		}
	}
}

func TestAskUserTakesLibraryTrack(t *testing.T) {
	ui := &fakeUI{answers: []string{"1"}}
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	lib := []Song{{ID: "lib1", Artist: "A", Title: "T1", Album: "Album"}}
	a := NewAgent(&fakeChatter{}, &fakeBackend{}, NewSession(lib, nil, 3), st, ui, nil, nil, "en")
	args := `{"questions":[{"question":"q","options":[
		{"artist":"A","title":"T1","label":"o1"},
		{"artist":"B","title":"T2","label":"o2"}
	],"default":1}]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	if strings.HasPrefix(res, "error") {
		t.Fatalf("ask_user: %s", res)
	}
	portion := a.Session.Portion()
	if len(portion) != 1 || portion[0].Song.Artist != "A" || portion[0].Song.Title != "T1" {
		t.Fatalf("порция: %+v, want библиотечный A — T1", portion)
	}
	if portion[0].Favorite {
		t.Errorf("трек не из Favorites, Favorite = true")
	}
}

func TestSubmitCandidatesPickedEvents(t *testing.T) {
	ui := &fakeUI{}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "plan accepted") {
		t.Fatalf("set_plan: %q", res)
	}
	if res := a.dispatch(context.Background(), call("s", "submit_candidates", `{"tracks":[
		{"artist":"A","title":"T1","why":"w"},
		{"artist":"Ghost","title":"T2","why":"w"},
		{"artist":"C","title":"T3","why":"w"}
	]}`)); strings.HasPrefix(res, "error") {
		t.Fatalf("submit_candidates: %q", res)
	}
	picks := ui.picks
	if len(picks) != 3 {
		t.Fatalf("picks: %d, want 3: %+v", len(picks), picks)
	}
	if !picks[0].Accepted || picks[0].Track.ID != "A/T1" || picks[0].Name != "A — T1" {
		t.Errorf("picks[0]: %+v", picks[0])
	}
	if picks[1].Accepted || picks[1].Name != "Ghost — T2" || picks[1].Reason != "not in Apple" || picks[1].Track.ID != "" {
		t.Errorf("picks[1]: %+v", picks[1])
	}
	if !picks[2].Accepted || picks[2].Track.ID != "C/T3" || picks[2].Name != "C — T3" {
		t.Errorf("picks[2]: %+v", picks[2])
	}
}

func TestNoteKeptUntilNextTurn(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(
			call("p", "set_plan", `{"seeds":[],"exclusions":[]}`),
			call("s1", "submit_candidates", `{"tracks":[{"artist":"A","title":"T1","why":"w"}],"note":"держу рэйдж"}`),
			call("s2", "submit_candidates", `{"tracks":[{"artist":"B","title":"T2","why":"w"}],"note":""}`),
		),
		textResp("готово"), textResp("готово"), textResp("готово"),
		textResp("готово"), textResp("готово"), textResp("готово"),
	}}
	a := newTestAgent(t, ch, &fakeBackend{}, &fakeUI{})
	if _, err := a.Turn(context.Background(), "запрос"); err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if a.Note != "держу рэйдж" {
		t.Errorf("Note после хода = %q, want «держу рэйдж»", a.Note)
	}
	if _, err := a.Turn(context.Background(), "ещё"); err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if a.Note != "" {
		t.Errorf("Note после второго хода = %q, want пусто", a.Note)
	}
}

func TestAskLaterLifecycle(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		textResp("готово"), textResp("готово"), textResp("готово"),
	}}
	a := newTestAgent(t, ch, &fakeBackend{}, &fakeUI{})
	a.History = []Track{{Artist: "A", Title: "T1"}, {Artist: "B", Title: "T2"}}
	res := a.dispatch(context.Background(), call("l1", "ask_later",
		`{"question":"ядро или редкости?","options":[{"n":1,"label":"ядро"},{"n":2,"label":"редкости"}],"default":1}`))
	if !strings.HasPrefix(res, "ok") {
		t.Fatalf("ask_later: %q", res)
	}
	if a.Later == nil || a.Later.Question != "ядро или редкости?" {
		t.Fatalf("Later: %+v", a.Later)
	}
	if len(a.Later.Tracks) != 2 || a.Later.Tracks[0].Artist != "A" || a.Later.Tracks[1].Artist != "B" {
		t.Errorf("Later.Tracks: %+v", a.Later.Tracks)
	}
	if a.Later.Options[0] != "ядро" || a.Later.Default != "1" {
		t.Errorf("Options/Default: %v %q", a.Later.Options, a.Later.Default)
	}
	res2 := a.dispatch(context.Background(), call("l2", "ask_later",
		`{"question":"другое?","options":[{"n":1,"label":"один"},{"n":2,"label":"два"}],"default":1}`))
	if !strings.HasPrefix(res2, "error") {
		t.Errorf("второй ask_later: got %q, want ошибка", res2)
	}
	if a.Later == nil || a.Later.Question != "ядро или редкости?" {
		t.Errorf("Later после второго: %+v", a.Later)
	}
	res3 := a.dispatch(context.Background(), call("l3", "ask_later",
		`{"question":"один вариант?","options":[{"n":1,"label":"только один"}],"default":1}`))
	if !strings.HasPrefix(res3, "error") {
		t.Errorf("один вариант: got %q, want ошибка", res3)
	}
	if _, err := a.Turn(context.Background(), "поток"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if a.Later != nil {
		t.Errorf("Turn не обнулил Later: %+v", a.Later)
	}
}

func TestAskLaterTrackOptions(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	a.History = []Track{{Artist: "A", Title: "T1"}, {Artist: "B", Title: "T2"}, {Artist: "C", Title: "T3"}}
	res := a.dispatch(context.Background(), call("l1", "ask_later",
		`{"question":"дальше?","options":[{"n":1,"label":"дрим-поп"},{"n":3,"label":"шугейз"}],"default":1}`))
	if !strings.HasPrefix(res, "ok") {
		t.Fatalf("ask_later: %q", res)
	}
	if a.Later == nil || len(a.Later.Tracks) != 2 || a.Later.Tracks[0].Artist != "A" || a.Later.Tracks[1].Artist != "C" {
		t.Fatalf("Later.Tracks: %+v, want треки 1 и 3", a.Later)
	}
	if a.Later.Options[0] != "дрим-поп" || a.Later.Options[1] != "шугейз" || a.Later.Default != "1" {
		t.Errorf("Options/Default: %v %q", a.Later.Options, a.Later.Default)
	}
	a.Later = nil
	res2 := a.dispatch(context.Background(), call("l2", "ask_later",
		`{"question":"дальше?","options":[{"n":4,"label":"за пределом"}],"default":1}`))
	if !strings.HasPrefix(res2, "error") {
		t.Errorf("n за пределом History: got %q, want ошибка", res2)
	}
	a.History = nil
	res3 := a.dispatch(context.Background(), call("l3", "ask_later",
		`{"question":"дальше?","options":[{"n":1,"label":"дрим-поп"},{"n":2,"label":"шугейз"}],"default":1}`))
	if !strings.HasPrefix(res3, "error") {
		t.Errorf("пустая History: got %q, want ошибка", res3)
	}
}

func TestSubmitSchemaRequiresNoteAndAskLaterTool(t *testing.T) {
	var later, submit *llm.Tool
	for _, tool := range agentTools() {
		switch tool.Name {
		case "ask_later":
			later = &tool
		case "submit_candidates":
			submit = &tool
		}
	}
	if later == nil {
		t.Fatal("нет инструмента ask_later")
	}
	if submit == nil {
		t.Fatal("нет инструмента submit_candidates")
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(submit.Parameters, &schema); err != nil {
		t.Fatalf("submitSchema не JSON: %v", err)
	}
	note := false
	for _, r := range schema.Required {
		if r == "note" {
			note = true
		}
	}
	if !note {
		t.Errorf("submitSchema required не содержит note: %v", schema.Required)
	}
}

func TestPromptRulesCardNoteLater(t *testing.T) {
	for _, want := range []string{"ask_user", "ask_later", "note in submit_candidates", "duel"} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules не содержит %q", want)
		}
	}
	if strings.Contains(promptRules, "kind=scale") {
		t.Error("осталась шкала kind=scale")
	}
}

func TestPromptRulesFirstTurnQuestions(t *testing.T) {
	for _, want := range []string{
		"before set_plan decide whether an ask_user card is needed",
		"every option is a real track (artist, title) and a label of up to 3 words",
		"the request is only about a mood or an activity",
		"no question, straight to set_plan — only if the request names both the referent and the direction of the sound",
		"Do not ask abstractly about vocals, energy, language or popularity",
		"Do not ask a stock card",
		`The code answered "not found in Apple Music" — replace those options and call ask_user again`,
		"the taken field",
		`Do not repeat questions from the "Questions already asked" list`,
	} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules не содержит %q", want)
		}
	}
	for _, gone := range []string{"The listener likes to be asked", "with vocals or without"} {
		if strings.Contains(promptRules, gone) {
			t.Errorf("осталась старая формулировка %q", gone)
		}
	}
}

func TestPromptRulesWantItShare(t *testing.T) {
	for _, want := range []string{
		"portions of 5 tracks",
		"an artist name with a mood, time or activity",
		"3–4 tracks out of 5",
		`"only X", "X and that's it" — the whole portion from X`,
		"the last 20 tracks of the stream",
		"No more than 2 tracks of one artist in the window, features included, except the want_it referent",
		"no more than 4 songs with them into the window",
		"in batches of 5–8",
	} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules не содержит %q", want)
		}
	}
}

func TestPromptMissAndNudge(t *testing.T) {
	if !strings.Contains(promptRules, `"missed" — no option fit: follow the next field in the answer.`) {
		t.Error("promptRules не содержит строку про «missed» и поле next")
	}
	if !strings.Contains(promptRules, "2+ tracks skipped since the last question while something was played to the end") {
		t.Error("promptRules не содержит новую формулировку развилки")
	}
	if strings.Contains(promptRules, "of one direction, and the other played to the end") {
		t.Error("осталась старая формулировка развилки")
	}
}

func TestPromptRulesAskLaterPlayedTracks(t *testing.T) {
	if !strings.Contains(promptRules, `an option is the number n of a played-through track from the "Served" list`) {
		t.Error("ask_later в промпте не про номера дослушанных треков")
	}
}

func TestAskUserOnlyOnFirstTurn(t *testing.T) {
	ui := &fakeUI{answers: []string{"1"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":[{"artist":"A","title":"T1","label":"a"},{"artist":"B","title":"T2","label":"b"}],"default":1}]}`
	a.turns = 2
	want := "error: ask_user is allowed only in the first turn of the session; ask a fork in the skips through ask_later"
	if res := a.dispatch(context.Background(), call("q2", "ask_user", args)); res != want {
		t.Errorf("ход 2: got %q, want %q", res, want)
	}
	if ui.asks != 0 {
		t.Errorf("UI.Ask вызван %d раз, want 0", ui.asks)
	}
	a.turns = 1
	if res := a.dispatch(context.Background(), call("q1", "ask_user", args)); strings.HasPrefix(res, "error") {
		t.Errorf("ход 1: got %q", res)
	}
	if ui.asks != 1 {
		t.Errorf("UI.Ask вызван %d раз, want 1", ui.asks)
	}
}

func TestDispatchSetPlanFromFavorites(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[],"from_favorites":true}`))
	if res != "plan accepted: referents 0, exclusions 0, favorites mode" {
		t.Errorf("from_favorites: got %q", res)
	}
	a2 := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	res = a2.dispatch(context.Background(), call("p2", "set_plan", `{"seeds":[],"exclusions":[]}`))
	if res != "plan accepted: referents 0, exclusions 0" {
		t.Errorf("без поля: got %q", res)
	}
}
