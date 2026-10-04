package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type SyncResult struct {
	Added     int
	Updated   int
	Unchanged int
}

type Syncer struct {
	store *Store
}

func NewSyncer(store *Store) *Syncer {
	return &Syncer{store: store}
}

// Sync indexes every note under dir. The walk is recursive, because a note's
// folder is a location the author chose; hidden directories and assets/ are not
// note storage, so they are skipped whole.
func (s *Syncer) Sync(dir string) (*SyncResult, error) {
	res := &SyncResult{}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path == dir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "assets" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}

		outcome, err := s.syncNote(dir, path)
		if err != nil {
			return err
		}
		switch outcome {
		case syncAdded:
			res.Added++
		case syncUpdated:
			res.Updated++
		default:
			res.Unchanged++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// syncOutcome tells the walk whether a note was new, changed, or already indexed.
type syncOutcome int

const (
	syncUnchanged syncOutcome = iota
	syncAdded
	syncUpdated
)

// syncNote parses one note and upserts it, so the skip-on-parse-failure rule and
// the "did anything change" question live in one place. A move changes no bytes,
// so the folder is part of what decides whether the index is current.
func (s *Syncer) syncNote(root, path string) (syncOutcome, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return syncUnchanged, fmt.Errorf("read %q: %w", path, err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		// A note that will not parse is skipped rather than fatal: a missing
		// entity in the codex is almost always a frontmatter typo, and one bad
		// file must not stop the rest of the campaign from indexing.
		return syncUnchanged, nil
	}
	if ent.ID == "" {
		ent.ID = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	ent.Folder = folderFromPath(root, path)

	existing, existingErr := s.store.GetEntity(ent.ID)
	if existingErr == nil && existing.Hash == ent.Hash && existing.Folder == ent.Folder {
		return syncUnchanged, nil
	}

	if err := s.store.SaveEntity(ent); err != nil {
		return syncUnchanged, fmt.Errorf("save entity %q: %w", ent.ID, err)
	}
	if existingErr != nil {
		return syncAdded, nil
	}
	return syncUpdated, nil
}

// folderFromPath is the stored folder form: the directory relative to the
// entities root, slash-separated, with "" for the root itself.
func folderFromPath(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil || rel == "." || rel == "" {
		return ""
	}
	return filepath.ToSlash(rel)
}

// SyncFile indexes one note, which is what a save uses so the index never waits
// for a full resync. The collection root is found by walking up to the entities/
// directory, so no caller can pass the wrong one and the signature stays a single
// path.
func (s *Syncer) SyncFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}

	ent.Folder = folderFromPath(entitiesRootFor(path), path)
	return s.store.SaveEntity(ent)
}

// entitiesRootFor walks up from a note's path to the entities/ directory that
// contains it. A note always lives under one, so the root needs no argument.
func entitiesRootFor(path string) string {
	dir := filepath.Dir(path)
	for {
		if filepath.Base(dir) == "entities" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}
