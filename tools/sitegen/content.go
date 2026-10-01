package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// docFrontmatter mirrors the header the GUI's documentation reader expects, so
// the site and the in-app reader agree about titles, ordering and grouping.
type docFrontmatter struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Category    string `yaml:"category"`
	Order       int    `yaml:"order"`
	Description string `yaml:"description"`
}

// doc is one rendered documentation article.
type doc struct {
	ID          string
	Title       string
	Category    string
	Description string
	Order       int
	Source      string // repository-relative path, used to resolve links
	URL         string // site-relative URL
	Body        string // markdown body with the frontmatter removed
}

// docGroup collects articles under one category for the navigation.
type docGroup struct {
	Category string
	Docs     []doc
}

// shot is one entry in the screenshot gallery. A missing image still renders,
// as an on-brand placeholder, so the gallery is never empty.
type shot struct {
	File    string
	Title   string
	Caption string
	Present bool
}

// referenceOrder pushes the supplementary pages (README, debugging guide) to
// the bottom of the navigation without disturbing the guide's own ordering.
const referenceOrder = 100

// docSource points at one Markdown file and supplies the metadata a file may
// not carry itself.
type docSource struct {
	rel      string
	id       string
	title    string
	category string
	order    int
}

// loadDocs reads the documentation the application embeds and turns it into
// site articles. Anything malformed is reported rather than skipped: a broken
// article should fail the build, not silently vanish from the site.
func loadDocs(cfg config) ([]doc, error) {
	dir := filepath.Join(cfg.root, "pkg", "gui", "docs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read embedded docs: %w", err)
	}

	var docs []doc
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		article, err := loadDoc(cfg.root, docSource{rel: path.Join("pkg", "gui", "docs", entry.Name())})
		if err != nil {
			return nil, err
		}
		docs = append(docs, article)
	}

	extra := []docSource{
		{rel: "README.md", id: "readme", title: "Project README", category: "Reference", order: referenceOrder},
		{rel: "docs/debugging.md", id: "debugging", category: "Reference", order: referenceOrder + 1},
	}
	for _, src := range extra {
		if _, err := os.Stat(filepath.Join(cfg.root, src.rel)); err != nil {
			continue // supplementary pages are optional
		}
		article, err := loadDoc(cfg.root, src)
		if err != nil {
			return nil, err
		}
		docs = append(docs, article)
	}

	for i := range docs {
		docs[i].URL = "docs/" + docs[i].ID + ".html"
	}
	return docs, nil
}

// loadDoc parses one Markdown file into an article. Files without frontmatter
// fall back to the source's id, title and category, and finally to the first
// heading as the title.
func loadDoc(root string, src docSource) (doc, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(src.rel)))
	if err != nil {
		return doc{}, fmt.Errorf("read %s: %w", src.rel, err)
	}

	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return doc{}, fmt.Errorf("parse %s: %w", src.rel, err)
	}

	article := doc{
		ID:          fm.ID,
		Title:       fm.Title,
		Category:    fm.Category,
		Description: fm.Description,
		Order:       fm.Order,
		Source:      src.rel,
		Body:        stripLeadingHeading(body),
	}
	if article.ID == "" {
		article.ID = src.id
	}
	if article.ID == "" {
		article.ID = strings.TrimSuffix(path.Base(src.rel), ".md")
	}
	if article.Title == "" {
		article.Title = src.title
	}
	if article.Title == "" {
		article.Title = firstHeading(body)
	}
	if article.Title == "" {
		article.Title = article.ID
	}
	if article.Category == "" {
		article.Category = src.category
	}
	if article.Order == 0 {
		article.Order = src.order
	}
	return article, nil
}

// stripLeadingHeading removes a document's opening level-one heading, because
// the page template already prints the title above the body.
func stripLeadingHeading(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "# ") {
			return body
		}
		remaining := append(lines[:i:i], lines[i+1:]...)
		return strings.TrimLeft(strings.Join(remaining, "\n"), "\n")
	}
	return body
}

// splitFrontmatter separates an optional YAML header from the Markdown body.
// A file with no header is treated as all body.
func splitFrontmatter(raw []byte) (docFrontmatter, string, error) {
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return docFrontmatter{}, content, nil
	}
	end := strings.Index(content[4:], "\n---\n")
	if end == -1 {
		return docFrontmatter{}, "", fmt.Errorf("unclosed frontmatter")
	}
	var fm docFrontmatter
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &fm); err != nil {
		return docFrontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, strings.TrimSpace(content[4+end+5:]), nil
}

// firstHeading returns the text of the first level-one heading, which is how a
// document without frontmatter gets a title.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// groupDocs buckets articles by category, ordering the articles within each
// group the way the in-app reader does and the groups by the earliest article
// they contain.
func groupDocs(docs []doc) []docGroup {
	byCategory := map[string][]doc{}
	for _, d := range docs {
		byCategory[d.Category] = append(byCategory[d.Category], d)
	}

	groups := make([]docGroup, 0, len(byCategory))
	for category, list := range byCategory {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Order != list[j].Order {
				return list[i].Order < list[j].Order
			}
			return list[i].Title < list[j].Title
		})
		groups = append(groups, docGroup{Category: category, Docs: list})
	}
	sort.Slice(groups, func(i, j int) bool {
		li, lj := groups[i].Docs[0], groups[j].Docs[0]
		if li.Order != lj.Order {
			return li.Order < lj.Order
		}
		return groups[i].Category < groups[j].Category
	})
	return groups
}

// showcaseShots is the gallery the site wants to show. A shot whose file is
// missing renders as a placeholder frame instead of being dropped.
//
// The first four are captured by website/scenarios/screenshots.yaml; the last
// two need a campaign open in the theatre, so they are captured by hand.
var showcaseShots = []shot{
	{
		File:    "01-launcher.jpg",
		Title:   "Launcher Hub",
		Caption: "Campaigns, worlds and systems in one dock, with a hero stage for the one you played last.",
	},
	{
		File:    "02-worlds-studio.jpg",
		Title:   "Worlds Studio",
		Caption: "Author lore prompts, art direction and starter entities without leaving the app.",
	},
	{
		File:    "03-systems-studio.jpg",
		Title:   "Systems Studio",
		Caption: "Write rules prompts, wire mechanics.js hooks, and test dice expressions inline.",
	},
	{
		File:    "04-settings.jpg",
		Title:   "Settings Studio",
		Caption: "Route every role to a local or CLI model, with live latency diagnostics and cost tracking.",
	},
	{
		File:    "05-codex.jpg",
		Title:   "Codex Drawer",
		Caption: "Entity notes, wikilink graphs and per-character voice overrides in one drawer.",
	},
	{
		File:    "06-story-theatre.jpg",
		Title:   "Story Theatre",
		Caption: "Replay a campaign as a visual novel, each line spoken in its own character's voice.",
	},
}

// loadShots reports which showcase screenshots are on disk.
func loadShots(dir string) []shot {
	shots := make([]shot, len(showcaseShots))
	copy(shots, showcaseShots)
	for i := range shots {
		if _, err := os.Stat(filepath.Join(dir, shots[i].File)); err == nil {
			shots[i].Present = true
		}
	}
	return shots
}

// presentShots counts how many gallery entries have a real image.
func presentShots(shots []shot) int {
	n := 0
	for _, s := range shots {
		if s.Present {
			n++
		}
	}
	return n
}
