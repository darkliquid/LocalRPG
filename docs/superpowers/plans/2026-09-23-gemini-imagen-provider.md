# Google Gemini & Imagen Image Generation Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Phase 2 of the Google Gemini integration: a native Go image generation client supporting both the Imagen 3 and native Gemini Image ("Nano Banana") suites for location and scene art.

**Architecture:** A new `GeminiImageClient` in `pkg/media` implements the `ImageClient` interface using the official `google.golang.org/genai` SDK. Models starting with `imagen-` route to `client.Models.GenerateImages`, while native Gemini image models route to `client.Models.GenerateContent` with `ResponseModalities: ["IMAGE"]`. The cache layer is updated to recognize JPEG and PNG byte signatures, and the Settings Studio UI provides preset pills and tunables for aspect ratio and person generation.

**Tech Stack:** Go 1.27.1, `google.golang.org/genai v1.71.0`, TypeScript, React 19, Tailwind v4.

---

### Task 1: Configuration Schema & Presets for Image Generation

**Files:**
- Modify: `pkg/config/types.go:174-188`
- Modify: `pkg/config/presets.go:173-201`
- Test: `pkg/config/types_test.go`
- Test: `pkg/config/presets_test.go`

- [ ] **Step 1: Write failing tests for image configuration and presets**

In `pkg/config/types_test.go`, add:
```go
func TestConfigParsesImageTunables(t *testing.T) {
	yamlData := `
version: "1"
paths:
  systems: "./systems"
  worlds: "./worlds"
  games: "./games"
  cache: "./cache"
media:
  image:
    type: "gemini"
    model: "imagen-3.0-generate-002"
    aspect_ratio: "16:9"
    person_generation: "ALLOW_ADULT"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	if cfg.Media.Image.AspectRatio != "16:9" {
		t.Errorf("expected aspect_ratio 16:9, got %q", cfg.Media.Image.AspectRatio)
	}
	if cfg.Media.Image.PersonGeneration != "ALLOW_ADULT" {
		t.Errorf("expected person_generation ALLOW_ADULT, got %q", cfg.Media.Image.PersonGeneration)
	}
}
```

In `pkg/config/presets_test.go`, add:
```go
func TestGetGeminiImagePresets(t *testing.T) {
	presetIDs := []string{
		"imagen-3",
		"imagen-3-fast",
		"nano-banana-2",
		"nano-banana-2-lite",
		"nano-banana-pro",
		"nano-banana",
	}

	for _, id := range presetIDs {
		preset, ok := GetImagePreset(id)
		if !ok {
			t.Fatalf("expected preset %q to exist", id)
		}
		if preset.Type != "gemini" {
			t.Errorf("preset %q expected type gemini, got %q", id, preset.Type)
		}
		if preset.AspectRatio != "16:9" {
			t.Errorf("preset %q expected aspect_ratio 16:9, got %q", id, preset.AspectRatio)
		}
		if preset.PersonGeneration != "ALLOW_ADULT" {
			t.Errorf("preset %q expected person_generation ALLOW_ADULT, got %q", id, preset.PersonGeneration)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestConfigParsesImageTunables|TestGetGeminiImagePresets" ./pkg/config/`  
Expected: FAIL compilation errors (`AspectRatio undefined`, `preset not found`).

- [ ] **Step 3: Implement image config fields and presets**

In `pkg/config/types.go`, update `ImageConfig`:
```go
type ImageConfig struct {
	Type             string   `yaml:"type" json:"type"`
	BuiltinName      string   `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Command          string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args             []string `yaml:"args,omitempty" json:"args,omitempty"`
	Endpoint         string   `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Model            string   `yaml:"model,omitempty" json:"model,omitempty"`
	APIKey           string   `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	AutoGenerate     bool     `yaml:"auto_generate" json:"auto_generate"`
	BuiltinFallback  bool     `yaml:"builtin_fallback" json:"builtin_fallback"`
	AspectRatio      string   `yaml:"aspect_ratio,omitempty" json:"aspect_ratio,omitempty"`
	PersonGeneration string   `yaml:"person_generation,omitempty" json:"person_generation,omitempty"`
}
```

In `pkg/config/presets.go`, add inside `ImagePresets`:
```go
	"imagen-3": {
		Type:             "gemini",
		Model:            "imagen-3.0-generate-002",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"imagen-3-fast": {
		Type:             "gemini",
		Model:            "imagen-3.0-fast-generate-001",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-2": {
		Type:             "gemini",
		Model:            "gemini-3.1-flash-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-2-lite": {
		Type:             "gemini",
		Model:            "gemini-3.1-flash-lite-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana-pro": {
		Type:             "gemini",
		Model:            "gemini-3-pro-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
	"nano-banana": {
		Type:             "gemini",
		Model:            "gemini-2.5-flash-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run "TestConfigParsesImageTunables|TestGetGeminiImagePresets" ./pkg/config/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/config/types.go pkg/config/presets.go pkg/config/types_test.go pkg/config/presets_test.go
git commit -m "feat(config): add image aspect ratio, person generation, and Gemini image presets"
```

---

### Task 2: Cache Extension Detection & Content-Type Mapping

**Files:**
- Modify: `pkg/media/image.go:40-54` and `pkg/media/image.go:115-125`
- Modify: `pkg/gui/service.go:1611-1617`
- Test: `pkg/media/image_test.go`

- [ ] **Step 1: Write failing tests for image extension detection and caching**

In `pkg/media/image_test.go`, add:
```go
func TestArtExtensionDetectsMagicBytes(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"svg", []byte("<svg viewBox='0 0 100 100'></svg>"), ".svg"},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), ".png"},
		{"jpeg", []byte("\xff\xd8\xff\xe0\x00\x10JFIF"), ".jpg"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), ".webp"},
		{"unknown fallback", []byte("raw data"), ".webp"},
	}

	for _, tc := range cases {
		ext := artExtension(tc.data)
		if ext != tc.expected {
			t.Errorf("%s: expected %q, got %q", tc.name, tc.expected, ext)
		}
	}
}

func TestGenerateLocationImageCachesJPEG(t *testing.T) {
	cache := NewContentCache(t.TempDir())
	jpegBytes := []byte("\xff\xd8\xff\xe0\x00\x10JFIFdummy-jpeg-data")
	client := &stubImageClient{body: jpegBytes}
	pipeline := NewImagePipeline(client, cache)

	location := locationFixture()
	path, err := pipeline.GenerateLocationImage(context.Background(), location, "dark fantasy", "gemini:imagen-3", false)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".jpg" {
		t.Errorf("expected .jpg extension, got %q", filepath.Ext(path))
	}

	// Second call should return cached path without re-generating
	client.calls = 0
	cachedPath, err := pipeline.GenerateLocationImage(context.Background(), location, "dark fantasy", "gemini:imagen-3", false)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if cachedPath != path {
		t.Errorf("expected cached path %q, got %q", path, cachedPath)
	}
	if client.calls != 0 {
		t.Errorf("expected 0 calls on cache hit, got %d", client.calls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestArtExtensionDetectsMagicBytes|TestGenerateLocationImageCachesJPEG" ./pkg/media/`  
Expected: FAIL (`.jpg` expected but got `.webp`).

- [ ] **Step 3: Implement magic byte detection and cache lookup expansion**

In `pkg/media/image.go`:
Update `artExtension`:
```go
func artExtension(data []byte) string {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return ".svg"
	}
	if bytes.HasPrefix(head, []byte("\x89PNG")) {
		return ".png"
	}
	if bytes.HasPrefix(head, []byte("\xff\xd8\xff")) {
		return ".jpg"
	}
	return ".webp"
}
```

Update `GenerateLocationImage`:
```go
	if !force {
		for _, ext := range []string{".svg", ".webp", ".jpg", ".jpeg", ".png"} {
			if p.cache.Exists("images", base+ext) {
				return filepath.Join(p.cache.Subdir("images"), base+ext), nil
			}
		}
	}
```

In `pkg/gui/service.go`:
Update `contentTypeForArt`:
```go
func contentTypeForArt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "image/webp"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run "TestArtExtensionDetectsMagicBytes|TestGenerateLocationImageCachesJPEG" ./pkg/media/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/image.go pkg/media/image_test.go pkg/gui/service.go
git commit -m "feat(media): detect image magic bytes and support JPEG and PNG in art cache"
```

---

### Task 3: Core `GeminiImageClient` Implementation & Testing

**Files:**
- Create: `pkg/media/gemini_image.go`
- Create: `pkg/media/gemini_image_test.go`

- [ ] **Step 1: Write failing tests for GeminiImageClient (Credential Resolution, Imagen routing, Gemini Nano Banana routing)**

In `pkg/media/gemini_image_test.go`:
```go
package media_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestResolveGeminiImageAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	_, err := media.ResolveGeminiImageAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided")
	}

	// 2. Fallback to GOOGLE_API_KEY
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	k, err := media.ResolveGeminiImageAPIKey("", "")
	if err != nil || k != "env-google-key" {
		t.Errorf("expected env-google-key, got %q", k)
	}

	// 3. Fallback to GEMINI_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	k, err = media.ResolveGeminiImageAPIKey("", "")
	if err != nil || k != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q", k)
	}

	// 4. Shared provider key
	k, err = media.ResolveGeminiImageAPIKey("", "shared-key")
	if err != nil || k != "shared-key" {
		t.Errorf("expected shared-key, got %q", k)
	}

	// 5. Config image key override
	k, err = media.ResolveGeminiImageAPIKey("override-key", "shared-key")
	if err != nil || k != "override-key" {
		t.Errorf("expected override-key, got %q", k)
	}
}

func TestGeminiImageClientImagenGeneratesJPEG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "ancient castle") {
			t.Errorf("expected prompt text in request, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "16:9") {
			t.Errorf("expected 16:9 aspect ratio in request, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		// Simulate GenerateImages response with base64 JPEG
		fmt.Fprint(w, `{
			"generatedImages": [
				{
					"image": {
						"imageBytes": "/9j/4AAQSkZJRg==",
						"mimeType": "image/jpeg"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	imgClient, err := media.NewGeminiImageClientWithClient(client, config.ImageConfig{
		Model:            "imagen-3.0-generate-002",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	})
	if err != nil {
		t.Fatalf("NewGeminiImageClientWithClient: %v", err)
	}

	imgBytes, err := imgClient.GenerateImage(ctx, "ancient castle at sunset")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	if len(imgBytes) == 0 {
		t.Errorf("expected non-empty image bytes")
	}
	if !strings.HasPrefix(string(imgBytes), "\xff\xd8\xff") {
		t.Errorf("expected JPEG magic bytes, got %x", imgBytes[:min(len(imgBytes), 4)])
	}
}

func TestGeminiImageClientNanoBananaGeneratesImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "mystic grove") {
			t.Errorf("expected prompt text in request, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "IMAGE") {
			t.Errorf("expected IMAGE response modality, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		// Simulate GenerateContent response with inlineData JPEG
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"inlineData": {
									"data": "/9j/4AAQSkZJRg==",
									"mimeType": "image/jpeg"
								}
							}
						],
						"role": "model"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	imgClient, err := media.NewGeminiImageClientWithClient(client, config.ImageConfig{
		Model:            "gemini-3.1-flash-image",
		AspectRatio:      "16:9",
		PersonGeneration: "ALLOW_ADULT",
	})
	if err != nil {
		t.Fatalf("NewGeminiImageClientWithClient: %v", err)
	}

	imgBytes, err := imgClient.GenerateImage(ctx, "mystic grove under twin moons")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	if len(imgBytes) == 0 {
		t.Errorf("expected non-empty image bytes")
	}
	if !strings.HasPrefix(string(imgBytes), "\xff\xd8\xff") {
		t.Errorf("expected JPEG magic bytes, got %x", imgBytes[:min(len(imgBytes), 4)])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run "TestResolveGeminiImageAPIKey|TestGeminiImageClientImagenGeneratesJPEG|TestGeminiImageClientNanoBananaGeneratesImage" ./pkg/media/`  
Expected: FAIL (`undefined: media.ResolveGeminiImageAPIKey`, `undefined: media.NewGeminiImageClientWithClient`).

- [ ] **Step 3: Implement `pkg/media/gemini_image.go`**

Create `pkg/media/gemini_image.go`:
```go
package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/config"
)

var ErrGeminiImageAPIKeyRequired = errors.New("gemini: an API key is required for image generation; set media.image.api_key, providers.gemini.api_key, or GEMINI_API_KEY")

func ResolveGeminiImageAPIKey(imageKey, sharedKey string) (string, error) {
	if k := strings.TrimSpace(imageKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(sharedKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GEMINI_API_KEY")); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); k != "" {
		return k, nil
	}
	return "", ErrGeminiImageAPIKeyRequired
}

type GeminiImageClient struct {
	client           *genai.Client
	model            string
	aspectRatio      string
	personGeneration string
}

func NewGeminiImageClient(cfg config.ImageConfig, sharedKey string) (*GeminiImageClient, error) {
	apiKey, err := ResolveGeminiImageAPIKey(cfg.APIKey, sharedKey)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create image client: %w", err)
	}

	return NewGeminiImageClientWithClient(client, cfg)
}

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

func (g *GeminiImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("gemini image: prompt cannot be empty")
	}

	if strings.HasPrefix(g.model, "imagen-") {
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
	reqCfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{"IMAGE"},
		ImageConfig: &genai.ImageConfig{
			AspectRatio:      g.aspectRatio,
			PersonGeneration: g.personGeneration,
		},
	}

	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.model, contents, reqCfg)
	if err != nil {
		return nil, mapGeminiImageError(err)
	}

	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				return part.InlineData.Data, nil
			}
		}
	}

	return nil, errors.New("gemini image: no image data found in response candidates")
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run "TestResolveGeminiImageAPIKey|TestGeminiImageClientImagenGeneratesJPEG|TestGeminiImageClientNanoBananaGeneratesImage" ./pkg/media/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/gemini_image.go pkg/media/gemini_image_test.go
git commit -m "feat(media): implement GeminiImageClient supporting Imagen 3 and Nano Banana"
```

---

### Task 4: Provider Factory Wiring & Service Integration

**Files:**
- Modify: `pkg/media/providers.go:635-663`
- Modify: `pkg/gui/service.go:1230-1235` and `pkg/gui/service.go:2453-2457`
- Test: `pkg/media/providers_test.go`

- [ ] **Step 1: Write failing test for NewImageClient with Gemini provider**

In `pkg/media/providers_test.go`, add:
```go
func TestNewImageClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	client, err := media.NewImageClient(config.ImageConfig{
		Type:  "gemini",
		Model: "imagen-3.0-generate-002",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for gemini: %v", err)
	}
	if client == nil {
		t.Fatalf("expected non-nil image client")
	}

	builtinClient, err := media.NewImageClient(config.ImageConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-3.1-flash-image",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for builtin gemini: %v", err)
	}
	if builtinClient == nil {
		t.Fatalf("expected non-nil builtin gemini image client")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestNewImageClientBuildsGemini ./pkg/media/`  
Expected: FAIL (`unsupported image provider type: gemini`).

- [ ] **Step 3: Update `pkg/media/providers.go` and `pkg/gui/service.go`**

In `pkg/media/providers.go`:
Update `NewImageClient`:
```go
func NewImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewImageClientWithSharedKey(cfg, "")
}

func NewImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledImageClient{}, nil
	case "builtin":
		if cfg.BuiltinName == "procedural-art" {
			return NewProceduralArtClient(), nil
		}
		if cfg.BuiltinName == "gemini" {
			return NewGeminiImageClient(cfg, sharedKey)
		}
		return &echoImageClient{}, nil
	case "gemini":
		return NewGeminiImageClient(cfg, sharedKey)
	case "cli":
		return &cliImageClient{command: cfg.Command, args: cfg.Args}, nil
	case "comfyui":
		return &comfyUIImageClient{
			endpoint: cfg.Endpoint,
			client:   &http.Client{Timeout: 120 * time.Second},
		}, nil
	case "http":
		if isComfyUI(cfg.Endpoint) {
			return &comfyUIImageClient{
				endpoint: cfg.Endpoint,
				client:   &http.Client{Timeout: 120 * time.Second},
			}, nil
		}
		return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 60 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", cfg.Type)
	}
}

func NewSceneImageClient(cfg config.ImageConfig) (ImageClient, error) {
	return NewSceneImageClientWithSharedKey(cfg, "")
}

func NewSceneImageClientWithSharedKey(cfg config.ImageConfig, sharedKey string) (ImageClient, error) {
	primary, err := NewImageClientWithSharedKey(cfg, sharedKey)
	if err != nil {
		return nil, err
	}
	if !cfg.BuiltinFallback {
		return primary, nil
	}

	fallback, err := NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"})
	if err != nil {
		return nil, err
	}
	return &fallbackImageClient{primary: primary, fallback: fallback}, nil
}
```

In `pkg/gui/service.go`:
In `GetSceneArt` (around line 1230):
```go
	cfg := s.configMgr.Get()
	client, err := media.NewSceneImageClientWithSharedKey(cfg.Media.Image, cfg.Providers.Gemini.APIKey)
	if err != nil {
		return "", "", fmt.Errorf("build image client: %w", err)
	}
```

In `TestProvider` (around line 2450):
```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestNewImageClientBuildsGemini ./pkg/media/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go pkg/gui/service.go
git commit -m "feat(media): wire GeminiImageClient into NewImageClient factory with shared key support"
```

---

### Task 5: Frontend Types, Presets, and Settings Studio UI

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Update frontend types**

In `frontend/src/types.ts`:
Update `ImageConfig`:
```typescript
export interface ImageConfig {
  type: 'builtin' | 'http' | 'comfyui' | 'cli' | 'disabled' | 'gemini';
  builtin_name?: string;
  command?: string;
  args?: string[];
  endpoint?: string;
  model?: string;
  api_key?: string;
  auto_generate?: boolean;
  builtin_fallback?: boolean;
  aspect_ratio?: string;
  person_generation?: string;
}
```

- [ ] **Step 2: Add Gemini & Nano Banana presets in providerPresets.ts**

In `frontend/src/templates/providerPresets.ts`:
Add to `IMAGE_PRESETS`:
```typescript
  'imagen-3': {
    label: 'Google Imagen 3 (Cloud API)',
    description: 'High-fidelity cinematic and dark fantasy illustration via Google Imagen 3.0.',
    config: {
      type: 'gemini',
      model: 'imagen-3.0-generate-002',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
  'imagen-3-fast': {
    label: 'Google Imagen 3 Fast (Cloud API)',
    description: 'Rapid turnaround low-latency generation for turn-by-turn scene updates.',
    config: {
      type: 'gemini',
      model: 'imagen-3.0-fast-generate-001',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
  'nano-banana-2': {
    label: 'Google Nano Banana 2 (Gemini 3.1 Flash Image)',
    description: 'Generalist native Gemini image model balancing speed, 4K rendering, and scene consistency.',
    config: {
      type: 'gemini',
      model: 'gemini-3.1-flash-image',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
  'nano-banana-2-lite': {
    label: 'Google Nano Banana 2 Lite (Gemini 3.1 Flash-Lite Image)',
    description: 'Fastest and most lightweight native Gemini image generator.',
    config: {
      type: 'gemini',
      model: 'gemini-3.1-flash-lite-image',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
  'nano-banana-pro': {
    label: 'Google Nano Banana Pro (Gemini 3 Pro Image)',
    description: 'Complex visual composition, deep world knowledge, and fine creative steering.',
    config: {
      type: 'gemini',
      model: 'gemini-3-pro-image',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
  'nano-banana': {
    label: 'Google Nano Banana Original (Gemini 2.5 Flash Image)',
    description: 'Original high-volume low-latency Gemini image generator.',
    config: {
      type: 'gemini',
      model: 'gemini-2.5-flash-image',
      aspect_ratio: '16:9',
      person_generation: 'ALLOW_ADULT',
      auto_generate: false,
    },
  },
```

- [ ] **Step 3: Update SettingsStudio.tsx with Gemini/Imagen UI controls**

In `frontend/src/components/SettingsStudio.tsx`:
1. In the `Image Provider Type` `<select>`:
   Add option:
   ```tsx
   <option value="gemini">Google Gemini / Imagen (GenAI Cloud)</option>
   ```
2. When `config.media.image.type === 'gemini'` (or `builtin_name === 'gemini'`):
   Render:
   - Model input with preset quick-select pills (`imagen-3`, `imagen-3-fast`, `nano-banana-2`, `nano-banana-2-lite`, `nano-banana-pro`, `nano-banana`)
   - Aspect Ratio selector:
     ```tsx
     <select
       value={config.media.image.aspect_ratio || '16:9'}
       onChange={(e) => setConfig({
         ...config,
         media: { ...config.media, image: { ...config.media.image, aspect_ratio: e.target.value } }
       })}
     >
       <option value="16:9">16:9 (Cinematic Widescreen - Default)</option>
       <option value="1:1">1:1 (Square)</option>
       <option value="4:3">4:3 (Landscape)</option>
       <option value="3:4">3:4 (Portrait)</option>
       <option value="9:16">9:16 (Vertical)</option>
       <option value="21:9">21:9 (Ultrawide)</option>
     </select>
     ```
   - Person Generation selector:
     ```tsx
     <select
       value={config.media.image.person_generation || 'ALLOW_ADULT'}
       onChange={(e) => setConfig({
         ...config,
         media: { ...config.media, image: { ...config.media.image, person_generation: e.target.value } }
       })}
     >
       <option value="ALLOW_ADULT">ALLOW_ADULT (Default — Adults, NPCs, Guards)</option>
       <option value="ALLOW_ALL">ALLOW_ALL (All Characters & Children)</option>
       <option value="DONT_ALLOW">DONT_ALLOW (No Characters / Landscapes Only)</option>
     </select>
     ```
   - API Key override field with helper text pointing to `config.providers.gemini.api_key`.

- [ ] **Step 4: Run frontend TypeScript verification**

Run: `mise run test:frontend`  
Expected: PASS with 0 type errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Gemini Imagen and Nano Banana image presets and UI controls"
```

---

### Task 6: Full Verification & Integration Check

**Files:**
- All touched files

- [ ] **Step 1: Run complete backend test suite**

Run: `mise run test:backend`  
Expected: PASS (all packages `./...` exit code 0)

- [ ] **Step 2: Run linter**

Run: `mise run lint`  
Expected: PASS (`go vet ./...` clean)

- [ ] **Step 3: Run complete frontend build**

Run: `mise run build:frontend`  
Expected: PASS (Vite bundles successfully into `pkg/gui/dist`)

- [ ] **Step 4: Restore `.gitkeep` if removed by Vite build**

Run: `git checkout pkg/gui/dist/.gitkeep 2>/dev/null || true`

- [ ] **Step 5: Run full binary build**

Run: `mise run build:backend`  
Expected: PASS (`bin/localrpg` built)
