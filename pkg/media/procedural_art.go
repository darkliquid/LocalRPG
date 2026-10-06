package media

import "context"

// proceduralArtClient is the built-in, zero-GPU image generator. It draws an SVG
// scene from structured hints, so a scene's look is deterministic and the art
// cache key stays valid.
type proceduralArtClient struct{}

func NewProceduralArtClient() ImageClient {
	return &proceduralArtClient{}
}

// GenerateImage derives hints from the prose prompt and composes a scene, so
// callers that only have a prompt still get the richer generator.
func (p *proceduralArtClient) GenerateImage(_ context.Context, prompt string) ([]byte, error) {
	return GenerateSceneSVG(sceneRequestFromPrompt(prompt)), nil
}
