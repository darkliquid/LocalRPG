package storage

import (
	"fmt"
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

		fullPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", fullPath, err)
		}

		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// Skip or log malformed markdown
			continue
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
