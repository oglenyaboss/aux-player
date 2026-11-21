package vibeai

import (
	"context"
	"testing"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

type fakeTokens struct{}

func (fakeTokens) Access(context.Context, bool) (string, string, error) {
	return "token", "account", nil
}

// H7: a subscription sign-in validates without an API key, and the chatter is the
// Responses client when the tokens are set.
func TestValidateTokensNoKey(t *testing.T) {
	cfg := Config{
		AppleDeveloperToken: "dev",
		AppleUserToken:      "user",
		LLM:                 llm.Config{Model: "m"},
		Tokens:              fakeTokens{},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate with tokens and no key: %v", err)
	}
	cfg.Tokens = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate without a key and without tokens passed, want an error")
	}
}

func TestNewChatter(t *testing.T) {
	key := newChatter(Config{LLM: llm.Config{Model: "m"}})
	if _, ok := key.(*llm.Client); !ok {
		t.Fatalf("newChatter without tokens = %T, want *llm.Client", key)
	}
	tok := newChatter(Config{LLM: llm.Config{Model: "m"}, Tokens: fakeTokens{}})
	if _, ok := tok.(*llm.ResponsesClient); !ok {
		t.Fatalf("newChatter with tokens = %T, want *llm.ResponsesClient", tok)
	}
}

// H7: SetLLM keeps the chatter for an unchanged agent, swaps it for a changed one and
// leaves the session alone.
func TestEngineSetLLM(t *testing.T) {
	e := newTestEngine(t, &fakeChatter{})
	base := Config{LLM: llm.Config{BaseURL: "https://a/v1", APIKey: "k", Model: "m"}}
	e.cfg = base
	before := e.agent.LLM
	e.SetLLM(base)
	if e.agent.LLM != before {
		t.Fatal("SetLLM replaced the chatter for an unchanged config")
	}

	changed := base
	changed.LLM.Model = "n"
	e.SetLLM(changed)
	if e.agent.LLM == before {
		t.Fatal("SetLLM kept the chatter after the model changed")
	}
	if _, ok := e.agent.LLM.(*llm.Client); !ok {
		t.Fatalf("chatter after a key change = %T, want *llm.Client", e.agent.LLM)
	}

	session, sessionID := e.session, e.sessionID
	withTokens := Config{LLM: llm.Config{Model: "n"}, Tokens: fakeTokens{}}
	e.SetLLM(withTokens)
	if _, ok := e.agent.LLM.(*llm.ResponsesClient); !ok {
		t.Fatalf("chatter after tokens = %T, want *llm.ResponsesClient", e.agent.LLM)
	}
	if e.cfg.Tokens != withTokens.Tokens {
		t.Fatalf("engine tokens = %v, want the fake store", e.cfg.Tokens)
	}
	if e.session != session || e.sessionID != sessionID {
		t.Fatal("SetLLM touched the session")
	}
}
