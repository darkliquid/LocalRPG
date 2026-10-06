package harness

import (
	"strings"
	"testing"
)

func TestNewCheckIDIsUniqueAndPrefixed(t *testing.T) {
	a, b := NewCheckID(), NewCheckID()
	if !strings.HasPrefix(a, "chk_") {
		t.Fatalf("id %q lacks the chk_ prefix", a)
	}
	if a == b {
		t.Fatalf("two ids collided: %q", a)
	}
}
