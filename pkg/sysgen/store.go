package sysgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/pathutil"
	"gopkg.in/yaml.v3"
)

// DraftsDirName is the dot-directory drafts live in, beneath systems/. The
// syncer skips a dot-directory, so a draft is never mistaken for a system.
const DraftsDirName = ".drafts"

// DraftPath is where a draft with id lives inside a drafts directory. The id is a
// base name rather than a path, so a draft can never be written or read outside
// the directory it belongs to.
func DraftPath(dir, id string) string {
	cleanDir := filepath.Clean(dir)
	cleanID := filepath.Base(id)
	cleanID = strings.TrimSuffix(cleanID, ".yaml")
	target := filepath.Clean(filepath.Join(cleanDir, cleanID+".yaml"))
	rel, err := filepath.Rel(cleanDir, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return filepath.Join(cleanDir, "invalid.yaml")
	}
	return target
}

// SaveDraft writes a draft to dir/<id>.yaml, creating the directory.
func SaveDraft(dir string, d System) error {
	if err := pathutil.ValidateID(d.ID); err != nil {
		return fmt.Errorf("invalid draft id %q: %w", d.ID, err)
	}
	cleanDir := filepath.Clean(dir)
	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return fmt.Errorf("create drafts dir: %w", err)
	}
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal draft: %w", err)
	}
	target := DraftPath(cleanDir, d.ID)
	rel, err := filepath.Rel(cleanDir, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("draft path %q escapes directory %q", target, cleanDir)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write draft: %w", err)
	}
	return nil
}

// LoadDraft reads a draft from dir/<id>.yaml.
func LoadDraft(dir, id string) (System, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return System{}, fmt.Errorf("invalid draft id %q: %w", id, err)
	}
	cleanDir := filepath.Clean(dir)
	target := DraftPath(cleanDir, id)
	rel, err := filepath.Rel(cleanDir, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return System{}, fmt.Errorf("draft path %q escapes directory %q", target, cleanDir)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return System{}, fmt.Errorf("read draft %s: %w", id, err)
	}
	var d System
	if err := yaml.Unmarshal(data, &d); err != nil {
		return System{}, fmt.Errorf("parse draft %s: %w", id, err)
	}
	if d.ID == "" {
		d.ID = id
	}
	return d, nil
}

// DeleteDraft removes a draft. A draft that is already gone is not an error.
func DeleteDraft(dir, id string) error {
	if err := pathutil.ValidateID(id); err != nil {
		return fmt.Errorf("invalid draft id %q: %w", id, err)
	}
	cleanDir := filepath.Clean(dir)
	target := DraftPath(cleanDir, id)
	rel, err := filepath.Rel(cleanDir, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("draft path %q escapes directory %q", target, cleanDir)
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete draft %s: %w", id, err)
	}
	return nil
}

// ListDraftIDs returns the ids of the drafts in a directory.
func ListDraftIDs(dir string) ([]string, error) {
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
