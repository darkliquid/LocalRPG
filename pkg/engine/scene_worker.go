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

// ScenePromptContext is everything a scene prompt is composed from, so the image
// reflects what happened rather than only where the player is.
type ScenePromptContext struct {
	Cue        string   // the extractor cue or rule-break paragraph, if any
	Narration  string   // the turn's narration, for an excerpt
	Action     string   // the player's raw action
	Entities   []string // present characters, by display name
	Location   string   // the location's name
	Appearance string   // the location's authored appearance, if any
	Style      string   // the world art style
	Outcome    string   // the resolved check's outcome, if any
}

const (
	sceneNarrationCap = 200
	sceneActionCap    = 160
	sceneEntityCap    = 4
	// sceneSuffix is the fixed quality suffix every scene prompt ends with.
	sceneSuffix = "cinematic scene illustration, high quality, atmospheric lighting, detailed environment, no text, no borders"
)

// BuildScenePrompt composes the generation prompt for a turn scene illustration
// from the turn's context, each part bounded so the provider is not asked to
// reconcile a page of prose.
func BuildScenePrompt(ctx ScenePromptContext) string {
	parts := make([]string, 0, 8)

	// Subject: the cue when present, else a short narration excerpt.
	subject := strings.TrimSpace(ctx.Cue)
	if subject == "" {
		subject = sceneExcerpt(ctx.Narration, sceneNarrationCap)
	}
	if subject != "" {
		parts = append(parts, subject)
	}

	if action := sceneExcerpt(ctx.Action, sceneActionCap); action != "" {
		parts = append(parts, "action: "+action)
	}

	if len(ctx.Entities) > 0 {
		entities := ctx.Entities
		if len(entities) > sceneEntityCap {
			entities = entities[:sceneEntityCap]
		}
		parts = append(parts, "characters: "+strings.Join(entities, ", "))
	}

	if location := strings.TrimSpace(ctx.Location); location != "" {
		parts = append(parts, "location: "+location)
	}
	if appearance := strings.TrimSpace(ctx.Appearance); appearance != "" {
		parts = append(parts, appearance)
	}

	if style := strings.TrimSpace(ctx.Style); style != "" {
		parts = append(parts, style)
	}
	if tone := outcomeToneWords(ctx.Outcome); tone != "" {
		parts = append(parts, tone)
	}

	parts = append(parts, sceneSuffix)
	return strings.Join(parts, ", ")
}

// BuildScenePromptFor is the previous three-argument builder, kept for callers
// that only have a cue, a location, and a style.
func BuildScenePromptFor(visualCue string, location *entity.Entity, worldStyle string) string {
	ctx := ScenePromptContext{Cue: visualCue, Style: worldStyle}
	if location != nil {
		ctx.Location = location.Name
		ctx.Appearance = location.Appearance
	}
	return BuildScenePrompt(ctx)
}

// sceneExcerpt returns the first sentence of text, capped at limit characters.
func sceneExcerpt(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexAny(trimmed, ".!?"); idx >= 0 && idx < limit {
		trimmed = trimmed[:idx+1]
	}
	if len(trimmed) > limit {
		trimmed = strings.TrimSpace(trimmed[:limit])
	}
	return trimmed
}

// outcomeToneWords maps the common outcome families to short prompt tone words,
// and returns "" for anything else so a custom vocabulary adds no wrong mood.
func outcomeToneWords(outcome string) string {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "strong", "success", "pass", "critical", "hit":
		return "triumphant, bright"
	case "weak", "partial", "mixed", "success_with_cost":
		return "tense, uncertain"
	case "miss", "fail", "failure":
		return "ominous, shadowed"
	}
	return ""
}

// presentEntityNames resolves the display names of the entities a turn involved,
// so the scene can show them. It caps the list so the prompt stays short.
func (o *TurnOrchestrator) presentEntityNames(turn *Turn) []string {
	seen := make(map[string]bool, len(turn.Entities)+len(turn.Segments))
	names := make([]string, 0, sceneEntityCap)
	add := func(id string) {
		if id == "" || seen[id] || len(names) >= sceneEntityCap {
			return
		}
		seen[id] = true
		name := id
		if o.store != nil {
			if ent, err := o.store.GetEntity(id); err == nil && ent != nil && ent.Name != "" {
				name = ent.Name
			}
		}
		names = append(names, name)
	}
	for _, mention := range turn.Entities {
		add(mention.ID)
	}
	for _, segment := range turn.Segments {
		add(segment.SpeakerID)
	}
	return names
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
