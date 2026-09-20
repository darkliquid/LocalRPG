package media_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestProceduralArt_GeneratesValidSVG(t *testing.T) {
	client, err := media.NewImageClient(config.ImageConfig{
		Type:        "builtin",
		BuiltinName: "procedural-art",
	})
	if err != nil {
		t.Fatalf("failed to create procedural art client: %v", err)
	}

	prompts := []string{
		"Grim dark fortress towering over a misty swamp",
		"Sunken catacombs beneath ancient ruins with glowing runes",
		"Mountain citadel under a blood moon sky",
	}

	for _, prompt := range prompts {
		bytes, err := client.GenerateImage(context.Background(), prompt)
		if err != nil {
			t.Fatalf("failed to generate art for %q: %v", prompt, err)
		}
		svg := string(bytes)
		if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
			maxLen := len(svg)
			if maxLen > 50 {
				maxLen = 50
			}
			t.Errorf("expected valid SVG format for %q, got: %s", prompt, svg[:maxLen])
		}
		if !strings.Contains(svg, "<defs>") || !strings.Contains(svg, "<linearGradient") {
			t.Errorf("expected atmospheric gradient defs in SVG for %q", prompt)
		}
	}
}
