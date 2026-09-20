package media

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type ImageClient interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

type ImagePipeline struct {
	client ImageClient
	cache  *ContentCache
}

func NewImagePipeline(client ImageClient, cache *ContentCache) *ImagePipeline {
	return &ImagePipeline{
		client: client,
		cache:  cache,
	}
}

func (p *ImagePipeline) BuildPrompt(appearance, worldStyle string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(appearance) != "" {
		parts = append(parts, strings.TrimSpace(appearance))
	}
	if strings.TrimSpace(worldStyle) != "" {
		parts = append(parts, strings.TrimSpace(worldStyle))
	}
	return strings.Join(parts, ", ")
}

func (p *ImagePipeline) GenerateSceneImage(ctx context.Context, entityID, appearance, worldStyle string) (string, error) {
	cacheKey := ComputeArtCacheKey(entityID, appearance, worldStyle) + ".webp"
	if p.cache.Exists("images", cacheKey) {
		return filepath.Join(p.cache.Subdir("images"), cacheKey), nil
	}

	prompt := p.BuildPrompt(appearance, worldStyle)
	imgBytes, err := p.client.GenerateImage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("generate image for %q: %w", entityID, err)
	}

	return p.cache.Put("images", cacheKey, imgBytes)
}
