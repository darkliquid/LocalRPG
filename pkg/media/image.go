package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type ImageClient interface {
	GenerateImage(ctx context.Context, prompt string) ([]byte, error)
}

type ImagePipeline struct {
	client ImageClient
	cache  *ContentCache
}

func NewImagePipeline(client ImageClient, cache *ContentCache) *ImagePipeline {
	return &ImagePipeline{
		client: client,
		cache:  cache,
	}
}

// GenerateLocationImage returns a cached scene image path, generating it when the
// appearance has changed or force is set. The key covers authored intent only; the
// prompt may include prose because it runs only on a miss.
func (p *ImagePipeline) GenerateLocationImage(ctx context.Context, location *entity.Entity, worldStyle, providerParams string, force bool) (string, error) {
	base := ComputeArtCacheKey(location.ID, AppearanceHash(location, providerParams), worldStyle)

	if !force {
		for _, ext := range []string{".svg", ".webp", ".jpg", ".jpeg", ".png"} {
			if p.cache.Exists("images", base+ext) {
				return filepath.Join(p.cache.Subdir("images"), base+ext), nil
			}
		}
	}

	prompt := BuildLocationPrompt(location, worldStyle)
	imgBytes, err := p.generateScene(ctx, location, worldStyle, prompt, providerParams)
	if err != nil {
		return "", fmt.Errorf("generate image for %q: %w", location.ID, err)
	}

	return p.cache.Put("images", base+artExtension(imgBytes), imgBytes)
}

// generateScene prefers a provider that understands structured hints, and falls
// back to the prose prompt for every other provider.
func (p *ImagePipeline) generateScene(ctx context.Context, location *entity.Entity, worldStyle, prompt, providerParams string) ([]byte, error) {
	if hp, ok := p.client.(SceneHintProvider); ok {
		return hp.GenerateScene(ctx, sceneRequest(location, worldStyle, AppearanceHash(location, providerParams)))
	}
	return p.client.GenerateImage(ctx, prompt)
}

// sceneRequest derives structured hints for a location's scene. Every hint is
// best-effort: an absent one falls back to a deterministic derivation in the
// generator rather than failing the image.
func sceneRequest(location *entity.Entity, worldStyle, appearanceHash string) SceneRequest {
	return SceneRequest{
		Prompt:    BuildLocationPrompt(location, worldStyle),
		Genre:     genreFor(location, worldStyle),
		Mood:      moodFor(location),
		TimeOfDay: stateString(location, "time_of_day"),
		Weather:   stateString(location, "weather"),
		Seed:      location.ID + "|" + appearanceHash,
	}
}

// genreFor reads a genre from the world style or the location tags, matching a
// known palette so an unknown genre falls back to the generator's default.
func genreFor(location *entity.Entity, worldStyle string) string {
	text := strings.ToLower(worldStyle)
	if location != nil {
		text += " " + strings.ToLower(strings.Join(location.Tags, " "))
	}
	for _, genre := range paletteGenres {
		if strings.Contains(text, genre) {
			return genre
		}
	}
	return ""
}

// moodFor reads a mood from the location state, then its tags.
func moodFor(location *entity.Entity) string {
	if mood := stateString(location, "mood"); mood != "" {
		return mood
	}
	if location == nil {
		return ""
	}
	text := strings.ToLower(strings.Join(location.Tags, " "))
	switch {
	case containsAny(text, "grim", "dark", "ominous", "dread", "dire"):
		return "grim"
	case containsAny(text, "serene", "calm", "peaceful", "bright", "warm"):
		return "serene"
	}
	return ""
}

// stateString reads a string value from an entity's state, or "" when absent.
func stateString(ent *entity.Entity, key string) string {
	if ent == nil || ent.State == nil {
		return ""
	}
	raw, ok := ent.State.Get(key)
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

// AppearanceHash is the variation input for a scene's art: the authored appearance
// when set, otherwise sorted tags and canonical state, plus the provider
// parameters so one model's art is never served for another. The note body is
// deliberately excluded: extraction appends to it almost every turn, so hashing it
// would regenerate art for no visual reason.
func AppearanceHash(ent *entity.Entity, providerParams string) string {
	parts := make([]string, 0, 3)

	if appearance := strings.TrimSpace(ent.Appearance); appearance != "" {
		parts = append(parts, appearance)
	} else {
		tags := append([]string(nil), ent.Tags...)
		sort.Strings(tags)

		state := ""
		if ent.State != nil {
			if data, err := json.Marshal(ent.State.Raw()); err == nil {
				state = string(data)
			}
		}
		parts = append(parts, strings.Join(tags, ","), state)
	}

	parts = append(parts, providerParams)
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}

// BuildLocationPrompt composes the generation prompt: the authored appearance when
// present, otherwise the name, a bounded body excerpt, and the tags, plus the
// world's art style.
func BuildLocationPrompt(ent *entity.Entity, worldStyle string) string {
	parts := make([]string, 0, 4)

	if appearance := strings.TrimSpace(ent.Appearance); appearance != "" {
		parts = append(parts, appearance)
	} else {
		parts = append(parts, ent.Name)

		body := strings.Join(strings.Fields(ent.Body), " ")
		if len(body) > 200 {
			body = body[:200] + "…"
		}
		if body != "" {
			parts = append(parts, body)
		}
		if len(ent.Tags) > 0 {
			parts = append(parts, strings.Join(ent.Tags, ", "))
		}
	}

	if style := strings.TrimSpace(worldStyle); style != "" {
		parts = append(parts, style)
	}
	return strings.Join(parts, ", ")
}

// ArtExtension picks the file's extension from the bytes, because the
// built-in generator returns SVG while providers return raster data.
func ArtExtension(data []byte) string {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(bytes.ToLower(head), []byte("<svg")) {
		return ".svg"
	}
	if bytes.HasPrefix(head, []byte("\x89PNG")) {
		return ".png"
	}
	if bytes.HasPrefix(head, []byte("\xff\xd8\xff")) {
		return ".jpg"
	}
	return ".webp"
}

func artExtension(data []byte) string {
	return ArtExtension(data)
}
