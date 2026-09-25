package gui

import (
	"context"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
)

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
						Role: role, Provider: router.ProviderIDForRole(role),
						Code: failure.Code, Detail: failure.Message,
						DurationMS: time.Since(roleStarted).Milliseconds(),
					})
				}
				continue
			}
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureProviderError, Detail: err.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		if resp == nil || strings.TrimSpace(resp.Text) == "" {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureEmptyResponse, Detail: "model returned no text",
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		values, decodeErr := decodeGeneratedValuesChecked(resp.Text)
		if decodeErr != nil {
			out.Attempts = append(out.Attempts, harness.Attempt{
				Role: role, Provider: router.ProviderIDForRole(role),
				Code: harness.FailureParseError, Detail: decodeErr.Error(),
				DurationMS: time.Since(roleStarted).Milliseconds(),
			})
			continue
		}
		out.Values = values
		out.GeneratedBy = role
		break
	}
	return out
}
