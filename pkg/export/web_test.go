// pkg/export/web_test.go
package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportWebBundle(t *testing.T) {
	tempDir := t.TempDir()
	gameDir := filepath.Join(tempDir, "games", "test-web")
	_ = os.MkdirAll(gameDir, 0755)

	manifestContent := `id: test-web
name: Web Replay Test
system: core-d20
world: fantasy
player: elena
`
	_ = os.WriteFile(filepath.Join(gameDir, "game.yaml"), []byte(manifestContent), 0644)

	compiler := NewScriptCompiler(tempDir)
	script, err := compiler.Compile(context.Background(), "test-web")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	outDir := filepath.Join(tempDir, "dist-web")
	exporter := NewWebExporter(tempDir)
	bundlePath, err := exporter.Export(context.Background(), script, outDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("read index.html failed: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Web Replay Test") {
		t.Errorf("expected title in exported html: %s", content)
	}
	if !strings.Contains(content, "const REPLAY_SCRIPT =") {
		t.Errorf("expected embedded replay script json: %s", content)
	}
}
