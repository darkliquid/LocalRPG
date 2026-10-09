package harness

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/trace"
)

type Router struct {
	mu          sync.RWMutex
	providers   map[string]ModelProvider
	roleMap     map[string]string // role -> providerID
	roleKeys    map[string]provider.Key
	fallbacks   map[string]string // role -> fallback providerID
	chains      map[string]chainSpec
	chainPrice  func(providerID string) (int64, bool)
	chainTier   func(providerID string) (provider.Tier, []provider.Feature)
	logger      trace.Logger
	recorder    UsageRecorder
	buildErrors []RoleBuildError
}

// chainSpec is a role's declared provider chain and the rule that orders it.
type chainSpec struct {
	ids  []string
	rule string
	tag  string
}

// RoleBuildError records a configured role whose provider could not be built.
type RoleBuildError struct {
	Role string
	Type string
	Name string // builtin_name or command, when set
	Err  error
}

func NewRouter() *Router {
	return &Router{
		providers: make(map[string]ModelProvider),
		roleMap:   make(map[string]string),
		roleKeys:  make(map[string]provider.Key),
		fallbacks: make(map[string]string),
		chains:    make(map[string]chainSpec),
	}
}

// SetChain assigns an ordered chain and a selection rule to a role. An empty
// chain clears it, restoring the single provider and its configured fallback, so
// a role with no chain behaves exactly as it did before chains existed.
func (r *Router) SetChain(role string, ids []string, rule, tag string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(ids) == 0 {
		delete(r.chains, role)
		return
	}
	r.chains[role] = chainSpec{ids: slices.Clone(ids), rule: rule, tag: tag}
}

// SetChainPrice installs the price accessor, in micros, that the cheapest rule
// uses. It is injected because pkg/pricing imports pkg/harness and cannot be
// imported back. Absent means no chain member is priced, so cheapest keeps the
// declared order.
func (r *Router) SetChainPrice(price func(providerID string) (int64, bool)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chainPrice = price
}

// SetChainTier installs the tier and feature accessor the local-first and by-tag
// rules use. Absent means those rules keep the declared order.
func (r *Router) SetChainTier(tier func(providerID string) (provider.Tier, []provider.Feature)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chainTier = tier
}

// SetLogger attaches a trace sink, so a chain selection is explainable.
func (r *Router) SetLogger(logger trace.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = trace.OrNil(logger)
}

// recordBuildError appends a role build failure so callers can surface it
// instead of silently falling back.
func (r *Router) recordBuildError(e RoleBuildError) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buildErrors = append(r.buildErrors, e)
}

// BuildErrors returns the per-role build failures from the configuration this
// router was built from, or nil when every configured role built.
func (r *Router) BuildErrors() []RoleBuildError {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.buildErrors) == 0 {
		return nil
	}
	return append([]RoleBuildError(nil), r.buildErrors...)
}

func (r *Router) RegisterProvider(p ModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

// SetUsageRecorder installs the sink LLM usage is reported to.
func (r *Router) SetUsageRecorder(rec UsageRecorder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorder = rec
}

// AssignRoleKey records the canonical key a role's provider reports usage
// under, so adapters never name themselves.
func (r *Router) AssignRoleKey(role string, key provider.Key) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roleKeys[role] = key
}

// ProviderKeyForRole reports the canonical key a role's provider records under.
func (r *Router) ProviderKeyForRole(role string) (provider.Key, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.roleKeys[role]
	return key, ok
}

// attemptProvider names the provider an attempt ran against: the role's
// canonical key when one is set, else the provider ID.
func (r *Router) attemptProvider(role, providerID string) string {
	if key, ok := r.ProviderKeyForRole(role); ok && key != "" {
		return string(key)
	}
	return providerID
}

// recordUsage reports a provider's usage to the recorder, stamped with the
// role's canonical key when one is set.
func (r *Router) recordUsage(role string, u *Usage) {
	if u == nil {
		return
	}
	r.mu.RLock()
	rec := r.recorder
	key := r.roleKeys[role]
	r.mu.RUnlock()
	if rec == nil {
		return
	}
	if key != "" {
		u.Provider = string(key)
	}
	rec.RecordUsage(role, *u)
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

// providerByID returns a registered provider, or false when the id names none.
func (r *Router) providerByID(id string) (ModelProvider, bool) {
	if id == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// orderedProviders returns the providers to try for a role, in order: the
// configured chain ordered by its rule, or the single provider and its fallback
// when no chain is declared. A chain member that is not registered is skipped,
// because validation already rejected an unknown name and a stale one must not
// fail a turn.
func (r *Router) orderedProviders(role string) ([]ModelProvider, error) {
	r.mu.RLock()
	chain, hasChain := r.chains[role]
	primaryID, hasPrimary := r.roleMap[role]
	fallbackID := r.fallbacks[role]
	price, tier := r.chainPrice, r.chainTier
	r.mu.RUnlock()

	if hasChain && len(chain.ids) > 0 {
		ordered := OrderChain(chain.ids, chain.rule, chain.tag, price, tier)
		r.traceSelection(role, chain, ordered)
		providers := make([]ModelProvider, 0, len(ordered))
		for _, id := range ordered {
			if p, ok := r.providerByID(id); ok {
				providers = append(providers, p)
			}
		}
		if len(providers) > 0 {
			return providers, nil
		}
	}

	if !hasPrimary {
		return nil, fmt.Errorf("no provider assigned to role %q", role)
	}
	primary, ok := r.providerByID(primaryID)
	if !ok {
		return nil, fmt.Errorf("provider %q for role %q not registered", primaryID, role)
	}
	providers := []ModelProvider{primary}
	if fallback, ok := r.providerByID(fallbackID); ok && fallback != nil {
		providers = append(providers, fallback)
	}
	return providers, nil
}

// traceSelection records the rule and the ordered chain a role resolved to, so a
// trace explains why one provider was preferred over another.
func (r *Router) traceSelection(role string, chain chainSpec, ordered []string) {
	r.mu.RLock()
	logger := r.logger
	r.mu.RUnlock()
	if logger == nil || !logger.Enabled(trace.LevelSummary) {
		return
	}
	selected := ""
	if len(ordered) > 0 {
		selected = ordered[0]
	}
	logger.Event("router.select", map[string]interface{}{
		"role":     role,
		"rule":     chain.rule,
		"tag":      chain.tag,
		"chain":    ordered,
		"selected": selected,
	})
}

// attemptProviderFor names the provider an attempt ran against, preferring the
// provider's own canonical key so a chain member is named by its own key rather
// than the requesting role's.
func (r *Router) attemptProviderFor(role, providerID string) string {
	if key, ok := r.ProviderKeyForRole(providerID); ok && key != "" {
		return string(key)
	}
	return r.attemptProvider(role, providerID)
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
// silently defeat the configured fallback. A declared chain is tried in its
// rule's order; with no chain the single provider is tried, then its fallback.
func (r *Router) GenerateForRole(ctx context.Context, role string, req GenerateRequest) (*GenerateResponse, error) {
	providers, err := r.orderedProviders(role)
	if err != nil {
		return nil, &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, len(providers))
	started := time.Now()

	for _, p := range providers {
		attemptStarted := time.Now()
		res, callErr := p.Generate(ctx, req)
		code, detail := attemptOutcome(res, callErr)
		if code == "" {
			r.recordUsage(role, res.Usage)
			return res, nil
		}
		attempts = append(attempts, Attempt{
			Role: role, Provider: r.attemptProviderFor(role, p.ID()), Code: code, Detail: detail,
			DurationMS: time.Since(attemptStarted).Milliseconds(),
		})
	}

	if len(attempts) == 0 {
		return nil, &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q", role),
		}
	}
	return nil, &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   SummarizeAttempts(attempts, fmt.Sprintf("role %q produced no usable response", role)),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}

func (r *Router) StreamForRole(ctx context.Context, role string, req GenerateRequest, out chan<- StreamChunk) error {
	providers, err := r.orderedProviders(role)
	if err != nil {
		close(out)
		return &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q: %v", role, err),
		}
	}

	attempts := make([]Attempt, 0, len(providers))
	started := time.Now()

	for _, p := range providers {
		attemptStarted := time.Now()
		streamed, code, detail := r.forwardStream(ctx, role, p, req, out)
		if streamed {
			return nil
		}
		attempts = append(attempts, Attempt{
			Role: role, Provider: r.attemptProviderFor(role, p.ID()), Code: code, Detail: detail,
			DurationMS: time.Since(attemptStarted).Milliseconds(),
		})
	}

	close(out)
	if len(attempts) == 0 {
		return &GenerationFailure{
			Code:    FailureProviderUnavailable,
			Message: fmt.Sprintf("no provider available for role %q", role),
		}
	}
	return &GenerationFailure{
		Code:      attempts[len(attempts)-1].Code,
		Message:   SummarizeAttempts(attempts, fmt.Sprintf("role %q streamed no text", role)),
		Attempts:  attempts,
		ElapsedMS: time.Since(started).Milliseconds(),
	}
}

// forwardStream relays a provider's chunks to out once a usable first chunk
// arrives, and returns immediately after that first chunk so a caller can range
// out without the two loops deadlocking. A terminal provider error is relayed as
// a final StreamChunk with Error set. When nothing usable arrives it returns the
// classified failure code so the caller can fall back and explain why.
func (r *Router) forwardStream(ctx context.Context, role string, provider ModelProvider, req GenerateRequest, out chan<- StreamChunk) (bool, FailureCode, string) {
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
		r.recordUsage(role, first.Usage)
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					if streamErr := <-errCh; streamErr != nil {
						out <- StreamChunk{Error: streamErr, Done: true}
					}
					return
				}
				r.recordUsage(role, chunk.Usage)
				out <- chunk
			case <-ctx.Done():
				return
			}
		}
	}()
	return true, "", ""
}
