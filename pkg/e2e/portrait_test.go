//go:build e2e

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// TestMirroredPortraitRendersInsideItsRoundedBox renders the theatre's own portrait
// markup with the player build's stylesheet and checks that a mirrored portrait both
// paints and stays inside its rounded border. The image carries the rounding itself,
// because a transformed child escapes a parent's rounded clip in some engines.
func TestMirroredPortraitRendersInsideItsRoundedBox(t *testing.T) {
	assets, err := gui.AssetFS()
	if err != nil {
		t.Skipf("no player build: %v", err)
	}
	entries, err := fs.Glob(assets, "player/assets/*.css")
	if err != nil || len(entries) == 0 {
		t.Skipf("no player stylesheet: %v", err)
	}
	css, err := fs.ReadFile(assets, entries[0])
	if err != nil {
		t.Fatal(err)
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" width="100" height="100"><rect width="100" height="100" fill="#ff0000"/></svg>`)
	solid := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)

	img := func(extra string) string {
		return `<img style="width:100%;height:100%;object-fit:cover;object-position:top" ` + extra + ` src="` + solid + `">`
	}
	box := func(inner string) string {
		return `<div class="probe-box overflow-hidden rounded-2xl border-2 bg-stone-900" style="width:150px;height:150px;flex:none">` + inner + `</div>`
	}
	variants := []string{
		box(img("")),
		box(img(`class="scale-x-[-1]"`)),
		box(img(`style="width:100%;height:100%;object-fit:cover;object-position:top;transform:scaleX(-1)"`)),
		box(`<div style="width:100%;height:100%" class="scale-x-[-1]">` + img("") + `</div>`),
		box(img(`class="scale-x-[-1] rounded-2xl"`)),
	}

	page := `<!DOCTYPE html><html><head><meta charset="utf-8"><style>` + string(css) +
		`</style></head><body style="margin:0;background:#000;display:flex;gap:20px;padding:20px">` +
		strings.Join(variants, "") + `</body></html>`

	file := filepath.Join(t.TempDir(), "probe.html")
	if err := os.WriteFile(file, []byte(page), 0644); err != nil {
		t.Fatal(err)
	}

	b := NewBrowser(t, "")
	b.Navigate((&url.URL{Scheme: "file", Path: file}).String())
	b.Poll(`Array.from(document.images).every(i => i.complete && i.naturalWidth > 0)`, "the probe images load")

	var raw string
	const rects = `JSON.stringify(Array.from(document.querySelectorAll('.probe-box')).map(el => { var r = el.getBoundingClientRect(); return {X:r.x,Y:r.y,W:r.width,H:r.height}; }))`
	raw = b.Eval(rects)
	var boxes []struct{ X, Y, W, H float64 }
	if err := json.Unmarshal([]byte(raw), &boxes); err != nil {
		t.Fatalf("boxes %q: %v", raw, err)
	}

	shot := b.FullScreenshot(100)
	shotPath := filepath.Join(t.TempDir(), "probe.png")
	if err := os.WriteFile(shotPath, shot, 0644); err != nil {
		t.Fatal(err)
	}
	handle, err := os.Open(shotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	rendered, _, err := image.Decode(handle)
	if err != nil {
		t.Fatal(err)
	}

	scale := float64(rendered.Bounds().Dx()) / 1084.0
	at := func(box struct{ X, Y, W, H float64 }, fx, fy float64) string {
		px, py := int((box.X+box.W*fx)*scale), int((box.Y+box.H*fy)*scale)
		if px < 0 || py < 0 || px >= rendered.Bounds().Dx() || py >= rendered.Bounds().Dy() {
			return "out-of-frame"
		}
		r, g, b, _ := rendered.At(px, py).RGBA()
		return fmt.Sprintf("%02x%02x%02x", r>>8, g>>8, b>>8)
	}

	// The theatre's markup: mirrored, and rounded on the image itself.
	type sample struct{ centre, corner string }
	got := map[string]sample{}
	names := []string{"plain", "theatre mirrored", "transform mirror", "flipped wrapper", "rounded mirrored"}
	for i, box := range boxes {
		if i >= len(names) {
			continue
		}
		got[names[i]] = sample{centre: at(box, 0.5, 0.5), corner: at(box, 0.02, 0.02)}
	}

	for _, name := range []string{"plain", "theatre mirrored", "rounded mirrored"} {
		if got[name].centre != "ff0000" {
			t.Errorf("%s: centre = %s, want the image to paint", name, got[name].centre)
		}
		if got[name].corner == "ff0000" {
			t.Errorf("%s: corner = %s, want the image clipped to its rounded border", name, got[name].corner)
		}
	}
}
