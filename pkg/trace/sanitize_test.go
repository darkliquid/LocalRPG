package trace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeRedactsSecretsAtEveryLevel(t *testing.T) {
	fields := map[string]any{
		"role":     "gm",
		"api_key":  "sk-live-1234567890",
		"endpoint": "http://localhost:8880",
	}

	for _, level := range []Level{LevelSummary, LevelFull} {
		clean := Sanitize(fields, level, 20000)
		if clean["api_key"] != "[redacted]" {
			t.Errorf("level %v: api_key = %v, want [redacted]", level, clean["api_key"])
		}
		if clean["role"] != "gm" {
			t.Errorf("level %v: unexpected field loss: %+v", level, clean)
		}
	}
}

func TestSanitizeRedactsNestedSecrets(t *testing.T) {
	fields := map[string]any{
		"config": map[string]any{
			"provider": map[string]any{"authorization": "Bearer abc", "model": "kokoro"},
		},
		"attempts": []any{map[string]any{"token": "abc"}},
	}

	clean := Sanitize(fields, LevelFull, 20000)
	encoded, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Bearer abc", `"abc"`} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("secret %q survived sanitising: %s", secret, encoded)
		}
	}
	if !strings.Contains(string(encoded), "kokoro") {
		t.Errorf("sanitising removed a harmless field: %s", encoded)
	}
}

func TestSanitizeOmitsPayloadsAtSummaryAndKeepsThemAtFull(t *testing.T) {
	fields := map[string]any{"prompt": "the whole question", "prompt_chars": 19}

	summary := Sanitize(fields, LevelSummary, 20000)
	if _, present := summary["prompt"]; present {
		t.Errorf("summary level must not carry a payload")
	}
	if summary["prompt_chars"] != 19 {
		t.Errorf("summary level must keep the size, got %v", summary["prompt_chars"])
	}

	full := Sanitize(fields, LevelFull, 20000)
	if full["prompt"] != "the whole question" {
		t.Errorf("full level must keep the payload, got %v", full["prompt"])
	}
}

func TestSanitizeTruncatesLongStringsWithAMarker(t *testing.T) {
	fields := map[string]any{"prompt": strings.Repeat("a", 50)}

	clean := Sanitize(fields, LevelFull, 10)
	text, ok := clean["prompt"].(string)
	if !ok {
		t.Fatalf("prompt is not a string: %T", clean["prompt"])
	}
	if !strings.HasPrefix(text, strings.Repeat("a", 10)) {
		t.Errorf("truncation did not keep the head: %q", text)
	}
	if !strings.Contains(text, "truncated") || !strings.Contains(text, "50") {
		t.Errorf("truncation marker must say what was cut: %q", text)
	}
}

func TestSanitizeKeepsCacheKeys(t *testing.T) {
	// A cache key is a content hash. Redacting anything named "key" would hide
	// exactly the value needed to find the cached clip.
	fields := map[string]any{"cache_key": "06e763d1"}

	clean := Sanitize(fields, LevelFull, 20000)
	if clean["cache_key"] != "06e763d1" {
		t.Errorf("cache_key = %v, want it preserved", clean["cache_key"])
	}
}
