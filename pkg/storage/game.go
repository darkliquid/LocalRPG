package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/pathutil"
)

// legacyGameDB is the pre-timeline database name. It is retired on first open.
const legacyGameDB = "game.db"

var gameStores = NewPool()

// OpenGameStore returns the shared canonical store for a campaign, retiring a
// legacy game.db first. It is the only way a game database is opened.
func OpenGameStore(paths *core.PathResolver, gameID string) (*Store, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("open game store: invalid game id %q: %w", gameID, err)
	}
	if paths == nil {
		return nil, fmt.Errorf("open game store %q: missing path resolver", gameID)
	}

	dbPath := paths.GameDBPath(gameID)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create game cache dir: %w", err)
	}

	if err := retireLegacyGameDB(paths, gameID); err != nil {
		return nil, err
	}

	store, err := gameStores.Store(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open game store %q: %w", gameID, err)
	}
	return store, nil
}

// CloseGameStores releases every pooled game store.
func CloseGameStores() error {
	return gameStores.Close()
}

// CloseGameStore releases the pooled handle for one campaign so its directory can
// be removed or rebuilt. Closing it is safe even when it was never opened.
func CloseGameStore(paths *core.PathResolver, gameID string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("close game store: invalid game id %q: %w", gameID, err)
	}
	if paths == nil {
		return nil
	}
	if err := gameStores.Evict(paths.GameDBPath(gameID)); err != nil {
		return fmt.Errorf("close game store %q: %w", gameID, err)
	}
	return nil
}

func retireLegacyGameDB(paths *core.PathResolver, gameID string) error {
	legacy, err := pathutil.ResolveSafeChild(paths.GameDir(gameID), legacyGameDB)
	if err != nil {
		return fmt.Errorf("inspect legacy game database: %w", err)
	}

	if _, err := os.Stat(legacy); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect legacy game database: %w", err)
	}

	// Walk the WAL sidecars too, so no stale bytes are left behind.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from := legacy + suffix
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, from+".legacy"); err != nil {
			return fmt.Errorf("retire legacy game database: %w", err)
		}
	}
	return nil
}
