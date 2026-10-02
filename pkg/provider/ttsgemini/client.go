package ttsgemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// geminiPrebuiltVoices defines the 30 Google Gemini TTS prebuilt voices with their
// acoustic tone descriptions and character archetype tags.
var geminiPrebuiltVoices = []media.ProviderVoice{
	{ID: "Aoede", Name: "Aoede", Description: "Breezy", Language: "en", Tags: []string{"narrator", "storyteller", "breezy", "female"}},
	{ID: "Sulafat", Name: "Sulafat", Description: "Warm", Language: "en", Tags: []string{"warm", "elder", "wise", "female"}},
	{ID: "Sadaltager", Name: "Sadaltager", Description: "Knowledgeable", Language: "en", Tags: []string{"knowledgeable", "sage", "scholar", "male"}},
	{ID: "Charon", Name: "Charon", Description: "Informative", Language: "en", Tags: []string{"informative", "guide", "neutral"}},
	{ID: "Kore", Name: "Kore", Description: "Firm", Language: "en", Tags: []string{"firm", "noble", "guard", "female"}},
	{ID: "Orus", Name: "Orus", Description: "Firm", Language: "en", Tags: []string{"firm", "authoritative", "elder", "male"}},
	{ID: "Alnilam", Name: "Alnilam", Description: "Firm", Language: "en", Tags: []string{"firm", "soldier", "warrior", "male"}},
	{ID: "Fenrir", Name: "Fenrir", Description: "Excitable", Language: "en", Tags: []string{"excitable", "warrior", "fierce", "male"}},
	{ID: "Puck", Name: "Puck", Description: "Upbeat", Language: "en", Tags: []string{"upbeat", "trickster", "youthful", "male"}},
	{ID: "Laomedeia", Name: "Laomedeia", Description: "Upbeat", Language: "en", Tags: []string{"upbeat", "bard", "lively", "female"}},
	{ID: "Sadachbia", Name: "Sadachbia", Description: "Lively", Language: "en", Tags: []string{"lively", "merchant", "cheerful", "neutral"}},
	{ID: "Achernar", Name: "Achernar", Description: "Soft", Language: "en", Tags: []string{"soft", "healer", "gentle", "neutral"}},
	{ID: "Vindemiatrix", Name: "Vindemiatrix", Description: "Gentle", Language: "en", Tags: []string{"gentle", "healer", "kind", "female"}},
	{ID: "Achird", Name: "Achird", Description: "Friendly", Language: "en", Tags: []string{"friendly", "innkeeper", "warm", "male"}},
	{ID: "Zubenelgenubi", Name: "Zubenelgenubi", Description: "Casual", Language: "en", Tags: []string{"casual", "thief", "rogue", "neutral"}},
	{ID: "Algenib", Name: "Algenib", Description: "Gravelly", Language: "en", Tags: []string{"gravelly", "villain", "rough", "male"}},
	{ID: "Gacrux", Name: "Gacrux", Description: "Mature", Language: "en", Tags: []string{"mature", "elder", "gruff", "male"}},
	{ID: "Iapetus", Name: "Iapetus", Description: "Clear", Language: "en", Tags: []string{"clear", "herald", "noble", "male"}},
	{ID: "Erinome", Name: "Erinome", Description: "Clear", Language: "en", Tags: []string{"clear", "scholar", "precise", "female"}},
	{ID: "Rasalgethi", Name: "Rasalgethi", Description: "Informative", Language: "en", Tags: []string{"informative", "sage", "neutral"}},
	{ID: "Leda", Name: "Leda", Description: "Youthful", Language: "en", Tags: []string{"youthful", "servant", "young", "female"}},
	{ID: "Zephyr", Name: "Zephyr", Description: "Bright", Language: "en", Tags: []string{"bright", "traveller", "airy", "neutral"}},
	{ID: "Autonoe", Name: "Autonoe", Description: "Bright", Language: "en", Tags: []string{"bright", "bard", "musical", "female"}},
	{ID: "Callirrhoe", Name: "Callirrhoe", Description: "Easy-going", Language: "en", Tags: []string{"easy-going", "companion", "relaxed", "female"}},
	{ID: "Umbriel", Name: "Umbriel", Description: "Easy-going", Language: "en", Tags: []string{"easy-going", "rogue", "casual", "neutral"}},
	{ID: "Algieba", Name: "Algieba", Description: "Smooth", Language: "en", Tags: []string{"smooth", "noble", "courtly", "neutral"}},
	{ID: "Despina", Name: "Despina", Description: "Smooth", Language: "en", Tags: []string{"smooth", "courtier", "female"}},
	{ID: "Enceladus", Name: "Enceladus", Description: "Breathy", Language: "en", Tags: []string{"breathy", "mystic", "ethereal", "male"}},
	{ID: "Pulcherrima", Name: "Pulcherrima", Description: "Forward", Language: "en", Tags: []string{"forward", "villain", "bold", "female"}},
	{ID: "Schedar", Name: "Schedar", Description: "Even", Language: "en", Tags: []string{"even", "narrator", "steady", "neutral"}},
}

// GeminiTTSClient is the provider for Google Gemini speech synthesis. It
// implements media.TTSClient, media.VoiceCatalog, media.MeteredProvider, and media.SpeechCueAdvertiser.
type GeminiTTSClient struct {
	client       *genai.Client
	model        string
	defaultVoice string

	usageMu   sync.Mutex
	lastUsage media.Usage
	apiKey    string
	logger    trace.Logger
}

// NewGeminiTTSClient builds a GeminiTTSClient using the configured or shared API key.
func NewGeminiTTSClient(cfg config.TTSConfig, sharedKey string) (*GeminiTTSClient, error) {
	apiKey, err := media.ResolveGeminiTTSAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}

	trace.RegisterSecret(apiKey)

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: telemetry.HTTPTransport(nil)},
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create tts client: %w", err)
	}

	ttsClient, err := NewGeminiTTSClientWithClient(client, cfg)
	if err != nil {
		return nil, err
	}
	ttsClient.apiKey = apiKey
	return ttsClient, nil
}

// NewGeminiTTSClientWithClient wraps an existing genai.Client (used for testing or custom configs).
func NewGeminiTTSClientWithClient(client *genai.Client, cfg config.TTSConfig) (*GeminiTTSClient, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gemini-3.8-flash-tts"
	}

	defaultVoice := strings.TrimSpace(cfg.DefaultVoice)
	if defaultVoice == "" {
		defaultVoice = "Aoede"
	}

	return &GeminiTTSClient{
		client:       client,
		model:        model,
		defaultVoice: defaultVoice,
	}, nil
}

// NewGeminiTTSClientOffline creates a client without a network connection (used for inspecting voices).
func NewGeminiTTSClientOffline(model, defaultVoice string) *GeminiTTSClient {
	if model == "" {
		model = "gemini-3.8-flash-tts"
	}
	if defaultVoice == "" {
		defaultVoice = "Aoede"
	}
	return &GeminiTTSClient{
		model:        model,
		defaultVoice: defaultVoice,
	}
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *GeminiTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that Gemini TTS charges per character.
func (c *GeminiTTSClient) Metered() bool {
	return true
}

// LastUsage reports the token usage of the last synthesis, so the caller can
// price it. A request Gemini could not meter still counts as one request.
func (c *GeminiTTSClient) LastUsage() media.Usage {
	c.usageMu.Lock()
	defer c.usageMu.Unlock()
	return c.lastUsage
}

func (c *GeminiTTSClient) setLastUsage(meta *genai.GenerateContentResponseUsageMetadata) {
	usage := media.Usage{Requests: 1}
	if meta != nil {
		usage.InputTokens = int(meta.PromptTokenCount)
		usage.OutputTokens = int(meta.CandidatesTokenCount)
	}
	c.usageMu.Lock()
	c.lastUsage = usage
	c.usageMu.Unlock()
}

// SupportsMarkdown reports whether this provider natively interprets Markdown emphasis.
func (c *GeminiTTSClient) SupportsMarkdown() bool {
	return false
}

// VoiceOptions declares the tunables Gemini TTS accepts. Direction is free text
// prepended to the utterance as director's notes, which is how the Gemini API
// takes style steering: it has no separate style field.
func (c *GeminiTTSClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{Key: "direction", Label: "Voice direction", Kind: "string",
			Help: "Director's notes steering delivery, e.g. \"weary and guarded, speaking slowly\"."},
	}
}

// ListVoices enumerates the 30 Gemini prebuilt voices.
func (c *GeminiTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	voices := make([]media.ProviderVoice, len(geminiPrebuiltVoices))
	copy(voices, geminiPrebuiltVoices)
	return voices, nil
}

// ListExtendedVoices searches the extended Gemini voice library. It delegates to
// the REST catalogue, so the client advertises the capability only when it was
// built with a key.
func (c *GeminiTTSClient) ListExtendedVoices(ctx context.Context, query string) ([]media.ProviderVoice, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, media.ErrGeminiTTSAPIKeyRequired
	}
	return media.ListGeminiVoices(ctx, c.apiKey, media.GeminiVoiceSearch{Query: query})
}

// SpeechCueCapabilities declares that Gemini TTS supports bracketed vocal cues/tags.
func (c *GeminiTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"whispers", "shouting", "laughs", "sighs", "gasp", "giggles",
			"amazed", "crying", "curious", "excited", "mischievously",
			"panicked", "sarcastic", "serious", "tired", "trembling",
			"cough", "excitedly", "bored", "reluctantly",
		},
		PromptGuidance: "Use [tag] inline modifiers in the transcript to control delivery. " +
			"Examples: [whispers], [shouting], [laughs], [sighs], [trembling]. " +
			"Tags can be combined and placed mid-sentence. Use English tags even for non-English text.",
	}
}

// Synthesize generates speech audio bytes for the given text and voice configuration.
func (c *GeminiTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("gemini tts: text cannot be empty")
	}
	if c.client == nil {
		return nil, errors.New("gemini tts: client not initialized")
	}

	voiceName := c.voiceNameFor(voice)

	// Gemini TTS has no style field; delivery is steered by director's notes
	// prepended to the transcript. Sanitise against the declared schema so a
	// hand-edited note cannot smuggle in a different shape.
	options, _ := media.ValidateVoiceOptions(c.VoiceOptions(), media.VoiceOptionsOf(voice))
	promptText := text
	if direction, ok := options["direction"].(string); ok {
		if trimmed := strings.TrimSpace(direction); trimmed != "" {
			promptText = "### DIRECTOR'S NOTES\nStyle: " + trimmed + "\n\n#### TRANSCRIPT\n" + text
		}
	}

	reqConfig := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO"},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: voiceName,
				},
			},
		},
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(promptText), reqConfig)
	if err != nil {
		return nil, mapGeminiTTSError(err, c.model)
	}
	return c.audioFromResponse(resp)
}

// audioFromResponse pulls the audio out of a TTS response and wraps headerless
// PCM so a decoder or browser can play it. An already-containerised clip (some
// models return WAV) is passed through unchanged.
func (c *GeminiTTSClient) audioFromResponse(resp *genai.GenerateContentResponse) ([]byte, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, errors.New("gemini tts: no candidates returned from model")
	}
	c.setLastUsage(resp.UsageMetadata)

	cand := resp.Candidates[0]
	if cand.Content == nil || len(cand.Content.Parts) == 0 {
		return nil, errors.New("gemini tts: empty content returned from model")
	}

	for _, part := range cand.Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			data := part.InlineData.Data
			if media.IsPCMAudio(part.InlineData.MIMEType) {
				return media.WrapPCMAsWAV(data, media.PCMSampleRate(part.InlineData.MIMEType)), nil
			}
			return data, nil
		}
	}

	return nil, errors.New("gemini tts: no audio data found in response")
}

// TTSCapabilities declares what one Gemini request can render: two speakers, the
// documented 8192-token input cap (approximated in characters for grouping), and
// batch and streaming support.
func (c *GeminiTTSClient) TTSCapabilities() media.TTSCapabilities {
	return media.TTSCapabilities{
		MaxSpeakers:         2,
		MaxCharsPerRequest:  8192,
		MaxTokensPerRequest: 8192,
		SupportsGrouping:    true,
		SupportsBatch:       true,
		SupportsStreaming:   true,
	}
}

// SynthesizeGroup renders a two-speaker scene in one request. Gemini addresses
// speakers by a label that must match the transcript, so each line's label is
// used both in the speaker configuration and in the text.
func (c *GeminiTTSClient) SynthesizeGroup(ctx context.Context, lines []media.SpeakerLine) ([]byte, error) {
	if len(lines) == 0 {
		return nil, errors.New("gemini tts: a group needs at least one line")
	}

	speakerConfigs := make([]*genai.SpeakerVoiceConfig, 0, len(lines))
	seen := map[string]bool{}
	var transcript strings.Builder
	for _, line := range lines {
		label := strings.TrimSpace(line.Label)
		if label == "" {
			label = strings.TrimSpace(line.SpeakerID)
		}
		if label == "" {
			return nil, errors.New("gemini tts: every speaker needs a label")
		}
		if !seen[label] {
			seen[label] = true
			speakerConfigs = append(speakerConfigs, &genai.SpeakerVoiceConfig{
				Speaker:     label,
				VoiceConfig: &genai.VoiceConfig{PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: c.voiceNameFor(line.Voice)}},
			})
		}
		transcript.WriteString(label)
		transcript.WriteString(": ")
		transcript.WriteString(strings.TrimSpace(line.Text))
		transcript.WriteByte('\n')
	}

	if len(speakerConfigs) != 2 {
		return nil, fmt.Errorf("gemini tts: exactly two speakers are required, got %d", len(speakerConfigs))
	}
	if c.client == nil {
		return nil, errors.New("gemini tts: client not initialized")
	}

	reqConfig := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO"},
		SpeechConfig: &genai.SpeechConfig{
			MultiSpeakerVoiceConfig: &genai.MultiSpeakerVoiceConfig{SpeakerVoiceConfigs: speakerConfigs},
		},
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(transcript.String()), reqConfig)
	if err != nil {
		return nil, mapGeminiTTSError(err, c.model)
	}
	return c.audioFromResponse(resp)
}

// voiceNameFor resolves a line's voice, falling back to the client's default.
func (c *GeminiTTSClient) voiceNameFor(voice *entity.VoiceConfig) string {
	if voice != nil && strings.TrimSpace(voice.VoiceID) != "" {
		return strings.TrimSpace(voice.VoiceID)
	}
	return c.defaultVoice
}

func mapGeminiTTSError(err error, model string) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "API_KEY_INVALID") || strings.Contains(msg, "PERMISSION_DENIED") || strings.Contains(msg, "403"):
		return fmt.Errorf("gemini tts: invalid or unauthorized API key: %w", err)
	case strings.Contains(msg, "RESOURCE_EXHAUSTED") || strings.Contains(msg, "429"):
		return fmt.Errorf("gemini tts: quota or rate limit exceeded: %w", err)
	case strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "404"):
		return fmt.Errorf("gemini tts: model %q not found or not supported for audio: %w", model, err)
	default:
		return fmt.Errorf("gemini tts: synthesis failed: %w", err)
	}
}
