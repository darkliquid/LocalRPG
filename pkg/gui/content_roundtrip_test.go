package gui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestContentExportImportRoundTrip(t *testing.T) {
	dir1 := t.TempDir()
	svc1 := NewService(dir1)

	worldID := "roundtrip_realm"
	worldDir1 := svc1.resolver.WorldDir(worldID)
	if err := os.MkdirAll(filepath.Join(worldDir1, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(worldDir1, "entities"), 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"world.yaml":        "id: roundtrip_realm\nname: Roundtrip Realm\ndescription: Test export-import roundtrip\n",
		"prompts/lore.md":   "# The Lore of Roundtrip\nDeep in the mountains...",
		"entities/hero.md":  "---\nid: hero\nname: Brave Hero\ntype: character\n---\nThe hero lives here.",
	}

	for rel, content := range files {
		fullPath := filepath.Join(worldDir1, rel)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Export from svc1
	var buf bytes.Buffer
	manifest, err := svc1.ExportContent(context.Background(), "world", worldID, &buf)
	if err != nil {
		t.Fatalf("ExportContent: %v", err)
	}
	if manifest.ID != worldID {
		t.Errorf("manifest.ID = %q, want %q", manifest.ID, worldID)
	}

	// 2. Import into a fresh svc2
	dir2 := t.TempDir()
	svc2 := NewService(dir2)

	importRes, err := svc2.ImportContent(context.Background(), &buf, "refuse")
	if err != nil {
		t.Fatalf("ImportContent: %v", err)
	}
	if importRes.ID != worldID {
		t.Errorf("importRes.ID = %q, want %q", importRes.ID, worldID)
	}
	if importRes.Action != "installed" {
		t.Errorf("importRes.Action = %q, want installed", importRes.Action)
	}

	// 3. Verify all files in svc2 match
	worldDir2 := svc2.resolver.WorldDir(worldID)
	for rel, expectedContent := range files {
		fullPath := filepath.Join(worldDir2, rel)
		gotContent, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("missing file in imported world: %s: %v", rel, err)
			continue
		}
		if string(gotContent) != expectedContent {
			t.Errorf("content mismatch for %s:\ngot: %s\nwant: %s", rel, string(gotContent), expectedContent)
		}
	}
}
