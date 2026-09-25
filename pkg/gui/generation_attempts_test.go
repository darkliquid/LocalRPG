package gui

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
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
			wantCodes: []harness.FailureCode{harness.FailureProviderError, ""},
		},
		{
			name:      "role fallback after an empty reply",
			primary:   &scriptedProvider{id: "p1", text: "   "},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			roles:     []string{"character", "gm"},
			wantValue: "Vela",
			wantBy:    "gm",
			wantCodes: []harness.FailureCode{harness.FailureEmptyResponse, ""},
		},
		{
			name:      "role fallback after unparseable text",
			primary:   &scriptedProvider{id: "p1", text: "no json here"},
			fallback:  &scriptedProvider{id: "p2", text: `{"name":"Vela"}`},
			roles:     []string{"character", "gm"},
			wantValue: "Vela",
			wantBy:    "gm",
			wantCodes: []harness.FailureCode{harness.FailureParseError, ""},
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
			wantCodes: []harness.FailureCode{""},
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

func stubRouterFor(provider *scriptedProvider) func(*config.Config, trace.Logger) (*harness.Router, error) {
	return func(*config.Config, trace.Logger) (*harness.Router, error) {
		return routerForRoles(map[string]*scriptedProvider{"gm": provider}), nil
	}
}

func TestGenerateTextFailureCodes(t *testing.T) {
	_, svc := setupTestGame(t)
	original := textRouterFactory
	defer func() { textRouterFactory = original }()

	cases := []struct {
		name     string
		provider *scriptedProvider
		wantCode harness.FailureCode
	}{
		{"provider error", &scriptedProvider{id: "p", err: errors.New("boom")}, harness.FailureProviderError},
		{"empty reply", &scriptedProvider{id: "p", text: "   "}, harness.FailureEmptyResponse},
		{"unparseable", &scriptedProvider{id: "p", text: "not json"}, harness.FailureParseError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			textRouterFactory = stubRouterFor(tc.provider)
			resp, err := svc.GenerateText(context.Background(), GenerateTextRequest{FormType: "world", FieldName: "name"})
			if resp != nil {
				t.Fatalf("resp = %+v, want nil", resp)
			}
			failure, ok := harness.FailureFrom(err)
			if !ok || failure.Code != tc.wantCode {
				t.Fatalf("err = %v, want code %q", err, tc.wantCode)
			}
		})
	}
}

func TestGenerateTextWarningOnlyWhenFieldsMissing(t *testing.T) {
	_, svc := setupTestGame(t)
	original := textRouterFactory
	defer func() { textRouterFactory = original }()

	// A single requested field that is present must not warn.
	textRouterFactory = stubRouterFor(&scriptedProvider{id: "p", text: `{"name":"Vela"}`})
	resp, err := svc.GenerateText(context.Background(), GenerateTextRequest{FormType: "world", FieldName: "name"})
	if err != nil {
		t.Fatalf("GenerateText: %v", err)
	}
	if resp.Warning != nil {
		t.Fatalf("warning = %+v, want none when the field was generated", resp.Warning)
	}

	// An _all request that produced one of five fields must warn.
	resp, err = svc.GenerateText(context.Background(), GenerateTextRequest{FormType: "world", FieldName: "_all"})
	if err != nil {
		t.Fatalf("GenerateText(_all): %v", err)
	}
	if resp.Warning == nil {
		t.Fatal("warning = nil, want a partial warning for _all")
	}
}
