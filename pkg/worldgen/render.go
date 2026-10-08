package worldgen

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// RenderEntityNote renders a draft entity as a Markdown note with YAML
// frontmatter, the shape entity.ParseMarkdownEntity and the studio writer both
// expect. Source is written as frontmatter so provenance survives the commit.
func RenderEntityNote(e DraftEntity) string {
	frontmatter := struct {
		ID     string   `yaml:"id"`
		Name   string   `yaml:"name"`
		Type   string   `yaml:"type"`
		Tags   []string `yaml:"tags,omitempty"`
		Source string   `yaml:"source,omitempty"`
	}{ID: e.ID, Name: e.Name, Type: e.Type, Tags: e.Tags, Source: e.Source}

	head, err := yaml.Marshal(frontmatter)
	if err != nil {
		head = []byte("id: " + e.ID + "\n")
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(head)
	b.WriteString("---\n\n")
	body := strings.TrimSpace(e.Body)
	if e.Name != "" && !strings.HasPrefix(body, "# ") {
		b.WriteString("# ")
		b.WriteString(e.Name)
		b.WriteString("\n\n")
	}
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}

// RenderLore joins accepted lore sections into a document, one heading each.
func RenderLore(sections []DraftSection) string {
	var b strings.Builder
	for _, section := range sections {
		body := strings.TrimSpace(section.Body)
		title := strings.TrimSpace(section.Title)
		if title == "" {
			if body != "" {
				b.WriteString(body + "\n\n")
			}
			continue
		}
		b.WriteString("## " + title + "\n\n")
		if body != "" {
			b.WriteString(body + "\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
