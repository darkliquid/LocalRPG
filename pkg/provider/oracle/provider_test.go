package oracle

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestOracleQuoting(t *testing.T) {
	p := NewNarrativeOracleProvider("test-oracle")
	rawAction := `I say "hello"`
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{
			{Role: "user", Content: "Player Action: " + rawAction},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := strconv.Quote(rawAction)
	if !strings.Contains(resp.Text, expected) {
		t.Errorf("expected quoted string %s in output: %s", expected, resp.Text)
	}
}

func TestCraftProseComposesParts(t *testing.T) {
	prompt := "[MECHANICS RESULT: miss]\nPlayer Action: x\nLocation: Saltmarch\nNear [[Garrick]].\n"
	got := craftProse(prompt)
	if !strings.Contains(got, "Saltmarch") || !strings.Contains(got, "Garrick") {
		t.Fatalf("prose = %q", got)
	}
}

func TestCraftProseIsDeterministic(t *testing.T) {
	prompt := "[MECHANICS RESULT: weak]\nPlayer Action: y\n"
	if craftProse(prompt) != craftProse(prompt) {
		t.Fatal("the same prompt should yield the same prose")
	}
}

