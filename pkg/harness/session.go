package harness

import (
	"context"
	"time"
)

// SessionHandle identifies a server-held conversation. The turn-context work
// persists it with the turn and uses it only when it covers the current tip.
type SessionHandle struct {
	ID           string
	ThroughTurn  int
	Response     *GenerateResponse
	CachedTokens int
}

// SessionProvider is implemented by providers that can continue a server-held
// conversation instead of being re-sent the whole history.
type SessionProvider interface {
	StartSession(ctx context.Context, req GenerateRequest) (*SessionHandle, error)
	ContinueSession(ctx context.Context, session *SessionHandle, req GenerateRequest) (*GenerateResponse, error)
}

// ContextCacher is implemented by providers that manage an explicit cached
// prefix.
type ContextCacher interface {
	EnsureCache(ctx context.Context, prefix string, ttl time.Duration) (string, error)
	InvalidateCache(ctx context.Context, cacheID string) error
}
