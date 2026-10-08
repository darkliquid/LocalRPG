package ingest

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFolderReadsTextAndSkipsBinaries(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.md"), "# Saltmarch\n\nA port.\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "Notes.\n")
	if err := os.WriteFile(filepath.Join(dir, "c.png"), []byte{0x89, 0x50}, 0o644); err != nil {
		t.Fatal(err)
	}

	chunks, err := ExtractFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	if chunks[0].Title == "" {
		t.Fatal("a chunk should carry its source title")
	}
	if chunks[0].Source != "a.md" {
		t.Fatalf("source = %q, want a.md", chunks[0].Source)
	}
}

func TestExtractFolderSkipsDotDirectories(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "keep.md"), "Kept.\n")
	mustWrite(t, filepath.Join(dir, ".git", "hidden.md"), "Hidden.\n")

	chunks, err := ExtractFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0].Source != "keep.md" {
		t.Fatalf("chunks = %+v", chunks)
	}
}

func TestExtractFolderRejectsAnEmptyFolder(t *testing.T) {
	if _, err := ExtractFolder(t.TempDir()); err == nil {
		t.Fatal("an empty folder must error, not produce an empty world")
	}
}

func TestExtractFolderMakesNoNetworkCall(t *testing.T) {
	original := httpClient
	httpClient = &http.Client{Transport: failTransport{t: t}}
	defer func() { httpClient = original }()

	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.md"), "A port.\n")
	if _, err := Extract(context.Background(), Source{Kind: "folder", Path: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestExtractRejectsAnUnknownKind(t *testing.T) {
	if _, err := Extract(context.Background(), Source{Kind: "nonsense"}); err == nil {
		t.Fatal("an unknown source kind must error")
	}
}

func TestSplitTextChunksAtHeadings(t *testing.T) {
	var b strings.Builder
	b.WriteString("# One\n\n" + strings.Repeat("a", 3000) + "\n\n")
	b.WriteString("## Two\n\n" + strings.Repeat("b", 3000) + "\n")
	parts := SplitText(b.String())
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want at least 2", len(parts))
	}
	for _, p := range parts {
		if len(p) > ChunkLimit {
			t.Fatalf("chunk of %d bytes exceeds the limit", len(p))
		}
	}
}

func TestSplitTextKeepsASmallDocumentWhole(t *testing.T) {
	parts := SplitText("A port.\n")
	if len(parts) != 1 || parts[0] != "A port." {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestBatchGroupsChunks(t *testing.T) {
	chunks := []Chunk{{Text: "a"}, {Text: "b"}, {Text: "c"}}
	if got := Batch(chunks, 2); len(got) != 2 || len(got[1]) != 1 {
		t.Fatalf("batches = %+v", got)
	}
	if got := Batch(nil, 2); got != nil {
		t.Fatalf("batches = %+v", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSplitTextStripsFrontmatterAndNavigation(t *testing.T) {
	page := "---\ntitle: Saltmarch\nmythicId: abc123\nmythicOrder: 3/2\n---\n\n# Saltmarch\n\nA port.\n\n- [Overview](overview.md)\n- [Key events](key-events.md)\n"
	parts := SplitText(page)
	if len(parts) != 1 {
		t.Fatalf("parts = %+v", parts)
	}
	got := parts[0]
	if strings.Contains(got, "mythicId") || strings.Contains(got, "abc123") {
		t.Fatalf("frontmatter survived: %q", got)
	}
	if strings.Contains(got, "overview.md") {
		t.Fatalf("a link list survived: %q", got)
	}
	if !strings.Contains(got, "A port.") {
		t.Fatalf("the prose was lost: %q", got)
	}
}

func TestSplitTextDropsAPageThatIsOnlyNavigation(t *testing.T) {
	if parts := SplitText("---\ntitle: Glossary\n---\n\n- [The Cold Dark](the-cold-dark.md)\n"); parts != nil {
		t.Fatalf("parts = %+v, want none", parts)
	}
}

func TestSplitTextKeepsAProseBulletWithALink(t *testing.T) {
	// A list item that carries prose is content, not a table of contents.
	page := "- **The Mourning March:** the deceased is carried through the city, see [the route](route.md).\n"
	parts := SplitText(page)
	if len(parts) != 1 || !strings.Contains(parts[0], "Mourning March") {
		t.Fatalf("parts = %+v", parts)
	}
}
