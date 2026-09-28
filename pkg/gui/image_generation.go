package gui

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// imageClientFactory builds the image client. It is a package variable so a
// test can substitute a stub without a live provider.
var imageClientFactory = func(cfg config.ImageConfig, sharedKey string) (media.ImageClient, error) {
	return media.NewImageClientWithSharedKey(cfg, sharedKey)
}

// usageScope names the ledger an image's spend belongs to: a campaign, a
// creation flow's deferred token, or the shared studio ledger when both are
// empty.
type usageScope struct {
	gameID string
	token  string
}

// generateImage runs one image request: span, provider call, byte guard, and
// observability. It never persists. The scope decides which ledger pays.
func (s *Service) generateImage(ctx context.Context, kind, prompt string, scope usageScope) ([]byte, *harness.GenerationFailure) {
	started := time.Now()
	ctx, span := startImageSpan(ctx, s.logger, kind)
	defer span.End()

	if err := s.guardRole("image"); err != nil {
		failure := &harness.GenerationFailure{Code: harness.FailureRateLimited, Message: err.Error(), Cause: err}
		return nil, failure
	}

	cfg := s.configMgr.Get()
	client, err := imageClientFactory(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		failure := &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: fmt.Sprintf("image provider: %v", err)}
		s.recordImage(ctx, span, kind, "", 0, started, failure)
		return nil, failure
	}

	key, hasKey := media.ImageKeyFor(cfg.Media.Image)
	provider := string(key)
	if !hasKey {
		provider = cfg.Media.Image.BuiltinName
		if provider == "" {
			provider = cfg.Media.Image.Type
		}
	}

	imgBytes, err := client.GenerateImage(ctx, prompt)
	if err != nil {
		s.noteFailure("image", err)
		failure := &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: fmt.Sprintf("generate image: %v", err), Cause: err}
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	s.noteSuccess("image")
	checked, failure := guardImageBytes(imgBytes)
	if failure != nil {
		s.recordImage(ctx, span, kind, provider, 0, started, failure)
		return nil, failure
	}
	s.recordImage(ctx, span, kind, provider, len(checked), started, nil)
	if hasKey {
		s.recordImageUsage(scope, key, cfg.Media.Image.Model, client)
	}
	return checked, nil
}

// recordImageUsage prices what the image provider reported and files it against
// the campaign, a deferred token, or shared spend.
func (s *Service) recordImageUsage(scope usageScope, key provider.Key, model string, client media.ImageClient) {
	reporter, ok := client.(media.UsageReporter)
	if !ok {
		return
	}
	usage := mediaUsage(reporter.LastUsage(), key, model)
	if usage.Requests == 0 && usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return
	}
	switch {
	case scope.gameID != "":
		s.RecordUsage(scope.gameID, 0, "image", usage)
	case scope.token != "":
		s.RecordUsageDeferred(scope.token, "image", usage)
	default:
		s.RecordUsageGlobal("image", usage)
	}
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
