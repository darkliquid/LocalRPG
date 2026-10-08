package worldgen

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// jsonGen is a scripted Generator: it returns responses in order and an empty
// object once the script is exhausted, so a test only scripts the steps it cares
// about. failAt, when positive, makes the nth call fail.
type jsonGen struct {
	responses  []string
	calls      int
	failAt     int
	lastPrompt string
	lastSchema string
}

func (g *jsonGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	g.calls++
	g.lastPrompt = prompt
	g.lastSchema = schema
	if g.failAt > 0 && g.calls == g.failAt {
		return nil, fmt.Errorf("scripted failure at call %d", g.calls)
	}
	if g.calls > len(g.responses) {
		return []byte(`{}`), nil
	}
	return []byte(g.responses[g.calls-1]), nil
}

// mustDraft runs the pipeline and fails the test on error.
func mustDraft(t *testing.T, gen Generator, brief Brief) Draft {
	t.Helper()
	draft, err := Generate(context.Background(), gen, brief, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return draft
}

// containsAll reports whether every want appears in got.
func containsAll(got string, want ...string) bool {
	for _, w := range want {
		if !strings.Contains(got, w) {
			return false
		}
	}
	return true
}
