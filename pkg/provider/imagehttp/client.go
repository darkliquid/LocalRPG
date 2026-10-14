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
	"sync"
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

// usageFromBody overrides the estimated usage when the JSON body carries one.
func (h *httpImageClient) usageFromBody(body map[string]any) {
	usage, ok := body["usage"].(map[string]any)
	if !ok {
		return
	}
	input, output := 0, 0
	if value, ok := usage["input_tokens"].(float64); ok {
		input = int(value)
	}
	if value, ok := usage["output_tokens"].(float64); ok {
		output = int(value)
	}
	if input == 0 && output == 0 {
		return
	}
	h.setLastUsage(media.Usage{InputTokens: input, OutputTokens: output, Requests: 1})
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

// LastUsage reports one request; ComfyUI advertises no token or character count,
// so there is nothing to record and the zero value is the answer.
func (c *comfyUIImageClient) LastUsage() media.Usage {
	return media.Usage{}
}

// comfyImageRef is the output a ComfyUI prompt produced: the file the view endpoint
// serves it by.
type comfyImageRef struct {
	filename  string
	subfolder string
	kind      string
}

// comfyWorkflow is the ComfyUI graph a generation runs: one checkpoint, one sampler, one
// 512x512 latent, the prompt and its negative counterpart, a decode and a save. It is
// written out rather than loaded from a file because the node ids are what the graph
// wires together, and a wrong one is a prompt the provider rejects.
func comfyWorkflow(prompt string) map[string]any {
	return map[string]any{
		"prompt": map[string]any{
			"3": map[string]any{
				"inputs": map[string]any{
					"seed":         156680208700286,
					"steps":        20,
					"cfg":          8,
					"sampler_name": "euler",
					"scheduler":    "normal",
					"denoise":      1,
					"model":        []any{"4", 0},
					"positive":     []any{"6", 0},
					"negative":     []any{"7", 0},
					"latent_image": []any{"5", 0},
				},
				"class_type": "KSampler",
			},
			"4": map[string]any{
				"inputs": map[string]any{
					"ckpt_name": "v1-5-pruned-emaonly.ckpt",
				},
				"class_type": "CheckpointLoaderSimple",
			},
			"5": map[string]any{
				"inputs": map[string]any{
					"width":      512,
					"height":     512,
					"batch_size": 1,
				},
				"class_type": "EmptyLatentImage",
			},
			"6": map[string]any{
				"inputs": map[string]any{
					"text": prompt,
					"clip": []any{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"7": map[string]any{
				"inputs": map[string]any{
					"text": "bad quality, blurry",
					"clip": []any{"4", 1},
				},
				"class_type": "CLIPTextEncode",
			},
			"8": map[string]any{
				"inputs": map[string]any{
					"samples": []any{"3", 0},
					"vae":     []any{"4", 2},
				},
				"class_type": "VAEDecode",
			},
			"9": map[string]any{
				"inputs": map[string]any{
					"filename_prefix": "LocalRPG",
					"images":          []any{"8", 0},
				},
				"class_type": "SaveImage",
			},
		},
	}
}

func (c *comfyUIImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	baseURL := getComfyBaseURL(c.endpoint)

	workflow := comfyWorkflow(prompt)

	bodyBytes, err := json.Marshal(workflow)
	if err != nil {
		return nil, fmt.Errorf("marshal comfyui prompt: %w", err)
	}

	promptID, err := c.submitComfyPrompt(ctx, baseURL, bodyBytes)
	if err != nil {
		return nil, err
	}

	return c.awaitComfyImage(ctx, baseURL, promptID)
}

// submitComfyPrompt queues the workflow and returns the id the prompt is polled under.
func (c *comfyUIImageClient) submitComfyPrompt(ctx context.Context, baseURL string, bodyBytes []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/prompt", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create comfyui prompt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("post comfyui prompt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return "", fmt.Errorf("comfyui prompt failed (%d): %s", resp.StatusCode, provider.TruncateDetail(b))
	}

	var promptResp struct {
		PromptID string `json:"prompt_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promptResp); err != nil {
		return "", fmt.Errorf("decode comfyui prompt response: %w", err)
	}
	if promptResp.PromptID == "" {
		return "", fmt.Errorf("comfyui returned empty prompt_id")
	}
	return promptResp.PromptID, nil
}

// awaitComfyImage polls the provider until the queued prompt has produced an image, and
// fetches it. A poll the provider refuses is retried on the next tick rather than given up
// on: the prompt is still running server-side, and a dropped connection while it does is
// not a failed generation.
//
// The sixty-second cap is the generation's own budget; the context carries the caller's.
func (c *comfyUIImageClient) awaitComfyImage(ctx context.Context, baseURL, promptID string) ([]byte, error) {
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
			ref, ready, err := c.comfyImageOutput(ctx, baseURL, promptID)
			if err != nil {
				return nil, err
			}
			if !ready {
				continue
			}
			return c.fetchComfyImage(ctx, baseURL, ref)
		}
	}
}

// comfyImageOutput is the image a queued prompt has produced, if it has produced one yet.
// The history entry appears the moment the prompt is queued and carries no outputs until
// it finishes, so a prompt still running reports nothing rather than an error; only a
// request that cannot be built is an error.
func (c *comfyUIImageClient) comfyImageOutput(ctx context.Context, baseURL, promptID string) (comfyImageRef, bool, error) {
	histReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/history/%s", baseURL, promptID), nil)
	if err != nil {
		return comfyImageRef{}, false, err
	}
	histResp, err := c.client.Do(histReq)
	if err != nil {
		return comfyImageRef{}, false, nil
	}
	if histResp.StatusCode != http.StatusOK {
		histResp.Body.Close()
		return comfyImageRef{}, false, nil
	}

	var histData map[string]any
	err = json.NewDecoder(histResp.Body).Decode(&histData)
	histResp.Body.Close()
	if err != nil {
		return comfyImageRef{}, false, nil
	}

	pData, ok := histData[promptID].(map[string]any)
	if !ok {
		return comfyImageRef{}, false, nil
	}
	outputs, ok := pData["outputs"].(map[string]any)
	if !ok {
		return comfyImageRef{}, false, nil
	}

	for _, nodeOut := range outputs {
		nodeMap, ok := nodeOut.(map[string]any)
		if !ok {
			continue
		}
		images, ok := nodeMap["images"].([]any)
		if !ok || len(images) == 0 {
			continue
		}
		imgMap, ok := images[0].(map[string]any)
		if !ok {
			continue
		}

		var ref comfyImageRef
		ref.filename, _ = imgMap["filename"].(string)
		ref.subfolder, _ = imgMap["subfolder"].(string)
		ref.kind, _ = imgMap["type"].(string)
		if ref.filename != "" {
			return ref, true, nil
		}
	}
	return comfyImageRef{}, false, nil
}

// fetchComfyImage downloads the image the provider rendered, through the view endpoint
// that serves the output directory by name.
func (c *comfyUIImageClient) fetchComfyImage(ctx context.Context, baseURL string, ref comfyImageRef) ([]byte, error) {
	viewURL := fmt.Sprintf("%s/view?filename=%s&subfolder=%s&type=%s", baseURL, ref.filename, ref.subfolder, ref.kind)
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

type httpImageClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client

	usageMu   sync.Mutex
	lastUsage media.Usage
}

// LastUsage reports what the last image generation consumed: the provider's own
// counts when the body carried a usage block, otherwise one estimated request.
func (h *httpImageClient) LastUsage() media.Usage {
	h.usageMu.Lock()
	defer h.usageMu.Unlock()
	return h.lastUsage
}

func (h *httpImageClient) setLastUsage(u media.Usage) {
	h.usageMu.Lock()
	h.lastUsage = u
	h.usageMu.Unlock()
}

func (h *httpImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	h.setLastUsage(media.Usage{Requests: 1, Estimated: true})
	if isComfyUI(h.endpoint) {
		return h.comfyImage(ctx, prompt)
	}

	bodyBytes, err := h.postImageRequest(ctx, h.imagePayload(prompt))
	if err != nil {
		return nil, err
	}

	// 1. Raw Binary detection: PNG, JPEG, WebP
	if looksLikeImage(bodyBytes) {
		return bodyBytes, nil
	}

	// 2. Parse JSON response
	var respData map[string]any
	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return bodyBytes, nil
	}
	h.usageFromBody(respData)

	// AUTOMATIC1111 / Forge WebUI: {"images": ["base64..."]}
	if image, matched, err := automatic1111Image(respData); matched {
		return image, err
	}
	// OpenAI format: {"data": [{"b64_json": "..."} | {"url": "..."}]}
	if image, matched, err := h.openAIImage(ctx, respData); matched {
		return image, err
	}

	return bodyBytes, nil
}

// comfyImage runs the generation through the ComfyUI client, which speaks a different
// protocol: its endpoint is a base URL rather than a route, and the work is queued and
// polled. The usage it reports is folded back in, because the estimate was set before the
// endpoint was known.
func (h *httpImageClient) comfyImage(ctx context.Context, prompt string) ([]byte, error) {
	comfyClient := &comfyUIImageClient{
		endpoint: h.endpoint,
		client:   h.client,
	}
	image, err := comfyClient.GenerateImage(ctx, prompt)
	h.setLastUsage(comfyClient.LastUsage())
	return image, err
}

// imagePayload is the request body for whichever of the two shapes this endpoint speaks:
// AUTOMATIC1111's own field names, or the OpenAI ones with the model named when one is
// configured. The size and the step count are fixed here, because the prompt is a scene
// description rather than a set of generation parameters.
func (h *httpImageClient) imagePayload(prompt string) []byte {
	var payload []byte
	if strings.Contains(h.endpoint, "/sdapi/v1/txt2img") {
		payload, _ = json.Marshal(map[string]any{
			"prompt":          prompt,
			"negative_prompt": "blurry, low quality, deformed",
			"steps":           20,
			"width":           512,
			"height":          512,
		})
		return payload
	}

	payloadMap := map[string]any{
		"prompt":          prompt,
		"n":               1,
		"size":            "512x512",
		"response_format": "b64_json",
	}
	if h.model != "" {
		payloadMap["model"] = h.model
	}
	payload, _ = json.Marshal(payloadMap)
	return payload
}

// postImageRequest sends the generation request and returns the body, whatever shape the
// provider answers in: every endpoint here answers 200 with either the bytes themselves or
// a document describing where they are.
func (h *httpImageClient) postImageRequest(ctx context.Context, payload []byte) ([]byte, error) {
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

	return io.ReadAll(resp.Body)
}

// looksLikeImage reports whether a body is the image itself rather than a document about
// one, judged by its magic bytes.
func looksLikeImage(bodyBytes []byte) bool {
	return bytes.HasPrefix(bodyBytes, []byte("\x89PNG")) ||
		bytes.HasPrefix(bodyBytes, []byte("\xff\xd8\xff")) ||
		(bytes.HasPrefix(bodyBytes, []byte("RIFF")) && bytes.Contains(bodyBytes[:min(len(bodyBytes), 16)], []byte("WEBP")))
}

// automatic1111Image is the image in an AUTOMATIC1111 or Forge WebUI reply, which carries
// one base64 string in images[0]. It reports whether the reply was in that shape at all, so
// a reply that is not can be read as another.
func automatic1111Image(respData map[string]any) ([]byte, bool, error) {
	images, ok := respData["images"].([]any)
	if !ok || len(images) == 0 {
		return nil, false, nil
	}
	imgStr, ok := images[0].(string)
	if !ok {
		return nil, false, nil
	}

	if idx := strings.Index(imgStr, ","); idx != -1 && strings.HasPrefix(imgStr, "data:") {
		imgStr = imgStr[idx+1:]
	}
	decoded, err := base64.StdEncoding.DecodeString(imgStr)
	if err != nil {
		return nil, true, fmt.Errorf("decode a1111 base64: %w", err)
	}
	return decoded, true, nil
}

// openAIImage is the image in an OpenAI-shaped reply, which carries either the bytes inline
// as b64_json or a URL to fetch them from. It reports whether the reply was in that shape at
// all.
func (h *httpImageClient) openAIImage(ctx context.Context, respData map[string]any) ([]byte, bool, error) {
	data, ok := respData["data"].([]any)
	if !ok || len(data) == 0 {
		return nil, false, nil
	}
	item, ok := data[0].(map[string]any)
	if !ok {
		return nil, false, nil
	}

	if b64, ok := item["b64_json"].(string); ok && b64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, true, fmt.Errorf("decode openai b64_json: %w", err)
		}
		return decoded, true, nil
	}
	imgURL, ok := item["url"].(string)
	if !ok || imgURL == "" {
		return nil, false, nil
	}

	image, err := h.fetchImageURL(ctx, imgURL)
	if err != nil {
		return nil, true, err
	}
	return image, true, nil
}

// fetchImageURL downloads an image the provider pointed at rather than inlined.
func (h *httpImageClient) fetchImageURL(ctx context.Context, imgURL string) ([]byte, error) {
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

// Factory Constructors
