package media

import "context"

// SceneRequest carries structured hints for a generated scene image. Every hint
// is optional: an absent one falls back to a deterministic derivation.
type SceneRequest struct {
	Prompt    string
	Genre     string
	Mood      string
	TimeOfDay string
	Weather   string
	Seed      string
}

// SceneHintProvider is implemented by image providers that can use structured
// hints rather than only the prose prompt. The pipeline asserts it and falls back
// to GenerateImage when it is absent.
type SceneHintProvider interface {
	GenerateScene(ctx context.Context, req SceneRequest) ([]byte, error)
}
