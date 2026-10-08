package worldgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
	"gopkg.in/yaml.v3"
)

// DraftsDirName is the dot-directory drafts live in, beneath worlds/. The
// syncer skips a dot-directory, so a draft is never mistaken for a world.
const DraftsDirName = ".drafts"

// DraftPath is where a draft with id lives inside a drafts directory. The id is a
// base name rather than a path, so a draft can never be written or read outside
// the directory it belongs to.
func DraftPath(dir, id string) string {
	return filepath.Join(dir, filepath.Base(id)+".yaml")
}

// SaveDraft writes a draft to dir/<id>.yaml, creating the directory.
func SaveDraft(dir string, d Draft) error {
	if err := pathutil.ValidateID(d.ID); err != nil {
		return fmt.Errorf("invalid draft id %q: %w", d.ID, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create drafts dir: %w", err)
	}
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal draft: %w", err)
	}
	if err := os.WriteFile(DraftPath(dir, d.ID), data, 0o644); err != nil {
		return fmt.Errorf("write draft: %w", err)
	}
	return nil
}

// LoadDraft reads a draft from dir/<id>.yaml.
func LoadDraft(dir, id string) (Draft, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return Draft{}, fmt.Errorf("invalid draft id %q: %w", id, err)
	}
	data, err := os.ReadFile(DraftPath(dir, id))
	if err != nil {
		return Draft{}, fmt.Errorf("read draft %s: %w", id, err)
	}
	var d Draft
	if err := yaml.Unmarshal(data, &d); err != nil {
		return Draft{}, fmt.Errorf("parse draft %s: %w", id, err)
	}
	if d.ID == "" {
		d.ID = id
	}
	// A draft written before sections existed, or by hand, still reviews: the
	// lore is split into accept/reject units on load.
	if len(d.Sections) == 0 && strings.TrimSpace(d.Lore) != "" {
		d.Sections = SplitLoreSections(d.Lore)
	}
	return d, nil
}

// DeleteDraft removes a draft. A draft that is already gone is not an error.
func DeleteDraft(dir, id string) error {
	if err := pathutil.ValidateID(id); err != nil {
		return fmt.Errorf("invalid draft id %q: %w", id, err)
	}
	if err := os.Remove(DraftPath(dir, id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete draft %s: %w", id, err)
	}
	return nil
}

// ListDrafts returns the ids of the drafts in a directory, most recent first
// when the filesystem reports a usable order.
func ListDrafts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read drafts dir: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return ids, nil
}

// SplitLoreSections splits a lore document at its headings, so each section is
// an accept/reject unit. Content before the first heading becomes a section with
// an empty title.
func SplitLoreSections(lore string) []DraftSection {
	lines := strings.Split(strings.ReplaceAll(lore, "\r\n", "\n"), "\n")
	sections := make([]DraftSection, 0, 4)
	current := -1

	flush := func() {
		if current < 0 {
			return
		}
		sections[current].Body = strings.TrimSpace(sections[current].Body)
	}
	start := func(title string) {
		flush()
		sections = append(sections, DraftSection{Title: title})
		current = len(sections) - 1
	}

	for _, line := range lines {
		if title, ok := headingTitle(line); ok {
			start(title)
			continue
		}
		if current < 0 {
			if strings.TrimSpace(line) == "" {
				continue
			}
			start("")
		}
		sections[current].Body += line + "\n"
	}
	flush()
	return sections
}

// headingTitle reports whether a line is an ATX Markdown heading and returns its
// text. A "#hashtag" is not a heading.
func headingTitle(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	hashes := 0
	for hashes < len(trimmed) && trimmed[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 {
		return "", false
	}
	rest := trimmed[hashes:]
	if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}
