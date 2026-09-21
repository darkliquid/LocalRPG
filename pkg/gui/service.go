package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

type Service struct {
	mu        sync.RWMutex
	rootDir   string
	resolver  *core.PathResolver
	configMgr *config.ConfigManager
	indexed   map[string]bool
}

func NewService(rootDir string) *Service {
	configDir := os.Getenv("LOCALRPG_CONFIG_DIR")
	if configDir == "" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			configDir = filepath.Join(xdg, "localrpg")
		} else {
			userHome, _ := os.UserHomeDir()
			configDir = filepath.Join(userHome, ".config", "localrpg")
		}
	}
	userPath := filepath.Join(configDir, "config.yaml")
	localPath := filepath.Join(rootDir, "localrpg.yaml")
	if rootDir != "" && rootDir != "." {
		userPath = filepath.Join(rootDir, "config.yaml")
	}
	mgr := config.NewConfigManagerWithPaths(userPath, localPath)
	cfg, _ := mgr.Load()

	sysDir := cfg.Paths.Systems
	worldDir := cfg.Paths.Worlds
	gameDir := cfg.Paths.Games
	cacheDir := cfg.Paths.Cache
	if !filepath.IsAbs(sysDir) && rootDir != "" {
		sysDir = filepath.Join(rootDir, sysDir)
	}
	if !filepath.IsAbs(worldDir) && rootDir != "" {
		worldDir = filepath.Join(rootDir, worldDir)
	}
	if !filepath.IsAbs(gameDir) && rootDir != "" {
		gameDir = filepath.Join(rootDir, gameDir)
	}
	if !filepath.IsAbs(cacheDir) && rootDir != "" {
		cacheDir = filepath.Join(rootDir, cacheDir)
	}

	return &Service{
		rootDir:   rootDir,
		resolver:  core.NewCustomPathResolver(sysDir, worldDir, gameDir, cacheDir),
		configMgr: mgr,
		indexed:   make(map[string]bool),
	}
}

func (s *Service) GetResolver() *core.PathResolver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resolver
}

// store returns the campaign's canonical index. The store is pooled, so callers
// must not close it.
func (s *Service) store(gameID string) (*storage.Store, error) {
	return storage.OpenGameStore(s.resolver, gameID)
}

// ensureIndexed repairs the campaign index the first time this process serves it,
// so timeline queries answer from the database rather than re-reading files.
func (s *Service) ensureIndexed(gameID string) {
	s.mu.Lock()
	if s.indexed[gameID] {
		s.mu.Unlock()
		return
	}
	s.indexed[gameID] = true
	s.mu.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return
	}

	_, _ = storage.NewSyncer(store).Sync(filepath.Join(gameDir, "entities"))
	history := engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl"))
	_ = engine.NewTimeline(s.resolver, store, history, gameID).EnsureIndexed()
}

// GetEntityTurns returns the turns an entity took part in.
func (s *Service) GetEntityTurns(ctx context.Context, gameID, entityID string) ([]TurnDTO, error) {
	s.ensureIndexed(gameID)

	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	numbers, err := store.ListTurnsForEntity(entityID)
	if err != nil {
		return nil, err
	}
	if len(numbers) == 0 {
		return []TurnDTO{}, nil
	}

	wanted := make(map[int]bool, len(numbers))
	for _, number := range numbers {
		wanted[number] = true
	}

	all, err := s.GetChronicle(ctx, gameID)
	if err != nil {
		return nil, err
	}

	turns := make([]TurnDTO, 0, len(numbers))
	for _, turn := range all {
		if wanted[turn.TurnNumber] {
			turns = append(turns, turn)
		}
	}
	return turns, nil
}

func mentionIDs(mentions []entity.Mention) []string {
	ids := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		ids = append(ids, mention.ID)
	}
	return ids
}

func segmentDTOs(segments []entity.TurnSegment) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for _, segment := range segments {
		dtos = append(dtos, SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      segment.Text,
		})
	}
	return dtos
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

	var backlinks []string
	if store, err := s.store(gameID); err == nil {
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
		History:   ent.History,
	}, nil
}

func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	gameDir := s.resolver.GameDir(gameID)
	path := filepath.Join(gameDir, "entities", entityID+".md")
	if err := os.WriteFile(path, []byte(rawMarkdown), 0644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

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
			TurnNumber:  turn.Number,
			InputText:   turn.Input,
			Mode:        turn.Mode,
			Prose:       turn.Prose(),
			AudioURL:    audioURL,
			EntitiesHit: mentionIDs(turn.Entities),
			Segments:    segmentDTOs(turn.Segments),
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
	id := entity.Slugify(s)
	if id == "" {
		return "campaign"
	}
	return id
}

const defaultMechanicsScript = `// LocalRPG Rule System Engine
// Globals available: roll(notation), state, log(msg)

function evaluateRoll(stats, diceExpr) {
  const result = roll(diceExpr || "2d6");
  return {
    total: result.total,
    success: result.total >= 10,
    rolls: result.rolls
  };
}
`

func (s *Service) GetSystem(ctx context.Context, id string) (*SystemDetailDTO, error) {
	sysDir := s.resolver.SystemDir(id)
	m, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system manifest: %w", err)
	}

	script := ""
	if data, err := os.ReadFile(filepath.Join(sysDir, "mechanics.js")); err == nil {
		script = string(data)
	}

	rulesPrompt := ""
	if data, err := os.ReadFile(filepath.Join(sysDir, "prompts", "rules.md")); err == nil {
		rulesPrompt = string(data)
	}

	return &SystemDetailDTO{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Script:      script,
		RulesPrompt: rulesPrompt,
	}, nil
}

func (s *Service) SaveSystem(ctx context.Context, req CreateSystemRequestDTO) (*SystemDetailDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("system name is required")
	}
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}
	script := req.Script
	if script == "" {
		script = defaultMechanicsScript
	}

	sysDir := s.resolver.SystemDir(id)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		return nil, fmt.Errorf("create system dir: %w", err)
	}

	manifest := core.SystemManifest{
		ID:          id,
		Name:        req.Name,
		Version:     req.Version,
		Description: req.Description,
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal system manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write system.yaml: %w", err)
	}

	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("write mechanics.js: %w", err)
	}

	if req.RulesPrompt != "" {
		promptDir := filepath.Join(sysDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create system prompts dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "rules.md"), []byte(req.RulesPrompt), 0644); err != nil {
			return nil, fmt.Errorf("write rules.md: %w", err)
		}
	}

	return s.GetSystem(ctx, id)
}

func (s *Service) GetWorld(ctx context.Context, id string) (*WorldDetailDTO, error) {
	worldDir := s.resolver.WorldDir(id)
	m, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world manifest: %w", err)
	}

	entitiesDir := filepath.Join(worldDir, "entities")
	var entities []WorldEntitySummaryDTO
	if entries, err := os.ReadDir(entitiesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			entID := strings.TrimSuffix(e.Name(), ".md")
			data, err := os.ReadFile(filepath.Join(entitiesDir, e.Name()))
			if err != nil {
				continue
			}
			ent, err := entity.ParseMarkdownEntity(data)
			name := entID
			entType := "concept"
			if err == nil {
				if ent.Name != "" {
					name = ent.Name
				}
				if ent.Type != "" {
					entType = ent.Type
				}
			}
			entities = append(entities, WorldEntitySummaryDTO{
				ID:   entID,
				Name: name,
				Type: entType,
			})
		}
	}

	lorePrompt := ""
	if data, err := os.ReadFile(filepath.Join(worldDir, "prompts", "lore.md")); err == nil {
		lorePrompt = string(data)
	}

	return &WorldDetailDTO{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		Genre:         m.Genre,
		DefaultSystem: m.DefaultSystem,
		ArtStyle:      m.ArtStyle,
		Tags:          m.Tags,
		LorePrompt:    lorePrompt,
		Entities:      entities,
	}, nil
}

func (s *Service) SaveWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("world name is required")
	}
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}

	worldDir := s.resolver.WorldDir(id)
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		return nil, fmt.Errorf("create world entities dir: %w", err)
	}

	manifest := core.WorldManifest{
		ID:            id,
		Name:          req.Name,
		Description:   req.Description,
		Genre:         req.Genre,
		DefaultSystem: req.DefaultSystem,
		ArtStyle:      req.ArtStyle,
		Tags:          req.Tags,
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal world manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write world.yaml: %w", err)
	}

	if req.LorePrompt != "" {
		promptDir := filepath.Join(worldDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "lore.md"), []byte(req.LorePrompt), 0644); err != nil {
			return nil, fmt.Errorf("write lore.md: %w", err)
		}
	}

	return s.GetWorld(ctx, id)
}

func (s *Service) GetWorldEntity(ctx context.Context, worldID, entityID string) (*WorldEntityDetailDTO, error) {
	path := filepath.Join(s.resolver.WorldDir(worldID), "entities", entityID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world entity %s: %w", entityID, err)
	}
	return &WorldEntityDetailDTO{
		ID:       entityID,
		Markdown: string(data),
	}, nil
}

func (s *Service) SaveWorldEntity(ctx context.Context, worldID, entityID, markdown string) error {
	dir := filepath.Join(s.resolver.WorldDir(worldID), "entities")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create world entities dir: %w", err)
	}
	path := filepath.Join(dir, entityID+".md")
	return os.WriteFile(path, []byte(markdown), 0644)
}

func (s *Service) DeleteWorldEntity(ctx context.Context, worldID, entityID string) error {
	path := filepath.Join(s.resolver.WorldDir(worldID), "entities", entityID+".md")
	return os.Remove(path)
}

func (s *Service) GetSettings(ctx context.Context) (*SettingsResponseDTO, error) {
	cfg, err := s.configMgr.Load()
	if err != nil {
		return nil, err
	}
	return &SettingsResponseDTO{
		Config:          *cfg,
		ConfigFilePath:  s.configMgr.ActiveFilePath(),
		IsLocalOverride: s.configMgr.IsLocalOverride(),
	}, nil
}

func (s *Service) SaveSettings(ctx context.Context, cfg config.Config) (*SettingsResponseDTO, error) {
	if err := s.configMgr.Save(&cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	sysDir := cfg.Paths.Systems
	worldDir := cfg.Paths.Worlds
	gameDir := cfg.Paths.Games
	cacheDir := cfg.Paths.Cache

	if !filepath.IsAbs(sysDir) && s.rootDir != "" {
		sysDir = filepath.Join(s.rootDir, sysDir)
	}
	if !filepath.IsAbs(worldDir) && s.rootDir != "" {
		worldDir = filepath.Join(s.rootDir, worldDir)
	}
	if !filepath.IsAbs(gameDir) && s.rootDir != "" {
		gameDir = filepath.Join(s.rootDir, gameDir)
	}
	if !filepath.IsAbs(cacheDir) && s.rootDir != "" {
		cacheDir = filepath.Join(s.rootDir, cacheDir)
	}

	s.mu.Lock()
	s.resolver.SetPaths(sysDir, worldDir, gameDir, cacheDir)
	s.mu.Unlock()

	_ = os.MkdirAll(sysDir, 0755)
	_ = os.MkdirAll(worldDir, 0755)
	_ = os.MkdirAll(gameDir, 0755)
	_ = os.MkdirAll(cacheDir, 0755)

	return &SettingsResponseDTO{
		Config:          cfg,
		ConfigFilePath:  s.configMgr.ActiveFilePath(),
		IsLocalOverride: s.configMgr.IsLocalOverride(),
	}, nil
}

func (s *Service) TestProvider(ctx context.Context, req TestProviderRequestDTO) (*TestProviderResponseDTO, error) {
	start := time.Now()

	data, err := json.Marshal(req.Provider)
	if err != nil {
		return &TestProviderResponseDTO{
			Success: false,
			Message: fmt.Sprintf("invalid provider data: %v", err),
		}, nil
	}

	switch req.Category {
	case "llm":
		var agentCfg harness.ProviderConfig
		if err := json.Unmarshal(data, &agentCfg); err != nil {
			return &TestProviderResponseDTO{
				Success: false,
				Message: fmt.Sprintf("invalid llm config: %v", err),
			}, nil
		}
		p, err := harness.NewModelProvider("test", agentCfg)
		if err != nil {
			return &TestProviderResponseDTO{
				Success: false,
				Message: fmt.Sprintf("failed to create llm provider: %v", err),
			}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = "ping"
		}
		resp, err := p.Generate(ctx, harness.GenerateRequest{Prompt: prompt})
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   fmt.Sprintf("generate failed: %v", err),
			}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   "LLM provider responded successfully",
			Preview:   resp.Text,
		}, nil

	case "tts":
		var ttsCfg config.TTSConfig
		if err := json.Unmarshal(data, &ttsCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		client, err := media.NewTTSClient(ttsCfg)
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = "Test utterance"
		}
		voice := &entity.VoiceConfig{
			VoiceID:    ttsCfg.DefaultVoice,
			Pitch:      ttsCfg.Pitch,
			SpeechRate: ttsCfg.SpeechRate,
		}
		audio, err := client.Synthesize(ctx, prompt, voice)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{Success: false, LatencyMS: latency, Message: err.Error()}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   fmt.Sprintf("Synthesized %d bytes of audio successfully", len(audio)),
		}, nil

	case "stt":
		var sttCfg config.STTConfig
		if err := json.Unmarshal(data, &sttCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		client, err := media.NewSTTClient(sttCfg)
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		text, err := client.Transcribe(ctx, []byte("fake-audio-header"))
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{Success: false, LatencyMS: latency, Message: err.Error()}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   "Transcribed audio successfully",
			Preview:   text,
		}, nil

	case "image":
		var imgCfg config.ImageConfig
		if err := json.Unmarshal(data, &imgCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		client, err := media.NewImageClient(imgCfg)
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = "a dark forest path"
		}
		imgBytes, err := client.GenerateImage(ctx, prompt)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{Success: false, LatencyMS: latency, Message: err.Error()}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   fmt.Sprintf("Generated %d bytes of image data successfully", len(imgBytes)),
		}, nil

	default:
		return &TestProviderResponseDTO{
			Success: false,
			Message: fmt.Sprintf("unsupported test category: %s", req.Category),
		}, nil
	}
}
