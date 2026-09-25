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

		prompt := BuildPortraitPrompt(entCopy.Name, entCopy.Gender, entCopy.Age, entCopy.Appearance, artStyle)
		imgBytes, err := w.generator.GenerateImage(context.Background(), prompt)
		if err != nil || len(imgBytes) == 0 {
			return
		}

		ext := media.ArtExtension(imgBytes)
		if ext == "" {
			ext = ".png"
		}
		relPath := filepath.Join("assets", "portraits", entCopy.ID+ext)
		fullPath := filepath.Join(w.resolver.GameDir(gameID), relPath)

		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return
		}
		if err := os.WriteFile(fullPath, imgBytes, 0644); err != nil {
			return
		}

		notePath := filepath.Join(w.resolver.GameDir(gameID), "entities", entCopy.ID+".md")
		if existingData, err := os.ReadFile(notePath); err == nil {
			if existingEnt, err := entity.ParseMarkdownEntity(existingData); err == nil {
				existingEnt.Portrait = relPath
				if data, err := existingEnt.SerializeMarkdown(); err == nil {
					_ = os.WriteFile(notePath, data, 0644)
					if w.store != nil {
						_ = storage.NewSyncer(w.store).SyncFile(notePath)
					}
					return
				}
			}
		}

		entCopy.Portrait = relPath
		if data, err := entCopy.SerializeMarkdown(); err == nil {
			_ = os.WriteFile(notePath, data, 0644)
			if w.store != nil {
				_ = storage.NewSyncer(w.store).SyncFile(notePath)
			}
		}
	}()
}
