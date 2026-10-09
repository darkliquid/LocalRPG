package media

import (
	"fmt"
	"math/rand"
	"strings"
)

// structure draws one scene's foreground composition, relative to the horizon.
type structure func(p palette, rng *rand.Rand, w, h int) string

// structureTags maps a location tag or prompt word to a structure name.
var structureTags = map[string]string{
	"forest": "forest", "wood": "forest", "woods": "forest", "jungle": "forest", "grove": "forest",
	"city": "city", "town": "city", "street": "city", "fortress": "city", "citadel": "city", "tower": "city",
	"coast": "coast", "shore": "coast", "sea": "coast", "harbour": "coast", "harbor": "coast", "beach": "coast",
	"interior": "interior", "room": "interior", "hall": "interior", "chamber": "interior", "tavern": "interior",
	"dungeon": "dungeon", "crypt": "dungeon", "catacomb": "dungeon", "catacombs": "dungeon", "cellar": "dungeon",
	"ruin": "ruins", "ruins": "ruins", "wreck": "ruins",
	"mountain": "ridge", "ridge": "ridge", "cliff": "ridge", "peak": "ridge", "valley": "ridge",
	"sky": "sky", "field": "sky", "plain": "sky", "meadow": "sky", "desert": "sky",
}

// genreStructures is the structure a genre falls back to when no tag matches.
var genreStructures = map[string]string{
	"fantasy":   "ridge",
	"cyberpunk": "city",
	"horror":    "dungeon",
	"scifi":     "city",
	"wildwest":  "ridge",
	"modern":    "city",
	"noir":      "city",
	"steampunk": "city",
}

// structureOrder is the deterministic fallback order for a seeded choice.
var structureOrder = []string{"ridge", "forest", "city", "coast", "interior", "dungeon", "ruins", "sky"}

// structures maps a structure name to its drawing function.
var structures = map[string]structure{
	"ridge":    drawRidge,
	"forest":   drawForest,
	"city":     drawCity,
	"coast":    drawCoast,
	"interior": drawInterior,
	"dungeon":  drawDungeon,
	"ruins":    drawRuins,
	"sky":      drawSky,
}

// structureFor picks a structure from the tags, then the genre default, then a
// seeded choice, so it never returns nil.
func structureFor(tags []string, genre string, rng *rand.Rand) structure {
	for _, tag := range tags {
		if name, ok := structureTags[strings.ToLower(strings.TrimSpace(tag))]; ok {
			return structures[name]
		}
	}
	if name, ok := ActiveTables().SceneStructures[strings.ToLower(strings.TrimSpace(genre))]; ok {
		return structures[name]
	}
	return structures[structureOrder[rng.Intn(len(structureOrder))]]
}

func drawRidge(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 3 / 4
	var b strings.Builder
	fmt.Fprintf(&b, `<path d="M 0 %d L %d %d L %d %d L %d %d L %d %d L %d %d L %d %d L %d %d Z" fill="%s"/>`,
		horizon,
		w/8, horizon-120,
		w*3/8, horizon-40,
		w/2, horizon-160,
		w*5/8, horizon-50,
		w*3/4, horizon-110,
		w, horizon-30,
		w, h,
		p.Ground)
	fmt.Fprintf(&b, `<path d="M 0 %d Q %d %d %d %d T %d %d L %d %d L 0 %d Z" fill="%s" opacity="0.75"/>`,
		horizon+10, w/4, horizon-30, w/2, horizon+5, w, horizon+20, w, h, h, p.Ridge)
	return b.String()
}

func drawForest(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 3 / 4
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, horizon, w, h-horizon, p.Ground)
	for i := 0; i < 14; i++ {
		cx := rng.Intn(w)
		th := 60 + rng.Intn(90)
		tw := 18 + rng.Intn(16)
		top := horizon - th
		fmt.Fprintf(&b, `<path d="M %d %d L %d %d L %d %d Z" fill="%s"/>`,
			cx, top, cx-tw, horizon, cx+tw, horizon, p.AccentA)
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="4" height="%d" fill="%s"/>`, cx-2, horizon, 18, p.Ridge)
	}
	return b.String()
}

func drawCity(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 3 / 4
	var b strings.Builder
	x := 0
	for x < w {
		bw := 40 + rng.Intn(60)
		bh := 80 + rng.Intn(220)
		top := horizon - bh
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x, top, bw, bh, p.Ridge)
		for wy := top + 12; wy < horizon-10; wy += 20 {
			for wx := x + 8; wx < x+bw-8; wx += 16 {
				if rng.Intn(3) == 0 {
					fmt.Fprintf(&b, `<rect x="%d" y="%d" width="6" height="8" fill="%s" opacity="0.8"/>`, wx, wy, p.Celestial)
				}
			}
		}
		x += bw + 6
	}
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, horizon, w, h-horizon, p.Ground)
	return b.String()
}

func drawCoast(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 3 / 4
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, horizon, w, h-horizon, p.AccentB)
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, h*85/100, w, h-h*85/100, shade(p.AccentB, 0.6))
	for i := 0; i < 6; i++ {
		y := horizon + 14 + i*18
		fmt.Fprintf(&b, `<path d="M 0 %d Q %d %d %d %d T %d %d" fill="none" stroke="%s" stroke-width="2" opacity="0.5"/>`,
			y, w/4, y-6, w/2, y, w, y-4, p.Fog)
	}
	fmt.Fprintf(&b, `<path d="M 0 %d Q %d %d %d %d T %d %d L %d %d L 0 %d Z" fill="%s" opacity="0.85"/>`,
		horizon-30, w/3, horizon-90, w*2/3, horizon-10, w, horizon-40, w, horizon, h, p.Ridge)
	return b.String()
}

func drawInterior(p palette, rng *rand.Rand, w, h int) string {
	floor := h * 3 / 5
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="%s" opacity="0.9"/>`, w, floor, p.Ridge)
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, floor, w, h-floor, p.Ground)
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s" opacity="0.5"/>`, w/2-90, floor-220, 180, 220, p.Celestial)
	for i := 0; i < 4; i++ {
		x := 80 + i*(w-160)/3
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="18" height="%d" fill="%s"/>`, x, floor-260, 260, p.Ridge)
	}
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="4" fill="%s" opacity="0.4"/>`, floor, w, p.Fog)
	return b.String()
}

func drawDungeon(p palette, rng *rand.Rand, w, h int) string {
	floor := h * 3 / 4
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="%s"/>`, w, floor, p.Ground)
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, floor, w, h-floor, shade(p.Ground, 0.7))
	for y := 40; y < floor-40; y += 44 {
		off := 0
		if (y/44)%2 == 0 {
			off = 40
		}
		for x := -off; x < w; x += 80 {
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="76" height="40" fill="none" stroke="%s" stroke-width="2" opacity="0.35"/>`, x, y, p.Ridge)
		}
	}
	fmt.Fprintf(&b, `<path d="M %d %d L %d %d L %d %d L %d %d L %d %d L %d %d Z" fill="%s"/>`,
		w/2-70, floor, w/2-70, floor-160, w/2-40, floor-190,
		w/2+40, floor-190, w/2+70, floor-160, w/2+70, floor, shade(p.Ground, 0.55))
	fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="6" fill="%s" opacity="0.8"/>`, w/2+120, floor-120, p.AccentA)
	return b.String()
}

func drawRuins(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 3 / 4
	var b strings.Builder
	fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s"/>`, horizon, w, h-horizon, p.Ground)
	for i := 0; i < 6; i++ {
		x := 60 + i*(w-120)/5
		ch := 120 + rng.Intn(160)
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="26" height="%d" fill="%s"/>`, x, horizon-ch, ch, p.Ridge)
		if rng.Intn(2) == 0 {
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="40" height="16" fill="%s"/>`, x-7, horizon-ch, p.Ridge)
		}
	}
	fmt.Fprintf(&b, `<path d="M 0 %d Q %d %d %d %d T %d %d L %d %d L 0 %d Z" fill="%s" opacity="0.6"/>`,
		horizon, w/3, horizon-60, w*2/3, horizon, w, horizon-10, w, h, h, p.Ridge)
	return b.String()
}

func drawSky(p palette, rng *rand.Rand, w, h int) string {
	horizon := h * 4 / 5
	var b strings.Builder
	fmt.Fprintf(&b, `<path d="M 0 %d Q %d %d %d %d T %d %d L %d %d L 0 %d Z" fill="%s" opacity="0.7"/>`,
		horizon-40, w/4, horizon-100, w/2, horizon-30, w, horizon-70, w, h, h, p.Ridge)
	for i := 0; i < 4; i++ {
		cx := rng.Intn(w)
		cy := 60 + rng.Intn(200)
		fmt.Fprintf(&b, `<ellipse cx="%d" cy="%d" rx="%d" ry="%d" fill="%s" opacity="0.25"/>`,
			cx, cy, 60+rng.Intn(50), 18+rng.Intn(12), p.Fog)
	}
	for i := 0; i < 5; i++ {
		bx := rng.Intn(w)
		by := 40 + rng.Intn(160)
		fmt.Fprintf(&b, `<path d="M %d %d q 8 -6 16 0 q 8 -6 16 0" fill="none" stroke="%s" stroke-width="2" opacity="0.6"/>`,
			bx, by, p.AccentB)
	}
	return b.String()
}
