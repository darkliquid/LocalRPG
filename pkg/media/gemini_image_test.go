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

func TestGeminiImageErrorMapping(t *testing.T) {
	cases := []struct {
		errStr   string
		expected string
	}{
		{"401 Unauthorized", "gemini image: invalid API key or permission denied"},
		{"403 Forbidden: PERMISSION_DENIED", "gemini image: invalid API key or permission denied"},
		{"429 RESOURCE_EXHAUSTED", "gemini image: quota exceeded or rate limit reached"},
		{"404 NOT_FOUND: models/unknown", "gemini image: model not found"},
	}

	for _, tc := range cases {
		mapped := media.MapGeminiImageErrorForTest(fmt.Errorf("%s", tc.errStr))
		if !strings.Contains(mapped.Error(), tc.expected) {
			t.Errorf("error %q mapped to %q, want %q", tc.errStr, mapped.Error(), tc.expected)
		}
	}
}
