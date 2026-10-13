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

func (s *Service) TestProvider(ctx context.Context, req TestProviderRequestDTO) (*TestProviderResponseDTO, error) {
	start := time.Now()

	data, err := json.Marshal(req.Provider)
	if err != nil {
		return &TestProviderResponseDTO{
			Success: false,
			Message: fmt.Sprintf("invalid provider data: %v", err),
		}, nil
	}

	switch req.Category {
	case "llm":
		var agentCfg harness.ProviderConfig
		if err := json.Unmarshal(data, &agentCfg); err != nil {
			return &TestProviderResponseDTO{
				Success: false,
				Message: fmt.Sprintf("invalid llm config: %v", err),
			}, nil
		}
		// The editor never sends the shared key, so resolve it here the same way
		// the router does. A role-level override still wins inside the factory.
		if cfg := s.configMgr.Get(); cfg != nil {
			agentCfg.SharedAPIKey = cfg.Providers.Gemini.APIKey
		}
		p, err := harness.NewModelProvider("test", agentCfg)
		if err != nil {
			return &TestProviderResponseDTO{
				Success: false,
				Message: fmt.Sprintf("failed to create llm provider: %v", err),
			}, nil
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
			}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   "LLM provider responded successfully",
			Preview:   resp.Text,
		}, nil

	case "tts":
		var ttsCfg config.TTSConfig
		if err := json.Unmarshal(data, &ttsCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		if ttsCfg.Type == "builtin" && (ttsCfg.BuiltinName == "sherpa-onnx" || ttsCfg.BuiltinName == "kokoro") {
			if ttsCfg.ModelPath == "" && s.modelsManager != nil {
				ttsCfg.ModelPath = s.modelsManager.ModelDir("kokoro-tts")
			}
			if s.modelsManager != nil {
				status := s.modelsManager.Status("kokoro-tts")
				if !status.Installed {
					return &TestProviderResponseDTO{
						Success:      false,
						ModelMissing: true,
						ModelID:      "kokoro-tts",
						Message:      "Kokoro voice pack is not installed; download required",
					}, nil
				}
			}
		}
		cfg := s.configMgr.Get()
		ttsKey, hasTTSKey := media.TTSKeyFor(ttsCfg)
		client, err := media.NewTTSClientWithSharedKey(ttsCfg, media.SharedProviderKey(cfg, ttsKey, hasTTSKey))
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = defaultTTSPreviewText
		}
		spoken := media.SpeakableTextFor(media.TextPolicyFromConfig(ttsCfg), client, prompt)
		if strings.TrimSpace(spoken) == "" {
			return &TestProviderResponseDTO{Success: false, Message: "The test phrase reduced to no speakable text"}, nil
		}
		voiceID := ttsCfg.DefaultVoice
		if req.VoiceID != "" {
			voiceID = req.VoiceID
		}
		voice := &entity.VoiceConfig{
			VoiceID:    voiceID,
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
			}, nil
		}
		if len(audio) == 0 {
			return &TestProviderResponseDTO{
				Success:   false,
				LatencyMS: latency,
				Message:   "Synthesis returned no audio; check the provider endpoint and voice ID",
			}, nil
		}
		return &TestProviderResponseDTO{
			Success:      true,
			LatencyMS:    latency,
			Message:      fmt.Sprintf("Synthesized %d bytes of audio successfully", len(audio)),
			AudioDataURI: fmt.Sprintf("data:%s;base64,%s", media.AudioContentType(audio), base64.StdEncoding.EncodeToString(audio)),
		}, nil

	case "stt":
		var sttCfg config.STTConfig
		if err := json.Unmarshal(data, &sttCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		if sttCfg.Type == "web-speech" {
			return &TestProviderResponseDTO{
				Success: false,
				Message: "web-speech runs in the browser; choose an HTTP or CLI Whisper provider",
			}, nil
		}
		key, hasKey := media.STTKeyFor(sttCfg)
		client, err := media.NewSTTClientWithSharedKey(sttCfg, media.SharedProviderKey(s.configMgr.Get(), key, hasKey))
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
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
			}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   "Transcribed audio successfully",
			Preview:   text,
		}, nil

	case "image":
		var imgCfg config.ImageConfig
		if err := json.Unmarshal(data, &imgCfg); err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		cfg := s.configMgr.Get()
		client, err := media.NewImageClientWithSharedKey(imgCfg, cfg.Providers.Gemini.APIKey)
		if err != nil {
			return &TestProviderResponseDTO{Success: false, Message: err.Error()}, nil
		}
		prompt := req.TestPrompt
		if prompt == "" {
			prompt = "a dark forest path"
		}
		imgBytes, err := client.GenerateImage(ctx, prompt)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			return &TestProviderResponseDTO{Success: false, LatencyMS: latency, Message: err.Error()}, nil
		}
		return &TestProviderResponseDTO{
			Success:   true,
			LatencyMS: latency,
			Message:   fmt.Sprintf("Generated %d bytes of image data successfully", len(imgBytes)),
		}, nil

	default:
		return &TestProviderResponseDTO{
			Success: false,
			Message: fmt.Sprintf("unsupported test category: %s", req.Category),
		}, nil
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
