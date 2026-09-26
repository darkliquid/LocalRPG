package desktop

import (
	"fmt"
	"image"
	"sync"

	. "go.hasen.dev/shirei"

	xdraw "golang.org/x/image/draw"
)

var coverCache sync.Map // "path|w|h" -> *image.RGBA

// coverID registers path cropped and scaled to fill w×h pixels, and returns
// its image handle (0 when the file cannot be loaded). Shirei's Image only
// scales down and preserves aspect ratio; this does a centre-crop "cover".
func coverID(key, path string, w, h int) ImageId {
	if path == "" || w <= 0 || h <= 0 {
		return 0
	}
	ck := fmt.Sprintf("%s|%d|%d", path, w, h)
	if cached, ok := coverCache.Load(ck); ok {
		return UseImage(ck, cached.(*image.RGBA))
	}

	data := LoadImage(path)
	if data == nil || data.Rect.Dx() == 0 || data.Rect.Dy() == 0 {
		return 0
	}
	src := &data.RGBA
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	targetAR := float64(w) / float64(h)

	var crop image.Rectangle
	if sw/sh > targetAR {
		nw := int(sh * targetAR)
		x0 := b.Min.X + (b.Dx()-nw)/2
		crop = image.Rect(x0, b.Min.Y, x0+nw, b.Max.Y)
	} else {
		nh := int(sw / targetAR)
		y0 := b.Min.Y + (b.Dy()-nh)/2
		crop = image.Rect(b.Min.X, y0, b.Max.X, y0+nh)
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, xdraw.Src, nil)
	coverCache.Store(ck, dst)
	return UseImage(ck, dst)
}

// coverImage draws path filling the given box (rounded by the caller's Clip).
func coverImage(key, path string, w, h float32) {
	id := coverID(key, path, int(w), int(h))
	if id == 0 {
		return
	}
	ImageViewAt(id, Vec2{w, h})
}
