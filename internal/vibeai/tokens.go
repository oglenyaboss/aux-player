package vibeai

import (
	"strconv"

	"github.com/oglenyaboss/aux-player/internal/vibeai/llm"
)

type tokenTally struct {
	Steps       int
	In          int
	Cached      int
	Out         int
	Reasoning   int
	CachedKnown bool
}

func (t *tokenTally) add(u llm.Usage) {
	t.Steps++
	t.In += u.PromptTokens
	t.Out += u.CompletionTokens
	t.Reasoning += u.ReasoningTokens
	if u.CachedKnown {
		t.Cached += u.CachedTokens
		t.CachedKnown = true
	}
}

func (t *tokenTally) plus(o tokenTally) {
	t.Steps += o.Steps
	t.In += o.In
	t.Cached += o.Cached
	t.Out += o.Out
	t.Reasoning += o.Reasoning
	t.CachedKnown = t.CachedKnown || o.CachedKnown
}

func cachedText(n int, known bool) string {
	if !known {
		return "?"
	}
	return strconv.Itoa(n)
}
