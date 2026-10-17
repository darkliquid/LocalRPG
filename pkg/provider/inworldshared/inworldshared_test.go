package inworldshared_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider/inworldshared"
)

func TestResolveAPIKey(t *testing.T) {
	t.Setenv("INWORLD_API_KEY", "env-key")

	// 1. Explicit key takes precedence
	key, err := inworldshared.ResolveAPIKey("explicit-key", "shared-key")
	if err != nil || key != "explicit-key" {
		t.Fatalf("expected explicit-key, got key=%q, err=%v", key, err)
	}

	// 2. Shared key used when explicit is empty or whitespace
	key, err = inworldshared.ResolveAPIKey("   ", "shared-key")
	if err != nil || key != "shared-key" {
		t.Fatalf("expected shared-key, got key=%q, err=%v", key, err)
	}

	// 3. Env key used when explicit and shared are empty
	key, err = inworldshared.ResolveAPIKey("", "")
	if err != nil || key != "env-key" {
		t.Fatalf("expected env-key, got key=%q, err=%v", key, err)
	}

	// 4. Missing key error when env is also empty
	t.Setenv("INWORLD_API_KEY", "")
	key, err = inworldshared.ResolveAPIKey("", "   ")
	if !errors.Is(err, inworldshared.ErrMissingAPIKey) {
		t.Fatalf("expected ErrMissingAPIKey, got key=%q, err=%v", key, err)
	}
}

func TestMapError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       []byte
		wantSubstr string
	}{
		{
			name:       "unauthorized",
			status:     http.StatusUnauthorized,
			body:       []byte("ignored"),
			wantSubstr: "invalid API key",
		},
		{
			name:       "forbidden",
			status:     http.StatusForbidden,
			body:       nil,
			wantSubstr: "invalid API key",
		},
		{
			name:       "rate limit",
			status:     http.StatusTooManyRequests,
			body:       nil,
			wantSubstr: "quota exceeded",
		},
		{
			name:       "bad request with detail",
			status:     http.StatusBadRequest,
			body:       []byte("parameter 'model' is unsupported"),
			wantSubstr: "parameter 'model' is unsupported (status 400)",
		},
		{
			name:       "server error empty body",
			status:     http.StatusInternalServerError,
			body:       nil,
			wantSubstr: "request failed with status 500",
		},
		{
			name:       "bad gateway whitespace body",
			status:     http.StatusBadGateway,
			body:       []byte("   \n\t  "),
			wantSubstr: "request failed with status 502",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := inworldshared.MapError(tc.status, tc.body)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("MapError(%d, %q) = %v; want substring %q", tc.status, string(tc.body), err, tc.wantSubstr)
			}
		})
	}
}
