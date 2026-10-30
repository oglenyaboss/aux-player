package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oglenyaboss/aux-player/internal/version"
)

type Config struct{ BaseURL, APIKey, Model, Effort string }

func LoadEnv(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("llm: %w", err)
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	var missing []string
	for _, k := range []string{"LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL"} {
		if vals[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("llm: LoadEnv: missing required keys: %s", strings.Join(missing, ", "))
	}
	return Config{BaseURL: vals["LLM_BASE_URL"], APIKey: vals["LLM_API_KEY"], Model: vals["LLM_MODEL"]}, nil
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
	Raw        json.RawMessage
}

type Usage struct{ PromptTokens, CompletionTokens, ReasoningTokens int }

type Response struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

type Client struct {
	cfg     Config
	http    *http.Client
	session string
	sleep   func(time.Duration)
}

func New(cfg Config, h *http.Client) *Client {
	if h == nil {
		h = http.DefaultClient
	}
	return &Client{
		cfg:     cfg,
		http:    h,
		session: newSession(),
		sleep:   time.Sleep,
	}
}

func (c *Client) Chat(ctx context.Context, msgs []Message, tools []Tool, jsonMode bool) (Response, error) {
	body, err := c.requestBody(msgs, tools, jsonMode)
	if err != nil {
		return Response{}, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			c.sleep(time.Duration(attempt) * time.Second)
		}
		resp, err := c.attempt(ctx, body, jsonMode)
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

type wireRequest struct {
	Model           string            `json:"model"`
	Messages        []json.RawMessage `json:"messages"`
	Tools           []wireTool        `json:"tools,omitempty"`
	ResponseFormat  *wireFormat       `json:"response_format,omitempty"`
	Reasoning       *wireReasoning    `json:"reasoning,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
}

type wireReasoning struct {
	Effort string `json:"effort"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type wireFormat struct {
	Type string `json:"type"`
}

type wireMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

func marshalMessage(m Message) (json.RawMessage, error) {
	if len(m.Raw) > 0 {
		return m.Raw, nil
	}
	wm := wireMessage{Role: m.Role, Content: m.Content}
	if m.Role == "tool" {
		wm.ToolCallID = m.ToolCallID
	}
	return json.Marshal(wm)
}

func (c *Client) requestBody(msgs []Message, tools []Tool, jsonMode bool) ([]byte, error) {
	rawMsgs := make([]json.RawMessage, len(msgs))
	for i, m := range msgs {
		rm, err := marshalMessage(m)
		if err != nil {
			return nil, fmt.Errorf("llm: message %d: %w", i, err)
		}
		rawMsgs[i] = rm
	}
	req := wireRequest{Model: c.cfg.Model, Messages: rawMsgs}
	for _, t := range tools {
		wt := wireTool{Type: "function"}
		wt.Function.Name = t.Name
		wt.Function.Description = t.Description
		wt.Function.Parameters = t.Parameters
		req.Tools = append(req.Tools, wt)
	}
	if jsonMode {
		req.ResponseFormat = &wireFormat{Type: "json_object"}
	}
	if c.cfg.Effort != "" {
		if isOpenRouter(c.cfg.BaseURL) {
			req.Reasoning = &wireReasoning{Effort: c.cfg.Effort}
		} else {
			req.ReasoningEffort = c.cfg.Effort
		}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return b, nil
}

func (c *Client) attempt(ctx context.Context, body []byte, jsonMode bool) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, retryableError{fmt.Errorf("llm: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	setHeaders(req, c.cfg.APIKey, c.session)

	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, retryableError{fmt.Errorf("llm: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		httpErr := fmt.Errorf("llm: HTTP %d: %s", resp.StatusCode, bodySnippet(snippet))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return Response{}, retryableError{httpErr}
		}
		return Response{}, httpErr
	}

	var wire wireResponse
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return Response{}, fmt.Errorf("llm: decoding the response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return Response{}, retryableError{errors.New("llm: response without choices")}
	}
	msg, err := parseMessage(wire.Choices[0].Message)
	if err != nil {
		return Response{}, err
	}
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return Response{}, retryableError{errors.New("llm: response without content and tool_calls")}
	}
	if jsonMode && msg.Content != "" && !json.Valid([]byte(msg.Content)) {
		return Response{}, retryableError{errors.New("llm: jsonMode: content is not JSON")}
	}
	return Response{
		Message:      msg,
		FinishReason: wire.Choices[0].FinishReason,
		Usage:        wire.Usage.usage(),
	}, nil
}

type wireResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage wireUsage `json:"usage"`
}

type wireUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u wireUsage) usage() Usage {
	t := 0
	if u.CompletionTokensDetails != nil {
		t = u.CompletionTokensDetails.ReasoningTokens
	}
	return Usage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, ReasoningTokens: t}
}

type wireMessageResp struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []wireToolCall `json:"tool_calls"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func parseMessage(raw json.RawMessage) (Message, error) {
	var wm wireMessageResp
	if err := json.Unmarshal(raw, &wm); err != nil {
		return Message{}, fmt.Errorf("llm: parsing message: %w", err)
	}
	m := Message{Role: wm.Role, Content: wm.Content, Raw: raw}
	for _, tc := range wm.ToolCalls {
		m.ToolCalls = append(m.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return m, nil
}

func setHeaders(req *http.Request, apiKey, session string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "aux/"+version.Version)
	req.Header["x-opencode-session"] = []string{session}
}

func bodySnippet(b []byte) string {
	if len(b) > 200 {
		b = b[:200]
	}
	return string(b)
}

type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }
