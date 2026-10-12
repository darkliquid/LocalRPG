package imagehttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestHTTPImageClient_DecodesA1111Base64(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\nfake-png-content")
	encoded := base64.StdEncoding.EncodeToString(rawPng)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["prompt"] != "a misty graveyard" {
			t.Errorf("unexpected prompt: %v", req["prompt"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"images": []string{encoded},
		})
	}))
	defer ts.Close()

	client := NewHTTPImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + "/sdapi/v1/txt2img",
	})

	data, err := client.GenerateImage(context.Background(), "a misty graveyard")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected decoded png bytes, got %v", data)
	}
}

func TestHTTPImageClient_DecodesOpenAIBase64(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\nopenai-image-bytes")
	encoded := base64.StdEncoding.EncodeToString(rawPng)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"b64_json": encoded},
			},
		})
	}))
	defer ts.Close()

	client := NewHTTPImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + "/v1/images/generations",
	})

	data, err := client.GenerateImage(context.Background(), "a glowing orb")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected decoded png bytes, got %v", data)
	}
}

func TestComfyUIImageClient_GeneratesImage(t *testing.T) {
	rawPng := []byte("\x89PNG\r\n\x1a\ncomfy-image-bytes")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prompt":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"prompt_id": "prompt-123"})
		case "/history/prompt-123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt-123": map[string]any{
					"outputs": map[string]any{
						"9": map[string]any{
							"images": []map[string]string{
								{"filename": "out_001.png", "subfolder": "", "type": "output"},
							},
						},
					},
				},
			})
		case "/view":
			if r.URL.Query().Get("filename") != "out_001.png" {
				t.Errorf("unexpected filename query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(rawPng)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := NewHTTPImageClient(config.ImageConfig{
		Type:     "http",
		Endpoint: ts.URL + ":8188", // ComfyUI endpoint marker
	})

	data, err := client.GenerateImage(context.Background(), "ancient citadel")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !bytes.Equal(data, rawPng) {
		t.Errorf("expected comfyui image bytes, got %v", data)
	}
}
