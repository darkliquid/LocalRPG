package scene

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"

	// Decoders register themselves, so raster art of these kinds can be read.
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// artCache decodes art once per export. Art is loaded many times (a scene's
// background for every frame of every beat), so the cache is what keeps an export
// from re-rasterizing the same file thousands of times.
type artCache struct {
	mu     sync.Mutex
	images map[string]image.Image
}

func newArtCache() *artCache {
	return &artCache{images: map[string]image.Image{}}
}

// load decodes a raster image or rasterizes an SVG. SVG is the default case, not
// an edge one: the built-in image provider emits SVG, and a portrait-less
// character always gets the procedural bust SVG.
func (c *artCache) load(path string) (image.Image, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("load art: no path")
	}

	c.mu.Lock()
	if img, ok := c.images[path]; ok {
		c.mu.Unlock()
		return img, nil
	}
	c.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load art %q: %w", path, err)
	}

	img, err := decodeArt(data)
	if err != nil {
		return nil, fmt.Errorf("decode art %q: %w", path, err)
	}

	c.mu.Lock()
	c.images[path] = img
	c.mu.Unlock()
	return img, nil
}

func decodeArt(data []byte) (image.Image, error) {
	if isSVG(data) {
		return rasterizeSVG(data)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func isSVG(data []byte) bool {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	lower := bytes.ToLower(head)
	return bytes.Contains(lower, []byte("<svg")) || bytes.Contains(lower, []byte("<?xml"))
}

func rasterizeSVG(data []byte) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse SVG: %w", err)
	}

	width, height := int(icon.ViewBox.W), int(icon.ViewBox.H)
	if width < 1 || height < 1 {
		width, height = 256, 256
	}
	icon.SetTarget(0, 0, float64(width), float64(height))

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	scanner := rasterx.NewScannerGV(width, height, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(width, height, scanner), 1.0)
	return img, nil
}
