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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/version"
)

type captured struct {
	method string
	path   string
	body   string
	header http.Header
}

func newTestClient(t *testing.T, respond func(n int) (int, string)) (*Client, func() []captured) {
	t.Helper()
	var mu sync.Mutex
	var reqs []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		n := len(reqs)
		reqs = append(reqs, captured{method: r.Method, path: r.URL.Path, body: string(b), header: r.Header.Clone()})
		mu.Unlock()
		code, out := respond(n)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, APIKey: "sk-secret-123", Model: "test-model"}, srv.Client())
	c.sleep = func(time.Duration) {}
	return c, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured(nil), reqs...)
	}
}

func newTestClientEffort(t *testing.T, effort string, respond func(n int) (int, string)) (*Client, func() []captured) {
	t.Helper()
	var mu sync.Mutex
	var reqs []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		n := len(reqs)
		reqs = append(reqs, captured{method: r.Method, path: r.URL.Path, body: string(b), header: r.Header.Clone()})
		mu.Unlock()
		code, out := respond(n)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, APIKey: "sk-secret-123", Model: "test-model", Effort: effort}, srv.Client())
	c.sleep = func(time.Duration) {}
	return c, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured(nil), reqs...)
	}
}

const okBody = `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`

func TestToolCallRoundtrip(t *testing.T) {
	const first = `{"choices":[{"message":{"role":"assistant","content":"","reasoning_content":"думаю","tool_calls":[{"id":"c1","type":"function","function":{"name":"describe","arguments":"{\"kind\":\"album\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`
	c, reqs := newTestClient(t, func(n int) (int, string) {
		if n == 0 {
			return http.StatusOK, first
		}
		return http.StatusOK, okBody
	})
	resp, err := c.Chat(context.Background(),
		[]Message{{Role: "user", Content: "что за трек?"}},
		[]Tool{{Name: "describe", Description: "описание", Parameters: json.RawMessage(`{"type":"object"}`)}},
		false)
	if err != nil {
		t.Fatal(err)
	}
	want := ToolCall{ID: "c1", Name: "describe", Arguments: `{"kind":"album"}`}
	if len(resp.Message.ToolCalls) != 1 || resp.Message.ToolCalls[0] != want {
		t.Fatalf("ToolCalls = %+v, want [%+v]", resp.Message.ToolCalls, want)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.FinishReason)
	}
	if !strings.Contains(string(resp.Message.Raw), `"reasoning_content":"думаю"`) {
		t.Errorf("Raw without reasoning_content: %s", resp.Message.Raw)
	}
	wantTools := `"tools":[{"type":"function","function":{"name":"describe","description":"описание","parameters":{"type":"object"}}}]`
	if !strings.Contains(reqs()[0].body, wantTools) {
		t.Errorf("the body has no tools in the R2 format: %s", reqs()[0].body)
	}

	resp2, err := c.Chat(context.Background(), []Message{
		{Role: "user", Content: "что за трек?"},
		resp.Message,
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Message.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp2.Message.Content)
	}
	body := reqs()[1].body
	if !strings.Contains(body, `"reasoning_content":"думаю"`) {
		t.Errorf("the second request has no reasoning_content: %s", body)
	}
	if !strings.Contains(body, `"tool_call_id":"c1"`) {
		t.Errorf("the second request has no tool_call_id: %s", body)
	}
}

func TestRequestHeaders(t *testing.T) {
	var mu sync.Mutex
	var rs []captured
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		rs = append(rs, captured{method: r.Method, path: r.URL.Path, header: r.Header.Clone()})
		mu.Unlock()
		return httpResponse(http.StatusOK, okBody), nil
	})
	c := New(Config{BaseURL: "https://opencode.ai", APIKey: "sk-secret-123", Model: "test-model"}, &http.Client{Transport: rt})
	c.sleep = func(time.Duration) {}
	for range 2 {
		if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(rs) != 2 {
		t.Fatalf("requests %d, want 2", len(rs))
	}
	for i, r := range rs {
		if r.method != http.MethodPost {
			t.Errorf("request %d: method = %q, want POST", i, r.method)
		}
		if r.path != "/chat/completions" {
			t.Errorf("request %d: path = %q", i, r.path)
		}
		if got := r.header.Get("Authorization"); got != "Bearer sk-secret-123" {
			t.Errorf("request %d: Authorization = %q", i, got)
		}
		if got := r.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("request %d: Content-Type = %q", i, got)
		}
		if got := r.header.Get("User-Agent"); got != "aux/"+version.Version {
			t.Errorf("request %d: User-Agent = %q", i, got)
		}
		sess := r.header.Get("x-opencode-session")
		if !strings.HasPrefix(sess, "aux-") {
			t.Errorf("request %d: session = %q, no aux- prefix", i, sess)
		}
		if len(sess) != len("aux-")+32 {
			t.Errorf("request %d: session = %q, want 16 bytes of hex", i, sess)
		}
		if i > 0 && sess != rs[0].header.Get("x-opencode-session") {
			t.Errorf("request %d: session %q != %q", i, sess, rs[0].header.Get("x-opencode-session"))
		}
	}
}

func TestRetryOn500(t *testing.T) {
	c, reqs := newTestClient(t, func(n int) (int, string) {
		if n == 0 {
			return http.StatusInternalServerError, "boom"
		}
		return http.StatusOK, okBody
	})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Message.Content)
	}
	if n := len(reqs()); n != 2 {
		t.Errorf("requests %d, want 2", n)
	}
}

func TestRetryExhaustedOnEmptyMessage(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) {
		return http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":""}}]}`
	})
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err == nil {
		t.Fatal("want error")
	}
	if n := len(reqs()); n != 3 {
		t.Errorf("requests %d, want 3", n)
	}
}

func TestJSONModeRetry(t *testing.T) {
	c, reqs := newTestClient(t, func(n int) (int, string) {
		if n == 0 {
			return http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"не json"}}]}`
		}
		return http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"{\"a\":1}"}}]}`
	})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != `{"a":1}` {
		t.Errorf("Content = %q, want {\"a\":1}", resp.Message.Content)
	}
	if !strings.Contains(reqs()[0].body, `"response_format":{"type":"json_object"}`) {
		t.Errorf("the body has no response_format: %s", reqs()[0].body)
	}
}

func TestUnauthorizedNoRetry(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusUnauthorized, "bad key" })
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("err = %v, no HTTP 401", err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("requests %d, want 1", n)
	}
	if strings.Contains(err.Error(), "sk-secret-123") {
		t.Errorf("the error contains the key: %v", err)
	}
}

func TestChatContextLengthError(t *testing.T) {
	const tooLong = `{"error":{"message":"This model's maximum context length is 128000 tokens. However, your messages resulted in 130000 tokens.","code":"context_length_exceeded"}}`
	cases := []struct {
		name string
		code int
		body string
		want bool
	}{
		{"400 context length", http.StatusBadRequest, tooLong, true},
		{"400 other error", http.StatusBadRequest, `{"error":{"message":"invalid tool schema"}}`, false},
		{"413 prompt too long", http.StatusRequestEntityTooLarge, "prompt is too long", true},
	}
	for _, tc := range cases {
		c, _ := newTestClient(t, func(int) (int, string) { return tc.code, tc.body })
		_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
		if err == nil {
			t.Fatalf("%s: want error", tc.name)
		}
		if got := errors.Is(err, ErrContextLength); got != tc.want {
			t.Errorf("%s: errors.Is(err, ErrContextLength) = %v, want %v (err = %v)", tc.name, got, tc.want, err)
		}
	}
}

func TestNoToolsInBody(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusOK, okBody })
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reqs()[0].body, `"tools"`) {
		t.Errorf("the body contains tools: %s", reqs()[0].body)
	}
}

func TestLoadEnv(t *testing.T) {
	p := filepath.Join(t.TempDir(), "llm.env")
	content := "# комментарий\nLLM_BASE_URL=http://localhost:4096\nLLM_API_KEY=sk-test\n\nLLM_MODEL=qwen3\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{BaseURL: "http://localhost:4096", APIKey: "sk-test", Model: "qwen3"}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}

	p2 := filepath.Join(t.TempDir(), "nokey.env")
	if err := os.WriteFile(p2, []byte("LLM_BASE_URL=http://localhost:4096\nLLM_MODEL=qwen3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadEnv(p2)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "http://localhost:4096") || strings.Contains(err.Error(), "qwen3") {
		t.Errorf("the error contains the values: %v", err)
	}
}

func TestUsage(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"completion_tokens_details":{"reasoning_tokens":3}}}`
	c, _ := newTestClient(t, func(int) (int, string) { return http.StatusOK, body })
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	want := Usage{PromptTokens: 10, CompletionTokens: 5, ReasoningTokens: 3}
	if resp.Usage != want {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
}

func chatUsage(t *testing.T, body string) Usage {
	t.Helper()
	c, _ := newTestClient(t, func(int) (int, string) { return http.StatusOK, body })
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return resp.Usage
}

func TestUsageCached(t *testing.T) {
	u := chatUsage(t, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":64}}}`)
	if u.CachedTokens != 64 || !u.CachedKnown || u.PromptTokens != 100 {
		t.Errorf("Usage = %+v, want cached 64 known, prompt 100", u)
	}
}

func TestUsageCachedZero(t *testing.T) {
	u := chatUsage(t, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":0}}}`)
	if u.CachedTokens != 0 || !u.CachedKnown {
		t.Errorf("Usage = %+v, want cached 0 known", u)
	}
}

func TestUsageCachedDeepSeek(t *testing.T) {
	u := chatUsage(t, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_cache_hit_tokens":40}}`)
	if u.CachedTokens != 40 || !u.CachedKnown {
		t.Errorf("Usage = %+v, want cached 40 known", u)
	}
}

func TestUsageCachedAbsent(t *testing.T) {
	cases := []struct{ name, body string }{
		{"no details", `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"completion_tokens_details":{"reasoning_tokens":3}}}`},
		{"details without cached_tokens", `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"other":1}}}`},
	}
	for _, tc := range cases {
		if u := chatUsage(t, tc.body); u.CachedKnown {
			t.Errorf("%s: Usage = %+v, want cached unknown", tc.name, u)
		}
	}
}

func TestEffortOpenAIStyle(t *testing.T) {
	c, reqs := newTestClientEffort(t, "high", func(int) (int, string) { return http.StatusOK, okBody })
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	body := reqs()[0].body
	if !strings.Contains(body, `"reasoning_effort":"high"`) {
		t.Errorf("the body has no reasoning_effort: %s", body)
	}
	if strings.Contains(body, `"reasoning":{`) {
		t.Errorf("the body contains a reasoning object: %s", body)
	}
}

func TestEffortOpenRouter(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(okBody)),
		}, nil
	})
	c := New(Config{BaseURL: "https://openrouter.ai/api/v1", APIKey: "sk-or-test", Model: "test-model", Effort: "low"}, &http.Client{Transport: rt})
	c.sleep = func(time.Duration) {}
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	body := bodies[0]
	if !strings.Contains(body, `"reasoning":{"effort":"low"}`) {
		t.Errorf("the body has no reasoning object: %s", body)
	}
	if strings.Contains(body, "reasoning_effort") {
		t.Errorf("the body contains reasoning_effort: %s", body)
	}
}

func TestNoEffort(t *testing.T) {
	c, reqs := newTestClientEffort(t, "", func(int) (int, string) { return http.StatusOK, okBody })
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	body := reqs()[0].body
	if strings.Contains(body, "reasoning_effort") || strings.Contains(body, `"reasoning"`) {
		t.Errorf("the body contains reasoning without Effort: %s", body)
	}
}
