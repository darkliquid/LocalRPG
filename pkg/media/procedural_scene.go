package media

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"
)

// GenerateSceneSVG composes a scene from structured hints. It is deterministic:
// the seed and the hints fully determine the bytes, so the art cache key stays
// correct.
func GenerateSceneSVG(req SceneRequest) []byte {
	rng := rand.New(rand.NewSource(seedFor(req)))
	p := paletteFor(req.Genre, req.Mood, req.TimeOfDay, rng)
	st := structureFor(strings.Fields(req.Prompt), req.Genre, rng)
	const w, h = 800, 600

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, w, h, w, h)
	writeSky(&b, p, req.TimeOfDay, rng, w, h)
	b.WriteString(st(p, rng, w, h))
	writeWeather(&b, req.Weather, p, rng, w, h)
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// seedFor hashes the seed and hints into one RNG seed, so identical requests
// produce identical art and a changed hint or seed changes it.
func seedFor(req SceneRequest) int64 {
	h := fnv.New64a()
	h.Write([]byte(strings.Join([]string{req.Seed, req.Genre, req.Mood, req.TimeOfDay, req.Weather}, "|")))
	return int64(h.Sum64())
}

func writeSky(b *strings.Builder, p palette, timeOfDay string, rng *rand.Rand, w, h int) {
	fmt.Fprintf(b, `<defs>`+
		`<linearGradient id="skyGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient>`+
		`<linearGradient id="groundGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient>`+
		`<radialGradient id="celestialGrad" cx="50%%" cy="50%%" r="50%%"><stop offset="0%%" stop-color="%s" stop-opacity="0.8"/><stop offset="100%%" stop-color="%s" stop-opacity="0"/></radialGradient>`+
		`</defs>`,
		p.SkyTop, p.SkyBottom, p.Ground, shade(p.Ground, 0.7), p.Celestial, p.Celestial)
	fmt.Fprintf(b, `<rect width="%d" height="%d" fill="url(#skyGrad)"/>`, w, h)

	cx, cy := celestialPosition(timeOfDay, w, h)
	fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="140" fill="url(#celestialGrad)"/>`, cx, cy)
	fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="45" fill="%s" opacity="0.85"/>`, cx, cy, p.Celestial)

	if isNight(timeOfDay) {
		writeStars(b, p, rng, w, h)
	} else {
		writeParticles(b, p, rng, w, h)
	}
}

func writeStars(b *strings.Builder, p palette, rng *rand.Rand, w, h int) {
	for i := 0; i < 40; i++ {
		cx := rng.Intn(w)
		cy := rng.Intn(h * 3 / 5)
		r := rng.Float64()*1.2 + 0.4
		opacity := rng.Float64()*0.7 + 0.3
		fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="%.2f"/>`, cx, cy, r, p.Celestial, opacity)
	}
}

func writeParticles(b *strings.Builder, p palette, rng *rand.Rand, w, h int) {
	for i := 0; i < 24; i++ {
		cx := rng.Intn(w)
		cy := rng.Intn(h * 3 / 5)
		r := rng.Float64()*1.5 + 0.5
		opacity := rng.Float64()*0.5 + 0.2
		fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="%.1f" fill="%s" opacity="%.2f"/>`, cx, cy, r, p.AccentA, opacity)
	}
}

// writeWeather draws an overlay for rain, fog, or snow; clear draws nothing.
func writeWeather(b *strings.Builder, weather string, p palette, rng *rand.Rand, w, h int) {
	switch strings.ToLower(strings.TrimSpace(weather)) {
	case "rain", "storm", "downpour":
		for i := 0; i < 90; i++ {
			x := rng.Intn(w)
			y := rng.Intn(h)
			fmt.Fprintf(b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="1" opacity="0.35"/>`, x, y, x-6, y+18, p.Fog)
		}
	case "fog", "mist", "haze":
		for i := 0; i < 5; i++ {
			y := h/5 + i*(h/6)
			fmt.Fprintf(b, `<rect x="0" y="%d" width="%d" height="%d" fill="%s" opacity="0.18"/>`, y, w, h/12, p.Fog)
		}
	case "snow", "sleet", "blizzard":
		for i := 0; i < 120; i++ {
			cx := rng.Intn(w)
			cy := rng.Intn(h)
			r := rng.Float64()*2 + 0.8
			fmt.Fprintf(b, `<circle cx="%d" cy="%d" r="%.1f" fill="#ffffff" opacity="0.7"/>`, cx, cy, r)
		}
	}
}

func isNight(timeOfDay string) bool {
	return strings.ToLower(strings.TrimSpace(timeOfDay)) == "night"
}

// celestialPosition places the sun or moon by time of day, so dawn and dusk read
// differently from noon.
func celestialPosition(timeOfDay string, w, h int) (int, int) {
	switch strings.ToLower(strings.TrimSpace(timeOfDay)) {
	case "dawn":
		return w / 5, h / 3
	case "dusk":
		return w * 4 / 5, h / 3
	case "day":
		return w / 2, h / 5
	case "night":
		return w * 3 / 4, h / 6
	}
	return w * 3 / 4, h / 4
}

// GenerateScene implements SceneHintProvider for the built-in generator.
func (p *proceduralArtClient) GenerateScene(_ context.Context, req SceneRequest) ([]byte, error) {
	return GenerateSceneSVG(req), nil
}

// sceneRequestFromPrompt derives hints from prose, so GenerateImage keeps working
// unchanged while still gaining the richer generator.
func sceneRequestFromPrompt(prompt string) SceneRequest {
	return SceneRequest{
		Prompt:    prompt,
		Genre:     genreFromText(prompt),
		Mood:      moodFromText(prompt),
		TimeOfDay: timeFromText(prompt),
		Weather:   weatherFromText(prompt),
		Seed:      prompt,
	}
}

func containsAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func genreFromText(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case containsAny(lower, "cyber", "neon", "hack", "chrome", "netrunner"):
		return "cyberpunk"
	case containsAny(lower, "starship", "space", "nebula", "orbital", "station"):
		return "scifi"
	case containsAny(lower, "cowboy", "saloon", "frontier", "desert", "gunslinger"):
		return "wildwest"
	case containsAny(lower, "blood", "crypt", "catacomb", "dungeon", "ruin", "horror", "undead"):
		return "horror"
	case containsAny(lower, "steam", "cog", "brass", "airship"):
		return "steampunk"
	case containsAny(lower, "street", "apartment", "office", "city"):
		return "modern"
	}
	return "fantasy"
}

func moodFromText(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case containsAny(lower, "grim", "dark", "ominous", "dread", "doom", "blood"):
		return "grim"
	case containsAny(lower, "serene", "calm", "peaceful", "sunlit", "bright"):
		return "serene"
	}
	return ""
}

func timeFromText(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case containsAny(lower, "dawn", "sunrise", "morning"):
		return "dawn"
	case containsAny(lower, "dusk", "sunset", "evening"):
		return "dusk"
	case containsAny(lower, "night", "midnight", "moon", "stars"):
		return "night"
	case containsAny(lower, "day", "noon", "sunlit"):
		return "day"
	}
	return ""
}

func weatherFromText(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case containsAny(lower, "rain", "storm", "downpour"):
		return "rain"
	case containsAny(lower, "fog", "mist", "haze"):
		return "fog"
	case containsAny(lower, "snow", "blizzard", "frost"):
		return "snow"
	}
	return ""
}
