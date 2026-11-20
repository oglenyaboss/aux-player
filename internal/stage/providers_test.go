package stage

import (
	"slices"
	"testing"
)

// A14: the onboarding lists the providers in the settings order.
func TestProviderListOrder(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
	ids := make([]string, len(providers))
	for i, p := range providers {
		ids[i] = p.id
	}
	want := []string{"opencode-go", "commandcode", "openrouter", "openai", "anthropic", "deepseek", "custom"}
	if !slices.Equal(ids, want) {
		t.Fatalf("provider ids = %v, want %v", ids, want)
	}
}

// A14: the second provider picked with a model travels as commandcode's id and URL.
func TestSetupCarriesProvider(t *testing.T) {
	t.Cleanup(func() { SetLang("ru") })
	SetLang("en")
	o := newOnboard()
	o.prov, o.key, o.picked = 1, []rune("k"), "m"
	s := o.setup()
	want := Setup{Lang: "en", Provider: "commandcode", URL: "https://api.commandcode.ai/provider/v1", Key: "k", Model: "m"}
	if s != want {
		t.Fatalf("setup = %+v, want %+v", s, want)
	}
	if s := (&onboard{}).setup(); s.Provider != "" || s.URL != "" {
		t.Fatalf("setup without a model = %+v, want no provider and no URL", s)
	}
}
