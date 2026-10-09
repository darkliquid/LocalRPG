package media

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// ArtStore resolves scene art through the image pipeline, caching by appearance.
// It is the one implementation of "where does this location's image come from",
// shared by the GUI and the export so they cannot diverge.
type ArtStore struct {
	pipeline       *ImagePipeline
	worldStyle     string
	providerParams string
}

// NewArtStore builds a resolver for one campaign's art settings.
func NewArtStore(client ImageClient, cache *ContentCache, worldStyle, providerParams string) *ArtStore {
	return &ArtStore{
		pipeline:       NewImagePipeline(client, cache),
		worldStyle:     worldStyle,
		providerParams: providerParams,
	}
}

// SceneLayers returns a location's scene as layers, when the image provider is
// the built-in generator. ok is false for a provider that only makes flat images,
// so a caller falls back to the single image.
func (s *ArtStore) SceneLayers(location *entity.Entity) (LayeredScene, bool) {
	if s == nil || s.pipeline == nil {
		return LayeredScene{}, false
	}
	return s.pipeline.LocationLayers(location, s.worldStyle, s.providerParams)
}

// SceneArt returns a location's image path, generating it when the appearance has
// changed or force is set.
func (s *ArtStore) SceneArt(ctx context.Context, location *entity.Entity, force bool) (string, error) {
	if location == nil {
		return "", fmt.Errorf("scene art: no location")
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("scene art for %q: %w", location.ID, err)
	}

	path, err := s.pipeline.GenerateLocationImage(ctx, location, s.worldStyle, s.providerParams, force)
	if err != nil {
		return "", fmt.Errorf("scene art for %q: %w", location.ID, err)
	}
	return path, nil
}
