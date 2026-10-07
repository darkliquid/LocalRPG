package content_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

const goldenWorldDigest = "4ff48b9ed2fe4d28e29aca691f0aac1abab899a8ba1c0e9b75d2706a733e1e5d"

func packToBytes(t *testing.T, dir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := content.Pack(dir, "world", content.ManifestMeta{}, &buf); err != nil {
		t.Fatalf("pack: %v", err)
	}
	return buf.Bytes()
}

func TestPackIsDeterministic(t *testing.T) {
	dir := writeFixtureWorld(t)
	a := packToBytes(t, dir)
	b := packToBytes(t, dir)
	if !bytes.Equal(a, b) {
		t.Fatal("packing twice produced different bytes")
	}
}

func TestPackGoldenDigest(t *testing.T) {
	dir := writeFixtureWorld(t)
	sum := sha256.Sum256(packToBytes(t, dir))
	got := hex.EncodeToString(sum[:])
	if got != goldenWorldDigest {
		t.Fatalf("digest = %s, want %s", got, goldenWorldDigest)
	}
}

func TestUnpackReproducesTree(t *testing.T) {
	testCases := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "single file system",
			files: map[string]string{
				"system.yaml": "id: test_sys\nname: Test Sys\nversion: 1.0.0\n",
			},
		},
		{
			name: "nested directories and multiple files",
			files: map[string]string{
				"world.yaml":              "id: test_world\nname: Test World\n",
				"prompts/lore.md":         "# Lore\nAncient ruins and dusty roads.",
				"entities/places/city.md": "---\nname: City\n---\nA grand metropolis.",
				"assets/notes.txt":        "Some plain notes.",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srcDir := t.TempDir()
			for relPath, contentStr := range tc.files {
				fullPath := filepath.Join(srcDir, filepath.FromSlash(relPath))
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, []byte(contentStr), 0644); err != nil {
					t.Fatal(err)
				}
			}

			var buf bytes.Buffer
			typ := "world"
			if _, ok := tc.files["system.yaml"]; ok {
				typ = "system"
			}
			m, err := content.Pack(srcDir, typ, content.ManifestMeta{}, &buf)
			if err != nil {
				t.Fatalf("Pack failed: %v", err)
			}

			destDir := filepath.Join(t.TempDir(), "out")
			unpackedM, err := content.Unpack(&buf, destDir)
			if err != nil {
				t.Fatalf("Unpack failed: %v", err)
			}

			if unpackedM.ID != m.ID {
				t.Errorf("ID mismatch: got %q, want %q", unpackedM.ID, m.ID)
			}

			for relPath, wantContent := range tc.files {
				destFile := filepath.Join(destDir, filepath.FromSlash(relPath))
				gotBytes, err := os.ReadFile(destFile)
				if err != nil {
					t.Errorf("missing unpacked file %s: %v", relPath, err)
					continue
				}
				if string(gotBytes) != wantContent {
					t.Errorf("file %s content mismatch: got %q, want %q", relPath, string(gotBytes), wantContent)
				}
			}
		})
	}
}
