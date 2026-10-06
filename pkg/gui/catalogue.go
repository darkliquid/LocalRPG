package gui

import (
	"context"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// ListProviders returns every registered provider descriptor, so the settings UI
// can render from capabilities instead of provider names. Each descriptor's
// caveat is filled in with its tier's default when it sets none, so the client
// never has to know the tier vocabulary.
func (s *Service) ListProviders(ctx context.Context) (*ProviderCatalogDTO, error) {
	descs := provider.List()
	for i := range descs {
		if descs[i].Caveat == "" {
			descs[i].Caveat = descs[i].EffectiveCaveat()
		}
	}
	return &ProviderCatalogDTO{Providers: descs}, nil
}

// ListModels returns the live model catalogue a Gemini key can reach, so the
// editor reflects what the account actually has instead of a static list. An
// absent key still returns a response: the error is data, not a request failure.
func (s *Service) ListModels(ctx context.Context, req ModelCatalogueRequestDTO) (*ModelCatalogueResponseDTO, error) {
	sharedKey := ""
	if cfg := s.configMgr.Get(); cfg != nil {
		sharedKey = cfg.Providers.Gemini.APIKey
	}
	apiKey, err := harness.ResolveGeminiAPIKey(req.APIKey, sharedKey)
	if err != nil {
		return &ModelCatalogueResponseDTO{Error: err.Error()}, nil
	}

	models, err := media.ListGeminiModels(ctx, apiKey)
	if err != nil {
		return &ModelCatalogueResponseDTO{Error: err.Error()}, nil
	}
	return &ModelCatalogueResponseDTO{Models: models}, nil
}

// SearchTTSVoices queries a provider's extended voice library. Only Gemini
// publishes one today; any other provider yields an explanatory error.
func (s *Service) SearchTTSVoices(ctx context.Context, req VoiceSearchRequestDTO) (*VoiceSearchResponseDTO, error) {
	if !isGeminiTTSConfig(req.Config) {
		return &VoiceSearchResponseDTO{Error: "extended voice search is only available for Google Gemini TTS"}, nil
	}

	sharedKey := ""
	if cfg := s.configMgr.Get(); cfg != nil {
		sharedKey = cfg.Providers.Gemini.APIKey
	}
	apiKey, err := media.ResolveGeminiTTSAPIKey(req.Config.APIKey, sharedKey)
	if err != nil {
		return &VoiceSearchResponseDTO{Error: err.Error()}, nil
	}

	voices, err := media.ListGeminiVoices(ctx, apiKey, media.GeminiVoiceSearch{
		Query:        req.Query,
		VoiceType:    req.Type,
		LanguageCode: req.LanguageCode,
		Gender:       req.Gender,
		Accent:       req.Accent,
		Persona:      req.Persona,
	})
	if err != nil {
		return &VoiceSearchResponseDTO{Error: err.Error()}, nil
	}
	return &VoiceSearchResponseDTO{Voices: voices}, nil
}

func isGeminiTTSConfig(cfg config.TTSConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.Type), "gemini") ||
		strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "gemini")
}
