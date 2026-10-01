package webm

import (
	"fmt"
	"image"

	"github.com/gen2brain/vpx/vp8"
)

// encodeMethod is the encoder's quality/speed trade-off, 0-6. Method 2 keeps the
// encoder near its fastest: the higher methods add subblock and rate-distortion
// search that cost about six times as much per frame for a marginal size gain,
// and Quality is what actually governs how a frame looks.
const encodeMethod = 2

// Encoder turns RGBA frames into a VP8 bitstream. It keeps its reference frames
// across calls, so a keyframe must precede any inter frame.
type Encoder struct {
	quality int
	method  int
	enc     vp8.Encoder
	picture *vp8.Picture
}

// NewEncoder builds an encoder for a fixed frame size.
func NewEncoder(width, height, quality int) *Encoder {
	return &Encoder{
		quality: quality,
		method:  encodeMethod,
		picture: newPicture(width, height),
	}
}

// Encode writes one frame. A keyframe stands alone and refreshes every reference;
// an inter frame predicts from the frame before it and is far smaller when the
// picture barely changes.
func (e *Encoder) Encode(img *image.RGBA, keyframe bool) ([]byte, error) {
	toYUV420(img, e.picture)
	opts := vp8.EncodeOptions{Quality: e.quality, Method: e.method}
	if keyframe {
		return e.enc.Encode(e.picture, opts)
	}
	data, err := e.enc.EncodeInter(e.picture, opts)
	if err != nil {
		return nil, fmt.Errorf("webm: encode inter frame: %w", err)
	}
	return data, nil
}

// newPicture allocates 4:2:0 planes at their own strides.
func newPicture(width, height int) *vp8.Picture {
	uvW, uvH := (width+1)/2, (height+1)/2
	return &vp8.Picture{
		Y:        make([]byte, width*height),
		U:        make([]byte, uvW*uvH),
		V:        make([]byte, uvW*uvH),
		YStride:  width,
		UVStride: uvW,
		Width:    width,
		Height:   height,
	}
}

// toYUV420 converts an RGBA frame to BT.601 4:2:0, the colour space WebP and WebM
// VP8 both use. Chroma is averaged over each 2×2 block.
func toYUV420(img *image.RGBA, dst *vp8.Picture) {
	bounds := img.Bounds()
	width, height := dst.Width, dst.Height
	uvW := dst.UVStride

	for y := 0; y < height; y++ {
		sy := min(bounds.Min.Y+y, bounds.Max.Y-1)
		for x := 0; x < width; x++ {
			sx := min(bounds.Min.X+x, bounds.Max.X-1)
			c := img.RGBAAt(sx, sy)
			r, g, b := float64(c.R), float64(c.G), float64(c.B)
			dst.Y[y*dst.YStride+x] = clampYUV(0.257*r + 0.504*g + 0.098*b + 16)
		}
	}

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			var sumR, sumG, sumB float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					sx := min(bounds.Min.X+x+dx, bounds.Max.X-1)
					sy := min(bounds.Min.Y+y+dy, bounds.Max.Y-1)
					c := img.RGBAAt(sx, sy)
					sumR += float64(c.R)
					sumG += float64(c.G)
					sumB += float64(c.B)
				}
			}
			r, g, b := sumR/4, sumG/4, sumB/4
			index := (y/2)*uvW + x/2
			dst.U[index] = clampYUV(-0.148*r - 0.291*g + 0.439*b + 128)
			dst.V[index] = clampYUV(0.439*r - 0.368*g - 0.071*b + 128)
		}
	}
}

func clampYUV(v float64) byte {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return byte(v + 0.5)
	}
}
