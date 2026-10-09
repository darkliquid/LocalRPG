package media

import (
	"fmt"
	"strings"
)

// Layer is one depth of a scene, drawn back to front. SVG is a self-contained
// document sized to the scene, transparent wherever the layer below should show.
type Layer struct {
	Depth float64
	SVG   []byte
}

// LayeredScene is a scene as one or more layers, ordered back to front. A scene
// with a single layer at depth 0 is exactly a flat image, so every consumer that
// takes one image still works.
type LayeredScene struct {
	Width  int
	Height int
	Layers []Layer
}

// Flatten composes the layers into one SVG document in the declared order, which
// is the single image the flat path serves.
func (s LayeredScene) Flatten() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`,
		s.Width, s.Height, s.Width, s.Height)
	for _, layer := range s.Layers {
		b.WriteString(layerContent(layer.SVG))
	}
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// layerContent returns a layer document's inner content, so a flattened scene can
// inline it without nesting a viewport per layer.
func layerContent(doc []byte) string {
	text := string(doc)
	if start := strings.Index(text, ">"); start >= 0 {
		text = text[start+1:]
	}
	return strings.TrimSuffix(text, "</svg>")
}

// sceneLayerDoc wraps a layer's content in a self-contained SVG document.
func sceneLayerDoc(width, height int, content string) []byte {
	return []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">%s</svg>`,
		width, height, width, height, content))
}
