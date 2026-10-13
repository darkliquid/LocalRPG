package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

// TestIsHexSHA256 fixes what a checksum may contain, because the checksum becomes a
// filename in the package cache. A value carrying a separator or a parent reference
// is not a checksum and must be refused before it reaches filepath.Join.
func TestIsHexSHA256(t *testing.T) {
	valid := strings.Repeat("a1b2", 16) // 64 characters
	if !isHexSHA256(valid) {
		t.Errorf("a 64-character hex string should be accepted: %q", valid)
	}
	if !isHexSHA256(strings.ToUpper(valid)) {
		t.Error("uppercase hex should be accepted; the cache path lowercases it")
	}

	bad := map[string]string{
		"empty":            "",
		"too short":        "abc",
		"too long":         valid + "a",
		"a traversal":      "../../../../etc/passwd",
		"a separator":      strings.Repeat("a", 32) + "/" + strings.Repeat("b", 31),
		"a parent segment": strings.Repeat("..", 32),
		"non-hex letters":  strings.Repeat("z", 64),
	}
	for what, s := range bad {
		if isHexSHA256(s) {
			t.Errorf("%s should be refused: %q", what, s)
		}
	}
}

// TestInstallRefusesAChecksumThatIsNotOne is the guard at the boundary rather than at
// the helper: a manifest with a checksum that could traverse must not reach the cache
// path, and the refusal must happen before the download is attempted.
func TestInstallRefusesAChecksumThatIsNotOne(t *testing.T) {
	client := NewClient(config.RegistriesConfig{}, t.TempDir())

	// A scheme the transport understands but the app does not intend to speak must
	// be refused where the URL reaches the request, which is now inline in Install.
	_, err := client.Install(context.Background(), PackageRef{
		Package: Package{Download: "file:///etc/passwd", SHA256: strings.Repeat("a", 64)},
	}, "overwrite")
	if err == nil || !strings.Contains(err.Error(), "http") {
		t.Errorf("a file:// download URL should be refused, got %v", err)
	}

	for _, checksum := range []string{"../../../../tmp/escape", strings.Repeat("a", 32)} {
		_, err := client.Install(context.Background(), PackageRef{
			Package: Package{Download: "https://example.invalid/p.lrpgpack", SHA256: checksum},
		}, "overwrite")
		if err == nil {
			t.Fatalf("a checksum of %q should be refused", checksum)
		}
		if !strings.Contains(err.Error(), "checksum") {
			t.Errorf("the error should name the checksum, got %v", err)
		}
	}
}
