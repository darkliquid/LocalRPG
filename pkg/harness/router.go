package harness

import (
	"context"
	"fmt"
	"sync"
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

func (r *Router) GenerateForRole(ctx context.Context, role string, req GenerateRequest) (*GenerateResponse, error) {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		return nil, err
	}

	res, err := primary.Generate(ctx, req)
	if err == nil {
		return res, nil
	}

	// Try fallback if available
	r.mu.RLock()
	fallbackID, hasFallback := r.fallbacks[role]
	var fallback ModelProvider
	if hasFallback {
		fallback = r.providers[fallbackID]
	}
	r.mu.RUnlock()

	if fallback != nil {
		return fallback.Generate(ctx, req)
	}

	return nil, fmt.Errorf("primary role %q failed: %w", role, err)
}

func (r *Router) StreamForRole(ctx context.Context, role string, req GenerateRequest, out chan<- StreamChunk) error {
	primary, err := r.GetProviderForRole(role)
	if err != nil {
		close(out)
		return err
	}

	// Attempt primary stream
	tempOut := make(chan StreamChunk, 20)
	errCh := make(chan error, 1)

	go func() {
		errCh <- primary.Stream(ctx, req, tempOut)
	}()

	firstChunk, ok := <-tempOut
	if !ok || (firstChunk.Error != nil) {
		// Fallback
		r.mu.RLock()
		fallbackID, hasFallback := r.fallbacks[role]
		var fallback ModelProvider
		if hasFallback {
			fallback = r.providers[fallbackID]
		}
		r.mu.RUnlock()

		if fallback != nil {
			return fallback.Stream(ctx, req, out)
		}
		close(out)
		return <-errCh
	}

	// Forward stream
	go func() {
		defer close(out)
		out <- firstChunk
		for chunk := range tempOut {
			out <- chunk
		}
	}()

	return <-errCh
}
