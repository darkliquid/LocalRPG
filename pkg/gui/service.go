package gui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/media/playback"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
	"gopkg.in/yaml.v3"
)

type Service struct {
	mu        sync.RWMutex
	rootDir   string
	resolver  *core.PathResolver
	configMgr *config.ConfigManager
	indexed   map[string]bool
	locks     map[string]*sync.Mutex
	// Audio playback belongs to the process so narration never depends on a
	// browser's autoplay policy. It is opened once, on first use, because most
	// requests never need it.
	playerOnce sync.Once
	player     *playback.Player
	logger     trace.Logger
}

// SetLogger attaches a trace sink to the service and to every turn it prepares.
func (s *Service) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
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
		locks:     make(map[string]*sync.Mutex),
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

// wikilinkPattern matches [[Target]] and [[Target|Label]].
var wikilinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// resolveWikilinks rewrites a note's links so the target is an entity ID the
// client can open, keeping the author's display label. A link whose target is
// unknown degrades to plain text, because a button that goes nowhere is worse
// than no button at all.
func resolveWikilinks(text string, resolve func(string) string) string {
	if resolve == nil || !strings.Contains(text, "[[") {
		return text
	}
	return wikilinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		groups := wikilinkPattern.FindStringSubmatch(match)
		target := strings.TrimSpace(groups[1])
		label := strings.TrimSpace(groups[2])
		if label == "" {
			label = target
		}
		if id := resolve(target); id != "" {
			return "[[" + id + "|" + label + "]]"
		}
		return label
	})
}

func segmentDTOs(segments []entity.TurnSegment, gameID string, turnNumber int, audioAvailable bool, resolve func(string) string) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for i, segment := range segments {
		text := resolveWikilinks(segment.Text, resolve)
		dto := SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      text,
			// The reading estimate is the same one the exports pace with, so the
			// app and a rendered bundle hold a line for the same length of time.
			Duration: scene.ReadingDuration(text).Seconds(),
		}
		if audioAvailable {
			dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio", gameID, turnNumber, i)
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

// ErrAudioUnavailable is the scene package's sentinel, kept as an alias here so
// the route and its tests read unchanged and there is only one value to compare.
var ErrAudioUnavailable = scene.ErrAudioUnavailable

// storeOrNil opens a campaign's index, returning nil rather than an error so a
// caller that can fall back does not have to branch on the error value.
func (s *Service) storeOrNil(gameID string) *storage.Store {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}
	return store
}

func (s *Service) GetGameState(ctx context.Context, gameID string) (*GameStateDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	manifestPath := filepath.Join(gameDir, "game.yaml")
	gameManifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}

	playerID, err := engine.ResolvePlayerID(s.storeOrNil(gameID), gameManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve player: %w", err)
	}
	if playerID == "" {
		return nil, fmt.Errorf("campaign %q has no player note", gameID)
	}
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

	arcs, clocks, locations := s.gameCorpus(gameID)

	return &GameStateDTO{
		GameID:   gameID,
		GameName: gameManifest.Name,
		Player: PlayerDTO{
			ID:    playerID,
			Name:  ent.Name,
			Type:  ent.Type,
			State: stateMap,
		},
		Arcs:          arcs,
		Clocks:        clocks,
		Locations:     locations,
		OpeningPrompt: engine.OpeningPrompt(gameManifest),
	}, nil
}

// gameCorpus reads the campaign's living world from the index: the locations it
// can visit, the arcs running in the background, and any faction clocks. It is
// derived rather than stored, so a note edited by hand appears immediately.
func (s *Service) gameCorpus(gameID string) ([]NarrativeArcDTO, []FactionClockDTO, []string) {
	arcs := make([]NarrativeArcDTO, 0)
	clocks := make([]FactionClockDTO, 0)
	locations := make([]string, 0)

	store := s.storeOrNil(gameID)
	if store == nil {
		return arcs, clocks, locations
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return arcs, clocks, locations
	}

	for _, summary := range summaries {
		switch summary.Type {
		case "location":
			locations = append(locations, summary.Name)
		case "arc":
			raw := s.entityState(store, summary.ID)
			progress, maxProgress := arcProgress(raw)
			arcs = append(arcs, NarrativeArcDTO{
				ID:          summary.ID,
				Name:        summary.Name,
				Progress:    progress,
				MaxProgress: maxProgress,
				Status:      stateString(raw, "status"),
			})
		case "clock", "faction":
			raw := s.entityState(store, summary.ID)
			ticks, maxTicks := clockTicks(raw)
			clocks = append(clocks, FactionClockDTO{
				Faction:  summary.Name,
				Name:     summary.Name,
				Ticks:    ticks,
				MaxTicks: maxTicks,
			})
		}
	}

	return arcs, clocks, locations
}

func (s *Service) entityState(store *storage.Store, entityID string) map[string]interface{} {
	entity, err := store.GetEntity(entityID)
	if err != nil || entity == nil || entity.State == nil {
		return nil
	}
	return entity.State.Raw()
}

// arcProgress reads an arc's clock. A note may express it as `progress: 3/6` or
// as separate `clock_ticks`/`clock_max` fields, and both are honoured.
func arcProgress(raw map[string]interface{}) (int, int) {
	if ticks, maxTicks, ok := stateFraction(raw, "progress"); ok {
		return ticks, maxTicks
	}
	ticks := stateNumber(raw, "clock_ticks", "ticks", "progress")
	maxTicks := stateNumber(raw, "clock_max", "max_ticks", "max", "max_progress")
	return ticks, maxInt(maxTicks, 1)
}

func clockTicks(raw map[string]interface{}) (int, int) {
	ticks := stateNumber(raw, "clock_ticks", "ticks")
	maxTicks := stateNumber(raw, "clock_max", "max_ticks", "max")
	return ticks, maxInt(maxTicks, 1)
}

// stateFraction reads an "a/b" style value.
func stateFraction(raw map[string]interface{}, key string) (int, int, bool) {
	value, ok := raw[key]
	if !ok {
		return 0, 0, false
	}
	text, ok := value.(string)
	if !ok {
		return 0, 0, false
	}
	parts := strings.SplitN(text, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	ticks, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	maxTicks, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	return ticks, maxInt(maxTicks, 1), true
}

// stateNumber coerces the first present key to an int. YAML and JSON disagree on
// numeric types, so every plausible one is accepted.
func stateNumber(raw map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case string:
			text := typed
			if index := strings.Index(text, "/"); index > 0 {
				text = text[:index]
			}
			if parsed, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func stateString(raw map[string]interface{}, key string) string {
	if raw == nil {
		return ""
	}
	if value, ok := raw[key].(string); ok {
		return value
	}
	return ""
}

func maxInt(value, floor int) int {
	if value < floor {
		return floor
	}
	return value
}

// ListEntities returns every note in a campaign, ordered by name, which is what
// the codex browser draws. It reads the notes themselves rather than the index so
// a note that has not been synced yet still appears, and so the browser can never
// disagree with the graph about what exists.
func (s *Service) ListEntities(ctx context.Context, gameID string) ([]EntitySummaryDTO, error) {
	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")
	entries, err := os.ReadDir(entitiesDir)
	if err != nil {
		return nil, fmt.Errorf("read entities dir: %w", err)
	}

	summaries := make([]EntitySummaryDTO, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(entitiesDir, entry.Name()))
		if err != nil {
			continue
		}
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			continue
		}

		name := parsed.Name
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), ".md")
		}

		summaries = append(summaries, EntitySummaryDTO{
			ID:       strings.TrimSuffix(entry.Name(), ".md"),
			Name:     name,
			Type:     parsed.Type,
			Location: parsed.Location,
			Tags:     parsed.Tags,
		})
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})
	return summaries, nil
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

	cfg := s.configMgr.Get()
	store, err := s.store(gameID)
	if err != nil {
		store = nil // location names and art URLs are decoration, not prerequisites
	}

	dtos := make([]TurnDTO, len(turns))
	for i, turn := range turns {
		dtos[i] = s.turnDTO(turn, store, cfg, gameID)
	}
	return dtos, nil
}

// turnDTO maps a persisted turn for the API. GetChronicle and the turn endpoint
// share it so a live turn and a replayed one are the same shape, which is what
// lets the client render both with one code path.
func (s *Service) turnDTO(turn engine.Turn, store *storage.Store, cfg *config.Config, gameID string) TurnDTO {
	audioAvailable := cfg.Media.TTS.Type != "" && cfg.Media.TTS.Type != "disabled"
	artAvailable := cfg.Media.Image.BuiltinFallback || cfg.Media.Image.Type != "disabled"

	dto := TurnDTO{
		TurnNumber:   turn.Number,
		InputText:    turn.Input,
		Mode:         turn.Mode,
		Prose:        turn.Prose(),
		Outcome:      turn.Outcome,
		Truncated:    turn.Truncated,
		ContextNotes: turn.ContextNotes,
		EntitiesHit:  mentionIDs(turn.Entities),
		Segments: segmentDTOs(turn.Segments, gameID, turn.Number, audioAvailable, func(name string) string {
			return harness.ResolveSpeakerID(store, name)
		}),
	}

	if turn.Location != "" {
		dto.LocationID = turn.Location
		if store != nil {
			if location, err := store.GetEntity(turn.Location); err == nil && location != nil {
				dto.LocationName = location.Name
			}
		}
		if artAvailable {
			dto.LocationArtURL = "/api/game/" + gameID + "/location/" + turn.Location + "/art"
		}
	}

	return dto
}

// ErrTurnInFlight means another turn is already running for this campaign.
var ErrTurnInFlight = errors.New("a turn is already in flight")

// ErrCampaignNotPlayable means the campaign's files are not ready for a turn, so
// the caller can answer before any bytes are sent.
var ErrCampaignNotPlayable = errors.New("campaign cannot be prepared")

// gameLock returns the campaign's turn lock, creating it on first use.
func (s *Service) gameLock(gameID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locks == nil {
		s.locks = make(map[string]*sync.Mutex)
	}
	if lock, ok := s.locks[gameID]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	s.locks[gameID] = lock
	return lock
}

// TurnSession is one prepared turn: the campaign's timeline and orchestrator,
// wired exactly as the CLI wires them, holding the campaign's turn lock until
// Close. Preparing up front is what lets the caller answer 409 or 503 as a status
// code rather than as an event after streaming has begun.
type TurnSession struct {
	service      *Service
	gameID       string
	cfg          *config.Config
	store        *storage.Store
	timeline     *engine.Timeline
	orchestrator *engine.TurnOrchestrator
	release      func()
}

func (s *Service) BeginTurn(gameID string) (*TurnSession, error) {
	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return nil, ErrTurnInFlight
	}
	release := lock.Unlock

	session, err := s.prepareTurn(gameID)
	if err != nil {
		release()
		// Both the sentinel and the cause are wrapped, so the route can tell an
		// unknown game (404) from a campaign that exists but cannot be played (503).
		return nil, fmt.Errorf("%w: %w", ErrCampaignNotPlayable, err)
	}

	session.release = release
	return session, nil
}

// Close releases the campaign's turn lock. It is safe to call twice.
func (t *TurnSession) Close() {
	if t.release == nil {
		return
	}
	t.release()
	t.release = nil
}

// prepareTurn assembles everything a turn needs. It is built per turn on purpose:
// a cached orchestrator would miss settings changes and note edits, which is a
// failure this codebase has already produced twice.
func (s *Service) prepareTurn(gameID string) (*TurnSession, error) {
	s.ensureIndexed(gameID)

	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load game manifest: %w", err)
	}
	if _, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(manifest.SystemID), "system.yaml")); err != nil {
		return nil, fmt.Errorf("load system %q: %v", manifest.SystemID, err)
	}
	if _, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml")); err != nil {
		return nil, fmt.Errorf("load world %q: %v", manifest.WorldID, err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	cfg := s.configMgr.Get()
	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	timeline.SetVoiceProfiles(cfg.Media.TTS.VoiceProfiles)

	// A campaign written before player_name existed holds a display name in
	// player:, which is repaired once here so every later read is exact.
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(s.resolver, store, manifest); err == nil && resolved != "" {
		playerID = resolved
	}

	router, err := harness.RouterFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, playerID))

	startLocation := ""
	if pinned, ok := manifest.Settings[engine.StartLocationSetting].(string); ok {
		startLocation = pinned
	}

	orchestrator := engine.NewTurnOrchestrator(store, timeline, jsEngine, router, startLocation, playerID)
	orchestrator.SetExtractor(harness.ExtractorFromConfig(cfg, router))
	orchestrator.LoadPrompts(s.resolver, manifest.SystemID, manifest.WorldID)
	orchestrator.SetChunkTimeout(cfg.ChunkTimeout())
	orchestrator.SetOpeningPrompt(engine.OpeningPrompt(manifest))
	orchestrator.SetContextLimits(harness.ContextLimits{
		TokenBudget:     cfg.ContextBudget(),
		RecentTurns:     cfg.RecentTurns(),
		RecentTurnChars: cfg.RecentTurnChars(),
	})

	return &TurnSession{
		service:      s,
		gameID:       gameID,
		cfg:          cfg,
		store:        store,
		timeline:     timeline,
		orchestrator: orchestrator,
	}, nil
}

// Run plays one turn, emitting events as they happen. An emit failure cancels the
// turn, which is how a disconnected client stops generation rather than paying for
// a turn nobody will see.
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.TurnTimeout())
	defer cancel()

	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
		return emit(TurnEvent{Type: "chunk", Text: text})
	})
	if err != nil {
		return err
	}

	dto := t.service.turnDTO(*turn, t.store, t.cfg, t.gameID)
	if err := emit(TurnEvent{Type: "turn", Turn: &dto}); err != nil {
		return err
	}

	// Narration is the application's own responsibility, detached from the
	// request: the turn is already recorded, and a slow synthesis must not hold
	// the stream open.
	if t.cfg.Media.TTS.AutoPlay {
		go func() {
			_ = t.service.PlayTurnAudio(context.Background(), t.gameID, turn.Number)
		}()
	}
	return nil
}

// GetLocationArt returns a location's scene image and its content type, drawing it
// on first request and reusing it until the appearance changes.
func (s *Service) GetLocationArt(ctx context.Context, gameID, locationID string, force bool) (string, string, error) {
	// Read the note from disk rather than the index: appearance, tags, and state are
	// authored content, and the index is derived from them.
	notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", locationID+".md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		return "", "", fmt.Errorf("location %q not found: %w", locationID, err)
	}

	location, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return "", "", fmt.Errorf("parse location %q: %w", locationID, err)
	}

	cfg := s.configMgr.Get()
	client, err := media.NewSceneImageClient(cfg.Media.Image)
	if err != nil {
		return "", "", fmt.Errorf("build image client: %w", err)
	}

	worldStyle := s.worldArtStyle(gameID)
	providerParams := cfg.Media.Image.Type + ":" + cfg.Media.Image.Model
	store := media.NewArtStore(client, media.NewContentCache(s.resolver.CacheDir()), worldStyle, providerParams)

	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.image.request", map[string]interface{}{
		"location": locationID,
		"provider": providerParams,
		"force":    force,
	})

	path, err := store.SceneArt(ctx, location, force)
	if err != nil {
		s.logger.Event("provider.error", map[string]interface{}{"role": "image", "error": err.Error()})
		return "", "", err
	}

	if data, err := os.ReadFile(path); err == nil {
		s.logger.Event("media.image.result", map[string]interface{}{
			"bytes":       len(data),
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}

	return path, contentTypeForArt(path), nil
}

// worldArtStyle reads the art style and genre of the campaign's world, which keeps a
// setting's imagery visually consistent.
func (s *Service) worldArtStyle(gameID string) string {
	manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return ""
	}

	world, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
}

// GetSegmentAudio synthesizes one segment on demand and returns the cached clip,
// reusing it for every later request.
func (s *Service) GetSegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int) (string, error) {
	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	turns, err := engine.NewHistoryLogger(historyPath).LoadHistory()
	if err != nil {
		return "", fmt.Errorf("load history: %w", err)
	}

	var turn *engine.Turn
	for i := range turns {
		if turns[i].Number == turnNumber {
			turn = &turns[i]
			break
		}
	}
	if turn == nil {
		return "", fmt.Errorf("turn %d not found", turnNumber)
	}
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return "", fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turnNumber)
	}

	cfg := s.configMgr.Get()
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return "", ErrAudioUnavailable
	}

	client, err := media.NewTTSClient(cfg.Media.TTS)
	if err != nil {
		return "", fmt.Errorf("build tts client: %w", err)
	}

	narratorVoice := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
	}

	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	return pipeline.SynthesizeSegment(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID))
}

// audioPlayer opens the process-wide player on first use. A host with no audio
// device leaves it nil, and callers fall back to client-side playback.
func (s *Service) audioPlayer() *playback.Player {
	s.playerOnce.Do(func() {
		player, err := playback.Open(s.configMgr.Get().Media.TTS.MasterVolume)
		if err != nil {
			return
		}
		s.player = player
	})
	return s.player
}

// AudioAvailable reports whether this process can play audio itself, which is
// what decides between application playback and a browser audio element.
func (s *Service) AudioAvailable() bool {
	return s.audioPlayer().Available()
}

// AudioPlaying reports whether a narration queue is running.
func (s *Service) AudioPlaying() bool {
	player := s.audioPlayer()
	if player == nil {
		return false
	}
	return player.Playing()
}

// StopAudio cancels the current narration queue.
func (s *Service) StopAudio() {
	if player := s.audioPlayer(); player != nil {
		player.Stop()
	}
}

// findTurn reads one turn from the canonical log.
func (s *Service) findTurn(gameID string, turnNumber int) (*engine.Turn, error) {
	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	turns, err := engine.NewHistoryLogger(historyPath).LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	for i := range turns {
		if turns[i].Number == turnNumber {
			return &turns[i], nil
		}
	}
	return nil, fmt.Errorf("turn %d not found", turnNumber)
}

// PlayTurnAudio synthesizes any beat the turn has not already cached and plays
// the whole turn in order. Clips are content-addressed, so a replay is instant.
func (s *Service) PlayTurnAudio(ctx context.Context, gameID string, turnNumber int) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return err
	}

	paths := make([]string, 0, len(turn.Segments))
	for i := range turn.Segments {
		path, err := s.GetSegmentAudio(ctx, gameID, turnNumber, i)
		if err != nil {
			// A beat that cannot be synthesized is skipped so one failure does
			// not silence the rest of the turn.
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return scene.ErrAudioUnavailable
	}

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	return player.PlayFiles(paths)
}

// PlaySegmentAudio plays one beat, which is what a speaker chip triggers.
func (s *Service) PlaySegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	path, err := s.GetSegmentAudio(ctx, gameID, turnNumber, segmentIndex)
	if err != nil {
		return err
	}

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	return player.PlayFiles([]string{path})
}

// voiceFor resolves a speaker entity's configured voice, if it has one.
func (s *Service) voiceFor(gameID string) func(speakerID string) *entity.VoiceConfig {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}

	return func(speakerID string) *entity.VoiceConfig {
		if speakerID == "" {
			return nil
		}
		ent, err := store.GetEntity(speakerID)
		if err != nil || ent == nil {
			return nil
		}
		return ent.Voice
	}
}

func contentTypeForArt(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return "image/svg+xml"
	}
	return "image/webp"
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

		playerName := m.PlayerName
		if playerName == "" {
			playerName = m.Player
		}

		summaries = append(summaries, GameSummaryDTO{
			ID:         gameID,
			Name:       name,
			SystemID:   m.SystemID,
			WorldID:    m.WorldID,
			PlayerName: playerName,
			TurnCount:  turnCount,
			LastPlayed: lastPlayed,
		})
	}

	// Most recently played first, so "Resume" is the campaign the player left.
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].LastPlayed > summaries[j].LastPlayed
	})
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

	session, err := engine.InitGame(s.resolver, engine.InitOptions{
		GameID:        gameID,
		Name:          req.Name,
		SystemID:      req.SystemID,
		WorldID:       req.WorldID,
		PlayerName:    req.PlayerName,
		OpeningPrompt: req.OpeningPrompt,
	})
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

// UpdateGameSettings merges a patch into a campaign's settings and writes the
// manifest. The caller supplies whole values; nothing is inferred or coerced.
func (s *Service) UpdateGameSettings(ctx context.Context, gameID string, patch map[string]interface{}) error {
	path := filepath.Join(s.resolver.GameDir(gameID), "game.yaml")
	manifest, err := core.LoadGameManifest(path)
	if err != nil {
		return fmt.Errorf("load game manifest: %w", err)
	}
	if manifest.Settings == nil {
		manifest.Settings = map[string]interface{}{}
	}
	for key, value := range patch {
		manifest.Settings[key] = value
	}
	if err := core.SaveGameManifest(path, manifest); err != nil {
		return fmt.Errorf("save game settings: %w", err)
	}
	return nil
}

// DeleteGame removes a campaign and everything it generated. The turn lock is
// taken first so a turn in flight finishes or is refused rather than writing into
// a directory that is being deleted.
func (s *Service) DeleteGame(ctx context.Context, gameID string) error {
	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return ErrTurnInFlight
	}
	defer lock.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(filepath.Join(gameDir, "game.yaml")); err != nil {
		return fmt.Errorf("campaign %q: %w", gameID, err)
	}

	if err := storage.CloseGameStore(s.resolver, gameID); err != nil {
		return err
	}
	if err := os.RemoveAll(gameDir); err != nil {
		return fmt.Errorf("remove campaign: %w", err)
	}
	s.forgetGame(gameID)
	return nil
}

// RestartGame returns a campaign to its opening state: history, the derived index,
// and every entity created during play are discarded, while the campaign's
// identity, system, world, protagonist, opening prompt, and pinned start location
// are carried across.
func (s *Service) RestartGame(ctx context.Context, gameID string) (*GameSummaryDTO, error) {
	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return nil, ErrTurnInFlight
	}
	defer lock.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load game manifest: %w", err)
	}

	playerName := manifest.PlayerName
	details := ""
	if store, err := s.store(gameID); err == nil {
		if playerID, err := engine.ResolvePlayerID(store, manifest); err == nil && playerID != "" {
			if data, err := os.ReadFile(filepath.Join(gameDir, "entities", playerID+".md")); err == nil {
				if ent, err := entity.ParseMarkdownEntity(data); err == nil {
					if playerName == "" {
						playerName = ent.Name
					}
					details = strings.TrimSpace(ent.Body)
				}
			}
		}
	}

	startLocation := ""
	if pinned, ok := manifest.Settings[engine.StartLocationSetting].(string); ok {
		startLocation = pinned
	}
	openingPrompt := engine.OpeningPrompt(manifest)

	if err := storage.CloseGameStore(s.resolver, gameID); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(gameDir); err != nil {
		return nil, fmt.Errorf("remove campaign: %w", err)
	}
	s.forgetGame(gameID)

	if strings.TrimSpace(playerName) == "" {
		playerName = "Adventurer"
	}

	session, err := engine.InitGame(s.resolver, engine.InitOptions{
		GameID:        gameID,
		Name:          manifest.Name,
		SystemID:      manifest.SystemID,
		WorldID:       manifest.WorldID,
		PlayerName:    playerName,
		PlayerDetails: details,
	})
	if err != nil {
		return nil, fmt.Errorf("recreate campaign: %w", err)
	}
	_ = session.Close()

	settings := map[string]interface{}{}
	if startLocation != "" {
		settings[engine.StartLocationSetting] = startLocation
	}
	if openingPrompt != "" {
		settings[engine.OpeningPromptSetting] = openingPrompt
	}
	if len(settings) > 0 {
		if err := s.UpdateGameSettings(ctx, gameID, settings); err != nil {
			return nil, err
		}
	}

	name := manifest.Name
	if name == "" {
		name = gameID
	}

	return &GameSummaryDTO{
		ID:         gameID,
		Name:       name,
		SystemID:   manifest.SystemID,
		WorldID:    manifest.WorldID,
		PlayerName: playerName,
		TurnCount:  0,
		LastPlayed: time.Now().Format(time.RFC3339),
	}, nil
}

// forgetGame clears the one-time index repair marker so a recreated campaign is
// indexed again rather than trusting the deleted database.
func (s *Service) forgetGame(gameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.indexed, gameID)
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

// defaultTTSPreviewText is long enough to expose cadence, pitch, and pacing
// differences between voice profiles during a probe.
const defaultTTSPreviewText = "Local RPG can use a wide range of voices to bring life to your characters, NPCs, and story narration."

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
			prompt = defaultTTSPreviewText
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
		if len(audio) == 0 {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   "Synthesis returned no audio; check the provider endpoint and voice ID",
			}, nil
		}
		return &TestProviderResponseDTO{
			Success:      true,
			LatencyMS:    latency,
			Message:      fmt.Sprintf("Synthesized %d bytes of audio successfully", len(audio)),
			AudioDataURI: fmt.Sprintf("data:%s;base64,%s", media.AudioContentType(audio), base64.StdEncoding.EncodeToString(audio)),
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
		text, err := client.Transcribe(ctx, media.GenerateToneWAV(440, 0.1))
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

// TranscribeAudio transcribes recorded audio data using the configured STT client.
func (s *Service) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.STT.Type == "" || cfg.Media.STT.Type == "disabled" {
		return "", fmt.Errorf("STT engine is disabled or unconfigured")
	}

	client, err := media.NewSTTClient(cfg.Media.STT)
	if err != nil {
		return "", fmt.Errorf("initialize STT client: %w", err)
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.stt.request", map[string]interface{}{
		"provider": cfg.Media.STT.Type,
		"bytes":    len(audioData),
	})

	text, err := client.Transcribe(ctx, audioData)
	if err != nil {
		s.logger.Event("provider.error", map[string]interface{}{"role": "stt", "error": err.Error()})
		return "", err
	}

	s.logger.Event("media.stt.result", map[string]interface{}{
		"chars":       len([]rune(text)),
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return text, nil
}
