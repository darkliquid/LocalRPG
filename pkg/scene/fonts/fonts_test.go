package fonts

import (
	"testing"

	"golang.org/x/image/font/opentype"
)

// A checkout that has not run `go generate` must still load: the Go fonts are the
// fallback, so a build never depends on the download.
func TestLoadAlwaysReturnsFaces(t *testing.T) {
	set, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, face := range []struct {
		name string
		font *opentype.Font
	}{
		{"Serif", set.Serif},
		{"SerifItalic", set.SerifItalic},
		{"Sans", set.Sans},
		{"SansItalic", set.SansItalic},
		{"Mono", set.Mono},
	} {
		if face.font == nil {
			t.Errorf("%s is nil", face.name)
			continue
		}
		if face.font.UnitsPerEm() == 0 {
			t.Errorf("%s has no units per em", face.name)
		}
	}
}
