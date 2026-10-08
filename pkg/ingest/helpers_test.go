package ingest

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// jsonGen is a scripted worldgen.Generator: it returns responses in order and an
// empty object once the script is exhausted.
type jsonGen struct {
	responses  []string
	calls      int
	lastPrompt string
}

func (g *jsonGen) GenerateJSON(_ context.Context, prompt, schema string) ([]byte, error) {
	g.calls++
	g.lastPrompt = prompt
	if g.calls > len(g.responses) {
		return []byte(`{}`), nil
	}
	return []byte(g.responses[g.calls-1]), nil
}

// failTransport fails the test if it is ever used, proving a path makes no
// network call.
type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("folder ingestion must not make a network call")
	return nil, errors.New("network call attempted")
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
