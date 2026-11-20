package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTokens struct {
	mu      sync.Mutex
	token   string
	refresh string
	account string
	forced  []bool
	err     error
}

func (f *fakeTokens) Access(_ context.Context, force bool) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forced = append(f.forced, force)
	if f.err != nil {
		return "", "", f.err
	}
	if force && f.refresh != "" {
		return f.refresh, f.account, nil
	}
	return f.token, f.account, nil
}

func (f *fakeTokens) calls() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.forced...)
}

func newTestResponses(t *testing.T, cfg Config, tok Tokens, onLimits func(Limits), respond func(n int) (int, http.Header, string)) (*ResponsesClient, func() []captured) {
	t.Helper()
	var mu sync.Mutex
	var reqs []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		n := len(reqs)
		reqs = append(reqs, captured{method: r.Method, path: r.URL.Path, body: string(b), header: r.Header.Clone()})
		mu.Unlock()
		code, hdr, out := respond(n)
		for k, vs := range hdr {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	if cfg.BaseURL == "" {
		cfg.BaseURL = srv.URL
	}
	c := NewResponses(cfg, tok, srv.Client(), onLimits)
	c.sleep = func(time.Duration) {}
	return c, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured(nil), reqs...)
	}
}

func readSSE(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decodeBody(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("body: %v: %s", err, s)
	}
	return m
}

func TestResponsesToolCall(t *testing.T) {
	c, _ := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_tool_call.sse")
		})
	resp, err := c.Chat(context.Background(),
		[]Message{{Role: "user", Content: "call echo"}},
		[]Tool{{Name: "echo", Description: "echo text", Parameters: json.RawMessage(`{"type":"object"}`)}}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ToolCall{ID: "call_ea385f4de28a4d5cbd9eef7cf0e90089", Name: "echo", Arguments: `{"text":"hi"}`}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0] != want {
		t.Fatalf("ToolCalls = %+v, want [%+v]", resp.Message.ToolCalls, want)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}
	if resp.Usage.PromptTokens != 135 || resp.Usage.CompletionTokens != 17 {
		t.Errorf("Usage = %+v, want prompt 135 completion 17", resp.Usage)
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(resp.Message.Raw, &raw); err != nil {
		t.Fatalf("Raw is not a JSON array: %v: %s", err, resp.Message.Raw)
	}
	if len(raw) != 1 {
		t.Errorf("Raw length = %d, want 1", len(raw))
	}
}

func TestResponsesMessage(t *testing.T) {
	c, _ := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_message.sse")
		})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "done" {
		t.Errorf("Content = %q, want done", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %+v, want none", resp.Message.ToolCalls)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", resp.FinishReason)
	}
}

func TestResponsesRequest(t *testing.T) {
	tools := []Tool{{
		Name:        "echo",
		Description: "echo text",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
	}}
	c, reqs := newTestResponses(t, Config{Model: "m", Effort: "low"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_message.sse")
		})
	if _, err := c.Chat(context.Background(), []Message{
		{Role: "system", Content: "S1"},
		{Role: "system", Content: "S2"},
		{Role: "user", Content: "hi"},
	}, tools, false); err != nil {
		t.Fatal(err)
	}
	rs := reqs()
	if len(rs) != 1 {
		t.Fatalf("requests = %d, want 1", len(rs))
	}
	r := rs[0]
	if r.method != http.MethodPost || r.path != "/responses" {
		t.Errorf("request = %s %s, want POST /responses", r.method, r.path)
	}
	for name, want := range map[string]string{
		"Authorization":      "Bearer tok1",
		"chatgpt-account-id": "acc1",
		"originator":         "codex_cli_rs",
		"Accept":             "text/event-stream",
	} {
		if got := r.header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	session := r.header.Get("session_id")
	if session == "" {
		t.Error("session_id header is empty")
	}
	body := decodeBody(t, r.body)
	if body["instructions"] != "S1\n\nS2" {
		t.Errorf("instructions = %q, want %q", body["instructions"], "S1\n\nS2")
	}
	wantInput := []any{map[string]any{
		"role":    "user",
		"content": []any{map[string]any{"type": "input_text", "text": "hi"}},
	}}
	if input, ok := body["input"].([]any); !ok || !reflect.DeepEqual(input, wantInput) {
		t.Errorf("input = %#v, want %#v", body["input"], wantInput)
	}
	wantTool := map[string]any{
		"type":        "function",
		"name":        "echo",
		"description": "echo text",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
	}
	if got, ok := body["tools"].([]any); !ok || len(got) != 1 || !reflect.DeepEqual(got[0], wantTool) {
		t.Errorf("tools = %#v, want [%#v]", body["tools"], wantTool)
	}
	if body["store"] != false {
		t.Errorf("store = %v, want false", body["store"])
	}
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
	if !reflect.DeepEqual(body["include"], []any{"reasoning.encrypted_content"}) {
		t.Errorf("include = %#v, want reasoning.encrypted_content", body["include"])
	}
	if !reflect.DeepEqual(body["reasoning"], map[string]any{"effort": "low"}) {
		t.Errorf("reasoning = %#v, want effort low", body["reasoning"])
	}
	if body["prompt_cache_key"] != session {
		t.Errorf("prompt_cache_key = %q, want session %q", body["prompt_cache_key"], session)
	}
}

func TestResponsesSecondTurn(t *testing.T) {
	tools := []Tool{{Name: "echo", Description: "echo text", Parameters: json.RawMessage(`{"type":"object"}`)}}
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_tool_call.sse")
		})
	first, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, tools, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Chat(context.Background(), []Message{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "hi"},
		first.Message,
		{Role: "tool", ToolCallID: "call_ea385f4de28a4d5cbd9eef7cf0e90089", Content: "hi"},
	}, tools, false); err != nil {
		t.Fatal(err)
	}
	body := decodeBody(t, reqs()[1].body)
	input, ok := body["input"].([]any)
	if !ok || len(input) != 3 {
		t.Fatalf("input = %#v, want 3 items", body["input"])
	}
	call, ok := input[1].(map[string]any)
	if !ok {
		t.Fatalf("input[1] = %#v", input[1])
	}
	if _, ok := call["id"]; ok {
		t.Errorf("replayed function_call keeps id: %#v", call)
	}
	if _, ok := call["status"]; ok {
		t.Errorf("replayed function_call keeps status: %#v", call)
	}
	if call["call_id"] != "call_ea385f4de28a4d5cbd9eef7cf0e90089" || call["name"] != "echo" || call["arguments"] != `{"text":"hi"}` {
		t.Errorf("replayed function_call = %#v", call)
	}
	wantOut := map[string]any{
		"type":    "function_call_output",
		"call_id": "call_ea385f4de28a4d5cbd9eef7cf0e90089",
		"output":  "hi",
	}
	if !reflect.DeepEqual(input[2], wantOut) {
		t.Errorf("input[2] = %#v, want %#v", input[2], wantOut)
	}
}

func TestResponsesReplayKeepsEncryptedContent(t *testing.T) {
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_message.sse")
		})
	raw := json.RawMessage(`[{"type":"reasoning","id":"r1","status":"completed","encrypted_content":"abc","summary":[]},{"type":"function_call","id":"fc","status":"completed","call_id":"c1","name":"n","arguments":"{}"}]`)
	if _, err := c.Chat(context.Background(), []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Raw: raw, ToolCalls: []ToolCall{{ID: "c1", Name: "n", Arguments: "{}"}}},
	}, nil, false); err != nil {
		t.Fatal(err)
	}
	input := decodeBody(t, reqs()[0].body)["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input = %#v, want 3 items (no duplicate function_call)", input)
	}
	reasoning := input[1].(map[string]any)
	if reasoning["encrypted_content"] != "abc" || reasoning["type"] != "reasoning" {
		t.Errorf("replayed reasoning = %#v, want encrypted_content preserved", reasoning)
	}
	if _, ok := reasoning["id"]; ok {
		t.Errorf("replayed reasoning keeps id: %#v", reasoning)
	}
	if _, ok := reasoning["status"]; ok {
		t.Errorf("replayed reasoning keeps status: %#v", reasoning)
	}
}

func TestResponsesLongEvent(t *testing.T) {
	padding := strings.Repeat("x", 100<<10)
	stream := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"instructions\":\"" + padding +
		"\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	c, _ := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, stream
		})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "done" {
		t.Errorf("Content = %q, want done", resp.Message.Content)
	}
}

func TestResponsesMixedHistory(t *testing.T) {
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_message.sse")
		})
	if _, err := c.Chat(context.Background(), []Message{
		{Role: "user", Content: "hi"},
		{
			Role:      "assistant",
			Raw:       json.RawMessage(`{"role":"assistant","content":null,"tool_calls":[]}`),
			ToolCalls: []ToolCall{{ID: "c1", Name: "n", Arguments: "{}"}},
		},
	}, nil, false); err != nil {
		t.Fatal(err)
	}
	input := decodeBody(t, reqs()[0].body)["input"].([]any)
	want := map[string]any{"type": "function_call", "call_id": "c1", "name": "n", "arguments": "{}"}
	if len(input) != 2 || !reflect.DeepEqual(input[1], want) {
		t.Errorf("input[1] = %#v, want %#v", input, want)
	}
}

func TestChatCompletionsReplaysResponsesArray(t *testing.T) {
	rc, _ := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, readSSE(t, "responses_tool_call.sse")
		})
	first, err := rc.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	msg := first.Message
	msg.ToolCalls = []ToolCall{{ID: "call_x", Name: "echo", Arguments: "{}"}}
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusOK, okBody })
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}, msg}, nil, false); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(reqs()[0].body), &body); err != nil {
		t.Fatal(err)
	}
	var got any
	if err := json.Unmarshal(body.Messages[1], &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"role":    "assistant",
		"content": "",
		"tool_calls": []any{map[string]any{
			"id":   "call_x",
			"type": "function",
			"function": map[string]any{
				"name":      "echo",
				"arguments": "{}",
			},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("message = %#v, want %#v", got, want)
	}
}

func TestResponsesLimits(t *testing.T) {
	T := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var got []Limits
	c, _ := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"},
		func(l Limits) { got = append(got, l) },
		func(int) (int, http.Header, string) {
			h := http.Header{}
			h.Set("x-codex-primary-used-percent", "12.5")
			h.Set("x-codex-secondary-used-percent", "71")
			h.Set("x-codex-primary-window-minutes", "300")
			h.Set("x-codex-secondary-window-minutes", "10080")
			h.Set("x-codex-primary-reset-after-seconds", "600")
			h.Set("x-codex-plan-type", "plus")
			return http.StatusOK, h, readSSE(t, "responses_message.sse")
		})
	c.now = func() time.Time { return T }
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	want := Limits{
		PrimaryPercent:   12.5,
		SecondaryPercent: 71,
		PrimaryWindow:    5 * time.Hour,
		SecondaryWindow:  168 * time.Hour,
		PrimaryReset:     T.Add(10 * time.Minute),
		Plan:             "plus",
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("onLimits = %+v, want exactly [%+v]", got, want)
	}
}

func TestResponses401Refresh(t *testing.T) {
	tok := &fakeTokens{token: "tok1", refresh: "tok2", account: "acc1"}
	c, reqs := newTestResponses(t, Config{Model: "m"}, tok, nil,
		func(n int) (int, http.Header, string) {
			if n == 0 {
				return http.StatusUnauthorized, nil, "bad key"
			}
			return http.StatusOK, nil, readSSE(t, "responses_message.sse")
		})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "done" {
		t.Errorf("Content = %q, want done", resp.Message.Content)
	}
	if got, want := tok.calls(), []bool{false, true}; !reflect.DeepEqual(got, want) {
		t.Errorf("Access force flags = %v, want %v", got, want)
	}
	rs := reqs()
	if len(rs) != 2 {
		t.Fatalf("requests = %d, want 2", len(rs))
	}
	if got := rs[1].header.Get("Authorization"); got != "Bearer tok2" {
		t.Errorf("second request Authorization = %q, want Bearer tok2", got)
	}
}

func TestResponses401Twice(t *testing.T) {
	tok := &fakeTokens{token: "tok1", refresh: "tok2", account: "acc1"}
	c, reqs := newTestResponses(t, Config{Model: "m"}, tok, nil,
		func(int) (int, http.Header, string) {
			return http.StatusUnauthorized, nil, "bad key"
		})
	var slept []time.Duration
	c.sleep = func(d time.Duration) { slept = append(slept, d) }
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
	if n := len(reqs()); n != 2 {
		t.Errorf("requests = %d, want 2", n)
	}
	if len(slept) != 0 {
		t.Errorf("slept = %v, want none", slept)
	}
}

func TestResponsesUsageLimit(t *testing.T) {
	const body = `{"error":{"type":"usage_limit_reached","message":"You've hit your usage limit.","resets_in_seconds":4800}}`
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusTooManyRequests, nil, body
		})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v, want *LimitError", err)
	}
	if le.ResetsIn != 80*time.Minute {
		t.Errorf("ResetsIn = %v, want 80m", le.ResetsIn)
	}
	if !strings.Contains(err.Error(), "subscription limit reached") {
		t.Errorf("err = %v, want subscription limit message", err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}
}

func TestResponsesRetries(t *testing.T) {
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(n int) (int, http.Header, string) {
			if n < 2 {
				return http.StatusInternalServerError, nil, "boom"
			}
			return http.StatusOK, nil, readSSE(t, "responses_tool_call.sse")
		})
	var slept []time.Duration
	c.sleep = func(d time.Duration) { slept = append(slept, d) }
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Errorf("ToolCalls = %+v, want one", resp.Message.ToolCalls)
	}
	if n := len(reqs()); n != 3 {
		t.Errorf("requests = %d, want 3", n)
	}
	if want := []time.Duration{time.Second, 2 * time.Second}; !reflect.DeepEqual(slept, want) {
		t.Errorf("slept = %v, want %v", slept, want)
	}
}

func TestResponsesStreamWithoutCompleted(t *testing.T) {
	const stream = "event: response.created\ndata: {\"type\":\"response.created\"}\n\n"
	c, reqs := newTestResponses(t, Config{Model: "m"}, &fakeTokens{token: "tok1", account: "acc1"}, nil,
		func(int) (int, http.Header, string) {
			return http.StatusOK, nil, stream
		})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err == nil {
		t.Fatal("want error")
	}
	if n := len(reqs()); n != 3 {
		t.Errorf("requests = %d, want 3", n)
	}
}

func TestListChatGPTModels(t *testing.T) {
	const body = `{"models":[{"slug":"b","visibility":"list","supported_in_api":true,"priority":2,"supported_reasoning_levels":[{"effort":"low"}]},{"slug":"a","visibility":"list","supported_in_api":true,"priority":1,"supported_reasoning_levels":[]},{"slug":"h","visibility":"hide","supported_in_api":true,"priority":0}]}`
	var path, query, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		query = r.URL.RawQuery
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	got, err := ListChatGPTModels(context.Background(), srv.Client(), srv.URL, &fakeTokens{token: "tok1", account: "acc1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelInfo{
		{ID: "a", Tools: true, Known: true},
		{ID: "b", Tools: true, Reasoning: true, Known: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListChatGPTModels = %+v, want %+v", got, want)
	}
	if path != "/models" {
		t.Errorf("path = %q, want /models", path)
	}
	if query != "client_version="+CodexClientVersion {
		t.Errorf("query = %q, want client_version=%s", query, CodexClientVersion)
	}
	if auth != "Bearer tok1" {
		t.Errorf("Authorization = %q, want Bearer tok1", auth)
	}
}

func TestListChatGPTModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	_, err := ListChatGPTModels(context.Background(), srv.Client(), srv.URL, &fakeTokens{token: "bad", account: "acc1"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}
