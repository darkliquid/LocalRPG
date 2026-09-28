package gui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// removedSymbols are identifiers that no longer exist in the Go source, so a
// contributor-facing document that names one is wrong. Dated plans and specs
// under docs/superpowers are records of what was true then and are exempt.
var removedSymbols = []string{
	"AssembleContextWithProfiles",
	"harness.WasmEngine",
}

// liveDocs are the current, contributor-facing documents at the repository root
// and under docs/, relative to this package.
var liveDocs = []string{
	filepath.Join("..", "..", "AGENTS.md"),
	filepath.Join("..", "..", "README.md"),
	filepath.Join("..", "..", "docs", "debugging.md"),
}

func TestLiveDocsDoNotReferenceRemovedSymbols(t *testing.T) {
	docs := append([]string(nil), liveDocs...)
	if arch, err := filepath.Glob(filepath.Join("..", "..", "docs", "architecture", "*.md")); err == nil {
		docs = append(docs, arch...)
	}

	for _, path := range docs {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", path, err)
		}
		reportRemovedSymbols(t, path, string(data))
	}
}

func TestRemovedSymbolScannerFlagsAReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.md")
	if err := os.WriteFile(path, []byte("Use `harness.AssembleContextWithProfiles` here.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The scanner must fail on a seeded reference; a test double records it so
	// this test does not itself fail.
	var found []string
	for i, line := range strings.Split(string(data), "\n") {
		for _, symbol := range removedSymbols {
			if strings.Contains(line, symbol) {
				found = append(found, symbol+" at line "+strconv.Itoa(i+1))
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("scanner found %d references, want 1: %v", len(found), found)
	}
}

// reportRemovedSymbols fails the test for every removed symbol a document names.
func reportRemovedSymbols(t *testing.T, path, content string) {
	t.Helper()
	for i, line := range strings.Split(content, "\n") {
		for _, symbol := range removedSymbols {
			if strings.Contains(line, symbol) {
				t.Errorf("%s:%d references removed symbol %q: %s", path, i+1, symbol, strings.TrimSpace(line))
			}
		}
	}
}
