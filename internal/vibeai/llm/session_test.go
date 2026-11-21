package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const openCodeBase = "https://opencode.ai/inference/go/v1"

// A15: the session header goes to OpenCode for chat and for model listing, nowhere else.
func TestChatOpenCodeSessionHeader(t *testing.T) {
	var got http.Header
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header.Clone()
		return httpResponse(http.StatusOK, okBody), nil
	})
	c := New(Config{BaseURL: openCodeBase, APIKey: "k", Model: "m"}, &http.Client{Transport: rt})
	c.sleep = func(time.Duration) {}
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if sess := got.Get("x-opencode-session"); !strings.HasPrefix(sess, "aux-") {
		t.Fatalf("x-opencode-session = %q, want an aux- prefix", sess)
	}
}

func TestChatNoSessionHeaderElsewhere(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(okBody))
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}, srv.Client())
	c.sleep = func(time.Duration) {}
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if sess := got.Get("x-opencode-session"); sess != "" {
		t.Fatalf("x-opencode-session = %q, want none outside OpenCode", sess)
	}
}

func TestListModelsOpenCodeSessionHeader(t *testing.T) {
	var got http.Header
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header.Clone()
		return httpResponse(http.StatusOK, `{"data":[]}`), nil
	})
	if _, err := ListModels(context.Background(), &http.Client{Transport: rt}, openCodeBase, "k"); err != nil {
		t.Fatal(err)
	}
	if sess := got.Get("x-opencode-session"); !strings.HasPrefix(sess, "aux-") {
		t.Fatalf("x-opencode-session = %q, want an aux- prefix", sess)
	}
}

func TestListModelsNoSessionHeaderElsewhere(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := ListModels(context.Background(), srv.Client(), srv.URL, "k"); err != nil {
		t.Fatal(err)
	}
	if sess := got.Get("x-opencode-session"); sess != "" {
		t.Fatalf("x-opencode-session = %q, want none outside OpenCode", sess)
	}
}
