// The turn lifecycle: preparing a turn, recording it, resolving its checks and summarising it.

package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/tools"
	"github.com/darkliquid/localrpg/pkg/trace"
)

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
		if key, hasKey := embeddings.KeyFor(cfg.Embeddings); hasKey {
			model := cfg.Embeddings.Model
			worker.SetUsageReporting(func(u storage.EmbeddingUsage) {
				s.RecordEmbeddingUsage(string(key), model, u.InputTokens, u.Characters, u.Requests)
			}, storage.EmbeddingUsage{ProviderKey: string(key), Model: model})
		}
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

// share it so a live turn and a replayed one are the same shape, which is what
// lets the client render both with one code path.
func (s *Service) turnDTO(turn engine.Turn, store *storage.Store, cfg *config.Config, gameID, engagement string) TurnDTO {
	plan := s.clipPlanFor(cfg, gameID, turn.Segments)

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
		Rejected:        turn.Rejected,
		Checks:          turn.Checks,
		RecordReport:    recordReportDTO(turn.RecordReport),
		PendingCheck:    s.pendingCheckDTO(turn.PendingCheck, store),
		ContinuationOf:  turn.ContinuationOf,
		HealthEffects:   healthEffectDTOs(turn.HealthEffects),
		WorldTick:       turn.WorldTick,
		ClipGroups:      plan.groups,
		SceneBreak:      turn.SceneBreak,
		Engagement:      engagement,
		Segments: segmentDTOs(turn.Segments, gameID, plan, func(name string) string {
			return harness.ResolveSpeakerID(store, name)
		}, func(charID string) bool {
			return s.hasCustomPortrait(gameID, charID)
		}),
	}

	scenesDir := filepath.Join(s.resolver.GameDir(gameID), "assets", "scenes")
	for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
		p := filepath.Join(scenesDir, fmt.Sprintf("turn-%d%s", turn.Number, ext))
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Size() > 0 {
			dto.ImageURL = fmt.Sprintf("/api/game/%s/turn/%d/scene-image", gameID, turn.Number)
			break
		}
	}

	if turn.Location != "" {
		dto.LocationID = turn.Location
		if store != nil {
			if location, err := store.GetEntity(turn.Location); err == nil && location != nil {
				dto.LocationName = location.Name
			}
		}
	}

	return dto
}

// pendingCheckDTO maps a GM-proposed check for the roll card, computing the
// notation and the bonuses that would apply so the player can see the arithmetic
// before rolling.
func (s *Service) pendingCheckDTO(p *harness.PendingCheck, store *storage.Store) *PendingCheckDTO {
	if p == nil {
		return nil
	}
	dto := &PendingCheckDTO{
		Ref:        p.Ref,
		ProposedBy: p.ProposedBy,
		Request:    p.Request,
		Notation:   p.Request.Notation,
	}
	if store == nil || p.Request.Actor == "" {
		return dto
	}
	actor, err := store.GetEntity(p.Request.Actor)
	if err != nil || actor == nil {
		return dto
	}
	readState := func(name string) (int, bool) {
		if actor.State == nil || name == "" {
			return 0, false
		}
		raw, ok := actor.State.Get(name)
		if !ok {
			return 0, false
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		}
		return 0, false
	}
	if _, applied := rules.SumBonuses(readState, p.Request); len(applied) > 0 {
		dto.Bonuses = applied
	}
	values := map[string]int{}
	for _, name := range []string{p.Request.Stat, p.Request.Skill} {
		if value, ok := readState(name); ok {
			values[name] = value
		}
	}
	if len(values) > 0 {
		dto.ActorValues = values
	}
	return dto
}

// engagementFor resolves the campaign's mechanics policy for the API, so a turn
// DTO can say why mechanics ran or did not. An unreadable campaign yields "".
func (s *Service) engagementFor(gameID string) string {
	manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return ""
	}
	systemManifest, _ := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(manifest.SystemID), "system.yaml"))
	return engine.ResolveEngagement(manifest, systemManifest, s.configMgr.Get())
}

// ErrTurnInFlight means another turn is already running for this campaign.
var ErrTurnInFlight = errors.New("a turn is already in flight")

// ErrCampaignNotPlayable means the campaign's files are not ready for a turn, so
// the caller can answer before any bytes are sent.
var ErrCampaignNotPlayable = errors.New("campaign cannot be prepared")

func (s *Service) BeginTurn(gameID string) (*TurnSession, error) {
	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return nil, ErrTurnInFlight
	}
	release := lock.Unlock

	// Refuse a turn while the GM's provider is backed off, before taking any work.
	if err := s.guardRole("gm"); err != nil {
		release()
		return nil, err
	}

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

// ErrNoPendingCheck means the turn carries no pending check to resolve.
var ErrNoPendingCheck = errors.New("the turn has no pending check")

// ErrPendingCheckMismatch means the pending ref does not match the turn's check.
var ErrPendingCheckMismatch = errors.New("the pending check ref does not match the turn")

// BeginResolveCheck validates the turn's pending check and acquires the campaign
// turn lock, so a handler can choose a status code before streaming. It returns
// the session and the pending ref to resolve.
func (s *Service) BeginResolveCheck(gameID string, turnNumber int, req ResolveCheckRequestDTO) (*TurnSession, string, error) {
	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return nil, "", err
	}
	found := false
	var pending *harness.PendingCheck
	for i := range turns {
		if turns[i].Number == turnNumber {
			found = true
			pending = turns[i].PendingCheck
			break
		}
	}
	if !found {
		return nil, "", fmt.Errorf("%w: turn %d", fs.ErrNotExist, turnNumber)
	}
	if pending == nil {
		return nil, "", ErrNoPendingCheck
	}
	if req.PendingRef != "" && req.PendingRef != pending.Ref {
		return nil, "", ErrPendingCheckMismatch
	}
	session, err := s.BeginTurn(gameID)
	if err != nil {
		return nil, "", err
	}
	return session, pending.Ref, nil
}

// ResolveCheck rolls and resolves a pending check, streaming the GM's
// adjudication as a continuation turn without a fresh player action.
func (s *Service) ResolveCheck(ctx context.Context, gameID string, turnNumber int, req ResolveCheckRequestDTO, emit func(TurnEvent) error) error {
	session, ref, err := s.BeginResolveCheck(gameID, turnNumber, req)
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(ctx, TurnRequest{
		Mode:            "roll",
		Input:           req.Note,
		PendingCheckRef: ref,
		ForcedTotal:     req.ManualResult,
		ManualDice:      req.ManualDice,
	}, emit)
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
	router, err := routerWithChains(cfg, s.logger)
	if err != nil {
		return nil
	}

	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")), gameID)
	chronicler := engine.NewChronicler(timeline, store, harness.SummariserFromConfig(cfg, router, s.logger))
	chronicler.SetEvery(cfg.SummaryEvery())
	chronicler.SetLogger(s.logger)
	return chronicler
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
		logger.Event("rules.load_error", map[string]any{"error": err.Error()})
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
		Mode:              cfg.CompletionMode(),
		MaxAttempts:       cfg.CompletionAttempts(),
		MaxRepairAttempts: cfg.CompletionMaxRepairAttempts(),
		TailChars:         cfg.CompletionTailChars(),
		MinChars:          cfg.CompletionMinChars(),
		Timeout:           cfg.CompletionTimeout(),
	})

	toolExecutor := tools.NewExecutor(store, cfg.ToolResultChars())
	toolExecutor.SetVoiceProfiles(timeline.VoiceProfiles())
	toolExecutor.SetEntityWriter(timeline)
	if embProvider, err := embeddings.NewProviderFromConfig(cfg.Embeddings); err == nil && embProvider != nil {
		toolExecutor.SetEmbeddingsProvider(embProvider)
		if key, hasKey := embeddings.KeyFor(cfg.Embeddings); hasKey {
			toolExecutor.SetEmbeddingUsage(s.RecordEmbeddingUsage, string(key), cfg.Embeddings.Model)
		}
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
	orchestrator.SetMechanicsEngagement(runtime.engagement)
	if runtime.mechanics != nil {
		orchestrator.SetMechanics(runtime.mechanics)
		orchestrator.SetHealthSpec(runtime.mechanics.Health)
		// The resolution instruction is rebuilt each turn from the player's
		// current stats, so it names values the player actually has.
		orchestrator.SetMechanicsSchema(runtime.mechanics, runtime.engagement)
	} else {
		orchestrator.SetMechanicsPrompt(runtime.mechanicsPrompt)
	}
	orchestrator.SetMechanicsCadence(cfg.MechanicsCadenceTurns())
	orchestrator.SetWorldTickTurns(cfg.MechanicsWorldTickTurns())
	orchestrator.SetRulesPrompt(runtime.rulesPrompt)
	orchestrator.SetLorePrompt(runtime.lorePrompt)
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

	if cfg.Media.Image.Type != "" && cfg.Media.Image.Type != "disabled" {
		if imgClient, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey); err == nil && imgClient != nil {
			portraitWorker := engine.NewPortraitWorker(s.resolver, store, imgClient)
			portraitWorker.SetOnReady(func(gID, charID, relPath string) {
				s.broadcastPortraitReady(gID, charID, relPath)
			})
			orchestrator.SetPortraitWorker(portraitWorker)

			sceneWorker := engine.NewSceneWorker(s.resolver, imgClient)
			sceneWorker.SetOnReady(func(gID string, turnNum int, relPath string) {
				s.broadcastSceneImageReady(gID, turnNum, relPath)
			})
			// Without this the worker plans every scene against a zero budget and an
			// empty approval mode, so the campaign's image allowance is never
			// enforced and its spend is never attributed.
			s.configureSceneBudget(sceneWorker, gameID, cfg)
			orchestrator.SetSceneWorker(sceneWorker)
		}
	}
	orchestrator.SetWorldArtStyle(s.worldArtStyle(gameID))

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

// logParityMismatch traces streamed clip keys that the turn's clip plan does not
// contain, which means the streamer and the plan folded the same text
// differently. It is diagnostic only: it never changes what plays.
func (s *Service) logParityMismatch(gameID string, segments []entity.TurnSegment, streamed map[string]bool) {
	if len(streamed) == 0 {
		return
	}
	plan := s.clipPlanFor(s.configMgr.Get(), gameID, segments)
	known := map[string]bool{}
	for _, keys := range plan.segmentKeys {
		for _, key := range keys {
			known[key] = true
		}
	}
	for _, key := range plan.groupKey {
		if key != "" {
			known[key] = true
		}
	}
	var missing []string
	for key := range streamed {
		if !known[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		trace.OrNil(s.logger).Event("turn.audio_parity_mismatch", map[string]any{"keys": missing})
	}
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
