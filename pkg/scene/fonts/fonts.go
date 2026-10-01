// Package fonts embeds the typefaces the story theatre uses, so the video
// compositor renders the same typography the exported page does. The families are
// OFL; see gen/main.go for their provenance and licences.
package fonts

//go:generate go run ./gen fonts

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

//go:embed all:fonts
var embedded embed.FS

// The files `go generate` writes into fonts/. The families ship as variable
// fonts, and x/image cannot instance them, so a renderer uses the default
// instance and synthesises bold by drawing a glyph twice.
const (
	serifFile       = "fonts/EBGaramond.ttf"
	serifItalicFile = "fonts/EBGaramond-Italic.ttf"
	sansFile        = "fonts/Inter.ttf"
	sansItalicFile  = "fonts/Inter-Italic.ttf"
	monoFile        = "fonts/JetBrainsMono.ttf"
)

// Set is the parsed families a renderer draws with.
type Set struct {
	Serif       *opentype.Font
	SerifItalic *opentype.Font
	Sans        *opentype.Font
	SansItalic  *opentype.Font
	Mono        *opentype.Font

	// Bundled is true when the embedded OFL files were used rather than the
	// fallback Go fonts.
	Bundled bool
}

// Load parses the embedded fonts. A checkout that has not run `go generate`
// falls back to the Go fonts, so a build never depends on the download.
func Load() (*Set, error) {
	set, err := loadEmbedded()
	if err == nil {
		return set, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return loadFallback()
}

func loadEmbedded() (*Set, error) {
	serif, err := parseFile(serifFile)
	if err != nil {
		return nil, err
	}
	serifItalic, err := parseFile(serifItalicFile)
	if err != nil {
		return nil, err
	}
	sans, err := parseFile(sansFile)
	if err != nil {
		return nil, err
	}
	sansItalic, err := parseFile(sansItalicFile)
	if err != nil {
		return nil, err
	}
	mono, err := parseFile(monoFile)
	if err != nil {
		return nil, err
	}
	return &Set{
		Serif:       serif,
		SerifItalic: serifItalic,
		Sans:        sans,
		SansItalic:  sansItalic,
		Mono:        mono,
		Bundled:     true,
	}, nil
}

func parseFile(name string) (*opentype.Font, error) {
	data, err := fs.ReadFile(embedded, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func loadFallback() (*Set, error) {
	serif, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse fallback serif: %w", err)
	}
	italic, err := opentype.Parse(goitalic.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse fallback italic: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse fallback bold: %w", err)
	}
	mono, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse fallback mono: %w", err)
	}
	return &Set{
		Serif:       serif,
		SerifItalic: italic,
		Sans:        bold,
		SansItalic:  italic,
		Mono:        mono,
		Bundled:     false,
	}, nil
}
