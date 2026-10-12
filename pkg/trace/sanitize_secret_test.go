package trace

import (
	"strings"
	"testing"
)

func TestSanitizeRedactsRegisteredSecretValues(t *testing.T) {
	const secret = "sk-test-abcdef0123456789"
	RegisterSecret(secret)

	clean := Sanitize(map[string]any{
		"headers": "xi-api-key: " + secret + "\ncontent-type: application/json",
		"note":    "no secret here",
	}, LevelFull, 20000)

	headers, _ := clean["headers"].(string)
	if strings.Contains(headers, secret) {
		t.Errorf("a registered secret leaked: %q", headers)
	}
	if !strings.Contains(headers, "[redacted]") {
		t.Errorf("headers = %q, want a redaction marker", headers)
	}
	if clean["note"] != "no secret here" {
		t.Errorf("unrelated text was altered: %v", clean["note"])
	}
}

func TestRegisterSecretIgnoresEmptyValues(t *testing.T) {
	RegisterSecret("")
	if got := redactSecrets("nothing to hide"); got != "nothing to hide" {
		t.Errorf("redactSecrets altered text with no registered secret: %q", got)
	}
}
