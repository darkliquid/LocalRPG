package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (s *Service) ListGames(ctx context.Context) ([]GameSummaryDTO, error) {
	gamesDir := s.resolver.GamesDir()
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []GameSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read games dir: %w", err)
	}

	summaries := make([]GameSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gameID := e.Name()
		gameDir := filepath.Join(gamesDir, gameID)
		manifestPath := filepath.Join(gameDir, "game.yaml")
		m, err := core.LoadGameManifest(manifestPath)
		if err != nil {
			continue
		}

		turnCount := 0
		historyPath := filepath.Join(gameDir, "history.jsonl")
		logger := engine.NewHistoryLogger(historyPath)
		if turns, err := logger.LoadHistory(); err == nil {
			turnCount = len(turns)
		}

		lastPlayed := ""
		if fi, err := os.Stat(manifestPath); err == nil {
			lastPlayed = fi.ModTime().Format(time.RFC3339)
		}

		name := m.Name
		if name == "" {
			name = gameID
		}

		summaries = append(summaries, GameSummaryDTO{
			ID:         gameID,
			Name:       name,
			SystemID:   m.SystemID,
			WorldID:    m.WorldID,
			PlayerName: m.Player,
			TurnCount:  turnCount,
			LastPlayed: lastPlayed,
		})
	}
	return summaries, nil
}

func (s *Service) ListSystems(ctx context.Context) ([]SystemSummaryDTO, error) {
	sysDir := s.resolver.SystemsDir()
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SystemSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read systems dir: %w", err)
	}

	summaries := make([]SystemSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(sysDir, e.Name(), "system.yaml")
		m, err := core.LoadSystemManifest(manifestPath)
		if err != nil {
			continue
		}
		summaries = append(summaries, SystemSummaryDTO{
			ID:          m.ID,
			Name:        m.Name,
			Description: m.Description,
			Version:     m.Version,
		})
	}
	return summaries, nil
}

func (s *Service) ListWorlds(ctx context.Context) ([]WorldSummaryDTO, error) {
	worldsDir := s.resolver.WorldsDir()
	entries, err := os.ReadDir(worldsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []WorldSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read worlds dir: %w", err)
	}

	summaries := make([]WorldSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(worldsDir, e.Name(), "world.yaml")
		m, err := core.LoadWorldManifest(manifestPath)
		if err != nil {
			continue
		}
		compat := make([]string, 0)
		if m.DefaultSystem != "" {
			compat = append(compat, m.DefaultSystem)
		}
		summaries = append(summaries, WorldSummaryDTO{
			ID:                m.ID,
			Name:              m.Name,
			Description:       m.Description,
			Genre:             m.Genre,
			CompatibleSystems: compat,
		})
	}
	return summaries, nil
}

func (s *Service) CreateGame(ctx context.Context, req CreateGameRequestDTO) (*GameSummaryDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("campaign name is required")
	}
	if req.SystemID == "" {
		return nil, fmt.Errorf("system_id is required")
	}
	if req.WorldID == "" {
		return nil, fmt.Errorf("world_id is required")
	}
	if req.PlayerName == "" {
		req.PlayerName = "Adventurer"
	}

	gameID := req.ID
	if gameID == "" {
		gameID = slugify(req.Name)
	}

	session, err := engine.InitGame(s.resolver, gameID, req.SystemID, req.WorldID, req.PlayerName)
	if err != nil {
		return nil, fmt.Errorf("init game: %w", err)
	}
	_ = session.Close()

	return &GameSummaryDTO{
		ID:         gameID,
		Name:       req.Name,
		SystemID:   req.SystemID,
		WorldID:    req.WorldID,
		PlayerName: req.PlayerName,
		TurnCount:  0,
		LastPlayed: time.Now().Format(time.RFC3339),
	}, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			buf.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if buf.Len() > 0 && !strings.HasSuffix(buf.String(), "-") {
				buf.WriteRune('-')
			}
		}
	}
	res := strings.Trim(buf.String(), "-")
	if res == "" {
		return "campaign"
	}
	return res
}

