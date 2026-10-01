package driver

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg" // registers the JPEG decoder used by image.Decode below
	"image/png"
	"testing"
)

// testPNG builds a small PNG, standing in for a Chrome screenshot capture.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 10))
	for x := 0; x < 16; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 25), B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestEncodeScreenshotPassesPNGThrough(t *testing.T) {
	data := testPNG(t)
	got, err := encodeScreenshot(data, "png", 0)
	if err != nil {
		t.Fatalf("encodeScreenshot: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("a PNG capture should be written unchanged")
	}
}

func TestEncodeScreenshotJPEG(t *testing.T) {
	data := testPNG(t)
	got, err := encodeScreenshot(data, "jpeg", 90)
	if err != nil {
		t.Fatalf("encodeScreenshot: %v", err)
	}
	if bytes.HasPrefix(got, []byte("\x89PNG")) {
		t.Fatal("expected JPEG bytes, got a PNG signature")
	}

	img, format, err := image.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("format = %q, want jpeg", format)
	}
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 10 {
		t.Errorf("decoded size = %dx%d, want 16x10", b.Dx(), b.Dy())
	}

	// An unset or out-of-range quality falls back to the default.
	for _, quality := range []int{0, -5, 500} {
		if _, err := encodeScreenshot(data, "jpg", quality); err != nil {
			t.Errorf("quality %d: %v", quality, err)
		}
	}
}

func TestEncodeScreenshotRejectsUnknownFormat(t *testing.T) {
	if _, err := encodeScreenshot(testPNG(t), "webp", 0); err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
}

func TestScreenshotFormat(t *testing.T) {
	cases := []struct {
		name string
		step Step
		want string
	}{
		{"explicit format wins", Step{Path: "shot.png", Format: "JPEG"}, "jpeg"},
		{"jpg extension", Step{Path: "shot.jpg"}, "jpeg"},
		{"jpeg extension", Step{Path: "shot.jpeg"}, "jpeg"},
		{"png extension", Step{Path: "shot.png"}, "png"},
		{"no extension", Step{Path: "shot"}, "png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := screenshotFormat(tc.step); got != tc.want {
				t.Errorf("screenshotFormat(%+v) = %q, want %q", tc.step, got, tc.want)
			}
		})
	}
}
