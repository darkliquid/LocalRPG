package worldgen

import "strings"

// HooksHeading is the lore section story hooks are appended under.
const HooksHeading = "Hooks"

// ApplyLore appends the accepted lore proposals to a document. A proposal with a
// target naming an existing section is inserted into it; otherwise it becomes a
// new section at the end. An empty set returns the document unchanged.
func ApplyLore(existing string, proposals []Enhancement) string {
	out := existing
	for _, p := range proposals {
		if p.Kind != KindLore {
			continue
		}
		block := loreBlock(p)
		if block == "" {
			continue
		}
		out = appendSection(out, p.Target, block)
	}
	return out
}

// ApplyHooks appends the accepted hook proposals under a "## Hooks" section,
// creating the section when the document has none. An empty set returns the
// document unchanged.
func ApplyHooks(existing string, proposals []Enhancement) string {
	blocks := make([]string, 0, len(proposals))
	for _, p := range proposals {
		if p.Kind != KindHook {
			continue
		}
		if block := hookBlock(p); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return existing
	}

	doc := existing
	if !hasHeading(doc, HooksHeading) {
		base := strings.TrimRight(doc, "\n")
		if base == "" {
			doc = "## " + HooksHeading + "\n"
		} else {
			doc = base + "\n\n## " + HooksHeading + "\n"
		}
	}
	return appendSection(doc, HooksHeading, strings.Join(blocks, "\n\n"))
}

// loreBlock renders one lore proposal as a Markdown section.
func loreBlock(p Enhancement) string {
	title := strings.TrimSpace(p.Title)
	body := strings.TrimSpace(p.Body)
	if title == "" && body == "" {
		return ""
	}
	var b strings.Builder
	if title != "" {
		b.WriteString("## " + title + "\n\n")
	}
	if body != "" {
		b.WriteString(body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// hookBlock renders one hook proposal as a Markdown subsection.
func hookBlock(p Enhancement) string {
	title := strings.TrimSpace(p.Title)
	body := strings.TrimSpace(p.Body)
	if title == "" && body == "" {
		return ""
	}
	var b strings.Builder
	if title != "" {
		b.WriteString("### " + title + "\n\n")
	}
	if body != "" {
		b.WriteString(body + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// appendSection adds a block to a document, under a named heading when one
// exists, otherwise as a new section at the end. The existing text is never
// rewritten.
func appendSection(doc, target, block string) string {
	if strings.TrimSpace(target) != "" {
		if out, ok := insertUnderHeading(doc, strings.TrimSpace(target), block); ok {
			return out
		}
	}
	base := strings.TrimRight(doc, "\n")
	if base == "" {
		return block + "\n"
	}
	return base + "\n\n" + block + "\n"
}

// hasHeading reports whether a document already has a heading with this title.
func hasHeading(doc, title string) bool {
	for _, line := range strings.Split(doc, "\n") {
		if found, ok := headingTitle(line); ok && strings.EqualFold(found, title) {
			return true
		}
	}
	return false
}

// insertUnderHeading appends a block to the end of the named section, before the
// next heading. It reports false when no such heading exists.
func insertUnderHeading(doc, target, block string) (string, bool) {
	lines := strings.Split(doc, "\n")
	start := -1
	for i, line := range lines {
		if title, ok := headingTitle(line); ok && strings.EqualFold(title, target) {
			start = i
			break
		}
	}
	if start < 0 {
		return doc, false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if _, ok := headingTitle(lines[i]); ok {
			end = i
			break
		}
	}
	// Drop the section's trailing blank lines so the insert keeps one gap.
	insertAt := end
	for insertAt > start+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:insertAt]...)
	out = append(out, "")
	out = append(out, strings.Split(block, "\n")...)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n"), true
}
