package harness_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestNewModelProvider(t *testing.T) {
	// Disabled
	disabled, err := harness.NewModelProvider("p1", harness.ProviderConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disabled.ID() != "p1" {
		t.Errorf("expected id p1, got %q", disabled.ID())
	}
	_, err = disabled.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Errorf("expected error from disabled provider")
	}

	// CLI
	cli, err := harness.NewModelProvider("p2", harness.ProviderConfig{
		Type:    "cli",
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := cli.Generate(context.Background(), harness.GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if resp.Text == "" {
		t.Errorf("expected output from echo cli provider")
	}

	// Builtin
	builtin, err := harness.NewModelProvider("p3", harness.ProviderConfig{
		Type:        "builtin",
		BuiltinName: "echo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp3, err := builtin.Generate(context.Background(), harness.GenerateRequest{Prompt: "ping"})
	if err != nil {
		t.Fatalf("builtin generate failed: %v", err)
	}
	if resp3 == nil {
		t.Errorf("expected response from builtin echo")
	}
}
