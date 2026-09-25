package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
)

// generateImage runs one image request: span, provider call, byte guard, and
// observability. It never persists.
func (s *Service) generateImage(ctx context.Context, kind, prompt string) ([]byte, *harness.GenerationFailure) {
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, kind)
	defer span.End()

	cfg := s.configMgr.Get()
	client, err := media.NewImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		failure := &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: fmt.Sprintf("image provider: %v", err)}
		s.recordImage(ctx, span, kind, "", 0, started, failure)
		return nil, failure
	}

	provider := cfg.Media.Image.BuiltinName
	if provider == "" {
		provider = cfg.Media.Image.Type
	}

	imgBytes, err := client.GenerateImage(ctx, prompt)
	if err != nil {
		failure := &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: fmt.Sprintf("generate image: %v", err)}
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	checked, failure := guardImageBytes(imgBytes)
	if failure != nil {
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	s.recordImage(ctx, span, kind, provider, len(checked), started, nil)
	return checked, nil
}

// imageContentType maps image bytes to a served MIME type.
func imageContentType(imgBytes []byte) string {
	switch media.ArtExtension(imgBytes) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
