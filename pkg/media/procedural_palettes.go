package media

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// palette is the colour scheme of one scene. Every field is a hex string, so a
// palette is comparable and a test can assert equality.
type palette struct {
	SkyTop    string
	SkyBottom string
	Ground    string
	Ridge     string
	Fog       string
	Celestial string
	AccentA   string
	AccentB   string
}

// basePalettes maps a genre to its base palette. A mood desaturates or warms it,
// and the time of day shifts the sky.
var basePalettes = map[string]palette{
	"fantasy":   {SkyTop: "#1a2740", SkyBottom: "#3d5a80", Ground: "#20262e", Ridge: "#2b3440", Fog: "#9fb3c8", Celestial: "#ffe8a3", AccentA: "#7fb069", AccentB: "#8d99ae"},
	"cyberpunk": {SkyTop: "#0b0f1a", SkyBottom: "#2b1b4d", Ground: "#12121c", Ridge: "#1c1c2b", Fog: "#7a5cff", Celestial: "#ff4fd8", AccentA: "#00e5ff", AccentB: "#ff2e88"},
	"horror":    {SkyTop: "#0a0a0a", SkyBottom: "#1b1b1b", Ground: "#101010", Ridge: "#181818", Fog: "#4a4a4a", Celestial: "#c9c9c9", AccentA: "#6b1f1f", AccentB: "#3a3a3a"},
	"scifi":     {SkyTop: "#050914", SkyBottom: "#123055", Ground: "#0d1520", Ridge: "#16283c", Fog: "#7fd4ff", Celestial: "#d6f0ff", AccentA: "#39a0ed", AccentB: "#9fd8ff"},
	"wildwest":  {SkyTop: "#3a2a1a", SkyBottom: "#c98a4b", Ground: "#2a1f14", Ridge: "#4a3524", Fog: "#d9b382", Celestial: "#ffd9a0", AccentA: "#a34a28", AccentB: "#7a5a3a"},
	"modern":    {SkyTop: "#26343f", SkyBottom: "#6d8a9c", Ground: "#20252b", Ridge: "#2c333b", Fog: "#b9c6cf", Celestial: "#fff4d6", AccentA: "#4f8a8b", AccentB: "#8a8f98"},
	"noir":      {SkyTop: "#0e1013", SkyBottom: "#2a2e33", Ground: "#14171a", Ridge: "#1d2126", Fog: "#5c626a", Celestial: "#e6e6e6", AccentA: "#8a1c1c", AccentB: "#4a4f55"},
	"steampunk": {SkyTop: "#2a1f18", SkyBottom: "#8a5a2b", Ground: "#241a12", Ridge: "#3a2a1c", Fog: "#c8a06a", Celestial: "#ffd28a", AccentA: "#c8782a", AccentB: "#6a4a2a"},
}

// paletteGenres is the sorted list of known genres, so an unknown genre can pick
// one deterministically from the seed.
var paletteGenres = func() []string {
	genres := make([]string, 0, len(basePalettes))
	for genre := range basePalettes {
		genres = append(genres, genre)
	}
	sort.Strings(genres)
	return genres
}()

// paletteFor resolves a palette from the hints, defaulting gracefully. An
// unknown genre picks a base palette deterministically from the seed, so two
// scenes of one unknown genre still differ while a single scene stays stable.
func paletteFor(genre, mood, timeOfDay string, rng *rand.Rand) palette {
	p, ok := basePalettes[strings.ToLower(strings.TrimSpace(genre))]
	if !ok {
		if rng != nil {
			p = basePalettes[paletteGenres[rng.Intn(len(paletteGenres))]]
		} else {
			p = basePalettes["fantasy"]
		}
	}
	p = applyMood(p, mood)
	p = applyTimeOfDay(p, timeOfDay)
	return p
}

// applyMood darkens a grim mood and warms a serene one.
func applyMood(p palette, mood string) palette {
	switch strings.ToLower(strings.TrimSpace(mood)) {
	case "grim", "tense", "dark", "ominous", "dread", "dire":
		p.SkyTop = shade(p.SkyTop, 0.65)
		p.SkyBottom = shade(p.SkyBottom, 0.65)
		p.Ground = shade(p.Ground, 0.85)
		p.Ridge = shade(p.Ridge, 0.85)
		p.AccentA = desaturate(p.AccentA, 0.35)
		p.AccentB = desaturate(p.AccentB, 0.35)
	case "serene", "calm", "peaceful", "hopeful", "bright", "warm":
		p.SkyTop = lighten(p.SkyTop, 0.18)
		p.SkyBottom = lighten(p.SkyBottom, 0.18)
		p.AccentA = blend(p.AccentA, "#ffb347", 0.25)
	}
	return p
}

// applyTimeOfDay shifts the sky and dims the celestial body.
func applyTimeOfDay(p palette, timeOfDay string) palette {
	switch strings.ToLower(strings.TrimSpace(timeOfDay)) {
	case "dawn":
		p.SkyTop = blend(p.SkyTop, "#ff9e6d", 0.35)
		p.SkyBottom = blend(p.SkyBottom, "#ffd0a0", 0.35)
		p.Celestial = "#ffd9a0"
	case "day":
		p.SkyTop = lighten(p.SkyTop, 0.22)
		p.SkyBottom = lighten(p.SkyBottom, 0.22)
		p.Celestial = "#fff6d0"
	case "dusk":
		p.SkyTop = blend(p.SkyTop, "#7a3b6a", 0.35)
		p.SkyBottom = blend(p.SkyBottom, "#e0703a", 0.35)
		p.Celestial = "#ffb37a"
	case "night":
		p.SkyTop = shade(p.SkyTop, 0.5)
		p.SkyBottom = shade(p.SkyBottom, 0.5)
		p.Ground = shade(p.Ground, 0.8)
		p.Celestial = shade(p.Celestial, 0.7)
	}
	return p
}

func parseHex(s string) (int, int, int) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0
	}
	var r, g, b int
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0
	}
	return r, g, b
}

func clampChannel(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func hexOf(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", clampChannel(r), clampChannel(g), clampChannel(b))
}

// blend mixes two colours: t=0 is a, t=1 is b.
func blend(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	return hexOf(
		int(float64(ar)*(1-t)+float64(br)*t),
		int(float64(ag)*(1-t)+float64(bg)*t),
		int(float64(ab)*(1-t)+float64(bb)*t),
	)
}

func shade(hex string, factor float64) string {
	r, g, b := parseHex(hex)
	return hexOf(int(float64(r)*factor), int(float64(g)*factor), int(float64(b)*factor))
}

func lighten(hex string, factor float64) string {
	r, g, b := parseHex(hex)
	return hexOf(
		r+int(float64(255-r)*factor),
		g+int(float64(255-g)*factor),
		b+int(float64(255-b)*factor),
	)
}

func desaturate(hex string, amount float64) string {
	r, g, b := parseHex(hex)
	grey := int(0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b))
	return blend(hex, hexOf(grey, grey, grey), amount)
}
