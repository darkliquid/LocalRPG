package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

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
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/all"
	"github.com/darkliquid/localrpg/pkg/scene"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
	"github.com/darkliquid/localrpg/pkg/turnstream"
)

// Service is the whole surface the desktop window, the HTTP routes and the socket
// share: campaigns, worlds, systems, entities, settings, media and models. It owns
// the process's resolution of the configuration and one index per campaign it has
// opened, and every exported method here is reachable from a route.
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
	// playback is the session's playback ledger and owner: which clips the
	// player has heard, and how far, and whether the device or the browser plays
	// them. It is in memory only; a fresh session has heard nothing.
	playback playbackState
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

// imageRegistry returns the service's named image registry, building it lazily.
func (s *Service) imageRegistry() *media.ImageRegistry {
	s.imageRegMu.Lock()
	defer s.imageRegMu.Unlock()
	if s.imageReg == nil {
		s.imageReg = media.NewImageRegistry(s.configMgr.Get(), s.logger)
	}
	return s.imageReg
}

// NewService resolves the configuration and the directories the service works in,
// and returns it ready to use. A rootDir of anything but "." puts the process in
// project mode: <rootDir>/config.yaml is then the user configuration and a
// relative paths.* value resolves against rootDir rather than the working
// directory. The manager's own load error is deliberately not returned, because
// the app is meant to open so the file can be fixed in Settings; it is in
// Warnings instead.
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
			trace.OrNil(svc.logger).Event("paths.legacy_relative", map[string]any{"detail": warning})
		}
	}
	for _, problem := range mgr.Warnings() {
		trace.OrNil(svc.logger).Event("config.problem", map[string]any{"problem": problem})
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

// GetResolver returns the path resolver the service was built with, so a caller
// resolves the same directories the app writes to instead of deriving its own.
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

// storeOrNil opens a campaign's index, returning nil rather than an error so a
// caller that can fall back does not have to branch on the error value.
func (s *Service) storeOrNil(gameID string) *storage.Store {
	store, err := s.store(gameID)
	if err != nil {
		return nil
	}
	return store
}

// arcProgress reads an arc's clock. A note may express it as `progress: 3/6` or
// as separate `clock_ticks`/`clock_max` fields, and both are honoured.
func arcProgress(raw map[string]any) (int, int) {
	if ticks, maxTicks, ok := stateFraction(raw, "progress"); ok {
		return ticks, maxTicks
	}
	ticks := stateNumber(raw, "clock_ticks", "ticks", "progress")
	maxTicks := stateNumber(raw, "clock_max", "max_ticks", "max", "max_progress")
	return ticks, maxInt(maxTicks, 1)
}

func clockTicks(raw map[string]any) (int, int) {
	ticks := stateNumber(raw, "clock_ticks", "ticks")
	maxTicks := stateNumber(raw, "clock_max", "max_ticks", "max")
	return ticks, maxInt(maxTicks, 1)
}

// stateFraction reads an "a/b" style value.
func stateFraction(raw map[string]any, key string) (int, int, bool) {
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
func stateNumber(raw map[string]any, keys ...string) int {
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

func stateString(raw map[string]any, key string) string {
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

// folderIsEmpty reports whether a directory holds anything, so a non-recursive
// delete can refuse instead of destroying notes.
func folderIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read folder %q: %w", dir, err)
	}
	return len(entries) == 0, nil
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

// errNoURLOpener means the service is running without a desktop window, so it
// cannot hand a link to the system browser. The frontend opens a tab instead.
var errNoURLOpener = errors.New("no url opener available")

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

// turnEmitter serialises one turn's events onto the wire. Tool activity arrives
// from the orchestrator while a streamer worker announces a finished sentence, and
// the wire is one newline-delimited stream, so every emission passes this lock.
type turnEmitter struct {
	mu   sync.Mutex
	emit func(TurnEvent) error
}

func newTurnEmitter(emit func(TurnEvent) error) *turnEmitter {
	return &turnEmitter{emit: emit}
}

func (e *turnEmitter) announce(event TurnEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.emit(event)
}

// Run plays one turn, emitting events as they happen. An emit failure cancels the
// turn, which is how a disconnected client stops generation rather than paying for
// a turn nobody will see.
func (t *TurnSession) Run(ctx context.Context, req TurnRequest, emit func(TurnEvent) error) error {
	runCtx, cancel := context.WithTimeout(ctx, t.cfg.TurnTimeout())
	defer cancel()

	emitter := newTurnEmitter(emit)

	// Tool activity is streamed as it happens, so a lookup reads as progress
	// rather than as a stall. A failed emit is ignored: the turn still records,
	// and a disconnected client is handled by the chunk listener below.
	t.orchestrator.SetToolObserver(func(activity engine.ToolActivity) {
		_ = emitter.announce(toolEvent(activity))
	})
	t.orchestrator.SetPendingCheckRef(req.PendingCheckRef)
	t.orchestrator.SetForcedTotal(req.ForcedTotal)
	t.orchestrator.SetManualDice(req.ManualDice)
	t.orchestrator.SetSingleTurnMode(t.cfg.InteractiveRolls() == "single-turn")
	t.orchestrator.SetImageTrigger(t.cfg.ImageTrigger())

	turnNumber := t.nextTurnNumber()
	plan, audioEnabled := t.startTurnAudio(turnNumber)
	streamer, streamedKeys := t.startTurnStreamer(runCtx, emitter, plan, turnNumber)
	defer streamer.Close()

	removePortrait := t.service.addPortraitListener(t.gameID, func(evt TurnEvent) {
		_ = emitter.announce(evt)
	})
	defer removePortrait()

	t.observeTurnSegments(streamer, emitter)

	t.orchestrator.SetSceneOnly(req.SceneOnly)
	turn, err := t.orchestrator.ProcessActionStream(runCtx, req.Mode, req.Input, func(text string) error {
		return emitter.announce(TurnEvent{Type: "chunk", Text: text})
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

	if err := emitter.announce(TurnEvent{Type: "turn", Turn: &dto}); err != nil {
		return err
	}

	t.announceMissingTTSModel(emitter)

	// The rest of the turn's clips are synthesized behind the turn and appended to
	// the same queue, so playback continues without a second start and a clip that
	// failed mid-stream is retried here. The played set makes the handover exact.
	// Start memory summarization and character enrichment / portrait generation
	// in the background as soon as the turn is recorded.
	t.service.summariseBehind(t.gameID, t.chronicler)
	t.service.goBackground(func() { t.service.scanAndEnrichCharacters(t.gameID, t.store) })

	t.drainTurnAudio(streamer, *turn, plan, streamedKeys, audioEnabled)
	return nil
}

// nextTurnNumber is the number the turn about to be recorded will carry.
func (t *TurnSession) nextTurnNumber() int {
	if max, err := t.store.MaxTurnNumber(); err == nil && max >= 0 {
		return max + 1
	}
	return 1
}

// startTurnAudio opens the turn's clip plan and, when a player is available, the
// application playback queue. It reports whether audio is configured at all, which
// is what decides whether the finalise pass runs.
//
// Playback runs on one queue opened before generation: a sentence the streamer
// synthesizes is heard as soon as it lands, and the finalise pass adds only what
// the stream has not already played. A session with no player still warms the
// clips, because the URLs a client is handed are content-addressed and cannot
// synthesize on demand.
func (t *TurnSession) startTurnAudio(turnNumber int) (*turnAudioPlan, bool) {
	audioEnabled := t.cfg.Media.TTS.Type != "" && t.cfg.Media.TTS.Type != "disabled"
	plan := newTurnAudioPlan(nil)
	plan.ledger = t.service.turnLedger(t.gameID, turnNumber)
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
	return plan, audioEnabled
}

// startTurnStreamer builds the pre-synthesiser, wires the voice each speaker is
// heard in, and returns it with the set that records the clip keys it emitted, so
// the finalise pass can assert they are the keys the plan contains.
//
// Sentences are synthesized while the model is still writing, so a finished segment
// whose text is one of them is a cache hit at finalise rather than a second
// provider call. The streamer is nil when streaming is disabled or no provider is
// configured.
func (t *TurnSession) startTurnStreamer(ctx context.Context, emitter *turnEmitter, plan *turnAudioPlan, turnNumber int) (*sentenceStreamer, map[string]bool) {
	streamedKeys := map[string]bool{}
	streamer := t.service.sentenceStreamerFor(ctx, t.gameID, t.cfg, func(speech provisionalSpeech) {
		if speech.AudioKey != "" {
			streamedKeys[speech.AudioKey] = true
		}
		plan.markHeard(speech.Segments)
		plan.enqueueClip(speech.AudioKey, t.service.clipPath(speech.AudioKey))
		_ = emitter.announce(speechEvent(speech))
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
	streamer.SetTurnNumber(turnNumber)
	streamer.SetOwner(t.service.PlaybackOwner())
	streamer.SetProgressObserver(func(progress AudioProgressDTO) {
		progress.Owner = t.service.PlaybackOwner()
		_ = emitter.announce(TurnEvent{Type: "audio_progress", AudioProgress: &progress, Owner: t.service.PlaybackOwner()})
	})
	return streamer, streamedKeys
}

// observeTurnSegments announces parsed segments as they arrive, so the client
// renders attributed speech while the model is still writing, and the streamer
// voices each one in its speaker's own voice. A failed emit is ignored, exactly as
// tool activity is: the turn still records.
//
// segmentIndex is the position of the event in the turn's segment order, which is
// the same index space the clip plan uses, so the heard ledger and the plan agree
// on what a segment is.
func (t *TurnSession) observeTurnSegments(streamer *sentenceStreamer, emitter *turnEmitter) {
	segmentIndex := 0
	t.orchestrator.SetSegmentObserver(func(event turnstream.Event) {
		segment, isSegment := liveSegmentDTO(event, t.gameID)
		if isSegment {
			if segment.SpeakerID != "" && t.service.hasCustomPortrait(t.gameID, segment.SpeakerID) {
				segment.HasCustomPortrait = true
			}
			_ = emitter.announce(TurnEvent{Type: "segment", Segment: &segment})
		}
		streamer.FeedSegment(event, segmentIndex)
		if isSegment {
			segmentIndex++
		}
	})
}

// announceMissingTTSModel tells the client when built-in TTS is configured and its
// model weights are missing, so the user can be prompted to download the voice
// pack.
func (t *TurnSession) announceMissingTTSModel(emitter *turnEmitter) {
	if t.cfg.Media.TTS.Type == "builtin" && (t.cfg.Media.TTS.BuiltinName == "sherpa-onnx" || t.cfg.Media.TTS.BuiltinName == "kokoro") {
		status := t.service.modelsManager.Status("kokoro-tts")
		if !status.Installed {
			_ = emitter.announce(TurnEvent{
				Type:    "model_missing",
				ModelID: "kokoro-tts",
				Name:    status.Name,
				Size:    status.TotalBytes,
			})
		}
	}
}

// drainTurnAudio flushes the turn's remaining clips once it is recorded: it waits
// for the in-flight synthesis jobs and their progress events, synthesizes behind
// the turn what the stream did not reach, and records a parity mismatch between the
// keys the streamer emitted and the ones the plan holds. The plan closes either
// way.
func (t *TurnSession) drainTurnAudio(streamer *sentenceStreamer, turn engine.Turn, plan *turnAudioPlan, streamedKeys map[string]bool, audioEnabled bool) {
	streamer.Close()
	if audioEnabled {
		streamer.Wait()
		t.finishTurnAudio(context.Background(), turn, plan)
		t.service.logParityMismatch(t.gameID, turn.Segments, streamedKeys)
	}
	plan.close()
}

// finishTurnAudio synthesizes every clip the turn still needs once it is recorded,
// appending to the plan only what the streamed sentences did not already play, so
// no line is heard twice and none is missed.
func (t *TurnSession) finishTurnAudio(ctx context.Context, turn engine.Turn, plan *turnAudioPlan) {
	if plan != nil && plan.ledger == nil {
		plan.ledger = t.service.turnLedger(t.gameID, turn.Number)
	}
	t.service.emitTurnClips(ctx, t.gameID, turn, false, func(clip string) {
		plan.enqueueClip(media.ClipKeyForPath(clip), clip)
	}, plan)
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
		var raw map[string]any
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

// versionedAssetURL stamps an asset URL with its source file's size and
// modification time, so a changed file is a new URL.
func versionedAssetURL(base, path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return base
	}
	return fmt.Sprintf("%s?v=%d-%d", base, fi.ModTime().UnixNano(), fi.Size())
}

func slugify(s string) string {
	id := entity.Slugify(s)
	if id == "" {
		return "campaign"
	}
	return id
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

// GetSettings returns the configuration in force, the file it was read from, and
// anything wrong with it. The paths in it are the resolved ones, so the editor
// shows where files actually live.
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

// SaveSettings writes the configuration and returns what is now in force. It
// validates the four path values, drops the media registries that were built from
// the old configuration, repoints the resolver and creates the directories, so a
// change takes effect without a restart.
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

// ApplyOfflinePreset switches the configuration to one that needs no network and
// returns what it changed. It saves through SaveSettings, so the preset is in
// force before it answers.
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

// CheckOffline reports what the configuration in force would need from the
// network, for the offline banner. It answers from the configuration already
// loaded when there is one, and loads it otherwise.
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

// gameSettingString reads a string campaign setting, or "" when it is absent.
func gameSettingString(manifest *core.GameManifest, key string) string {
	if manifest == nil || manifest.Settings == nil {
		return ""
	}
	value, _ := manifest.Settings[key].(string)
	return strings.TrimSpace(value)
}

// GenerateAssetRequestDTO asks for one campaign or world image. Kind names which
// asset it is, and an empty Prompt is filled in from the entity being illustrated.
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

// A registry source can clash with one already configured, or name one that is
// not, so the handler can answer 409 and 404 rather than a generic failure.
var (
	ErrRegistrySourceExists   = errors.New("registry source already configured")
	ErrRegistrySourceNotFound = errors.New("registry source not configured")
)

// ImportContentOptions specifies optional options and validation when importing packages.
type ImportContentOptions struct {
	ConflictMode string
	Filename     string
	ExpectedType string
}
