package sysgen

import (
	"context"
	"testing"
)

type stubGen struct{}

func (stubGen) GenerateJSON(context.Context, string, string) ([]byte, error) {
	return []byte(`{"name":"Gritty","stats":[{"id":"sanity"}]}`), nil
}

func TestGenerateReturnsASystem(t *testing.T) {
	s, err := Generate(context.Background(), stubGen{}, Brief{Description: "gritty d20"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name == "" || s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
}
