package media

import (
	"strings"
	"testing"
)

func TestGenerateProceduralBustSVG(t *testing.T) {
	svgBytes := GenerateProceduralBustSVG("stretch-layabout", "Stretch Layabout", "non-binary")
	if len(svgBytes) == 0 {
		t.Fatal("expected non-empty SVG bytes")
	}

	svgStr := string(svgBytes)
	if !strings.HasPrefix(svgStr, "<svg") || !strings.HasSuffix(strings.TrimSpace(svgStr), "</svg>") {
		t.Errorf("expected valid SVG root tag, got %s", svgStr)
	}
	if !strings.Contains(svgStr, "viewBox=\"0 0 256 256\"") {
		t.Errorf("expected 256x256 viewBox, got %s", svgStr)
	}

	// Determinism test
	repeatBytes := GenerateProceduralBustSVG("stretch-layabout", "Stretch Layabout", "non-binary")
	if string(svgBytes) != string(repeatBytes) {
		t.Errorf("expected deterministic SVG generation for the same character ID")
	}
}
