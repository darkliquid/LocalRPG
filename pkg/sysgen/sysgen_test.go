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
