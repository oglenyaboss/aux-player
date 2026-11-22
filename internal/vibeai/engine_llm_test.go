package vibeai

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
	"github.com/oglenyaboss/aux-player/internal/vibeai/memory"
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

// D6: ReloadMemory and ReloadSignals pick up edits made on disk outside the engine,
// and stay no-ops on a nil engine or without configured paths.
func TestEngineReloadMemoryAndSignals(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "memory.md")
	sigPath := filepath.Join(dir, "signals.jsonl")
	a := newTestAgent(t, &fakeChatter{}, &fakeBackend{}, nil)
	sig, err := OpenSignals(sigPath)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	e := &Engine{
		agent:     a,
		session:   a.Session,
		mem:       a.Memory,
		log:       &runLog{},
		sig:       sig,
		cfg:       Config{MemoryPath: memPath, SignalsPath: sigPath},
		sessionID: "s1",
		start:     time.Now(),
	}
	if used, _ := e.MemorySize(); used != 0 {
		t.Fatalf("memory used: %d, want 0", used)
	}
	if rated := e.sig.Rated(); len(rated) != 0 {
		t.Fatalf("rated: %+v, want empty", rated)
	}

	extMem, err := memory.Open(memPath, memory.DefaultLimit)
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	if err := extMem.Add("dislikes Gunna", "owner's words"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	extSig, err := OpenSignals(sigPath)
	if err != nil {
		t.Fatalf("OpenSignals: %v", err)
	}
	if err := extSig.SetRating("1", "A", "T1", "more"); err != nil {
		t.Fatalf("SetRating: %v", err)
	}

	if err := e.ReloadMemory(); err != nil {
		t.Fatalf("ReloadMemory: %v", err)
	}
	used, limit := e.MemorySize()
	if used == 0 || limit != memory.DefaultLimit {
		t.Fatalf("memory after reload: (%d, %d), want non-zero and %d", used, limit, memory.DefaultLimit)
	}
	entries := e.agent.Memory.Entries()
	if len(entries) != 1 || entries[0].Text != "dislikes Gunna" {
		t.Fatalf("agent memory after reload: %+v", entries)
	}

	if err := e.ReloadSignals(); err != nil {
		t.Fatalf("ReloadSignals: %v", err)
	}
	rated := e.sig.Rated()
	if len(rated) != 1 || rated[0].Kind != "more" || rated[0].ID != "1" {
		t.Fatalf("rated after reload: %+v", rated)
	}

	var nilEngine *Engine
	if err := nilEngine.ReloadMemory(); err != nil {
		t.Fatalf("nil ReloadMemory: %v", err)
	}
	if err := nilEngine.ReloadSignals(); err != nil {
		t.Fatalf("nil ReloadSignals: %v", err)
	}
	plain := &Engine{agent: a, mem: e.mem, sig: e.sig, log: &runLog{}}
	if err := plain.ReloadMemory(); err != nil {
		t.Fatalf("ReloadMemory without a path: %v", err)
	}
	if err := plain.ReloadSignals(); err != nil {
		t.Fatalf("ReloadSignals without a path: %v", err)
	}
	if plain.mem != e.mem || plain.sig != e.sig {
		t.Fatal("a reload without a path replaced the store or the signals")
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
