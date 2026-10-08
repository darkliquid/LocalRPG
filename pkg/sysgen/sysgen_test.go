package sysgen

import (
	"context"
	"strings"
	"testing"
)

type stubGen struct{}

func (stubGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	if strings.Contains(schema, `"resolution"`) {
		return []byte(`{"resolution":"d20","stats":["sanity"]}`), nil
	}
	return []byte(`{"name":"Gritty","stats":[{"id":"sanity"}]}`), nil
}

func TestGenerateReturnsASystem(t *testing.T) {
	s, err := Generate(context.Background(), stubGen{}, Brief{Name: "Gritty", Description: "gritty d20"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name == "" || s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
}

type noHooksGen struct{}

func (noHooksGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	if strings.Contains(schema, `"resolution"`) {
		return []byte(`{"resolution":"d20","stats":["sanity"]}`), nil
	}
	if strings.Contains(schema, `"checks"`) {
		return []byte(`{"stats":[{"id":"sanity"}],"checks":{"notation":"1d20","outcome":["failure","success"],"profiles":{"check":{"notation":"1d20","dc":10}}}}`), nil
	}
	if strings.Contains(schema, `"hooks"`) {
		return []byte(`{"hooks":[]}`), nil
	}
	if strings.Contains(schema, `"rules"`) {
		return []byte(`{"rules":"# Rules"}`), nil
	}
	return []byte(`{}`), nil
}

func TestGeneratedSystemWithNoHooksHasEmptyScript(t *testing.T) {
	s, err := Generate(context.Background(), noHooksGen{}, Brief{Name: "Clean System", Description: "no hooks"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if s.Script != "" {
		t.Errorf("expected s.Script to be empty string, got %q", s.Script)
	}
}
