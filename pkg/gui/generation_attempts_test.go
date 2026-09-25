package gui

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type scriptedProvider struct {
	id   string
	text string
	err  error
}

func (p *scriptedProvider) ID() string { return p.id }

func (p *scriptedProvider) Generate(context.Context, harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: p.text}, p.err
}

func (p *scriptedProvider) Stream(context.Context, harness.GenerateRequest, chan<- harness.StreamChunk) error {
	return nil
}

// routerForRoles assigns one distinct provider per role, so a test exercises
// role fallback rather than the router's own per-role provider fallback.
func routerForRoles(roles map[string]*scriptedProvider) *harness.Router {
	router := harness.NewRouter()
	for role, provider := range roles {
		router.RegisterProvider(provider)
		router.AssignRole(role, provider.id)
	}
	return router
}

func TestCollectTextAttempts(t *testing.T) {
	tests := []struct {
		name      string
		primary   *scriptedProvider
		fallback  *scriptedProvider
		roles     []string
		wantValue string
		wantBy    string
		wantCodes []harness.FailureCode
	}{
		{
			name:      "role fallback after a provider error",
			primary:   &scriptedProvider{id: "p1", err: errors.New("boom")},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			roles:     []string{"character", "gm"},
			wantValue: "Vela",
			wantBy:    "gm",
			wantCodes: []harness.FailureCode{harness.FailureProviderError},
		},
		{
			name:      "role fallback after an empty reply",
			primary:   &scriptedProvider{id: "p1", text: "   "},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			roles:     []string{"character", "gm"},
			wantValue: "Vela",
			wantBy:    "gm",
			wantCodes: []harness.FailureCode{harness.FailureEmptyResponse},
		},
		{
			name:      "role fallback after unparseable text",
			primary:   &scriptedProvider{id: "p1", text: "no json here"},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			roles:     []string{"character", "gm"},
			wantValue: "Vela",
			wantBy:    "gm",
			wantCodes: []harness.FailureCode{harness.FailureParseError},
		},
		{
			name:      "both roles empty",
			primary:   &scriptedProvider{id: "p1", text: ""},
			fallback:  &scriptedProvider{id: "p2", text: ""},
			roles:     []string{"character", "gm"},
			wantCodes: []harness.FailureCode{harness.FailureEmptyResponse, harness.FailureEmptyResponse},
		},
		{
			name:      "primary role succeeds",
			primary:   &scriptedProvider{id: "p1", text: `{"name":"Vela"}`},
			roles:     []string{"character"},
			wantValue: "Vela",
			wantBy:    "character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providers := map[string]*scriptedProvider{tt.roles[0]: tt.primary}
			if tt.fallback != nil {
				providers[tt.roles[1]] = tt.fallback
			}
			router := routerForRoles(providers)
			out := collectTextAttempts(context.Background(), router, tt.roles, harness.GenerateRequest{Prompt: "hi"})
			if out.GeneratedBy != tt.wantBy {
				t.Fatalf("GeneratedBy = %q, want %q", out.GeneratedBy, tt.wantBy)
			}
			if tt.wantValue != "" && out.Values["name"] != tt.wantValue {
				t.Fatalf("Values = %v, want name=%q", out.Values, tt.wantValue)
			}
			if len(out.Attempts) != len(tt.wantCodes) {
				t.Fatalf("attempts = %d (%v), want %d", len(out.Attempts), out.Attempts, len(tt.wantCodes))
			}
			for i, code := range tt.wantCodes {
				if out.Attempts[i].Code != code {
					t.Fatalf("attempt %d code = %q, want %q", i, out.Attempts[i].Code, code)
				}
			}
		})
	}
}
