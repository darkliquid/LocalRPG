# Pure-Go Video Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ffmpeg-based video exporter with a pure-Go one that composites the story theatre's own look frame by frame and muxes VP8 video with the campaign's Opus audio into one seekable `.webm`, with no external binary on any platform.

**Architecture:** `pkg/scene` gains a theatre compositor (`Renderer.Frame` draws background, portraits, name plate, dialogue panel, prose) plus an art loader (raster via `x/image`, SVG via `oksvg`) and a font set. `pkg/media/webm` (new) encodes frames with `gen2brain/vpx` (keyframes and inter frames), builds one continuous Opus track from the campaign's clips, and muxes both with `at-wat/ebml-go`. `pkg/export/video.go` orchestrates and writes atomically. ffmpeg and ffprobe leave the export path.

**Tech Stack:** Go 1.27 (stdlib `testing` only), `gen2brain/vpx`, `at-wat/ebml-go`, `srwiley/oksvg`, `srwiley/rasterx`, `pion/opus` (already present), `golang.org/x/image`.

**Spec:** `docs/superpowers/specs/2026-10-01-pure-go-video-export-design.md`

## Global Constraints

- Module path `github.com/darkliquid/localrpg`; use `interface{}`, never `any`; `go vet ./...` must stay clean.
- Tests use the standard library only (`testing`, `t.TempDir()`); no testify.
- No test may invoke ffmpeg or ffprobe, and no test may require the network.
- Errors wrapped with `fmt.Errorf("...: %w", err)`.
- `CGO_ENABLED=0` must build on linux/amd64, darwin/arm64, and windows/amd64.
- The web exporter (`pkg/export/web.go`) and the theatre components are not touched.
- Conventional Commits with a scope; subject under 72 chars.
- Pin `gen2brain/vpx`, `at-wat/ebml-go`, `srwiley/oksvg`, `srwiley/rasterx` to explicit versions.

---

## File Map

| File | Responsibility after this change |
| --- | --- |
| `pkg/scene/fonts/fonts.go` | embeds the OFL typefaces, parses them, falls back to the Go fonts |
| `pkg/scene/fonts/gen/main.go` | `go:generate` downloader; carries the licences as comments |
| `pkg/scene/fonts/fonts/` | the downloaded `.ttf` files and their `OFL-*.txt` notices (committed) |
| `pkg/scene/scene.go` | gains `DisplayMode` and its constants |
| `pkg/scene/prose.go` | the inline/block grammar `MarkdownProse` applies, as styled runs |
| `pkg/scene/art.go` | loads raster and SVG art into an `image.Image`, cached per path |
| `pkg/scene/render.go` | keeps the shared helpers; `Renderer`/`FrameRequest` live here |
| `pkg/scene/theater.go` | draws one frame: stage, portraits, dialogue, scene card |
| `pkg/media/webm/opus.go` | `OpusTrack`: demuxes clips, lays them on one timeline |
| `pkg/media/webm/encoder.go` | RGB→YUV420 and VP8 keyframe/inter encoding |
| `pkg/media/webm/muxer.go` | VP8 + Opus into a seekable WebM with cues and duration |
| `pkg/media/opus/opus.go` | gains `Duration` (granule-based) |
| `pkg/media/probe.go` | deleted; `ffprobe` leaves the export path |
| `pkg/export/video.go` | orchestrates scene → frames → WebM, writes atomically |
| `pkg/export/script.go` | measures clips with `opus.Duration` |
| `pkg/gui/export.go` | drops the ffmpeg gate; names the artefact `<game-id>.webm` |
| `pkg/gui/server.go` | drops the ffmpeg error case |
| `cmd/localrpg/export.go` | default `.webm`, adds `--quality` |
| `frontend/src/components/ExportModal.tsx` | no ffmpeg gate |
| `frontend/src/types.ts` | no ffmpeg capability fields |
| `README.md` | describes the pure-Go video export |

---

### Task 1: The font set

**Files:**
- Create: `pkg/scene/fonts/fonts.go`
- Create: `pkg/scene/fonts/gen/main.go`
- Create: `pkg/scene/fonts/fonts/.gitkeep`
- Test: `pkg/scene/fonts/fonts_test.go`

**Interfaces:**
- Produces: `fonts.Load() (*fonts.Set, error)`; `Set` fields `Serif`, `SerifItalic`, `Sans`, `SansItalic`, `Mono` (all `*opentype.Font`) and `Bundled bool`.

- [ ] **Step 1: Create the embed directory placeholder**

```bash
mkdir -p pkg/scene/fonts/fonts
touch pkg/scene/fonts/fonts/.gitkeep
```

- [ ] **Step 2: Write the failing test**

```go
package fonts

import "testing"

// A checkout that has not run `go generate` must still load: the Go fonts are the
// fallback, so a build never depends on the download.
func TestLoadAlwaysReturnsFaces(t *testing.T) {
	set, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for name, f := range map[string]interface{ UnitsPerEm() uint16 }{
		"Serif":       set.Serif,
		"SerifItalic": set.SerifItalic,
		"Sans":        set.Sans,
		"SansItalic":  set.SansItalic,
		"Mono":        set.Mono,
	} {
		if f == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if f.UnitsPerEm() == 0 {
			t.Errorf("%s has no units per em", name)
		}
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/scene/fonts/ -run TestLoadAlwaysReturnsFaces`
Expected: FAIL with "undefined: Load"

- [ ] **Step 4: Write the font set**

`pkg/scene/fonts/fonts.go`:

```go
// Package fonts embeds the typefaces the story theatre uses, so the video
// compositor renders the same typography the exported page does. The families are
// OFL; see gen/main.go for their provenance and licences.
package fonts

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

//go:embed fonts
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
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./pkg/scene/fonts/ -run TestLoadAlwaysReturnsFaces`
Expected: PASS

- [ ] **Step 6: Write the downloader**

`pkg/scene/fonts/gen/main.go`. The licence text is inline as comments, as
required, and the downloaded `OFL-*.txt` files travel with the fonts so the
distributed binary carries the notices.

```go
// Command gen downloads the OFL typefaces the video compositor embeds.
//
// Run it with `go generate ./pkg/scene/fonts`. The files land in
// pkg/scene/fonts/fonts/ and are committed, so a build never needs the network;
// if the download is unavailable the fonts package falls back to the Go fonts and
// everything still builds.
//
// The fonts are licensed under the SIL Open Font License, Version 1.1. Each
// family's notice is downloaded beside its .ttf as OFL-<Family>.txt and embedded
// with the binary. The licences are reproduced here so the terms travel with the
// downloader that fetches the fonts.
//
// --- EB Garamond -------------------------------------------------------------
// Copyright 2017 The EB Garamond Project Authors
// (https://github.com/octaviopardo/EBGaramond12)
//
// --- Inter -------------------------------------------------------------------
// Copyright 2016 The Inter Project Authors (https://github.com/rsms/inter)
//
// --- JetBrains Mono ----------------------------------------------------------
// Copyright 2020 The JetBrains Mono Project Authors
// (https://github.com/JetBrains/JetBrainsMono)
//
// -----------------------------------------------------------------------------
// SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007
// -----------------------------------------------------------------------------
//
// PREAMBLE
// The goals of the Open Font License (OFL) are to stimulate worldwide
// development of collaborative font projects, to support the font creation
// efforts of academic and linguistic communities, and to provide a free and
// open framework in which fonts may be shared and improved in partnership
// with others.
//
// The OFL allows the licensed fonts to be used, studied, modified and
// redistributed freely as long as they are not sold by themselves. The
// fonts, including any derivative works, can be bundled, embedded,
// redistributed and/or sold with any software provided that any reserved
// names are not used by derivative works. The fonts and derivatives,
// however, cannot be released under any other type of license. The
// requirement for fonts to remain under this license does not apply
// to any document created using the fonts or their derivatives.
//
// DEFINITIONS
// "Font Software" refers to the set of files released by the Copyright
// Holder(s) under this license and clearly marked as such. This may
// include source files, build scripts and documentation.
//
// "Reserved Font Name" refers to any names specified as such after the
// copyright statement(s).
//
// "Original Version" refers to the collection of Font Software components as
// distributed by the Copyright Holder(s).
//
// "Modified Version" refers to any derivative made by adding to, deleting,
// or substituting -- in part or in whole -- any of the components of the
// Original Version, by changing formats or by porting the Font Software to a
// new environment.
//
// "Author" refers to any designer, engineer, programmer, technical
// writer or other person who contributed to the Font Software.
//
// PERMISSION & CONDITIONS
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of the Font Software, to use, study, copy, merge, embed, modify,
// redistribute, and sell modified and unmodified copies of the Font
// Software, subject to the following conditions:
//
// 1) Neither the Font Software nor any of its individual components,
// in Original or Modified Versions, may be sold by itself.
//
// 2) Original or Modified Versions of the Font Software may be bundled,
// redistributed and/or sold with any software, provided that each copy
// contains the above copyright notice and this license. These can be
// included either as stand-alone text files, human-readable headers or
// in the appropriate machine-readable metadata fields within text or
// binary files as long as those fields can be easily viewed by the user.
//
// 3) No Modified Version of the Font Software may use the Reserved Font
// Name(s) unless explicit written permission is granted by the corresponding
// Copyright Holder. This restriction only applies to the primary font name as
// presented to the users.
//
// 4) The name(s) of the Copyright Holder(s) or the Author(s) of the Font
// Software shall not be used to promote, endorse or advertise any
// Modified Version, except to acknowledge the contribution(s) of the
// Copyright Holder(s) and the Author(s) or with their explicit written
// permission.
//
// 5) The Font Software, modified or unmodified, in part or in whole,
// must be distributed entirely under this license, and must not be
// distributed under any other license. The requirement for fonts to
// remain under this license does not apply to any document created
// using the Font Software.
//
// TERMINATION
// This license becomes null and void if any of the above conditions are
// not met.
//
// DISCLAIMER
// THE FONT SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO ANY WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT
// OF COPYRIGHT, PATENT, TRADEMARK, OR OTHER RIGHT. IN NO EVENT SHALL THE
// COPYRIGHT HOLDER BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// INCLUDING ANY GENERAL, SPECIAL, INDIRECT, INCIDENTAL, OR CONSEQUENTIAL
// DAMAGES, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF THE USE OR INABILITY TO USE THE FONT SOFTWARE OR FROM
// OTHER DEALINGS IN THE FONT SOFTWARE.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ref pins google/fonts to one commit so a re-run is reproducible.
const ref = "9710da1eacb3be272583c3224dcb70f9da6eadbb"

const base = "https://raw.githubusercontent.com/google/fonts/" + ref + "/"

// source maps a destination file to its pinned source. The []byte are
// percent-encoded because the upstream filenames carry [ ] and ,.
var source = []struct{ dest, src string }{
	{"EBGaramond.ttf", base + "ofl/ebgaramond/EBGaramond%5Bwght%5D.ttf"},
	{"EBGaramond-Italic.ttf", base + "ofl/ebgaramond/EBGaramond-Italic%5Bwght%5D.ttf"},
	{"OFL-EBGaramond.txt", base + "ofl/ebgaramond/OFL.txt"},
	{"Inter.ttf", base + "ofl/inter/Inter%5Bopsz%2Cwght%5D.ttf"},
	{"Inter-Italic.ttf", base + "ofl/inter/Inter-Italic%5Bopsz%2Cwght%5D.ttf"},
	{"OFL-Inter.txt", base + "ofl/inter/OFL.txt"},
	{"JetBrainsMono.ttf", base + "ofl/jetbrainsmono/JetBrainsMono%5Bwght%5D.ttf"},
	{"OFL-JetBrainsMono.txt", base + "ofl/jetbrainsmono/OFL.txt"},
}

func main() {
	dir := "fonts"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	for _, entry := range source {
		if err := fetch(entry.src, filepath.Join(dir, entry.dest)); err != nil {
			fmt.Fprintf(os.Stderr, "gen: %s: %v\n", entry.dest, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", filepath.Join(dir, entry.dest))
	}
}

func fetch(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
```

- [ ] **Step 7: Declare the generate directive and run it**

Add to the top of `pkg/scene/fonts/fonts.go`, immediately above `package fonts`:

```go
//go:generate go run ./gen fonts
```

Run: `go generate ./pkg/scene/fonts/`
Expected: nine `wrote fonts/...` lines. If the network is unavailable, skip this
step; the fallback keeps the build working and Step 5 still passes.

- [ ] **Step 8: Commit**

```bash
git add pkg/scene/fonts
git commit -m "feat(scene): embed the theatre's typefaces for video frames"
```

---

### Task 2: The inline prose grammar

**Files:**
- Create: `pkg/scene/prose.go`
- Modify: `pkg/scene/scene.go` (add `DisplayMode`)
- Test: `pkg/scene/prose_test.go`

**Interfaces:**
- Produces: `type DisplayMode string` and `DisplayStageDirections`, `DisplayHidden`, `DisplayRaw`; `type RunStyle int` with `StyleRegular`, `StyleBold`, `StyleItalic`, `StyleCode`, `StyleLink`, `StyleDirection`; `type Run struct{ Text string; Style RunStyle }`; `type Block struct{ Kind BlockKind; Runs []Run }`; `func ParseProse(text string, mode DisplayMode) []Block`.

- [ ] **Step 1: Write the failing tests**

```go
package scene

import (
	"strings"
	"testing"
)

func plainRuns(blocks []Block) string {
	var b strings.Builder
	for _, block := range blocks {
		for _, run := range block.Runs {
			b.WriteString(run.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestParseProseRendersAWikilinkLabel(t *testing.T) {
	blocks := ParseProse("She met [[lady-evelyn|Lady Evelyn]] there.", DisplayStageDirections)
	if got := plainRuns(blocks); !strings.Contains(got, "Lady Evelyn") || strings.Contains(got, "[[") {
		t.Fatalf("wikilink not unwrapped: %q", got)
	}
	run := blocks[0].Runs[1]
	if run.Style != StyleLink {
		t.Errorf("wikilink style = %v, want StyleLink", run.Style)
	}
}

func TestParseProseAppliesTheInlineStyles(t *testing.T) {
	blocks := ParseProse("a **bold** *ital* `code` end", DisplayStageDirections)
	styles := map[string]RunStyle{}
	for _, run := range blocks[0].Runs {
		styles[run.Text] = run.Style
	}
	if styles["bold"] != StyleBold || styles["ital"] != StyleItalic || styles["code"] != StyleCode {
		t.Fatalf("styles = %v", styles)
	}
}

func TestParseProseHonoursDisplayModeForDirections(t *testing.T) {
	text := "He paused [whispering] then spoke."

	shown := ParseProse(text, DisplayStageDirections)
	if plainRuns(shown) != "He paused whispering then spoke.\n" {
		t.Fatalf("stage_directions = %q", plainRuns(shown))
	}

	hidden := ParseProse(text, DisplayHidden)
	if strings.Contains(plainRuns(hidden), "whispering") {
		t.Fatalf("hidden kept the direction: %q", plainRuns(hidden))
	}

	raw := ParseProse(text, DisplayRaw)
	if !strings.Contains(plainRuns(raw), "[whispering]") {
		t.Fatalf("raw dropped the brackets: %q", plainRuns(raw))
	}
}

func TestParseProseSplitsParagraphsOnBlankLines(t *testing.T) {
	blocks := ParseProse("one\n\ntwo", DisplayStageDirections)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/scene/ -run TestParseProse`
Expected: FAIL with "undefined: ParseProse"

- [ ] **Step 3: Add the display mode to the scene model**

Append to `pkg/scene/scene.go`:

```go
// DisplayMode is how the prose grammar treats a performance direction such as
// "[whispering]": the theatre's setting, mirrored from the campaign's speech cues.
type DisplayMode string

const (
	// DisplayStageDirections shows a direction as a styled pill.
	DisplayStageDirections DisplayMode = "stage_directions"
	// DisplayHidden strips a direction from the prose.
	DisplayHidden DisplayMode = "hidden"
	// DisplayRaw shows a direction verbatim.
	DisplayRaw DisplayMode = "raw"
)
```

- [ ] **Step 4: Write the grammar**

`pkg/scene/prose.go`:

```go
package scene

import (
	"regexp"
	"strings"
)

// RunStyle is the inline style of a run of prose.
type RunStyle int

const (
	// StyleRegular is unstyled body prose.
	StyleRegular RunStyle = iota
	// StyleBold is **strong** text.
	StyleBold
	// StyleItalic is *emphasised* or _emphasised_ text.
	StyleItalic
	// StyleCode is `inline code`.
	StyleCode
	// StyleLink is the label of a [[wikilink]].
	StyleLink
	// StyleDirection is a [performance direction].
	StyleDirection
)

// Run is a span of prose with one inline style.
type Run struct {
	Text  string
	Style RunStyle
}

// BlockKind is the block structure of a paragraph of prose.
type BlockKind int

const (
	// BlockParagraph is ordinary prose.
	BlockParagraph BlockKind = iota
	// BlockHeading is a # or ## line.
	BlockHeading
	// BlockList is a run of - or * items.
	BlockList
	// BlockQuote is a > line.
	BlockQuote
	// BlockRule is a --- line.
	BlockRule
)

// Block is one paragraph of prose, split into styled runs.
type Block struct {
	Kind  BlockKind
	Runs  []Run
	Level int // heading level, for BlockHeading
}

// inlinePattern mirrors the theatre's MarkdownProse grammar exactly, so the video
// formats the same string the exported page does.
var inlinePattern = regexp.MustCompile(`(\[\[[^\]]+\]\])|(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])|(` + "`[^`]+`" + `)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)`)

var (
	headingPattern  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listItemPattern = regexp.MustCompile(`^\s*[-*]\s+`)
	rulePattern     = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
)

// ParseProse splits text into blocks and their inline runs, applying the same
// grammar MarkdownProse applies in the player.
func ParseProse(text string, mode DisplayMode) []Block {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if mode == DisplayHidden {
		normalized = hiddenDirections.ReplaceAllString(normalized, " ")
	}
	if strings.TrimSpace(normalized) == "" {
		return nil
	}

	raw := strings.Split(normalized, "\n\n")
	blocks := make([]Block, 0, len(raw))
	for _, chunk := range raw {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		if rulePattern.MatchString(trimmed) {
			blocks = append(blocks, Block{Kind: BlockRule})
			continue
		}
		if m := headingPattern.FindStringSubmatch(trimmed); m != nil {
			blocks = append(blocks, Block{Kind: BlockHeading, Level: len(m[1]), Runs: inlineRuns(m[2], mode)})
			continue
		}
		if lines := strings.Split(chunk, "\n"); allMatch(lines, listItemPattern) {
			for _, line := range lines {
				blocks = append(blocks, Block{Kind: BlockList, Runs: inlineRuns(listItemPattern.ReplaceAllString(line, ""), mode)})
			}
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			joined := strings.Join(mapLines(strings.Split(chunk, "\n"), quotePrefix), "\n")
			blocks = append(blocks, Block{Kind: BlockQuote, Runs: inlineRuns(joined, mode)})
			continue
		}
		blocks = append(blocks, Block{Kind: BlockParagraph, Runs: inlineRuns(chunk, mode)})
	}
	return blocks
}

var (
	hiddenDirections = regexp.MustCompile(`(?:^|\s)\[[a-zA-Z][a-zA-Z\s_-]{1,28}\](?:\s|$)`)
	quotePrefix      = regexp.MustCompile(`^>\s?`)
)

func allMatch(lines []string, pattern *regexp.Regexp) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !pattern.MatchString(line) {
			return false
		}
	}
	return true
}

func mapLines(lines []string, pattern *regexp.Regexp) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = pattern.ReplaceAllString(line, "")
	}
	return out
}

func inlineRuns(text string, mode DisplayMode) []Run {
	runs := make([]Run, 0, 8)
	last := 0
	for _, match := range inlinePattern.FindAllStringIndex(text, -1) {
		if match[0] > last {
			runs = append(runs, Run{Text: text[last:match[0]], Style: StyleRegular})
		}
		token := text[match[0]:match[1]]
		runs = append(runs, styleToken(token, mode))
		last = match[1]
	}
	if last < len(text) {
		runs = append(runs, Run{Text: text[last:], Style: StyleRegular})
	}
	return runs
}

func styleToken(token string, mode DisplayMode) Run {
	switch {
	case strings.HasPrefix(token, "[["):
		inner := token[2 : len(token)-2]
		label := inner
		if sep := strings.Index(inner, "|"); sep != -1 {
			label = strings.TrimSpace(inner[sep+1:])
		}
		return Run{Text: label, Style: StyleLink}
	case strings.HasPrefix(token, "["):
		switch mode {
		case DisplayHidden:
			return Run{Text: "", Style: StyleDirection}
		case DisplayRaw:
			return Run{Text: token, Style: StyleDirection}
		default:
			return Run{Text: token[1 : len(token)-1], Style: StyleDirection}
		}
	case strings.HasPrefix(token, "`"):
		return Run{Text: token[1 : len(token)-1], Style: StyleCode}
	case strings.HasPrefix(token, "**"):
		return Run{Text: token[2 : len(token)-2], Style: StyleBold}
	default:
		return Run{Text: token[1 : len(token)-1], Style: StyleItalic}
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/scene/ -run TestParseProse`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/scene/scene.go pkg/scene/prose.go pkg/scene/prose_test.go
git commit -m "feat(scene): parse the theatre's prose grammar for frames"
```

---

### Task 3: The art loader

**Files:**
- Create: `pkg/scene/art.go`
- Test: `pkg/scene/art_test.go`
- Modify: `go.mod` (add `srwiley/oksvg`, `srwiley/rasterx`)

**Interfaces:**
- Produces: `type artCache struct{}`; `newArtCache() *artCache`; `(*artCache).load(path string) (image.Image, error)`; `(*artCache).cover(dst *image.RGBA, src image.Image, scale float64)`.

- [ ] **Step 1: Add the dependencies**

Run: `go get github.com/srwiley/oksvg@latest github.com/srwiley/rasterx@latest`

- [ ] **Step 2: Write the failing tests**

```go
package scene

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestArtCacheRasterizesTheProceduralBust(t *testing.T) {
	path := writeFile(t, "bust.svg", media.GenerateProceduralBustSVG("evelyn", "Evelyn", "female"))
	img, err := newArtCache().load(path)
	if err != nil {
		t.Fatalf("load SVG: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatalf("rasterized SVG is empty: %v", img.Bounds())
	}
}

func TestArtCacheLoadsRasterArt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	file.Close()

	img, err := newArtCache().load(path)
	if err != nil {
		t.Fatalf("load PNG: %v", err)
	}
	if img.Bounds().Dx() != 8 {
		t.Fatalf("bounds = %v", img.Bounds())
	}
}

func TestArtCacheReportsAMissingFile(t *testing.T) {
	if _, err := newArtCache().load(filepath.Join(t.TempDir(), "absent.png")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
```

Add `"image/png"` and `"os"` to the test imports.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./pkg/scene/ -run TestArtCache`
Expected: FAIL with "undefined: newArtCache"

- [ ] **Step 4: Write the loader**

`pkg/scene/art.go`:

```go
package scene

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
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
	box := icon.ViewBox
	width, height := int(box.W), int(box.H)
	if width < 1 || height < 1 {
		width, height = 256, 256
	}
	icon.SetTarget(0, 0, float64(width), float64(height))

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	scanner := rasterx.NewScannerGV(width, height, img, img.Bounds())
	icon.Draw(rasterx.NewDasher(width, height, scanner), 1.0)
	return img, nil
}

// cover scales src to cover dst, centred, so art of any aspect ratio fills the
// frame without distortion.
func (c *artCache) cover(dst *image.RGBA, src image.Image, scale float64) {
	drawCover(dst, src, scale)
}
```

- [ ] **Step 5: Remove the old decoder from `render.go`**

In `pkg/scene/render.go`, delete the `loadArt` function and the now-unused
decoder imports (`image/jpeg`, `image/png`, `golang.org/x/image/webp`) and the
`os` import, because `art.go` owns art decoding now. Keep `drawCover`,
`scaleImage`, `drawCoverAlpha`, `clamp01`, `revealText`, `wrapText`, and
`proceduralBackground` in `render.go`; Task 4 removes the ones the new stage no
longer uses.

Run: `go build ./pkg/scene/`
Expected: builds (some helpers may now be unused; Task 4 removes them).

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/scene/ -run TestArtCache`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/scene/art.go pkg/scene/art_test.go pkg/scene/render.go go.mod go.sum
git commit -m "feat(scene): load raster and SVG art for video frames"
```

---

### Task 4: The stage frame

**Files:**
- Create: `pkg/scene/theater.go`
- Modify: `pkg/scene/render.go`
- Test: `pkg/scene/theater_test.go`

**Interfaces:**
- Consumes: `fonts.Set` (Task 1), `artCache` (Task 3).
- Produces: `type FrameRequest struct{ Script *Script; SceneIndex int; Scene Scene; Beat Beat; Progress float64; PreviousArt string; Animate bool; DisplayMode DisplayMode }`; `func NewRenderer(width, height int) (*Renderer, error)`; `func (r *Renderer) Frame(req FrameRequest) *image.RGBA`.

- [ ] **Step 1: Write the failing test**

```go
package scene

import (
	"image/color"
	"testing"
)

func stageScript() *Script {
	return &Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			Beats:        []Beat{{Kind: BeatNarration, Text: "Warm light.", Duration: 2000 * time.Millisecond}},
		}},
	}
}

func TestFrameDrawsTheBackgroundAndScrim(t *testing.T) {
	renderer, err := NewRenderer(320, 180)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	script := stageScript()
	frame := renderer.Frame(FrameRequest{
		Script: script, Scene: script.Scenes[0], Beat: script.Scenes[0].Beats[0],
		Progress: 1, Animate: false,
	})

	if frame.Bounds().Dx() != 320 || frame.Bounds().Dy() != 180 {
		t.Fatalf("frame bounds = %v", frame.Bounds())
	}
	// The top of the scrim is darker than the base colour, so a blank canvas
	// would fail this: the stage must have painted something.
	if frame.RGBAAt(160, 2) == (color.RGBA{0, 0, 0, 0}) {
		t.Fatal("expected the stage to fill the frame")
	}
}
```

Add `"time"` to the test imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/scene/ -run TestFrameDraws`
Expected: FAIL with "too many values" or "unknown field" against the old `FrameRequest`

- [ ] **Step 3: Replace the renderer core**

Replace the `FrameRequest`, `Renderer`, and `NewRenderer` definitions in
`pkg/scene/render.go` with the following, and delete the old `Frame` and
`drawBackground`, `sceneArt`, `crossfadeAlpha`, `drawText`, `drawCentred`,
`wrapText` (the stage in `theater.go` replaces them). Keep `baseColour`,
`drawCover`, `scaleImage`, `drawCoverAlpha`, `clamp01`, `revealText`,
`proceduralBackground`, and the drift/crossfade constants.

```go
// FrameRequest describes one frame to draw.
type FrameRequest struct {
	Script      *Script
	SceneIndex  int
	Scene       Scene
	Beat        Beat
	Progress    float64 // 0 at the beat's start, 1 at its end
	PreviousArt string  // the outgoing scene's art, for the crossfade
	Animate     bool
	DisplayMode DisplayMode
}

// Renderer draws the story theatre's own stage into video frames. Faces live as
// long as the renderer, which keeps parsing the embedded fonts to once per export.
type Renderer struct {
	width  int
	height int
	fonts  *fonts.Set
	art    *artCache
	faces  map[faceKey]font.Face
}

// NewRenderer builds a frame renderer at the given size.
func NewRenderer(width, height int) (*Renderer, error) {
	set, err := fonts.Load()
	if err != nil {
		return nil, fmt.Errorf("load fonts: %w", err)
	}
	return &Renderer{width: width, height: height, fonts: set, art: newArtCache(), faces: map[faceKey]font.Face{}}, nil
}

// Frame draws one frame: the stage behind, the beat's dialogue over it.
func (r *Renderer) Frame(req FrameRequest) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))
	draw.Draw(img, img.Bounds(), image.NewUniform(baseColour), image.Point{}, draw.Src)

	r.drawBackground(img, req)
	r.drawScrim(img)
	r.drawHeader(img, req)
	r.drawPortraits(img, req)
	r.drawDialogue(img, req)
	return img
}

// faceKey names a sized face so it is parsed once per renderer.
type faceKey struct {
	family string
	size   int
}
```

Add the `fonts` import to `render.go`:

```go
"github.com/darkliquid/localrpg/pkg/scene/fonts"
```

- [ ] **Step 4: Write the stage layout**

`pkg/scene/theater.go`:

```go
package scene

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Colours are the theatre's, copied from the Tailwind classes the components use.
var (
	scrimColour   = color.RGBA{0, 0, 0, 255}
	headerPurple  = color.RGBA{216, 180, 254, 255} // purple-300
	stoneText     = color.RGBA{231, 229, 228, 255} // stone-200
	stoneBright   = color.RGBA{250, 250, 249, 255} // stone-50
	panelFill     = color.RGBA{12, 10, 9, 230}     // stone-950/90
	amberLabel    = color.RGBA{252, 211, 77, 255}  // amber-300
	skyAccent     = color.RGBA{56, 189, 248, 255}  // sky-400
	purpleAccent  = color.RGBA{192, 132, 252, 255} // purple-400
	inactiveEdge  = color.RGBA{255, 255, 255, 76}  // white/30
	chipBackground = color.RGBA{0, 0, 0, 153}      // black/60
)

// face returns a sized face, parsing it once per renderer.
func (r *Renderer) face(f *opentype.Font, size int) font.Face {
	key := faceKey{family: fontName(f), size: size}
	if cached, ok := r.faces[key]; ok {
		return cached
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	r.faces[key] = face
	return face
}

func fontName(f *opentype.Font) string {
	if f == nil {
		return "nil"
	}
	return f.Name(nil, 0).String()
}

// drawBackground paints the scene's imagery: the beat's art, the scene's art, or
// the campaign banner, cover-fit. A scene change blends in over the opening share
// of a beat, and the drift scale keeps a held beat alive. With no art at all the
// stage shows the theatre's own radial gradient.
func (r *Renderer) drawBackground(img *image.RGBA, req FrameRequest) {
	scale := driftStart + (driftEnd-driftStart)*clamp01(req.Progress)

	art := r.backgroundArt(req)
	if art == nil {
		drawRadialGradient(img, color.RGBA{38, 30, 27, 255}, baseColour)
		return
	}

	if blend := crossfadeAlpha(req.Progress); blend < 1 && req.PreviousArt != "" {
		if previous, err := r.art.load(req.PreviousArt); err == nil {
			drawCover(img, previous, scale)
			drawCoverAlpha(img, art, scale, blend)
			return
		}
	}
	drawCover(img, art, scale)
}

// backgroundArt is the first art the theatre would show: the beat's own, then the
// scene's, then the campaign's banner.
func (r *Renderer) backgroundArt(req FrameRequest) image.Image {
	for _, path := range []string{req.Beat.ArtPath, req.Scene.ArtPath} {
		if img, err := r.art.load(path); err == nil {
			return img
		}
	}
	if req.Script != nil && req.Script.Banner != "" {
		if img, err := r.art.load(req.Script.Banner); err == nil {
			return img
		}
	}
	return nil
}

// drawScrim is the theatre's bottom-heavy black gradient: black/90 at the base,
// black/45 in the middle, black/60 at the top.
func (r *Renderer) drawScrim(img *image.RGBA) {
	for y := 0; y < r.height; y++ {
		t := float64(y) / float64(max(1, r.height-1))
		alpha := scrimAlpha(t)
		row := color.RGBA{0, 0, 0, alpha}
		draw.Draw(img, image.Rect(0, y, r.width, y+1), image.NewUniform(row), image.Point{}, draw.Over)
	}
}

func scrimAlpha(t float64) uint8 {
	var a float64
	switch {
	case t < 0.5:
		a = 0.60 + (0.45-0.60)*(t/0.5)
	default:
		a = 0.45 + (0.90-0.45)*((t-0.5)/0.5)
	}
	return uint8(a * 255)
}

// drawRadialGradient fills img with the theatre's no-art background: a warm
// centre fading to the base colour.
func drawRadialGradient(img *image.RGBA, centre, edge color.RGBA) {
	b := img.Bounds()
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	radius := math.Hypot(cx, cy)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / radius
			img.SetRGBA(x, y, lerpColour(centre, edge, clamp01(d)))
		}
	}
}

func lerpColour(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}
```

Add `"math"` to the imports.

- [ ] **Step 5: Stub the remaining stage methods so the package builds**

Append to `theater.go` the three methods the next tasks fill in, each drawing
nothing yet:

```go
func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest)    {}
func (r *Renderer) drawPortraits(img *image.RGBA, req FrameRequest) {}
func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest)  {}
```

Also add the string helper the later tasks need:

```go
// upper is the theatre's label case: uppercase, trimmed.
func upper(text string) string { return strings.ToUpper(strings.TrimSpace(text)) }
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./pkg/scene/ -run TestFrameDraws`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/scene/theater.go pkg/scene/render.go pkg/scene/theater_test.go
git commit -m "feat(scene): draw the theatre stage as a video frame"
```

---

### Task 5: The portrait band

**Files:**
- Modify: `pkg/scene/theater.go`
- Test: `pkg/scene/theater_test.go`

**Interfaces:**
- Consumes: `FrameRequest` (Task 4), `artCache` (Task 3).
- Produces: `func (r *Renderer) drawPortraits(img *image.RGBA, req FrameRequest)` filling the two square boxes.

- [ ] **Step 1: Write the failing test**

```go
func TestFrameDrawsTheActiveSpeakerBorder(t *testing.T) {
	renderer, err := NewRenderer(640, 360)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	script := stageScript()
	script.PlayerPortrait = writeFile(t, "player.svg", media.GenerateProceduralBustSVG("hero", "Hero", "female"))
	beat := Beat{Kind: BeatSpeech, Speaker: "Hero", Player: true, Text: "Hello.", Duration: 2000 * time.Millisecond}

	frame := renderer.Frame(FrameRequest{
		Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1, Animate: false,
	})

	if !hasColourNear(frame, skyAccent, 60) {
		t.Fatal("expected the player's active border colour somewhere in the frame")
	}
}

// hasColourNear reports whether any pixel is within tolerance of want.
func hasColourNear(img *image.RGBA, want color.RGBA, tolerance int) bool {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.RGBAAt(x, y)
			if absDiff(c.R, want.R) <= tolerance && absDiff(c.G, want.G) <= tolerance && absDiff(c.B, want.B) <= tolerance {
				return true
			}
		}
	}
	return false
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a) - int(b)
	}
	return int(b) - int(a)
}
```

Add `"github.com/darkliquid/localrpg/pkg/media"` to the test imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/scene/ -run TestFrameDrawsTheActiveSpeakerBorder`
Expected: FAIL with "expected the player's active border colour"

- [ ] **Step 3: Implement the band**

Replace the `drawPortraits` stub in `theater.go`:

```go
// drawPortraits paints the theatre's two square character boxes: the protagonist
// on the left for the whole story, and the current speaker on the right. The
// active speaker gets a coloured border and glow; a portrait is never dimmed.
func (r *Renderer) drawPortraits(img *image.RGBA, req FrameRequest) {
	playerPortrait := ""
	if req.Script != nil {
		playerPortrait = req.Script.PlayerPortrait
	}
	npcPortrait := ""
	npcLabel := ""
	if req.Beat.Kind == BeatSpeech && !req.Beat.Player {
		npcPortrait = req.Beat.PortraitPath
		npcLabel = req.Beat.Speaker
	}

	box := int(math.Min(float64(r.width)*0.24, float64(r.height)*0.34))
	box = max(box, 48)
	margin := int(float64(r.width) * 0.04)
	bandTop := int(float64(r.height) * 0.08)
	bandBottom := int(float64(r.height) * 0.70)
	top := bandBottom - box

	if playerPortrait != "" {
		label := ""
		if req.Script != nil {
			label = req.Script.PlayerName
		}
		r.drawPortraitBox(img, playerPortrait, margin, top, box, req.Beat.Player, skyAccent, upper(label), req.Beat.Player)
	}
	if npcPortrait != "" {
		right := r.width - margin - box
		r.drawPortraitBox(img, npcPortrait, right, top, box, true, purpleAccent, upper(npcLabel), true)
	}
	_ = bandTop
}

// drawPortraitBox draws one rounded, bordered portrait with its label chip.
func (r *Renderer) drawPortraitBox(img *image.RGBA, path string, x, y, size int, active bool, accent color.RGBA, label string, mirror bool) {
	edge := inactiveEdge
	if active {
		edge = accent
	}
	fillRoundRect(img, image.Rect(x, y, x+size, y+size), size/8, color.RGBA{28, 25, 23, 255})
	if active {
		glowRoundRect(img, image.Rect(x, y, x+size, y+size), size/8, accent)
	}

	if art, err := r.art.load(path); err == nil {
		drawCoverRect(img, art, image.Rect(x, y, x+size, y+size), mirror)
	}
	strokeRoundRect(img, image.Rect(x, y, x+size, y+size), size/8, 2, edge)

	if label != "" {
		r.drawChip(img, x+size/2, y+size+int(float64(r.height)*0.02), label, active, accent)
	}
}
```

Add the rounded-rectangle and cover-rect helpers to `theater.go`:

```go
// fillRoundRect fills a rounded rectangle.
func fillRoundRect(img *image.RGBA, rect image.Rectangle, radius int, fill color.RGBA) {
	drawRoundRect(img, rect, radius, func(x, y int) {
		img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), fill))
	})
}

// strokeRoundRect outlines a rounded rectangle with the given width.
func strokeRoundRect(img *image.RGBA, rect image.Rectangle, radius, width int, stroke color.RGBA) {
	for w := 0; w < width; w++ {
		inner := rect.Inset(w)
		outlineRoundRect(img, inner, radius, func(x, y int) {
			img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), stroke))
		})
	}
}

// glowRoundRect draws a soft coloured halo just outside a rounded rectangle.
func glowRoundRect(img *image.RGBA, rect image.Rectangle, radius int, accent color.RGBA) {
	const spread = 14
	for i := 0; i < spread; i++ {
		alpha := uint8(float64(90) * (1 - float64(i)/spread))
		tint := color.RGBA{accent.R, accent.G, accent.B, alpha}
		outlineRoundRect(img, rect.Inset(-i), radius+i, func(x, y int) {
			img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), tint))
		})
	}
}

// drawRoundRect visits the pixels inside a rounded rectangle.
func drawRoundRect(img *image.RGBA, rect image.Rectangle, radius int, visit func(x, y int)) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			if insideRounded(rect, radius, x, y) {
				visit(x, y)
			}
		}
	}
}

// outlineRoundRect visits the one-pixel border of a rounded rectangle.
func outlineRoundRect(img *image.RGBA, rect image.Rectangle, radius int, visit func(x, y int)) {
	drawRoundRect(img, rect, radius, func(x, y int) {
		if !insideRounded(rect.Inset(1), radius-1, x, y) {
			visit(x, y)
		}
	})
}

func insideRounded(rect image.Rectangle, radius, x, y int) bool {
	if radius <= 0 {
		return image.Pt(x, y).In(rect)
	}
	if !image.Pt(x, y).In(rect) {
		return false
	}
	corners := []struct{ cx, cy int }{
		{rect.Min.X + radius, rect.Min.Y + radius},
		{rect.Max.X - radius - 1, rect.Min.Y + radius},
		{rect.Min.X + radius, rect.Max.Y - radius - 1},
		{rect.Max.X - radius - 1, rect.Max.Y - radius - 1},
	}
	dx := 0
	if x < rect.Min.X+radius {
		dx = rect.Min.X + radius - x
	} else if x > rect.Max.X-radius-1 {
		dx = x - (rect.Max.X - radius - 1)
	}
	dy := 0
	if y < rect.Min.Y+radius {
		dy = rect.Min.Y + radius - y
	} else if y > rect.Max.Y-radius-1 {
		dy = y - (rect.Max.Y - radius - 1)
	}
	if dx == 0 || dy == 0 {
		return true
	}
	_ = corners
	return dx*dx+dy*dy <= radius*radius
}

// blendOver composites src over dst by its alpha.
func blendOver(dst, src color.RGBA) color.RGBA {
	a := float64(src.A) / 255
	return color.RGBA{
		R: uint8(float64(dst.R)*(1-a) + float64(src.R)*a),
		G: uint8(float64(dst.G)*(1-a) + float64(src.G)*a),
		B: uint8(float64(dst.B)*(1-a) + float64(src.B)*a),
		A: 255,
	}
}

// drawCoverRect cover-fits art into a rectangle, optionally mirrored.
func drawCoverRect(img *image.RGBA, art image.Image, rect image.Rectangle, mirror bool) {
	scaled := scaleToCover(art, rect.Dx(), rect.Dy())
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			sx := x
			if mirror {
				sx = rect.Dx() - 1 - x
			}
			if !image.Pt(rect.Min.X+x, rect.Min.Y+y).In(img.Bounds()) {
				continue
			}
			img.Set(rect.Min.X+x, rect.Min.Y+y, scaled.At(sx, y))
		}
	}
}

// drawChip draws a rounded label chip centred on (cx, baselineTop).
func (r *Renderer) drawChip(img *image.RGBA, cx, top int, label string, active bool, accent color.RGBA) {
	face := r.face(r.fonts.Sans, max(12, r.height/52))
	if face == nil {
		return
	}
	textWidth := font.MeasureString(face, label).Ceil()
	height := face.Metrics().Height.Ceil() + r.height/60
	rect := image.Rect(cx-textWidth/2-r.height/80, top, cx+textWidth/2+r.height/80, top+height)

	fill := chipBackground
	if active {
		fill = accent
	}
	fillRoundRect(img, rect, height/2, fill)
	r.drawText(img, face, label, rect.Min.X+r.height/80, rect.Min.Y+face.Metrics().Ascent.Ceil()+r.height/120, color.RGBA{255, 255, 255, 255}, false)
}
```

Also add the `scaleToCover` helper and the text drawing helpers in Task 6; for
this task add `scaleToCover` to `theater.go`:

```go
// scaleToCover resamples src to fill width×height, cropping the overflow.
func scaleToCover(src image.Image, width, height int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW < 1 || srcH < 1 {
		return image.NewRGBA(image.Rect(0, 0, width, height))
	}
	scale := math.Max(float64(width)/float64(srcW), float64(height)/float64(srcH))
	targetW := int(float64(srcW) * scale)
	targetH := int(float64(srcH) * scale)
	resized := scaleImage(src, max(1, targetW), max(1, targetH))

	out := image.NewRGBA(image.Rect(0, 0, width, height))
	offX := (resized.Bounds().Dx() - width) / 2
	offY := (resized.Bounds().Dy() - height) / 2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.Set(x, y, resized.At(x+offX, y+offY))
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/scene/ -run TestFrameDrawsTheActiveSpeakerBorder`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/theater.go pkg/scene/theater_test.go
git commit -m "feat(scene): draw the theatre's portrait boxes"
```

---

### Task 6: The dialogue panel and prose

**Files:**
- Modify: `pkg/scene/theater.go`
- Test: `pkg/scene/theater_test.go`

**Interfaces:**
- Consumes: `ParseProse` (Task 2), `face`/`drawPortraitBox` (Task 5).
- Produces: `func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest)`; `func (r *Renderer) drawText(img *image.RGBA, face font.Face, text string, x, baseline int, col color.RGBA, bold bool)`; `func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFrameDrawsTheSpeakerNamePlate(t *testing.T) {
	renderer, _ := NewRenderer(640, 360)
	script := stageScript()
	beat := Beat{Kind: BeatSpeech, Speaker: "Evelyn", Text: "Well met.", Duration: 2000 * time.Millisecond}

	frame := renderer.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1})

	if !hasColourNear(frame, purpleAccent, 40) {
		t.Fatal("expected the speech panel's purple accent")
	}
}

func TestFrameDrawsTheSceneCardInAmber(t *testing.T) {
	renderer, _ := NewRenderer(640, 360)
	script := stageScript()
	beat := SceneCard(script.Scenes[0])

	frame := renderer.Frame(FrameRequest{Script: script, Scene: script.Scenes[0], Beat: beat, Progress: 1})

	if !hasColourNear(frame, amberLabel, 30) {
		t.Fatal("expected the scene card's amber title")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/scene/ -run 'TestFrameDrawsTheSpeakerNamePlate|TestFrameDrawsTheSceneCard'`
Expected: FAIL

- [ ] **Step 3: Implement the header, text, and dialogue**

Replace the `drawHeader` and `drawDialogue` stubs and add the text helpers in
`theater.go`:

```go
// drawHeader is the theatre's top band: the campaign's name, the location pill,
// and the scene counter.
func (r *Renderer) drawHeader(img *image.RGBA, req FrameRequest) {
	if req.Script == nil {
		return
	}
	face := r.face(r.fonts.Sans, max(14, r.height/40))
	if face == nil {
		return
	}
	top := int(float64(r.height) * 0.03)
	r.drawText(img, face, upper(req.Script.GameName), int(float64(r.width)*0.03), top+face.Metrics().Ascent.Ceil(), headerPurple, true)

	x := int(float64(r.width) * 0.03)
	y := top + face.Metrics().Height.Ceil() + r.height/60
	if req.Scene.LocationName != "" {
		x = r.drawPill(img, face, req.Scene.LocationName, x, y, chipBackground, stoneText) + r.height/80
	}
	if req.Script != nil && len(req.Script.Scenes) > 0 {
		counter := fmt.Sprintf("Scene %d of %d", req.SceneIndex+1, len(req.Script.Scenes))
		r.drawPill(img, r.face(r.fonts.Mono, max(12, r.height/56)), counter, x, y, chipBackground, stoneText)
	}
}

// drawPill draws a rounded chip and returns the x it ended at.
func (r *Renderer) drawPill(img *image.RGBA, face font.Face, label string, x, top int, fill, text colour) int {
	if face == nil {
		return x
	}
	padding := r.height / 60
	textWidth := font.MeasureString(face, label).Ceil()
	height := face.Metrics().Height.Ceil() + padding
	rect := image.Rect(x, top, x+textWidth+2*padding, top+height)
	fillRoundRect(img, rect, height/3, fill)
	r.drawText(img, face, label, rect.Min.X+padding, rect.Min.Y+face.Metrics().Ascent.Ceil()+padding/2, text, false)
	return rect.Max.X
}

// drawText draws a line, synthesising bold by drawing it twice a pixel apart
// because the embedded families are variable fonts x/image cannot instance.
func (r *Renderer) drawText(img *image.RGBA, face font.Face, text string, x, baseline int, col color.RGBA, bold bool) {
	if face == nil || strings.TrimSpace(text) == "" {
		return
	}
	drawer := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face, Dot: fixed.P(x, baseline)}
	drawer.DrawString(text)
	if bold {
		drawer.Dot = fixed.P(x+1, baseline)
		drawer.DrawString(text)
	}
}

// drawDialogue draws the bottom panel: a scene card is a centred amber title,
// anything else is the name plate, the prose, and the advance caret.
func (r *Renderer) drawDialogue(img *image.RGBA, req FrameRequest) {
	panel := r.dialogueRect()
	if req.Beat.Kind == BeatSceneCard {
		r.drawSceneCard(img, panel, req)
		return
	}

	shown := req.Beat.Text
	if req.Animate {
		shown = revealText(shown, req.Progress)
	}

	isPlayer := req.Beat.Kind == BeatSpeech && req.Beat.Player
	isSpeech := req.Beat.Kind == BeatSpeech
	edge := inactiveEdge
	name := "Narrator"
	nameFill := color.RGBA{68, 64, 60, 255}
	switch {
	case isPlayer:
		edge, nameFill = skyAccent, color.RGBA{2, 132, 199, 255}
	case isSpeech:
		edge, nameFill = purpleAccent, color.RGBA{147, 51, 234, 255}
	}
	if isSpeech {
		if strings.TrimSpace(req.Beat.Speaker) != "" {
			name = req.Beat.Speaker
		} else {
			name = "Unknown"
		}
	}

	fillRoundRect(img, panel, panel.Dy()/10, panelFill)
	strokeRoundRect(img, panel, panel.Dy()/10, 2, edge)
	r.drawNamePlate(img, panel, name, nameFill)
	r.drawProse(img, panel, shown, isSpeech, req.DisplayMode)

	if !req.Animate || req.Progress >= 1 {
		caretX := panel.Max.X - panel.Dy()/8
		caretY := panel.Max.Y - panel.Dy()/10
		drawDiamond(img, caretX, caretY, 5, color.RGBA{216, 180, 254, 200})
	}
}

// dialogueRect is the theatre's panel: max-w-4xl, centred, min-h 20vh, above the
// bottom edge.
func (r *Renderer) dialogueRect() image.Rectangle {
	width := int(math.Min(float64(r.width)*0.72, float64(r.height)*1.15))
	height := int(math.Max(float64(r.height)*0.20, float64(r.height)*0.22))
	left := (r.width - width) / 2
	bottom := int(float64(r.height) * 0.94)
	return image.Rect(left, bottom-height, left+width, bottom)
}

// drawNamePlate draws the panel's name chip, straddling the top edge.
func (r *Renderer) drawNamePlate(img *image.RGBA, panel image.Rectangle, name string, fill color.RGBA) {
	face := r.face(r.fonts.Sans, max(13, r.height/44))
	if face == nil {
		return
	}
	padding := r.height / 90
	textWidth := font.MeasureString(face, name).Ceil()
	height := face.Metrics().Height.Ceil() + padding
	rect := image.Rect(panel.Min.X+panel.Dy()/12, panel.Min.Y-height/2, panel.Min.X+panel.Dy()/12+textWidth+2*padding, panel.Min.Y+height/2)
	fillRoundRect(img, rect, height/4, fill)
	r.drawText(img, face, name, rect.Min.X+padding, rect.Min.Y+face.Metrics().Ascent.Ceil()+padding/2, color.RGBA{255, 255, 255, 255}, true)
}

// drawProse lays out the beat's prose inside the panel.
func (r *Renderer) drawProse(img *image.RGBA, panel image.Rectangle, text string, speech bool, mode DisplayMode) {
	body := text
	if speech {
		body = "\u201c" + text + "\u201d"
	}
	blocks := ParseProse(body, mode)
	if len(blocks) == 0 {
		return
	}

	base := r.face(r.fonts.Serif, max(16, r.height/26))
	italic := r.face(r.fonts.SerifItalic, max(16, r.height/26))
	if speech {
		base = italic
	}
	mono := r.face(r.fonts.Mono, max(14, r.height/30))
	sans := r.face(r.fonts.Sans, max(13, r.height/34))
	if base == nil {
		return
	}

	padding := panel.Dy() / 8
	left := panel.Min.X + padding
	width := panel.Dx() - 2*padding
	lineHeight := base.Metrics().Height.Ceil() + r.height/120
	y := panel.Min.Y + padding + base.Metrics().Ascent.Ceil()

	for _, block := range blocks {
		if block.Kind == BlockRule {
			draw.Draw(img, image.Rect(left, y, left+width, y+1), image.NewUniform(inactiveEdge), image.Point{}, draw.Over)
			y += lineHeight
			continue
		}
		for _, line := range wrapRuns(block.Runs, base, width) {
			x := left
			for _, run := range line {
				face, col := r.runFace(run.Style, base, italic, mono, sans), runColour(run.Style, stoneText)
				if speech && run.Style == StyleRegular {
					col = stoneBright
				}
				r.drawText(img, face, run.Text, x, y, col, run.Style == StyleBold)
				x += font.MeasureString(face, run.Text).Ceil()
			}
			y += lineHeight
			if y > panel.Max.Y-padding {
				return
			}
		}
	}
}

// runFace picks the face a run draws with.
func (r *Renderer) runFace(style RunStyle, base, italic, mono, sans font.Face) font.Face {
	switch style {
	case StyleCode:
		return mono
	case StyleDirection:
		return sans
	case StyleItalic:
		return italic
	default:
		return base
	}
}

func runColour(style RunStyle, body color.RGBA) color.RGBA {
	switch style {
	case StyleLink:
		return purpleAccent
	case StyleDirection:
		return color.RGBA{192, 132, 252, 230}
	case StyleCode:
		return stoneText
	case StyleBold:
		return stoneBright
	default:
		return body
	}
}

// drawSceneCard centres the location name in amber, the theatre's title beat.
func (r *Renderer) drawSceneCard(img *image.RGBA, panel image.Rectangle, req FrameRequest) {
	face := r.face(r.fonts.Serif, max(22, r.height/16))
	if face == nil {
		return
	}
	shown := upper(req.Beat.Text)
	if req.Animate {
		shown = upper(revealText(req.Beat.Text, req.Progress))
	}
	textWidth := font.MeasureString(face, shown).Ceil()
	x := (r.width - textWidth) / 2
	baseline := panel.Min.Y + panel.Dy()/2 + face.Metrics().Ascent.Ceil()/2
	r.drawText(img, face, shown, x, baseline, amberLabel, true)
}

// drawDiamond draws the small advance caret as a rotated square.
func drawDiamond(img *image.RGBA, cx, cy, radius int, fill color.RGBA) {
	for dy := -radius; dy <= radius; dy++ {
		span := radius - abs(dy)
		for dx := -span; dx <= span; dx++ {
			x, y := cx+dx, cy+dy
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, blendOver(img.RGBAAt(x, y), fill))
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

Add `"fmt"` to the imports of `theater.go`.

Also add the run-wrapping helper:

```go
// wrapRuns breaks a block's runs into lines that fit width, preserving styles.
func wrapRuns(runs []Run, face font.Face, width int) [][]Run {
	var lines [][]Run
	current := []Run{}
	currentWidth := 0

	flush := func() {
		if len(current) > 0 {
			lines = append(lines, current)
			current = []Run{}
			currentWidth = 0
		}
	}

	for _, run := range runs {
		words := strings.SplitAfter(run.Text, " ")
		for _, word := range words {
			if word == "" {
				continue
			}
			wordWidth := font.MeasureString(face, word).Ceil()
			if currentWidth > 0 && currentWidth+wordWidth > width {
				flush()
			}
			if len(current) > 0 && current[len(current)-1].Style == run.Style {
				current[len(current)-1].Text += word
			} else {
				current = append(current, Run{Text: word, Style: run.Style})
			}
			currentWidth += wordWidth
		}
	}
	flush()
	return lines
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/scene/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/scene/theater.go pkg/scene/theater_test.go
git commit -m "feat(scene): draw the dialogue panel and the prose grammar"
```

---

### Task 7: Animation and the pipeline flag

**Files:**
- Modify: `pkg/scene/theater.go`
- Modify: `pkg/scene/render.go`
- Test: `pkg/scene/theater_test.go`

**Interfaces:**
- Produces: `func FramesForBeat(beat Beat, fps int, animate bool) int`.

- [ ] **Step 1: Write the failing test**

```go
func TestFramesForBeatIsOneWhenStatic(t *testing.T) {
	beat := Beat{Kind: BeatNarration, Text: "A line.", Duration: 2 * time.Second}
	if got := FramesForBeat(beat, 15, false); got != 1 {
		t.Errorf("static frames = %d, want 1", got)
	}
	if got := FramesForBeat(beat, 15, true); got != 30 {
		t.Errorf("animated frames = %d, want 30", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/scene/ -run TestFramesForBeat`
Expected: FAIL with "undefined: FramesForBeat"

- [ ] **Step 3: Implement the frame count**

Add to `pkg/scene/theater.go`:

```go
// FramesForBeat is how many frames a beat occupies: one when animation is off,
// and the beat's paced frame count when it is on.
func FramesForBeat(beat Beat, fps int, animate bool) int {
	if !animate {
		return 1
	}
	return FramesFor(beat.Duration, fps)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/scene/ -run TestFramesForBeat`
Expected: PASS

- [ ] **Step 5: Remove the now-unused helpers from `render.go`**

Delete `drawCoverAlpha` if no longer referenced, and any helper the stage does
not call. Run `go vet ./pkg/scene/` and remove whatever it reports as unused
(Go does not flag unused functions, so check with `grep -rn "drawCoverAlpha\|revealText\|proceduralBackground" pkg/scene/` and delete only those with no callers).

- [ ] **Step 6: Run the package tests**

Run: `go test ./pkg/scene/ && go vet ./pkg/scene/`
Expected: PASS and clean

- [ ] **Step 7: Commit**

```bash
git add pkg/scene
git commit -m "feat(scene): make frame animation an option per beat"
```

---

### Task 8: Pure-Go clip durations

**Files:**
- Modify: `pkg/media/opus/opus.go`
- Modify: `pkg/media/opus/opus_test.go`
- Modify: `pkg/export/script.go:133-143`
- Delete: `pkg/media/probe.go`, `pkg/media/probe_test.go`

**Interfaces:**
- Produces: `func opus.Duration(data []byte) (time.Duration, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestDurationReadsTheGranulePosition(t *testing.T) {
	data, err := Encode(tone(SampleRate, 1.5), SampleRate, 1, DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Duration(data)
	if err != nil {
		t.Fatalf("Duration: %v", err)
	}
	if got < 1400*time.Millisecond || got > 1600*time.Millisecond {
		t.Errorf("Duration = %v, want about 1.5s", got)
	}
}
```

Add `"time"` to the test imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/media/opus/ -run TestDuration`
Expected: FAIL with "undefined: Duration"

- [ ] **Step 3: Implement `Duration`**

Append to `pkg/media/opus/opus.go`:

```go
// Duration reads a clip's length from the final Ogg page's granule position. It
// replaces probing the file with ffprobe, which the export no longer needs: every
// clip is an Ogg/Opus stream this package wrote, so the granule is authoritative.
func Duration(data []byte) (time.Duration, error) {
	reader, head, err := oggreader.NewWith(bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("opus: read headers: %w", err)
	}

	var lastGranule uint64
	for {
		_, page, err := reader.ParseNextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, fmt.Errorf("opus: read packet: %w", err)
		}
		lastGranule = page.GranulePosition
	}
	if lastGranule == 0 {
		return 0, errors.New("opus: no audio packets")
	}

	skip := uint64(head.PreSkip)
	if lastGranule <= skip {
		return 0, nil
	}
	samples := lastGranule - skip
	return time.Duration(float64(samples) / float64(SampleRate) * float64(time.Second)), nil
}
```

Add `"time"` to the imports of `opus.go`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/media/opus/ -run TestDuration`
Expected: PASS

- [ ] **Step 5: Use it in the export compiler**

In `pkg/export/script.go`, replace the duration loop:

```go
	// A clip whose length cannot be read contributes nothing rather than throwing
	// the beat's pacing away; the beat falls back to the reading estimate.
	var total time.Duration
	for _, clip := range clips {
		data, err := os.ReadFile(clip)
		if err != nil {
			continue
		}
		duration, err := opus.Duration(data)
		if err != nil {
			continue
		}
		total += duration
	}
	return clips, total, nil
```

Add the `opus` import:

```go
"github.com/darkliquid/localrpg/pkg/media/opus"
```

- [ ] **Step 6: Delete the ffprobe probe**

```bash
git rm pkg/media/probe.go pkg/media/probe_test.go
```

Run: `go build ./... && go vet ./...`
Expected: clean

- [ ] **Step 7: Commit**

```bash
git add pkg/media/opus pkg/export/script.go
git commit -m "feat(media): measure clips from the Opus granule, not ffprobe"
```

---

### Task 9: The Opus track

**Files:**
- Create: `pkg/media/webm/opus.go`
- Test: `pkg/media/webm/opus_test.go`

**Interfaces:**
- Produces: `type OpusPacket struct{ Data []byte; Time time.Duration; Duration time.Duration }`; `type OpusTrack struct{ Head []byte; Channels uint64; Packets []OpusPacket }`; `func NewOpusTrack(channels uint64) *OpusTrack`; `func (t *OpusTrack) AppendClip(ogg []byte) error`; `func (t *OpusTrack) AppendSilence(d time.Duration) error`; `func (t *OpusTrack) Duration() time.Duration`.

- [ ] **Step 1: Write the failing tests**

```go
package webm

import (
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/media/opus"
)

func clip(t *testing.T, seconds float64) []byte {
	t.Helper()
	pcm := make([]int16, int(float64(opus.SampleRate)*seconds))
	data, err := opus.Encode(pcm, opus.SampleRate, 1, opus.DefaultBitrate)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return data
}

func TestOpusTrackLaysClipsOnOneTimeline(t *testing.T) {
	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 1.0)); err != nil {
		t.Fatalf("AppendClip: %v", err)
	}
	if err := track.AppendClip(clip(t, 0.5)); err != nil {
		t.Fatalf("AppendClip: %v", err)
	}
	if len(track.Head) < 8 || string(track.Head[:8]) != "OpusHead" {
		t.Fatalf("CodecPrivate is not an OpusHead: %q", track.Head)
	}
	if got := track.Duration(); got < 1400*time.Millisecond || got > 1600*time.Millisecond {
		t.Errorf("Duration = %v, want about 1.5s", got)
	}

	// Timestamps must be monotonic and start at zero.
	var last time.Duration
	for _, packet := range track.Packets {
		if packet.Time < last {
			t.Fatalf("packet time went backwards: %v after %v", packet.Time, last)
		}
		last = packet.Time
	}
	if track.Packets[0].Time != 0 {
		t.Errorf("first packet at %v, want 0", track.Packets[0].Time)
	}
}

func TestOpusTrackFillsAGapWithSilence(t *testing.T) {
	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 0.2)); err != nil {
		t.Fatal(err)
	}
	before := track.Duration()
	if err := track.AppendSilence(3 * time.Second); err != nil {
		t.Fatalf("AppendSilence: %v", err)
	}
	if got := track.Duration() - before; got < 2900*time.Millisecond || got > 3100*time.Millisecond {
		t.Errorf("silence added %v, want about 3s", got)
	}
	// A silence packet is the canonical mono frame.
	found := false
	for _, packet := range track.Packets {
		if len(packet.Data) == 3 && packet.Data[0] == 0xf8 {
			found = true
		}
	}
	if !found {
		t.Error("expected a canonical silence packet in the track")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/media/webm/ -run TestOpusTrack`
Expected: FAIL with "undefined: NewOpusTrack"

- [ ] **Step 3: Implement the track**

`pkg/media/webm/opus.go`:

```go
// Package webm muxes VP8 video and Opus audio into a WebM file, entirely in Go.
package webm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pion/opus/pkg/oggreader"
)

// silenceMono is the canonical 20 ms mono Opus silence packet (RFC 6716): a
// CELT-only TOC followed by an empty frame.
var silenceMono = []byte{0xf8, 0xff, 0xfe}

// OpusPacket is one Opus packet and where it plays.
type OpusPacket struct {
	Data     []byte
	Time     time.Duration
	Duration time.Duration
}

// OpusTrack is one continuous Opus stream: the OpusHead that becomes the WebM
// track's CodecPrivate, plus every packet in presentation order.
type OpusTrack struct {
	Head     []byte
	Channels uint64
	Packets  []OpusPacket

	position time.Duration
}

// NewOpusTrack starts an empty track for a channel count.
func NewOpusTrack(channels uint64) *OpusTrack {
	return &OpusTrack{Channels: channels}
}

// AppendClip demuxes one Ogg/Opus clip and appends its packets to the timeline.
// Speech is copied, never re-encoded.
func (t *OpusTrack) AppendClip(ogg []byte) error {
	reader, head, err := oggreader.NewWith(bytes.NewReader(ogg))
	if err != nil {
		return fmt.Errorf("webm: read clip headers: %w", err)
	}
	if t.Head == nil {
		t.Head = opusHead(head, t.Channels)
	}
	if uint64(head.Channels) != t.Channels {
		return fmt.Errorf("webm: clip has %d channels, track has %d", head.Channels, t.Channels)
	}

	for {
		packet, _, err := reader.ParseNextPacket()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("webm: read clip packet: %w", err)
		}
		if bytes.HasPrefix(packet, []byte("OpusHead")) || bytes.HasPrefix(packet, []byte("OpusTags")) {
			continue
		}
		duration := packetDuration(packet)
		t.Packets = append(t.Packets, OpusPacket{
			Data:     append([]byte(nil), packet...),
			Time:     t.position,
			Duration: duration,
		})
		t.position += duration
	}
}

// AppendSilence advances the timeline by d. A short gap is left to the
// container's timecodes; a longer one is padded with silence packets so a player
// never starves its decoder across a clip-less beat.
func (t *OpusTrack) AppendSilence(d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if d < 2*time.Second {
		t.position += d
		return nil
	}
	frame := packetDuration(silenceMono)
	for filled := time.Duration(0); filled < d; filled += frame {
		t.Packets = append(t.Packets, OpusPacket{
			Data:     silenceMono,
			Time:     t.position,
			Duration: frame,
		})
		t.position += frame
	}
	return nil
}

// Duration is the length of the timeline so far.
func (t *OpusTrack) Duration() time.Duration { return t.position }

// opusHead builds the RFC 7845 identification header, which is the WebM track's
// CodecPrivate.
func opusHead(head *oggreader.OggHead, channels uint64) []byte {
	ch := byte(channels)
	if ch == 0 {
		ch = byte(head.Channels)
	}
	buf := &bytes.Buffer{}
	buf.WriteString("OpusHead")
	buf.WriteByte(1)
	buf.WriteByte(ch)
	_ = binary.Write(buf, binary.LittleEndian, head.PreSkip)
	_ = binary.Write(buf, binary.LittleEndian, uint32(head.SampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(0)) // output gain
	buf.WriteByte(0)                                      // channel mapping family
	return buf.Bytes()
}

// packetDuration reads a packet's length from its own TOC byte (RFC 6716 §3.1):
// the config field gives the frame duration and the frame-count code gives how
// many frames it carries. No decoding.
func packetDuration(packet []byte) time.Duration {
	if len(packet) == 0 {
		return 0
	}
	toc := packet[0]
	config := (toc >> 3) & 0x1F
	frames := int(toc&0x03) + 1

	var frameMs float64
	switch {
	case config < 12:
		// SILK: 10, 20, 40 or 60 ms.
		frameMs = []float64{10, 20, 40, 60}[(config>>2)&0x03]
	case config < 16:
		// Hybrid: 10 or 20 ms.
		frameMs = []float64{10, 20}[(config>>1)&0x01]
	default:
		// CELT: 2.5, 5, 10 or 20 ms.
		frameMs = []float64{2.5, 5, 10, 20}[config&0x03]
	}
	return time.Duration(frameMs*float64(frames)*float64(time.Millisecond) + 0.5)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/media/webm/ -run TestOpusTrack`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/webm
git commit -m "feat(webm): lay a campaign's clips on one Opus timeline"
```

---

### Task 10: The VP8 encoder

**Files:**
- Create: `pkg/media/webm/encoder.go`
- Test: `pkg/media/webm/encoder_test.go`
- Modify: `go.mod` (add `gen2brain/vpx`)

**Interfaces:**
- Produces: `type Encoder struct{}`; `func NewEncoder(width, height, quality int) *Encoder`; `func (e *Encoder) Encode(img *image.RGBA, keyframe bool) ([]byte, error)`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/gen2brain/vpx@latest`

- [ ] **Step 2: Write the failing test**

```go
package webm

import (
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
	if _, err := encoder.Encode(solidFrame(64, 48, color.RGBA{10, 10, 10, 255}), true); err != nil {
		t.Fatalf("keyframe: %v", err)
	}
	inter, err := encoder.Encode(solidFrame(64, 48, color.RGBA{12, 12, 12, 255}), false)
	if err != nil {
		t.Fatalf("inter frame: %v", err)
	}
	var decoder vp8.Decoder
	pic, err := decoder.DecodeFrame(inter)
	if err != nil {
		t.Fatalf("decode inter: %v", err)
	}
	if pic == nil {
		t.Fatal("inter frame decoded to nothing")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./pkg/media/webm/ -run TestEncoder`
Expected: FAIL with "undefined: NewEncoder"

- [ ] **Step 4: Implement the encoder**

`pkg/media/webm/encoder.go`:

```go
package webm

import (
	"fmt"
	"image"
	"image/color"

	"github.com/gen2brain/vpx/vp8"
)

// Encoder turns RGBA frames into a VP8 bitstream. It keeps its reference frames
// across calls, so a keyframe must precede any inter frame.
type Encoder struct {
	width   int
	height  int
	quality int
	enc     vp8.Encoder
	picture *vp8.Picture
}

// NewEncoder builds an encoder for a fixed frame size.
func NewEncoder(width, height, quality int) *Encoder {
	return &Encoder{
		width:   width,
		height:  height,
		quality: quality,
		picture: newPicture(width, height),
	}
}

// Encode writes one frame. A keyframe stands alone and refreshes every reference;
// an inter frame predicts from the frame before it and is far smaller when the
// picture barely changes.
func (e *Encoder) Encode(img *image.RGBA, keyframe bool) ([]byte, error) {
	toYUV420(img, e.picture)
	opts := vp8.EncodeOptions{Quality: e.quality, Method: 5}
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
		Y:       make([]byte, width*height),
		U:       make([]byte, uvW*uvH),
		V:       make([]byte, uvW*uvH),
		YStride: width,
		UVStride: uvW,
		Width:   width,
		Height:  height,
	}
}

// toYUV420 converts an RGBA frame to BT.601 4:2:0, the colour space WebP and WebM
// VP8 both use. Chroma is averaged over each 2×2 block.
func toYUV420(img *image.RGBA, dst *vp8.Picture) {
	bounds := img.Bounds()
	width, height := dst.Width, dst.Height
	uvW := dst.UVStride

	clamp := func(v float64) byte {
		switch {
		case v < 0:
			return 0
		case v > 255:
			return 255
		default:
			return byte(v + 0.5)
		}
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := img.RGBAAt(bounds.Min.X+min(x, bounds.Dx()-1), bounds.Min.Y+min(y, bounds.Dy()-1))
			r, g, b := float64(c.R), float64(c.G), float64(c.B)
			dst.Y[y*dst.YStride+x] = clamp(0.257*r + 0.504*g + 0.098*b + 16)
		}
	}

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			var sumR, sumG, sumB float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					sx := min(bounds.Min.X+x+dx, bounds.Min.X+bounds.Dx()-1)
					sy := min(bounds.Min.Y+y+dy, bounds.Min.Y+bounds.Dy()-1)
					c := img.RGBAAt(sx, sy)
					sumR += float64(c.R)
					sumG += float64(c.G)
					sumB += float64(c.B)
				}
			}
			r, g, b := sumR/4, sumG/4, sumB/4
			index := (y/2)*uvW + x/2
			dst.U[index] = clamp(-0.148*r - 0.291*g + 0.439*b + 128)
			dst.V[index] = clamp(0.439*r - 0.368*g - 0.071*b + 128)
		}
	}
}

var _ = color.RGBA{}
```

Remove the `color` import and the `var _` line if `color` is unused.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/media/webm/ -run TestEncoder`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/media/webm go.mod go.sum
git commit -m "feat(webm): encode frames as VP8 key and inter frames"
```

---

### Task 11: The muxer

**Files:**
- Create: `pkg/media/webm/muxer.go`
- Test: `pkg/media/webm/muxer_test.go`
- Modify: `go.mod` (add `at-wat/ebml-go`)

**Interfaces:**
- Consumes: `OpusTrack` (Task 9).
- Produces: `type Muxer struct{}`; `func NewMuxer(w io.WriteSeeker, width, height int, track *OpusTrack) (*Muxer, error)`; `func (m *Muxer) WriteVideo(data []byte, keyframe bool, t time.Duration) error`; `func (m *Muxer) Close() error`.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/at-wat/ebml-go@latest`

- [ ] **Step 2: Write the failing test**

```go
package webm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-wat/ebml-go/webm"
)

func TestMuxerWritesASeekableWebM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.webm")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	track := NewOpusTrack(1)
	if err := track.AppendClip(clip(t, 0.5)); err != nil {
		t.Fatal(err)
	}

	muxer, err := NewMuxer(file, 64, 48, track)
	if err != nil {
		t.Fatalf("NewMuxer: %v", err)
	}

	encoder := NewEncoder(64, 48, 80)
	for i := 0; i < 5; i++ {
		data, err := encoder.Encode(solidFrame(64, 48, color.RGBA{byte(20 * i), 10, 10, 255}), i == 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := muxer.WriteVideo(data, i == 0, time.Duration(i)*100*time.Millisecond); err != nil {
			t.Fatalf("WriteVideo: %v", err)
		}
	}
	if err := muxer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	read, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()

	var container webm.Container
	parser, err := webm.NewSimpleBlockReader(read, &container)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(container.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(container.Tracks))
	}
	codecs := map[string]bool{}
	for _, track := range container.Tracks {
		codecs[track.CodecID] = true
	}
	if !codecs["V_VP8"] || !codecs["A_OPUS"] {
		t.Fatalf("codec ids = %v", codecs)
	}
	if _, err := parser.Read(); err != nil {
		t.Fatalf("read a block: %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./pkg/media/webm/ -run TestMuxer`
Expected: FAIL with "undefined: NewMuxer"

- [ ] **Step 4: Implement the muxer**

`pkg/media/webm/muxer.go`:

```go
package webm

import (
	"container/heap"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkvcore"
	ebmlwebm "github.com/at-wat/ebml-go/webm"
)

// keyframeInterval is how often a video keyframe is forced, so a seek always
// lands on one and every cluster can begin with one.
const keyframeInterval = 5 * time.Second

// Muxer writes VP8 video and Opus audio into one seekable WebM file.
type Muxer struct {
	video ebmlwebm.BlockWriteCloser
	audio ebmlwebm.BlockWriteCloser

	started      bool
	lastKeyframe time.Duration
}

// NewMuxer builds the two tracks. The audio track's packets are written up front
// in timestamp order by the interleaver, so the caller only feeds video.
func NewMuxer(w io.WriteSeeker, width, height int, track *OpusTrack) (*Muxer, error) {
	if track == nil || len(track.Packets) == 0 {
		return nil, fmt.Errorf("webm: no audio to mux")
	}

	tracks := []ebmlwebm.TrackEntry{
		{
			TrackNumber: 1,
			TrackUID:    1,
			CodecID:     "V_VP8",
			TrackType:   1,
			Video:       &ebmlwebm.Video{PixelWidth: uint64(width), PixelHeight: uint64(height)},
		},
		{
			TrackNumber:  2,
			TrackUID:     2,
			CodecID:      "A_OPUS",
			CodecPrivate: track.Head,
			CodecDelay:   6500000,
			TrackType:    2,
			Audio:        &ebmlwebm.Audio{SamplingFrequency: 48000, Channels: track.Channels},
		},
	}

	writers, err := ebmlwebm.NewSimpleBlockWriter(w, tracks,
		mkvcore.WithSeekHead(true),
		mkvcore.WithCues(4096),
		mkvcore.WithMaxKeyframeInterval(1, int64(keyframeInterval/time.Millisecond)),
	)
	if err != nil {
		return nil, fmt.Errorf("webm: open block writer: %w", err)
	}

	muxer := &Muxer{video: writers[0], audio: writers[1]}
	for _, packet := range track.Packets {
		if _, err := muxer.audio.Write(false, packet.Time.Milliseconds(), packet.Data); err != nil {
			return nil, fmt.Errorf("webm: write audio packet: %w", err)
		}
	}
	return muxer, nil
}

// WriteVideo adds one encoded frame. The first frame is always a keyframe, and a
// keyframe is forced every keyframeInterval so seeking stays cheap.
func (m *Muxer) WriteVideo(data []byte, keyframe bool, t time.Duration) error {
	if !m.started {
		keyframe = true
		m.started = true
	} else if t-m.lastKeyframe >= keyframeInterval {
		keyframe = true
	}
	if keyframe {
		m.lastKeyframe = t
	}
	if _, err := m.video.Write(keyframe, t.Milliseconds(), data); err != nil {
		return fmt.Errorf("webm: write video frame: %w", err)
	}
	return nil
}

// Close flushes the trailing cluster and writes the seek index. Both writers must
// be closed; the last one closes the file.
func (m *Muxer) Close() error {
	if err := m.video.Close(); err != nil {
		return fmt.Errorf("webm: close video track: %w", err)
	}
	if err := m.audio.Close(); err != nil {
		return fmt.Errorf("webm: close audio track: %w", err)
	}
	return nil
}

var _ = ebml.Marshal
var _ = heap.Init
var _ = sort.Ints
```

Remove the unused placeholder imports (`ebml`, `container/heap`, `sort`) and
their `var _` lines.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./pkg/media/webm/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/media/webm go.mod go.sum
git commit -m "feat(webm): mux VP8 and Opus into a seekable WebM"
```

---

### Task 12: The video pipeline

**Files:**
- Modify: `pkg/export/video.go` (rewrite)
- Modify: `pkg/export/video_test.go` (rewrite)
- Test: `pkg/export/video_test.go`

**Interfaces:**
- Consumes: `scene.Renderer` (Tasks 4–7), `webm.Muxer`/`webm.Encoder`/`webm.OpusTrack` (Tasks 9–11).
- Produces: `VideoPipeline` with `SetSize`, `SetFPS`, `SetStill`, `SetQuality`, `SetDisplayMode`, `SetProgress`, `RenderVideo`.

- [ ] **Step 1: Write the failing tests**

Replace `pkg/export/video_test.go` with:

```go
package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/at-wat/ebml-go/webm"
	"github.com/darkliquid/localrpg/pkg/scene"
)

func smallScript() *scene.Script {
	return &scene.Script{
		GameID:   "campaign-01",
		GameName: "Campaign One",
		Scenes: []scene.Scene{{
			LocationID:   "alden-tavern",
			LocationName: "Alden Tavern",
			Duration:     4 * time.Second,
			Beats: []scene.Beat{
				{Kind: scene.BeatSceneCard, Text: "Alden Tavern", Duration: 2 * time.Second},
				{Kind: scene.BeatNarration, Text: "Warm light.", Duration: 2 * time.Second},
			},
		}},
		TotalDuration: 4 * time.Second,
	}
}

func TestRenderVideoWritesAPlayableWebM(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)
	pipeline.SetFPS(5)

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(context.Background(), smallScript(), out); err != nil {
		t.Fatalf("RenderVideo: %v", err)
	}

	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var container webm.Container
	if _, err := webm.NewSimpleBlockReader(file, &container); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(container.Tracks) == 0 {
		t.Fatal("the file has no tracks")
	}
}

func TestRenderVideoLeavesNothingWhenCancelled(t *testing.T) {
	pipeline := NewVideoPipeline(".")
	pipeline.SetSize(64, 48)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := filepath.Join(t.TempDir(), "replay.webm")
	if err := pipeline.RenderVideo(ctx, smallScript(), out); err == nil {
		t.Fatal("expected a cancelled render to fail")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("a cancelled render left a file: %v", err)
	}
}

func TestRenderVideoRequiresScenes(t *testing.T) {
	if err := NewVideoPipeline(".").RenderVideo(context.Background(), &scene.Script{}, filepath.Join(t.TempDir(), "x.webm")); err == nil {
		t.Fatal("expected an error for a script with no scenes")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/export/ -run TestRenderVideo`
Expected: FAIL (the old pipeline shells out to ffmpeg)

- [ ] **Step 3: Rewrite the pipeline**

Replace `pkg/export/video.go` with:

```go
package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/media/opus"
	"github.com/darkliquid/localrpg/pkg/media/webm"
	"github.com/darkliquid/localrpg/pkg/scene"
)

// defaultQuality is the VP8 quality used when a caller asks for none.
const defaultQuality = 80

// VideoPipeline renders a script to a WebM file, entirely in Go.
type VideoPipeline struct {
	rootDir     string
	width       int
	height      int
	fps         int
	quality     int
	still       bool
	displayMode scene.DisplayMode
	progress    scene.ProgressFunc
}

// NewVideoPipeline builds a renderer rooted at a campaign directory.
func NewVideoPipeline(rootDir string) *VideoPipeline {
	return &VideoPipeline{
		rootDir:     rootDir,
		width:       1920,
		height:      1080,
		fps:         scene.DefaultFPS,
		quality:     defaultQuality,
		displayMode: scene.DisplayStageDirections,
	}
}

// SetSize changes the output resolution.
func (v *VideoPipeline) SetSize(width, height int) {
	if width > 0 && height > 0 {
		v.width, v.height = width, height
	}
}

// SetFPS changes the frame rate.
func (v *VideoPipeline) SetFPS(fps int) {
	if fps > 0 {
		v.fps = fps
	}
}

// SetQuality changes the VP8 quality, 0-100.
func (v *VideoPipeline) SetQuality(quality int) {
	if quality > 0 && quality <= 100 {
		v.quality = quality
	}
}

// SetStill disables animation: one fully revealed frame per beat.
func (v *VideoPipeline) SetStill(still bool) { v.still = still }

// SetDisplayMode chooses how performance tags render.
func (v *VideoPipeline) SetDisplayMode(mode scene.DisplayMode) {
	if mode != "" {
		v.displayMode = mode
	}
}

// SetProgress routes structured progress to fn.
func (v *VideoPipeline) SetProgress(fn scene.ProgressFunc) { v.progress = fn }

// RenderVideo draws every frame, muxes the campaign's audio, and renames the
// result into place, so a failed or cancelled render leaves no file behind.
func (v *VideoPipeline) RenderVideo(ctx context.Context, script *scene.Script, outputFile string) error {
	if script == nil || len(script.Scenes) == 0 {
		return fmt.Errorf("render video: script has no scenes")
	}

	renderer, err := scene.NewRenderer(v.width, v.height)
	if err != nil {
		return fmt.Errorf("build renderer: %w", err)
	}

	track, err := v.opusTrack(script)
	if err != nil {
		return err
	}

	part := stagingPath(outputFile)
	file, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("create video: %w", err)
	}

	if err := v.writeFrames(ctx, renderer, script, file, track); err != nil {
		file.Close()
		os.Remove(part)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(part)
		return fmt.Errorf("close video: %w", err)
	}
	if err := os.Rename(part, outputFile); err != nil {
		os.Remove(part)
		return fmt.Errorf("publish video: %w", err)
	}
	if v.progress != nil {
		v.progress(scene.Progress{Phase: "done"})
	}
	return nil
}

// opusTrack lays every beat's clips on one continuous timeline.
func (v *VideoPipeline) opusTrack(script *scene.Script) (*webm.OpusTrack, error) {
	track := webm.NewOpusTrack(1)
	for _, beat := range script.Beats() {
		if len(beat.AudioPaths) == 0 {
			if err := track.AppendSilence(beat.Duration); err != nil {
				return nil, err
			}
			continue
		}
		for _, path := range beat.AudioPaths {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read clip %q: %w", path, err)
			}
			if err := track.AppendClip(data); err != nil {
				return nil, err
			}
		}
	}
	return track, nil
}

// writeFrames walks the beats, renders and encodes each frame, and feeds the
// muxer. Video and audio are interleaved by the muxer, which received the audio
// up front.
func (v *VideoPipeline) writeFrames(ctx context.Context, renderer *scene.Renderer, script *scene.Script, file *os.File, track *webm.OpusTrack) error {
	muxer, err := webm.NewMuxer(file, v.width, v.height, track)
	if err != nil {
		return err
	}
	encoder := webm.NewEncoder(v.width, v.height, v.quality)

	animate := !v.still
	elapsed := time.Duration(0)
	previousArt := ""
	first := true

	for index := range script.Scenes {
		sc := script.Scenes[index]
		for _, beat := range sc.Beats {
			if err := ctx.Err(); err != nil {
				return err
			}
			frames := scene.FramesForBeat(beat, v.fps, animate)
			for f := 0; f < frames; f++ {
				progress := 1.0
				if frames > 1 {
					progress = float64(f) / float64(frames-1)
				}
				img := renderer.Frame(scene.FrameRequest{
					Script:      script,
					SceneIndex:  index,
					Scene:       sc,
					Beat:        beat,
					Progress:    progress,
					PreviousArt: previousArt,
					Animate:     animate,
					DisplayMode: v.displayMode,
				})
				data, err := encoder.Encode(img, first)
				if err != nil {
					return err
				}
				if err := muxer.WriteVideo(data, first, elapsed); err != nil {
					return err
				}
				first = false
				if frames > 0 {
					elapsed += beat.Duration / time.Duration(frames)
				}
			}
			if v.progress != nil {
				v.progress(scene.Progress{Phase: "frames", Done: index + 1, Total: len(script.Scenes)})
			}
		}
		previousArt = sc.ArtPath
	}

	if v.progress != nil {
		v.progress(scene.Progress{Phase: "encode"})
	}
	return muxer.Close()
}

// stagingPath names the file written before it is published.
func stagingPath(outputFile string) string {
	ext := filepath.Ext(outputFile)
	if ext == "" {
		return outputFile + ".part"
	}
	return strings.TrimSuffix(outputFile, ext) + ".part" + ext
}
```

Add `"strings"` and `"time"` to the imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/export/ -run TestRenderVideo`
Expected: PASS

- [ ] **Step 5: Run the whole backend suite**

Run: `go test ./... && go vet ./...`
Expected: PASS and clean

- [ ] **Step 6: Commit**

```bash
git add pkg/export
git commit -m "feat(export): render video with the pure-Go WebM pipeline"
```

---

### Task 13: The Go surface

**Files:**
- Modify: `pkg/gui/export.go`
- Modify: `pkg/gui/server.go:1366`
- Modify: `pkg/gui/export_test.go`
- Modify: `cmd/localrpg/export.go:95-115`
- Test: `pkg/gui/export_test.go`

**Interfaces:**
- Consumes: `VideoPipeline.SetQuality` (Task 12).
- Produces: `ExportCapabilitiesDTO` without ffmpeg fields; `exportArtifactPath` returning `.webm`.

- [ ] **Step 1: Update the failing GUI test**

In `pkg/gui/export_test.go`, delete `TestStartExportVideoRequiresFFmpeg` (lines 67–74) and add:

```go
func TestExportArtifactPathNamesAWebM(t *testing.T) {
	got := exportArtifactPath("/tmp/out", "campaign-01", "video")
	if !strings.HasSuffix(got, "campaign-01.webm") {
		t.Fatalf("video artefact = %q, want a .webm", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/gui/ -run TestExportArtifactPath`
Expected: FAIL with "campaign-01.mp4"

- [ ] **Step 3: Remove the ffmpeg gate**

In `pkg/gui/export.go`:

- Delete `ErrExportNoFFmpeg`.
- Remove the `FFmpeg` and `FFmpegPath` fields from `ExportCapabilitiesDTO`.
- Replace `ExportCapabilities` with:

```go
// ExportCapabilities reports the default destination and whether a native picker
// exists.
func (s *Service) ExportCapabilities() ExportCapabilitiesDTO {
	s.mu.RLock()
	native := s.directoryPicker != nil
	s.mu.RUnlock()
	return ExportCapabilitiesDTO{
		DefaultDir:   s.defaultExportDir(),
		NativeDialog: native,
	}
}
```

- Delete the `if format == "video" { ... }` ffmpeg check in `StartExport`.
- Change `exportArtifactPath` to return `filepath.Join(outDir, gameID+".webm")` for video.

In `pkg/gui/server.go:1366`, drop `errors.Is(err, ErrExportNoFFmpeg),` from the error case.

- [ ] **Step 4: Run the GUI tests to verify they pass**

Run: `go test ./pkg/gui/ -run 'TestExport'`
Expected: PASS

- [ ] **Step 5: Update the CLI**

In `cmd/localrpg/export.go`, change the video default target and add `--quality`:

```go
	case "video":
		target := *out
		if target == "" {
			target = fmt.Sprintf("dist/%s.webm", gameID)
		}
		pipeline := export.NewVideoPipeline(*dir)
		pipeline.SetStill(*still)
		pipeline.SetFPS(*fps)
		pipeline.SetQuality(*quality)
		if cfg, cfgErr := config.NewConfigManager().Load(); cfgErr == nil && cfg != nil {
			pipeline.SetDisplayMode(scene.DisplayMode(cfg.Media.TTS.SpeechCues.DisplayMode))
		}
		if width, height, err := parseSize(*size); err == nil {
			pipeline.SetSize(width, height)
		} else {
			fmt.Fprintf(os.Stderr, "export: ignoring --size %q: %v\n", *size, err)
		}

		if err := pipeline.RenderVideo(context.Background(), script, target); err != nil {
			fmt.Fprintf(os.Stderr, "Video render failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Rendered video replay to %s\n", target)
```

Register the flag next to the existing video flags:

```go
	quality := videoCmd.Int("quality", 80, "VP8 quality, 0-100")
```

- [ ] **Step 6: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: clean

- [ ] **Step 7: Commit**

```bash
git add pkg/gui cmd/localrpg
git commit -m "feat(export): make video export available without ffmpeg"
```

---

### Task 14: The frontend and the docs

**Files:**
- Modify: `frontend/src/components/ExportModal.tsx`
- Modify: `frontend/src/types.ts:686-687`
- Modify: `README.md`
- Test: `cd frontend && npx tsc --noEmit`

- [ ] **Step 1: Remove the ffmpeg fields from the DTO type**

In `frontend/src/types.ts`, delete the `ffmpeg: boolean;` and `ffmpeg_path?: string;` lines from the export capabilities interface.

- [ ] **Step 2: Remove the gate from the modal**

In `frontend/src/components/ExportModal.tsx`:

- Delete `const ffmpegMissing = capabilities !== null && !capabilities.ffmpeg;`.
- On the video button, change `disabled={running || ffmpegMissing}` to `disabled={running}` and delete the `title={ffmpegMissing ? ... : undefined}` line.
- Delete the `{ffmpegMissing && ( ... )}` block (lines 173–177).
- Change the submit button's `disabled` to drop `(format === 'video' && ffmpegMissing)`.

- [ ] **Step 3: Type-check the frontend**

Run: `cd frontend && npx tsc --noEmit`
Expected: no errors (unused `capabilities` state must be removed if it becomes unused, since `noUnusedLocals` is on)

- [ ] **Step 4: Update the README**

In `README.md`, replace the "Requirements" sentence and the video description with:

```markdown
**Video.** Draws every frame in Go with the theatre's own stage, portraits, and
dialogue panel, then encodes VP8 video and the campaign's Opus clips into one
`.webm` with a seek index. No browser, no external binary, and no `ffmpeg` is
required. `--still` renders one fully revealed frame per beat instead of
animating, which is the fast path on a weak machine. `--quality` sets the VP8
quality (0-100).
```

Change the CLI usage line to `[--out FILE] [--still] [--fps N] [--size WxH] [--quality N] [--no-art] [--no-audio]`.

- [ ] **Step 5: Run the full test task**

Run: `mise run test`
Expected: `go test ./...` passes and `npx tsc --noEmit` passes

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/ExportModal.tsx frontend/src/types.ts README.md
git commit -m "docs(export): describe the pure-Go video export"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
| --- | --- |
| §3 Architecture (three packages) | 1–12 |
| §4.1 Layout | 4, 5, 6 |
| §4.2 Inline prose grammar | 2, 6 |
| §4.3 Fonts | 1 |
| §4.4 Animation (dual path) | 7, 12 |
| §4.5 Art loading (raster + SVG) | 3 |
| §5.1 Video track (key + inter frames) | 10, 11 |
| §5.2 Audio track (TOC durations, silence) | 9 |
| §5.3 Interleaving, clusters, cues, duration | 11 |
| §6 Pipeline, atomic write, ffprobe removal | 8, 12 |
| §7 Interfaces | 1–12 |
| §8 Fonts and licensing | 1 |
| §9 CLI and GUI surface | 13, 14 |
| §10 Testing | every task |
| §11 Risks | 1 (fallback), 10 (decoder verification), 3 (oksvg) |

**Placeholder scan:** no "TBD"/"TODO"; every code step shows real code. The one
conditional instruction (Task 1 Step 7, network) states the exact fallback rather
than leaving a gap.

**Type consistency:** `scene.Renderer`/`FrameRequest` (Task 4) are consumed by
Task 12; `webm.NewOpusTrack`/`AppendClip`/`AppendSilence` (Task 9) by Tasks 11 and
12; `webm.NewEncoder`/`Encode` (Task 10) by Task 12; `webm.NewMuxer`/`WriteVideo`/
`Close` (Task 11) by Task 12; `scene.FramesForBeat` (Task 7) by Task 12;
`opus.Duration` (Task 8) by Task 12's sibling change in `script.go`.
