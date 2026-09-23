package gui

import (
	"context"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

// InspectTTS describes one TTS configuration: its provider identity, whether it
// is metered, the tunables it accepts, and its voice catalog. It builds the
// client from the supplied configuration rather than the saved one, so the editor
// can describe a provider before it is applied.
func (s *Service) InspectTTS(ctx context.Context, req TTSInspectRequestDTO) (*TTSInspectResponseDTO, error) {
	cfg := req.Config
	if cfg.Type == "builtin" && (cfg.BuiltinName == "sherpa-onnx" || cfg.BuiltinName == "kokoro") {
		if cfg.ModelPath == "" && s.modelsManager != nil {
			cfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
		}
	}

	sharedKey := ""
	if s.configMgr != nil && s.configMgr.Get() != nil {
		sharedKey = s.configMgr.Get().Providers.Gemini.APIKey
	}
	isGemini := strings.EqualFold(strings.TrimSpace(cfg.Type), "gemini") || strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "gemini")

	response := &TTSInspectResponseDTO{
		ProviderKey: media.ProviderKey(cfg),
		KeyPresent:  media.KeyPresentWithSharedKey(cfg, sharedKey),
		KeyRequired: strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "elevenlabs") || isGemini,
		Catalog:     VoiceCatalogDTO{Voices: []media.ProviderVoice{}},
	}

	client, err := s.ttsClientFor(cfg)
	if err != nil {
		response.Error = err.Error()
		return response, nil
	}

	if options, ok := client.(media.VoiceOptions); ok {
		response.Options = options.VoiceOptions()
	}

	response.Metered = cfg.Metered != nil && *cfg.Metered
	if cfg.Metered == nil {
		if metered, ok := client.(media.MeteredProvider); ok {
			response.Metered = metered.Metered()
		}
	}

	if _, ok := client.(media.VoiceCatalog); ok {
		response.Catalog.Available = true

		catalog := media.NewCachedVoiceCatalog(s.resolver.CacheDir())
		snapshot, err := catalog.Load(ctx, response.ProviderKey, client, req.Refresh)
		response.Catalog.FetchedAt = snapshot.FetchedAt
		response.Catalog.Stale = snapshot.Stale
		if snapshot.Voices != nil {
			response.Catalog.Voices = snapshot.Voices
		}
		if err != nil {
			response.Error = err.Error()
		}
	}

	response.SpeechCues = media.ResolveSpeechCueCapabilities(cfg, client)

	return response, nil
}

// ttsClientFor builds a TTS client, using the injectable factory so a test can
// describe a provider that needs no network.
func (s *Service) ttsClientFor(cfg config.TTSConfig) (media.TTSClient, error) {
	if s.newTTSClient != nil {
		return s.newTTSClient(cfg)
	}
	sharedKey := ""
	if s.configMgr != nil && s.configMgr.Get() != nil {
		sharedKey = s.configMgr.Get().Providers.Gemini.APIKey
	}
	return media.NewTTSClientWithSharedKey(cfg, sharedKey)
}
