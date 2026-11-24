package vibeai

import (
	"testing"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

func TestTallyAllUnknown(t *testing.T) {
	var tt tokenTally
	tt.add(llm.Usage{PromptTokens: 10, CompletionTokens: 2})
	tt.add(llm.Usage{PromptTokens: 20, CompletionTokens: 3})
	if got := cachedText(tt.Cached, tt.CachedKnown); got != "?" {
		t.Errorf("cachedText = %q, want ?", got)
	}
	if tt.Steps != 2 || tt.In != 30 || tt.Cached != 0 || tt.Out != 5 || tt.Reasoning != 0 {
		t.Errorf("tally = %+v, want steps 2 in 30 out 5", tt)
	}
}

func TestTallyKnownSteps(t *testing.T) {
	var tt tokenTally
	tt.add(llm.Usage{PromptTokens: 100, CachedTokens: 60, CachedKnown: true})
	tt.add(llm.Usage{PromptTokens: 200})
	tt.add(llm.Usage{CachedTokens: 10, CachedKnown: true})
	if got := cachedText(tt.Cached, tt.CachedKnown); got != "70" {
		t.Errorf("cachedText = %q, want 70", got)
	}
	var o tokenTally
	o.add(llm.Usage{PromptTokens: 50, CompletionTokens: 5, ReasoningTokens: 2, CachedTokens: 30, CachedKnown: true})
	tt.plus(o)
	if tt.Steps != 4 || tt.In != 350 || tt.Cached != 100 || tt.Out != 5 || tt.Reasoning != 2 || !tt.CachedKnown {
		t.Errorf("tally = %+v, want steps 4 in 350 cached 100 out 5 reasoning 2 known", tt)
	}
}
