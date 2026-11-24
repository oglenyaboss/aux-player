package vibeai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
)

func newTestEngineUI(t *testing.T, ch *fakeChatter, ui UI) *Engine {
	a := newTestAgent(t, ch, &fakeBackend{}, ui)
	sig, err := OpenSignals("")
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	sig.now = func() time.Time { return testNow }
	e := &Engine{
		agent:     a,
		session:   a.Session,
		mem:       a.Memory,
		log:       &runLog{},
		sig:       sig,
		sessionID: "s1",
		start:     time.Now(),
	}
	e.agent.Log = e.log
	e.agent.State = e.history
	return e
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
		t.Fatalf("turn 1: %v", err)
	}
	if len(p.Tracks) != 3 {
		t.Fatalf("turn 1: %d tracks, want 3", len(p.Tracks))
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
		t.Fatalf("turn 2: %v", err)
	}
	for i, want := range []string{"D", "E", "F"} {
		if p2.Tracks[i].Artist != want {
			t.Errorf("turn 2, track %d: %q, want %q", i, p2.Tracks[i].Artist, want)
		}
	}
	return e
}

func compactionPlan(id string) llm.Response {
	return usageResp(llm.Usage{PromptTokens: 9000, CompletionTokens: 10},
		call(id, "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`))
}

func compactionSubmit(id string, artists ...string) llm.Response {
	items := make([]string, 0, len(artists))
	for i, a := range artists {
		items = append(items, fmt.Sprintf(`{"artist":%q,"title":"T%d","why":"w"}`, a, i+1))
	}
	return usageResp(llm.Usage{PromptTokens: 9000, CompletionTokens: 10},
		call(id, "submit_candidates", `{"tracks":[`+strings.Join(items, ",")+`]}`))
}

func TestCompactionByLimit(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		compactionPlan("c1"),
		compactionSubmit("c2", "A", "B", "C"),
		compactionSubmit("c3", "D", "E", "F"),
		compactionSubmit("c4", "G", "H", "I"),
		compactionSubmit("c5", "J", "K", "L"),
	}}
	e := newTestEngine(t, ch)
	e.agent.ContextLimit = 8000
	starts := make([]int, 4)
	for i, text := range []string{"в духе X", "", "", ""} {
		starts[i] = len(ch.got)
		p, err := e.Turn(context.Background(), text)
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
		if len(p.Tracks) != 3 {
			t.Fatalf("turn %d: %d tracks, want 3", i+1, len(p.Tracks))
		}
		for _, msgs := range ch.got[starts[i]:] {
			if err := validHistory(msgs); err != nil {
				t.Errorf("turn %d: invalid history: %v", i+1, err)
			}
		}
		if i == 1 && strings.Contains(e.log.b.String(), "compaction") {
			t.Fatalf("a compaction happened with only two turns:\n%s", e.log.b.String())
		}
	}
	msgs := ch.got[starts[2]]
	if msgs[0].Role != "system" {
		t.Errorf("the first message of the turn 3 call is %q, want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" ||
		!strings.HasPrefix(msgs[1].Content, "Earlier turns of this stream, compacted by the code:\n") ||
		!strings.Contains(msgs[1].Content, "Session: request") {
		t.Errorf("the state block is wrong: role %s:\n%s", msgs[1].Role, msgs[1].Content)
	}
	turn2 := ch.got[starts[1]]
	rest := msgs[2:]
	if len(rest) != 4 {
		t.Fatalf("the turn 3 call keeps %d messages after the state block, want 4 (turn 2 and turn 3):\n%+v", len(rest), rest)
	}
	if !sameMessage(rest[0], turn2[len(turn2)-1]) {
		t.Errorf("the turn 2 user message is not verbatim:\ngot  %+v\nwant %+v", rest[0], turn2[len(turn2)-1])
	}
	if len(rest[1].ToolCalls) != 1 || rest[1].ToolCalls[0].ID != "c3" || rest[2].Role != "tool" || rest[2].ToolCallID != "c3" {
		t.Errorf("the turn 2 tool call lost its result:\n%+v\n%+v", rest[1], rest[2])
	}
	if last := rest[3]; last.Role != "user" {
		t.Errorf("the last message of the turn 3 call is %q, want user", last.Role)
	}
	log := e.log.b.String()
	for _, want := range []string{"compaction 1: input 9000 → 9000 tokens", "compaction 2: input 9000 → 9000 tokens"} {
		if n := strings.Count(log, want); n != 1 {
			t.Errorf("%q appears %d times, want 1:\n%s", want, n, log)
		}
	}
	for _, num := range []string{"compaction 1:", "compaction 2:"} {
		if n := strings.Count(log, num); n != 2 {
			t.Errorf("%q appears %d times, want 2 (messages and input):\n%s", num, n, log)
		}
	}
	if strings.Contains(log, "compaction 3") {
		t.Errorf("a third compaction happened:\n%s", log)
	}
	if _, err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if last := lastLogLine(e.log.b.String()); !strings.HasSuffix(last, ", compactions 2") {
		t.Errorf("the last log line = %q, want it to end with ', compactions 2'", last)
	}
}

func TestCompactionOnContextError(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		compactionPlan("c1"),
		compactionSubmit("c2", "A", "B", "C"),
		compactionSubmit("c3", "D", "E", "F"),
		compactionSubmit("c4", "G", "H", "I"),
		compactionSubmit("c5", "J", "K", "L"),
	}}
	e := newTestEngine(t, ch)
	e.agent.ContextLimit = 1000000000
	for i, text := range []string{"в духе X", "", ""} {
		if _, err := e.Turn(context.Background(), text); err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
	}
	e.agent.LLM = &erroringChatter{ch: ch, err: fmt.Errorf("x: %w", llm.ErrContextLength), failAt: []int{1}}
	p, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("turn 4: %v", err)
	}
	if len(p.Tracks) != 3 || p.Tracks[0].Artist != "J" {
		t.Fatalf("turn 4 portion: %+v, want J, K, L", p.Tracks)
	}
	log := e.log.b.String()
	if n := strings.Count(log, "compaction 1:"); n != 2 {
		t.Errorf("compaction 1 lines = %d, want 2 (messages and input):\n%s", n, log)
	}
	if strings.Contains(log, "compaction 2") {
		t.Errorf("a second compaction happened:\n%s", log)
	}
	retried := ch.got[5]
	if len(retried) < 2 || retried[1].Role != "user" ||
		!strings.HasPrefix(retried[1].Content, "Earlier turns of this stream, compacted by the code:\n") {
		t.Fatalf("the retried call's second message is not the state block: %+v", retried[1])
	}
	if err := validHistory(retried); err != nil {
		t.Errorf("the retried call: %v", err)
	}
}

func TestCompactionErrorRetriesOnce(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		compactionPlan("c1"),
		compactionSubmit("c2", "A", "B", "C"),
		compactionSubmit("c3", "D", "E", "F"),
		compactionSubmit("c4", "G", "H", "I"),
		compactionSubmit("c6", "M", "N", "O"),
	}}
	e := newTestEngine(t, ch)
	e.agent.ContextLimit = 1000000000
	for i, text := range []string{"в духе X", "", ""} {
		if _, err := e.Turn(context.Background(), text); err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
	}
	e.agent.LLM = &erroringChatter{ch: ch, err: fmt.Errorf("x: %w", llm.ErrContextLength), failAt: []int{1, 2}}
	if _, err := e.Turn(context.Background(), ""); err == nil || !errors.Is(err, llm.ErrContextLength) {
		t.Fatalf("turn 4: err = %v, want a wrapped ErrContextLength", err)
	}
	if n := strings.Count(e.log.b.String(), "compaction 1:"); n != 1 {
		t.Errorf("the failed turn compacted %d times, want 1:\n%s", n, e.log.b.String())
	}
	e.agent.LLM = validatingChatter{ch: ch}
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 5: %v", err)
	}
	for _, msgs := range ch.got[start:] {
		if err := validHistory(msgs); err != nil {
			t.Errorf("turn 5: invalid history after the rollback: %v", err)
		}
	}
}

func TestCompactionNotPossibleEarly(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		compactionPlan("c1"),
		compactionSubmit("c2", "A", "B", "C"),
		compactionSubmit("c3", "D", "E", "F"),
	}}
	e := newTestEngine(t, ch)
	e.agent.ContextLimit = 8000
	for i, text := range []string{"в духе X", ""} {
		p, err := e.Turn(context.Background(), text)
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
		if len(p.Tracks) != 3 {
			t.Fatalf("turn %d: %d tracks, want 3", i+1, len(p.Tracks))
		}
	}
	if strings.Contains(e.log.b.String(), "compaction") {
		t.Errorf("a compaction happened with only two turns:\n%s", e.log.b.String())
	}
}

func lastUser(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func sameMessage(a, b llm.Message) bool {
	if a.Role != b.Role || a.Content != b.Content || a.ToolCallID != b.ToolCallID || len(a.ToolCalls) != len(b.ToolCalls) {
		return false
	}
	for i := range a.ToolCalls {
		if a.ToolCalls[i] != b.ToolCalls[i] {
			return false
		}
	}
	return true
}

func validHistory(msgs []llm.Message) error {
	pending := map[string]bool{}
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			for _, c := range m.ToolCalls {
				pending[c.ID] = true
			}
		case "tool":
			if !pending[m.ToolCallID] {
				return fmt.Errorf("tool result %q without a pending call", m.ToolCallID)
			}
			delete(pending, m.ToolCallID)
		}
	}
	for id := range pending {
		return fmt.Errorf("tool call %q has no result", id)
	}
	return nil
}

type failingChatter struct {
	ch  *fakeChatter
	err error
	at  int
}

func (f *failingChatter) Chat(ctx context.Context, msgs []llm.Message, tools []llm.Tool, jsonMode bool) (llm.Response, error) {
	f.at--
	if f.at == 0 {
		f.ch.got = append(f.ch.got, append([]llm.Message(nil), msgs...))
		return llm.Response{}, f.err
	}
	return f.ch.Chat(ctx, msgs, tools, jsonMode)
}

type validatingChatter struct{ ch *fakeChatter }

func (v validatingChatter) Chat(ctx context.Context, msgs []llm.Message, tools []llm.Tool, jsonMode bool) (llm.Response, error) {
	if err := validHistory(msgs); err != nil {
		v.ch.got = append(v.ch.got, append([]llm.Message(nil), msgs...))
		return llm.Response{}, fmt.Errorf("invalid history: %w", err)
	}
	return v.ch.Chat(ctx, msgs, tools, jsonMode)
}

func lastLogLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return lines[i]
		}
	}
	return ""
}

func turnTotalSteps(t *testing.T, text, after string, ends ...string) int {
	t.Helper()
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if l == after {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("log has no %q line:\n%s", after, text)
	}
	for _, l := range lines[start+1:] {
		for _, end := range ends {
			if strings.HasPrefix(l, end) {
				t.Fatalf("no turn total between %q and %q:\n%s", after, end, text)
			}
		}
		rest, ok := strings.CutPrefix(l, "turn total: steps ")
		if !ok {
			continue
		}
		num, _, _ := strings.Cut(rest, ",")
		n, err := strconv.Atoi(num)
		if err != nil {
			t.Fatalf("turn total %q: %v", l, err)
		}
		return n
	}
	t.Fatalf("no turn total after %q:\n%s", after, text)
	return 0
}

func TestTurnKeepsConversation(t *testing.T) {
	ch := &fakeChatter{}
	flowEngine(t, ch)
	if len(ch.got) != 3 {
		t.Fatalf("Chat called %d times, want 3", len(ch.got))
	}
	msgs := ch.got[2]
	step := len(ch.got[1]) + 2
	if len(msgs) != step+1 {
		t.Fatalf("turn 2 call: %d messages, want %d — turn 1, its last step and the new user message", len(msgs), step+1)
	}
	for i := range ch.got[1] {
		if !sameMessage(msgs[i], ch.got[1][i]) {
			t.Errorf("message %d of the turn 2 call differs from turn 1:\ngot  %+v\nwant %+v", i, msgs[i], ch.got[1][i])
		}
	}
	extra := msgs[len(ch.got[1]):step]
	if extra[0].Role != "assistant" || len(extra[0].ToolCalls) != 1 {
		t.Fatalf("message %d: %s with %d tool calls, want an assistant with 1 call", len(ch.got[1]), extra[0].Role, len(extra[0].ToolCalls))
	}
	if extra[1].Role != "tool" || extra[1].ToolCallID != extra[0].ToolCalls[0].ID {
		t.Errorf("the last step's tool result: %+v, want the ID of the call above", extra[1])
	}
	if last := msgs[len(msgs)-1]; last.Role != "user" {
		t.Fatalf("the last message is %q, want user", last.Role)
	}
}

func TestTurnDeltaListsOnlyChanges(t *testing.T) {
	ch := &fakeChatter{}
	e := flowEngine(t, ch)
	user := lastUser(ch.got[2])
	for _, want := range []string{
		"Portion 1 is served, numbered as on the listener's screen:",
		"1. A — T1",
		"Since the last turn:",
		"- 1. A — T1 — played through",
		"- 2. B — T2 — skipped at 0:12",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("turn 2: user does not contain %q:\n%s", want, user)
		}
	}
	for _, gone := range []string{"3. C — T3 —", "Served (continuous numbering"} {
		if strings.Contains(user, gone) {
			t.Errorf("turn 2: user still contains %q:\n%s", gone, user)
		}
	}
	d := e.portions[1][0]
	if err := e.Record([]Outcome{{ID: d.ID, Artist: d.Artist, Title: d.Title, Kind: "more"}}); err != nil {
		t.Fatalf("Record: %v", err)
	}
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
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	user = lastUser(ch.got[3])
	for _, want := range []string{"Portion 2 is served", "4. D — T4", "- 4. D — T4 — not listened, ▲ more of this"} {
		if !strings.Contains(user, want) {
			t.Errorf("turn 3: user does not contain %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "A — T1") {
		t.Errorf("turn 3: user still carries portion 1:\n%s", user)
	}
}

func TestTurnRollbackOnCancel(t *testing.T) {
	ch := &fakeChatter{}
	e := flowEngine(t, ch)
	d := e.portions[1][0]
	if err := e.Record([]Outcome{{ID: d.ID, Artist: d.Artist, Title: d.Title, Kind: "played"}}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	ch.resps = append(ch.resps, toolResp(call("d9", "describe", `{"kind":"artist","artist":"Z"}`)))
	e.agent.LLM = &failingChatter{ch: ch, err: context.Canceled, at: 2}
	if _, err := e.Turn(context.Background(), ""); err == nil {
		t.Fatal("turn 3: want an error")
	}
	e.agent.LLM = ch
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
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 4: %v", err)
	}
	msgs := ch.got[start]
	if err := validHistory(msgs); err != nil {
		t.Errorf("turn 4: the history is invalid: %v", err)
	}
	for _, m := range msgs {
		for _, c := range m.ToolCalls {
			if c.Name == "describe" {
				t.Errorf("turn 4: a tool call of the cancelled turn survived: %+v", c)
			}
		}
	}
	if user := lastUser(msgs); !strings.Contains(user, "- 4. D — T4 — played through") {
		t.Errorf("turn 4: the delta recorded before the cancelled turn is lost:\n%s", user)
	}
}

func TestTurnRollbackOnError(t *testing.T) {
	ch := &fakeChatter{}
	e := flowEngine(t, ch)
	d := e.portions[1][0]
	if err := e.Record([]Outcome{{ID: d.ID, Artist: d.Artist, Title: d.Title, Kind: "played"}}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	ch.resps = append(ch.resps, toolResp(call("d9", "describe", `{"kind":"artist","artist":"Z"}`)))
	e.agent.LLM = &failingChatter{ch: ch, err: errors.New("boom"), at: 2}
	if _, err := e.Turn(context.Background(), ""); err == nil {
		t.Fatal("turn 3: want an error")
	}
	e.agent.LLM = validatingChatter{ch: ch}
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
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 4: %v", err)
	}
	msgs := ch.got[start]
	if err := validHistory(msgs); err != nil {
		t.Errorf("turn 4: the history is invalid: %v", err)
	}
	for _, m := range msgs {
		for _, c := range m.ToolCalls {
			if c.Name == "describe" {
				t.Errorf("turn 4: a tool call of the failed turn survived: %+v", c)
			}
		}
	}
	if user := lastUser(msgs); !strings.Contains(user, "- 4. D — T4 — played through") {
		t.Errorf("turn 4: the delta recorded before the failed turn is lost:\n%s", user)
	}
}

func TestTurnFirstTurnFailedSendsHistory(t *testing.T) {
	ch := &fakeChatter{}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err == nil {
		t.Fatal("turn 1: want an error")
	}
	ch.resps = append(ch.resps,
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
	)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	msgs := ch.got[1]
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("the continuation call: %d messages, want exactly [system user]", len(msgs))
	}
	for _, want := range []string{"Session: request", "Continue the stream"} {
		if !strings.Contains(msgs[1].Content, want) {
			t.Errorf("the continuation message does not contain %q:\n%s", want, msgs[1].Content)
		}
	}
}

func TestTurnDeltaNotes(t *testing.T) {
	firstTurn := func(t *testing.T, ch *fakeChatter) *Engine {
		ch.resps = append(ch.resps,
			toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
			toolResp(call("c2", "submit_candidates", `{"tracks":[
				{"artist":"A","title":"T1","why":"w"},
				{"artist":"B","title":"T2","why":"w"},
				{"artist":"C","title":"T3","why":"w"}
			]}`)),
		)
		e := newTestEngine(t, ch)
		if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
			t.Fatalf("turn 1: %v", err)
		}
		return e
	}
	ch := &fakeChatter{}
	e := firstTurn(t, ch)
	e.NoteSaved(Song{ID: e.portions[0][0].ID, Artist: "A", Title: "T1"}, true)
	q := Question{Question: "дальше?", Options: []string{"дрим-поп"},
		Tracks: []Track{{Artist: "A", Title: "T1"}}, Default: "1"}
	e.Answer(q, "дрим-поп")
	ch.resps = append(ch.resps, toolResp(call("c3", "submit_candidates", `{"tracks":[
		{"artist":"D","title":"T4","why":"w"},
		{"artist":"E","title":"T5","why":"w"},
		{"artist":"F","title":"T6","why":"w"}
	]}`)))
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	user := lastUser(ch.got[2])
	for _, want := range []string{"- saved to Favorites: A — T1", "- answer to the question \""} {
		if !strings.Contains(user, want) {
			t.Errorf("turn 2: user does not contain %q:\n%s", want, user)
		}
	}

	ch2 := &fakeChatter{}
	e2 := firstTurn(t, ch2)
	e2.DropLast()
	ch2.resps = append(ch2.resps, toolResp(call("c3", "submit_candidates", `{"tracks":[
		{"artist":"D","title":"T4","why":"w"},
		{"artist":"E","title":"T5","why":"w"},
		{"artist":"F","title":"T6","why":"w"}
	]}`)))
	if _, err := e2.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2 after the drop: %v", err)
	}
	user2 := lastUser(ch2.got[2])
	if !strings.Contains(user2, "- portion 1 was dropped before it played: the listener replied") {
		t.Errorf("turn 2 after the drop: user does not contain the drop note:\n%s", user2)
	}
	if strings.Contains(user2, "Portion 1 is served") {
		t.Errorf("turn 2 after the drop: the dropped portion is announced:\n%s", user2)
	}
}

func TestRunLogTurnTotals(t *testing.T) {
	u := llm.Usage{PromptTokens: 100, CompletionTokens: 10, CachedTokens: 50, CachedKnown: true}
	ch := &fakeChatter{resps: []llm.Response{
		usageResp(u, call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		usageResp(u, call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		usageResp(u, call("c3", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"},
			{"artist":"E","title":"T5","why":"w"},
			{"artist":"F","title":"T6","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	p, err := e.Turn(context.Background(), "в духе X")
	if err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	turn1Calls := len(ch.got)
	if err := e.Record([]Outcome{
		{ID: p.Tracks[0].ID, Artist: "A", Title: "T1", Kind: "played"},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	turn2Calls := len(ch.got) - turn1Calls
	if _, err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	text := e.log.b.String()
	if n := turnTotalSteps(t, text, "## portion 1", "## portion 2", "## listener"); n != turn1Calls {
		t.Errorf("portion 1 turn total: steps %d, want %d", n, turn1Calls)
	}
	if n := turnTotalSteps(t, text, "## portion 2", "## listener"); n != turn2Calls {
		t.Errorf("portion 2 turn total: steps %d, want %d", n, turn2Calls)
	}
	last := lastLogLine(text)
	if !strings.HasPrefix(last, "session total: steps 3, in 300, cached 150, out 30, reasoning 0, wall ") || !strings.HasSuffix(last, ", compactions 0") {
		t.Errorf("last line = %q", last)
	}
}

func TestRunLogSessionTotalCountsFailedTurn(t *testing.T) {
	u := llm.Usage{PromptTokens: 100, CompletionTokens: 10, CachedTokens: 50, CachedKnown: true}
	ch := &fakeChatter{resps: []llm.Response{
		usageResp(u, call("c1", "set_plan", `{"seeds":[{"artist":"X","mode":"and_similar"}],"exclusions":[]}`)),
		usageResp(u, call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		usageResp(u, call("c3", "set_plan", `{"seeds":[{"artist":"Y","mode":"and_similar"}],"exclusions":[]}`)),
	}}
	e := newTestEngine(t, ch)
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err == nil {
		t.Fatal("turn 2: want error")
	}
	if _, err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	text := e.log.b.String()
	if n := strings.Count(text, "turn total:"); n != 1 {
		t.Errorf("%d turn total lines, want 1 (the failed turn has none):\n%s", n, text)
	}
	last := lastLogLine(text)
	if !strings.HasPrefix(last, "session total: steps 3, in 300, cached 150, out 30, reasoning 0, wall ") || !strings.HasSuffix(last, ", compactions 0") {
		t.Errorf("last line = %q", last)
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
		t.Fatalf("turn 3: %v", err)
	}
	user := lastUser(ch.got[3])
	for _, want := range []string{"Portion 2 is served", "4. D — ", `New reply from the listener: "меньше такого"`} {
		if !strings.Contains(user, want) {
			t.Errorf("turn 3: user does not contain %q:\n%s", want, user)
		}
	}
	if strings.Contains(user, "Listener replies:") {
		t.Errorf("turn 3: user contains \"Listener replies:\n%s", user)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 4: %v", err)
	}
	if user := lastUser(ch.got[4]); !strings.Contains(user, "Portion 3 is served") {
		t.Errorf("turn 4: user does not announce portion 3:\n%s", user)
	}
	h := e.history()
	for _, want := range []string{"Listener replies:", `- after portion 2: "меньше такого"`} {
		if !strings.Contains(h, want) {
			t.Errorf("history does not contain %q:\n%s", want, h)
		}
	}
	log := e.log.b.String()
	for _, want := range []string{"## listener: (stream)", "## listener: меньше такого"} {
		if !strings.Contains(log, want) {
			t.Errorf("log does not contain %q:\n%s", want, log)
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
		t.Fatalf("turn 1: %v", err)
	}
	key := songArtists(Song{Artist: "Ghostemane", Title: "X"})[0]
	user := lastUser(ch.got[0])
	want := "в духе X\n\n" + globalBlock(e.sig.globalSkips()) + "\n\n" + skippedBlock(e.sig.skippedElsewhere("s1"))
	if user != want {
		t.Errorf("first message:\n%s\nwant:\n%s", user, want)
	}
	if !strings.Contains(user, "- "+key+": early skips 2 in 2 sessions, played through 0; queries: \"q1\", \"q2\"") {
		t.Errorf("no stats line:\n%s", user)
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
	want := "In past sessions the listener dropped these before the middle:\n" +
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
		t.Fatalf("turn 1: %v", err)
	}
	user := lastUser(ch.got[0])
	want := "в духе X\n\n" + skippedBlock(e.sig.skippedElsewhere("s1"))
	if user != want {
		t.Errorf("first message:\n%s\nwant:\n%s", user, want)
	}
}

func TestHistorySkippedBlock(t *testing.T) {
	e := newTestEngine(t, &fakeChatter{})
	if h := e.history(); strings.Contains(h, "In past sessions the listener dropped") {
		t.Fatalf("history without records contains the block:\n%s", h)
	}
	addSignals(t, e.sig, signalRec{Time: testNow, Kind: "skip_mid", Session: "other", Artist: "SoFaygo", Title: "Knock Knock"})
	h := e.history()
	want := skippedBlock(e.sig.skippedElsewhere("s1"))
	if want == "" {
		t.Fatal("skippedBlock is empty, the test is meaningless")
	}
	if !strings.HasSuffix(h, want) {
		t.Errorf("history does not end with the block:\n%s\nwant suffix:\n%s", h, want)
	}
}

// A4: every continuation delta ends with the Window summary once a portion has closed.
func TestTurnDeltaWindowSummary(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
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
	}}
	e := newTestEngine(t, ch)
	e.session = NewSession(nil, []Song{{ID: "f1", Artist: "A", Title: "FAV"}}, 3)
	e.agent.Session = e.session
	if _, err := e.Turn(context.Background(), "в духе X"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	user := lastUser(ch.got[2])
	want := "Window, the last 3 served tracks: by artist — 3 artists with 1 each; by Favorites artists: 1; by referents: 0."
	if !strings.Contains(user, want) {
		t.Errorf("turn 2: the delta does not carry the window summary:\n%s", user)
	}
	if since, ws := strings.Index(user, "Since the last turn:"), strings.Index(user, want); since >= 0 && since > ws {
		t.Errorf("turn 2: the window summary must be the last line of the delta:\n%s", user)
	}

	s := NewSession(nil, nil, 5)
	s.SetPlan(Plan{Seeds: []Seed{{Artist: "X", Mode: "not_it"}}})
	for _, song := range []Song{
		{ID: "1", Artist: "X", Title: "a"},
		{ID: "2", Artist: "X", Title: "b"},
		{ID: "3", Artist: "Y", Title: "c"},
	} {
		if v := s.Offer(Suggestion{Artist: song.Artist, Title: song.Title}, song, true); !v.Accepted {
			t.Fatalf("%s — %s: rejected: %s", song.Artist, song.Title, v.Reason)
		}
	}
	s.ClosePortion()
	// by artist the summary lists songArtists keys, so X shows as its key "x";
	// referents keep the name as written in the plan
	want2 := "Window, the last 3 served tracks: by artist — x 2, 1 more artists with 1 each; by Favorites artists: 0; by referents: 2 (X 2)."
	if got := s.windowSummary(); got != want2 {
		t.Errorf("windowSummary = %q, want %q", got, want2)
	}
}

func TestGlobalBlock(t *testing.T) {
	if got := globalBlock(nil); got != "" {
		t.Errorf("globalBlock(nil) = %q, want \"\"", got)
	}
	got := globalBlock([]skipStat{{Artist: "ghostemane", Early: 2, Sessions: 2, Played: 1}})
	want := "Lasting skips from past sessions:\n" +
		"- ghostemane: early skips 2 in 2 sessions, played through 1\n" +
		"If they share a sound or a direction — write it to memory as one entry (memory_add or memory_replace), evidence — these numbers. Do not write about single artists to memory."
	if got != want {
		t.Errorf("globalBlock:\n%s\nwant:\n%s", got, want)
	}
}

func TestPromptRulesModelDecides(t *testing.T) {
	for _, want := range []string{
		"This is one conversation for the whole stream.",
		"The code checks only facts: that a track exists in Apple Music and that it has not played in this stream yet.",
		"search_tracks looks up to 10 queries at once",
		"by id from search_tracks, or by artist and title",
		"again: true only when the listener asked to hear a track once more",
		"library — the track is in the listener's library; favorite — in Favorites.",
		"in this stream — already played in this stream",
		"N× (last date)",
		"The signal is fatigue",
		"\"Something new\", \"something I don't know\"",
	} {
		if !strings.Contains(promptRules, want) {
			t.Errorf("promptRules does not contain %q", want)
		}
	}
	for _, gone := range []string{"known_left", "the code caps", "The code counts limits", "the limits scale", "the code lets no more", "the code blocks", "against the rules", "There is no limit on known artists"} {
		if strings.Contains(promptRules, gone) {
			t.Errorf("promptRules still contains %q", gone)
		}
	}
}

func TestPromptRulesReactionWording(t *testing.T) {
	if !strings.Contains(promptRules, "Step aside only from tracks played to the end") {
		t.Error("the new wording about stepping aside is missing")
	}
	if strings.Contains(promptRules, "may drift a step") {
		t.Error("the old drift wording is still there")
	}
}

func TestValidateNoAppleTokens(t *testing.T) {
	err := Config{}.Validate()
	if err == nil || !strings.Contains(err.Error(), "no Apple tokens") {
		t.Errorf("empty config: err = %v, want \"no Apple tokens\"", err)
	}
}

func TestValidateNoLLMKey(t *testing.T) {
	cfg := Config{AppleDeveloperToken: "dev", AppleUserToken: "user"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "no LLM key") {
		t.Errorf("without the LLM key: err = %v, want \"no LLM key\"", err)
	}
}

func TestCacheFresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if cacheFresh(dir, time.Hour, now) {
		t.Error("no files: want false")
	}
	lib := filepath.Join(dir, "library.json")
	favs := filepath.Join(dir, "favorites.json")
	for _, p := range []string{lib, favs} {
		if err := os.WriteFile(p, []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !cacheFresh(dir, time.Hour, now) {
		t.Error("both files, mtime now: want true")
	}
	old := now.Add(-2 * time.Hour)
	for _, p := range []string{lib, favs} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if cacheFresh(dir, time.Hour, now) {
		t.Error("mtime older than maxAge: want false")
	}
}

func TestLastfmClientNoKey(t *testing.T) {
	l := &lastfmClient{}
	_, err := l.call(context.Background(), "artist.getsimilar", url.Values{"artist": {"Yeat"}})
	if err == nil || err.Error() != "lastfm: no key" {
		t.Errorf("err = %v, want \"lastfm: no key\"", err)
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
		t.Errorf("fields from Song were not carried over: %+v", tr)
	}
	if tr.ArtworkURL != "https://example.com/a/{w}x{h}bb.jpg" || tr.DurationMs != 191000 {
		t.Errorf("ArtworkURL/DurationMs: %q %d", tr.ArtworkURL, tr.DurationMs)
	}
	if tr.Why != "рейдж" || !tr.Favorite {
		t.Errorf("Why/Favorite: %q %v", tr.Why, tr.Favorite)
	}
	if got[1].Favorite || got[1].Why != "" {
		t.Errorf("empty entry: %+v", got[1])
	}
}

func TestToTrackReleased(t *testing.T) {
	tr := toTrack(Entry{Song: Song{ID: "1", Released: "2020-12-25"}})
	if tr.Released != "2020-12-25" {
		t.Errorf("Released = %q, want 2020-12-25", tr.Released)
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
		t.Fatalf("turn 1: %v", err)
	}
	if p.Note != "без крика" {
		t.Errorf("Note = %q, want the submitted note", p.Note)
	}
	if p.Later != nil {
		t.Fatalf("turn 1: Later: %+v, want empty", p.Later)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if p2.Later == nil || p2.Later.Question != "убрать хоррор?" {
		t.Fatalf("Later: %+v", p2.Later)
	}
	if len(p2.Later.Tracks) != 2 || p2.Later.Tracks[0].Artist != "A" || p2.Later.Tracks[1].Artist != "B" {
		t.Errorf("Later.Tracks: %+v, want tracks 1 and 2 of the first portion", p2.Later.Tracks)
	}
	if len(p2.Tracks) != 3 {
		t.Fatalf("Tracks: %d, want 3", len(p2.Tracks))
	}
	log := e.log.b.String()
	for _, want := range []string{"note: без крика", "later question: убрать хоррор? [да нет]"} {
		if !strings.Contains(log, want) {
			t.Errorf("log does not contain %q:\n%s", want, log)
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
		t.Errorf("history does not contain the answer:\n%s", h)
	}
	if !strings.Contains(e.log.b.String(), `## listener answer: "дальше?" → дрим-поп (A — T1)`) {
		t.Errorf("log does not contain the listener answer:\n%s", e.log.b.String())
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
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if ui.asks != 1 {
		t.Errorf("UI.Ask called %d times, want 1", ui.asks)
	}
	warmup := `to the question "какая эпоха?" — поздний (B — T2)`
	if h := e.history(); !strings.Contains(h, warmup) {
		t.Errorf("history does not contain the warmup answer:\n%s", h)
	}
	wantErr := "error: ask_user is allowed only in the first turn of the session; ask a fork in the skips through ask_later"
	found := false
	for _, m := range ch.got[4] {
		if m.Role == "tool" && strings.Contains(m.Content, wantErr) {
			found = true
		}
	}
	if !found {
		t.Errorf("turn 2: the ask_user error is not visible in the next call:\n%+v", ch.got[4])
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if n := strings.Count(e.history(), warmup); n != 1 {
		t.Errorf("history carries the warmup answer %d times, want 1:\n%s", n, e.history())
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
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if h := e.history(); !strings.Contains(h, `Questions already asked (do not repeat): "убрать хоррор?"`) {
		t.Errorf("turn 3: history does not contain the asked question:\n%s", h)
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
		t.Fatalf("turn 1: %v", err)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if p2.Later == nil || len(p2.Later.Tracks) != 2 || p2.Later.Tracks[0].Artist != "A" || p2.Later.Tracks[1].Artist != "B" {
		t.Fatalf("Later.Tracks: %+v, want tracks 1 and 2 of the first portion", p2.Later)
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
			t.Errorf("history does not contain %q:\n%s", want, h)
		}
	}
}

func TestTurnTailAndPromptMentionRatings(t *testing.T) {
	ch := &fakeChatter{}
	flowEngine(t, ch)
	if user := lastUser(ch.got[2]); !strings.Contains(user, "▲ is the main referent") {
		t.Errorf("the stream tail does not contain \"▲ is the main referent\":\n%s", user)
	}
	if !strings.Contains(promptRules, "explicit ratings from the listener") {
		t.Error("promptRules does not contain \"explicit ratings from the listener\"")
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
		t.Fatalf("turn 1: %v", err)
	}
	start := len(ch.got)
	p, err := e.Turn(context.Background(), "чето мимо")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if len(p.Tracks) != 3 || p.Tracks[0].Artist != "E" {
		t.Errorf("portion 2: %+v, want E, F, G", p.Tracks)
	}
	if user := lastUser(ch.got[start]); strings.Contains(user, "Candidates from the previous turn") {
		t.Errorf("the reserve was not cleared after the reply:\n%s", user)
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
		t.Fatalf("turn 1: %v", err)
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
		t.Fatalf("turn 2: %v", err)
	}
	user := lastUser(ch.got[start])
	for _, want := range []string{"Skipped since the last question: A — T1; B — T2.", "call ask_later", "options are played-through tracks by their numbers above"} {
		if !strings.Contains(user, want) {
			t.Errorf("turn 2: user does not contain %q:\n%s", want, user)
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
		t.Fatalf("turn 2: %v", err)
	}
	if user := lastUser(ch.got[start]); strings.Contains(user, "Skipped since the last question") {
		t.Errorf("turn 2: a hint with a single skip:\n%s", user)
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
		t.Fatalf("turn 2: %v", err)
	}
	start := len(ch.got)
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if user := lastUser(ch.got[start]); strings.Contains(user, "Skipped since the last question") {
		t.Errorf("turn 3: the hint was not reset after ask_later:\n%s", user)
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
		t.Fatalf("turn 2: %v", err)
	}
	if user := lastUser(ch.got[start]); strings.Contains(user, "Skipped since the last question") {
		t.Errorf("turn 2: a hint on a reply:\n%s", user)
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
		t.Fatalf("turn: %v", err)
	}
	for _, r := range e.sig.recs {
		if r.Kind == "served" {
			t.Fatalf("the portion must not write served: %+v", r)
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
		t.Fatalf("turn 1: %v", err)
	}
	if len(p1.Tracks) != 3 {
		t.Fatalf("turn 1: %d tracks, want 3", len(p1.Tracks))
	}
	v := e.session.Offer(Suggestion{Artist: "Z", Title: "TZ", Why: "w"}, Song{ID: "z", Artist: "Z", Title: "TZ"}, true)
	if !v.Accepted {
		t.Fatalf("Z: rejected: %s", v.Reason)
	}
	p2, err := e.Turn(context.Background(), "")
	if err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	if len(p2.Tracks) != 3 {
		t.Fatalf("turn 2: %d tracks, want 3", len(p2.Tracks))
	}
	for i, want := range []string{"D", "E", "F"} {
		if p2.Tracks[i].Artist != want {
			t.Errorf("turn 2, track %d: %q, want %q", i, p2.Tracks[i].Artist, want)
		}
	}
	for _, tr := range p2.Tracks {
		if tr.Artist == "Z" {
			t.Errorf("turn 2 contains Z: %+v", p2.Tracks)
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
		t.Fatalf("turn 3: %v", err)
	}
	if len(p3.Tracks) != 3 {
		t.Fatalf("turn 3: %d tracks, want 3", len(p3.Tracks))
	}
	user := lastUser(ch.got[3])
	for _, gone := range []string{"D — T4", "E — T5", "F — T6"} {
		if strings.Contains(user, gone) {
			t.Errorf("user contains a track of the dropped portion %q:\n%s", gone, user)
		}
	}
	if !strings.Contains(user, "- portion 2 was dropped before it played: the listener replied") {
		t.Errorf("user does not contain the drop note:\n%s", user)
	}
	if !strings.Contains(e.history(), "A — T1") {
		t.Errorf("history lost a track of portion 1:\n%s", e.history())
	}
	if !strings.Contains(e.log.b.String(), "## portion 2 dropped: listener reply") {
		t.Errorf("no dropped-portion entry:\n%s", e.log.b.String())
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
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := e.Turn(context.Background(), ""); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	e.DropLast()
	if _, err := e.Turn(context.Background(), "потише"); err != nil {
		t.Fatalf("turn 3: %v", err)
	}
	if user := lastUser(ch.got[4]); strings.Contains(user, "убрать хоррор?") {
		t.Errorf("turn 3: the question of the dropped portion is still among the asked ones:\n%s", user)
	}
}

func TestEngineNoteSavedFavoriteUpdatesPromptOnce(t *testing.T) {
	e := newTestEngine(t, &fakeChatter{})
	song := Song{ID: "1", Artist: "New Artist", Title: "Saved Song", Album: "Saved Album"}
	line := "- New Artist — Saved Song (Saved Album)\n"
	e.NoteSaved(song, true)
	e.NoteSaved(song, true)
	msg := e.agent.msgs[0].Content
	if n := strings.Count(msg, line); n != 1 {
		t.Errorf("system message contains the line %d times, want 1", n)
	}
}

func TestEngineNoteSavedLibraryLeavesStateAlone(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "memory.md")
	mem, err := memory.Open(memPath, memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	if err := mem.Add("likes long intros", "the listener said so"); err != nil {
		t.Fatalf("memory.Add: %v", err)
	}
	sigPath := filepath.Join(dir, "signals.jsonl")
	sig, err := OpenSignals(sigPath)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if err := sig.append(signalRec{Time: testNow, Session: "s1", Kind: "played", ID: "x"}); err != nil {
		t.Fatalf("signals append: %v", err)
	}

	e := newTestEngine(t, &fakeChatter{})
	e.mem = mem
	e.sig = sig
	promptBefore := e.agent.msgs[0].Content
	memoryBefore := readFileBytes(t, memPath)
	signalsBefore := readFileBytes(t, sigPath)

	e.NoteSaved(Song{ID: "1", Artist: "New Artist", Title: "Saved Song"}, false)

	if got := e.agent.msgs[0].Content; got != promptBefore {
		t.Errorf("system message changed:\n%s", got)
	}
	if got := readFileBytes(t, memPath); !bytes.Equal(got, memoryBefore) {
		t.Errorf("memory file changed: %q -> %q", memoryBefore, got)
	}
	if got := readFileBytes(t, sigPath); !bytes.Equal(got, signalsBefore) {
		t.Errorf("signals file changed: %q -> %q", signalsBefore, got)
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: a file inside the test's temp dir
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// Songs the listener queued by hand are never offered again in the session.
func TestServedSongsAreNotRepeated(t *testing.T) {
	ch := &fakeChatter{resps: []llm.Response{
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"A","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"A","title":"T1","why":"w"},
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"}
		]}`)),
		toolResp(call("c3", "submit_candidates", `{"tracks":[
			{"artist":"D","title":"T4","why":"w"}
		]}`)),
	}}
	e := newTestEngine(t, ch)
	e.Served([]Song{{ID: "x1", Artist: "A", Title: "T1"}})
	p, err := e.Turn(context.Background(), "after the album")
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	var got []string
	for _, tr := range p.Tracks {
		got = append(got, tr.Artist+" — "+tr.Title)
	}
	if want := "B — T2,C — T3,D — T4"; strings.Join(got, ",") != want {
		t.Fatalf("portion = %v, want %s", got, want)
	}
}

// A named session stores its name, not the request text, with each signal.
func TestLabelNamesSignals(t *testing.T) {
	ch := &fakeChatter{}
	e := newTestEngine(t, ch)
	e.Label("Blonde")
	ch.resps = append(ch.resps,
		toolResp(call("c1", "set_plan", `{"seeds":[{"artist":"A","mode":"and_similar"}],"exclusions":[]}`)),
		toolResp(call("c2", "submit_candidates", `{"tracks":[
			{"artist":"B","title":"T2","why":"w"},
			{"artist":"C","title":"T3","why":"w"},
			{"artist":"D","title":"T4","why":"w"}
		]}`)),
	)
	if _, err := e.Turn(context.Background(), "The listener put on the album \"Blonde\".\n1. A — T1"); err != nil {
		t.Fatalf("Turn: %v", err)
	}
	if err := e.Record([]Outcome{{ID: "x", Artist: "B", Title: "T2", Kind: "played"}}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got := e.sig.recs[len(e.sig.recs)-1].Query; got != "Blonde" {
		t.Fatalf("signal query = %q, want Blonde", got)
	}
	if !strings.Contains(e.history(), "1. A — T1") {
		t.Fatal("the history lost the request text")
	}
}
