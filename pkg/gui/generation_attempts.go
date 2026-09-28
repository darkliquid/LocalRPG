package gui

import (
	"context"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// textRouterFactory builds the router for one-shot text generation. It is a
// package variable so a test can inject a scripted router.
var textRouterFactory = harness.RouterFromConfigWithLogger

// generationOutcome is the result of walking a role fallback chain.
type generationOutcome struct {
	Values      map[string]string
	Attempts    []harness.Attempt
	GeneratedBy string
}

// collectTextAttempts asks each role in order and returns the first decodable
// result. It owns per-attempt timing and the empty-attempt synthesis so the text
// and character generators share one fallback implementation.
func collectTextAttempts(ctx context.Context, router *harness.Router, roles []string, request harness.GenerateRequest) generationOutcome {
	out := generationOutcome{Attempts: make([]harness.Attempt, 0, len(roles))}
	for _, role := range roles {
		roleStarted := time.Now()
		resp, err := router.GenerateForRole(ctx, role, request)
		if err != nil {
			if failure, ok := harness.FailureFrom(err); ok {
				out.Attempts = append(out.Attempts, failure.Attempts...)
				if len(failure.Attempts) == 0 {
					out.Attempts = append(out.Attempts, harness.Attempt{
						Role: role, Provider: providerKeyLabel(router, role),
						Code: failure.Code, Detail: failure.Message,
						DurationMS: time.Since(roleStarted).Milliseconds(),
					})
				}
				continue
			}
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: providerKeyLabel(router, role),
				Code: harness.FailureProviderError, Detail: err.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		if resp == nil || strings.TrimSpace(resp.Text) == "" {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: providerKeyLabel(router, role),
				Code: harness.FailureEmptyResponse, Detail: "model returned no text",
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		values, decodeErr := decodeGeneratedValuesChecked(resp.Text)
		if decodeErr != nil {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: providerKeyLabel(router, role),
				Code: harness.FailureParseError, Detail: decodeErr.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		out.Values = values
		out.GeneratedBy = role
		// Record the winning attempt too, so the span's gen_ai.system and
		// attempt count name the provider that actually answered.
		out.Attempts = append(out.Attempts, harness.Attempt{
			Role: role, Provider: providerKeyLabel(router, role),
			DurationMS: time.Since(roleStarted).Milliseconds(),
		})
		break
	}
	return out
}

// providerKeyLabel names a role's provider for an attempt, preferring the
// canonical key so a failure, a block, and a ledger row can be joined.
func providerKeyLabel(router *harness.Router, role string) string {
	if key, ok := router.ProviderKeyForRole(role); ok && key != "" {
		return string(key)
	}
	return router.ProviderIDForRole(role)
}
