package gui

import (
	"bytes"
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
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/media/playback"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/tools"
	"github.com/darkliquid/localrpg/pkg/trace"
	"gopkg.in/yaml.v3"
)

type Service struct {
	mu            sync.RWMutex
	rootDir       string
	resolver      *core.PathResolver
	configMgr     *config.ConfigManager
	indexed       map[string]bool
	embWorkers    map[string]*storage.EmbeddingWorker
	locks         map[string]*sync.Mutex
	modelsManager *models.Manager
	// newTTSClient builds a TTS client from configuration. It is a field so a test
	// can describe a provider without a network, and nil means the real factory.
	newTTSClient func(config.TTSConfig) (media.TTSClient, error)
	// Audio playback belongs to the process so narration never depends on a
	// browser's autoplay policy. It is opened once, on first use, because most
	// requests never need it.
	playerOnce sync.Once
	player     *playback.Player
	logger     trace.Logger
	// A regeneration is detached and coalesced: the flag records that one is in
	// flight, so a player turning quickly triggers a catch-up run rather than a
	// queue of overlapping ones.
	summaryMu      sync.Mutex
	summaryPending map[string]bool
	// The audio pipeline is shared and rebuilt only when the configuration
	// object changes, so a built-in TTS model is loaded once, not per segment.
	ttsMu       sync.Mutex
	ttsConfig   *config.Config
	ttsPipeline *media.TTSPipeline
	// Background work (entity enrichment, playback warm-up, retro-summary) is
	// tracked so Close can wait for it. Untracked writers outlived a caller's
	// view of the service and raced shutdown and test cleanup.
	bgMu   sync.Mutex
	bg     sync.WaitGroup
	closed bool
	// The turn runtime is the config-derived wiring that does not change from
	// turn to turn. It is rebuilt only when the config revision or a source
	// file's mtime changes, so a hand edit still takes effect next turn.
	runtimeMu   sync.Mutex
	runtime     *turnRuntime
	runtimeKey  runtimeKey
	runtimeGame string
	// The canonical timeline is re-read only when history.jsonl changes size or
	// mtime, so serving one beat's audio does not re-parse the whole log.
	historyMu    sync.Mutex
	historyCache map[string][]engine.Turn
	historySize  map[string]int64
	historyMtime map[string]int64
	// The shared usage ledger holds spend that belongs to no campaign, plus the
	// pending rows a creation flow produces before its campaign exists.
	globalUsageMu sync.Mutex
	globalUsage   *storage.Store
}

// Config returns the configuration the service is running with, so a command can
// build shared infrastructure, such as a trace sink, from the same values.
func (s *Service) Config() *config.Config {
	return s.configMgr.Get()
}

// SetLogger attaches a trace sink to the service and to every turn it prepares.
func (s *Service) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
}

func NewService(rootDir string) *Service {
	projectMode := rootDir != "" && rootDir != "."
	userPath, _ := config.DetectConfigFile()
	localPath := filepath.Join(rootDir, "localrpg.yaml")
	if projectMode {
		userPath = filepath.Join(rootDir, "config.yaml")
	}
	mgr := config.NewConfigManagerWithPaths(userPath, localPath)
	cfg, _ := mgr.Load()

	projectRoot := ""
	if projectMode {
		projectRoot = rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)

	svc := &Service{
		rootDir:        rootDir,
		resolver:       core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache),
		configMgr:      mgr,
		indexed:        make(map[string]bool),
		embWorkers:     make(map[string]*storage.EmbeddingWorker),
		locks:          make(map[string]*sync.Mutex),
		modelsManager:  models.NewManager(dirs.Cache),
		summaryPending: make(map[string]bool),
	}
	if !projectMode {
		if warning := paths.LegacyWarning(paths.System(), cfg.Paths); warning != "" {
			trace.OrNil(svc.logger).Event("paths.legacy_relative", map[string]interface{}{"detail": warning})
		}
	}
	return svc
}

// goBackground runs fn in a goroutine that Close waits for. Work submitted after
// Close is dropped, so shutdown never starts a new write.
func (s *Service) goBackground(fn func()) {
	s.bgMu.Lock()
	if s.closed {
		s.bgMu.Unlock()
		return
	}
	s.bg.Add(1)
	s.bgMu.Unlock()

	go func() {
		defer s.bg.Done()
		fn()
	}()
}

// Close stops background work and waits for it, so no goroutine writes after a
// caller considers the service done. It is safe to call more than once.
func (s *Service) Close() {
	s.bgMu.Lock()
	s.closed = true
	s.bgMu.Unlock()
	s.bg.Wait()

	s.mu.Lock()
	workers := make([]*storage.EmbeddingWorker, 0, len(s.embWorkers))
	for _, worker := range s.embWorkers {
		workers = append(workers, worker)
	}
	s.embWorkers = make(map[string]*storage.EmbeddingWorker)
	s.mu.Unlock()
	for _, worker := range workers {
		worker.Stop()
	}

	s.globalUsageMu.Lock()
	ledger := s.globalUsage
	s.globalUsage = nil
	s.globalUsageMu.Unlock()
	if ledger != nil {
		_ = ledger.Close()
	}
}

func (s *Service) GetModelsStatus() []models.ModelStatus {
	return s.modelsManager.ListStatuses()
}

func (s *Service) DownloadModel(ctx context.Context, id string) error {
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	_, err := s.modelsManager.Download(ctx, id)
	return err
}

func (s *Service) SubscribeModelEvents() chan models.ModelStatus {
	return s.modelsManager.Subscribe()
}

func (s *Service) UnsubscribeModelEvents(ch chan models.ModelStatus) {
	s.modelsManager.Unsubscribe(ch)
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
	timeline := engine.NewTimeline(s.resolver, store, history, gameID)
	worker := s.ensureEmbeddingWorker(gameID, store)
	if worker != nil {
		timeline.SetEmbeddingWorker(worker)
	}
	_ = timeline.EnsureIndexed()
	if worker != nil {
		if ents, err := store.ListEntities(); err == nil {
			for _, eSummary := range ents {
				if ent, err := store.GetEntity(eSummary.ID); err == nil && ent != nil {
					worker.Enqueue(storage.EmbeddingItem{
						TargetType: "entity",
						TargetID:   ent.ID,
						Text:       ent.Name + " " + ent.Body,
					})
				}
			}
		}
	}
	s.goBackground(func() { s.scanAndEnrichCharacters(gameID, store) })
}

func (s *Service) ensureEmbeddingWorker(gameID string, store *storage.Store) *storage.EmbeddingWorker {
	cfg := s.Config()
	embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings)
	if err != nil || embProvider == nil {
		return nil
	}

	s.mu.Lock()
	if s.embWorkers == nil {
		s.embWorkers = make(map[string]*storage.EmbeddingWorker)
	}
	worker, ok := s.embWorkers[gameID]
	if !ok {
		worker = storage.NewEmbeddingWorker(store, embProvider, storage.EmbeddingWorkerOptions{
			BatchSize: cfg.Embeddings.BatchSize,
		})
		worker.Start()
		s.embWorkers[gameID] = worker
	}
	s.mu.Unlock()
	return worker
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

func segmentDTOs(segments []entity.TurnSegment, gameID string, turnNumber int, audioAvailable bool, resolve func(string) string, voiceFor func(string) *entity.VoiceConfig) []SegmentDTO {
	dtos := make([]SegmentDTO, 0, len(segments))
	for i, segment := range segments {
		text := resolveWikilinks(segment.Text, resolve)
		dto := SegmentDTO{
			Kind:      segment.Kind,
			Speaker:   segment.Speaker,
			SpeakerID: segment.SpeakerID,
			Text:      text,
			CheckRef:  segment.CheckRef,
			Player:    segment.Player,
			// The reading estimate is the same one the exports pace with, so the
			// app and a rendered bundle hold a line for the same length of time.
			Duration: scene.ReadingDuration(text).Seconds(),
		}
		if segment.Kind == "speech" {
			refID := segment.SpeakerID
			if refID == "" && resolve != nil {
				refID = resolve(segment.Speaker)
			}
			if refID == "" && segment.Speaker != "" {
				refID = entity.Slugify(segment.Speaker)
			}
			if refID != "" {
				dto.PortraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, refID)
			}
		}
		if audioAvailable {
			// The ref is what the synthesis pipeline uses to find a voice, so the
			// same value is used here to derive a voice-sensitive version token. A
			// changed voice or provider option changes the URL, which keeps the
			// browser from serving a clip read under the previous tuning.
			ref := segment.SpeakerID
			if ref == "" {
				ref = segment.Speaker
			}
			var voice *entity.VoiceConfig
			if voiceFor != nil {
				voice = voiceFor(ref)
			}
			key := media.ComputeAudioCacheKeyForVoice(ref, voice, segment.Text)
			dto.AudioKey = key
			dto.AudioURL = fmt.Sprintf("/api/game/%s/turn/%d/segment/%d/audio?v=%s", gameID, turnNumber, i, key[:12])
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

// turnToolCallDTOs renders a turn's provenance for a client.
func turnToolCallDTOs(records []engine.ToolCallRecord) []ToolCallDTO {
	if len(records) == 0 {
		return nil
	}
	dtos := make([]ToolCallDTO, 0, len(records))
	for _, record := range records {
		dtos = append(dtos, ToolCallDTO{Name: record.Name, ResultChars: record.ResultChars})
	}
	return dtos
}

// toolEvent maps engine tool activity onto the stream's event framing, so a
// client can render an activity line that resolves rather than a silent wait.
func toolEvent(activity engine.ToolActivity) TurnEvent {
	return TurnEvent{
		Type:        "tool",
		ToolName:    activity.Name,
		ToolStatus:  activity.Status,
		ToolSummary: activity.Summary,
	}
}

// ErrAudioUnavailable is the scene package's sentinel, kept as an alias here so// the route and its tests read unchanged and there is only one value to compare.
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

	var narratorVoice, startLocation string
	if gameManifest.Settings != nil {
		if nv, ok := gameManifest.Settings["narrator_voice"].(string); ok {
			narratorVoice = nv
		}
		if sl, ok := gameManifest.Settings[engine.StartLocationSetting].(string); ok {
			startLocation = sl
		}
	}

	var bannerURL string
	if p, _ := findAssetFile(gameDir, "banner"); p != "" {
		bannerURL = fmt.Sprintf("/api/game/%s/banner", gameID)
	} else if gameManifest.WorldID != "" {
		worldDir := s.resolver.WorldDir(gameManifest.WorldID)
		if p, _ := findAssetFile(worldDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/world/%s/banner", gameManifest.WorldID)
		}
	}

	var systemManifest *core.SystemManifest
	if sm, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(gameManifest.SystemID), "system.yaml")); err == nil {
		systemManifest = sm
	}

	return &GameStateDTO{
		GameID:   gameID,
		GameName: gameManifest.Name,
		Player: PlayerDTO{
			ID:         playerID,
			Name:       ent.Name,
			Type:       ent.Type,
			State:      stateMap,
			Appearance: ent.Appearance,
			Voice:      voiceProfileDTO(ent.Voice),
		},
		Arcs:          arcs,
		Clocks:        clocks,
		Locations:     locations,
		OpeningPrompt: engine.OpeningPrompt(gameManifest),
		NarratorVoice: narratorVoice,
		StartLocation: startLocation,
		BannerURL:     bannerURL,

		MechanicsEngagement: engine.ResolveEngagement(gameManifest, systemManifest, s.configMgr.Get()),
		Advancement:         s.computeAdvancement(gameID, systemManifest),
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
		id := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(entitiesDir, entry.Name()))
		if err != nil {
			continue
		}
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// A note that fails to parse is still a note the player wrote. Show
			// it so it can be repaired instead of silently vanishing.
			summaries = append(summaries, EntitySummaryDTO{
				ID:         id,
				Name:       id,
				ParseError: true,
			})
			continue
		}

		name := parsed.Name
		if name == "" {
			name = id
		}

		summaries = append(summaries, EntitySummaryDTO{
			ID:       id,
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
		// Return the raw note so the codex can still open and repair it, rather
		// than failing the read outright.
		return &EntityDTO{
			ID:         entityID,
			Name:       entityID,
			Markdown:   string(data),
			ParseError: true,
		}, nil
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
	ent, err := entity.ParseMarkdownEntity([]byte(rawMarkdown))
	if err != nil {
		return fmt.Errorf("save entity %q: %w", entityID, err)
	}
	// The file name is the note's identity. Normalise the frontmatter id so a
	// hand-edited or copied id can never index a note under another note's key.
	ent.ID = entityID
	normalised, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("normalise entity %q: %w", entityID, err)
	}

	gameDir := s.resolver.GameDir(gameID)
	path := filepath.Join(gameDir, "entities", entityID+".md")
	if err := os.WriteFile(path, normalised, 0644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(path)
}

// MergeEntities folds one note into another: the survivor keeps its identity and
// gains the source's prose, tags, aliases, and turn history, every note that linked
// to the source is rewritten to point at the survivor, and the source is removed.
//
// It is deliberately explicit. Deciding that two names are one being is a judgement
// the engine cannot make, but it can carry the decision out once a player makes it.
func (s *Service) MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error) {
	if sourceID == targetID {
		return nil, fmt.Errorf("cannot merge %q into itself", sourceID)
	}

	gameDir := s.resolver.GameDir(gameID)
	sourceData, err := os.ReadFile(filepath.Join(gameDir, "entities", sourceID+".md"))
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", sourceID, err)
	}
	targetPath := filepath.Join(gameDir, "entities", targetID+".md")
	targetData, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read target %q: %w", targetID, err)
	}

	source, err := entity.ParseMarkdownEntity(sourceData)
	if err != nil {
		return nil, fmt.Errorf("parse source %q: %w", sourceID, err)
	}
	target, err := entity.ParseMarkdownEntity(targetData)
	if err != nil {
		return nil, fmt.Errorf("parse target %q: %w", targetID, err)
	}

	// The survivor keeps its name and gains what the source knew.
	if body := strings.TrimSpace(source.Body); body != "" {
		target.Body = strings.TrimSpace(target.Body) + "\n\n" + body
	}
	target.Aliases = appendUnique(target.Aliases, source.Name)
	target.Aliases = appendUnique(target.Aliases, source.Aliases...)
	target.Tags = appendUnique(target.Tags, source.Tags...)
	for _, number := range source.History {
		already := false
		for _, known := range target.History {
			if known == number {
				already = true
				break
			}
		}
		if !already {
			target.History = append(target.History, number)
		}
	}

	merged, err := target.SerializeMarkdown()
	if err != nil {
		return nil, fmt.Errorf("serialize merged note: %w", err)
	}

	// Write the survivor first: a failure after this point leaves both notes rather
	// than losing the source's content.
	if err := os.WriteFile(targetPath, merged, 0644); err != nil {
		return nil, fmt.Errorf("write merged note: %w", err)
	}

	if err := s.rewriteInboundLinks(gameDir, sourceID, targetID); err != nil {
		return nil, err
	}

	if err := os.Remove(filepath.Join(gameDir, "entities", sourceID+".md")); err != nil {
		return nil, fmt.Errorf("remove source note: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if err := store.DeleteEntity(sourceID); err != nil {
		return nil, fmt.Errorf("remove source from the index: %w", err)
	}

	syncer := storage.NewSyncer(store)
	for _, id := range []string{targetID, sourceID} {
		_ = syncer.SyncFile(filepath.Join(gameDir, "entities", id+".md"))
	}

	return s.GetEntity(ctx, gameID, targetID)
}

// rewriteInboundLinks points every note that linked to the source at the survivor,
// so no note is left pointing at an entity that no longer exists.
func (s *Service) rewriteInboundLinks(gameDir, sourceID, targetID string) error {
	entitiesDir := filepath.Join(gameDir, "entities")
	entries, err := os.ReadDir(entitiesDir)
	if err != nil {
		return fmt.Errorf("read entities dir: %w", err)
	}

	pattern := regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(sourceID) + `(\s*\|[^\]]*)?\]\]`)
	replacement := "[[" + targetID + "$1]]"

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") || strings.TrimSuffix(entry.Name(), ".md") == sourceID {
			continue
		}

		path := filepath.Join(entitiesDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !pattern.Match(data) {
			continue
		}

		updated := pattern.ReplaceAll(data, []byte(replacement))
		if err := os.WriteFile(path, updated, 0644); err != nil {
			return fmt.Errorf("rewrite links in %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// appendUnique adds values that are not already present, preserving order.
func appendUnique(existing []string, values ...string) []string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		found := false
		for _, candidate := range existing {
			if strings.EqualFold(candidate, trimmed) {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, trimmed)
		}
	}
	return existing
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
		TurnNumber:      turn.Number,
		InputText:       turn.Input,
		Mode:            turn.Mode,
		Prose:           turn.Prose(),
		Outcome:         turn.Outcome,
		Truncated:       turn.Truncated,
		Recovery:        turn.Recovery,
		ToolCalls:       turnToolCallDTOs(turn.ToolCalls),
		ContextNotes:    turn.ContextNotes,
		ContinuityNotes: turn.ContinuityNotes,
		EntitiesHit:     mentionIDs(turn.Entities),
		Verdict:         turn.Verdict,
		Rejected:        turn.Rejected,
		Checks:          turn.Checks,
		PendingCheck:    turn.PendingCheck,
		Segments: segmentDTOs(turn.Segments, gameID, turn.Number, audioAvailable, func(name string) string {
			return harness.ResolveSpeakerID(store, name)
		}, func(ref string) *entity.VoiceConfig {
			return harness.ResolveSpeakerVoice(store, ref)
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
	chronicler   *engine.Chronicler
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

// ContextLimits reports the limits the session's orchestrator was built with.
func (t *TurnSession) ContextLimits() harness.ContextLimits {
	return t.orchestrator.ContextLimits()
}

// Close releases the campaign's turn lock. It is safe to call twice.
func (t *TurnSession) Close() {
	if t.release == nil {
		return
	}
	t.release()
	t.release = nil
}

// SummaryPending reports whether a regeneration is in flight for a campaign.
func (s *Service) SummaryPending(gameID string) bool {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	return s.summaryPending[gameID]
}

// summariseBehind regenerates a campaign's story so far without delaying the turn
// that triggered it. A second model call must never be something a player waits on,
// and the turn that triggers one must not use its own summary.
func (s *Service) summariseBehind(gameID string, chronicler *engine.Chronicler) {
	if chronicler == nil {
		return
	}

	due, err := chronicler.Due(gameID)
	if err != nil || !due {
		return
	}

	// One pending regeneration per campaign, never a queue: a run in flight is
	// never restarted, and the next turn triggers a catch-up pass if one is missed.
	s.summaryMu.Lock()
	if s.summaryPending[gameID] {
		s.summaryMu.Unlock()
		return
	}
	s.summaryPending[gameID] = true
	s.summaryMu.Unlock()

	s.goBackground(func() {
		defer func() {
			s.summaryMu.Lock()
			delete(s.summaryPending, gameID)
			s.summaryMu.Unlock()
		}()

		if _, err := chronicler.Regenerate(context.Background(), gameID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update the story so far: %v\n", err)
		}
	})
}

// chronicler builds a campaign's chronicler from the current configuration, for a
// caller that is not a turn. It is per call like a turn's wiring, so a settings
// change takes effect without a restart.
func (s *Service) chronicler(gameID string) *engine.Chronicler {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}

	cfg := s.configMgr.Get()
	router, err := harness.RouterFromConfigWithLogger(cfg, s.logger)
	if err != nil {
		return nil
	}

	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")), gameID)
	chronicler := engine.NewChronicler(timeline, store, harness.SummariserFromConfig(cfg, router, s.logger))
	chronicler.SetEvery(cfg.SummaryEvery())
	chronicler.SetLogger(s.logger)
	return chronicler
}

// GetRecap returns the campaign's story so far.
func (s *Service) GetRecap(ctx context.Context, gameID string) (*RecapDTO, error) {
	chronicler := s.chronicler(gameID)
	if chronicler == nil {
		return &RecapDTO{}, nil
	}

	chronicle, err := chronicler.Recap(gameID)
	if err != nil {
		return nil, fmt.Errorf("read chronicle: %w", err)
	}

	latest := 0
	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	if turns, err := engine.NewHistoryLogger(historyPath).LoadHistory(); err == nil && len(turns) > 0 {
		latest = turns[len(turns)-1].Number
	}

	threads := make([]ThreadDTO, 0)
	if open, err := engine.OpenThreads(s.storeOrNil(gameID), latest); err == nil {
		for _, thread := range open {
			threads = append(threads, ThreadDTO{
				ID:           thread.ID,
				Name:         thread.Name,
				Status:       thread.Status,
				LastAdvanced: thread.LastAdvanced,
				Idle:         thread.Idle,
			})
		}
	}

	return &RecapDTO{
		Summary:     chronicle.Summary,
		ThroughTurn: chronicle.ThroughTurn,
		Enabled:     s.configMgr.Get().SummaryEvery() > 0,
		Threads:     threads,
	}, nil
}

// prepareTurn assembles everything a turn needs. It is built per turn on purpose:
// a cached orchestrator would miss settings changes and note edits, which is a
// failure this codebase has already produced twice.
func (s *Service) prepareTurn(gameID string) (*TurnSession, error) {
	s.ensureIndexed(gameID)

	_, prepareSpan := telemetry.Tracer("github.com/darkliquid/localrpg/pkg/gui").Start(context.Background(), "turn.prepare")
	defer prepareSpan.End()

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
	logger := trace.OrNil(s.logger)
	logger.SetGame(gameID)

	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	if worker := s.ensureEmbeddingWorker(gameID, store); worker != nil {
		timeline.SetEmbeddingWorker(worker)
	}
	timeline.SetVoiceProfiles(media.FilterVoiceProfiles(cfg.Media.TTS.VoiceProfiles, media.ProviderKey(cfg.Media.TTS)))

	// A campaign written before player_name existed holds a display name in
	// player:, which is repaired once here so every later read is exact.
	playerID := manifest.Player
	if resolved, err := engine.RepairPlayerIdentity(s.resolver, store, manifest); err == nil && resolved != "" {
		playerID = resolved
	}

	runtime, err := s.runtimeFor(gameID, manifest)
	if err != nil {
		return nil, err
	}
	router := runtime.router

	// One usage context per turn stamps every provider call the turn makes with
	// the campaign and turn number, including a concurrent extraction.
	usageCtx := harness.NewUsageContext(s, gameID)
	router.SetUsageRecorder(usageCtx)

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, playerID))

	// The VM is rebuilt every turn, so its hooks must be re-registered every
	// turn. Loading once per campaign left every turn after the first running
	// with an empty mechanics VM and no hooks.
	loader := rules.NewRuleLoader(s.resolver, jsEngine)
	if err := loader.LoadRules(manifest.SystemID, manifest.WorldID); err != nil {
		logger.Event("rules.load_error", map[string]interface{}{"error": err.Error()})
	}

	startLocation := ""
	if pinned, ok := manifest.Settings[engine.StartLocationSetting].(string); ok {
		startLocation = pinned
	}

	chronicler := engine.NewChronicler(timeline, store, harness.SummariserFromConfig(cfg, router, logger))
	chronicler.SetEvery(cfg.SummaryEvery())
	chronicler.SetLogger(logger)

	orchestrator := engine.NewTurnOrchestrator(store, timeline, jsEngine, router, startLocation, playerID)
	orchestrator.SetLogger(logger)
	orchestrator.SetChronicler(chronicler)
	orchestrator.SetUsageContext(usageCtx)
	extractor := harness.ExtractorFromConfigWithLogger(cfg, router, logger)
	if extractor != nil {
		extractor.SetUsageRecorder(usageCtx)
	}
	orchestrator.SetExtractor(extractor)
	orchestrator.SetCompletionProvider(harness.CompletionFromConfig(cfg, router, logger))
	orchestrator.SetCompletionPolicy(engine.CompletionPolicy{
		Mode:        cfg.CompletionMode(),
		MaxAttempts: cfg.CompletionAttempts(),
		TailChars:   cfg.CompletionTailChars(),
		MinChars:    cfg.CompletionMinChars(),
		Timeout:     cfg.CompletionTimeout(),
	})
	toolExecutor := tools.NewExecutor(store, cfg.ToolResultChars())
	toolExecutor.SetVoiceProfiles(timeline.VoiceProfiles())
	toolExecutor.SetEntityWriter(timeline)
	if embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings); err == nil && embProvider != nil {
		toolExecutor.SetEmbeddingsProvider(embProvider)
	}
	orchestrator.SetTools(toolExecutor, cfg.RoleSupportsTools("gm"))
	orchestrator.SetToolRounds(cfg.ToolRounds())

	// Hand the engine the declared stats so it can validate a state change, and
	// the engagement policy so the mechanics instruction reflects it. All of it
	// comes from the cached runtime, which is keyed on the config revision and
	// the source mtimes.
	if runtime.declaredStats != nil {
		orchestrator.SetDeclaredStats(runtime.declaredStats)
		orchestrator.SetAllowFreeformState(runtime.allowFreeform)
	}
	if runtime.mechanics != nil {
		orchestrator.SetMechanics(runtime.mechanics)
	}
	orchestrator.SetMechanicsEngagement(runtime.engagement)
	orchestrator.SetMechanicsCadence(cfg.MechanicsCadenceTurns())
	orchestrator.SetRulesPrompt(runtime.rulesPrompt)
	orchestrator.SetLorePrompt(runtime.lorePrompt)
	orchestrator.SetMechanicsPrompt(runtime.mechanicsPrompt)
	orchestrator.SetChunkTimeout(cfg.ChunkTimeout())
	orchestrator.SetOpeningPrompt(engine.OpeningPrompt(manifest))
	orchestrator.SetContextLimits(harness.ContextLimits{
		TokenBudget:       cfg.ContextBudget(),
		RecentTurns:       cfg.RecentTurns(),
		RecentTurnChars:   cfg.RecentTurnChars(),
		SceneRecallTurns:  cfg.SceneRecallTurns(),
		SceneRecallChars:  cfg.SceneRecallChars(),
		RetrievalTurns:    cfg.RetrievalTurns(),
		RetrievalChars:    cfg.RetrievalChars(),
		RetrievalHalflife: cfg.RetrievalHalfLifeTurns(),
	})
	orchestrator.SetThreadsMax(cfg.ThreadsMax())
	orchestrator.SetContinuityChecks(cfg.ContinuityChecks())
	orchestrator.SetActionEcho(cfg.ActionEcho())

	ttsClient, _ := s.ttsClientFor(cfg.Media.TTS)
	cueCaps := media.ResolveSpeechCueCapabilities(cfg.Media.TTS, ttsClient)
	orchestrator.SetSpeechCues(harness.SpeechCueContext{
		AudioTags:        cueCaps.AudioTags,
		MarkdownEmphasis: cueCaps.MarkdownEmphasis,
		SampleTags:       cueCaps.SupportedTags,
		CustomGuidance:   cueCaps.PromptGuidance,
	})

	return &TurnSession{
		service:      s,
		gameID:       gameID,
		cfg:          cfg,
		store:        store,
		timeline:     timeline,
		orchestrator: orchestrator,
		chronicler:   chronicler,
	}, nil
}

// Run plays one turn, emitting events as they happen. An emit failure cancels the
// turn, which is how a disconnected client stops generation rather than paying for
// a turn nobody will see.
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.TurnTimeout())
	defer cancel()

	// Tool activity is streamed as it happens, so a lookup reads as progress
	// rather than as a stall. A failed emit is ignored: the turn still records,
	// and a disconnected client is handled by the chunk listener below.
	t.orchestrator.SetToolObserver(func(activity engine.ToolActivity) {
		_ = emit(toolEvent(activity))
	})

	t.orchestrator.SetPendingCheckRef(req.PendingCheckRef)
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

	// When built-in TTS is configured and model weights are missing, inform the client
	// so the user can be prompted to download the voice pack.
	if t.cfg.Media.TTS.Type == "builtin" && (t.cfg.Media.TTS.BuiltinName == "sherpa-onnx" || t.cfg.Media.TTS.BuiltinName == "kokoro") {
		status := t.service.modelsManager.Status("kokoro-tts")
		if !status.Installed {
			_ = emit(TurnEvent{
				Type:    "model_missing",
				ModelID: "kokoro-tts",
				Name:    status.Name,
				Size:    status.TotalBytes,
			})
		}
	}

	// Narration is the application's own responsibility, detached from the
	// request: the turn is already recorded - and its entities, with their voices,
	// persisted - so a slow synthesis must not hold the stream open.
	if t.cfg.Media.TTS.AutoPlay {
		t.service.goBackground(func() {
			_ = t.service.PlayTurnAudio(context.Background(), t.gameID, turn.Number)
		})
	} else if t.cfg.Media.TTS.Type != "" && t.cfg.Media.TTS.Type != "disabled" {
		t.service.goBackground(func() {
			for i := range turn.Segments {
				_, _ = t.service.GetSegmentAudio(context.Background(), t.gameID, turn.Number, i)
			}
		})
	}

	// Memory is repaired behind the turn, on the same principle as playback: the
	// reply is already recorded, so nothing about it should wait for a second call.
	t.service.summariseBehind(t.gameID, t.chronicler)
	t.service.goBackground(func() { t.service.scanAndEnrichCharacters(t.gameID, t.store) })
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
	client, err := media.NewSceneImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey, s.logger)
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

// GetCharacterPortrait returns the portrait image for a character, or a procedural SVG fallback if not found.
func (s *Service) GetCharacterPortrait(ctx context.Context, gameID, characterID string) ([]byte, string, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, "", err
	}
	ent, err := store.GetEntity(characterID)
	if err != nil || ent == nil {
		// Fallback: check entity markdown file on disk directly
		notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", characterID+".md")
		if data, readErr := os.ReadFile(notePath); readErr == nil {
			if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
				ent = parsed
			}
		}
	}
	if ent == nil {
		return nil, "", fmt.Errorf("character %q not found", characterID)
	}

	if ent.Portrait != "" {
		portraitPath := filepath.Join(s.resolver.GameDir(gameID), ent.Portrait)
		if data, err := os.ReadFile(portraitPath); err == nil && len(data) > 0 {
			return data, imageContentType(data), nil
		}
	}

	// Procedural SVG fallback
	svg := media.GenerateProceduralBustSVG(ent.ID, ent.Name, ent.Gender)
	return svg, "image/svg+xml", nil
}

// RegenerateCharacterPortrait generates a fresh portrait for a character and
// overwrites any existing one, returning a cache-busted URL.
func (s *Service) RegenerateCharacterPortrait(ctx context.Context, gameID, characterID string) (CharacterPortraitDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return CharacterPortraitDTO{}, err
	}

	ent, err := store.GetEntity(characterID)
	if err != nil || ent == nil {
		notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", characterID+".md")
		if data, readErr := os.ReadFile(notePath); readErr == nil {
			if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
				ent = parsed
			}
		}
	}
	if ent == nil {
		return CharacterPortraitDTO{}, fmt.Errorf("character %q not found", characterID)
	}
	if ent.Type != "character" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureInvalidRequest,
			Message: fmt.Sprintf("%q is not a character", characterID),
		}
	}

	cfg := s.configMgr.Get()
	if cfg.Media.Image.Type == "" || cfg.Media.Image.Type == "disabled" {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: "image generation is disabled or unconfigured",
		}
	}
	client, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return CharacterPortraitDTO{}, &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("image provider: %v", err),
			Cause:   err,
		}
	}

	provider := cfg.Media.Image.BuiltinName
	if provider == "" {
		provider = cfg.Media.Image.Type
	}
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, "portrait")
	defer span.End()

	worker := engine.NewPortraitWorker(s.resolver, store, client)
	if _, err := worker.Regenerate(ctx, gameID, ent, s.worldArtStyle(gameID)); err != nil {
		failure := &harness.GenerationFailure{
			Code:    harness.ClassifyProviderError(err),
			Message: fmt.Sprintf("generate portrait: %v", err),
			Cause:   err,
		}
		s.recordImage(ctx, span, "portrait", provider, 0, started, failure)
		return CharacterPortraitDTO{}, failure
	}
	s.recordImage(ctx, span, "portrait", provider, 0, started, nil)

	return CharacterPortraitDTO{
		PortraitURL: fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixNano()),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
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

// scanAndEnrichCharacters scans a campaign's character entities, enriching missing attributes and generating portraits.
func (s *Service) scanAndEnrichCharacters(gameID string, store *storage.Store) {
	if store == nil {
		return
	}
	worldStyle := s.worldArtStyle(gameID)
	cfg := s.Config()

	ents, err := store.ListEntities()
	if err != nil {
		return
	}

	var enricher *engine.CharacterEnricher
	if cfg != nil {
		if router, err := harness.RouterFromConfig(cfg); err == nil && router != nil {
			enricher = engine.NewCharacterEnricher(router)
		}
	}

	var portraitWorker *engine.PortraitWorker
	if cfg != nil && cfg.Media.Image.Type != "" && cfg.Media.Image.Type != "disabled" {
		if imgClient, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey); err == nil && imgClient != nil {
			portraitWorker = engine.NewPortraitWorker(s.resolver, store, imgClient)
		}
	}

	for _, eSummary := range ents {
		if eSummary.Type != "character" {
			continue
		}
		ent, err := store.GetEntity(eSummary.ID)
		if err != nil || ent == nil {
			continue
		}

		if enricher != nil && enricher.NeedsEnrichment(ent) {
			if enriched, err := enricher.Enrich(context.Background(), ent, worldStyle); err == nil && enriched != nil {
				notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", enriched.ID+".md")
				if data, err := enriched.SerializeMarkdown(); err == nil {
					_ = os.WriteFile(notePath, data, 0644)
					_ = storage.NewSyncer(store).SyncFile(notePath)
				}
				ent = enriched
			}
		}

		if portraitWorker != nil && ent.Portrait == "" {
			portraitWorker.Enqueue(gameID, ent, worldStyle)
		}
	}
}

// tracePath is where the sink appends, matching what the composition root builds.
func (s *Service) tracePath() string {
	return filepath.Join(s.resolver.CacheDir(), "trace", "trace.jsonl")
}

// TraceEvents returns the newest trace events, oldest first. Only the tail of the
// file is read: a full trace is tens of megabytes, and a viewer never needs all
// of it.
func (s *Service) TraceEvents(limit int, gameID string) ([]TraceEventDTO, error) {
	if limit <= 0 || limit > 2000 {
		limit = 200
	}

	lines, err := tailLines(s.tracePath(), limit*4)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceEventDTO{}, nil
		}
		return nil, fmt.Errorf("read trace: %w", err)
	}

	events := make([]TraceEventDTO, 0, len(lines))
	for _, line := range lines {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // a torn final line is not a failure
		}
		if gameID != "" {
			if game, ok := raw["game"].(string); ok && game != gameID {
				continue
			}
		}

		dto := TraceEventDTO{}
		if text, ok := raw["ts"].(string); ok {
			dto.Time = text
		}
		if text, ok := raw["event"].(string); ok {
			dto.Event = text
		}
		if text, ok := raw["level"].(string); ok {
			dto.Level = text
		}
		delete(raw, "ts")
		delete(raw, "event")
		delete(raw, "level")
		if len(raw) > 0 {
			dto.Fields = raw
		}
		events = append(events, dto)
	}

	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

// ClearTrace removes the trace and its rotations.
func (s *Service) ClearTrace() error {
	base := s.tracePath()
	_ = os.Remove(base)
	for index := 1; index <= 32; index++ {
		_ = os.Remove(fmt.Sprintf("%s.%d", base, index))
	}
	return nil
}

// tailLines returns up to want lines from the end of a file, oldest first. It reads
// backwards in blocks so a large trace is never loaded to show its last page.
func tailLines(path string, want int) ([]string, error) {
	if want <= 0 {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	const block = 64 * 1024
	remaining := info.Size()
	buffer := make([]byte, 0, block)
	newlines := 0

	for remaining > 0 && newlines <= want {
		readSize := int64(block)
		if remaining < readSize {
			readSize = remaining
		}
		remaining -= readSize

		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, remaining); err != nil {
			return nil, err
		}
		buffer = append(chunk, buffer...)
		newlines = bytes.Count(buffer, []byte{'\n'})
	}

	lines := strings.Split(strings.TrimRight(string(buffer), "\n"), "\n")
	if len(lines) > want {
		lines = lines[len(lines)-want:]
	}
	return lines, nil
}

// GetSegmentAudio synthesizes one segment on demand and returns the cached clip,
// reusing it for every later request.
func (s *Service) GetSegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) (string, error) {
	turns, err := s.cachedHistory(gameID)
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
	narratorVoice := s.narratorVoiceFor(gameID, cfg)

	pipeline, err := s.audioPipeline()
	if err != nil {
		return "", err
	}
	isForce := len(force) > 0 && force[0]
	path, err := pipeline.SynthesizeSegmentForce(ctx, turn.Segments[segmentIndex], narratorVoice, s.voiceFor(gameID), isForce)
	if err != nil {
		return "", err
	}
	// A cache hit reports nothing, so only a real synthesis is recorded.
	if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
		s.RecordUsage(gameID, turnNumber, "tts", harness.Usage{
			Provider:     media.ProviderKey(cfg.Media.TTS),
			Model:        cfg.Media.TTS.Model,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			Characters:   u.Characters,
			Requests:     u.Requests,
			Estimated:    u.Estimated,
		})
	}
	return path, nil
}

// audioPipeline returns the shared TTS pipeline, building it when the current
// configuration object differs from the one it was built from. Sharing it means a
// built-in TTS model is loaded once, not once per segment.
func (s *Service) audioPipeline() (*media.TTSPipeline, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return nil, ErrAudioUnavailable
	}

	s.ttsMu.Lock()
	defer s.ttsMu.Unlock()
	if s.ttsPipeline != nil && s.ttsConfig == cfg {
		return s.ttsPipeline, nil
	}

	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, fmt.Errorf("build tts client: %w", err)
	}
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())
	s.ttsConfig, s.ttsPipeline = cfg, pipeline
	return pipeline, nil
}

// narratorVoiceFor resolves the narrator voice for a campaign, preferring any
// campaign-level setting in game.yaml and falling back to media.tts.default_voice.
func (s *Service) narratorVoiceFor(gameID string, cfg *config.Config) *entity.VoiceConfig {
	voiceID := ""
	if cfg != nil {
		voiceID = cfg.Media.TTS.DefaultVoice
	}
	if manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml")); err == nil && manifest != nil && manifest.Settings != nil {
		if nv, ok := manifest.Settings["narrator_voice"].(string); ok && strings.TrimSpace(nv) != "" {
			voiceID = strings.TrimSpace(nv)
		}
	}
	res := &entity.VoiceConfig{
		VoiceID: voiceID,
	}
	if cfg != nil {
		res.Pitch = cfg.Media.TTS.Pitch
		res.SpeechRate = cfg.Media.TTS.SpeechRate
		res.Options = cfg.Media.TTS.Options
	}
	return res
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

// CountUncachedBeats reports how much of a campaign's speech is already cached,
// so a bulk synthesis can warn before spending money on a metered provider.
func (s *Service) CountUncachedBeats(gameID string) (cached, uncached int, err error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return 0, 0, fmt.Errorf("build tts client: %w", err)
	}

	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return 0, 0, fmt.Errorf("load history: %w", err)
	}

	narratorVoice := s.narratorVoiceFor(gameID, cfg)
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(s.resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())

	voiceFor := s.voiceFor(gameID)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		turnCached, turnUncached := pipeline.CountUncached(turn.Segments, narratorVoice, voiceFor)
		cached += turnCached
		uncached += turnUncached
	}
	return cached, uncached, nil
}

// findTurn reads one turn from the canonical log.
func (s *Service) findTurn(gameID string, turnNumber int) (*engine.Turn, error) {
	turns, err := s.cachedHistory(gameID)
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
func (s *Service) PlayTurnAudio(ctx context.Context, gameID string, turnNumber int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return err
	}

	isForce := len(force) > 0 && force[0]

	// Synthesize in order and feed the player as each clip lands, so playback
	// starts on the first completed clip instead of waiting for the whole turn.
	clips := make(chan string)
	go func() {
		defer close(clips)
		for i := range turn.Segments {
			path, err := s.GetSegmentAudio(ctx, gameID, turnNumber, i, isForce)
			if err != nil || path == "" {
				continue
			}
			select {
			case clips <- path:
			case <-ctx.Done():
				return
			}
		}
	}()

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	return player.PlayQueue(ctx, clips)
}

// PlaySegmentAudio plays one beat, which is what a speaker chip triggers.
func (s *Service) PlaySegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	path, err := s.GetSegmentAudio(ctx, gameID, turnNumber, segmentIndex, force...)
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
		return harness.ResolveSpeakerVoice(store, speakerID)
	}
}

// voiceProfileDTO presents an entity's voice as the config-shaped profile the
// client edits, so the sheet can show how a character sounds.
func voiceProfileDTO(voice *entity.VoiceConfig) *config.VoiceProfile {
	if voice == nil || strings.TrimSpace(voice.VoiceID) == "" {
		return nil
	}
	return &config.VoiceProfile{
		ID:         voice.VoiceID,
		Name:       voice.VoiceID,
		VoiceID:    voice.VoiceID,
		Provider:   voice.Provider,
		Pitch:      voice.Pitch,
		SpeechRate: voice.SpeechRate,
	}
}

func contentTypeForArt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "image/webp"
	}
}

func findAssetFile(dir string, name string) (string, string) {
	assetsDir := filepath.Join(dir, "assets")
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, ext := range exts {
		path := filepath.Join(assetsDir, name+ext)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, ext
		}
	}
	return "", ""
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

		var latestTime time.Time
		if fi, err := os.Stat(manifestPath); err == nil {
			latestTime = fi.ModTime()
		}
		if fi, err := os.Stat(historyPath); err == nil {
			if fi.ModTime().After(latestTime) {
				latestTime = fi.ModTime()
			}
		}

		lastPlayed := ""
		if !latestTime.IsZero() {
			lastPlayed = latestTime.Format(time.RFC3339)
		}

		name := m.Name
		if name == "" {
			name = gameID
		}

		playerName := m.PlayerName
		if playerName == "" {
			playerName = m.Player
		}

		var bannerURL, iconURL string
		if p, _ := findAssetFile(gameDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/game/%s/banner", gameID)
		}
		if p, _ := findAssetFile(gameDir, "icon"); p != "" {
			iconURL = fmt.Sprintf("/api/game/%s/icon", gameID)
		}

		summaries = append(summaries, GameSummaryDTO{
			ID:              gameID,
			Name:            name,
			SystemID:        m.SystemID,
			WorldID:         m.WorldID,
			PlayerName:      playerName,
			TurnCount:       turnCount,
			LastPlayed:      lastPlayed,
			BannerURL:       bannerURL,
			IconURL:         iconURL,
			PlayTimeSeconds: int64(turnCount * 120),
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
		var bannerURL, iconURL string
		worldDir := filepath.Join(worldsDir, e.Name())
		if p, _ := findAssetFile(worldDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/world/%s/banner", m.ID)
		}
		if p, _ := findAssetFile(worldDir, "icon"); p != "" {
			iconURL = fmt.Sprintf("/api/world/%s/icon", m.ID)
		}

		summaries = append(summaries, WorldSummaryDTO{
			ID:                m.ID,
			Name:              m.Name,
			Description:       m.Description,
			Genre:             m.Genre,
			ArtStyle:          m.ArtStyle,
			Tags:              m.Tags,
			CompatibleSystems: compat,
			BannerURL:         bannerURL,
			IconURL:           iconURL,
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

	// The system decides which prompts a character must answer, falling back to
	// the engine's defaults when it defines none.
	sysManifest, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(req.SystemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", req.SystemID, err)
	}
	fields := engine.CharacterFields(sysManifest)
	answers := map[string]string{
		"appearance": req.Player.Appearance,
		"age":        req.Player.Age,
		"gender":     req.Player.Gender,
		"pronouns":   req.Player.Pronouns,
		"background": req.Player.Background,
	}
	for key, value := range req.Player.Extra {
		answers[key] = value
	}
	// Required prompts are enforced only when the caller is doing character
	// creation. A programmatic or legacy create with no player object keeps
	// working and simply leaves the protagonist lightly described.
	playerProvided := strings.TrimSpace(req.Player.Appearance) != "" ||
		strings.TrimSpace(req.Player.Background) != "" ||
		strings.TrimSpace(req.Player.Age) != "" ||
		strings.TrimSpace(req.Player.Gender) != "" ||
		strings.TrimSpace(req.Player.Pronouns) != "" ||
		len(req.Player.Extra) > 0 ||
		req.Player.Voice != nil
	if playerProvided {
		for _, id := range engine.RequiredCharacterFields(fields) {
			if strings.TrimSpace(answers[id]) == "" {
				return nil, fmt.Errorf("character field %q is required", id)
			}
		}
	}

	var voice *entity.VoiceConfig
	if req.Player.Voice != nil && strings.TrimSpace(req.Player.Voice.VoiceID) != "" {
		voice = &entity.VoiceConfig{
			Provider:   req.Player.Voice.Provider,
			VoiceID:    req.Player.Voice.VoiceID,
			Pitch:      req.Player.Voice.Pitch,
			SpeechRate: req.Player.Voice.SpeechRate,
		}
	}

	session, err := engine.InitGame(s.resolver, engine.InitOptions{
		GameID:     gameID,
		Name:       req.Name,
		SystemID:   req.SystemID,
		WorldID:    req.WorldID,
		PlayerName: req.PlayerName,
		PlayerCharacter: engine.PlayerCharacter{
			Appearance: req.Player.Appearance,
			Age:        req.Player.Age,
			Gender:     req.Player.Gender,
			Pronouns:   req.Player.Pronouns,
			Background: req.Player.Background,
			Voice:      voice,
			Extra:      req.Player.Extra,
		},
		OpeningPrompt: req.OpeningPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("init game: %w", err)
	}
	_ = session.Close()

	// Spend the client deferred while building this campaign now belongs to it.
	// Nothing is recorded under this token unless the client asked for deferral.
	if err := s.CommitDeferredUsage(gameID, gameID); err != nil {
		trace.OrNil(s.logger).Event("usage.commit_error", map[string]interface{}{"game": gameID, "error": err.Error()})
	}

	if strings.TrimSpace(req.NarratorVoice) != "" {
		_ = s.UpdateGameSettings(ctx, gameID, map[string]interface{}{
			"narrator_voice": strings.TrimSpace(req.NarratorVoice),
		})
	}
	if strings.TrimSpace(req.StartLocation) != "" {
		_ = s.UpdateGameSettings(ctx, gameID, map[string]interface{}{
			engine.StartLocationSetting: strings.TrimSpace(req.StartLocation),
		})
	}

	// A voice the player did not choose is chosen from their description, so the
	// protagonist can speak in their own voice from the first turn.
	if voice == nil {
		_ = s.assignPlayerVoice(gameID, req.PlayerName)
	}

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

// assignPlayerVoice gives the player note a voice profile matched from its
// description when the player did not choose one. A failure is not fatal: an
// unvoiced player simply reads in the narrator voice.
func (s *Service) assignPlayerVoice(gameID, playerName string) error {
	store, err := s.store(gameID)
	if err != nil {
		return err
	}

	id := entity.Slugify(playerName)
	if id == "" {
		id = "player"
	}

	ent, err := store.GetEntity(id)
	if err != nil || ent == nil || ent.Voice != nil {
		return err
	}

	// Only voices the active provider can synthesise are assignable, so a profile
	// authored for another engine is never chosen while it is unreachable.
	mediaCfg := s.configMgr.Get().Media.TTS
	harness.AssignVoiceProfile(ent, media.FilterVoiceProfiles(mediaCfg.VoiceProfiles, media.ProviderKey(mediaCfg)))
	if ent.Voice == nil {
		return nil
	}

	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(historyPath), gameID)
	return timeline.SaveEntity(ent)
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
	var playerCharacter engine.PlayerCharacter
	var portraitRel string
	var portraitBytes []byte
	if store, err := s.store(gameID); err == nil {
		if playerID, err := engine.ResolvePlayerID(store, manifest); err == nil && playerID != "" {
			playerPath := filepath.Join(gameDir, "entities", playerID+".md")
			if data, err := os.ReadFile(playerPath); err == nil {
				if ent, err := entity.ParseMarkdownEntity(data); err == nil {
					if playerName == "" {
						playerName = ent.Name
					}
					details = strings.TrimSpace(ent.Body)
					playerCharacter = engine.PlayerCharacterFromEntity(ent)
					if ent.Portrait != "" {
						if art, err := os.ReadFile(filepath.Join(gameDir, ent.Portrait)); err == nil {
							portraitRel = ent.Portrait
							portraitBytes = art
						}
					}
				}
			}
		}
	}
	// Only carry the portrait reference forward when the file itself survived, so
	// the recreated note never points at an asset that is no longer on disk.
	if portraitRel == "" {
		playerCharacter.Portrait = ""
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
		GameID:          gameID,
		Name:            manifest.Name,
		SystemID:        manifest.SystemID,
		WorldID:         manifest.WorldID,
		PlayerName:      playerName,
		PlayerDetails:   details,
		PlayerCharacter: playerCharacter,
	})
	if err != nil {
		return nil, fmt.Errorf("recreate campaign: %w", err)
	}
	_ = session.Close()

	// The recreated campaign starts with empty assets, so put the protagonist's
	// portrait back under the path its note already references.
	if portraitRel != "" && len(portraitBytes) > 0 {
		dest := filepath.Join(gameDir, portraitRel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return nil, fmt.Errorf("restore portrait dir: %w", err)
		}
		if err := os.WriteFile(dest, portraitBytes, 0644); err != nil {
			return nil, fmt.Errorf("restore portrait: %w", err)
		}
	}

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
		ID:                m.ID,
		Name:              m.Name,
		Version:           m.Version,
		Description:       m.Description,
		Script:            script,
		RulesPrompt:       rulesPrompt,
		CharacterCreation: m.CharacterCreation,
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
		ID:                id,
		Name:              req.Name,
		Version:           req.Version,
		Description:       req.Description,
		CharacterCreation: req.CharacterCreation,
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

// ErrWorldExists reports an attempt to create a world whose id is already taken.
var ErrWorldExists = errors.New("world already exists")

// ErrWorldNotFound reports an attempt to update a world that does not exist.
var ErrWorldNotFound = errors.New("world not found")

// writeWorld writes a world directory. It never decides create vs update; the
// caller does, so a create can refuse a duplicate and an update can require a
// target.
func (s *Service) writeWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
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

	lorePath := filepath.Join(worldDir, "prompts", "lore.md")
	if req.LorePrompt != "" {
		promptDir := filepath.Join(worldDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(lorePath, []byte(req.LorePrompt), 0644); err != nil {
			return nil, fmt.Errorf("write lore.md: %w", err)
		}
	} else if err := os.Remove(lorePath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove lore.md: %w", err)
	}

	return s.GetWorld(ctx, id)
}

// CreateWorld writes a new world and refuses an id that is already taken.
func (s *Service) CreateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	id := ""
	if req.ID != "" {
		id = slugify(req.ID)
	}
	if id == "" {
		id = slugify(req.Name)
	}
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(id), "world.yaml")); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldExists, id)
	}
	req.ID = id
	return s.writeWorld(ctx, req)
}

// UpdateWorld rewrites an existing world and refuses an unknown id.
func (s *Service) UpdateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(req.ID), "world.yaml")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldNotFound, req.ID)
	}
	return s.writeWorld(ctx, req)
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

// applyResolvedPaths fills a config's path fields with the absolute directories
// in effect, so the settings surface and a saved config both name real locations.
func (s *Service) applyResolvedPaths(cfg *config.Config) {
	projectRoot := ""
	if s.rootDir != "" && s.rootDir != "." {
		projectRoot = s.rootDir
	}
	dirs := paths.Resolve(paths.System(), cfg.Paths, projectRoot)
	cfg.Paths = config.PathsConfig{Systems: dirs.Systems, Worlds: dirs.Worlds, Games: dirs.Games, Cache: dirs.Cache}
}

func (s *Service) GetSettings(ctx context.Context) (*SettingsResponseDTO, error) {
	cfg, err := s.configMgr.Load()
	if err != nil {
		return nil, err
	}
	s.applyResolvedPaths(cfg)
	return &SettingsResponseDTO{
		Config:          *cfg,
		ConfigFilePath:  s.configMgr.ActiveFilePath(),
		IsLocalOverride: s.configMgr.IsLocalOverride(),
	}, nil
}

func (s *Service) SaveSettings(ctx context.Context, cfg config.Config) (*SettingsResponseDTO, error) {
	if err := s.validateVoiceOptionsInConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validate tts options: %w", err)
	}
	s.applyResolvedPaths(&cfg)
	if err := s.configMgr.Save(&cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	s.mu.Lock()
	s.resolver.SetPaths(cfg.Paths.Systems, cfg.Paths.Worlds, cfg.Paths.Games, cfg.Paths.Cache)
	s.mu.Unlock()

	_ = os.MkdirAll(cfg.Paths.Systems, 0755)
	_ = os.MkdirAll(cfg.Paths.Worlds, 0755)
	_ = os.MkdirAll(cfg.Paths.Games, 0755)
	_ = os.MkdirAll(cfg.Paths.Cache, 0755)

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
		// The editor never sends the shared key, so resolve it here the same way
		// the router does. A role-level override still wins inside the factory.
		if cfg := s.configMgr.Get(); cfg != nil {
			agentCfg.SharedAPIKey = cfg.Providers.Gemini.APIKey
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
		if ttsCfg.Type == "builtin" && (ttsCfg.BuiltinName == "sherpa-onnx" || ttsCfg.BuiltinName == "kokoro") {
			if ttsCfg.ModelPath == "" && s.modelsManager != nil {
				ttsCfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
			}
			if s.modelsManager != nil {
				status := s.modelsManager.Status("kokoro-tts")
				if !status.Installed {
					return &TestProviderResponseDTO{
						Success:      false,
						ModelMissing: true,
						ModelID:      "kokoro-tts",
						Message:      "Kokoro voice pack is not installed; download required",
					}, nil
				}
			}
		}
		cfg := s.configMgr.Get()
		client, err := media.NewTTSClientWithSharedKey(ttsCfg, cfg.Providers.Gemini.APIKey)
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = defaultTTSPreviewText
		}
		spoken := media.SpeakableTextFor(media.TextPolicyFromConfig(ttsCfg), client, prompt)
		if strings.TrimSpace(spoken) == "" {
			return &TestProviderResponseDTO{Success: false, Message: "The test phrase reduced to no speakable text"}, nil
		}
		voiceID := ttsCfg.DefaultVoice
		if req.VoiceID != "" {
			voiceID = req.VoiceID
		}
		voice := &entity.VoiceConfig{
			VoiceID:    voiceID,
			Pitch:      ttsCfg.Pitch,
			SpeechRate: ttsCfg.SpeechRate,
			Options:    ttsCfg.Options,
		}
		audio, err := client.Synthesize(ctx, spoken, voice)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   err.Error(),
				Failure:   &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: err.Error(), Cause: err},
			}, nil
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
		if reporter, ok := client.(media.UsageReporter); ok {
			provider := sttCfg.BuiltinName
			if provider == "" {
				provider = sttCfg.Type
			}
			s.RecordUsageGlobal("stt", mediaUsage(reporter.LastUsage(), provider, sttCfg.Model))
		}
		if err != nil {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   err.Error(),
				Failure:   &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: err.Error(), Cause: err},
			}, nil
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
		cfg := s.configMgr.Get()
		client, err := media.NewImageClientWithSharedKey(imgCfg, cfg.Providers.Gemini.APIKey)
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
		return "", &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: "STT engine is disabled or unconfigured"}
	}

	client, err := media.NewSTTClient(cfg.Media.STT)
	if err != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("initialize STT client: %v", err),
			Cause:   err,
		}
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.stt.request", map[string]interface{}{
		"provider": cfg.Media.STT.Type,
		"bytes":    len(audioData),
	})

	text, err := media.NewSTTProvider(client).TranscribeAudio(ctx, audioData)
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

func (s *Service) GetGameAsset(gameID, assetKind string) (string, string, error) {
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", "", os.ErrNotExist
	}
	p, ext := findAssetFile(gameDir, assetKind)
	if p == "" {
		return "", "", os.ErrNotExist
	}
	return p, contentTypeForArt(ext), nil
}

func (s *Service) GetWorldAsset(worldID, assetKind string) (string, string, error) {
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(worldDir); err != nil {
		return "", "", os.ErrNotExist
	}
	p, ext := findAssetFile(worldDir, assetKind)
	if p == "" {
		return "", "", os.ErrNotExist
	}
	return p, contentTypeForArt(ext), nil
}

func (s *Service) SaveGameAsset(gameID, assetKind string, data []byte, ext string) (string, error) {
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", fmt.Errorf("game not found: %w", err)
	}
	assetsDir := filepath.Join(gameDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, e := range exts {
		_ = os.Remove(filepath.Join(assetsDir, assetKind+e))
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath := filepath.Join(assetsDir, assetKind+ext)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("write asset: %w", err)
	}
	return fmt.Sprintf("/api/game/%s/%s", gameID, assetKind), nil
}

func (s *Service) SaveWorldAsset(worldID, assetKind string, data []byte, ext string) (string, error) {
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(worldDir); err != nil {
		return "", fmt.Errorf("world not found: %w", err)
	}
	assetsDir := filepath.Join(worldDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, e := range exts {
		_ = os.Remove(filepath.Join(assetsDir, assetKind+e))
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath := filepath.Join(assetsDir, assetKind+ext)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("write asset: %w", err)
	}
	return fmt.Sprintf("/api/world/%s/%s", worldID, assetKind), nil
}

type GenerateAssetRequestDTO struct {
	Kind   string `json:"kind"`
	Prompt string `json:"prompt,omitempty"`
}

// buildAssetPrompt renders the image prompt shared by game, world, and preview
// generation. A non-empty genre means world context; otherwise this is a
// campaign, where description carries the world name.
func buildAssetPrompt(kind, name, description, artStyle, genre string) string {
	if genre == "" {
		if kind == "icon" {
			return fmt.Sprintf("%s game app icon emblem for %s in %s, high contrast vector emblem, centered dark backdrop", artStyle, name, description)
		}
		return fmt.Sprintf("%s widescreen cinematic concept art landscape for %s in %s, highly detailed masterpiece environment", artStyle, name, description)
	}
	if kind == "icon" {
		return fmt.Sprintf("%s emblem icon badge for world %s (%s), %s, clean centered icon", artStyle, name, genre, description)
	}
	return fmt.Sprintf("%s widescreen landscape banner concept art for world %s (%s), %s, atmospheric panoramic background", artStyle, name, genre, description)
}

// GenerateAssetPreviewRequestDTO carries inline form metadata for a stateless
// preview render. Nothing is persisted.
type GenerateAssetPreviewRequestDTO struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ArtStyle    string `json:"art_style"`
	Genre       string `json:"genre,omitempty"`
}

// guardImageBytes rejects a provider that returned success with no image data,
// so a zero-byte file is never written or served as .webp/octet-stream.
func guardImageBytes(imgBytes []byte) ([]byte, *harness.GenerationFailure) {
	if len(imgBytes) == 0 {
		return nil, &harness.GenerationFailure{
			Code:    harness.FailureProviderError,
			Message: "image provider returned no data",
		}
	}
	return imgBytes, nil
}

// GenerateAssetPreview renders banner or icon image bytes in memory and returns
// them with a MIME type detected from the bytes.
func (s *Service) GenerateAssetPreview(ctx context.Context, req GenerateAssetPreviewRequestDTO) ([]byte, string, error) {
	imgBytes, failure := s.generateImage(ctx, req.Kind, buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre), "")
	if failure != nil {
		return nil, "", failure
	}
	return imgBytes, imageContentType(imgBytes), nil
}

func (s *Service) GenerateGameAsset(ctx context.Context, gameID string, req GenerateAssetRequestDTO) (string, error) {
	prompt := req.Prompt
	if prompt == "" {
		gameDir := s.resolver.GameDir(gameID)
		manifest, _ := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
		gameName := gameID
		worldName := ""
		artStyle := ""
		if manifest != nil {
			if manifest.Name != "" {
				gameName = manifest.Name
			}
			worldDir := s.resolver.WorldDir(manifest.WorldID)
			if wm, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml")); err == nil {
				worldName = wm.Name
				artStyle = wm.ArtStyle
			}
		}
		prompt = buildAssetPrompt(req.Kind, gameName, worldName, artStyle, "")
	}
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, gameID)
	if failure != nil {
		return "", failure
	}
	return s.SaveGameAsset(gameID, req.Kind, imgBytes, media.ArtExtension(imgBytes))
}

func (s *Service) GenerateWorldAsset(ctx context.Context, worldID string, req GenerateAssetRequestDTO) (string, error) {
	prompt := req.Prompt
	if prompt == "" {
		worldDir := s.resolver.WorldDir(worldID)
		wm, _ := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
		worldName := worldID
		artStyle := ""
		genre := ""
		desc := ""
		if wm != nil {
			if wm.Name != "" {
				worldName = wm.Name
			}
			artStyle = wm.ArtStyle
			genre = wm.Genre
			desc = wm.Description
		}
		prompt = buildAssetPrompt(req.Kind, worldName, desc, artStyle, genre)
	}
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, "")
	if failure != nil {
		return "", failure
	}
	return s.SaveWorldAsset(worldID, req.Kind, imgBytes, media.ArtExtension(imgBytes))
}

// GetTurnContext returns the most recent turn's context snapshot and prompt for a campaign.
func (s *Service) GetTurnContext(gameID string) (*TurnContextDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	turns, err := store.ListTurns(1, 0)
	if err != nil || len(turns) == 0 {
		return nil, fmt.Errorf("no turns recorded for game %q", gameID)
	}
	lastTurn := turns[0].Number
	raw, prompt, err := store.GetTurnContext(lastTurn)
	if err != nil {
		return nil, fmt.Errorf("get turn context: %w", err)
	}
	var turnCtx harness.TurnContext
	if err := json.Unmarshal(raw, &turnCtx); err != nil {
		return nil, fmt.Errorf("decode turn context: %w", err)
	}
	return toTurnContextDTO(&turnCtx, prompt), nil
}

// GetWorkingSet returns the active continuity working set for a campaign.
func (s *Service) GetWorkingSet(gameID string) ([]WorkingEntryDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	records, err := store.LoadWorkingSet()
	if err != nil {
		return nil, fmt.Errorf("load working set: %w", err)
	}
	entries := make([]WorkingEntryDTO, 0, len(records))
	for _, rec := range records {
		name := rec.EntityID
		if ent, err := store.GetEntity(rec.EntityID); err == nil && ent != nil && ent.Name != "" {
			name = ent.Name
		}
		entries = append(entries, WorkingEntryDTO{
			Kind:     rec.Kind,
			ID:       rec.EntityID,
			Name:     name,
			Weight:   rec.Weight,
			LastTurn: rec.LastTurn,
			Role:     rec.Role,
		})
	}
	return entries, nil
}

func toTurnContextDTO(tc *harness.TurnContext, prompt string) *TurnContextDTO {
	if tc == nil {
		return nil
	}
	sections := make([]SectionReportDTO, 0, len(tc.Sections))
	for _, s := range tc.Sections {
		refs := make([]RefDTO, 0, len(s.Refs))
		for _, r := range s.Refs {
			refs = append(refs, RefDTO{Kind: string(r.Kind), ID: r.ID, Relation: r.Relation})
		}
		sections = append(sections, SectionReportDTO{
			Name:     s.Name,
			Tokens:   s.Tokens,
			Included: s.Included,
			Source:   s.Source,
			Refs:     refs,
		})
	}
	refs := make([]RefDTO, 0, len(tc.Refs))
	for _, r := range tc.Refs {
		refs = append(refs, RefDTO{Kind: string(r.Kind), ID: r.ID, Relation: r.Relation})
	}
	ws := make([]RefDTO, 0, len(tc.WorkingSet))
	for _, r := range tc.WorkingSet {
		ws = append(ws, RefDTO{Kind: string(r.Kind), ID: r.ID, Relation: r.Relation})
	}
	var sess *ProviderSessionDTO
	if tc.Session != nil {
		sess = &ProviderSessionDTO{
			Provider:    tc.Session.Provider,
			ID:          tc.Session.ID,
			ThroughTurn: tc.Session.ThroughTurn,
			Model:       tc.Session.Model,
			PrefixHash:  tc.Session.PrefixHash,
		}
	}
	return &TurnContextDTO{
		TurnNumber:      tc.TurnNumber,
		Mode:            tc.Mode,
		Budget:          tc.Budget,
		EstimatedTokens: tc.EstimatedTokens,
		Sections:        sections,
		Refs:            refs,
		WorkingSet:      ws,
		Threads:         tc.Threads,
		SummaryVersion:  tc.SummaryVersion,
		WorldHash:       tc.WorldHash,
		SystemHash:      tc.SystemHash,
		PromptHash:      tc.PromptHash,
		Strategy:        string(tc.Strategy),
		PrefixHash:      tc.PrefixHash,
		Session:         sess,
		CachedTokens:    tc.CachedTokens,
		Prompt:          prompt,
	}
}


// ListEntityMemories returns an entity's memories newest-first for the codex
// timeline.
func (s *Service) ListEntityMemories(gameID, entityID string, limit int) ([]MemoryDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	memories, err := store.ListMemoriesForEntity(entityID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryDTO, 0, len(memories))
	for _, memory := range memories {
		out = append(out, MemoryDTO{
			Turn:       memory.Turn,
			Kind:       memory.Kind,
			Text:       memory.Text,
			Importance: memory.Importance,
			Tags:       memory.Tags,
		})
	}
	return out, nil
}
