package export

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	_ "image/gif"
	_ "golang.org/x/image/webp"
)

// The embedded display size. An illustration larger than this is downscaled to
// it, because the player never shows more and a bundle carries every image.
const (
	maxEmbedWidth    = 1280
	maxEmbedHeight   = 720
	embedJPEGQuality = 85
)

// imageDataURI embeds an image, downscaling it to the export's display size when
// it is larger and re-encoding it so the bundle stays small. A vector is embedded
// as-is, and a file that cannot be decoded falls back to its raw bytes. The
// campaign's own file is never touched.
func imageDataURI(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no asset")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if isSVG(path, data) {
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data), nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "data:" + mimeTypeFor(path, data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	bounds := img.Bounds()
	if bounds.Dx() <= maxEmbedWidth && bounds.Dy() <= maxEmbedHeight {
		return "data:" + mimeTypeFor(path, data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}

	encoded, mime, err := encodeImage(scaleToFit(img, maxEmbedWidth, maxEmbedHeight))
	if err != nil {
		return "data:" + mimeTypeFor(path, data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(encoded), nil
}

// isSVG reports whether an asset is a vector, which is embedded verbatim.
func isSVG(path string, data []byte) bool {
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return true
	}
	return bytes.Contains(data[:min(len(data), 512)], []byte("<svg"))
}

// scaleToFit scales an image down so it fits the display size, preserving its
// aspect ratio. It never scales up.
func scaleToFit(img image.Image, maxWidth, maxHeight int) *image.RGBA {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	scale := math.Min(float64(maxWidth)/float64(max(1, width)), float64(maxHeight)/float64(max(1, height)))
	if scale > 1 {
		scale = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)
	return dst
}

// encodeImage re-encodes a scaled image, as JPEG when it is fully opaque and PNG
// when it carries transparency, so a photo does not stay a large lossless file.
func encodeImage(img *image.RGBA) ([]byte, string, error) {
	var buffer bytes.Buffer
	if isOpaque(img) {
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: embedJPEGQuality}); err != nil {
			return nil, "", err
		}
		return buffer.Bytes(), "image/jpeg", nil
	}
	if err := png.Encode(&buffer, img); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "image/png", nil
}

// isOpaque reports whether every pixel is fully opaque.
func isOpaque(img *image.RGBA) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.RGBAAt(x, y).A < 255 {
				return false
			}
		}
	}
	return true
}
