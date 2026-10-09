package main

import (
	"github.com/darkliquid/localrpg/pkg/scene"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	raw := []byte("---\nid: 01-overview\ntitle: Architecture & Core Concepts\norder: 1\n---\n\n# Heading\n\nBody text.\n")

	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if fm.ID != "01-overview" {
		t.Errorf("id = %q, want %q", fm.ID, "01-overview")
	}
	if fm.Title != "Architecture & Core Concepts" {
		t.Errorf("title = %q", fm.Title)
	}
	if fm.Order != 1 {
		t.Errorf("order = %d, want 1", fm.Order)
	}
	if !strings.HasPrefix(body, "# Heading") {
		t.Errorf("body = %q, want it to start with the heading", body)
	}
}

func TestSplitFrontmatterWithoutHeader(t *testing.T) {
	fm, body, err := splitFrontmatter([]byte("# Just a heading\n\ntext\n"))
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if fm.ID != "" {
		t.Errorf("id = %q, want empty", fm.ID)
	}
	if !strings.HasPrefix(body, "# Just a heading") {
		t.Errorf("body = %q", body)
	}
}

func TestSplitFrontmatterRejectsUnclosedHeader(t *testing.T) {
	if _, _, err := splitFrontmatter([]byte("---\nid: x\n")); err == nil {
		t.Fatal("expected an error for an unclosed frontmatter block")
	}
}

func TestStripLeadingHeading(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"drops opening heading", "# Title\n\nBody.\n", "Body.\n"},
		{"keeps later headings", "# Title\n\n## Section\n", "## Section\n"},
		{"leaves body alone", "Intro first.\n\n# Title\n", "Intro first.\n\n# Title\n"},
		{"tolerates blank lines", "\n\n# Title\nBody.\n", "Body.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripLeadingHeading(tc.in); got != tc.want {
				t.Errorf("stripLeadingHeading(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRootPrefix(t *testing.T) {
	cases := map[string]string{
		"index.html":      "",
		"docs/foo.html":   "../",
		"a/b/foo.html":    "../../",
		"docs/index.html": "../",
		"deep/a/b/c.html": "../../../",
	}
	for url, want := range cases {
		if got := rootPrefix(url); got != want {
			t.Errorf("rootPrefix(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestLinkIndexResolve(t *testing.T) {
	docs := []doc{
		{ID: "05-providers", Source: "pkg/gui/docs/05-providers.md", URL: "docs/05-providers.html"},
		{ID: "11-usage-and-pricing", Source: "pkg/gui/docs/11-usage-and-pricing.md", URL: "docs/11-usage-and-pricing.html"},
		{ID: "readme", Source: "README.md", URL: "docs/readme.html"},
		{ID: "debugging", Source: "docs/debugging.md", URL: "docs/debugging.html"},
	}
	ix := newLinkIndex(docs, "https://example.com/repo")

	current := docs[0]
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{"doc id", "11-usage-and-pricing", "../docs/11-usage-and-pricing.html"},
		{"doc id with fragment", "11-usage-and-pricing#spend", "../docs/11-usage-and-pricing.html#spend"},
		{"source path", "pkg/gui/docs/05-providers.md", "../docs/05-providers.html"},
		{"external", "https://ollama.com", "https://ollama.com"},
		{"anchor", "#section", "#section"},
		{"mailto", "mailto:hi@example.com", "mailto:hi@example.com"},
		{"unknown falls back to repo", "docs/missing.md", "https://example.com/repo/blob/main/pkg/gui/docs/docs/missing.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ix.resolve(current, tc.target); got != tc.want {
				t.Errorf("resolve(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}

	// A link written relative to a root-level file must climb out of docs/ too.
	if got := ix.resolve(docs[2], "docs/debugging.md"); got != "../docs/debugging.html" {
		t.Errorf("resolve from README = %q, want %q", got, "../docs/debugging.html")
	}
}

func TestGuardOutput(t *testing.T) {
	for _, unsafe := range []string{".", "/", "dist", ""} {
		if err := guardOutput(unsafe); err == nil {
			t.Errorf("guardOutput(%q) allowed an unsafe output directory", unsafe)
		}
	}
	if err := guardOutput("website/dist"); err != nil {
		t.Errorf("guardOutput(%q) = %v, want nil", "website/dist", err)
	}
}

func TestBuildWritesSite(t *testing.T) {
	out := t.TempDir()
	cfg := config{
		root:        "../..",
		out:         out,
		screenshots: "../../website/screenshots",
		repoURL:     defaultRepoURL,
	}

	if err := build(cfg); err != nil {
		t.Fatalf("build: %v", err)
	}

	for _, rel := range []string{
		"index.html",
		".nojekyll",
		"assets/style.css",
		"assets/app.js",
		"docs/index.html",
		"docs/01-overview.html",
		"docs/readme.html",
	} {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Errorf("expected %s in the built site: %v", rel, err)
		}
	}

	home, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(home), "LocalRPG") {
		t.Error("home page does not mention LocalRPG")
	}

	// Documentation links must resolve from inside docs/, not from the root.
	article, err := os.ReadFile(filepath.Join(out, "docs", "05-providers.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(article), `href="../docs/11-usage-and-pricing.html"`) {
		t.Error("cross-reference link was not rewritten relative to the article")
	}
}

// TestSiteBackgroundUsesTheGenrePalette guards that the site's background reads
// the same palette table the app and the export use.
func TestSiteBackgroundUsesTheGenrePalette(t *testing.T) {
	palette := scene.GenrePaletteFor("")
	vars := string(genreVars())
	for _, want := range []string{palette.From, palette.To, palette.Accent} {
		if !strings.Contains(vars, want) {
			t.Fatalf("the genre variables omit %s: %s", want, vars)
		}
	}

	renderer, err := newRenderer()
	if err != nil {
		t.Fatalf("newRenderer: %v", err)
	}
	page, err := renderer.render("home.html.tmpl", &pageData{Root: "", Title: "Home", Description: "d"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(page), palette.From) {
		t.Fatal("the rendered page does not carry the palette")
	}
}
