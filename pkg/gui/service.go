package gui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/media/playback"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/registry"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/sysgen"
	"github.com/darkliquid/localrpg/pkg/systemtest"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/tools"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/ttsbatch"
	"github.com/darkliquid/localrpg/pkg/turnstream"
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
	// stylePackWarnings carries the reason the configured style pack was ignored,
	// so the settings can explain it without failing a render.
	stylePackWarnings []string
	// audioSubs are the clients watching for a playback completion, so the
	// theatre advances on a real event rather than a status poll. audioTurn and
	// audioSegment name the beat currently playing, so a completion event carries
	// its identity and a stale event cannot advance the wrong beat.
	audioSubMu   sync.Mutex
	audioSubs    map[chan AudioStatusDTO]struct{}
	audioTurn    int
	audioSegment int
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
	// Named media registries are built lazily from the current config and
	// invalidated when settings change, so a named provider's model loads once.
	ttsRegMu   sync.Mutex
	ttsReg     *media.TTSRegistry
	sttRegMu   sync.Mutex
	sttReg     *media.STTRegistry
	imageRegMu sync.Mutex
	imageReg   *media.ImageRegistry
	// Background work (entity enrichment, playback warm-up, retro-summary) is
	// tracked so Close can wait for it. Untracked writers outlived a caller's
	// view of the service and raced shutdown and test cleanup.
	bgMu   sync.Mutex
	bg     sync.WaitGroup
	closed bool
	// bgCtx is cancelled by Close, so long-lived background work (a batch poll
	// that can wait hours) stops instead of holding shutdown open. A batch job is
	// resumable, so cancelling it loses nothing.
	bgCtx    context.Context
	bgCancel context.CancelFunc
	// batchStartMu serialises starting a backfill, so a double-click cannot
	// submit two jobs for the same work.
	batchStartMu sync.Mutex
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
	// limits holds in-memory provider rate-limit blocks and funds failures, so a
	// 429 backs a provider off rather than being retried blindly.
	limits *harness.LimitRegistry
	// exports serialises story exports per campaign and fans progress to the
	// settings/theater UI.
	exports *exportManager
	// directoryPicker is the desktop window's native directory chooser. It is
	// nil in browser/socket mode, where the UI falls back to a path field.
	directoryPicker func(title, defaultDir string) (string, error)
	// directoryChoice holds the one native folder dialog that may be open, so a
	// blocking modal never sits inside a request the webview is waiting on.
	directoryChoice directoryChoice
	// saveFilePicker is the desktop window's native save file chooser. It is
	// nil in browser/socket mode, where the UI falls back to direct browser download.
	saveFilePicker func(req ChooseSaveFileRequestDTO) (string, error)
	// saveFileChoice holds the one native save file dialog that may be open.
	saveFileChoice saveFileChoice
	// urlOpener hands a link to the desktop window, which forwards it to the
	// system browser. It is nil in browser/socket mode, where the frontend opens
	// a tab itself.
	urlOpener func(url string) error
	// exportAssets supplies the built player a web export ships. It is a field so a
	// test can describe a build without one being present on the machine.
	exportAssets func() (fs.FS, error)
	// portraitListeners fan out background portrait generation events to active turn streams.
	portraitMu        sync.Mutex
	portraitSeq       uint64
	portraitListeners map[string]map[uint64]func(TurnEvent)
	// version is the application's build version, surfaced to the About dialog.
	// It is empty for callers (tests, the terminal client) that never set one.
	version string
}

// Config returns the configuration the service is running with, so a command can
// build shared infrastructure, such as a trace sink, from the same values.
func (s *Service) Config() *config.Config {
	return s.configMgr.Get()
}

// SetVersion records the application's build version so it can be reported to
// the UI. It is set once at startup, before any request is served.
func (s *Service) SetVersion(version string) {
	s.mu.Lock()
	s.version = version
	s.mu.Unlock()
}

// SetURLOpener installs the desktop window's link handler, which hands a URL to
// the system browser. Without one, OpenURL reports that no opener is available
// and the frontend opens a tab itself.
func (s *Service) SetURLOpener(opener func(url string) error) {
	s.mu.Lock()
	s.urlOpener = opener
	s.mu.Unlock()
}

// OpenURL asks the desktop window to open a link in the system browser. Only
// http and https are accepted: the handler is reachable from the app's own UI,
// so it must not become a way to launch arbitrary schemes.
func (s *Service) OpenURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q", parsed.Scheme)
	}
	s.mu.RLock()
	opener := s.urlOpener
	s.mu.RUnlock()
	if opener == nil {
		return errNoURLOpener
	}
	return opener(parsed.String())
}

// SetLogger attaches a trace sink to the service and to every turn it prepares.
func (s *Service) SetLogger(logger trace.Logger) {
	s.logger = trace.OrNil(logger)
	// The registries capture the logger when they build a client, so drop them so
	// a rebuilt client logs to the new sink.
	s.invalidateRegistries()
}

// invalidateRegistries drops every cached named media client, so a settings or
// logger change rebuilds them from the current configuration.
func (s *Service) invalidateRegistries() {
	s.ttsRegMu.Lock()
	if s.ttsReg != nil {
		s.ttsReg.Invalidate()
	}
	s.ttsRegMu.Unlock()
	s.sttRegMu.Lock()
	if s.sttReg != nil {
		s.sttReg.Invalidate()
	}
	s.sttRegMu.Unlock()
	s.imageRegMu.Lock()
	if s.imageReg != nil {
		s.imageReg.Invalidate()
	}
	s.imageRegMu.Unlock()
}

// ttsRegistry returns the service's named TTS registry, building it lazily.
func (s *Service) ttsRegistry() *media.TTSRegistry {
	s.ttsRegMu.Lock()
	defer s.ttsRegMu.Unlock()
	if s.ttsReg == nil {
		s.ttsReg = media.NewTTSRegistry(s.configMgr.Get(), s.logger)
	}
	return s.ttsReg
}

// sttRegistry returns the service's named STT registry, building it lazily.
func (s *Service) sttRegistry() *media.STTRegistry {
	s.sttRegMu.Lock()
	defer s.sttRegMu.Unlock()
	if s.sttReg == nil {
		s.sttReg = media.NewSTTRegistry(s.configMgr.Get())
	}
	return s.sttReg
}

// imageRegistry returns the service's named image registry, building it lazily.
func (s *Service) imageRegistry() *media.ImageRegistry {
	s.imageRegMu.Lock()
	defer s.imageRegMu.Unlock()
	if s.imageReg == nil {
		s.imageReg = media.NewImageRegistry(s.configMgr.Get(), s.logger)
	}
	return s.imageReg
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

	bgCtx, bgCancel := context.WithCancel(context.Background())
	svc := &Service{
		rootDir:        rootDir,
		resolver:       core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache),
		configMgr:      mgr,
		indexed:        make(map[string]bool),
		embWorkers:     make(map[string]*storage.EmbeddingWorker),
		locks:          make(map[string]*sync.Mutex),
		modelsManager:  models.NewManager(dirs.Cache),
		summaryPending: make(map[string]bool),
		limits:         harness.NewLimitRegistry(),
		exports:        newExportManager(),
		bgCtx:          bgCtx,
		bgCancel:       bgCancel,
	}
	// Point the embedding factory at the local model cache so the ONNX encoder
	// resolves, and let it report model events through the service logger.
	embeddings.SetModelDir(svc.modelsManager.ModelDir(models.EmbeddingEncoderModelID))
	embeddings.SetLogger(svc.logger)
	if !projectMode {
		if warning := paths.LegacyWarning(paths.System(), cfg.Paths); warning != "" {
			trace.OrNil(svc.logger).Event("paths.legacy_relative", map[string]interface{}{"detail": warning})
		}
	}
	for _, problem := range mgr.Warnings() {
		trace.OrNil(svc.logger).Event("config.problem", map[string]interface{}{"problem": problem})
	}
	svc.applyStylePack()
	return svc
}

// setStyleWarnings records why a style pack was ignored.
func (s *Service) setStyleWarnings(warnings []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stylePackWarnings = warnings
}

// styleWarnings returns why a style pack was ignored.
func (s *Service) styleWarnings() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.stylePackWarnings) == 0 {
		return nil
	}
	return append([]string(nil), s.stylePackWarnings...)
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
	// Cancel long-lived background work before waiting, so a batch poll stops
	// promptly rather than holding shutdown open for its next interval.
	if s.bgCancel != nil {
		s.bgCancel()
	}
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

// liveSegmentDTO renders one parsed turn-stream event as a client segment, or
// reports false for an event a client should not render: a control record, or an
// empty narration. It carries no audio, because the clips arrive with the
// authoritative turn that finalises the stream.
func liveSegmentDTO(event turnstream.Event, gameID ...string) (SegmentDTO, bool) {
	switch event.Kind {
	case turnstream.KindSpeech:
		if strings.TrimSpace(event.Text) == "" {
			return SegmentDTO{}, false
		}
		dto := SegmentDTO{
			Kind:      "speech",
			Speaker:   event.Speaker,
			SpeakerID: event.SpeakerID,
			Text:      event.Text,
			Player:    event.Player,
		}
		if len(gameID) > 0 && gameID[0] != "" && event.SpeakerID != "" {
			dto.PortraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID[0], event.SpeakerID)
		}
		return dto, true
	case turnstream.KindNarration:
		if strings.TrimSpace(event.Text) == "" {
			return SegmentDTO{}, false
		}
		return SegmentDTO{Kind: "narration", Text: event.Text}, true
	default:
		return SegmentDTO{}, false
	}
}

func segmentDTOs(segments []entity.TurnSegment, gameID string, plan clipPlan, resolve func(string) string, hasCustom ...func(string) bool) []SegmentDTO {
	var customChecker func(string) bool
	if len(hasCustom) > 0 {
		customChecker = hasCustom[0]
	}
	dtos := make([]SegmentDTO, 0, len(segments))
	for index, segment := range segments {
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
				if customChecker != nil {
					dto.HasCustomPortrait = customChecker(refID)
				}
			}
			if segment.SpeakerPortrait != "" {
				dto.SpeakerPortrait = segment.SpeakerPortrait
				dto.PortraitURL = segment.SpeakerPortrait
			}
		}
		// The keys come from the pipeline, so the URL a client is handed is the
		// URL of the audio synthesis writes: one sound, one name. A grouped
		// segment shares its group's key and names the group for the client.
		if index < len(plan.segmentKeys) {
			urls := make([]string, 0, len(plan.segmentKeys[index]))
			for _, key := range plan.segmentKeys[index] {
				urls = append(urls, clipURL(key))
			}
			dto.AudioURLs = urls
		}
		if index < len(plan.groupKey) {
			dto.ClipGroup = plan.groupKey[index]
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

// clipPlan is a turn's audio plan for the DTO: the clip keys per segment (a
// shared group key when grouped, the segment's own keys otherwise) and the group
// list the client renders controls for.
type clipPlan struct {
	segmentKeys [][]string
	groupKey    []string
	groups      []ClipGroupDTO
}

// clipPlanFor names a turn's clips from the shared pipeline. It returns an empty
// plan when no TTS provider is configured. Building the pipeline is cheap: a
// provider loads its model at first synthesis, not at construction.
func (s *Service) clipPlanFor(cfg *config.Config, gameID string, segments []entity.TurnSegment) clipPlan {
	plan := clipPlan{}
	if cfg.Media.TTS.Type == "" || cfg.Media.TTS.Type == "disabled" {
		return plan
	}
	pipeline, err := s.audioPipeline()
	if err != nil {
		return plan
	}
	narrator := s.narratorVoiceFor(gameID, cfg)
	voiceFor := s.voiceFor(gameID)
	plan.segmentKeys = make([][]string, len(segments))
	plan.groupKey = make([]string, len(segments))

	if s.groupingEnabled(cfg) {
		// The pipeline's group caps already fold under the live, single-speaker
		// caps when the streamer ran, so this names the clips it wrote and makes
		// the finalise pass a cache hit.
		groups := pipeline.GroupClipKeys(segments, narrator, voiceFor)
		plan.groups = make([]ClipGroupDTO, 0, len(groups))
		for _, group := range groups {
			plan.groups = append(plan.groups, ClipGroupDTO{
				Key:            group.Key,
				AudioURLs:      []string{clipURL(group.Key)},
				SegmentIndexes: group.SegmentIndexes,
			})
			for _, index := range group.SegmentIndexes {
				if index < 0 || index >= len(segments) {
					continue
				}
				plan.segmentKeys[index] = []string{group.Key}
				plan.groupKey[index] = group.Key
			}
		}
		return plan
	}

	for i, segment := range segments {
		keys, err := pipeline.SegmentClipKeys(segment, narrator, voiceFor)
		if err != nil {
			continue
		}
		plan.segmentKeys[i] = keys
	}
	return plan
}

// groupingEnabled reports whether a turn's audio is rendered as groups. "off"
// never groups; "auto" and "always" group. The streaming fold reproduces the
// batch plan, so a streamed turn groups live without paying twice.
func (s *Service) groupingEnabled(cfg *config.Config) bool {
	return cfg.TTSGrouping() != "off"
}

// streamerRuns reports whether the live pre-synthesiser runs for this turn.
func (s *Service) streamerRuns(cfg *config.Config) bool {
	return media.StreamerRuns(cfg)
}

// liveGrouping reports whether the streamer folds this turn's audio into groups,
// so the turn's clip plan must use the same single-speaker fold.
func (s *Service) liveGrouping(cfg *config.Config) bool {
	return media.LiveGrouping(cfg)
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

	_, contentWarnings, err := engine.ResolveContentLock(s.resolver, gameID)
	if err != nil {
		return nil, fmt.Errorf("resolve content lock: %w", err)
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

	// The campaign route serves the world's banner when the campaign has none of
	// its own, so one URL covers both and a change to either is a new URL.
	bannerURL, _ := s.gameAssetURL(gameDir, gameManifest.WorldID, "banner", gameID)

	var systemManifest *core.SystemManifest
	if sm, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(gameManifest.SystemID), "system.yaml")); err == nil {
		systemManifest = sm
	}

	// The world's genre tints the app's chrome, so a genre-less campaign keeps the
	// neutral look.
	worldGenre := ""
	if world, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(gameManifest.WorldID), "world.yaml")); err == nil {
		worldGenre = strings.TrimSpace(world.Genre)
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
		Genre:         worldGenre,
		StartLocation: startLocation,
		BannerURL:     bannerURL,

		MechanicsEngagement: engine.ResolveEngagement(gameManifest, systemManifest, s.configMgr.Get()),
		Advancement:         s.computeAdvancement(gameID, systemManifest),
		ContentWarnings:     contentWarnings,
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

	summaries := make([]EntitySummaryDTO, 0)
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		parsed, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			// A note that fails to parse is still a note the player wrote. Show
			// it so it can be repaired instead of silently vanishing.
			summaries = append(summaries, EntitySummaryDTO{
				ID:         filenameID,
				Name:       filenameID,
				Folder:     folder,
				ParseError: true,
			})
			return nil
		}

		// The declared id is the identity; the file name is only a convention.
		id := parsed.ID
		if id == "" {
			id = filenameID
		}
		name := parsed.Name
		if name == "" {
			name = id
		}

		hasPortrait := parsed.Portrait != "" && s.hasCustomPortrait(gameID, id)
		portraitURL := ""
		if entity.IsCharacterType(parsed.Type) {
			portraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait", gameID, id)
		}

		summaries = append(summaries, EntitySummaryDTO{
			ID:               id,
			Name:             name,
			Type:             parsed.Type,
			Location:         parsed.Location,
			Tags:             parsed.Tags,
			Aliases:          parsed.Aliases,
			Folder:           folder,
			FilenameMismatch: id != filenameID,
			HasPortrait:      hasPortrait,
			PortraitURL:      portraitURL,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})
	return summaries, nil
}

// eachEntityNote walks a collection's entities/ tree in path order, calling fn
// with each note's path, its folder relative to the root, and its bytes. Every
// reader of the tree goes through this, so "what counts as a note" is decided
// once: markdown files, not hidden, not under assets/.
func eachEntityNote(entitiesDir string, fn func(path, folder string, data []byte) error) error {
	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if entry.IsDir() {
			if path == entitiesDir {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "assets" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			return nil
		}

		rel, err := filepath.Rel(entitiesDir, filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		folder := ""
		if rel != "." && rel != "" {
			folder = filepath.ToSlash(rel)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // a note that vanished mid-walk is not an error
		}
		return fn(path, folder, data)
	})
	if err != nil {
		return fmt.Errorf("walk entities dir: %w", err)
	}
	return nil
}

// folderForNotePath is the stored folder form for a note path: the directory
// relative to the entities root, slash-separated, with "" for the root.
func folderForNotePath(entitiesDir, path string) string {
	rel, err := filepath.Rel(entitiesDir, filepath.Dir(path))
	if err != nil || rel == "." || rel == "" {
		return ""
	}
	return filepath.ToSlash(rel)
}

func (s *Service) GetEntity(ctx context.Context, gameID, entityID string) (*EntityDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return nil, fmt.Errorf("invalid entity id: %w", err)
	}

	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")

	path, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("read entity file: %w", fs.ErrNotExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity file: %w", err)
	}
	folder := folderForNotePath(entitiesDir, path)

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		// Return the raw note so the codex can still open and repair it, rather
		// than failing the read outright.
		return &EntityDTO{
			ID:         entityID,
			Name:       entityID,
			Markdown:   string(data),
			Folder:     folder,
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
		Folder:    folder,
		History:   ent.History,
	}, nil
}

// ErrDuplicateEntityID reports a save whose frontmatter id already belongs to a
// different note. The id is the identity, so a second claim is refused rather
// than silently overwriting a note in another folder.
var ErrDuplicateEntityID = errors.New("entity id already in use")

// SaveEntity writes a note at the collection root.
func (s *Service) SaveEntity(ctx context.Context, gameID, entityID, rawMarkdown string) error {
	return s.SaveEntityInFolder(ctx, gameID, entityID, "", rawMarkdown)
}

// SaveEntityInFolder writes a note at folder, moving it when it already lives
// somewhere else. The file name stays the id, because the id is the identity and
// the folder is only location: no inbound link is rewritten, which is the whole
// point of decoupling the two.
func (s *Service) SaveEntityInFolder(ctx context.Context, gameID, entityID, folder, rawMarkdown string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}
	cleanFolder, err := ValidateFolderPath(folder)
	if err != nil {
		return err
	}

	ent, err := entity.ParseMarkdownEntity([]byte(rawMarkdown))
	if err != nil {
		return fmt.Errorf("save entity %q: %w", entityID, err)
	}
	// The file name is the note's identity, so a hand-edited or copied id can
	// never index a note under another note's key.
	ent.ID = entityID
	ent.Folder = cleanFolder

	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")

	if err := s.assertIDIsFree(entitiesDir, entityID, rawMarkdown); err != nil {
		return err
	}

	existingPath, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return err
	}

	targetDir := entitiesDir
	if cleanFolder != "" {
		targetDir = filepath.Join(entitiesDir, filepath.FromSlash(cleanFolder))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create folder %q: %w", cleanFolder, err)
	}

	normalised, err := ent.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("normalise entity %q: %w", entityID, err)
	}

	targetPath, err := pathutil.ResolveSafeChild(targetDir, entityID+".md")
	if err != nil {
		return fmt.Errorf("resolve entity path: %w", err)
	}
	if strings.Contains(targetPath, "..") {
		return fmt.Errorf("invalid entity path: contains traversal")
	}
	if err := os.WriteFile(targetPath, normalised, 0o644); err != nil {
		return fmt.Errorf("write entity file: %w", err)
	}
	if existingPath != "" && existingPath != targetPath {
		if err := os.Remove(existingPath); err != nil {
			return fmt.Errorf("remove old note %q: %w", existingPath, err)
		}
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil // Non-fatal if db sync fails temporarily
	}

	syncer := storage.NewSyncer(store)
	return syncer.SyncFile(targetPath)
}

// findEntityNote returns the path a note currently occupies, or "" when it is
// new. A note is found by the id it declares or by its file name, so a note that
// an author renamed by hand is still reachable.
func (s *Service) findEntityNote(entitiesDir, entityID string) (string, error) {
	found := ""
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		if filenameID == entityID {
			found = path
			return nil
		}
		if entity.DeclaredID(data) == entityID {
			found = path
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return found, nil
}

// assertIDIsFree refuses a save whose frontmatter id already names another note.
// The requested file name is the id, so a body declaring a different id is a
// collision with that other note rather than a rename.
func (s *Service) assertIDIsFree(entitiesDir, entityID, rawMarkdown string) error {
	declared := entity.DeclaredID([]byte(rawMarkdown))
	if declared == "" || declared == entityID {
		return nil
	}
	if _, err := os.Stat(filepath.Join(entitiesDir, declared+".md")); err == nil {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	existing, err := s.findEntityNote(entitiesDir, declared)
	if err == nil && existing != "" {
		return fmt.Errorf("%w: %q; use a different id", ErrDuplicateEntityID, declared)
	}
	return nil
}

// MergeEntities folds one note into another: the survivor keeps its identity and
// gains the source's prose, tags, aliases, and turn history, every note that linked
// to the source is rewritten to point at the survivor, and the source is removed.
//
// It is deliberately explicit. Deciding that two names are one being is a judgement
// the engine cannot make, but it can carry the decision out once a player makes it.
func (s *Service) MergeEntities(ctx context.Context, gameID, sourceID, targetID string) (*EntityDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(sourceID); err != nil {
		return nil, fmt.Errorf("invalid source id: %w", err)
	}
	if err := pathutil.ValidateID(targetID); err != nil {
		return nil, fmt.Errorf("invalid target id: %w", err)
	}
	if sourceID == targetID {
		return nil, fmt.Errorf("cannot merge %q into itself", sourceID)
	}

	gameDir := s.resolver.GameDir(gameID)
	entitiesDir := filepath.Join(gameDir, "entities")
	sourcePath, err := pathutil.ResolveSafeChild(entitiesDir, sourceID+".md")
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	if strings.Contains(sourcePath, "..") {
		return nil, fmt.Errorf("invalid source path: contains traversal")
	}
	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", sourceID, err)
	}
	targetPath, err := pathutil.ResolveSafeChild(entitiesDir, targetID+".md")
	if err != nil {
		return nil, fmt.Errorf("resolve target path: %w", err)
	}
	if strings.Contains(targetPath, "..") {
		return nil, fmt.Errorf("invalid target path: contains traversal")
	}
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

	if err := os.Remove(sourcePath); err != nil {
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
	for _, p := range []string{targetPath, sourcePath} {
		_ = syncer.SyncFile(p)
	}

	return s.GetEntity(ctx, gameID, targetID)
}

// rewriteInboundLinks points every note that linked to the source at the survivor,
// so no note is left pointing at an entity that no longer exists.
func (s *Service) rewriteInboundLinks(gameDir, sourceID, targetID string) error {
	entitiesDir := filepath.Join(gameDir, "entities")

	pattern := regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(sourceID) + `(\s*\|[^\]]*)?\]\]`)
	replacement := "[[" + targetID + "$1]]"

	return eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		if strings.TrimSuffix(filepath.Base(path), ".md") == sourceID {
			return nil
		}
		if !pattern.Match(data) {
			return nil
		}

		updated := pattern.ReplaceAll(data, []byte(replacement))
		if err := os.WriteFile(path, updated, 0644); err != nil {
			return fmt.Errorf("rewrite links in %s: %w", filepath.Base(path), err)
		}
		return nil
	})
}

// ListFolders returns the folder tree under an entities directory. Folders are
// read from disk rather than from the index, so a folder with no notes in it is
// still visible and can be dragged into.
func (s *Service) ListFolders(entitiesDir string) ([]FolderDTO, error) {
	paths := make([]string, 0)

	err := filepath.WalkDir(entitiesDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if !entry.IsDir() || path == entitiesDir {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "assets" {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(entitiesDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return BuildFolderTree(paths), nil
}

// CreateFolder creates a folder and any missing parents.
func (s *Service) CreateFolder(entitiesDir, path string) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: a folder needs a name", ErrInvalidFolderPath)
	}
	return os.MkdirAll(filepath.Join(entitiesDir, filepath.FromSlash(clean)), 0o755)
}

// MoveFolder renames a folder, taking its notes with it. No link is rewritten,
// because a note is linked by id and not by the path it happens to sit at.
func (s *Service) MoveFolder(entitiesDir, from, to string) error {
	cleanFrom, err := ValidateFolderPath(from)
	if err != nil {
		return err
	}
	cleanTo, err := ValidateFolderPath(to)
	if err != nil {
		return err
	}
	if cleanFrom == "" || cleanTo == "" {
		return fmt.Errorf("%w: cannot move the entities root", ErrInvalidFolderPath)
	}

	source := filepath.Join(entitiesDir, filepath.FromSlash(cleanFrom))
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("move folder %q: %w", cleanFrom, err)
	}

	target := filepath.Join(entitiesDir, filepath.FromSlash(cleanTo))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create parent of %q: %w", cleanTo, err)
	}
	return os.Rename(source, target)
}

// DeleteFolder removes a folder. A folder holding notes is refused unless
// recursive is set, so a stray click cannot delete a campaign's lore.
func (s *Service) DeleteFolder(entitiesDir, path string, recursive bool) error {
	clean, err := ValidateFolderPath(path)
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("%w: cannot delete the entities root", ErrInvalidFolderPath)
	}

	target := filepath.Join(entitiesDir, filepath.FromSlash(clean))
	empty, err := folderIsEmpty(target)
	if err != nil {
		return err
	}
	if !empty && !recursive {
		return fmt.Errorf("folder %q is not empty; pass recursive=true to delete its notes", clean)
	}
	return os.RemoveAll(target)
}

// folderIsEmpty reports whether a directory holds anything, so a non-recursive
// delete can refuse instead of destroying notes.
func folderIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read folder %q: %w", dir, err)
	}
	return len(entries) == 0, nil
}

// GameFolders lists a campaign's folder tree.
func (s *Service) GameFolders(gameID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.GameDir(gameID), "entities"))
}

// WorldFolders lists a world's folder tree.
func (s *Service) WorldFolders(worldID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.WorldDir(worldID), "entities"))
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
	nodes := make([]GraphNodeDTO, 0)
	links := make([]GraphLinkDTO, 0)
	known := make(map[string]struct{})

	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		ent, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			return nil
		}
		// The declared id is the node's identity, so a nested note is a
		// first-class node rather than a file name that happens to be unique.
		id := ent.ID
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		known[id] = struct{}{}
		nodes = append(nodes, GraphNodeDTO{ID: id, Label: ent.Name, Type: ent.Type})

		for _, target := range ent.Wikilinks {
			links = append(links, GraphLinkDTO{Source: id, Target: target})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A hand-written path-qualified link lands on the note whose id is its final
	// segment, so the graph and the mention resolver agree.
	for i := range links {
		if _, ok := known[links[i].Target]; ok {
			continue
		}
		if base := entity.WikilinkBasename(links[i].Target); base != links[i].Target {
			if _, ok := known[base]; ok {
				links[i].Target = base
			}
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
	engagement := s.engagementFor(gameID)
	for i, turn := range turns {
		dtos[i] = s.turnDTO(turn, store, cfg, gameID, engagement)
	}
	return dtos, nil
}

// turnDTO maps a persisted turn for the API. GetChronicle and the turn endpoint
// recordReportDTO maps a turn's control-record report for the API, or nil when
// the turn had none.
func recordReportDTO(report *engine.RecordReport) *RecordReportDTO {
	if report == nil {
		return nil
	}
	dto := &RecordReportDTO{
		Total:    report.Total,
		Repaired: report.Repaired,
		Failed:   report.Failed,
	}
	for _, issue := range report.Issues {
		dto.Issues = append(dto.Issues, RecordIssueDTO{Type: issue.Type, Repair: issue.Repair, Error: issue.Error})
	}
	return dto
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

// healthEffectDTOs maps the engine's resolved health effects to the wire shape.
func healthEffectDTOs(effects []engine.HealthEffect) []HealthEffectDTO {
	if len(effects) == 0 {
		return nil
	}
	out := make([]HealthEffectDTO, 0, len(effects))
	for _, effect := range effects {
		out = append(out, HealthEffectDTO{Entity: effect.Entity, Effect: effect.Effect})
	}
	return out
}

// ErrTurnInFlight means another turn is already running for this campaign.
var ErrTurnInFlight = errors.New("a turn is already in flight")

// errNoURLOpener means the service is running without a desktop window, so it
// cannot hand a link to the system browser. The frontend opens a tab instead.
var errNoURLOpener = errors.New("no url opener available")

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

// Run plays one turn, emitting events as they happen. An emit failure cancels the
// turn, which is how a disconnected client stops generation rather than paying for
// a turn nobody will see.
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.TurnTimeout())
	defer cancel()

	// Every emission passes this lock: tool activity arrives from the orchestrator
	// while a streamer worker announces a finished sentence, and the wire is one
	// newline-delimited stream.
	var emitMu sync.Mutex
	announce := func(event TurnEvent) error {
		emitMu.Lock()
		defer emitMu.Unlock()
		return emit(event)
	}

	// Tool activity is streamed as it happens, so a lookup reads as progress
	// rather than as a stall. A failed emit is ignored: the turn still records,
	// and a disconnected client is handled by the chunk listener below.
	t.orchestrator.SetToolObserver(func(activity engine.ToolActivity) {
		_ = announce(toolEvent(activity))
	})

	t.orchestrator.SetPendingCheckRef(req.PendingCheckRef)
	t.orchestrator.SetForcedTotal(req.ForcedTotal)
	t.orchestrator.SetSingleTurnMode(t.cfg.InteractiveRolls() == "single-turn")
	t.orchestrator.SetImageTrigger(t.cfg.ImageTrigger())

	// Application playback runs on one queue opened before generation: a sentence
	// the streamer synthesizes is heard as soon as it lands, and the finalise pass
	// adds only what the stream has not already played. A session with no player
	// still warms the clips, because the URLs a client is handed are
	// content-addressed and cannot synthesize on demand.
	audioEnabled := t.cfg.Media.TTS.Type != "" && t.cfg.Media.TTS.Type != "disabled"
	plan := newTurnAudioPlan(nil)
	if audioEnabled && t.cfg.Media.TTS.AutoPlay {
		if player := t.service.audioPlayer(); player != nil && player.Available() {
			plan.queue = make(chan string, sentenceQueueDepth)
			player.SetVolume(t.cfg.Media.TTS.MasterVolume)
			// Playback is the application's own responsibility, detached from the
			// request: the queue is fed while the turn streams and drains after it.
			// StopAudio ends it early.
			t.service.goBackground(func() {
				if err := player.EnqueueQueue(plan.queue); err != nil && !errors.Is(err, playback.ErrUnavailable) {
					fmt.Fprintf(os.Stderr, "Warning: narration playback stopped: %v\n", err)
				}
			})
		}
	}

	// Sentences are synthesized while the model is still writing, so a finished
	// segment whose text is one of them is a cache hit at finalise rather than a
	// second provider call. Nil when disabled or no provider is configured.
	turnNum := 1
	if max, err := t.store.MaxTurnNumber(); err == nil && max >= 0 {
		turnNum = max + 1
	}

	// streamedKeys records the clip keys the streamer emitted, so finalise can
	// assert they are the keys the plan contains.
	streamedKeys := map[string]bool{}
	streamer := t.service.sentenceStreamerFor(runCtx, t.gameID, t.cfg, func(speech provisionalSpeech) {
		if speech.AudioKey != "" {
			streamedKeys[speech.AudioKey] = true
		}
		plan.markHeard(speech.Segments)
		plan.enqueueClip(speech.AudioKey, t.service.clipPath(speech.AudioKey))
		_ = announce(speechEvent(speech))
	})
	if streamer != nil {
		baseVoiceFor := t.service.voiceFor(t.gameID)
		streamer.SetVoiceResolver(func(speakerID string) *entity.VoiceConfig {
			if v := t.orchestrator.Voice(speakerID); v != nil {
				return v
			}
			if baseVoiceFor != nil {
				return baseVoiceFor(speakerID)
			}
			return nil
		})
	}
	streamer.SetTurnNumber(turnNum)
	streamer.SetProgressObserver(func(progress AudioProgressDTO) {
		_ = announce(TurnEvent{Type: "audio_progress", AudioProgress: &progress})
	})
	defer streamer.Close()

	removePortrait := t.service.addPortraitListener(t.gameID, func(evt TurnEvent) {
		_ = announce(evt)
	})
	defer removePortrait()

	// Parsed segments are announced as they arrive, so the client renders
	// attributed speech while the model is still writing, and the streamer voices
	// each one in its speaker's own voice. A failed emit is ignored, exactly as
	// tool activity is: the turn still records.
	// segmentIndex is the position of the event in the turn's segment order, which
	// is the same index space the clip plan uses, so the heard ledger and the plan
	// agree on what a segment is.
	segmentIndex := 0
	t.orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		segment, isSegment := liveSegmentDTO(event, t.gameID)
		if isSegment {
			if segment.SpeakerID != "" && t.service.hasCustomPortrait(t.gameID, segment.SpeakerID) {
				segment.HasCustomPortrait = true
			}
			_ = announce(TurnEvent{Type: "segment", Segment: &segment})
		}
		streamer.FeedSegment(event, segmentIndex)
		if isSegment {
			segmentIndex++
		}
	})

	t.orchestrator.SetSceneOnly(req.SceneOnly)
	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
		return announce(TurnEvent{Type: "chunk", Text: text})
	})
	if err != nil {
		t.service.noteFailure("gm", err)
		// Nothing more will be announced, so the queue can close without a worker
		// sending into it on its way out.
		streamer.StopEmitting()
		plan.close()
		return err
	}
	t.service.noteSuccess("gm")

	// The turn is authoritative from here, so a sentence still in flight is no
	// longer announced: it is synthesized all the same, and the finalise pass plays
	// it with the rest of the turn. That keeps the played set the client holds in
	// step with the audio it actually heard.
	streamer.StopEmitting()

	dto := t.service.turnDTO(*turn, t.store, t.cfg, t.gameID, t.service.engagementFor(t.gameID))

	// Release campaign turn lock immediately so the player can submit the next turn
	// without waiting for remaining background TTS audio to synthesize.
	t.Close()

	if err := announce(TurnEvent{Type: "turn", Turn: &dto}); err != nil {
		return err
	}

	// When built-in TTS is configured and model weights are missing, inform the client
	// so the user can be prompted to download the voice pack.
	if t.cfg.Media.TTS.Type == "builtin" && (t.cfg.Media.TTS.BuiltinName == "sherpa-onnx" || t.cfg.Media.TTS.BuiltinName == "kokoro") {
		status := t.service.modelsManager.Status("kokoro-tts")
		if !status.Installed {
			_ = announce(TurnEvent{
				Type:    "model_missing",
				ModelID: "kokoro-tts",
				Name:    status.Name,
				Size:    status.TotalBytes,
			})
		}
	}

	// The rest of the turn's clips are synthesized behind the turn and appended to
	// the same queue, so playback continues without a second start and a clip that
	// failed mid-stream is retried here. The played set makes the handover exact.
	// Start memory summarization and character enrichment / portrait generation
	// in the background as soon as the turn is recorded.
	t.service.summariseBehind(t.gameID, t.chronicler)
	t.service.goBackground(func() { t.service.scanAndEnrichCharacters(t.gameID, t.store) })

	// Flush and wait for remaining in-flight audio synthesis jobs to finish and
	// emit their progress events to the client before the turn stream closes.
	streamer.Close()
	if audioEnabled {
		streamer.Wait()
		t.finishTurnAudio(context.Background(), *turn, plan)
		t.service.logParityMismatch(t.gameID, turn.Segments, streamedKeys)
	}
	plan.close()
	return nil
}

// finishTurnAudio synthesizes every clip the turn still needs once it is recorded,
// appending to the plan only what the streamed sentences did not already play, so
// no line is heard twice and none is missed.
func (t *TurnSession) finishTurnAudio(ctx context.Context, turn engine.Turn, plan *turnAudioPlan) {
	t.service.emitTurnClips(ctx, t.gameID, turn, false, func(clip string) {
		plan.enqueueClip(media.ClipKeyForPath(clip), clip)
	}, plan)
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
		trace.OrNil(s.logger).Event("turn.audio_parity_mismatch", map[string]interface{}{"keys": missing})
	}
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

	store := s.sceneArtResolver(gameID)
	if store == nil {
		return "", "", fmt.Errorf("build image client")
	}

	cfg := s.configMgr.Get()
	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.image.request", map[string]interface{}{
		"location": locationID,
		"provider": cfg.Media.Image.Type + ":" + cfg.Media.Image.Model,
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

func (s *Service) addPortraitListener(gameID string, listener func(TurnEvent)) func() {
	s.portraitMu.Lock()
	defer s.portraitMu.Unlock()
	if s.portraitListeners == nil {
		s.portraitListeners = make(map[string]map[uint64]func(TurnEvent))
	}
	if s.portraitListeners[gameID] == nil {
		s.portraitListeners[gameID] = make(map[uint64]func(TurnEvent))
	}
	s.portraitSeq++
	id := s.portraitSeq
	s.portraitListeners[gameID][id] = listener

	return func() {
		s.portraitMu.Lock()
		defer s.portraitMu.Unlock()
		if m := s.portraitListeners[gameID]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(s.portraitListeners, gameID)
			}
		}
	}
}

func (s *Service) broadcastPortraitReady(gameID, characterID, relPath string, version ...int) {
	s.portraitMu.Lock()
	var listeners []func(TurnEvent)
	if m := s.portraitListeners[gameID]; m != nil {
		for _, fn := range m {
			listeners = append(listeners, fn)
		}
	}
	s.portraitMu.Unlock()

	ver := 0
	if len(version) > 0 {
		ver = version[0]
	}
	if ver <= 0 {
		ver = parsePortraitVersionFromPath(relPath)
	}
	portraitURL := fmt.Sprintf("/api/game/%s/character/%s/portrait?t=%d", gameID, characterID, time.Now().UnixMilli())
	if ver > 0 {
		portraitURL = fmt.Sprintf("/api/game/%s/character/%s/portrait?v=%d&t=%d", gameID, characterID, ver, time.Now().UnixMilli())
	}

	evt := TurnEvent{
		Type:              "portrait",
		CharacterID:       characterID,
		PortraitURL:       portraitURL,
		Version:           ver,
		HasCustomPortrait: true,
	}
	for _, fn := range listeners {
		fn(evt)
	}
}

func (s *Service) broadcastSceneImageReady(gameID string, turnNumber int, relPath string) {
	s.portraitMu.Lock()
	var listeners []func(TurnEvent)
	if m := s.portraitListeners[gameID]; m != nil {
		for _, fn := range m {
			listeners = append(listeners, fn)
		}
	}
	s.portraitMu.Unlock()

	evt := TurnEvent{
		Type:       "scene_image",
		TurnNumber: turnNumber,
		ImageURL:   fmt.Sprintf("/api/game/%s/turn/%d/scene-image", gameID, turnNumber),
	}
	for _, fn := range listeners {
		fn(evt)
	}
}

func parsePortraitVersionFromPath(relPath string) int {
	base := filepath.Base(relPath)
	idx := strings.LastIndex(base, "-v")
	if idx == -1 {
		return 0
	}
	dot := strings.LastIndex(base, ".")
	if dot == -1 || dot <= idx+2 {
		return 0
	}
	vStr := base[idx+2 : dot]
	v, err := strconv.Atoi(vStr)
	if err != nil {
		return 0
	}
	return v
}

func (s *Service) hasCustomPortrait(gameID, characterID string) bool {
	if s == nil || s.resolver == nil || gameID == "" || characterID == "" {
		return false
	}
	gameDir := s.resolver.GameDir(gameID)
	portraitsDir := filepath.Join(gameDir, "assets", "portraits")
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".svg"} {
		// Check both unversioned and versioned files
		matches, err := filepath.Glob(filepath.Join(portraitsDir, characterID+"*"+ext))
		if err == nil && len(matches) > 0 {
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && !info.IsDir() && info.Size() > 0 {
					return true
				}
			}
		}
	}
	return false
}

// GenerateTurnSceneImage enqueues a scene image for a turn on demand, regardless
// of the trigger policy, so a player can illustrate a beat the policy skipped.
func (s *Service) GenerateTurnSceneImage(ctx context.Context, gameID string, turnNumber int) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	cfg := s.configMgr.Get()
	if cfg.Media.Image.Type == "" || cfg.Media.Image.Type == "disabled" {
		return fmt.Errorf("image generation is disabled")
	}
	imgClient, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil || imgClient == nil {
		return fmt.Errorf("no image provider is configured")
	}
	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return err
	}

	store := s.storeOrNil(gameID)
	var locEntity *entity.Entity
	if turn.Location != "" && store != nil {
		locEntity, _ = store.GetEntity(turn.Location)
	}
	cue := turn.SceneBreakCue
	if cue == "" {
		cue = engine.ExtractSceneCue(turn.Narration)
	}
	sceneCtx := engine.ScenePromptContext{
		Cue:       cue,
		Narration: turn.Narration,
		Action:    turn.Input,
		Location:  turn.Location,
		Style:     s.worldArtStyle(gameID),
	}
	if locEntity != nil {
		sceneCtx.Location = locEntity.Name
		sceneCtx.Appearance = locEntity.Appearance
	}
	if len(turn.Checks) > 0 {
		sceneCtx.Outcome = turn.Checks[0].Outcome
	}
	prompt := engine.BuildScenePrompt(sceneCtx)

	worker := engine.NewSceneWorker(s.resolver, imgClient)
	worker.SetOnReady(func(gID string, n int, relPath string) {
		s.broadcastSceneImageReady(gID, n, relPath)
	})
	worker.Enqueue(gameID, turnNumber, prompt)
	return nil
}

// GetTurnSceneImage returns the generated scene illustration for a specific turn.
func (s *Service) GetTurnSceneImage(ctx context.Context, gameID string, turnNumber int) ([]byte, string, error) {
	scenesDir := filepath.Join(s.resolver.GameDir(gameID), "assets", "scenes")
	for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
		p := filepath.Join(scenesDir, fmt.Sprintf("turn-%d%s", turnNumber, ext))
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data, imageContentType(data), nil
		}
	}
	return nil, "", fmt.Errorf("scene image for turn %d not found", turnNumber)
}

// GetCharacterPortrait returns the portrait image for a character, or a procedural SVG fallback if not found.
// When an explicit version (> 0) is specified, it serves that exact historical portrait version.
func (s *Service) GetCharacterPortrait(ctx context.Context, gameID, characterID string, version ...int) ([]byte, string, error) {
	s.ensureIndexed(gameID)
	cleanID := entity.Slugify(characterID)
	if cleanID == "" {
		return nil, "", fmt.Errorf("invalid character id %q", characterID)
	}

	gameDir := filepath.Clean(s.resolver.GameDir(gameID))
	portraitsDir := filepath.Clean(filepath.Join(gameDir, "assets", "portraits"))

	store, err := s.store(gameID)
	if err != nil {
		return nil, "", err
	}
	ent, err := store.GetEntity(cleanID)
	if err != nil || ent == nil {
		// Fallback: check entity markdown file on disk directly
		notePath := filepath.Clean(filepath.Join(gameDir, "entities", cleanID+".md"))
		if strings.HasPrefix(notePath, gameDir+string(filepath.Separator)) {
			if data, readErr := os.ReadFile(notePath); readErr == nil {
				if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
					ent = parsed
				}
			}
		}
	}
	if ent == nil {
		return nil, "", fmt.Errorf("character %q not found", characterID)
	}

	safeID := ent.ID

	readFileInGameDir := func(relPath string) ([]byte, bool) {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			return nil, false
		}
		p := filepath.Clean(filepath.Join(gameDir, relPath))
		if !strings.HasPrefix(p, gameDir+string(filepath.Separator)) {
			return nil, false
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil || len(data) == 0 {
			return nil, false
		}
		return data, true
	}

	readPortraitFile := func(filename string) ([]byte, bool) {
		filename = filepath.Base(filename)
		p := filepath.Clean(filepath.Join(portraitsDir, filename))
		if !strings.HasPrefix(p, portraitsDir+string(filepath.Separator)) {
			return nil, false
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil || len(data) == 0 {
			return nil, false
		}
		return data, true
	}

	// If explicit version requested:
	if len(version) > 0 && version[0] > 0 {
		targetVer := version[0]
		// 1. Check direct file assets/portraits/<id>-v<version>.<ext>
		for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
			if data, ok := readPortraitFile(fmt.Sprintf("%s-v%d%s", safeID, targetVer, ext)); ok {
				return data, imageContentType(data), nil
			}
		}
		// 2. Check if current ent.Portrait matches this version
		if ent.PortraitVersion == targetVer && ent.Portrait != "" {
			if data, ok := readFileInGameDir(ent.Portrait); ok {
				return data, imageContentType(data), nil
			}
		}
		// 3. Check portrait history entries
		for _, histPath := range ent.PortraitHistory {
			if strings.Contains(histPath, fmt.Sprintf("-v%d.", targetVer)) {
				if data, ok := readFileInGameDir(histPath); ok {
					return data, imageContentType(data), nil
				}
			}
		}
		// 4. If targetVer == 1, check legacy unversioned files
		if targetVer == 1 {
			for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
				if data, ok := readPortraitFile(safeID + ext); ok {
					return data, imageContentType(data), nil
				}
			}
		}
	} else {
		// No version requested: serve current active portrait
		if ent.Portrait != "" {
			if data, ok := readFileInGameDir(ent.Portrait); ok {
				return data, imageContentType(data), nil
			}
		}
		// Check fallback unversioned file on disk
		for _, ext := range []string{".png", ".webp", ".jpg", ".jpeg", ".svg"} {
			if data, ok := readPortraitFile(safeID + ext); ok {
				return data, imageContentType(data), nil
			}
		}
	}

	// Procedural SVG fallback
	svg := media.GenerateProceduralPortrait(media.PortraitRequestFor(ent))
	return svg, "image/svg+xml", nil
}

// RegenerateCharacterPortrait generates a fresh portrait for a character and
// overwrites any existing one, returning a cache-busted URL.
func (s *Service) RegenerateCharacterPortrait(ctx context.Context, gameID, characterID string) (CharacterPortraitDTO, error) {
	s.ensureIndexed(gameID)
	cleanID := entity.Slugify(characterID)
	if cleanID == "" {
		return CharacterPortraitDTO{}, fmt.Errorf("invalid character id %q", characterID)
	}

	gameDir := filepath.Clean(s.resolver.GameDir(gameID))
	store, err := s.store(gameID)
	if err != nil {
		return CharacterPortraitDTO{}, err
	}
	ent, err := store.GetEntity(cleanID)
	if err != nil || ent == nil {
		notePath := filepath.Clean(filepath.Join(gameDir, "entities", cleanID+".md"))
		if strings.HasPrefix(notePath, gameDir+string(filepath.Separator)) {
			if data, readErr := os.ReadFile(notePath); readErr == nil {
				if parsed, parseErr := entity.ParseMarkdownEntity(data); parseErr == nil {
					ent = parsed
				}
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
	worker.SetOnReady(func(gID, charID, relPath string) {
		s.broadcastPortraitReady(gID, charID, relPath)
	})
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

// sceneArtResolver builds the one scene-art resolver for a campaign: the image client
// with the shared key, the world's art style, and the campaign's cache. The route that
// serves scene art and an export that carries it both use it, so a bundle shows the
// images the app already has rather than generating its own.
func (s *Service) sceneArtResolver(gameID string) *media.ArtStore {
	client, sceneCfg, err := s.imageRegistry().ForPurpose(config.PurposeScene)
	if err != nil {
		return nil
	}
	params := sceneCfg.Type + ":" + sceneCfg.Model
	return media.NewArtStore(client, media.NewContentCache(s.resolver.CacheDir()), s.worldArtStyle(gameID), params)
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
			portraitWorker.SetOnReady(func(gID, charID, relPath string) {
				s.broadcastPortraitReady(gID, charID, relPath)
			})
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

		if portraitWorker != nil && (ent.Portrait == "" || !s.hasCustomPortrait(gameID, ent.ID)) {
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

// ClipPath resolves a clip key to its stored file. The key is the clip's whole
// name and nothing else is consulted, so a key that is not one resolves to
// nothing rather than to a path.
func (s *Service) ClipPath(key string) (string, bool) {
	if !clipKeyPattern.MatchString(key) {
		return "", false
	}
	path := filepath.Join(s.resolver.CacheDir(), "audio", key+".opus")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

// clipPath names the file a clip key is stored under. The player plays files, and
// the key is the file name, so no lookup is needed.
func (s *Service) clipPath(key string) string {
	if key == "" {
		return ""
	}
	return filepath.Join(s.resolver.CacheDir(), "audio", key+".opus")
}

// GetSegmentClips synthesizes one segment on demand and returns its ordered clips,
// reusing every clip the cache already holds. When grouping is enabled a segment
// shares its group's clip, so regenerating one segment regenerates the group it
// belongs to.
func (s *Service) GetSegmentClips(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) ([]string, error) {
	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		return nil, err
	}
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return nil, fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turnNumber)
	}
	isForce := len(force) > 0 && force[0]

	cfg := s.configMgr.Get()
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	if s.groupingEnabled(cfg) {
		groups := pipeline.GroupClipKeys(turn.Segments, s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID))
		if group, ok := media.GroupForSegment(groups, segmentIndex); ok {
			rendered, err := pipeline.SynthesizeGroupsForce(ctx, []media.ClipGroup{group}, isForce)
			if err != nil {
				s.noteFailure("tts", err)
				return nil, err
			}
			s.noteSuccess("tts")
			s.recordTTSUsage(gameID, turnNumber, cfg, pipeline)
			if len(rendered) > 0 && rendered[0].Cached {
				return []string{s.clipPath(rendered[0].Key)}, nil
			}
			return nil, nil
		}
	}
	return s.synthesizeSegment(ctx, gameID, *turn, segmentIndex, isForce)
}

// synthesizeSegment renders one segment through the ungrouped per-segment path,
// the fallback when grouping is off or a group failed.
func (s *Service) synthesizeSegment(ctx context.Context, gameID string, turn engine.Turn, segmentIndex int, force bool) ([]string, error) {
	if segmentIndex < 0 || segmentIndex >= len(turn.Segments) {
		return nil, fmt.Errorf("segment %d out of range for turn %d", segmentIndex, turn.Number)
	}
	cfg := s.configMgr.Get()
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	clips, err := pipeline.SynthesizeSegmentClips(ctx, turn.Segments[segmentIndex], s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID), force)
	if err != nil {
		s.noteFailure("tts", err)
		return nil, err
	}
	s.noteSuccess("tts")
	s.recordTTSUsage(gameID, turn.Number, cfg, pipeline)
	return clips, nil
}

// synthesizeTurnGroups renders a whole turn's groups when grouping is enabled,
// reporting whether it did. It returns the rendered groups so a caller can fall
// back to per-segment synthesis for any group that failed.
func (s *Service) synthesizeTurnGroups(ctx context.Context, gameID string, turn engine.Turn, cfg *config.Config, force bool) ([]media.ClipGroup, bool, error) {
	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, false, err
	}
	if !s.groupingEnabled(cfg) {
		return nil, false, nil
	}
	// The clip plan and the synthesis fold under the pipeline's group caps, which
	// are already the live, single-speaker caps when the streamer ran, so the DTO
	// names the clips the finalise pass wrote.
	groups := pipeline.GroupClipKeys(turn.Segments, s.narratorVoiceFor(gameID, cfg), s.voiceFor(gameID))
	rendered, err := pipeline.SynthesizeGroupsForce(ctx, groups, force)
	if err != nil {
		s.noteFailure("tts", err)
	} else {
		s.noteSuccess("tts")
	}
	s.recordTTSUsage(gameID, turn.Number, cfg, pipeline)
	return rendered, true, err
}

// emitTurnClips yields a turn's clips in play order, grouped when grouping is
// enabled and per-segment otherwise, so every consumer agrees on what a turn
// sounds like. A group that failed falls back to per-segment synthesis so the
// beat is not silent. plan, when non-nil, is the heard ledger: a group whose
// segments are all heard is skipped, and a partially heard group is suppressed
// and traced, so no clip plays twice.
func (s *Service) emitTurnClips(ctx context.Context, gameID string, turn engine.Turn, force bool, emit func(string), plan *turnAudioPlan) {
	cfg := s.configMgr.Get()
	if groups, grouped, _ := s.synthesizeTurnGroups(ctx, gameID, turn, cfg, force); grouped {
		for _, group := range groups {
			if plan != nil {
				all, any := plan.heardState(group.SegmentIndexes)
				if all {
					continue
				}
				if any {
					trace.OrNil(s.logger).Event("turn.audio_partial", map[string]interface{}{
						"indexes": group.SegmentIndexes,
					})
					continue
				}
			}
			if group.Cached {
				emit(s.clipPath(group.Key))
				plan.markHeard(group.SegmentIndexes)
				continue
			}
			for _, index := range group.SegmentIndexes {
				clips, err := s.synthesizeSegment(ctx, gameID, turn, index, false)
				if err != nil {
					continue
				}
				for _, clip := range clips {
					emit(clip)
				}
			}
			plan.markHeard(group.SegmentIndexes)
		}
		return
	}
	for i := range turn.Segments {
		if plan != nil && plan.heardAll([]int{i}) {
			continue
		}
		clips, err := s.synthesizeSegment(ctx, gameID, turn, i, force)
		if err != nil {
			continue
		}
		for _, clip := range clips {
			emit(clip)
		}
		plan.markHeard([]int{i})
	}
}

// recordTTSUsage records what a synthesis consumed. A cache hit reports nothing,
// so only a real synthesis is recorded.
func (s *Service) recordTTSUsage(gameID string, turnNumber int, cfg *config.Config, pipeline *media.TTSPipeline) {
	key, ok := media.TTSKeyFor(cfg.Media.TTS)
	if !ok {
		return
	}
	if u := pipeline.LastUsage(); u.Characters != 0 || u.InputTokens != 0 || u.OutputTokens != 0 || u.Requests != 0 {
		s.RecordUsage(gameID, turnNumber, "tts", mediaUsage(u, key, cfg.Media.TTS.Model))
	}
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
	pipeline.SetGroupCaps(media.TurnGroupCaps(cfg, media.ResolveGroupCaps(cfg.Media.TTS, client)))
	pipeline.SetSpeechCues(media.ResolveSpeechCueCapabilities(cfg.Media.TTS, client))
	pipeline.SetAudioTagDelivery(cfg.Media.TTS.SpeechCues.AudioTags != nil && *cfg.Media.TTS.SpeechCues.AudioTags)
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
		player.SetOnComplete(func() {
			s.audioSubMu.Lock()
			status := AudioStatusDTO{Available: true, Playing: false, Turn: s.audioTurn, Segment: s.audioSegment}
			s.audioSubMu.Unlock()
			s.broadcastAudioStatus(status)
		})
		s.player = player
	})
	return s.player
}

// SubscribeAudioStatus registers a channel notified when application playback
// ends, so a client can advance on a real completion rather than polling.
func (s *Service) SubscribeAudioStatus() (<-chan AudioStatusDTO, func()) {
	ch := make(chan AudioStatusDTO, 1)
	s.audioSubMu.Lock()
	if s.audioSubs == nil {
		s.audioSubs = make(map[chan AudioStatusDTO]struct{})
	}
	s.audioSubs[ch] = struct{}{}
	s.audioSubMu.Unlock()
	return ch, func() {
		s.audioSubMu.Lock()
		delete(s.audioSubs, ch)
		s.audioSubMu.Unlock()
	}
}

func (s *Service) broadcastAudioStatus(status AudioStatusDTO) {
	s.audioSubMu.Lock()
	defer s.audioSubMu.Unlock()
	for ch := range s.audioSubs {
		select {
		case ch <- status:
		default:
		}
	}
}

// setAudioCurrent records the beat a queue is playing, so its completion event
// carries the identity a client needs to advance the right beat. Segment is -1
// for a whole-turn queue.
func (s *Service) setAudioCurrent(turn, segment int) {
	s.audioSubMu.Lock()
	s.audioTurn, s.audioSegment = turn, segment
	s.audioSubMu.Unlock()
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
	pipeline.SetGroupCaps(media.TurnGroupCaps(cfg, media.ResolveGroupCaps(cfg.Media.TTS, client)))
	pipeline.SetSpeechCues(media.ResolveSpeechCueCapabilities(cfg.Media.TTS, client))
	pipeline.SetAudioTagDelivery(cfg.Media.TTS.SpeechCues.AudioTags != nil && *cfg.Media.TTS.SpeechCues.AudioTags)

	voiceFor := s.voiceFor(gameID)
	grouped := s.groupingEnabled(cfg)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		if grouped {
			groups := pipeline.GroupClipKeys(turn.Segments, narratorVoice, voiceFor)
			turnCached, turnUncached := pipeline.CountUncachedGroups(groups)
			cached += turnCached
			uncached += turnUncached
			continue
		}
		turnCached, turnUncached := pipeline.CountUncached(turn.Segments, narratorVoice, voiceFor)
		cached += turnCached
		uncached += turnUncached
	}
	return cached, uncached, nil
}

// TTSBatchJobs lists a campaign's offline batch jobs, newest first.
func (s *Service) TTSBatchJobs(gameID string) ([]TTSBatchJobDTO, error) {
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return nil, err
	}
	out := make([]TTSBatchJobDTO, 0, len(jobs))
	for _, job := range jobs {
		dto := ttsBatchJobDTO(job)
		dto.GameID = gameID
		out = append(out, dto)
	}
	return out, nil
}

// AllTTSBatchJobs lists every campaign's batch jobs, so the global manager can
// show and filter them by campaign.
func (s *Service) AllTTSBatchJobs(ctx context.Context) ([]TTSBatchJobDTO, error) {
	games, err := s.ListGames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TTSBatchJobDTO, 0)
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		jobs, err := store.ListTTSJobs(game.ID)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			dto := ttsBatchJobDTO(job)
			dto.GameID = game.ID
			dto.GameName = game.Name
			out = append(out, dto)
		}
	}
	return out, nil
}

// CancelTTSBatch cancels a submitted batch job and records it as cancelled,
// preserving the progress it had reached.
func (s *Service) CancelTTSBatch(ctx context.Context, gameID, jobID string) error {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return fmt.Errorf("the configured TTS provider has no batch API")
	}
	if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: jobID}); err != nil {
		return err
	}
	store, err := s.store(gameID)
	if err != nil {
		return err
	}
	if job, err := store.GetTTSJob(jobID); err == nil && job != nil {
		return store.UpdateTTSJobStatus(jobID, "cancelled", job.Completed, job.FailedKeys)
	}
	return store.UpdateTTSJobStatus(jobID, "cancelled", 0, nil)
}

// DeleteTTSBatch removes a finished batch job. An in-flight job must be
// cancelled first, so a delete never hides work that is still running.
func (s *Service) DeleteTTSBatch(ctx context.Context, gameID, jobID string) error {
	store, err := s.store(gameID)
	if err != nil {
		return err
	}
	job, err := store.GetTTSJob(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	if batchJobActive(*job) {
		return fmt.Errorf("batch job %s is still running; cancel it first", jobID)
	}
	return store.DeleteTTSJob(jobID)
}

// ClearTTSBatch removes every finished batch job, for one campaign or, when
// gameID is empty, for all of them. It returns how many jobs it removed.
func (s *Service) ClearTTSBatch(ctx context.Context, gameID string) (int, error) {
	if gameID != "" {
		store, err := s.store(gameID)
		if err != nil {
			return 0, err
		}
		return store.DeleteFinishedTTSJobs(gameID)
	}

	games, err := s.ListGames(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		removed, err := store.DeleteFinishedTTSJobs(game.ID)
		if err != nil {
			continue
		}
		total += removed
	}
	return total, nil
}

// ResumeTTSBatch finishes a job now: it polls the provider and, once the job is
// done, downloads the output, converts it to Opus, and writes it to the cache.
// It is how a job that never stored its clips (an output file that arrived
// empty, an interrupted download) is completed without waiting for a restart.
func (s *Service) ResumeTTSBatch(ctx context.Context, gameID, jobID string) (*TTSBatchJobDTO, error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return nil, fmt.Errorf("the configured TTS provider has no batch API")
	}
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	job, err := store.GetTTSJob(jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("batch job %s not found", jobID)
	}

	engine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
	engine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: job.Provider, Model: job.Model}
	s.goBackground(func() {
		if _, err := engine.Resume(s.bgCtx, opts, jobID); err != nil {
			trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]interface{}{
				"game":  gameID,
				"job":   jobID,
				"error": err.Error(),
			})
			s.noteFailure("tts", err)
			return
		}
		s.noteSuccess("tts")
	})

	dto := ttsBatchJobDTO(*job)
	dto.GameID = gameID
	return &dto, nil
}

// StartTTSBatch submits an offline batch backfill for a campaign and finishes it
// in the background, so the request returns at once and the panel watches the
// job row. It returns the submitted job, or nil when every clip is already
// cached.
func (s *Service) StartTTSBatch(ctx context.Context, gameID string, force bool) (*TTSBatchJobDTO, error) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return nil, err
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return nil, fmt.Errorf("the configured TTS provider has no batch API")
	}
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}

	// Starting twice must not queue two jobs for the same work: a backfill already
	// in flight is returned as-is, so the button is idempotent.
	s.batchStartMu.Lock()
	defer s.batchStartMu.Unlock()
	if existing, ok := activeBatchJob(store, gameID); ok {
		dto := ttsBatchJobDTO(*existing)
		dto.GameID = gameID
		return &dto, nil
	}

	pipeline, err := s.audioPipeline()
	if err != nil {
		return nil, err
	}
	turns, err := s.cachedHistory(gameID)
	if err != nil {
		return nil, err
	}

	narrator := s.narratorVoiceFor(gameID, cfg)
	voiceFor := s.voiceFor(gameID)
	groups := make([]media.ClipGroup, 0)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		groups = append(groups, pipeline.GroupClipKeys(turn.Segments, narrator, voiceFor)...)
	}

	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}
	batchEngine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
	batchEngine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: providerKey, Model: cfg.Media.TTS.Model, Force: force}

	job, err := batchEngine.Submit(ctx, opts, groups)
	if err != nil {
		trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]interface{}{
			"game":     gameID,
			"provider": providerKey,
			"error":    err.Error(),
		})
		return nil, err
	}
	if job == nil {
		return nil, nil
	}

	// A batch job can take hours, so it finishes in the background and the panel
	// watches the job row rather than holding a request open. It runs on the
	// service's background context, so closing the app cancels the poll instead of
	// waiting it out; the job stays resumable and is picked up next launch.
	s.goBackground(func() {
		if _, err := batchEngine.Resume(s.bgCtx, opts, job.ID); err != nil {
			trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]interface{}{
				"game":     gameID,
				"job":      job.ID,
				"provider": providerKey,
				"error":    err.Error(),
			})
			s.noteFailure("tts", err)
			return
		}
		s.noteSuccess("tts")
	})

	dto := ttsBatchJobDTO(*job)
	return &dto, nil
}

// ResumePendingBatches finishes every unfinished batch job in the background, so
// a job started in an earlier session is collected on the next launch without
// the user waiting. It is safe to call once at startup; a provider with no batch
// API makes it a no-op.
func (s *Service) ResumePendingBatches(ctx context.Context) {
	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return
	}

	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}

	games, err := s.ListGames(ctx)
	if err != nil {
		return
	}
	for _, game := range games {
		store, err := s.store(game.ID)
		if err != nil {
			continue
		}
		jobs, err := store.ListTTSJobs(game.ID)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			if !batchJobActive(job) {
				continue
			}
			// A job belongs to the provider that created it, and a different
			// provider's client cannot poll it; leave it for when that provider is
			// selected again.
			if providerKey != "" && job.Provider != "" && job.Provider != providerKey {
				continue
			}
			opts := ttsbatch.Options{GameID: game.ID, Provider: job.Provider, Model: job.Model}
			engine := ttsbatch.New(batchClient, media.NewContentCache(s.resolver.CacheDir()), store)
			engine.SetOpusBitrate(cfg.OpusBitrate())
			trace.OrNil(s.logger).Event("media.tts.batch_resumed", map[string]interface{}{
				"game": game.ID,
				"job":  job.ID,
			})
			s.goBackground(func() {
				if _, err := engine.Resume(s.bgCtx, opts, job.ID); err != nil {
					trace.OrNil(s.logger).Event("media.tts.batch_error", map[string]interface{}{
						"game":  game.ID,
						"job":   job.ID,
						"error": err.Error(),
					})
					s.noteFailure("tts", err)
					return
				}
				s.noteSuccess("tts")
			})
		}
	}
}

// batchJobActive reports whether a job is still worth working on, so a start is
// not duplicated and a launch knows to resume it.
func batchJobActive(job storage.TTSJob) bool {
	switch job.Status {
	case "queued", "processing", "processed", "downloading", "storing", "submitted", "pending", "running":
		return true
	case "completed", "succeeded":
		// A job recorded as finished but short of its request count never stored
		// everything — an output file that arrived empty, or a download that was
		// interrupted — so it is still worth resuming.
		return job.RequestCount > 0 && job.Completed+len(job.FailedKeys) < job.RequestCount
	default:
		return false
	}
}

// activeBatchJob returns a campaign's in-flight batch job, if one exists.
func activeBatchJob(store *storage.Store, gameID string) (*storage.TTSJob, bool) {
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return nil, false
	}
	for i := range jobs {
		if batchJobActive(jobs[i]) {
			return &jobs[i], true
		}
	}
	return nil, false
}

// cancelBatchJobs stops a campaign's in-flight batch synthesis, best-effort, so a
// restart does not leave the provider working on narration that is about to be
// discarded. A job belongs to the provider that created it, so only jobs matching
// the selected provider are cancelled; a provider that cannot be reached, or a
// job from a provider that is no longer selected, is left for the reset to
// remove.
func (s *Service) cancelBatchJobs(ctx context.Context, gameID string) {
	store, err := s.store(gameID)
	if err != nil {
		return
	}
	jobs, err := store.ListTTSJobs(gameID)
	if err != nil {
		return
	}

	anyActive := false
	for i := range jobs {
		if batchJobActive(jobs[i]) {
			anyActive = true
			break
		}
	}
	if !anyActive {
		return
	}

	cfg := s.configMgr.Get()
	client, err := s.ttsClientFor(cfg.Media.TTS)
	if err != nil {
		return
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		return
	}

	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}

	for i := range jobs {
		job := jobs[i]
		if !batchJobActive(job) {
			continue
		}
		if providerKey != "" && job.Provider != "" && job.Provider != providerKey {
			continue
		}
		if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: job.ID}); err != nil {
			trace.OrNil(s.logger).Event("media.tts.batch_cancel_error", map[string]interface{}{
				"game":  gameID,
				"job":   job.ID,
				"error": err.Error(),
			})
		}
	}
}

// ttsBatchJobDTO maps a job row to the wire shape.
func ttsBatchJobDTO(job storage.TTSJob) TTSBatchJobDTO {
	return TTSBatchJobDTO{
		ID:           job.ID,
		Provider:     job.Provider,
		Model:        job.Model,
		Status:       job.Status,
		RequestCount: job.RequestCount,
		Completed:    job.Completed,
		FailedKeys:   job.FailedKeys,
		LastError:    job.LastError,
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

// PlayTurnAudio synthesizes any beat the turn has not already cached and plays
// the whole turn in order. Clips are content-addressed, so a replay is instant.
// It takes no context on purpose: narration belongs to the application, so the
// request that asked for it finishing must not silence it. StopAudio ends it.
func (s *Service) PlayTurnAudio(gameID string, turnNumber int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	if _, err := s.findTurn(gameID, turnNumber); err != nil {
		return err
	}

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	s.setAudioCurrent(turnNumber, -1)
	return player.PlayQueue(s.turnClipStream(gameID, turnNumber, len(force) > 0 && force[0]))
}

// turnClipStream yields a turn's clips in order, synthesizing each one as its
// predecessor is consumed, so playback starts on the first completed clip instead
// of waiting for the whole turn. The stream is deliberately not bound to the
// caller's context: a request that triggered narration finishing must not silence
// it, and the queue ends when the channel closes or the player is stopped.
func (s *Service) turnClipStream(gameID string, turnNumber int, force bool) <-chan string {
	clips := make(chan string)

	turn, err := s.findTurn(gameID, turnNumber)
	if err != nil {
		close(clips)
		return clips
	}

	go func() {
		defer close(clips)
		ctx := context.Background()
		s.emitTurnClips(ctx, gameID, *turn, force, func(path string) {
			clips <- path
		}, nil)
	}()

	return clips
}

// PlaySegmentAudio plays one beat, which is what a speaker chip triggers.
func (s *Service) PlaySegmentAudio(ctx context.Context, gameID string, turnNumber, segmentIndex int, force ...bool) error {
	player := s.audioPlayer()
	if player == nil || !player.Available() {
		return playback.ErrUnavailable
	}

	clips, err := s.GetSegmentClips(ctx, gameID, turnNumber, segmentIndex, force...)
	if err != nil {
		return err
	}

	player.SetVolume(s.configMgr.Get().Media.TTS.MasterVolume)
	s.setAudioCurrent(turnNumber, segmentIndex)
	return player.PlayFiles(clips)
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
	if err := pathutil.ValidateID(name); err != nil {
		return "", ""
	}
	assetsDir := filepath.Join(dir, "assets")
	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, ext := range exts {
		path, err := pathutil.ResolveSafeChild(assetsDir, name+ext)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			return path, ext
		}
	}
	return "", ""
}

// gameAssetSource resolves a campaign's banner or icon, preferring the
// campaign's own copy and falling back to the world's when it has none. The
// fallback is resolved at read time, so it copies nothing and a campaign that
// never had its own art still has none.
func (s *Service) gameAssetSource(gameDir, worldID, assetKind string) (string, string, bool) {
	if p, ext := findAssetFile(gameDir, assetKind); p != "" {
		return p, contentTypeForArt(ext), true
	}
	if strings.TrimSpace(worldID) == "" {
		return "", "", false
	}
	if p, ext := findAssetFile(s.resolver.WorldDir(worldID), assetKind); p != "" {
		return p, contentTypeForArt(ext), true
	}
	return "", "", false
}

// gameAssetURL returns the campaign route for a banner or icon, preferring the
// campaign's own asset and falling back to the world's, and reports which one it
// is ("campaign", "world", or "" when neither exists).
//
// The URL carries a token derived from the file it will serve. Without it the
// route is identical before and after an asset is generated or cleared, so a
// browser keeps showing the image it already cached and a new generation looks
// like it did nothing.
func (s *Service) gameAssetURL(gameDir, worldID, assetKind, gameID string) (string, string) {
	base := fmt.Sprintf("/api/game/%s/%s", gameID, assetKind)
	if p, _ := findAssetFile(gameDir, assetKind); p != "" {
		return versionedAssetURL(base, p), "campaign"
	}
	if strings.TrimSpace(worldID) != "" {
		if p, _ := findAssetFile(s.resolver.WorldDir(worldID), assetKind); p != "" {
			return versionedAssetURL(base, p), "world"
		}
	}
	return "", ""
}

// versionedAssetURL stamps an asset URL with its source file's size and
// modification time, so a changed file is a new URL.
func versionedAssetURL(base, path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return base
	}
	return fmt.Sprintf("%s?v=%d-%d", base, fi.ModTime().UnixNano(), fi.Size())
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

		bannerURL, bannerSource := s.gameAssetURL(gameDir, m.WorldID, "banner", gameID)
		iconURL, iconSource := s.gameAssetURL(gameDir, m.WorldID, "icon", gameID)

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
			BannerSource:    bannerSource,
			IconSource:      iconSource,
			PlayTimeSeconds: int64(turnCount * 120),
		})
	}

	// Most recently played first, so "Resume" is the campaign the player left.
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].LastPlayed > summaries[j].LastPlayed
	})
	return summaries, nil
}

// systemDraftsDir is where system drafts live: a dot-directory under systems/.
func (s *Service) systemDraftsDir() string {
	return filepath.Join(s.resolver.SystemsDir(), sysgen.DraftsDirName)
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
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}

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

// RestartGame returns a campaign to its opening state without disturbing its
// configuration: the timeline and the world's mutable cast are reset in place,
// while the manifest, the artwork, the narrator voice, the protagonist and the
// spend ledger are carried across untouched.
func (s *Service) RestartGame(ctx context.Context, gameID string) (*GameSummaryDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}

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

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open campaign store: %w", err)
	}

	// Stop any batch synthesis still speaking narration the reset is about to
	// discard, so the provider is not left working on it.
	s.cancelBatchJobs(ctx, gameID)

	if err := engine.ResetCampaign(s.resolver, store, manifest); err != nil {
		return nil, fmt.Errorf("reset campaign: %w", err)
	}

	name := manifest.Name
	if name == "" {
		name = gameID
	}
	playerName := manifest.PlayerName
	if playerName == "" {
		playerName = manifest.Player
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
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id: %w", err)
	}

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
		Mechanics:         m.Mechanics,
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
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id %q: %w", id, err)
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}
	script := req.Script
	if script == "" {
		script = defaultMechanicsScript
	}

	gate := sysgen.Gate(sysgen.System{ID: id, Script: script, Mechanics: req.Mechanics})
	if req.Strict && !gate.OK {
		return nil, fmt.Errorf("system failed the smoke test: %s", gate.FailureText())
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
		Mechanics:         req.Mechanics,
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

	detail, err := s.GetSystem(ctx, id)
	if err != nil {
		return nil, err
	}
	warnings := validateMechanics(req.Mechanics)
	if !gate.OK {
		warnings = append(warnings, "smoke test: "+gate.FailureText())
	}
	detail.Warnings = warnings
	return detail, nil
}

// ErrSystemNotFound reports an attempt to operate on a system that does not exist.
var ErrSystemNotFound = errors.New("system not found")

// ErrSystemInUse reports an attempt to delete a system that is still referenced by a campaign or world.
var ErrSystemInUse = errors.New("system is in use")

// DeleteSystem removes a system directory.
// When force is false, it refuses to delete if any campaign or world references this system.
func (s *Service) DeleteSystem(ctx context.Context, systemID string, force bool) error {
	if err := pathutil.ValidateID(systemID); err != nil {
		return fmt.Errorf("invalid system id: %w", err)
	}

	sysDir := s.resolver.SystemDir(systemID)
	if _, err := os.Stat(filepath.Join(sysDir, "system.yaml")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSystemNotFound, systemID)
		}
		return fmt.Errorf("system %q: %w", systemID, err)
	}

	if !force {
		games, err := s.ListGames(ctx)
		if err == nil {
			for _, g := range games {
				if g.SystemID == systemID {
					return fmt.Errorf("%w: campaign %q (%s)", ErrSystemInUse, g.Name, g.ID)
				}
			}
		}

		worlds, err := s.ListWorlds(ctx)
		if err == nil {
			for _, w := range worlds {
				for _, comp := range w.CompatibleSystems {
					if comp == systemID {
						return fmt.Errorf("%w: world %q (%s)", ErrSystemInUse, w.Name, w.ID)
					}
				}
			}
		}
	}

	if err := os.RemoveAll(sysDir); err != nil {
		return fmt.Errorf("remove system: %w", err)
	}
	return nil
}

// ListReferenceSystems returns the shipped starting systems, so the studio offers
// the same corpus the tests exercise.
func (s *Service) ListReferenceSystems(_ context.Context) (*ReferenceSystemsDTO, error) {
	systems := refsystems.List()
	out := &ReferenceSystemsDTO{Systems: make([]ReferenceSystemDTO, 0, len(systems))}
	for _, sys := range systems {
		out.Systems = append(out.Systems, ReferenceSystemDTO{
			ID:          sys.ID,
			Name:        sys.Name,
			Version:     sys.Version,
			Description: sys.Description,
			RulesPrompt: sys.RulesPrompt,
			Script:      sys.Script,
			Mechanics:   sys.Mechanics,
		})
	}
	return out, nil
}

// TestSystem runs scenarios against a system and returns the expectations that
// failed, so an authored system can be verified without a store or a provider.
func (s *Service) TestSystem(_ context.Context, req SystemTestRequestDTO) (*SystemTestResponseDTO, error) {
	sys := systemtest.System{ID: req.System.ID, Script: req.System.Script, Mechanics: req.System.Mechanics}
	failures := systemtest.RunAll(sys, req.Scenarios)
	out := &SystemTestResponseDTO{}
	for _, f := range failures {
		out.Failures = append(out.Failures, SystemTestFailureDTO{Scenario: f.Scenario, Step: f.Step, Detail: f.Detail})
	}
	return out, nil
}

// SystemScenarios reads a system's stored scenarios from its tests directory.
func (s *Service) SystemScenarios(_ context.Context, id string) (*SystemScenariosDTO, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid system id: %w", err)
	}
	dir := filepath.Join(s.resolver.SystemDir(id), "tests")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &SystemScenariosDTO{}, nil
	}
	out := &SystemScenariosDTO{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		scenario, err := systemtest.LoadScenario(data)
		if err != nil {
			return nil, fmt.Errorf("load scenario %q: %w", entry.Name(), err)
		}
		out.Scenarios = append(out.Scenarios, scenario)
	}
	return out, nil
}

func (s *Service) GetWorld(ctx context.Context, id string) (*WorldDetailDTO, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid world id: %w", err)
	}

	worldDir := s.resolver.WorldDir(id)
	m, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world manifest: %w", err)
	}

	entitiesDir := filepath.Join(worldDir, "entities")
	var entities []WorldEntitySummaryDTO
	_ = eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		// A world template is identified by its file name, unlike a campaign note:
		// templates are not indexed and are not linked by id, so taking the id from
		// the frontmatter here would rename every existing template the first time
		// it was saved.
		entID := strings.TrimSuffix(filepath.Base(path), ".md")
		name := entID
		entType := "concept"
		if ent, err := entity.ParseMarkdownEntity(data); err == nil {
			if ent.Name != "" {
				name = ent.Name
			}
			if ent.Type != "" {
				entType = ent.Type
			}
		}
		entities = append(entities, WorldEntitySummaryDTO{
			ID:     entID,
			Name:   name,
			Type:   entType,
			Folder: folder,
		})
		return nil
	})

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

// ErrWorldInUse reports an attempt to delete a world that is still referenced by one or more campaigns.
var ErrWorldInUse = errors.New("world is in use by a campaign")

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
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid world id %q: %w", id, err)
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

// DeleteWorld removes a world and all its entity templates, prompts, and assets.
// When force is false, it refuses to delete if any campaign references this world.
func (s *Service) DeleteWorld(ctx context.Context, worldID string, force bool) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}

	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(filepath.Join(worldDir, "world.yaml")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrWorldNotFound, worldID)
		}
		return fmt.Errorf("world %q: %w", worldID, err)
	}

	if !force {
		games, err := s.ListGames(ctx)
		if err == nil {
			for _, g := range games {
				if g.WorldID == worldID {
					return fmt.Errorf("%w: campaign %q (%s)", ErrWorldInUse, g.Name, g.ID)
				}
			}
		}
	}

	if err := os.RemoveAll(worldDir); err != nil {
		return fmt.Errorf("remove world: %w", err)
	}
	return nil
}

func (s *Service) GetWorldEntity(ctx context.Context, worldID, entityID string) (*WorldEntityDetailDTO, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return nil, fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return nil, fmt.Errorf("invalid entity id: %w", err)
	}

	worldDir := s.resolver.WorldDir(worldID)
	path, err := findWorldEntityNote(worldDir, entityID)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("read world entity %s: %w", entityID, fs.ErrNotExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world entity %s: %w", entityID, err)
	}
	folder := ""
	if rel, err := filepath.Rel(filepath.Join(worldDir, "entities"), filepath.Dir(path)); err == nil && rel != "." && rel != "" {
		folder = filepath.ToSlash(rel)
	}
	return &WorldEntityDetailDTO{
		ID:       entityID,
		Markdown: string(data),
		Folder:   folder,
	}, nil
}

// SaveWorldEntity writes a template into folder, moving it when it already lives
// somewhere else. World templates are not indexed, so there is no store to keep
// in step; the file is the record.
func (s *Service) SaveWorldEntity(ctx context.Context, worldID, entityID, folder, markdown string) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}
	cleanFolder, err := ValidateFolderPath(folder)
	if err != nil {
		return err
	}

	worldDir := s.resolver.WorldDir(worldID)
	entitiesDir := filepath.Join(worldDir, "entities")

	existingPath, err := findWorldEntityNote(worldDir, entityID)
	if err != nil {
		return err
	}

	targetDir := entitiesDir
	if cleanFolder != "" {
		targetDir = filepath.Join(entitiesDir, filepath.FromSlash(cleanFolder))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create world entities dir: %w", err)
	}

	targetPath, err := pathutil.ResolveSafeChild(targetDir, entityID+".md")
	if err != nil {
		return fmt.Errorf("resolve world entity path: %w", err)
	}
	if strings.Contains(targetPath, "..") {
		return fmt.Errorf("invalid world entity path: contains traversal")
	}
	if err := os.WriteFile(targetPath, []byte(markdown), 0o644); err != nil {
		return err
	}
	if existingPath != "" && existingPath != targetPath {
		if err := os.Remove(existingPath); err != nil {
			return fmt.Errorf("remove old template %q: %w", existingPath, err)
		}
	}
	return nil
}

func (s *Service) DeleteWorldEntity(ctx context.Context, worldID, entityID string) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}

	path, err := findWorldEntityNote(s.resolver.WorldDir(worldID), entityID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("delete world entity %s: %w", entityID, fs.ErrNotExist)
	}
	return os.Remove(path)
}

// findWorldEntityNote returns the path a world template occupies, or "" when it
// does not exist. A template is found by the id it declares or by its file name,
// so a hand-edited template that disagrees with itself is still reachable.
func findWorldEntityNote(worldDir, entityID string) (string, error) {
	entitiesDir := filepath.Join(worldDir, "entities")
	found := ""
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		declared := entity.DeclaredID(data)
		if filenameID == entityID || declared == entityID {
			found = path
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return found, nil
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
		Warnings:        s.configMgr.Warnings(),
		AppVersion:      s.appVersion(),
	}, nil
}

// appVersion reports the build version recorded by SetVersion.
func (s *Service) appVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

func (s *Service) SaveSettings(ctx context.Context, cfg config.Config) (*SettingsResponseDTO, error) {
	for _, p := range []string{cfg.Paths.Systems, cfg.Paths.Worlds, cfg.Paths.Games, cfg.Paths.Cache} {
		if p != "" {
			if _, err := pathutil.ValidateUserPath(p); err != nil {
				return nil, fmt.Errorf("invalid path %q: %w", p, err)
			}
		}
	}
	if err := s.validateVoiceOptionsInConfig(&cfg); err != nil {
		return nil, fmt.Errorf("validate tts options: %w", err)
	}
	s.applyResolvedPaths(&cfg)
	if err := s.configMgr.Save(&cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	// The named media registries cache clients built from the old configuration,
	// so drop them and let the next use rebuild from the new one.
	s.invalidateRegistries()
	// A style pack change takes effect with the next render, and an invalid pack is
	// reported rather than failing the save.
	s.applyStylePack()

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
		Warnings:        s.configMgr.Warnings(),
	}, nil
}

func (s *Service) ApplyOfflinePreset(ctx context.Context, req OfflinePresetRequestDTO) (*OfflinePresetResponseDTO, error) {
	cfg, err := s.configMgr.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	changes := config.ApplyOfflinePreset(cfg, req.TTS)
	if changes == nil {
		changes = []string{}
	}
	if _, err := s.SaveSettings(ctx, *cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}
	return &OfflinePresetResponseDTO{
		Changes: changes,
	}, nil
}

func (s *Service) CheckOffline(ctx context.Context) (provider.OfflineReport, error) {
	cfg := s.configMgr.Get()
	if cfg == nil {
		var err error
		cfg, err = s.configMgr.Load()
		if err != nil {
			return provider.OfflineReport{}, fmt.Errorf("load config: %w", err)
		}
	}
	return config.VerifyOffline(cfg), nil
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
		ttsKey, hasTTSKey := media.TTSKeyFor(ttsCfg)
		client, err := media.NewTTSClientWithSharedKey(ttsCfg, media.SharedProviderKey(cfg, ttsKey, hasTTSKey))
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
		if sttCfg.Type == "web-speech" {
			return &TestProviderResponseDTO{
				Success: false,
				Message: "web-speech runs in the browser; choose an HTTP or CLI Whisper provider",
			}, nil
		}
		key, hasKey := media.STTKeyFor(sttCfg)
		client, err := media.NewSTTClientWithSharedKey(sttCfg, media.SharedProviderKey(s.configMgr.Get(), key, hasKey))
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		text, err := client.Transcribe(ctx, media.GenerateToneWAV(440, 0.1))
		latency := time.Since(start).Milliseconds()
		if key, ok := media.STTKeyFor(sttCfg); ok {
			if reporter, ok := client.(media.UsageReporter); ok {
				s.RecordUsageGlobal("stt", mediaUsage(reporter.LastUsage(), key, sttCfg.Model))
			}
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
	if cfg.Media.STT.Type == "web-speech" {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: "the web-speech STT provider runs in the browser; configure an HTTP or CLI Whisper provider",
		}
	}

	sttKey, hasSTTKey := media.STTKeyFor(cfg.Media.STT)
	client, err := media.NewSTTClientWithSharedKey(cfg.Media.STT, media.SharedProviderKey(cfg, sttKey, hasSTTKey))
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
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", "", fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", "", os.ErrNotExist
	}
	worldID := ""
	if manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml")); err == nil {
		worldID = manifest.WorldID
	}
	if p, contentType, ok := s.gameAssetSource(gameDir, worldID, assetKind); ok {
		return p, contentType, nil
	}
	return "", "", os.ErrNotExist
}

func (s *Service) GetWorldAsset(worldID, assetKind string) (string, string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", "", fmt.Errorf("invalid asset kind: %w", err)
	}
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
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", fmt.Errorf("game not found: %w", err)
	}
	assetsDir := filepath.Join(gameDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := removeAssetFiles(assetsDir, assetKind); err != nil {
		return "", err
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath, err := pathutil.ResolveSafeChild(assetsDir, assetKind+ext)
	if err != nil {
		return "", fmt.Errorf("invalid asset path: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("write asset: %w", err)
	}
	return fmt.Sprintf("/api/game/%s/%s", gameID, assetKind), nil
}

// DeleteGameAsset removes a campaign's own banner or icon so it falls back to
// the world's artwork again. It is a no-op when the campaign has none of its own.
func (s *Service) DeleteGameAsset(gameID, assetKind string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return fmt.Errorf("game not found: %w", err)
	}
	return removeAssetFiles(filepath.Join(gameDir, "assets"), assetKind)
}

// removeAssetFiles clears every extension an asset kind may have been stored
// under, so a replacement never leaves a second file behind. It removes only
// names the directory itself reports, so a crafted asset kind can never name a
// path: the kind is compared against each entry rather than joined onto one.
func removeAssetFiles(assetsDir, assetKind string) error {
	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read assets dir: %w", err)
	}

	exts := []string{".png", ".webp", ".jpg", ".jpeg", ".svg"}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		matched := false
		for _, ext := range exts {
			if name == assetKind+ext {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if err := os.Remove(filepath.Join(assetsDir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove asset %q: %w", name, err)
		}
	}
	return nil
}

// campaignArtDescriptionLimit caps the campaign-specific description so a long
// opening directive cannot crowd out the style and composition instructions.
const campaignArtDescriptionLimit = 400

// campaignArtPrompt describes a campaign for its own banner or icon, so the
// generated art is about this campaign rather than a copy of its world's. It
// draws on the campaign's own material -- where it opens, its opening directive,
// and the protagonist -- while the world still supplies the art style and a
// grounding name.
func (s *Service) campaignArtPrompt(gameID, kind string) string {
	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil || manifest == nil {
		return buildAssetPrompt(kind, gameID, "", "", "")
	}

	artStyle, worldName := "", ""
	if wm, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml")); err == nil && wm != nil {
		artStyle = wm.ArtStyle
		worldName = wm.Name
	}

	parts := make([]string, 0, 4)
	if location := gameSettingString(manifest, engine.StartLocationSetting); location != "" {
		parts = append(parts, location)
	}
	if opening := engine.OpeningPrompt(manifest); opening != "" {
		parts = append(parts, opening)
	}
	if protagonist := s.protagonistArtSummary(gameDir, manifest); protagonist != "" {
		parts = append(parts, protagonist)
	}
	if worldName != "" {
		parts = append(parts, "set in "+worldName)
	}

	name := manifest.Name
	if name == "" {
		name = gameID
	}
	if len(parts) == 0 {
		// Nothing campaign-specific to say, so ground the prompt on the campaign
		// name alone rather than repeating the world.
		parts = append(parts, name)
	}

	description := harness.TruncateRunes(strings.Join(parts, ", "), campaignArtDescriptionLimit)
	return buildAssetPrompt(kind, name, description, artStyle, "")
}

// protagonistArtSummary names the protagonist and describes how they look, so a
// campaign's artwork can feature the character the player actually made.
func (s *Service) protagonistArtSummary(gameDir string, manifest *core.GameManifest) string {
	store, err := s.store(manifest.ID)
	if err != nil {
		return ""
	}
	playerID, err := engine.ResolvePlayerID(store, manifest)
	if err != nil || playerID == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gameDir, "entities", playerID+".md"))
	if err != nil {
		return ""
	}
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if ent.Name != "" {
		parts = append(parts, ent.Name)
	}
	if ent.Appearance != "" {
		parts = append(parts, ent.Appearance)
	}
	return strings.Join(parts, ", ")
}

// gameSettingString reads a string campaign setting, or "" when it is absent.
func gameSettingString(manifest *core.GameManifest, key string) string {
	if manifest == nil || manifest.Settings == nil {
		return ""
	}
	value, _ := manifest.Settings[key].(string)
	return strings.TrimSpace(value)
}

func (s *Service) SaveWorldAsset(worldID, assetKind string, data []byte, ext string) (string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(worldDir); err != nil {
		return "", fmt.Errorf("world not found: %w", err)
	}
	assetsDir := filepath.Join(worldDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := removeAssetFiles(assetsDir, assetKind); err != nil {
		return "", err
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath, err := pathutil.ResolveSafeChild(assetsDir, assetKind+ext)
	if err != nil {
		return "", fmt.Errorf("invalid asset path: %w", err)
	}
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
	// UsageToken defers the spend this preview incurs onto a campaign that is
	// still being created. Without it the preview is shared studio spend.
	UsageToken string `json:"usage_token,omitempty"`
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
	imgBytes, failure := s.generateImage(ctx, req.Kind, buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre), usageScope{token: req.UsageToken})
	if failure != nil {
		return nil, "", failure
	}
	return imgBytes, imageContentType(imgBytes), nil
}

func (s *Service) GenerateGameAsset(ctx context.Context, gameID string, req GenerateAssetRequestDTO) (string, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(req.Kind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	prompt := req.Prompt
	if prompt == "" {
		prompt = s.campaignArtPrompt(gameID, req.Kind)
	}
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, usageScope{gameID: gameID})
	if failure != nil {
		return "", failure
	}
	return s.SaveGameAsset(gameID, req.Kind, imgBytes, media.ArtExtension(imgBytes))
}

func (s *Service) GenerateWorldAsset(ctx context.Context, worldID string, req GenerateAssetRequestDTO) (string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(req.Kind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
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
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, usageScope{})
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

// ExportContent packs a world or system directory as a .lrpgpack archive and writes it to w.
func (s *Service) ExportContent(ctx context.Context, typ, id string, w io.Writer) (content.Manifest, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return content.Manifest{}, fmt.Errorf("invalid content ID: %w", err)
	}
	var dir string
	switch typ {
	case "world":
		dir = s.resolver.WorldDir(id)
	case "system":
		dir = s.resolver.SystemDir(id)
	default:
		return content.Manifest{}, fmt.Errorf("invalid content type %q: must be 'world' or 'system'", typ)
	}

	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return content.Manifest{}, fmt.Errorf("content %s %q not found", typ, id)
	}

	return content.Pack(dir, typ, content.ManifestMeta{}, w)
}

// ErrContentConflict reports an attempt to import content that already exists under refuse mode.
var ErrContentConflict = errors.New("content already exists")

// ImportContentOptions specifies optional options and validation when importing packages.
type ImportContentOptions struct {
	ConflictMode string
	Filename     string
	ExpectedType string
}

// ImportContent unpacks a package archive from r into a staging directory, validates its
// structure, resolves conflicts according to onConflict ("refuse", "rename", "overwrite"),
// and atomically moves it into the content directory.
func (s *Service) ImportContent(ctx context.Context, r io.Reader, onConflict string) (ImportResultDTO, error) {
	return s.ImportContentWithOptions(ctx, r, ImportContentOptions{ConflictMode: onConflict})
}

// ImportContentWithOptions unpacks a package archive from r with type validation and conflict handling.
func (s *Service) ImportContentWithOptions(ctx context.Context, r io.Reader, opts ImportContentOptions) (ImportResultDTO, error) {
	onConflict := opts.ConflictMode
	if onConflict == "" {
		onConflict = "refuse"
	}
	if onConflict != "refuse" && onConflict != "rename" && onConflict != "overwrite" {
		return ImportResultDTO{}, fmt.Errorf("invalid on_conflict mode %q: must be refuse, rename, or overwrite", onConflict)
	}

	stagingDir, err := os.MkdirTemp("", "lrpg-import-staging-*")
	if err != nil {
		return ImportResultDTO{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(stagingDir)
	}()

	m, sigBytes, err := content.Unpack(r, stagingDir)
	if err != nil {
		return ImportResultDTO{}, fmt.Errorf("unpack content: %w", err)
	}
	if err := pathutil.ValidateID(m.ID); err != nil {
		return ImportResultDTO{}, fmt.Errorf("invalid package content ID %q: %w", m.ID, err)
	}

	if opts.ExpectedType != "" {
		expected := strings.ToLower(strings.TrimSpace(opts.ExpectedType))
		if expected != "world" && expected != "system" {
			return ImportResultDTO{}, fmt.Errorf("invalid expected content type %q: must be 'world' or 'system'", expected)
		}
		if m.Type != expected {
			return ImportResultDTO{}, fmt.Errorf("package contains %s %q, cannot import as %s", m.Type, m.ID, expected)
		}
	}

	if opts.Filename != "" {
		ext := strings.ToLower(filepath.Ext(opts.Filename))
		switch ext {
		case ".lrpgworld":
			if m.Type != "world" {
				return ImportResultDTO{}, fmt.Errorf("package filename %q has extension .lrpgworld but package contains %s %q", opts.Filename, m.Type, m.ID)
			}
		case ".lrpgsystem":
			if m.Type != "system" {
				return ImportResultDTO{}, fmt.Errorf("package filename %q has extension .lrpgsystem but package contains %s %q", opts.Filename, m.Type, m.ID)
			}
		case ".lrpgpack":
			// Universal package format, allowed for both
		}
	}

	var trustedPublishers map[string]string
	if s.configMgr != nil {
		if cfg := s.configMgr.Get(); cfg != nil {
			trustedPublishers = cfg.Publishers
		}
	}

	trust, verifyErr := content.Verify(m, sigBytes, trustedPublishers)
	if verifyErr != nil || trust.State == "invalid" {
		return ImportResultDTO{Trust: trust}, fmt.Errorf("package signature is invalid: %w", verifyErr)
	}

	// Validate staged content structure
	var wm *core.WorldManifest
	var sm *core.SystemManifest
	switch m.Type {
	case "world":
		var loadErr error
		wm, loadErr = core.LoadWorldManifest(filepath.Join(stagingDir, "world.yaml"))
		if loadErr != nil {
			return ImportResultDTO{}, fmt.Errorf("invalid world package: missing or invalid world.yaml: %w", loadErr)
		}
		if wm.ID != m.ID {
			return ImportResultDTO{}, fmt.Errorf("world.yaml id %q does not match package id %q", wm.ID, m.ID)
		}
	case "system":
		var loadErr error
		sm, loadErr = core.LoadSystemManifest(filepath.Join(stagingDir, "system.yaml"))
		if loadErr != nil {
			return ImportResultDTO{}, fmt.Errorf("invalid system package: missing or invalid system.yaml: %w", loadErr)
		}
		if sm.ID != m.ID {
			return ImportResultDTO{}, fmt.Errorf("system.yaml id %q does not match package id %q", sm.ID, m.ID)
		}
	default:
		return ImportResultDTO{}, fmt.Errorf("invalid content type %q", m.Type)
	}

	// Detect whether package contains any executable script
	hasScript := false
	_ = filepath.WalkDir(stagingDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".js") {
			hasScript = true
			return fs.SkipAll
		}
		return nil
	})

	targetDirFor := func(id string) string {
		cleanID := filepath.Base(pathutil.SanitizeID(id))
		if m.Type == "world" {
			return filepath.Join(s.resolver.WorldsDir(), cleanID)
		}
		return filepath.Join(s.resolver.SystemsDir(), cleanID)
	}

	targetDir := targetDirFor(m.ID)
	finalID := m.ID
	action := "installed"

	if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
		switch onConflict {
		case "refuse":
			return ImportResultDTO{}, fmt.Errorf("%w: content %s %q already exists", ErrContentConflict, m.Type, m.ID)
		case "rename":
			n := 2
			for {
				candidate := fmt.Sprintf("%s-%d", m.ID, n)
				if _, err := os.Stat(targetDirFor(candidate)); os.IsNotExist(err) {
					finalID = candidate
					break
				}
				n++
			}
			// Rewrite manifest with new ID in staging
			if m.Type == "world" {
				wm.ID = finalID
				data, err := yaml.Marshal(wm)
				if err != nil {
					return ImportResultDTO{}, fmt.Errorf("marshal updated world manifest: %w", err)
				}
				if err := os.WriteFile(filepath.Join(stagingDir, "world.yaml"), data, 0644); err != nil {
					return ImportResultDTO{}, fmt.Errorf("write updated world manifest: %w", err)
				}
			} else if m.Type == "system" {
				sm.ID = finalID
				data, err := yaml.Marshal(sm)
				if err != nil {
					return ImportResultDTO{}, fmt.Errorf("marshal updated system manifest: %w", err)
				}
				if err := os.WriteFile(filepath.Join(stagingDir, "system.yaml"), data, 0644); err != nil {
					return ImportResultDTO{}, fmt.Errorf("write updated system manifest: %w", err)
				}
			}
			targetDir = targetDirFor(finalID)
			action = "renamed"
		case "overwrite":
			action = "overwritten"
		}
	}

	// Install: ensure parent dir exists
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return ImportResultDTO{}, fmt.Errorf("create content parent directory: %w", err)
	}

	if action == "overwritten" {
		// Temporary backup directory created directly under parentDir for atomic rollback.
		// lgtm[go/path-injection]
		backupDir, err := os.MkdirTemp(parentDir, ".backup-*")
		if err != nil {
			return ImportResultDTO{}, fmt.Errorf("create backup directory: %w", err)
		}
		_ = os.RemoveAll(backupDir)
		if err := os.Rename(targetDir, backupDir); err != nil {
			return ImportResultDTO{}, fmt.Errorf("backup existing content: %w", err)
		}
		swapSuccess := false
		defer func() {
			if !swapSuccess {
				_ = os.RemoveAll(targetDir)
				_ = os.Rename(backupDir, targetDir)
			} else {
				_ = os.RemoveAll(backupDir)
			}
		}()

		if err := os.Rename(stagingDir, targetDir); err != nil {
			return ImportResultDTO{}, fmt.Errorf("install content into %q: %w", targetDir, err)
		}
		swapSuccess = true
	} else {
		if err := os.Rename(stagingDir, targetDir); err != nil {
			return ImportResultDTO{}, fmt.Errorf("install content into %q: %w", targetDir, err)
		}
	}

	return ImportResultDTO{
		ID:          finalID,
		Name:        m.Name,
		Version:     m.Version,
		Type:        m.Type,
		Author:      m.Author,
		License:     m.License,
		Description: m.Description,
		FileCount:   len(m.Files),
		HasScript:   hasScript,
		Action:      action,
		Trust:       trust,
	}, nil
}

func (s *Service) registryClient() *registry.Client {
	var regCfg config.RegistriesConfig
	if s.configMgr != nil {
		if cfg := s.configMgr.Get(); cfg != nil {
			regCfg = cfg.Registries
		}
	}
	cacheDir := s.resolver.CacheDir()
	client := registry.NewClient(regCfg, cacheDir)

	client.SetInstaller(func(ctx context.Context, r io.Reader, conflictMode string) (content.Manifest, error) {
		res, err := s.ImportContent(ctx, r, conflictMode)
		if err != nil {
			return content.Manifest{}, err
		}
		return content.Manifest{
			ID:          res.ID,
			Name:        res.Name,
			Version:     res.Version,
			Type:        res.Type,
			Description: res.Description,
		}, nil
	})

	client.SetInstalledLister(func(ctx context.Context) ([]content.Manifest, error) {
		var manifests []content.Manifest

		// Scan worlds
		worldsDir := s.resolver.WorldsDir()
		if entries, err := os.ReadDir(worldsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					worldPath := filepath.Join(s.resolver.WorldDir(entry.Name()), "world.yaml")
					if wm, err := core.LoadWorldManifest(worldPath); err == nil && wm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      wm.ID,
							Name:    wm.Name,
							Version: wm.Version,
							Type:    "world",
						})
					}
				}
			}
		}

		// Scan systems
		systemsDir := s.resolver.SystemsDir()
		if entries, err := os.ReadDir(systemsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					sysPath := filepath.Join(s.resolver.SystemDir(entry.Name()), "system.yaml")
					if sm, err := core.LoadSystemManifest(sysPath); err == nil && sm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      sm.ID,
							Name:    sm.Name,
							Version: sm.Version,
							Type:    "system",
						})
					}
				}
			}
		}

		return manifests, nil
	})

	return client
}

// HandleRegistrySearch handles GET /api/registry/search?q=...
func (s *Service) HandleRegistrySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query().Get("q")
	client := s.registryClient()
	results, err := client.Search(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []registry.PackageRef{}
	}
	writeJSON(w, results)
}

// HandleRegistryInstall handles POST /api/registry/install
func (s *Service) HandleRegistryInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegistryInstallRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.OnConflict == "" {
		req.OnConflict = "refuse"
	}

	client := s.registryClient()
	var lastResult ImportResultDTO
	client.SetInstaller(func(ctx context.Context, r io.Reader, conflictMode string) (content.Manifest, error) {
		res, err := s.ImportContent(ctx, r, conflictMode)
		if err != nil {
			return content.Manifest{}, err
		}
		lastResult = res
		return content.Manifest{
			ID:          res.ID,
			Name:        res.Name,
			Version:     res.Version,
			Type:        res.Type,
			Description: res.Description,
		}, nil
	})

	m, err := client.Install(r.Context(), req.Ref, req.OnConflict)
	if err != nil {
		if errors.Is(err, ErrContentConflict) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if lastResult.ID == "" {
		lastResult = ImportResultDTO{
			ID:          m.ID,
			Name:        m.Name,
			Version:     m.Version,
			Type:        m.Type,
			Description: m.Description,
			Action:      "installed",
		}
	}
	writeJSON(w, lastResult)
}

// HandleRegistryUpdates handles GET /api/registry/updates
func (s *Service) HandleRegistryUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	client := s.registryClient()
	updates, err := client.Update(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if updates == nil {
		updates = []registry.PackageRef{}
	}
	writeJSON(w, updates)
}
