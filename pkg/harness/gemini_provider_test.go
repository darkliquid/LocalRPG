package harness_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestResolveGeminiAPIKeyPriority(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	key, err := harness.ResolveGeminiAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided, got %q", key)
	}

	// 2. Fallback to GOOGLE_API_KEY env
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-google-key" {
		t.Errorf("expected env-google-key, got %q (err: %v)", key, err)
	}

	// 3. GEMINI_API_KEY env overrides GOOGLE_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	key, err = harness.ResolveGeminiAPIKey("", "")
	if err != nil || key != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q (err: %v)", key, err)
	}

	// 4. Shared provider key overrides env
	key, err = harness.ResolveGeminiAPIKey("", "shared-config-key")
	if err != nil || key != "shared-config-key" {
		t.Errorf("expected shared-config-key, got %q (err: %v)", key, err)
	}

	// 5. Role key overrides shared config key
	key, err = harness.ResolveGeminiAPIKey("role-override-key", "shared-config-key")
	if err != nil || key != "role-override-key" {
		t.Errorf("expected role-override-key, got %q (err: %v)", key, err)
	}
}
