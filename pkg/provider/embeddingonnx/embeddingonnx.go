// Package embeddingonnx registers the local ONNX text embedding provider.
package embeddingonnx

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// buildPayload is the configuration the descriptor builds from. ModelDir names
// the directory holding the encoder; when empty the app's model cache is used.
type buildPayload struct {
	ModelDir   string `json:"model_dir"`
	Dimensions int    `json:"dimensions"`
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyEmbeddingONNX),
			Family:      provider.FamilyEmbedding,
			Label:       "BGE Small Encoder (Built-in)",
			Description: "Local CPU text embeddings from a small BGE encoder, downloaded on demand.",
			Source:      "builtin",
			Tier:        provider.TierOfflineNeural,
			Features:    []provider.Feature{provider.FeatureOffline},
			Presets: []provider.Preset{
				{ID: "onnx", Order: 1, Label: "Built-in BGE Encoder",
					Description: "Semantic embeddings from a small BGE encoder, run on the CPU with no key and no server (downloads the model on demand).",
					Config: map[string]interface{}{
						"type": "onnx",
					}},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload buildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			dir := payload.ModelDir
			if dir == "" {
				dir = embeddings.ModelDir()
			}
			p, err := embeddings.NewONNXProvider(dir)
			if err != nil {
				// The encoder or its runtime is unavailable; recall still needs
				// an answer, so fall back to the hash projection.
				dims := payload.Dimensions
				if dims <= 0 {
					dims = 384
				}
				return embeddings.NewBuiltinHashProjectionProvider(dims), nil
			}
			return p, nil
		},
	})
}
