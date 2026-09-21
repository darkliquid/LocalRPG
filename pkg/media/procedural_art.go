package media

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"
)

type proceduralArtClient struct{}

func NewProceduralArtClient() ImageClient {
	return &proceduralArtClient{}
}

func (p *proceduralArtClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	lower := strings.ToLower(prompt)

	// Determine palette based on prompt
	skyTop := "#0c0a09"
	skyBottom := "#292524"
	accentColor := "#d97706"
	isCrimson := strings.Contains(lower, "blood") || strings.Contains(lower, "fire") || strings.Contains(lower, "flame")
	isMire := strings.Contains(lower, "swamp") || strings.Contains(lower, "mire") || strings.Contains(lower, "toxic")
	isRuins := strings.Contains(lower, "ruin") || strings.Contains(lower, "catacomb") || strings.Contains(lower, "dungeon")

	if isCrimson {
		skyTop = "#1a0505"
		skyBottom = "#450a0a"
		accentColor = "#ef4444"
	} else if isMire {
		skyTop = "#05160e"
		skyBottom = "#064e3b"
		accentColor = "#10b981"
	} else if isRuins {
		skyTop = "#09090b"
		skyBottom = "#27272a"
		accentColor = "#a855f7"
	}

	// Deterministic variation keyed on the prompt's content, so identical prompts
	// produce identical art and prompts of equal length do not collide.
	hasher := fnv.New64a()
	hasher.Write([]byte(prompt))
	rng := rand.New(rand.NewSource(int64(hasher.Sum64())))

	// Generate stars/particles
	var particles strings.Builder
	for i := 0; i < 30; i++ {
		cx := rng.Intn(800)
		cy := rng.Intn(350)
		r := rng.Float64()*1.5 + 0.5
		opacity := rng.Float64()*0.7 + 0.3
		particles.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="%.2f" />`+"\n", cx, cy, r, accentColor, opacity))
	}

	// Foreground structure (citadel or peaks)
	structurePath := "M 0 450 Q 200 380 400 420 T 800 440 L 800 600 L 0 600 Z"
	if strings.Contains(lower, "tower") || strings.Contains(lower, "fortress") || strings.Contains(lower, "citadel") {
		structurePath = "M 0 520 L 150 500 L 180 320 L 220 320 L 240 500 L 350 480 L 380 260 L 420 260 L 450 480 L 800 520 L 800 600 L 0 600 Z"
	} else if strings.Contains(lower, "mountain") || strings.Contains(lower, "cliff") {
		structurePath = "M 0 520 L 120 380 L 240 450 L 400 310 L 560 460 L 680 370 L 800 520 L 800 600 L 0 600 Z"
	}

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600" width="800" height="600">
  <defs>
    <linearGradient id="skyGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%%" stop-color="%s" />
      <stop offset="100%%" stop-color="%s" />
    </linearGradient>
    <linearGradient id="groundGrad" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0%%" stop-color="#1c1917" />
      <stop offset="100%%" stop-color="#0c0a09" />
    </linearGradient>
    <radialGradient id="celestialGrad" cx="50%%" cy="50%%" r="50%%">
      <stop offset="0%%" stop-color="%s" stop-opacity="0.8" />
      <stop offset="100%%" stop-color="%s" stop-opacity="0" />
    </radialGradient>
  </defs>

  <!-- Sky -->
  <rect width="800" height="600" fill="url(#skyGrad)" />

  <!-- Celestial Glow & Body -->
  <circle cx="620" cy="180" r="140" fill="url(#celestialGrad)" />
  <circle cx="620" cy="180" r="45" fill="%s" opacity="0.85" />

  <!-- Ambient Particles -->
  %s
  <!-- Distant Ridge -->
  <path d="M 0 460 Q 250 390 500 440 T 800 450 L 800 600 L 0 600 Z" fill="#1c1917" opacity="0.6" />

  <!-- Main Silhouetted Structure -->
  <path d="%s" fill="url(#groundGrad)" />

  <!-- Fog / Mist Horizon Layer -->
  <rect x="0" y="470" width="800" height="40" fill="%s" opacity="0.15" />
</svg>`, skyTop, skyBottom, accentColor, accentColor, accentColor, particles.String(), structurePath, accentColor)

	return []byte(svg), nil
}
