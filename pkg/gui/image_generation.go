package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
)

// imageClientFactory builds the image client. It is a package variable so a
// test can substitute a stub without a live provider.
var imageClientFactory = func(cfg config.ImageConfig, sharedKey string) (media.ImageClient, error) {
	return media.NewImageClientWithSharedKey(cfg, sharedKey)
}

// generateImage runs one image request: span, provider call, byte guard, and
// observability. It never persists. An empty gameID records the spend against
// the shared ledger, which is what a studio asset or a preview is.
func (s *Service) generateImage(ctx context.Context, kind, prompt, gameID string) ([]byte, *harness.GenerationFailure) {
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, kind)
	defer span.End()

	cfg := s.configMgr.Get()
	client, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
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
	s.recordImageUsage(gameID, provider, cfg.Media.Image.Model, client)
	return checked, nil
}

// recordImageUsage prices what the image provider reported and files it against
// the campaign, or against shared spend when there is no campaign.
func (s *Service) recordImageUsage(gameID, provider, model string, client media.ImageClient) {
	reporter, ok := client.(media.UsageReporter)
	if !ok {
		return
	}
	usage := mediaUsage(reporter.LastUsage(), provider, model)
	if usage.Requests == 0 && usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return
	}
	if gameID != "" {
		s.RecordUsage(gameID, 0, "image", usage)
		return
	}
	s.RecordUsageGlobal("image", usage)
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
