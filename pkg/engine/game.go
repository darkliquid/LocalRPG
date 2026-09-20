package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

type Session struct {
	Paths    *core.PathResolver
	Manifest *core.GameManifest
	System   *core.SystemManifest
	World    *core.WorldManifest
	Store    *storage.Store
}

func (s *Session) Close() error {
	if s.Store != nil {
		return s.Store.Close()
	}
	return nil
}

func InitGame(paths *core.PathResolver, gameID, systemID, worldID, playerName string) (*Session, error) {
	// Verify system & world exist
	sysManifest, err := core.LoadSystemManifest(filepath.Join(paths.SystemDir(systemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", systemID, err)
	}

	worldManifest, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world %q: %w", worldID, err)
	}

	gameDir := paths.GameDir(gameID)
	gameEntitiesDir := filepath.Join(gameDir, "entities")
	gameCacheDir := filepath.Join(gameDir, "cache")
	gameAssetsDir := filepath.Join(gameDir, "assets")

	for _, d := range []string{gameDir, gameEntitiesDir, gameCacheDir, gameAssetsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create dir %q: %w", d, err)
		}
	}

	// Create game manifest
	manifest := &core.GameManifest{
		ID:       gameID,
		Name:     gameID,
		SystemID: systemID,
		WorldID:  worldID,
		Player:   playerName,
	}

	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("write game.yaml: %w", err)
	}

	// Copy initial template entities from world into game
	worldEntitiesDir := filepath.Join(paths.WorldDir(worldID), "entities")
	if entries, err := os.ReadDir(worldEntitiesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join(worldEntitiesDir, e.Name())
			dst := filepath.Join(gameEntitiesDir, e.Name())
			if err := copyFile(src, dst); err != nil {
				return nil, fmt.Errorf("copy entity template %q: %w", e.Name(), err)
			}
		}
	}

	// Open store and run initial sync
	dbPath := filepath.Join(gameCacheDir, "index.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}

	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(gameEntitiesDir); err != nil {
		store.Close()
		return nil, fmt.Errorf("initial sync: %w", err)
	}

	return &Session{
		Paths:    paths,
		Manifest: manifest,
		System:   sysManifest,
		World:    worldManifest,
		Store:    store,
	}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
