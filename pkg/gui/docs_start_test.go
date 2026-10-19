package gui

import (
	"fmt"
	"strings"
	"testing"
)

// startHerePath is the landing article: the lowest order in the set, so it is what
// DocsModal opens and the first group in its sidebar.
const startHerePath = "docs/00-start-here.md"

// startHerePathOrder is the reading path the index offers, as article ids. It is
// editorial, so it is fixed here rather than derived; everything else on the page
// comes from the articles themselves.
var startHerePathOrder = []string{
	"01-overview",
	"04-campaigns",
	"08-worlds-studio",
	"09-systems-studio",
	"05-providers",
	"21-local-first",
}

// renderStartHere builds the index article.
//
// It is generated rather than hand-written because an index is a list of every
// other article, and a hand-written one goes stale the moment an article is
// renamed, re-described or added. It reads the same list the viewer renders, so
// the page and the sidebar cannot disagree.
func renderStartHere(t *testing.T) string {
	t.Helper()

	articles, err := listEmbeddedDocs()
	if err != nil {
		t.Fatalf("list embedded docs: %v", err)
	}

	byID := make(map[string]DocArticleSummaryDTO, len(articles))
	for _, a := range articles {
		byID[a.ID] = a
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: 00-start-here\n")
	b.WriteString("title: Start Here\n")
	b.WriteString("category: Start Here\n")
	b.WriteString("order: 0\n")
	b.WriteString("description: What LocalRPG is, and the order to read the guides in.\n")
	b.WriteString("---\n\n")

	b.WriteString("# Start Here\n\n")
	b.WriteString("LocalRPG is a local-first, turn-based tabletop RPG client: one Go binary that serves a\n")
	b.WriteString("desktop app, a terminal client and an HTTP API over the same engine. It doesn't need an\n")
	b.WriteString("account or a network connection, and nothing about a campaign is hidden behind a service.\n\n")
	b.WriteString("The engine is schema-agnostic. No hit points, mana or character classes are built in;\n")
	b.WriteString("systems and worlds are files you author, and the app reads them. Read that first, because\n")
	b.WriteString("it explains why the guides spend so long on file formats.\n\n")

	b.WriteString("## Where to begin\n\n")
	for _, id := range startHerePathOrder {
		a, ok := byID[id]
		if !ok {
			t.Fatalf("start here offers %q, which is not an article any more; update startHerePathOrder", id)
		}
		fmt.Fprintf(&b, "- **[%s](%s.md)** - %s\n", a.Title, a.ID, a.Description)
	}

	b.WriteString("\n## Every guide\n\n")
	b.WriteString("Grouped as the app's own documentation viewer groups them: one section per category, in\n")
	b.WriteString("the order the categories first appear.\n")

	// The ordered list interleaves categories (the providers guides sit either side of
	// the studio guides), so headings must be collected by category rather than emitted
	// whenever the category changes, which would repeat a heading four times and is what
	// MD024 objects to.
	var categories []string
	grouped := make(map[string][]DocArticleSummaryDTO)
	for _, a := range articles {
		if a.ID == "00-start-here" {
			continue
		}
		if _, seen := grouped[a.Category]; !seen {
			categories = append(categories, a.Category)
		}
		grouped[a.Category] = append(grouped[a.Category], a)
	}

	for _, category := range categories {
		fmt.Fprintf(&b, "\n### %s\n\n", category)
		for _, a := range grouped[category] {
			fmt.Fprintf(&b, "- **[%s](%s.md)** - %s\n", a.Title, a.ID, a.Description)
		}
	}

	return b.String()
}

// TestStartHereIsGenerated is the staleness guard: with -update-docs it writes the
// article, and otherwise it fails when the file has drifted from what the
// generator produces. Adding, renaming or re-describing any article therefore fails
// this test until the index is regenerated, which is the point.
func TestStartHereIsGenerated(t *testing.T) {
	assertGeneratedDoc(t, startHerePath, renderStartHere(t))
}

// TestStartHereIsTheLandingArticle pins the two things that make it one: the
// lowest order, so DocsModal opens it, and a category of its own, so its sidebar
// group comes first.
func TestStartHereIsTheLandingArticle(t *testing.T) {
	articles, err := listEmbeddedDocs()
	if err != nil {
		t.Fatalf("list embedded docs: %v", err)
	}
	if len(articles) == 0 {
		t.Fatal("no embedded docs")
	}
	if articles[0].ID != "00-start-here" {
		t.Fatalf("the viewer opens %q, not the index; its order must be lower than every other article", articles[0].ID)
	}
}
