package pathutil_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/pathutil"
)

func TestValidateID(t *testing.T) {
	valid := []string{
		"game-1",
		"aldon_harbour",
		"player",
		"guard-kael-12",
		"d20",
		"System_01",
	}
	for _, id := range valid {
		if err := pathutil.ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) unexpected error: %v", id, err)
		}
	}

	invalid := []string{
		"",
		"   ",
		"../etc/passwd",
		"..",
		".",
		"/root",
		"C:\\Windows",
		"foo/bar",
		"foo\\bar",
		"foo\x00bar",
		"entity with spaces",
		"name*with?wildcards",
		"hello:world",
	}
	for _, id := range invalid {
		if err := pathutil.ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) expected error, got nil", id)
		}
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"../etc/passwd", "etc-passwd"},
		{"foo/bar/baz", "foo-bar-baz"},
		{"--test--", "test"},
		{"", "unnamed"},
		{"   ", "unnamed"},
		{"Valid-ID_123", "valid-id_123"},
	}
	for _, tc := range tests {
		got := pathutil.SanitizeID(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeID(%q) = %q; want %q", tc.input, got, tc.expected)
		}
		if err := pathutil.ValidateID(got); err != nil {
			t.Errorf("SanitizeID output %q fails ValidateID: %v", got, err)
		}
	}
}

func TestResolveSafeChild(t *testing.T) {
	tmp := t.TempDir()

	validChild, err := pathutil.ResolveSafeChild(tmp, "sub/file.txt")
	if err != nil {
		t.Fatalf("ResolveSafeChild unexpected error: %v", err)
	}
	if !strings.HasPrefix(validChild, filepath.Clean(tmp)) {
		t.Errorf("ResolveSafeChild returned path %q outside base %q", validChild, tmp)
	}

	escapes := []string{
		"../outside.txt",
		"sub/../../outside.txt",
		"/etc/passwd",
		"..",
	}
	for _, rel := range escapes {
		if _, err := pathutil.ResolveSafeChild(tmp, rel); err == nil {
			t.Errorf("ResolveSafeChild(%q, %q) expected error, got nil", tmp, rel)
		}
	}
}

func TestValidateUserPath(t *testing.T) {
	tmp := t.TempDir()
	cleaned, err := pathutil.ValidateUserPath(filepath.Join(tmp, "custom", "export.html"))
	if err != nil {
		t.Fatalf("ValidateUserPath unexpected error: %v", err)
	}
	if cleaned == "" {
		t.Errorf("ValidateUserPath returned empty string")
	}

	invalid := []string{
		"",
		"   ",
		"path\x00with-null",
	}
	for _, p := range invalid {
		if _, err := pathutil.ValidateUserPath(p); err == nil {
			t.Errorf("ValidateUserPath(%q) expected error, got nil", p)
		}
	}
}
