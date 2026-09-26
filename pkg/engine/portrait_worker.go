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
	"github.com/darkliquid/localrpg/pkg/storage"
)

// BuildPortraitPrompt formats the 3/4 bust prompt with world art style and character traits.
func BuildPortraitPrompt(name, gender, age, appearance, artStyle string) string {
	if artStyle == "" {
		artStyle = "digital illustration, character concept art"
	}
	var descParts []string
	if gender != "" {
		descParts = append(descParts, gender)
	}
	if age != "" {
		descParts = append(descParts, age+" years old")
	}
	if appearance != "" {
		descParts = append(descParts, strings.TrimSpace(appearance))
	}
	desc := strings.Join(descParts, ", ")
	if desc == "" {
		desc = "detailed character features"
	}

	return fmt.Sprintf("3/4 bust portrait, looking slightly to the right, head and upper torso centered, neutral plain studio backdrop, %s, %s, clean composition, high quality character portrait, no text, no borders", artStyle, desc)
}

// PortraitGenerator abstracts image generation for the worker.
type PortraitGenerator interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

// PortraitWorker manages queued portrait generation with in-memory deduplication.
type PortraitWorker struct {
	mu        sync.Mutex
	resolver  *core.PathResolver
	store     *storage.Store
	generator PortraitGenerator
	inFlight  map[string]bool
}

// NewPortraitWorker creates a new PortraitWorker.
func NewPortraitWorker(resolver *core.PathResolver, store *storage.Store, gen PortraitGenerator) *PortraitWorker {
	return &PortraitWorker{
		resolver:  resolver,
		store:     store,
		generator: gen,
		inFlight:  make(map[string]bool),
	}
}

// Enqueue asynchronously triggers portrait generation for a character if not already in flight or set.
func (w *PortraitWorker) Enqueue(gameID string, ent *entity.Entity, artStyle string) {
	if w.generator == nil || ent == nil || ent.ID == "" || ent.Type != "character" {
		return
	}
	if ent.Portrait != "" {
		return
	}

	key := gameID + ":" + ent.ID
	w.mu.Lock()
	if w.inFlight[key] {
		w.mu.Unlock()
		return
	}
	w.inFlight[key] = true
	w.mu.Unlock()

	entCopy := *ent

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.inFlight, key)
			w.mu.Unlock()
		}()

		_, _ = w.writePortrait(context.Background(), gameID, &entCopy, artStyle)
	}()
}

// Regenerate generates a fresh portrait for ent regardless of any existing
// Portrait value and returns the game-relative path written. Unlike Enqueue it
// runs synchronously and reports failures so a caller can surface them.
func (w *PortraitWorker) Regenerate(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error) {
	if w.generator == nil {
		return "", fmt.Errorf("portrait generator is not configured")
	}
	if ent == nil || ent.ID == "" || ent.Type != "character" {
		return "", fmt.Errorf("portrait regeneration requires a character entity")
	}
	return w.writePortrait(ctx, gameID, ent, artStyle)
}

// portraitExtensions are the file names a generated portrait may carry, used to
// remove a stale clip when the provider changes format.
var portraitExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".svg"}

func (w *PortraitWorker) writePortrait(ctx context.Context, gameID string, ent *entity.Entity, artStyle string) (string, error) {
	prompt := BuildPortraitPrompt(ent.Name, ent.Gender, ent.Age, ent.Appearance, artStyle)
	imgBytes, err := w.generator.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate portrait: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("portrait generator returned no image")
	}

	ext := media.ArtExtension(imgBytes)
	if ext == "" {
		ext = ".png"
	}
	relPath := filepath.Join("assets", "portraits", ent.ID+ext)
	fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("create portrait dir: %w", err)
	}
	if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("write portrait: %w", err)
	}
	w.removeStalePortraits(gameID, ent.ID, ext)

	if err := w.updateNote(gameID, ent, relPath); err != nil {
		return "", err
	}
	return relPath, nil
}

func (w *PortraitWorker) removeStalePortraits(gameID, id, keepExt string) {
	dir := filepath.Join(w.resolver.GameDir(gameID), "assets", "portraits")
	for _, ext := range portraitExtensions {
		if ext == keepExt {
			continue
		}
		_ = os.Remove(filepath.Join(dir, id+ext))
	}
}

// updateNote writes the portrait path into the entity's note frontmatter,
// preferring the on-disk note so a hand edit is not clobbered.
func (w *PortraitWorker) updateNote(gameID string, ent *entity.Entity, relPath string) error {
	notePath := filepath.Join(w.resolver.GameDir(gameID), "entities", ent.ID+".md")
	if existingData, err := os.ReadFile(notePath); err == nil {
		if existingEnt, err := entity.ParseMarkdownEntity(existingData); err == nil {
			existingEnt.Portrait = relPath
			if data, err := existingEnt.SerializeMarkdown(); err == nil {
				if err := os.WriteFile(notePath, data, 0644); err != nil {
					return fmt.Errorf("write note: %w", err)
				}
				if w.store != nil {
					_ = storage.NewSyncer(w.store).SyncFile(notePath)
				}
				return nil
			}
		}
	}

	updated := *ent
	updated.Portrait = relPath
	data, err := updated.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize note: %w", err)
	}
	if err := os.WriteFile(notePath, data, 0644); err != nil {
		return fmt.Errorf("write note: %w", err)
	}
	if w.store != nil {
		_ = storage.NewSyncer(w.store).SyncFile(notePath)
	}
	return nil
}
