package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type Service struct {
	rootDir  string
	resolver *core.PathResolver
}

func NewService(rootDir string) *Service {
	return &Service{
		rootDir:  rootDir,
		resolver: core.NewPathResolver(rootDir),
	}
}

func (s *Service) GetGameState(ctx context.Context, gameID string) (*GameStateDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	manifestPath := filepath.Join(gameDir, "game.yaml")
	gameManifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}

	playerID := gameManifest.Player
	playerFile := filepath.Join(gameDir, "entities", playerID+".md")
	data, err := os.ReadFile(playerFile)
	if err != nil {
		return nil, fmt.Errorf("read player entity: %w", err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return nil, fmt.Errorf("parse player entity: %w", err)
	}

	var stateMap map[string]interface{}
	if ent.State != nil {
		stateMap = ent.State.Raw()
	}

	return &GameStateDTO{
		GameID:   gameID,
		GameName: gameManifest.Name,
		Player: PlayerDTO{
			ID:    playerID,
			Name:  ent.Name,
			Type:  ent.Type,
			State: stateMap,
		},
		Arcs:      []NarrativeArcDTO{},
		Clocks:    []FactionClockDTO{},
		Locations: []string{},
	}, nil
}

func (s *Service) GetEntity(ctx context.Context, gameID, entityID string) (*EntityDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	path := filepath.Join(gameDir, "entities", entityID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity file: %w", err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return nil, fmt.Errorf("parse entity: %w", err)
	}

	var stateMap map[string]interface{}
	if ent.State != nil {
		stateMap = ent.State.Raw()
	}

	dbPath := filepath.Join(gameDir, "game.db")
	store, err := storage.NewStore(dbPath)
	var backlinks []string
	if err == nil {
		defer store.Close()
		edges, _ := store.GetEdgesTo(entityID)
		for _, e := range edges {
			backlinks = append(backlinks, e.SourceID)
		}
	}

	return &EntityDTO{
		ID:        entityID,
		Name:      ent.Name,
		Type:      ent.Type,
		Markdown:  string(data),
		State:     stateMap,
		Backlinks: backlinks,
	}, nil
}

func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	gameDir := s.resolver.GameDir(gameID)
	path := filepath.Join(gameDir, "entities", entityID+".md")
	if err := os.WriteFile(path, []byte(rawMarkdown), 0644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}

	dbPath := filepath.Join(gameDir, "game.db")
	store, err := storage.NewStore(dbPath)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}
	defer store.Close()

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(path)
}

func (s *Service) GetGraph(ctx context.Context, gameID string) (*GraphDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	entries, err := os.ReadDir(entitiesDir)
	if err != nil {
		return nil, fmt.Errorf("read entities dir: %w", err)
	}

	nodes := make([]GraphNodeDTO, 0, len(entries))
	links := make([]GraphLinkDTO, 0)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(entitiesDir, entry.Name()))
		if err != nil {
			continue
		}
		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			continue
		}

		nodes = append(nodes, GraphNodeDTO{
			ID:    id,
			Label: ent.Name,
			Type:  ent.Type,
		})

		for _, target := range ent.Wikilinks {
			links = append(links, GraphLinkDTO{
				Source: id,
				Target: target,
			})
		}
	}

	return &GraphDTO{
		Nodes: nodes,
		Links: links,
	}, nil
}

func (s *Service) GetChronicle(ctx context.Context, gameID string) ([]TurnDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)
	turns, err := logger.LoadHistory()
	if err != nil {
		return []TurnDTO{}, nil
	}

	dtos := make([]TurnDTO, len(turns))
	for i, turn := range turns {
		audioURL := ""
		if len(turn.AudioRefs) > 0 {
			audioURL = turn.AudioRefs[0]
		}
		dtos[i] = TurnDTO{
			TurnNumber: turn.Number,
			InputText:  turn.Input,
			Mode:       turn.Mode,
			Prose:      turn.Output,
			AudioURL:   audioURL,
		}
	}
	return dtos, nil
}
