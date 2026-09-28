package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"

	"github.com/darkliquid/localrpg/pkg/export"
	"github.com/darkliquid/localrpg/pkg/scene"
)

var (
	// ErrExportInFlight reports that the campaign already has an export running.
	ErrExportInFlight = errors.New("an export is already running for this campaign")
	// ErrExportFormat reports an unsupported export format.
	ErrExportFormat = errors.New("export format must be web or video")
	// ErrExportNoFFmpeg reports that video export needs ffmpeg, which is absent.
	ErrExportNoFFmpeg = errors.New("ffmpeg is required for video export")
	// ErrExportDirRequired reports that no destination was provided.
	ErrExportDirRequired = errors.New("an export destination directory is required")
	// ErrNoNativeDialog reports that no native directory picker is available, so
	// the UI must fall back to a path field.
	ErrNoNativeDialog = errors.New("native directory dialog is not available")
)

// ExportRequestDTO asks for a story replay to be generated for a campaign. OutDir
// is the destination directory and is required: the server never guesses where a
// user's artifact should land.
type ExportRequestDTO struct {
	GameID string `json:"game_id"`
	Format string `json:"format"` // "web" | "video"
	OutDir string `json:"out_dir"`
	Art    bool   `json:"art"`
	Audio  bool   `json:"audio"`
	Still  bool   `json:"still,omitempty"`
	FPS    int    `json:"fps,omitempty"`
	Size   string `json:"size,omitempty"` // "1920x1080"
}

// ExportJobDTO describes an accepted export and where it will land.
type ExportJobDTO struct {
	GameID     string `json:"game_id"`
	Format     string `json:"format"`
	OutputPath string `json:"output_path,omitempty"`
	Running    bool   `json:"running"`
}

// ExportEvent is one progress update on the export stream.
type ExportEvent struct {
	GameID     string `json:"game_id"`
	Format     string `json:"format"`
	Phase      string `json:"phase"` // "compile","frames","encode","done","error","cancelled"
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Message    string `json:"message,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ExportCapabilitiesDTO reports what this machine can export and where the UI
// should start a destination picker.
type ExportCapabilitiesDTO struct {
	FFmpeg       bool   `json:"ffmpeg"`
	FFmpegPath   string `json:"ffmpeg_path,omitempty"`
	DefaultDir   string `json:"default_dir,omitempty"`
	NativeDialog bool   `json:"native_dialog"`
}

// exportJob is the running export for one campaign.
type exportJob struct {
	format string
	cancel context.CancelFunc
}

// exportManager serialises exports per campaign and fans progress out to
// subscribers. It mirrors the model-download manager's shape.
type exportManager struct {
	mu      sync.Mutex
	running map[string]*exportJob

	subMu sync.Mutex
	subs  map[chan ExportEvent]struct{}
}

func newExportManager() *exportManager {
	return &exportManager{
		running: map[string]*exportJob{},
		subs:    map[chan ExportEvent]struct{}{},
	}
}

// begin claims the campaign's export slot, or reports that one is running.
func (m *exportManager) begin(gameID, format string) (context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.running[gameID]; ok {
		return nil, ErrExportInFlight
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running[gameID] = &exportJob{format: format, cancel: cancel}
	return ctx, nil
}

func (m *exportManager) finish(gameID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, gameID)
}

func (m *exportManager) cancel(gameID string) {
	m.mu.Lock()
	job, ok := m.running[gameID]
	m.mu.Unlock()
	if ok {
		job.cancel()
	}
}

// publish sends an event to every subscriber, dropping it for a subscriber that
// is not keeping up rather than blocking the export.
func (m *exportManager) publish(event ExportEvent) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (m *exportManager) subscribe() chan ExportEvent {
	ch := make(chan ExportEvent, 32)
	m.subMu.Lock()
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()
	return ch
}

func (m *exportManager) unsubscribe(ch chan ExportEvent) {
	m.subMu.Lock()
	if _, ok := m.subs[ch]; ok {
		delete(m.subs, ch)
		close(ch)
	}
	m.subMu.Unlock()
}

// SetDirectoryPicker installs a native directory chooser, used only by the Wails
// desktop window. Without one the UI falls back to a path field.
func (s *Service) SetDirectoryPicker(picker func(defaultDir string) (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directoryPicker = picker
}

// defaultExportDir is where a picker should open: the XDG Videos folder, then
// Documents, then the home directory. Only existing directories are offered.
func (s *Service) defaultExportDir() string {
	for _, candidate := range []string{xdg.UserDirs.Videos, xdg.UserDirs.Documents} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// ChooseExportDirectory opens the native picker, or reports that none is
// available so the UI can fall back to a text field. An empty path with no error
// means the user cancelled.
func (s *Service) ChooseExportDirectory(ctx context.Context) (string, error) {
	s.mu.RLock()
	picker := s.directoryPicker
	s.mu.RUnlock()
	if picker == nil {
		return "", ErrNoNativeDialog
	}
	return picker(s.defaultExportDir())
}

// ExportCapabilities reports ffmpeg availability, the default destination, and
// whether a native picker exists.
func (s *Service) ExportCapabilities() ExportCapabilitiesDTO {
	path, ok := export.FFmpegAvailable()
	s.mu.RLock()
	native := s.directoryPicker != nil
	s.mu.RUnlock()
	return ExportCapabilitiesDTO{
		FFmpeg:       ok,
		FFmpegPath:   path,
		DefaultDir:   s.defaultExportDir(),
		NativeDialog: native,
	}
}

// SubscribeExportEvents registers a channel for export progress.
func (s *Service) SubscribeExportEvents() chan ExportEvent { return s.exports.subscribe() }

// UnsubscribeExportEvents releases a channel and closes it.
func (s *Service) UnsubscribeExportEvents(ch chan ExportEvent) { s.exports.unsubscribe(ch) }

// CancelExport stops the campaign's running export, if any.
func (s *Service) CancelExport(gameID string) { s.exports.cancel(gameID) }

// StartExport validates a request, claims the campaign's export slot, and runs
// the export in the background. The returned job names where the artifact will
// land; progress arrives on the event stream.
func (s *Service) StartExport(ctx context.Context, req ExportRequestDTO) (*ExportJobDTO, error) {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format != "web" && format != "video" {
		return nil, ErrExportFormat
	}
	if strings.TrimSpace(req.GameID) == "" {
		return nil, fmt.Errorf("export: game id is required")
	}
	outDir := strings.TrimSpace(req.OutDir)
	if outDir == "" {
		return nil, ErrExportDirRequired
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return nil, fmt.Errorf("export: resolve destination: %w", err)
	}
	if format == "video" {
		if _, ok := export.FFmpegAvailable(); !ok {
			return nil, ErrExportNoFFmpeg
		}
	}

	jobCtx, err := s.exports.begin(req.GameID, format)
	if err != nil {
		return nil, err
	}

	req.Format = format
	req.OutDir = absOut
	outPath := uniquePath(exportArtifactPath(absOut, req.GameID, format))

	s.goBackground(func() {
		defer s.exports.finish(req.GameID)
		s.runExport(jobCtx, req, outPath)
	})

	return &ExportJobDTO{GameID: req.GameID, Format: format, OutputPath: outPath, Running: true}, nil
}

// exportArtifactPath names the artifact inside a chosen directory: a bundle
// directory for the web player, a video file otherwise.
func exportArtifactPath(outDir, gameID, format string) string {
	if format == "web" {
		return filepath.Join(outDir, gameID+"-web")
	}
	return filepath.Join(outDir, gameID+".mp4")
}

// uniquePath appends a timestamp when a destination already exists, so an export
// never overwrites a previous one.
func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s-%s%s", base, time.Now().UTC().Format("20060102-150405"), ext)
}

// runExport compiles and renders one export, publishing every phase.
func (s *Service) runExport(ctx context.Context, req ExportRequestDTO, outPath string) {
	emit := func(p scene.Progress) {
		s.exports.publish(ExportEvent{
			GameID:  req.GameID,
			Format:  req.Format,
			Phase:   p.Phase,
			Done:    p.Done,
			Total:   p.Total,
			Message: p.Message,
		})
	}
	fail := func(err error) {
		phase := "error"
		if errors.Is(err, context.Canceled) {
			phase = "cancelled"
		}
		s.exports.publish(ExportEvent{
			GameID: req.GameID, Format: req.Format, Phase: phase, Error: err.Error(),
		})
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fail(fmt.Errorf("create export dir: %w", err))
		return
	}

	compiler := export.NewScriptCompilerWithResolver(s.GetResolver(), s.Config())
	compiler.SetMedia(req.Art, req.Audio)
	compiler.SetProgress(emit)

	script, err := compiler.Compile(ctx, req.GameID)
	if err != nil {
		fail(err)
		return
	}

	switch req.Format {
	case "web":
		if _, err := export.NewWebExporter(s.rootDir).Export(ctx, script, outPath); err != nil {
			fail(err)
			return
		}
	case "video":
		pipeline := export.NewVideoPipeline(s.rootDir)
		pipeline.SetStill(req.Still)
		if req.FPS > 0 {
			pipeline.SetFPS(req.FPS)
		}
		if width, height, ok := parseExportSize(req.Size); ok {
			pipeline.SetSize(width, height)
		}
		pipeline.SetProgress(emit)
		if err := pipeline.RenderVideo(ctx, script, outPath); err != nil {
			fail(err)
			return
		}
	}

	s.exports.publish(ExportEvent{
		GameID: req.GameID, Format: req.Format, Phase: "done", OutputPath: outPath,
	})
}

// parseExportSize reads a "WxH" size, rejecting anything below 16x16 so a typo
// falls back to the pipeline default rather than rendering garbage.
func parseExportSize(size string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(size), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width < 16 || height < 16 {
		return 0, 0, false
	}
	return width, height, true
}
