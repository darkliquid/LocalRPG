# Design Specification: Google Gemini & Imagen Image Generation Provider

- **Date**: 2026-09-23
- **Author**: Antigravity
- **Status**: Approved
- **Topic**: Google Imagen 3 and Native Gemini Image ("Nano Banana") Generation Provider for Scene Art

## 1. Overview & Goals

LocalRPG generates atmospheric visual art for locations and story scenes. Currently, image generation is handled via:
- Built-in zero-GPU vector generator (`procedural-art` SVG)
- Local HTTP generators (ComfyUI on port 8188, AUTOMATIC1111 on port 7860, LocalAI)
- OpenAI DALL-E 3 HTTP endpoint
- Local CLI subprocesses (`sd-cli`)

This specification defines Phase 2 of the Google Gemini integration: a native Go image generation client (`GeminiImageClient`) in `pkg/media` using the official `google.golang.org/genai` SDK. It provides first-class support for both the standalone **Imagen 3** model family and the native multimodal **Gemini Image ("Nano Banana")** model family.

## 2. Model Families & Routing

The provider supports two distinct model families through a unified `ImageClient` interface:

### 2.1 Dedicated Imagen 3 Models
- Models: `imagen-3.0-generate-002` (standard high-fidelity), `imagen-3.0-fast-generate-001` (low-latency).
- SDK Execution: Invokes `client.Models.GenerateImages(ctx, model, prompt, cfg)` where `cfg` is `*genai.GenerateImagesConfig`:
  - `NumberOfImages: 1`
  - `OutputMIMEType: "image/jpeg"`
  - `AspectRatio`: Configured aspect ratio (default: `"16:9"`)
  - `PersonGeneration`: Configured person generation policy (default: `genai.PersonGenerationAllowAdult`)
- Response Processing: Extracts raw image bytes from `resp.GeneratedImages[0].Image.ImageBytes`.

### 2.2 Native Gemini Image ("Nano Banana") Models
- Models: `gemini-3.1-flash-image` (Nano Banana 2), `gemini-3.1-flash-lite-image` (Nano Banana 2 Lite), `gemini-3-pro-image` (Nano Banana Pro), `gemini-2.5-flash-image` (Nano Banana Original).
- SDK Execution: Invokes `client.Models.GenerateContent(ctx, model, contents, cfg)` where:
  - `contents` contains the scene prompt: `[]*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: prompt}}}}`
  - `cfg` is `*genai.GenerateContentConfig`:
    - `ResponseModalities: []string{"IMAGE"}`
    - `ImageConfig: &genai.ImageConfig{AspectRatio: aspectRatio, PersonGeneration: personGen}`
- Response Processing: Iterates candidates and parts for `part.InlineData`, extracting bytes from `part.InlineData.Data`.

## 3. Configuration & Credential Resolution

### 3.1 Credential Resolution
The client resolves API keys in priority order:
1. `config.Media.Image.APIKey` (per-provider override)
2. `config.Providers.Gemini.APIKey` (shared workspace Gemini key configured in Phase 1)
3. `GEMINI_API_KEY` environment variable
4. `GOOGLE_API_KEY` environment variable
5. If none is found, return `ErrGeminiAPIKeyRequired`.

### 3.2 Configuration Schema (`pkg/config/types.go` and `frontend/src/types.ts`)
Add tunables to `ImageConfig`:
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

Allowed values:
- `aspect_ratio`: `"16:9"` (default), `"1:1"`, `"4:3"`, `"3:4"`, `"9:16"`, `"21:9"`
- `person_generation`: `"ALLOW_ADULT"` (default), `"ALLOW_ALL"`, `"DONT_ALLOW"`

### 3.3 Presets (`pkg/config/presets.go` & `frontend/src/templates/providerPresets.ts`)
Add presets for:
- `imagen-3`: Google Imagen 3 (`imagen-3.0-generate-002`, `16:9`, `ALLOW_ADULT`)
- `imagen-3-fast`: Google Imagen 3 Fast (`imagen-3.0-fast-generate-001`, `16:9`, `ALLOW_ADULT`)
- `nano-banana-2`: Google Nano Banana 2 (`gemini-3.1-flash-image`, `16:9`, `ALLOW_ADULT`)
- `nano-banana-2-lite`: Google Nano Banana 2 Lite (`gemini-3.1-flash-lite-image`, `16:9`, `ALLOW_ADULT`)
- `nano-banana-pro`: Google Nano Banana Pro (`gemini-3-pro-image`, `16:9`, `ALLOW_ADULT`)
- `nano-banana`: Google Nano Banana Original (`gemini-2.5-flash-image`, `16:9`, `ALLOW_ADULT`)

## 4. Cache & Content Type Handling

1. **Magic Bytes Extension Detection (`pkg/media/image.go`)**:
   `artExtension(data []byte) string` detects format from bytes:
   - `<svg` -> `.svg`
   - `\x89PNG` -> `.png`
   - `\xff\xd8\xff` -> `.jpg`
   - Fallback -> `.webp`

2. **Cache Existence Lookups**:
   `GenerateLocationImage` checks existence of `base + ext` for `[".svg", ".webp", ".jpg", ".jpeg", ".png"]`, preventing duplicate API calls when scene art exists in cache.

3. **Content-Type Mapping (`pkg/gui/service.go`)**:
   `contentTypeForArt(path string)` maps file extensions:
   - `.svg` -> `"image/svg+xml"`
   - `.jpg`, `.jpeg` -> `"image/jpeg"`
   - `.png` -> `"image/png"`
   - Default -> `"image/webp"`

## 5. Settings Studio UI Integration

In `frontend/src/components/SettingsStudio.tsx`:
1. Add `"Google Gemini / Imagen (GenAI Cloud)"` (`value="gemini"`) to **Image Provider Type**.
2. When `type === "gemini"` or `builtin_name === "gemini"`:
   - Preset selector pills (`imagen-3`, `imagen-3-fast`, `nano-banana-2`, `nano-banana-2-lite`, `nano-banana-pro`, `nano-banana`)
   - Freeform model input field
   - **Aspect Ratio** dropdown selector
   - **Person Generation** dropdown selector
   - **API Key** override field pointing to shared Gemini provider key fallback
3. Test button: Calls `POST /api/settings/test-provider` with category `"image"`, running live generation with the configured prompt.

## 6. Verification & Testing

- Unit tests in `pkg/media/gemini_image_test.go` with mock `httptest.Server`:
  - Verify credential resolution priority
  - Verify `imagen-*` calls `GenerateImages` and extracts JPEG bytes
  - Verify `gemini-*-image` calls `GenerateContent` and extracts inline image data
  - Verify error handling (401/403 auth, 429 quota, 404 model not found)
- Unit tests in `pkg/media/image_test.go`:
  - Verify `artExtension` detects `.jpg`, `.png`, `.svg`, `.webp`
  - Verify `GenerateLocationImage` caches and reuses `.jpg` files
- Frontend type check: `mise run test:frontend`
- Backend test suite: `mise run test:backend`
- Linter: `mise run lint`
- Full build: `mise run build`
