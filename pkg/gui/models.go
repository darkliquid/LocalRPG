// Local model management and the provider probes: listing, downloading, and testing.

package gui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func (s *Service) GetModelsStatus() []models.ModelStatus {
	return s.modelsManager.ListStatuses()
}

func (s *Service) DownloadModel(ctx context.Context, id string) error {
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	_, err := s.modelsManager.Download(ctx, id)
	return err
}

func (s *Service) SubscribeModelEvents() chan models.ModelStatus {
	return s.modelsManager.Subscribe()
}

func (s *Service) UnsubscribeModelEvents(ch chan models.ModelStatus) {
	s.modelsManager.Unsubscribe(ch)
}

// TestProvider probes one configured provider and reports what happened. Each
// category is its own probe: the request's provider block is decoded into that
// category's config, the provider is built the way the router builds it, and one
// short request is sent.
//
// A probe that never reached its provider is not an error: the answer is 200 with
// Success false, so the editor can show the reason rather than a failure.
func (s *Service) TestProvider(ctx context.Context, req TestProviderRequestDTO) (*TestProviderResponseDTO, error) {
	start := time.Now()

	data, err := json.Marshal(req.Provider)
	if err != nil {
		return providerTestFailure(fmt.Sprintf("invalid provider data: %v", err)), nil
	}

	switch req.Category {
	case "llm":
		return s.testLLMProvider(ctx, req, data, start), nil
	case "tts":
		return s.testTTSProvider(ctx, req, data, start), nil
	case "stt":
		return s.testSTTProvider(ctx, data, start), nil
	case "image":
		return s.testImageProvider(ctx, req, data, start), nil
	default:
		return providerTestFailure(fmt.Sprintf("unsupported test category: %s", req.Category)), nil
	}
}

// providerTestFailure is the response a probe that never reached its provider gets.
func providerTestFailure(message string) *TestProviderResponseDTO {
	return &TestProviderResponseDTO{Success: false, Message: message}
}

// testLLMProvider sends the configured model a short prompt and reports what came
// back, with how long it took.
func (s *Service) testLLMProvider(ctx context.Context, req TestProviderRequestDTO, data []byte, start time.Time) *TestProviderResponseDTO {
	var agentCfg harness.ProviderConfig
	if err := json.Unmarshal(data, &agentCfg); err != nil {
		return providerTestFailure(fmt.Sprintf("invalid llm config: %v", err))
	}
	// The editor never sends the shared key, so resolve it here the same way
	// the router does. A role-level override still wins inside the factory.
	if cfg := s.configMgr.Get(); cfg != nil {
		agentCfg.SharedAPIKey = cfg.Providers.Gemini.APIKey
	}
	p, err := harness.NewModelProvider("test", agentCfg)
	if err != nil {
		return providerTestFailure(fmt.Sprintf("failed to create llm provider: %v", err))
	}
	prompt := req.TestPrompt
	if prompt == "" {
		prompt = "ping"
	}
	resp, err := p.Generate(ctx, harness.GenerateRequest{Prompt: prompt})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestProviderResponseDTO{
			Success:   false,
			LatencyMS: latency,
			Message:   fmt.Sprintf("generate failed: %v", err),
		}
	}
	return &TestProviderResponseDTO{
		Success:   true,
		LatencyMS: latency,
		Message:   "LLM provider responded successfully",
		Preview:   resp.Text,
	}
}

// testTTSProvider synthesizes a short phrase and reports the audio that came back.
func (s *Service) testTTSProvider(ctx context.Context, req TestProviderRequestDTO, data []byte, start time.Time) *TestProviderResponseDTO {
	var ttsCfg config.TTSConfig
	if err := json.Unmarshal(data, &ttsCfg); err != nil {
		return providerTestFailure(err.Error())
	}
	if missing := s.builtinVoicePackMissing(&ttsCfg); missing != nil {
		return missing
	}
	cfg := s.configMgr.Get()
	ttsKey, hasTTSKey := media.TTSKeyFor(ttsCfg)
	client, err := media.NewTTSClientWithSharedKey(ttsCfg, media.SharedProviderKey(cfg, ttsKey, hasTTSKey))
	if err != nil {
		return providerTestFailure(err.Error())
	}
	prompt := req.TestPrompt
	if prompt == "" {
		prompt = defaultTTSPreviewText
	}
	spoken := media.SpeakableTextFor(media.TextPolicyFromConfig(ttsCfg), client, prompt)
	if strings.TrimSpace(spoken) == "" {
		return providerTestFailure("The test phrase reduced to no speakable text")
	}
	voice := &entity.VoiceConfig{
		VoiceID:    ttsVoiceID(ttsCfg.DefaultVoice, req.VoiceID),
		Pitch:      ttsCfg.Pitch,
		SpeechRate: ttsCfg.SpeechRate,
		Options:    ttsCfg.Options,
	}
	audio, err := client.Synthesize(ctx, spoken, voice)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestProviderResponseDTO{
			Success:   false,
			LatencyMS: latency,
			Message:   err.Error(),
			Failure:   &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: err.Error(), Cause: err},
		}
	}
	if len(audio) == 0 {
		return &TestProviderResponseDTO{
			Success:   false,
			LatencyMS: latency,
			Message:   "Synthesis returned no audio; check the provider endpoint and voice ID",
		}
	}
	return &TestProviderResponseDTO{
		Success:      true,
		LatencyMS:    latency,
		Message:      fmt.Sprintf("Synthesized %d bytes of audio successfully", len(audio)),
		AudioDataURI: fmt.Sprintf("data:%s;base64,%s", media.AudioContentType(audio), base64.StdEncoding.EncodeToString(audio)),
	}
}

// builtinVoicePackMissing is the response a built-in TTS probe gets when the voice
// pack it needs is not installed, so the client can prompt for the download. It is
// nil when the pack is there, and when the provider is not one of the built-ins.
//
// It fills in the model path on the config it is given, which is the same config the
// client is then built from.
func (s *Service) builtinVoicePackMissing(ttsCfg *config.TTSConfig) *TestProviderResponseDTO {
	if ttsCfg.Type != "builtin" || (ttsCfg.BuiltinName != "sherpa-onnx" && ttsCfg.BuiltinName != "kokoro") {
		return nil
	}
	if ttsCfg.ModelPath == "" && s.modelsManager != nil {
		ttsCfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
	}
	if s.modelsManager == nil {
		return nil
	}
	status := s.modelsManager.Status("kokoro-tts")
	if status.Installed {
		return nil
	}
	return &TestProviderResponseDTO{
		Success:      false,
		ModelMissing: true,
		ModelID:      "kokoro-tts",
		Message:      "Kokoro voice pack is not installed; download required",
	}
}

// ttsVoiceID is the voice a preview is spoken in: the requested one when the
// request names it, and the configured default otherwise.
func ttsVoiceID(defaultVoice, requested string) string {
	if requested != "" {
		return requested
	}
	return defaultVoice
}

// testSTTProvider transcribes a generated tone, which is enough to prove the
// endpoint answers.
func (s *Service) testSTTProvider(ctx context.Context, data []byte, start time.Time) *TestProviderResponseDTO {
	var sttCfg config.STTConfig
	if err := json.Unmarshal(data, &sttCfg); err != nil {
		return providerTestFailure(err.Error())
	}
	if sttCfg.Type == "web-speech" {
		return providerTestFailure("web-speech runs in the browser; choose an HTTP or CLI Whisper provider")
	}
	key, hasKey := media.STTKeyFor(sttCfg)
	client, err := media.NewSTTClientWithSharedKey(sttCfg, media.SharedProviderKey(s.configMgr.Get(), key, hasKey))
	if err != nil {
		return providerTestFailure(err.Error())
	}
	text, err := client.Transcribe(ctx, media.GenerateToneWAV(440, 0.1))
	latency := time.Since(start).Milliseconds()
	if key, ok := media.STTKeyFor(sttCfg); ok {
		if reporter, ok := client.(media.UsageReporter); ok {
			s.RecordUsageGlobal("stt", mediaUsage(reporter.LastUsage(), key, sttCfg.Model))
		}
	}
	if err != nil {
		return &TestProviderResponseDTO{
			Success:   false,
			LatencyMS: latency,
			Message:   err.Error(),
			Failure:   &harness.GenerationFailure{Code: harness.ClassifyProviderError(err), Message: err.Error(), Cause: err},
		}
	}
	return &TestProviderResponseDTO{
		Success:   true,
		LatencyMS: latency,
		Message:   "Transcribed audio successfully",
		Preview:   text,
	}
}

// testImageProvider generates one small image and reports how large it came back.
func (s *Service) testImageProvider(ctx context.Context, req TestProviderRequestDTO, data []byte, start time.Time) *TestProviderResponseDTO {
	var imgCfg config.ImageConfig
	if err := json.Unmarshal(data, &imgCfg); err != nil {
		return providerTestFailure(err.Error())
	}
	cfg := s.configMgr.Get()
	client, err := media.NewImageClientWithSharedKey(imgCfg, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return providerTestFailure(err.Error())
	}
	prompt := req.TestPrompt
	if prompt == "" {
		prompt = "a dark forest path"
	}
	imgBytes, err := client.GenerateImage(ctx, prompt)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &TestProviderResponseDTO{Success: false, LatencyMS: latency, Message: err.Error()}
	}
	return &TestProviderResponseDTO{
		Success:   true,
		LatencyMS: latency,
		Message:   fmt.Sprintf("Generated %d bytes of image data successfully", len(imgBytes)),
	}
}

// TranscribeAudio transcribes recorded audio data using the configured STT client.
func (s *Service) TranscribeAudio(ctx context.Context, audioData []byte) (string, error) {
	cfg := s.configMgr.Get()
	if cfg.Media.STT.Type == "" || cfg.Media.STT.Type == "disabled" {
		return "", &harness.GenerationFailure{Code: harness.FailureProviderUnavailable, Message: "STT engine is disabled or unconfigured"}
	}
	if cfg.Media.STT.Type == "web-speech" {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: "the web-speech STT provider runs in the browser; configure an HTTP or CLI Whisper provider",
		}
	}

	sttKey, hasSTTKey := media.STTKeyFor(cfg.Media.STT)
	client, err := media.NewSTTClientWithSharedKey(cfg.Media.STT, media.SharedProviderKey(cfg, sttKey, hasSTTKey))
	if err != nil {
		return "", &harness.GenerationFailure{
			Code:    harness.FailureProviderUnavailable,
			Message: fmt.Sprintf("initialize STT client: %v", err),
			Cause:   err,
		}
	}

	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.stt.request", map[string]any{
		"provider": cfg.Media.STT.Type,
		"bytes":    len(audioData),
	})

	text, err := media.NewSTTProvider(client).TranscribeAudio(ctx, audioData)
	if err != nil {
		s.logger.Event("provider.error", map[string]any{"role": "stt", "error": err.Error()})
		return "", err
	}

	s.logger.Event("media.stt.result", map[string]any{
		"chars":       len([]rune(text)),
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return text, nil
}
