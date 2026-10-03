package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

// BuildScenePrompt composes the generation prompt for a turn scene illustration.
func BuildScenePrompt(visualCue string, location *entity.Entity, worldStyle string) string {
	parts := make([]string, 0, 4)
	if cue := strings.TrimSpace(visualCue); cue != "" {
		parts = append(parts, cue)
	}
	if location != nil {
		if locName := strings.TrimSpace(location.Name); locName != "" {
			parts = append(parts, "location: "+locName)
		}
		if appearance := strings.TrimSpace(location.Appearance); appearance != "" {
			parts = append(parts, appearance)
		}
	}
	if style := strings.TrimSpace(worldStyle); style != "" {
		parts = append(parts, style)
	}
	parts = append(parts, "cinematic scene illustration, high quality, atmospheric lighting, detailed environment, no text, no borders")
	return strings.Join(parts, ", ")
}

// ExtractSceneCue extracts a concise visual cue from narration text following a scene break delimiter.
func ExtractSceneCue(narration string) string {
	lines := strings.Split(narration, "\n")
	foundRule := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !foundRule {
			if trimmed == "---" || trimmed == "***" || trimmed == "___" {
				foundRule = true
			}
			continue
		}
		if trimmed != "" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	// Fallback to first non-empty line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "---" && trimmed != "***" && trimmed != "___" {
			if len(trimmed) > 200 {
				return trimmed[:200]
			}
			return trimmed
		}
	}
	return ""
}

// SceneGenerator abstracts image generation for scene illustrations.
type SceneGenerator interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

// SceneWorker manages asynchronous queued scene illustration generation.
type SceneWorker struct {
	mu        sync.Mutex
	resolver  *core.PathResolver
	generator SceneGenerator
	inFlight  map[string]bool
	onReady   func(gameID string, turnNumber int, relPath string)
}

// NewSceneWorker creates a new SceneWorker.
func NewSceneWorker(resolver *core.PathResolver, gen SceneGenerator) *SceneWorker {
	return &SceneWorker{
		resolver:  resolver,
		generator: gen,
		inFlight:  make(map[string]bool),
	}
}

// SetOnReady registers a callback invoked when a scene image has been generated and saved.
func (w *SceneWorker) SetOnReady(fn func(gameID string, turnNumber int, relPath string)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onReady = fn
}

// Enqueue asynchronously triggers scene image generation for a turn if not already in flight.
func (w *SceneWorker) Enqueue(gameID string, turnNumber int, prompt string) {
	if w == nil || w.generator == nil || gameID == "" || turnNumber <= 0 || strings.TrimSpace(prompt) == "" {
		return
	}

	key := fmt.Sprintf("%s:%d", gameID, turnNumber)
	w.mu.Lock()
	if w.inFlight[key] {
		w.mu.Unlock()
		return
	}
	w.inFlight[key] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.inFlight, key)
			w.mu.Unlock()
		}()

		_, _ = w.writeScene(context.Background(), gameID, turnNumber, prompt)
	}()
}

func (w *SceneWorker) writeScene(ctx context.Context, gameID string, turnNumber int, prompt string) (string, error) {
	imgBytes, err := w.generator.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate scene image: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("scene generator returned no image")
	}

	ext := media.ArtExtension(imgBytes)
	if ext == "" {
		ext = ".png"
	}

	filename := fmt.Sprintf("turn-%d%s", turnNumber, ext)
	relPath := filepath.Join("assets", "scenes", filename)
	fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("create scene dir: %w", err)
	}
	if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write scene image: %w", err)
	}

	w.mu.Lock()
	cb := w.onReady
	w.mu.Unlock()
	if cb != nil {
		cb(gameID, turnNumber, relPath)
	}
	return relPath, nil
}
