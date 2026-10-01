package webm

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/gen2brain/vpx/vp8"
)

func solidFrame(width, height int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestEncoderWritesADecodableKeyframe(t *testing.T) {
	encoder := NewEncoder(64, 48, 80)
	data, err := encoder.Encode(solidFrame(64, 48, color.RGBA{200, 40, 40, 255}), true)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("encoder produced no bytes")
	}

	var decoder vp8.Decoder
	pic, err := decoder.DecodeFrame(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pic == nil || pic.Width != 64 || pic.Height != 48 {
		t.Fatalf("decoded frame = %+v", pic)
	}
}

func TestEncoderWritesAnInterFrame(t *testing.T) {
	encoder := NewEncoder(64, 48, 80)
	key, err := encoder.Encode(solidFrame(64, 48, color.RGBA{10, 10, 10, 255}), true)
	if err != nil {
		t.Fatalf("keyframe: %v", err)
	}

	// An inter frame predicts from the frame before it, so the decoder must have
	// seen the keyframe first.
	var decoder vp8.Decoder
	if _, err := decoder.DecodeFrame(key); err != nil {
		t.Fatalf("decode keyframe: %v", err)
	}

	inter, err := encoder.Encode(solidFrame(64, 48, color.RGBA{12, 12, 12, 255}), false)
	if err != nil {
		t.Fatalf("inter frame: %v", err)
	}
	pic, err := decoder.DecodeFrame(inter)
	if err != nil {
		t.Fatalf("decode inter: %v", err)
	}
	if pic == nil {
		t.Fatal("inter frame decoded to nothing")
	}
}

// TestEncoderOutputSurvivesTheNextFrame pins the contract that a corrupted export
// came from breaking: the encoder reuses its output buffer across calls, and a
// muxer holds a block for a while before writing it, so a frame that has already
// been handed over must not change.
func TestEncoderOutputSurvivesTheNextFrame(t *testing.T) {
	encoder := NewEncoder(64, 48, 80)
	first, err := encoder.Encode(solidFrame(64, 48, color.RGBA{10, 10, 10, 255}), true)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	snapshot := append([]byte(nil), first...)

	if _, err := encoder.Encode(solidFrame(64, 48, color.RGBA{240, 240, 240, 255}), true); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if !bytes.Equal(first, snapshot) {
		t.Fatal("the encoder overwrote a frame it had already returned")
	}
}
