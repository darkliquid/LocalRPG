package imagehttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// NewHTTPImageClient builds the image client for an endpoint, choosing the
// ComfyUI API when the endpoint names it.
func NewHTTPImageClient(cfg config.ImageConfig) media.ImageClient {
	if cfg.Type == "comfyui" || isComfyUI(cfg.Endpoint) {
		return &comfyUIImageClient{endpoint: cfg.Endpoint, client: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second}}
	}
	return &httpImageClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 60 * time.Second}}
}

func isComfyUI(endpoint string) bool {
	return strings.Contains(endpoint, ":8188") || strings.HasSuffix(endpoint, "/prompt")
}

func getComfyBaseURL(endpoint string) string {
	ep := strings.TrimRight(endpoint, "/")
	ep = strings.TrimSuffix(ep, "/prompt")
	if strings.HasSuffix(ep, ":8188") && strings.Count(ep, ":") > 2 {
		ep = strings.TrimSuffix(ep, ":8188")
	}
	return ep
}

type comfyUIImageClient struct {
	endpoint string
	client   *http.Client
}

func (c *comfyUIImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	baseURL := getComfyBaseURL(c.endpoint)

	workflow := map[string]interface{}{
		"prompt": map[string]interface{}{
			"3": map[string]interface{}{
				"inputs": map[string]interface{}{
					"seed":         156680208700286,
					"steps":        20,
					"cfg":          8,
					"sampler_name": "euler",
					"scheduler":    "normal",
					"denoise":      1,
					"model":        []interface{}{"4", 0},
					"positive":     []interface{}{"6", 0},
					"negative":     []interface{}{"7", 0},
					"latent_image": []interface{}{"5", 0},
				},
				"class_type": "KSampler",
			},
			"4": map[string]interface{}{
				"inputs": map[string]interface{}{
					"ckpt_name": "v1-5-pruned-emaonly.ckpt",
				},
				"class_type": "CheckpointLoaderSimple",
			},
			"5": map[string]interface{}{
				"inputs": map[string]interface{}{
					"width":      512,
					"height":     512,
					"batch_size": 1,
				},
				"class_type": "EmptyLatentImage",
			},
			"6": map[string]interface{}{
				"inputs": map[string]interface{}{
					"text": prompt,
					"clip": []interface{}{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"7": map[string]interface{}{
				"inputs": map[string]interface{}{
					"text": "bad quality, blurry",
					"clip": []interface{}{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"8": map[string]interface{}{
				"inputs": map[string]interface{}{
					"samples": []interface{}{"3", 0},
					"vae":     []interface{}{"4", 2},
				},
				"class_type": "VAEDecode",
			},
			"9": map[string]interface{}{
				"inputs": map[string]interface{}{
					"filename_prefix": "LocalRPG",
					"images":          []interface{}{"8", 0},
				},
				"class_type": "SaveImage",
			},
		},
	}

	bodyBytes, err := json.Marshal(workflow)
	if err != nil {
		return nil, fmt.Errorf("marshal comfyui prompt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/prompt", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create comfyui prompt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post comfyui prompt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("comfyui prompt failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}

	var promptResp struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promptResp); err != nil {
		return nil, fmt.Errorf("decode comfyui prompt response: %w", err)
	}
	if promptResp.PromptID == "" {
		return nil, fmt.Errorf("comfyui returned empty prompt_id")
	}

	pollTicker := time.NewTicker(200 * time.Millisecond)
	defer pollTicker.Stop()

	timeout := time.After(60 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for comfyui image generation")
		case <-pollTicker.C:
			histReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/history/%s", baseURL, promptResp.PromptID), nil)
			if err != nil {
				return nil, err
			}
			histResp, err := c.client.Do(histReq)
			if err != nil {
				continue
			}
			if histResp.StatusCode != http.StatusOK {
				histResp.Body.Close()
				continue
			}

			var histData map[string]interface{}
			err = json.NewDecoder(histResp.Body).Decode(&histData)
			histResp.Body.Close()
			if err != nil {
				continue
			}

			pData, ok := histData[promptResp.PromptID].(map[string]interface{})
			if !ok {
				continue
			}
			outputs, ok := pData["outputs"].(map[string]interface{})
			if !ok {
				continue
			}

			var filename, subfolder, imgType string
			found := false
			for _, nodeOut := range outputs {
				nodeMap, ok := nodeOut.(map[string]interface{})
				if !ok {
					continue
				}
				images, ok := nodeMap["images"].([]interface{})
				if !ok || len(images) == 0 {
					continue
				}
				imgMap, ok := images[0].(map[string]interface{})
				if !ok {
					continue
				}
				filename, _ = imgMap["filename"].(string)
				subfolder, _ = imgMap["subfolder"].(string)
				imgType, _ = imgMap["type"].(string)
				if filename != "" {
					found = true
					break
				}
			}

			if !found {
				continue
			}

			viewURL := fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s", baseURL, filename, subfolder, imgType)
			viewReq, err := http.NewRequestWithContext(ctx, "GET", viewURL, nil)
			if err != nil {
				return nil, fmt.Errorf("create comfyui view request: %w", err)
			}
			viewResp, err := c.client.Do(viewReq)
			if err != nil {
				return nil, fmt.Errorf("fetch comfyui image: %w", err)
			}
			defer viewResp.Body.Close()

			if viewResp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(io.LimitReader(viewResp.Body, provider.MaxProviderDetailBytes))
				return nil, fmt.Errorf("comfyui view failed (%d): %s", viewResp.StatusCode, provider.TruncateDetail(b))
			}

			return io.ReadAll(viewResp.Body)
		}
	}
}

type httpImageClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

func (h *httpImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if isComfyUI(h.endpoint) {
		comfyClient := &comfyUIImageClient{
			endpoint: h.endpoint,
			client:   h.client,
		}
		return comfyClient.GenerateImage(ctx, prompt)
	}

	var payload []byte
	if strings.Contains(h.endpoint, "/sdapi/v1/txt2img") {
		payload, _ = json.Marshal(map[string]interface{}{
			"prompt":          prompt,
			"negative_prompt": "blurry, low quality, deformed",
			"steps":           20,
			"width":           512,
			"height":          512,
		})
	} else {
		payloadMap := map[string]interface{}{
			"prompt":          prompt,
			"n":               1,
			"size":            "512x512",
			"response_format": "b64_json",
		}
		if h.model != "" {
			payloadMap["model"] = h.model
		}
		payload, _ = json.Marshal(payloadMap)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("http image failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 1. Raw Binary detection: PNG, JPEG, WebP
	if bytes.HasPrefix(bodyBytes, []byte("\x89PNG")) ||
		bytes.HasPrefix(bodyBytes, []byte("\xff\xd8\xff")) ||
		(bytes.HasPrefix(bodyBytes, []byte("RIFF")) && bytes.Contains(bodyBytes[:min(len(bodyBytes), 16)], []byte("WEBP"))) {
		return bodyBytes, nil
	}

	// 2. Parse JSON response
	var respData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return bodyBytes, nil
	}

	// AUTOMATIC1111 / Forge WebUI: {"images": ["base64..."]}
	if images, ok := respData["images"].([]interface{}); ok && len(images) > 0 {
		if imgStr, ok := images[0].(string); ok {
			if idx := strings.Index(imgStr, ","); idx != -1 && strings.HasPrefix(imgStr, "data:") {
				imgStr = imgStr[idx+1:]
			}
			decoded, err := base64.StdEncoding.DecodeString(imgStr)
			if err != nil {
				return nil, fmt.Errorf("decode a1111 base64: %w", err)
			}
			return decoded, nil
		}
	}

	// OpenAI format: {"data": [{"b64_json": "..."} | {"url": "..."}]}
	if data, ok := respData["data"].([]interface{}); ok && len(data) > 0 {
		if item, ok := data[0].(map[string]interface{}); ok {
			if b64, ok := item["b64_json"].(string); ok && b64 != "" {
				decoded, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, fmt.Errorf("decode openai b64_json: %w", err)
				}
				return decoded, nil
			}
			if imgURL, ok := item["url"].(string); ok && imgURL != "" {
				getReq, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
				if err != nil {
					return nil, fmt.Errorf("create image get request: %w", err)
				}
				getResp, err := h.client.Do(getReq)
				if err != nil {
					return nil, fmt.Errorf("fetch image url: %w", err)
				}
				defer getResp.Body.Close()
				if getResp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(io.LimitReader(getResp.Body, provider.MaxProviderDetailBytes))
					return nil, fmt.Errorf("fetch image url failed (%d): %s", getResp.StatusCode, provider.TruncateDetail(body))
				}
				return io.ReadAll(getResp.Body)
			}
		}
	}

	return bodyBytes, nil
}

// Factory Constructors
