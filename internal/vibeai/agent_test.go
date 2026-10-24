package vibeai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
)

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
	f.findCalls = append(f.findCalls, sug.Artist+"|"+sug.Title)
	if sug.Artist == "Ghost" {
		return Song{}, false, nil
	}
	return Song{ID: sug.Artist + "/" + sug.Title, Artist: sug.Artist, Title: sug.Title, Album: "Album", Year: "2024"}, true, nil
}

type fakeUI struct {
	answers  []string
	asks     int
	progress []string
	picks    []Pick
}

func (f *fakeUI) Ask(qs []Question) []string {
	f.asks++
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
	return NewAgent(ch, be, NewSession(nil, nil, 3), st, ui, nil, nil)
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
	if !strings.Contains(res, "сначала set_plan") {
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
		if m.Role == "user" && strings.HasPrefix(m.Content, "Порция не заполнена") {
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
		{"question":"q1","options":["a1","a2"],"default":"default1"},
		{"question":"q2","options":["x1","x2"],"default":"d2"}
	]}`
	res := a.dispatch(context.Background(), call("q", "ask_user", args))
	want := `[{"question":"q1","answer":"default1"},{"question":"q2","answer":"x2"}]`
	if res != want {
		t.Errorf("ask_user: got %s, want %s", res, want)
	}
	res2 := a.dispatch(context.Background(), call("q2", "ask_user", args))
	if !strings.Contains(res2, "вопросы уже заданы") {
		t.Errorf("повтор: got %q", res2)
	}
	q := Question{Question: "q", Options: []string{"o1", "o2"}, Default: "d"}
	if got := resolveAnswer(q, "своё"); got != "свой вариант: своё" {
		t.Errorf("resolveAnswer(своё) = %q", got)
	}
	if got := resolveAnswer(q, "9"); got != "свой вариант: 9" {
		t.Errorf("resolveAnswer(9) = %q", got)
	}
	if got := resolveAnswer(q, " 1 "); got != "o1" {
		t.Errorf("resolveAnswer(« 1 ») = %q", got)
	}
}

func TestMemoryAddDuplicate(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	args := `{"text":"не нравится Gunna","evidence":"«Gunna нет»"}`
	res := a.dispatch(context.Background(), call("m1", "memory_add", args))
	if !strings.HasPrefix(res, "ok: память") {
		t.Errorf("первый add: got %q", res)
	}
	res2 := a.dispatch(context.Background(), call("m2", "memory_add", args))
	if !strings.HasPrefix(res2, "ошибка: уже есть похожая запись") {
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
	if res := a.dispatch(context.Background(), call("u", "foo", `{}`)); res != "ошибка: нет инструмента foo" {
		t.Errorf("неизвестный инструмент: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("u2", "set_plan", `{`)); res != "ошибка: аргументы не JSON" {
		t.Errorf("плохие аргументы: got %q", res)
	}
}

func TestSubmitAcceptedRejectedOrder(t *testing.T) {
	be := &fakeBackend{}
	a := newTestAgent(t, &fakeChatter{}, be, &fakeUI{})
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "план принят") {
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
	if len(out.Rejected) != 1 || out.Rejected[0].Track != "Ghost — X" || out.Rejected[0].Reason != "нет в Apple" {
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
	sp := systemPrompt(st, []Song{{Artist: "Yeat", Title: "bigger thën everything", Album: "bigger thën everything"}})
	for _, want := range []string{
		"Ты — музыкальный подборщик Vibes",
		"## Память о владельце",
		"- Yeat — bigger thën everything (bigger thën everything)",
	} {
		if !strings.Contains(sp, want) {
			t.Errorf("systemPrompt не содержит %q", want)
		}
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
	if res := a.dispatch(context.Background(), call("d1", "describe", `{"kind":"song","artist":"X"}`)); !strings.Contains(res, "ошибка: kind") {
		t.Errorf("kind song: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("d2", "describe", `{"kind":"album","artist":"X"}`)); res != "ошибка: нужен name" {
		t.Errorf("album без name: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("d3", "describe", `{"kind":"artist","artist":"X"}`)); strings.HasPrefix(res, "ошибка") {
		t.Errorf("artist без name: got %q", res)
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
	want := "· submit 3 → принято 3, отклонено 0 (порция 3/3)"
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

func TestTurnDrainsReserveFirst(t *testing.T) {
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
			{"artist":"D","title":"T4","why":"w"}
		]}`)),
	}}
	be := &fakeBackend{}
	ui := &fakeUI{}
	st, err := memory.Open(filepath.Join(t.TempDir(), "memory.md"), memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	a := NewAgent(ch, be, NewSession(nil, nil, 2), st, ui, nil, nil)

	entries, err := a.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("ход 1: %v", err)
	}
	if len(entries) != 2 || entries[0].Song.Artist != "A" || entries[1].Song.Artist != "B" {
		t.Fatalf("ход 1: %+v, want [A, B]", entries)
	}

	entries2, err := a.Turn(context.Background(), "ещё")
	if err != nil {
		t.Fatalf("ход 2: %v", err)
	}
	if len(entries2) != 2 || entries2[0].Song.Artist != "C" || entries2[1].Song.Artist != "D" {
		t.Fatalf("ход 2: %+v, want [C, D]", entries2)
	}
	if len(ch.got) != 2 {
		t.Fatalf("Chat вызван %d раз, want 2", len(ch.got))
	}
	user := ch.got[1][len(ch.got[1])-1]
	if user.Role != "user" {
		t.Fatalf("последнее сообщение хода 2: %q", user.Role)
	}
	want := "\n\nИз запаса прошлой порции уже приняты: C — T3. Порция 1/2, нужно ещё 1."
	if !strings.HasSuffix(user.Content, want) {
		t.Errorf("user не заканчивается на %q:\n%s", want, user.Content)
	}
	cIdx, dIdx := -1, -1
	for i, p := range ui.picks {
		if !p.Accepted {
			continue
		}
		switch p.Name {
		case "C — T3":
			if cIdx < 0 {
				cIdx = i
			}
		case "D — T4":
			dIdx = i
		}
	}
	if cIdx < 0 || dIdx < 0 {
		t.Fatalf("picks: %+v, want принятые C и D", ui.picks)
	}
	if dIdx < cIdx {
		t.Errorf("D (%d) раньше C (%d) в picks: %+v", dIdx, cIdx, ui.picks)
	}
}

func TestTurnStopsAtSlack(t *testing.T) {
	tracks := make([]string, 0, 18)
	for i := 0; i < 18; i++ {
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
	a := NewAgent(ch, &fakeBackend{}, NewSession(nil, nil, 20), st, &fakeUI{}, nil, nil)

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
			if m.Role == "user" && strings.HasPrefix(m.Content, "Порция не заполнена") {
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
	for _, want := range []string{"### шаг 1 (", "### шаг 2 (", "готово", "→ describe {\"kind\":\"artist\",\"artist\":\"Yeat\"}", "← {"} {
		if !strings.Contains(out, want) {
			t.Errorf("лог не содержит %q:\n%s", want, out)
		}
	}
}

func TestSubmitOverLimitAndBadMode(t *testing.T) {
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "план принят") {
		t.Fatalf("set_plan: %q", res)
	}
	tracks := `{"tracks":[`
	for i := 0; i < 31; i++ {
		if i > 0 {
			tracks += ","
		}
		tracks += fmt.Sprintf(`{"artist":"A%d","title":"T%d","why":"w"}`, i, i)
	}
	tracks += `]}`
	res := a.dispatch(context.Background(), call("s", "submit_candidates", tracks))
	if res != "ошибка: 1–30 треков" {
		t.Errorf("31 трек: got %q", res)
	}
	if res := a.dispatch(context.Background(), call("p2", "set_plan", `{"seeds":[{"artist":"X","mode":"bad"}],"exclusions":[]}`)); res != "ошибка: mode bad" {
		t.Errorf("mode: got %q", res)
	}
}

func TestResolveAnswerScale(t *testing.T) {
	q := Question{Kind: "scale", Steps: 5, Left: "фон", Right: "разъёб", Default: "3"}
	if got := resolveAnswer(q, ""); got != "3 из 5 (1 — фон, 5 — разъёб)" {
		t.Errorf("пусто: got %q", got)
	}
	if got := resolveAnswer(q, "5"); got != "5 из 5 (1 — фон, 5 — разъёб)" {
		t.Errorf("5: got %q", got)
	}
	if got := resolveAnswer(q, "9"); got != "свой вариант: 9" {
		t.Errorf("9: got %q", got)
	}
	if got := resolveAnswer(q, " громче "); got != "свой вариант: громче" {
		t.Errorf("громче: got %q", got)
	}
}

func TestAskUserChoiceAndScaleValidation(t *testing.T) {
	cases := []struct {
		args  string
		isErr bool
	}{
		{`{"questions":[{"question":"э","kind":"scale","steps":2,"left":"a","right":"b","default":"1"}]}`, true},
		{`{"questions":[{"question":"э","kind":"scale","steps":5,"left":"a","right":"b","default":"3"}]}`, false},
		{`{"questions":[{"question":"э","kind":"choice","options":["x"],"default":"x"}]}`, true},
		{`{"questions":[{"question":"э","kind":"","options":["x","y"],"default":"x"}]}`, false},
		{`{"questions":[{"question":"э","kind":"slider","options":["x","y"],"default":"x"}]}`, true},
	}
	for i, tc := range cases {
		a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, &fakeUI{})
		res := a.dispatch(context.Background(), call("q", "ask_user", tc.args))
		if tc.isErr {
			if res != askUserErr {
				t.Errorf("case %d: got %q, want %q", i, res, askUserErr)
			}
			continue
		}
		if strings.HasPrefix(res, "ошибка") {
			t.Errorf("case %d: got %q, want не ошибка", i, res)
		}
	}
}

func TestAskUserScaleDefaultAnswer(t *testing.T) {
	ui := &fakeUI{}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	res := a.dispatch(context.Background(), call("q", "ask_user",
		`{"questions":[{"question":"энергия","kind":"scale","steps":5,"left":"фон","right":"разъёб","default":"3"}]}`))
	want := `[{"question":"энергия","answer":"3 из 5 (1 — фон, 5 — разъёб)"}]`
	if res != want {
		t.Errorf("ask_user scale: got %s, want %s", res, want)
	}
	if len(ui.picks) != 0 {
		t.Errorf("ask_user не должен слать picks: %+v", ui.picks)
	}
}

func TestSubmitCandidatesPickedEvents(t *testing.T) {
	ui := &fakeUI{}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	if res := a.dispatch(context.Background(), call("p", "set_plan", `{"seeds":[],"exclusions":[]}`)); !strings.HasPrefix(res, "план принят") {
		t.Fatalf("set_plan: %q", res)
	}
	if res := a.dispatch(context.Background(), call("s", "submit_candidates", `{"tracks":[
		{"artist":"A","title":"T1","why":"w"},
		{"artist":"Ghost","title":"T2","why":"w"},
		{"artist":"C","title":"T3","why":"w"}
	]}`)); strings.HasPrefix(res, "ошибка") {
		t.Fatalf("submit_candidates: %q", res)
	}
	picks := ui.picks
	if len(picks) != 3 {
		t.Fatalf("picks: %d, want 3: %+v", len(picks), picks)
	}
	if !picks[0].Accepted || picks[0].Track.ID != "A/T1" || picks[0].Name != "A — T1" {
		t.Errorf("picks[0]: %+v", picks[0])
	}
	if picks[1].Accepted || picks[1].Name != "Ghost — T2" || picks[1].Reason != "нет в Apple" || picks[1].Track.ID != "" {
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
	res := a.dispatch(context.Background(), call("l1", "ask_later",
		`{"question":"ядро или редкости?","options":["ядро","редкости"],"default":"ядро"}`))
	if !strings.HasPrefix(res, "ok") {
		t.Fatalf("ask_later: %q", res)
	}
	if a.Later == nil || a.Later.Kind != "choice" || a.Later.Question != "ядро или редкости?" {
		t.Fatalf("Later: %+v", a.Later)
	}
	res2 := a.dispatch(context.Background(), call("l2", "ask_later",
		`{"question":"другое?","options":["один","два"],"default":"один"}`))
	if !strings.HasPrefix(res2, "ошибка") {
		t.Errorf("второй ask_later: got %q, want ошибка", res2)
	}
	if a.Later == nil || a.Later.Question != "ядро или редкости?" {
		t.Errorf("Later после второго: %+v", a.Later)
	}
	res3 := a.dispatch(context.Background(), call("l3", "ask_later",
		`{"question":"один вариант?","options":["только один"],"default":"x"}`))
	if !strings.HasPrefix(res3, "ошибка") {
		t.Errorf("один вариант: got %q, want ошибка", res3)
	}
	if _, err := a.Turn(context.Background(), "поток"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if a.Later != nil {
		t.Errorf("Turn не обнулил Later: %+v", a.Later)
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

func TestPromptRulesScaleNoteLater(t *testing.T) {
	for _, want := range []string{"kind=scale", "ask_later", "note в submit_candidates"} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules не содержит %q", want)
		}
	}
}

func TestPromptRulesFirstTurnQuestions(t *testing.T) {
	for _, want := range []string{
		"перед set_plan уточни вайб через ask_user",
		"Запрос только про настроение или занятие",
		"Без вопросов, сразу set_plan — только если запрос задаёт и ориентир, и направление звучания",
		"Не задавай дежурный набор вопросов",
		"Не повторяй вопросы из списка «Вопросы уже заданы»",
	} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules не содержит %q", want)
		}
	}
	if strings.Contains(promptRules, "Владельцу нравится, когда его спрашивают") {
		t.Error("осталась старая формулировка про допрос")
	}
}

func TestAskUserOnlyOnFirstTurn(t *testing.T) {
	ui := &fakeUI{answers: []string{"1"}}
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, ui)
	args := `{"questions":[{"question":"q","options":["a","b"],"default":"a"}]}`
	a.turns = 2
	want := "ошибка: ask_user — только в первом ходе сессии; развилку по скипам спроси через ask_later"
	if res := a.dispatch(context.Background(), call("q2", "ask_user", args)); res != want {
		t.Errorf("ход 2: got %q, want %q", res, want)
	}
	if ui.asks != 0 {
		t.Errorf("UI.Ask вызван %d раз, want 0", ui.asks)
	}
	a.turns = 1
	if res := a.dispatch(context.Background(), call("q1", "ask_user", args)); strings.HasPrefix(res, "ошибка") {
		t.Errorf("ход 1: got %q", res)
	}
	if ui.asks != 1 {
		t.Errorf("UI.Ask вызван %d раз, want 1", ui.asks)
	}
}
