package llm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

var ErrUnauthorized = errors.New("llm: key rejected")

type ModelInfo struct {
	ID        string
	Tools     bool
	Reasoning bool
	Known     bool
}

// ListModels reads GET {baseURL}/models with the client's headers and checks the key.
// Sorted by ID. OpenRouter: the key is checked with GET {baseURL}/key first.
func ListModels(ctx context.Context, h *http.Client, baseURL, apiKey string) ([]ModelInfo, error) {
	if h == nil {
		h = http.DefaultClient
	}
	base := strings.TrimRight(baseURL, "/")
	session := newSession()
	if isOpenRouter(baseURL) {
		if err := verifyKey(ctx, h, base, apiKey, session); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("llm: models: %w", err)
	}
	setHeaders(req, apiKey, session)
	resp, err := h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("llm: models: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, statusError(resp.StatusCode, body)
	}
	return parseModels(body)
}

func verifyKey(ctx context.Context, h *http.Client, base, apiKey, session string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/key", nil)
	if err != nil {
		return fmt.Errorf("llm: models: %w", err)
	}
	setHeaders(req, apiKey, session)
	resp, err := h.Do(req)
	if err != nil {
		return fmt.Errorf("llm: models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("llm: models: %w", err)
	}
	if resp.StatusCode >= 400 {
		return statusError(resp.StatusCode, body)
	}
	return nil
}

func statusError(code int, body []byte) error {
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return fmt.Errorf("llm: models: HTTP %d: %w", code, ErrUnauthorized)
	}
	return fmt.Errorf("llm: models: HTTP %d: %s", code, bodySnippet(body))
}

func parseModels(b []byte) ([]ModelInfo, error) {
	var wire struct {
		Data []struct {
			ID                  string    `json:"id"`
			SupportedParameters *[]string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("llm: models: parsing the response: %w", err)
	}
	mods := make([]ModelInfo, 0, len(wire.Data))
	for _, m := range wire.Data {
		mi := ModelInfo{ID: m.ID, Known: m.SupportedParameters != nil}
		if m.SupportedParameters != nil {
			for _, p := range *m.SupportedParameters {
				switch p {
				case "tools":
					mi.Tools = true
				case "reasoning", "reasoning_effort":
					mi.Reasoning = true
				}
			}
		}
		mods = append(mods, mi)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	return mods, nil
}

func newSession() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "aux-" + hex.EncodeToString(b)
}

func isOpenRouter(baseURL string) bool {
	u, err := url.Parse(baseURL)
	return err == nil && u.Hostname() == "openrouter.ai"
}
