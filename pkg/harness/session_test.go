package harness_test

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
)

type mockSessionProvider struct {
	stubToolProvider
	lastReq     harness.GenerateRequest
	lastSession *harness.SessionHandle
}

func (m *mockSessionProvider) StartSession(ctx context.Context, req harness.GenerateRequest) (*harness.SessionHandle, error) {
	m.lastReq = req
	return &harness.SessionHandle{ID: "sess-1", ThroughTurn: 1}, nil
}

func (m *mockSessionProvider) ContinueSession(ctx context.Context, session *harness.SessionHandle, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.lastSession = session
	m.lastReq = req
	return &harness.GenerateResponse{Text: "continued", CachedTokens: 50}, nil
}

type mockContextCacher struct {
	stubToolProvider
	cachedPrefix string
}

func (m *mockContextCacher) EnsureCache(ctx context.Context, prefix string, ttl time.Duration) (string, error) {
	m.cachedPrefix = prefix
	return "cache-1", nil
}

func (m *mockContextCacher) InvalidateCache(ctx context.Context, cacheID string) error {
	m.cachedPrefix = ""
	return nil
}

func TestDescribeReportsSessionsAndCache(t *testing.T) {
	sessProv := &mockSessionProvider{}
	caps := harness.Describe(sessProv)
	if !caps.Sessions {
		t.Errorf("expected Sessions capability to be true")
	}
	if caps.ContextCache {
		t.Errorf("expected ContextCache to be false for session-only provider")
	}

	cacher := &mockContextCacher{}
	caps2 := harness.Describe(cacher)
	if !caps2.ContextCache {
		t.Errorf("expected ContextCache capability to be true")
	}
	if caps2.Sessions {
		t.Errorf("expected Sessions to be false for cache-only provider")
	}
}
