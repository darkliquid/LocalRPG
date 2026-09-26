package provider

import (
	"strings"
	"testing"
)

func TestTruncateDetailStringTrims(t *testing.T) {
	if got := TruncateDetailString("  boom  "); got != "boom" {
		t.Fatalf("got %q, want %q", got, "boom")
	}
	if got := TruncateDetailString("   "); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestTruncateDetailStringBounds(t *testing.T) {
	long := strings.Repeat("x", MaxProviderDetailBytes+100)
	got := TruncateDetailString(long)
	if len([]rune(got)) != MaxProviderDetailBytes+3 {
		t.Fatalf("runes = %d, want %d", len([]rune(got)), MaxProviderDetailBytes+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("got %q, want a trailing ellipsis", got)
	}
}

func TestTruncateDetailAcceptsBytes(t *testing.T) {
	if got := TruncateDetail([]byte("short")); got != "short" {
		t.Fatalf("got %q", got)
	}
}
