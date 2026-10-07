package config

import "testing"

func TestEmbeddingProviderForResolvesNamedAndDefault(t *testing.T) {
	e := EmbeddingsConfig{
		Provider: "local",
		Providers: map[string]EmbeddingProviderConfig{
			"local": {Type: "onnx"},
			"oa":    {Type: "http", Endpoint: "https://api.openai.com/v1"},
		},
	}

	if got := e.ProviderFor("oa"); got.Type != "http" {
		t.Fatalf("ProviderFor(oa) = %+v", got)
	}
	if got := e.ProviderFor(""); got.Type != "onnx" {
		t.Fatalf("ProviderFor(empty) = %+v", got)
	}
	if got := e.ProviderFor(ReservedProviderName); got.Type != "onnx" {
		t.Fatalf("ProviderFor(default) = %+v", got)
	}
	if got := e.ProviderFor("missing"); got.Type != "builtin" {
		t.Fatalf("ProviderFor(missing) = %+v", got)
	}
}

func TestEmbeddingProviderNames(t *testing.T) {
	e := EmbeddingsConfig{Providers: map[string]EmbeddingProviderConfig{
		"zeta":  {},
		"alpha": {},
	}}
	names := e.ProviderNames()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("ProviderNames() = %v, want [alpha zeta]", names)
	}
	if got := (EmbeddingsConfig{}).ProviderNames(); len(got) != 0 {
		t.Fatalf("ProviderNames() on an empty config = %v, want none", got)
	}
}

func TestEmbeddingSelectedProvider(t *testing.T) {
	if got := (EmbeddingsConfig{}).SelectedProvider(); got != ReservedProviderName {
		t.Fatalf("SelectedProvider() = %q, want %q", got, ReservedProviderName)
	}
	if got := (EmbeddingsConfig{Provider: "local"}).SelectedProvider(); got != "local" {
		t.Fatalf("SelectedProvider() = %q, want local", got)
	}
}
