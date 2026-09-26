package imagegemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/media"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// GeminiImageClient generates location and scene imagery using Google's Imagen and
// native Gemini Image ("Nano Banana") models.
type GeminiImageClient struct {
	client           *genai.Client
	model            string
	aspectRatio      string
	personGeneration string
}

// NewGeminiImageClient initializes a new GeminiImageClient using credentials from
// the configuration or environment.
func NewGeminiImageClient(cfg config.ImageConfig, sharedKey string) (*GeminiImageClient, error) {
	apiKey, err := media.ResolveGeminiImageAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: telemetry.HTTPTransport(nil)},
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create image client: %w", err)
	}

	return NewGeminiImageClientWithClient(client, cfg)
}

// NewGeminiImageClientWithClient allows injecting an existing genai.Client (used for testing).
func NewGeminiImageClientWithClient(client *genai.Client, cfg config.ImageConfig) (*GeminiImageClient, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "imagen-3.0-generate-002"
	}

	aspectRatio := strings.TrimSpace(cfg.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	personGen := strings.TrimSpace(cfg.PersonGeneration)
	if personGen == "" {
		personGen = string(genai.PersonGenerationAllowAdult)
	}

	return &GeminiImageClient{
		client:           client,
		model:            model,
		aspectRatio:      aspectRatio,
		personGeneration: personGen,
	}, nil
}

// GenerateImage generates image bytes for a text prompt.
func (g *GeminiImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("gemini image: prompt cannot be empty")
	}

	if g.client.ClientConfig().Backend == genai.BackendVertexAI && strings.HasPrefix(g.model, "imagen-") {
		return g.generateImagen(ctx, prompt)
	}
	return g.generateGeminiImage(ctx, prompt)
}

func (g *GeminiImageClient) generateImagen(ctx context.Context, prompt string) ([]byte, error) {
	reqCfg := &genai.GenerateImagesConfig{
		NumberOfImages:   1,
		OutputMIMEType:   "image/jpeg",
		AspectRatio:      g.aspectRatio,
		PersonGeneration: genai.PersonGeneration(g.personGeneration),
	}

	resp, err := g.client.Models.GenerateImages(ctx, g.model, prompt, reqCfg)
	if err != nil {
		return nil, mapGeminiImageError(err)
	}

	if len(resp.GeneratedImages) == 0 || resp.GeneratedImages[0].Image == nil || len(resp.GeneratedImages[0].Image.ImageBytes) == 0 {
		return nil, errors.New("gemini imagen: no image data returned in response")
	}

	return resp.GeneratedImages[0].Image.ImageBytes, nil
}

func (g *GeminiImageClient) generateGeminiImage(ctx context.Context, prompt string) ([]byte, error) {
	imgCfg := &genai.ImageConfig{
		AspectRatio: g.aspectRatio,
	}
	if g.client.ClientConfig().Backend == genai.BackendVertexAI && g.personGeneration != "" {
		imgCfg.PersonGeneration = g.personGeneration
	}

	reqCfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{"IMAGE"},
		ImageConfig:        imgCfg,
	}
	// Some image models require text output alongside the image, and some reject
	// per-model image options; this fallback covers both.
	fallbackCfg := &genai.GenerateContentConfig{ResponseModalities: []string{"TEXT", "IMAGE"}}

	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, reqCfg)
	if err != nil && imgCfg.AspectRatio != "" && isUnsupportedImageOption(err) {
		// Model-specific options (such as an aspect ratio a given image model does
		// not accept) are worth dropping rather than failing the generation.
		resp, err = g.client.Models.GenerateContent(ctx, g.model, contents, fallbackCfg)
	}
	if err != nil {
		return nil, mapGeminiImageError(err)
	}
	if data := firstImageBytes(resp); data != nil {
		return data, nil
	}

	// The image may have been withheld because the model also wanted text output.
	resp, err = g.client.Models.GenerateContent(ctx, g.model, contents, fallbackCfg)
	if err != nil {
		return nil, mapGeminiImageError(err)
	}
	if data := firstImageBytes(resp); data != nil {
		return data, nil
	}

	return nil, errors.New("gemini image: no image data found in response candidates")
}

// firstImageBytes returns the first inline image in a response, or nil.
func firstImageBytes(resp *genai.GenerateContentResponse) []byte {
	if resp == nil {
		return nil
	}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				return part.InlineData.Data
			}
		}
	}
	return nil
}

// isUnsupportedImageOption reports whether an image error names a rejected
// request option, which is model-specific and worth retrying without.
func isUnsupportedImageOption(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"aspect", "image_config", "imageconfig", "invalid_argument",
		"unknown name", "unsupported", "cannot find field", "not supported",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func mapGeminiImageError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "403") || strings.Contains(errStr, "PERMISSION_DENIED") {
		return errors.New("gemini image: invalid API key or permission denied; check media.image.api_key, providers.gemini.api_key, or GEMINI_API_KEY")
	}
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
		return errors.New("gemini image: quota exceeded or rate limit reached; check your Google AI Studio plan")
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "NOT_FOUND") {
		return fmt.Errorf("gemini image: model not found: %w", err)
	}
	return fmt.Errorf("gemini image: generation failed: %w", err)
}

func MapGeminiImageErrorForTest(err error) error {
	return mapGeminiImageError(err)
}
