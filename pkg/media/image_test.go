package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type stubImageClient struct {
	calls int
	body  []byte
}

func (c *stubImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	c.calls++
	return c.body, nil
}

func locationFixture() *entity.Entity {
	return &entity.Entity{
		ID:       "alden-tavern",
		Name:     "Alden Tavern",
		Type:     "location",
		Tags:     []string{"tavern", "safehouse"},
		Body:     "A quiet tavern at the edge of the woods.",
		Location: "[[aldor]]",
	}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestLocationImageExtensionFollowsTheBytes(t *testing.T) {
	svg := &stubImageClient{body: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)}
	pipeline := NewImagePipeline(svg, NewContentCache(t.TempDir()))

	path, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false, true)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".svg" {
		t.Errorf("path = %q, want an .svg extension for SVG bytes", path)
	}

	again, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false, true)
	if err != nil {
		t.Fatalf("second GenerateLocationImage failed: %v", err)
	}
	if again != path {
		t.Errorf("expected a cache hit at %q, got %q", path, again)
	}
	if svg.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", svg.calls)
	}
}

func TestLocationImageRasterAndForce(t *testing.T) {
	raster := &stubImageClient{body: []byte("\x89PNG\r\n\x1a\nbinary")}
	pipeline := NewImagePipeline(raster, NewContentCache(t.TempDir()))

	path, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", false, true)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("path = %q, want a .png extension for PNG bytes", path)
	}

	if _, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "dark fantasy", "builtin:", true, true); err != nil {
		t.Fatalf("forced GenerateLocationImage failed: %v", err)
	}
	if raster.calls != 2 {
		t.Errorf("expected the force flag to bypass the cache, got %d calls", raster.calls)
	}
}

func TestAppearanceHashIgnoresProseAndTracksState(t *testing.T) {
	location := locationFixture()
	providerParams := "builtin:"

	original := AppearanceHash(location, providerParams, true)

	// Extraction appends to the body nearly every turn; that must not move the key.
	location.Body += " The floorboards creak."
	if AppearanceHash(location, providerParams, true) != original {
		t.Errorf("the body must not affect the cache key")
	}

	// Neither does tag order.
	location.Tags = []string{"safehouse", "tavern"}
	if AppearanceHash(location, providerParams, true) != original {
		t.Errorf("tag order must not affect the cache key")
	}

	// Structured state does.
	location.InitState(map[string]interface{}{"burned": true})
	if AppearanceHash(location, providerParams, true) == original {
		t.Errorf("state changes must move the cache key")
	}

	// An authored appearance overrides both.
	location.Appearance = "gutted by fire"
	withAppearance := AppearanceHash(location, providerParams, true)
	if withAppearance == original {
		t.Errorf("an appearance change must move the cache key")
	}

	location.State = nil
	if AppearanceHash(location, providerParams, true) != withAppearance {
		t.Errorf("an authored appearance must be the only input")
	}

	// A different provider model must not reuse another model's art.
	if AppearanceHash(location, "comfyui:sd-xl", true) == withAppearance {
		t.Errorf("provider parameters must move the cache key")
	}
}

func TestBuildLocationPromptPrefersTheAppearanceField(t *testing.T) {
	location := locationFixture()
	location.Body = strings.Repeat("prose ", 100)
	location.Appearance = "gutted by fire"

	prompt := BuildLocationPrompt(location, "dark fantasy", true)
	if !strings.Contains(prompt, "gutted by fire") {
		t.Errorf("expected the appearance field, got %q", prompt)
	}
	if strings.Contains(prompt, "prose") {
		t.Errorf("expected the body to be left out when an appearance exists, got %q", prompt)
	}

	location.Appearance = ""
	prompt = BuildLocationPrompt(location, "dark fantasy", true)
	if !strings.Contains(prompt, "Alden Tavern") || !strings.Contains(prompt, "dark fantasy") || !strings.Contains(prompt, "tavern") {
		t.Errorf("expected name, tags, and style, got %q", prompt)
	}
	if len(prompt) > 512 {
		t.Errorf("expected the body excerpt to be bounded, got %d characters", len(prompt))
	}
}

func TestProceduralArtVariesByContentNotLength(t *testing.T) {
	client := NewProceduralArtClient()

	first, err := client.GenerateImage(context.Background(), "market square")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	repeat, err := client.GenerateImage(context.Background(), "market square")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(first, repeat) {
		t.Errorf("identical prompts must produce identical art")
	}

	// Same length, different words: the old length-based seed collided here.
	other, err := client.GenerateImage(context.Background(), "square market")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if bytes.Equal(first, other) {
		t.Errorf("equal-length prompts must not share a layout")
	}

	if !strings.Contains(strings.ToLower(string(first)), "<svg") {
		t.Errorf("expected SVG art")
	}
}

func TestGeneratedArtIsWritableAndDecodable(t *testing.T) {
	// A raster stub proves the pipeline round-trips bytes it did not synthesise.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	pipeline := NewImagePipeline(&stubImageClient{body: pngBytes(t, img)}, NewContentCache(t.TempDir()))
	path, err := pipeline.GenerateLocationImage(context.Background(), locationFixture(), "", "test:", false, true)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("expected decodable PNG bytes, got %v", err)
	}
}

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
	path, err := pipeline.GenerateLocationImage(context.Background(), location, "dark fantasy", "gemini:imagen-3", false, true)
	if err != nil {
		t.Fatalf("GenerateLocationImage failed: %v", err)
	}
	if filepath.Ext(path) != ".jpg" {
		t.Errorf("expected .jpg extension, got %q", filepath.Ext(path))
	}

	// Second call should return cached path without re-generating
	client.calls = 0
	cachedPath, err := pipeline.GenerateLocationImage(context.Background(), location, "dark fantasy", "gemini:imagen-3", false, true)
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

func TestAppearanceHashFollowsTheLayerDecision(t *testing.T) {
	loc := &entity.Entity{
		ID:                "hall",
		Name:              "The Hall",
		Appearance:        "A vaulted stone hall.",
		CurrentAppearance: "Flooded.",
	}

	layered := AppearanceHash(loc, "builtin:", true)
	if layered == AppearanceHash(loc, "builtin:", false) {
		t.Error("layering the current appearance should change the key")
	}
	if layered != AppearanceHash(loc, "builtin:", true) {
		t.Error("the same decision should produce the same key")
	}

	original := AppearanceHash(loc, "builtin:", false)
	loc.CurrentAppearance = "On fire."
	if AppearanceHash(loc, "builtin:", false) != original {
		t.Error("with the current appearance ignored, a change to it must not move the key")
	}
	if AppearanceHash(loc, "builtin:", true) == layered {
		t.Error("with the current appearance layered, a change to it must move the key")
	}
}
