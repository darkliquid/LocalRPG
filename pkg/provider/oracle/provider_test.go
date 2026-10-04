package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestOracleQuoting(t *testing.T) {
	p := NewNarrativeOracleProvider("test-oracle")
	resp, err := p.Generate(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{
			{Role: "user", Content: "Player Action: I say \"hello\""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Text, `""hello""`) || strings.Contains(resp.Text, `"hello"`) {
		t.Errorf("unexpected double quotes in output: %s", resp.Text)
	}
	if !strings.Contains(resp.Text, `'hello'`) {
		t.Errorf("expected single quotes in output: %s", resp.Text)
	}
}
