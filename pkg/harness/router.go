package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Router struct {
	mu        sync.RWMutex
	providers map[string]ModelProvider
	roleMap   map[string]string // role -> providerID
	fallbacks map[string]string // role -> fallback providerID
}

func NewRouter() *Router {
	return &Router{
		providers: make(map[string]ModelProvider),
		roleMap:   make(map[string]string),
		fallbacks: make(map[string]string),
	}
}

func (r *Router) RegisterProvider(p ModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

func (r *Router) AssignRole(role, providerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roleMap[role] = providerID
}

func (r *Router) SetFallback(role, fallbackProviderID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks[role] = fallbackProviderID
}

// FallbackForRole reports the provider a role falls back to, if one is set.
func (r *Router) FallbackForRole(role string) (ModelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.fallbacks[role]
	if !ok {
		return nil, false
	}
	provider, ok := r.providers[id]
	return provider, ok
}

func (r *Router) GetProviderForRole(role string) (ModelProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerID, exists := r.roleMap[role]
	if !exists {
		return nil, fmt.Errorf("no provider assigned to role %q", role)
	}

	p, exists := r.providers[providerID]
	if !exists {
		return nil, fmt.Errorf("provider %q for role %q not registered", providerID, role)
	}
	return p, nil
}

// ProviderIDForRole names the provider assigned to a role, or "" when none is.
func (r *Router) ProviderIDForRole(role string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.roleMap[role]
}

// attemptOutcome pairs a provider result with the bounded reason it is unusable.
func attemptOutcome(res *GenerateResponse, err error) (FailureCode, string) {
	if err != nil {
		return ClassifyProviderError(err), err.Error()
	}
	if res == nil || strings.TrimSpace(res.Text) == "" {
		return FailureEmptyResponse, "model returned no text"
	}
	return "", ""
}

// GenerateForRole asks one role for a reply, treating a whitespace-only response
// as a failed attempt and falling back, so a model that answers 200 "" cannot
// silently defeat the configured fallback.
func (r *Router) GenerateForRole(ctx context.Context, role string, req GenerateRequest) (*GenerateResponse, error) {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		return nil, &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, 2)
	started := time.Now()

	primaryStarted := time.Now()
	res, callErr := primary.Generate(ctx, req)
	code, detail := attemptOutcome(res, callErr)
	if code == "" {
		return res, nil
	}
	attempts = append(attempts, Attempt{
		Role: role, Provider: primary.ID(), Code: code, Detail: detail,
		DurationMS: time.Since(primaryStarted).Milliseconds(),
	})

	if fallback, ok := r.FallbackForRole(role); ok && fallback != nil {
		fallbackStarted := time.Now()
		fbRes, fbErr := fallback.Generate(ctx, req)
		fbCode, fbDetail := attemptOutcome(fbRes, fbErr)
		if fbCode == "" {
			return fbRes, nil
		}
		attempts = append(attempts, Attempt{
			Role: role, Provider: fallback.ID(), Code: fbCode, Detail: fbDetail,
			DurationMS: time.Since(fallbackStarted).Milliseconds(),
		})
	}

	return nil, &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   fmt.Sprintf("role %q produced no usable response", role),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}

func (r *Router) StreamForRole(ctx context.Context, role string, req GenerateRequest, out chan<- StreamChunk) error {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		close(out)
		return &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, 2)
	started := time.Now()

	streamed, code, detail := r.forwardStream(ctx, primary, req, out)
	if streamed {
		return nil
	}
	attempts = append(attempts, Attempt{
		Role: role, Provider: primary.ID(), Code: code, Detail: detail,
		DurationMS: time.Since(started).Milliseconds(),
	})

	if fallback, ok := r.FallbackForRole(role); ok && fallback != nil {
		streamed, code, detail = r.forwardStream(ctx, fallback, req, out)
		if streamed {
			return nil
		}
		attempts = append(attempts, Attempt{
			Role: role, Provider: fallback.ID(), Code: code, Detail: detail,
			DurationMS: time.Since(started).Milliseconds(),
		})
	}

	close(out)
	return &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   fmt.Sprintf("role %q streamed no text", role),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}

// forwardStream relays a provider's chunks to out once a usable first chunk
// arrives, and returns immediately after that first chunk so a caller can range
// out without the two loops deadlocking. A terminal provider error is relayed as
// a final StreamChunk with Error set. When nothing usable arrives it returns the
// classified failure code so the caller can fall back and explain why.
func (r *Router) forwardStream(ctx context.Context, provider ModelProvider, req GenerateRequest, out chan<- StreamChunk) (bool, FailureCode, string) {
	chunks := make(chan StreamChunk, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- provider.Stream(ctx, req, chunks)
	}()

	first, ok := <-chunks
	if !ok {
		if streamErr := <-errCh; streamErr != nil {
			return false, ClassifyProviderError(streamErr), streamErr.Error()
		}
		return false, FailureEmptyResponse, "provider streamed no text"
	}
	if first.Error != nil {
		go func() {
			for range chunks {
			}
		}()
		<-errCh
		return false, ClassifyProviderError(first.Error), first.Error.Error()
	}

	go func() {
		defer close(out)
		out <- first
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					if streamErr := <-errCh; streamErr != nil {
						out <- StreamChunk{Error: streamErr, Done: true}
					}
					return
				}
				out <- chunk
			case <-ctx.Done():
				return
			}
		}
	}()
	return true, "", ""
}
