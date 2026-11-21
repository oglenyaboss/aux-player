package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oglenyaboss/aux-player/internal/version"
)

const ChatGPTBaseURL = "https://chatgpt.com/backend-api/codex"

const CodexClientVersion = "0.170.0"

type Tokens interface {
	Access(ctx context.Context, force bool) (token, accountID string, err error)
}

type Limits struct {
	PrimaryPercent   float64
	SecondaryPercent float64
	PrimaryWindow    time.Duration
	SecondaryWindow  time.Duration
	PrimaryReset     time.Time
	SecondaryReset   time.Time
	Plan             string
}

type LimitError struct {
	ResetsIn time.Duration
}

func (e *LimitError) Error() string {
	msg := "llm: subscription limit reached"
	if e.ResetsIn > 0 {
		msg += " (resets in " + e.ResetsIn.String() + ")"
	}
	return msg
}

type ResponsesClient struct {
	cfg      Config
	tok      Tokens
	http     *http.Client
	session  string
	sleep    func(time.Duration)
	now      func() time.Time
	onLimits func(Limits)
}

func NewResponses(cfg Config, tok Tokens, h *http.Client, onLimits func(Limits)) *ResponsesClient {
	if h == nil {
		h = http.DefaultClient
	}
	return &ResponsesClient{
		cfg:      cfg,
		tok:      tok,
		http:     h,
		session:  newSession(),
		sleep:    time.Sleep,
		now:      time.Now,
		onLimits: onLimits,
	}
}

func (c *ResponsesClient) Chat(ctx context.Context, msgs []Message, tools []Tool, jsonMode bool) (Response, error) {
	body, err := c.requestBody(msgs, tools, jsonMode)
	if err != nil {
		return Response{}, err
	}
	token, accountID, err := c.tok.Access(ctx, false)
	if err != nil {
		return Response{}, err
	}
	refreshed := false
	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			c.sleep(time.Duration(attempt) * time.Second)
		}
		resp, err := c.attempt(ctx, body, token, accountID)
		if err != nil && errors.Is(err, errResponse401) {
			if refreshed {
				return Response{}, err
			}
			refreshed = true
			token, accountID, err = c.tok.Access(ctx, true)
			if err != nil {
				return Response{}, err
			}
			resp, err = c.attempt(ctx, body, token, accountID)
		}
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var re retryableError
		if !errors.As(err, &re) {
			return Response{}, err
		}
	}
	return Response{}, lastErr
}

var errResponse401 = errors.New("llm: HTTP 401")

func (c *ResponsesClient) baseURL() string {
	if c.cfg.BaseURL != "" {
		return strings.TrimRight(c.cfg.BaseURL, "/")
	}
	return ChatGPTBaseURL
}

type responsesRequest struct {
	Model             string            `json:"model"`
	Instructions      string            `json:"instructions"`
	Input             []json.RawMessage `json:"input"`
	Tools             []responsesTool   `json:"tools,omitempty"`
	ToolChoice        string            `json:"tool_choice"`
	ParallelToolCalls bool              `json:"parallel_tool_calls"`
	Reasoning         *wireReasoning    `json:"reasoning,omitempty"`
	Store             bool              `json:"store"`
	Stream            bool              `json:"stream"`
	Include           []string          `json:"include"`
	PromptCacheKey    string            `json:"prompt_cache_key"`
	Text              *responsesText    `json:"text,omitempty"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type responsesText struct {
	Format wireFormat `json:"format"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesMessageItem struct {
	Role    string             `json:"role"`
	Content []responsesContent `json:"content"`
}

type responsesFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesFunctionOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func (c *ResponsesClient) requestBody(msgs []Message, tools []Tool, jsonMode bool) ([]byte, error) {
	input, instructions, err := responsesInput(msgs)
	if err != nil {
		return nil, err
	}
	req := responsesRequest{
		Model:             c.cfg.Model,
		Instructions:      instructions,
		Input:             input,
		ToolChoice:        "auto",
		ParallelToolCalls: true,
		Store:             false,
		Stream:            true,
		Include:           []string{"reasoning.encrypted_content"},
		PromptCacheKey:    c.session,
	}
	if c.cfg.Effort != "" {
		req.Reasoning = &wireReasoning{Effort: c.cfg.Effort}
	}
	if jsonMode {
		req.Text = &responsesText{Format: wireFormat{Type: "json_object"}}
	}
	for _, t := range tools {
		req.Tools = append(req.Tools, responsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return b, nil
}

func responsesInput(msgs []Message) ([]json.RawMessage, string, error) {
	input := []json.RawMessage{}
	var systems []string
	for i, m := range msgs {
		var item any
		switch m.Role {
		case "system":
			systems = append(systems, m.Content)
			continue
		case "user":
			item = responsesMessageItem{Role: "user", Content: []responsesContent{{Type: "input_text", Text: m.Content}}}
		case "assistant":
			if isJSONArray(m.Raw) {
				var items []map[string]json.RawMessage
				if err := json.Unmarshal(m.Raw, &items); err != nil {
					return nil, "", fmt.Errorf("llm: message %d: %w", i, err)
				}
				for _, it := range items {
					delete(it, "id")
					delete(it, "status")
					b, err := json.Marshal(it)
					if err != nil {
						return nil, "", fmt.Errorf("llm: message %d: %w", i, err)
					}
					input = append(input, b)
				}
				continue
			}
			var items []any
			if m.Content != "" {
				items = append(items, responsesMessageItem{Role: "assistant", Content: []responsesContent{{Type: "output_text", Text: m.Content}}})
			}
			for _, tc := range m.ToolCalls {
				items = append(items, responsesFunctionCall{Type: "function_call", CallID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
			}
			item = items
		case "tool":
			item = responsesFunctionOutput{Type: "function_call_output", CallID: m.ToolCallID, Output: m.Content}
		}
		switch v := item.(type) {
		case nil:
			continue
		case []any:
			for _, it := range v {
				b, err := json.Marshal(it)
				if err != nil {
					return nil, "", fmt.Errorf("llm: message %d: %w", i, err)
				}
				input = append(input, b)
			}
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, "", fmt.Errorf("llm: message %d: %w", i, err)
			}
			input = append(input, b)
		}
	}
	return input, strings.Join(systems, "\n\n"), nil
}

func isJSONArray(raw json.RawMessage) bool {
	return bytes.HasPrefix(bytes.TrimSpace(raw), []byte("["))
}

func (c *ResponsesClient) attempt(ctx context.Context, body []byte, token, accountID string) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/responses", bytes.NewReader(body))
	if err != nil {
		return Response{}, retryableError{fmt.Errorf("llm: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("session_id", c.session)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "aux/"+version.Version)

	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, retryableError{fmt.Errorf("llm: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	c.observeLimits(resp.Header)

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		httpErr := fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, bodySnippet(snippet))
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return Response{}, fmt.Errorf("%w: %w", errResponse401, ErrUnauthorized)
		case resp.StatusCode == http.StatusTooManyRequests:
			if le := parseLimitError(snippet, resp.Header); le != nil {
				return Response{}, le
			}
			return Response{}, retryableError{httpErr}
		case resp.StatusCode >= 500:
			return Response{}, retryableError{httpErr}
		default:
			return Response{}, statusError(resp.StatusCode, snippet)
		}
	}
	return c.readStream(resp.Body)
}

type responsesEvent struct {
	Type     string                `json:"type"`
	Message  string                `json:"message"`
	Error    *responsesError       `json:"error"`
	Response *responsesEventResult `json:"response"`
}

type responsesError struct {
	Message string `json:"message"`
}

type responsesEventResult struct {
	Output []json.RawMessage `json:"output"`
	Usage  *responsesUsage   `json:"usage"`
	Error  *responsesError   `json:"error"`
}

type responsesUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	OutputTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (c *ResponsesClient) readStream(r io.Reader) (Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var ev responsesEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return Response{}, fmt.Errorf("llm: parsing the event stream: %w", err)
		}
		switch ev.Type {
		case "response.completed":
			return completedResponse(ev.Response)
		case "response.failed", "error":
			return Response{}, eventError(ev)
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, retryableError{fmt.Errorf("llm: reading the event stream: %w", err)}
	}
	return Response{}, retryableError{errors.New("llm: stream ended without response.completed")}
}

func eventError(ev responsesEvent) error {
	msg := ev.Message
	if ev.Response != nil && ev.Response.Error != nil && ev.Response.Error.Message != "" {
		msg = ev.Response.Error.Message
	} else if ev.Error != nil && ev.Error.Message != "" {
		msg = ev.Error.Message
	}
	if msg == "" {
		msg = "response failed"
	}
	return fmt.Errorf("llm: %s", msg)
}

func completedResponse(result *responsesEventResult) (Response, error) {
	if result == nil {
		return Response{}, retryableError{errors.New("llm: response.completed without a response")}
	}
	output := result.Output
	if output == nil {
		output = []json.RawMessage{}
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return Response{}, fmt.Errorf("llm: %w", err)
	}
	msg := Message{Role: "assistant", Raw: raw}
	var content strings.Builder
	for _, itemRaw := range output {
		var item struct {
			Type      string             `json:"type"`
			CallID    string             `json:"call_id"`
			Name      string             `json:"name"`
			Arguments string             `json:"arguments"`
			Content   []responsesContent `json:"content"`
		}
		if err := json.Unmarshal(itemRaw, &item); err != nil {
			return Response{}, retryableError{fmt.Errorf("llm: parsing an output item: %w", err)}
		}
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					content.WriteString(part.Text)
				}
			}
		case "function_call":
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments})
		}
	}
	msg.Content = content.String()
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return Response{}, retryableError{errors.New("llm: response without content and tool_calls")}
	}
	finish := "stop"
	if len(msg.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	return Response{Message: msg, FinishReason: finish, Usage: result.Usage.usage()}, nil
}

func (u *responsesUsage) usage() Usage {
	if u == nil {
		return Usage{}
	}
	t := 0
	if u.OutputTokensDetails != nil {
		t = u.OutputTokensDetails.ReasoningTokens
	}
	return Usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, ReasoningTokens: t}
}

func parseLimitError(body []byte, h http.Header) *LimitError {
	var wire struct {
		Error struct {
			Type            string   `json:"type"`
			ResetsInSeconds *float64 `json:"resets_in_seconds"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wire); err != nil || wire.Error.Type != "usage_limit_reached" {
		return nil
	}
	le := &LimitError{}
	if wire.Error.ResetsInSeconds != nil {
		le.ResetsIn = time.Duration(*wire.Error.ResetsInSeconds * float64(time.Second))
	} else if secs, ok := intHeader(h.Get("Retry-After")); ok {
		le.ResetsIn = time.Duration(secs) * time.Second
	}
	return le
}

func (c *ResponsesClient) observeLimits(h http.Header) {
	if c.onLimits == nil {
		return
	}
	primary := h.Get("x-codex-primary-used-percent")
	secondary := h.Get("x-codex-secondary-used-percent")
	if primary == "" && secondary == "" {
		return
	}
	now := c.now()
	c.onLimits(Limits{
		PrimaryPercent:   floatHeader(primary),
		SecondaryPercent: floatHeader(secondary),
		PrimaryWindow:    windowHeader(h.Get("x-codex-primary-window-minutes")),
		SecondaryWindow:  windowHeader(h.Get("x-codex-secondary-window-minutes")),
		PrimaryReset:     resetHeader(h, "x-codex-primary", now),
		SecondaryReset:   resetHeader(h, "x-codex-secondary", now),
		Plan:             h.Get("x-codex-plan-type"),
	})
}

func floatHeader(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}

func intHeader(v string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

func windowHeader(v string) time.Duration {
	n, ok := intHeader(v)
	if !ok {
		return 0
	}
	return time.Duration(n) * time.Minute
}

func resetHeader(h http.Header, prefix string, now time.Time) time.Time {
	if secs, ok := intHeader(h.Get(prefix + "-reset-after-seconds")); ok {
		return now.Add(time.Duration(secs) * time.Second)
	}
	if at, ok := intHeader(h.Get(prefix + "-reset-at")); ok {
		return time.Unix(int64(at), 0)
	}
	return time.Time{}
}

func ListChatGPTModels(ctx context.Context, h *http.Client, baseURL string, tok Tokens) ([]ModelInfo, error) {
	if h == nil {
		h = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = ChatGPTBaseURL
	}
	token, accountID, err := tok.Access(ctx, false)
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(baseURL, "/") + "/models?" + url.Values{"client_version": {CodexClientVersion}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("llm: models: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("User-Agent", "aux/"+version.Version)
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
	return parseChatGPTModels(body)
}

func parseChatGPTModels(b []byte) ([]ModelInfo, error) {
	var wire struct {
		Models []struct {
			Slug                     string `json:"slug"`
			Visibility               string `json:"visibility"`
			SupportedInAPI           bool   `json:"supported_in_api"`
			Priority                 int    `json:"priority"`
			SupportedReasoningLevels []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return nil, fmt.Errorf("llm: models: parsing the response: %w", err)
	}
	sort.SliceStable(wire.Models, func(i, j int) bool { return wire.Models[i].Priority < wire.Models[j].Priority })
	models := make([]ModelInfo, 0, len(wire.Models))
	for _, m := range wire.Models {
		if m.Visibility != "list" || !m.SupportedInAPI {
			continue
		}
		models = append(models, ModelInfo{
			ID:        m.Slug,
			Tools:     true,
			Reasoning: len(m.SupportedReasoningLevels) > 0,
			Known:     true,
		})
	}
	return models, nil
}
