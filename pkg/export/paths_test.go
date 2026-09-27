package export

import (
	"path/filepath"
	"testing"
)

func TestScriptCompilerUsesTheResolvedCache(t *testing.T) {
	t.Setenv("LOCALRPG_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	compiler := NewScriptCompiler(root)
	if got := compiler.resolver.CacheDir(); got != filepath.Join(root, "cache") {
		t.Fatalf("CacheDir = %q, want %q", got, filepath.Join(root, "cache"))
	}
}
