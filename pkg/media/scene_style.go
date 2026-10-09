package media

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// SceneStyle is the stable look of one scene: a seed, a palette, and a lighting
// phrase derived from the scene and the world style. Two turns in one scene share
// it, so successive images of a place keep its look while the moment varies.
type SceneStyle struct {
	SceneID   string
	Seed      int64
	Palette   string
	Lighting  string
	Described string
}

// scenePalettes and sceneLighting are the small deterministic tables a scene's
// look is drawn from. The palette names colours rather than hues, so a provider
// renders a described look rather than a numeric one.
var (
	scenePalettes = []string{
		"muted ochre and cold blue",
		"weathered stone and sea grey",
		"deep green and lamplight amber",
		"ash grey and dull crimson",
		"sun-bleached bone and dust",
		"ink blue and pale gold",
		"bruised purple and rust",
		"frost white and slate",
	}
	sceneLighting = []string{
		"overcast", "lamplight", "dawn", "dusk", "harsh noon", "moonlit", "fog-diffused", "firelit",
	}
)

// NewSceneStyle derives a scene's stable style from the scene's identity and the
// world's art style. It is a pure function of its inputs, so the same scene always
// yields the same look and the art cache key stays correct.
func NewSceneStyle(sceneID, worldStyle, appearance string) SceneStyle {
	id := strings.TrimSpace(sceneID)
	if id == "" {
		// An unlocated turn has no place, so it has no derived look: only the
		// authored appearance, when there is one.
		return SceneStyle{Described: strings.TrimSpace(appearance)}
	}
	seed := sceneSeed(id, worldStyle)
	return SceneStyle{
		SceneID:   id,
		Seed:      seed,
		Palette:   scenePalettes[int(uint64(seed)%uint64(len(scenePalettes)))],
		Lighting:  sceneLighting[int(uint64(seed/7)%uint64(len(sceneLighting)))],
		Described: strings.TrimSpace(appearance),
	}
}

// sceneSeed hashes a scene's identity into a seed, so a different scene or world
// style yields a different look.
func sceneSeed(sceneID, worldStyle string) int64 {
	h := fnv.New64a()
	h.Write([]byte(strings.TrimSpace(sceneID) + "|" + strings.TrimSpace(worldStyle)))
	return int64(h.Sum64())
}

// Clause is the prompt fragment that names the look: the palette and the lighting,
// and the authored description when there is one. It is identical for every turn
// in one scene, which is the consistency a provider without image conditioning
// gets.
func (s SceneStyle) Clause() string {
	parts := make([]string, 0, 3)
	if s.Palette != "" {
		parts = append(parts, "palette: "+s.Palette)
	}
	if s.Lighting != "" {
		parts = append(parts, "lighting: "+s.Lighting)
	}
	if s.Described != "" {
		parts = append(parts, s.Described)
	}
	return strings.Join(parts, ", ")
}

// Empty reports whether the style carries nothing to say, so a caller can leave a
// prompt untouched.
func (s SceneStyle) Empty() bool {
	return s.Clause() == ""
}

// SceneRequestFromPrompt derives a structured request from prose, so a caller that
// only has a prompt can still reach a provider that understands hints.
func SceneRequestFromPrompt(prompt string) SceneRequest {
	return sceneRequestFromPrompt(prompt)
}

// String names a scene style for a log or a trace.
func (s SceneStyle) String() string {
	return fmt.Sprintf("scene %s seed %d (%s)", s.SceneID, s.Seed, s.Clause())
}
