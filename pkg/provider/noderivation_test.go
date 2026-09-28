package provider_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// keyLiteral matches a canonical key written as a string literal.
var keyLiteral = regexp.MustCompile(`"(llm|tts|stt|image|embedding):[a-z0-9-]+`)

// deadNames are the duplicate derivations this change deleted. Their return
// would mean a second source of provider identity.
var deadNames = regexp.MustCompile(`\b(ProviderIDFor|TTSProviderIDFor|STTProviderIDFor|ImageProviderIDFor|roleProviderKey)\b`)

// TestNoSecondKeyDerivation fails when a package outside pkg/provider builds a
// canonical key literal, or when one of the removed identity helpers reappears.
// Inside pkg/provider the constants in keys.go are the single definition.
func TestNoSecondKeyDerivation(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if strings.Contains(rel, "pkg/provider/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if keyLiteral.MatchString(line) {
				t.Errorf("%s:%d builds a canonical key literal; use the pkg/provider constants and resolvers: %s",
					rel, i+1, trimmed)
			}
			if deadNames.MatchString(line) {
				t.Errorf("%s:%d revives a duplicate provider identity helper: %s", rel, i+1, trimmed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
