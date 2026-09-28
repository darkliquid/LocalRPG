package harness

import (
	"fmt"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/provider"
)

// LimitKey is one provider+role pair.
type LimitKey struct{ Provider, Role string }

// LimitState is one provider+role the UI should show: a backoff deadline, a
// funds failure, or both.
type LimitState struct {
	Provider     string    `json:"provider"`
	Role         string    `json:"role"`
	Until        time.Time `json:"until,omitempty"`
	FundsFailure string    `json:"funds_failure,omitempty"`
}

// LimitRegistry holds in-memory rate-limit blocks and funding failures. Blocks
// expire lazily on read; funds failures clear only on a later success.
type LimitRegistry struct {
	mu     sync.Mutex
	blocks map[LimitKey]time.Time
	funds  map[LimitKey]string
}

func NewLimitRegistry() *LimitRegistry {
	return &LimitRegistry{blocks: map[LimitKey]time.Time{}, funds: map[LimitKey]string{}}
}

// Block records a backoff until a deadline. A later deadline wins, so a longer
// block is never shortened by a stale one.
func (r *LimitRegistry) Block(providerKey, role string, until time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := LimitKey{providerKey, role}
	if existing, ok := r.blocks[key]; ok && existing.After(until) {
		return
	}
	r.blocks[key] = until
}

// Blocked reports whether the pair is currently backed off, expiring a stale
// block on read. It consults the instance key first, then the adapter key, so a
// block on one endpoint does not stop another while a block on the adapter stops
// all of them.
func (r *LimitRegistry) Blocked(providerKey, role string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, candidate := range limitKeys(providerKey) {
		key := LimitKey{candidate, role}
		until, ok := r.blocks[key]
		if !ok {
			continue
		}
		if now.After(until) {
			delete(r.blocks, key)
			continue
		}
		return until, true
	}
	return time.Time{}, false
}

// limitKeys is the key ladder for a provider key: the key itself, then its
// adapter parent when it is an instance key. A key that does not parse is used
// verbatim, so a legacy value still matches a block recorded against it.
func limitKeys(providerKey string) []string {
	parsed, err := provider.ParseKey(providerKey)
	if err != nil {
		return []string{providerKey}
	}
	if _, ok := parsed.Instance(); ok {
		return []string{string(parsed), string(parsed.Parent())}
	}
	return []string{string(parsed)}
}

func (r *LimitRegistry) Clear(providerKey, role string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.blocks, LimitKey{providerKey, role})
}

func (r *LimitRegistry) RecordFundsFailure(providerKey, role, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funds[LimitKey{providerKey, role}] = message
}

func (r *LimitRegistry) ClearFundsFailure(providerKey, role string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.funds, LimitKey{providerKey, role})
}

// Snapshot reports every live block and funds failure, expiring stale blocks.
func (r *LimitRegistry) Snapshot() []LimitState {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]LimitState, 0, len(r.blocks)+len(r.funds))
	for key, until := range r.blocks {
		if now.After(until) {
			delete(r.blocks, key)
			continue
		}
		out = append(out, LimitState{Provider: key.Provider, Role: key.Role, Until: until})
	}
	for key, message := range r.funds {
		out = append(out, LimitState{Provider: key.Provider, Role: key.Role, FundsFailure: message})
	}
	return out
}

// ErrRateLimitedUntil is returned when work is refused because a provider+role
// is backed off. The API maps it to 429 with a Retry-After.
type ErrRateLimitedUntil struct {
	Provider string
	Role     string
	Until    time.Time
}

func (e *ErrRateLimitedUntil) Error() string {
	return fmt.Sprintf("provider %q (%s) is rate limited until %s", e.Provider, e.Role, e.Until.Format(time.RFC3339))
}

// RetryAfter is the remaining backoff, zero once the deadline has passed.
func (e *ErrRateLimitedUntil) RetryAfter() time.Duration {
	if d := time.Until(e.Until); d > 0 {
		return d
	}
	return 0
}
