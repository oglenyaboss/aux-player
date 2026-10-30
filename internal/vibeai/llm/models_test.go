package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/version"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpResponse(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestListModelsFlags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"b","supported_parameters":["tools","reasoning"]},{"id":"a","supported_parameters":["max_tokens"]},{"id":"c","supported_parameters":[]}]}`))
	}))
	t.Cleanup(srv.Close)
	got, err := ListModels(context.Background(), srv.Client(), srv.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelInfo{
		{ID: "a", Known: true},
		{ID: "b", Tools: true, Reasoning: true, Known: true},
		{ID: "c", Known: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListModels = %+v, want %+v", got, want)
	}
}

func TestListModelsPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"x","object":"model"}]}`))
	}))
	t.Cleanup(srv.Close)
	got, err := ListModels(context.Background(), srv.Client(), srv.URL, "k")
	if err != nil {
		t.Fatal(err)
	}
	want := []ModelInfo{{ID: "x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListModels = %+v, want %+v", got, want)
	}
}

func TestListModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	_, err := ListModels(context.Background(), srv.Client(), srv.URL, "bad-key")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestListModelsOpenRouterKey(t *testing.T) {
	var modelsReqs int
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/key") {
			return httpResponse(http.StatusUnauthorized, "bad key"), nil
		}
		modelsReqs++
		return httpResponse(http.StatusOK, `{"data":[{"id":"x"}]}`), nil
	})
	_, err := ListModels(context.Background(), &http.Client{Transport: rt}, "https://openrouter.ai/api/v1", "bad-key")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
	if modelsReqs != 0 {
		t.Errorf("запросов к /models = %d, want 0", modelsReqs)
	}
}

func TestListModelsOpenRouterOK(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/key") {
			return httpResponse(http.StatusOK, `{"data":{"label":"free"}}`), nil
		}
		return httpResponse(http.StatusOK, `{"data":[{"id":"m-1","object":"model"}]}`), nil
	})
	got, err := ListModels(context.Background(), &http.Client{Transport: rt}, "https://openrouter.ai/api/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "m-1" {
		t.Errorf("ListModels = %+v, want одну модель m-1", got)
	}
}

func TestListModelsHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := ListModels(context.Background(), srv.Client(), srv.URL, "k"); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer k" {
		t.Errorf("Authorization = %q, want Bearer k", got.Get("Authorization"))
	}
	if got.Get("User-Agent") != "aux/"+version.Version {
		t.Errorf("User-Agent = %q, want aux/%s", got.Get("User-Agent"), version.Version)
	}
	if sess := got.Get("x-opencode-session"); sess == "" || !strings.HasPrefix(sess, "aux-") {
		t.Errorf("x-opencode-session = %q, want непустой с префиксом aux-", sess)
	}
}
