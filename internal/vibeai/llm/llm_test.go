package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
		t.Errorf("Raw без reasoning_content: %s", resp.Message.Raw)
	}
	wantTools := `"tools":[{"type":"function","function":{"name":"describe","description":"описание","parameters":{"type":"object"}}}]`
	if !strings.Contains(reqs()[0].body, wantTools) {
		t.Errorf("в теле нет tools в формате R2: %s", reqs()[0].body)
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
		t.Errorf("второй запрос без reasoning_content: %s", body)
	}
	if !strings.Contains(body, `"tool_call_id":"c1"`) {
		t.Errorf("второй запрос без tool_call_id: %s", body)
	}
}

func TestRequestHeaders(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusOK, okBody })
	for i := 0; i < 2; i++ {
		if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	rs := reqs()
	if len(rs) != 2 {
		t.Fatalf("запросов %d, want 2", len(rs))
	}
	for i, r := range rs {
		if r.method != http.MethodPost {
			t.Errorf("запрос %d: method = %q, want POST", i, r.method)
		}
		if r.path != "/chat/completions" {
			t.Errorf("запрос %d: path = %q", i, r.path)
		}
		if got := r.header.Get("Authorization"); got != "Bearer sk-secret-123" {
			t.Errorf("запрос %d: Authorization = %q", i, got)
		}
		if got := r.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("запрос %d: Content-Type = %q", i, got)
		}
		if got := r.header.Get("User-Agent"); got != "vibelab/0.1" {
			t.Errorf("запрос %d: User-Agent = %q", i, got)
		}
		sess := r.header.Get("x-opencode-session")
		if !strings.HasPrefix(sess, "vibelab-") {
			t.Errorf("запрос %d: session = %q, нет префикса vibelab-", i, sess)
		}
		if len(sess) != len("vibelab-")+32 {
			t.Errorf("запрос %d: session = %q, want 16 байт hex", i, sess)
		}
		if i > 0 && sess != rs[0].header.Get("x-opencode-session") {
			t.Errorf("запрос %d: session %q != %q", i, sess, rs[0].header.Get("x-opencode-session"))
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
		t.Errorf("запросов %d, want 2", n)
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
		t.Errorf("запросов %d, want 3", n)
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
		t.Errorf("в теле нет response_format: %s", reqs()[0].body)
	}
}

func TestUnauthorizedNoRetry(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusUnauthorized, "bad key" })
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("err = %v, нет HTTP 401", err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("запросов %d, want 1", n)
	}
	if strings.Contains(err.Error(), "sk-secret-123") {
		t.Errorf("ошибка содержит ключ: %v", err)
	}
}

func TestNoToolsInBody(t *testing.T) {
	c, reqs := newTestClient(t, func(int) (int, string) { return http.StatusOK, okBody })
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reqs()[0].body, `"tools"`) {
		t.Errorf("тело содержит tools: %s", reqs()[0].body)
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
		t.Errorf("ошибка содержит значения: %v", err)
	}
}

func TestUsage(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"completion_tokens_details":{"reasoning_tokens":3}}}`
	c, _ := newTestClient(t, func(int) (int, string) { return http.StatusOK, body })
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage != (Usage{10, 5, 3}) {
		t.Errorf("Usage = %+v, want {10 5 3}", resp.Usage)
	}
}
