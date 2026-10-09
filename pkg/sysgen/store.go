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

// draftTarget is where a draft with id lives inside dir. The id is reduced to a
// single base name, and the resolved path is checked to sit inside dir, so a
// draft can never be written or read outside the directory it belongs to.
func draftTarget(dir, id string) (string, error) {
	safeDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return "", fmt.Errorf("resolve drafts dir: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(safeDir, filepath.Base(id)+".yaml"))
	if err != nil {
		return "", fmt.Errorf("resolve draft path: %w", err)
	}
	if !strings.HasPrefix(target, safeDir+string(filepath.Separator)) {
		return "", fmt.Errorf("draft path %q escapes directory %q", target, safeDir)
	}
	return target, nil
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
	target, err := draftTarget(cleanDir, d.ID)
	if err != nil {
		return err
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
	target, err := draftTarget(dir, id)
	if err != nil {
		return System{}, err
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
	target, err := draftTarget(dir, id)
	if err != nil {
		return err
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
