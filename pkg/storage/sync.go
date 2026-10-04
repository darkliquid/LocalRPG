package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
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

func (s *Syncer) Sync(dir string) (*SyncResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncResult{}, nil
		}
		return nil, fmt.Errorf("read entities dir: %w", err)
	}

	res := &SyncResult{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}

		fullPath, err := pathutil.ResolveSafeChild(dir, entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", fullPath, err)
		}

		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// Skip or log malformed markdown
			continue
		}
		if ent.ID == "" {
			ent.ID = strings.TrimSuffix(entry.Name(), ".md")
		}

		existing, err := s.store.GetEntity(ent.ID)
		if err != nil {
			// New entity
			if err := s.store.SaveEntity(ent); err != nil {
				return nil, fmt.Errorf("save entity %q: %w", ent.ID, err)
			}
			res.Added++
			continue
		}

		if existing.Hash == ent.Hash {
			res.Unchanged++
			continue
		}

		// Updated entity
		if err := s.store.SaveEntity(ent); err != nil {
			return nil, fmt.Errorf("update entity %q: %w", ent.ID, err)
		}
		res.Updated++
	}

	return res, nil
}

func (s *Syncer) SyncFile(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("sync file: invalid path %q: contains traversal", path)
	}
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("sync file: invalid path %q: contains traversal", cleanPath)
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("read %q: %w", cleanPath, err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return fmt.Errorf("parse %q: %w", cleanPath, err)
	}

	return s.store.SaveEntity(ent)
}
