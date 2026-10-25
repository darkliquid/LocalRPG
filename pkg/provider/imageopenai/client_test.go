package imageopenai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestOpenAIImageShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake")))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.ImageConfig{
		Type: "openai", Endpoint: server.URL, Model: "gpt-image-2.5-flare",
		APIKey: "k", AspectRatio: "3:2", Quality: "high",
	}, "")
	img, err := client.GenerateImage(context.Background(), "a windswept moor")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(img) != "\x89PNG-fake" {
		t.Errorf("image = %q", img)
	}
	if _, sent := body["response_format"]; sent {
		t.Errorf("response_format sent; gpt-image models reject it: %v", body)
	}
	if body["size"] != "1536x1024" || body["quality"] != "high" || body["model"] != "gpt-image-2.5-flare" || body["n"] != 1.0 {
		t.Errorf("body = %v", body)
	}
	if body["output_format"] != "png" {
		t.Errorf("output_format = %v", body["output_format"])
	}
}

func TestSizeFor(t *testing.T) {
	tests := map[string]string{
		"1:1":   "1024x1024",
		"3:2":   "1536x1024",
		"16:9":  "1536x1024",
		"2:3":   "1024x1536",
		"":      "1024x1024",
		"bogus": "1024x1024",
	}
	for ratio, want := range tests {
		if got := sizeFor(ratio); got != want {
			t.Errorf("sizeFor(%q) = %q, want %q", ratio, got, want)
		}
	}
}

func TestOpenAIImageErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.ImageConfig{Type: "openai", Endpoint: server.URL}, "")
	_, err := client.GenerateImage(context.Background(), "x")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestOpenAIImageAutoQualityOmitted(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString([]byte("x")))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.ImageConfig{Type: "openai", Endpoint: server.URL, Quality: "auto"}, "shared")
	if _, err := client.GenerateImage(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if _, sent := body["quality"]; sent {
		t.Errorf("quality sent for auto: %v", body)
	}
	if body["model"] != "gpt-image-2.5-flare" {
		t.Errorf("default model = %v", body["model"])
	}
}
