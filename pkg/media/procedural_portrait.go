package media

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// PortraitRequest carries what a procedural portrait is derived from. Every field
// is optional: an absent one falls back to a deterministic derivation, so a
// character always gets a portrait.
type PortraitRequest struct {
	ID     string
	Name   string
	Gender string
	Tags   []string
	State  map[string]any
}

// GenerateProceduralPortrait draws a bust for a character: a species silhouette
// and an archetype's hair, collar, and accessory from its tags, a palette from
// both, and an expression from its state. It is deterministic, so the same
// request yields the same bytes and the art cache stays correct.
func GenerateProceduralPortrait(req PortraitRequest) []byte {
	rng := rand.New(rand.NewSource(portraitSeed(req)))
	sp := speciesFor(req.Tags)
	arch := archetypeFor(req.Tags, rng)

	skin := sp.skinHue(rng)
	skinShade := shade(skin, 0.86)
	garment := arch.garmentHue(rng)
	hair := shade(garment, 0.55)
	expr := expressionFor(req)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="100%%" height="100%%">`)
	fmt.Fprintf(&b, `<defs>`+
		`<linearGradient id="bgGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%"><stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient>`+
		`<linearGradient id="skinGrad" x1="0%%" y1="0%%" x2="100%%" y2="100%%"><stop offset="0%%" stop-color="%s"/><stop offset="100%%" stop-color="%s"/></linearGradient>`+
		`</defs>`,
		shade(garment, 0.34), shade(garment, 0.16), skin, skinShade)
	fmt.Fprintf(&b, `<rect width="256" height="256" rx="32" fill="url(#bgGrad)"/>`)

	// Shoulders and torso, angled 3/4 to the right as the flat bust was.
	fmt.Fprintf(&b, `<path d="M 40 256 C 45 200, 75 170, 115 160 C 130 156, 155 156, 175 165 C 215 180, 235 210, 240 256 Z" fill="%s" opacity="0.95"/>`, garment)
	b.WriteString(drawCollar(arch.Collar, garment, skinShade))
	fmt.Fprintf(&b, `<path d="M 120 162 L 126 125 L 158 128 L 160 165 Z" fill="%s" opacity="0.95"/>`, skinShade)
	b.WriteString(drawEars(sp.EarShape, skin, skinShade))
	fmt.Fprintf(&b, `<ellipse cx="146" cy="95" rx="44" ry="54" fill="url(#skinGrad)"/>`)
	b.WriteString(drawJaw(sp.Jaw, skin, skinShade))
	b.WriteString(drawFace(sp.Brow, expr))
	b.WriteString(drawHair(arch.Hair, hair))
	b.WriteString(drawAccessory(arch.Accessory, garment))
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// PortraitRequestFor derives a portrait request from a character entity: its
// tags choose the species and archetype, and its state the expression. Only
// scalar state values are carried, because the generator reads no more.
func PortraitRequestFor(ent *entity.Entity) PortraitRequest {
	if ent == nil {
		return PortraitRequest{}
	}
	req := PortraitRequest{ID: ent.ID, Name: ent.Name, Gender: ent.Gender, Tags: ent.Tags}
	if ent.State == nil {
		return req
	}
	raw := ent.State.Raw()
	if len(raw) == 0 {
		return req
	}
	state := make(map[string]any, len(raw))
	for key, value := range raw {
		switch value.(type) {
		case int, int64, float64, string, bool:
			state[key] = value
		}
	}
	req.State = state
	return req
}

// portraitSeed hashes everything a portrait is derived from into one RNG seed.
// Tags and state are canonicalised, so neither order nor a map's iteration order
// can change the portrait.
func portraitSeed(req PortraitRequest) int64 {
	h := fnv.New64a()
	h.Write([]byte(strings.Join([]string{req.ID, req.Name, req.Gender}, "|")))

	tags := append([]string(nil), req.Tags...)
	sort.Strings(tags)
	h.Write([]byte("|" + strings.Join(tags, ",")))

	keys := make([]string, 0, len(req.State))
	for key := range req.State {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "|%s=%v", key, req.State[key])
	}
	return int64(h.Sum64())
}

// portraitHealthKeys and portraitMaxHealthKeys are the state keys a low-health
// expression is read from, so the generator works without the system schema.
var (
	portraitHealthKeys    = []string{"health", "hp", "hit_points", "hitpoints", "vitality", "vigor"}
	portraitMaxHealthKeys = []string{"health_max", "max_health", "hp_max", "max_hp", "max_vitality"}
)

// expressionFor derives a facial expression from the entity's state: an explicit
// expression or mood is used as-is, a health value at or below a third of its
// maximum reads as strained, and anything else is neutral. The read is
// best-effort, so an absent state is neutral rather than an error.
func expressionFor(req PortraitRequest) string {
	if value, ok := stateText(req.State, "expression"); ok {
		return value
	}
	if value, ok := stateText(req.State, "mood"); ok {
		return value
	}
	health, ok := stateNumber(req.State, portraitHealthKeys...)
	if !ok {
		return "neutral"
	}
	if max, ok := stateNumber(req.State, portraitMaxHealthKeys...); ok && max > 0 {
		if health*3 <= max {
			return "strained"
		}
		return "neutral"
	}
	if health <= 1 {
		return "strained"
	}
	return "neutral"
}

func stateText(state map[string]any, key string) (string, bool) {
	raw, ok := state[key]
	if !ok {
		return "", false
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(text)), true
}

func stateNumber(state map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := state[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		case string:
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &n); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// drawEars renders the species' ear silhouette at the head's sides. It is drawn
// before the head so the head covers the ear's base.
func drawEars(shape, skin, skinShade string) string {
	switch shape {
	case "pointed":
		return fmt.Sprintf(`<path d="M 110 84 L 90 44 L 120 76 Z" fill="%s"/><path d="M 182 84 L 202 44 L 172 76 Z" fill="%s"/>`, skin, skin)
	case "tufted":
		return fmt.Sprintf(`<path d="M 108 80 q -20 -28 -6 -36 q 12 12 18 26 Z" fill="%s"/><path d="M 184 80 q 20 -28 6 -36 q -12 12 -18 26 Z" fill="%s"/>`, skin, skin)
	case "plate":
		return fmt.Sprintf(`<rect x="96" y="74" width="12" height="28" rx="3" fill="%s"/><rect x="184" y="74" width="12" height="28" rx="3" fill="%s"/>`, skinShade, skinShade)
	default:
		return fmt.Sprintf(`<circle cx="106" cy="96" r="12" fill="%s"/><circle cx="186" cy="96" r="12" fill="%s"/>`, skin, skin)
	}
}

// drawJaw renders the species' jaw and chin silhouette.
func drawJaw(shape, skin, skinShade string) string {
	switch shape {
	case "heavy":
		return fmt.Sprintf(`<path d="M 110 98 Q 146 164 188 102 Q 198 82 190 72 Q 178 132 146 148 Q 116 132 106 84 Z" fill="%s"/>`, skinShade)
	case "broad":
		return fmt.Sprintf(`<path d="M 106 96 Q 146 158 192 100 Q 198 82 190 72 Q 178 128 146 144 Q 114 128 104 82 Z" fill="%s"/>`, skinShade)
	case "narrow":
		return fmt.Sprintf(`<path d="M 118 104 Q 146 144 180 106 Q 186 90 184 82 Q 170 122 146 134 Q 124 122 116 88 Z" fill="%s"/>`, skinShade)
	case "round":
		return fmt.Sprintf(`<path d="M 118 102 Q 146 140 178 104 Q 184 90 182 82 Q 168 118 146 128 Q 124 118 116 88 Z" fill="%s"/>`, skinShade)
	case "muzzle":
		return fmt.Sprintf(`<path d="M 118 104 Q 146 150 178 106 Q 186 92 182 84 Q 170 124 146 134 Q 124 124 116 90 Z" fill="%s"/><ellipse cx="146" cy="130" rx="21" ry="13" fill="%s" opacity="0.92"/>`, skinShade, skin)
	case "gaunt":
		return fmt.Sprintf(`<path d="M 120 106 Q 146 152 176 106 Q 182 94 180 88 Q 168 128 146 140 Q 126 128 118 92 Z" fill="%s"/>`, skinShade)
	case "angular":
		return fmt.Sprintf(`<path d="M 112 98 L 124 146 L 170 146 L 182 98 Z" fill="%s"/>`, skinShade)
	default:
		return fmt.Sprintf(`<path d="M 110 100 L 116 144 L 176 144 L 184 100 Z" fill="%s"/>`, skinShade)
	}
}

// drawFace renders the brow, eyes, and mouth. The expression reshapes the brow
// and the mouth, so a strained character reads differently from a calm one.
func drawFace(brow, expression string) string {
	brows := drawBrow(brow)
	eyes := `<circle cx="130" cy="92" r="5" fill="#1b1b1f"/><circle cx="166" cy="90" r="5" fill="#1b1b1f"/>`
	mouth := `<path d="M 134 122 Q 148 128 162 120" fill="none" stroke="#5a3a34" stroke-width="3" stroke-linecap="round"/>`

	switch expression {
	case "strained", "weary", "tired", "haggard":
		brows = `<path d="M 118 78 L 140 86" fill="none" stroke="#3a2a22" stroke-width="4" stroke-linecap="round"/><path d="M 178 76 L 156 86" fill="none" stroke="#3a2a22" stroke-width="4" stroke-linecap="round"/>`
		mouth = `<path d="M 134 126 Q 148 118 162 124" fill="none" stroke="#5a3a34" stroke-width="3" stroke-linecap="round"/>`
	case "angry", "furious", "hostile":
		brows = `<path d="M 116 78 L 142 90" fill="none" stroke="#3a2a22" stroke-width="5" stroke-linecap="round"/><path d="M 180 78 L 154 90" fill="none" stroke="#3a2a22" stroke-width="5" stroke-linecap="round"/>`
		mouth = `<path d="M 134 128 L 162 123" fill="none" stroke="#5a3a34" stroke-width="3" stroke-linecap="round"/>`
	case "calm", "serene", "content", "happy":
		mouth = `<path d="M 136 123 Q 148 130 160 121" fill="none" stroke="#5a3a34" stroke-width="3" stroke-linecap="round"/>`
	}
	return brows + eyes + mouth
}

// drawBrow renders the species' brow shape, which the expression may replace.
func drawBrow(shape string) string {
	switch shape {
	case "heavy":
		return `<path d="M 118 82 Q 130 76 142 82" fill="none" stroke="#3a2a22" stroke-width="5" stroke-linecap="round"/><path d="M 154 80 Q 166 74 178 80" fill="none" stroke="#3a2a22" stroke-width="5" stroke-linecap="round"/>`
	case "arched":
		return `<path d="M 118 84 Q 130 70 142 82" fill="none" stroke="#3a2a22" stroke-width="3" stroke-linecap="round"/><path d="M 154 82 Q 166 68 178 82" fill="none" stroke="#3a2a22" stroke-width="3" stroke-linecap="round"/>`
	case "hollow":
		return `<path d="M 118 80 Q 130 74 142 82" fill="none" stroke="#2a2220" stroke-width="6" stroke-linecap="round"/><path d="M 154 78 Q 166 72 178 80" fill="none" stroke="#2a2220" stroke-width="6" stroke-linecap="round"/>`
	case "ridge":
		return `<rect x="116" y="76" width="30" height="6" rx="2" fill="#4a4640"/><rect x="150" y="74" width="30" height="6" rx="2" fill="#4a4640"/>`
	default:
		return `<path d="M 118 82 Q 130 78 142 82" fill="none" stroke="#3a2a22" stroke-width="4" stroke-linecap="round"/><path d="M 154 80 Q 166 76 178 80" fill="none" stroke="#3a2a22" stroke-width="4" stroke-linecap="round"/>`
	}
}

// drawHair renders the archetype's hair over the head.
func drawHair(style, hair string) string {
	switch style {
	case "long":
		return fmt.Sprintf(`<path d="M 100 96 Q 96 40 146 36 Q 196 40 192 96 Q 196 140 186 150 Q 190 96 178 74 Q 160 60 146 62 Q 128 62 112 76 Q 102 96 106 150 Q 96 138 100 96 Z" fill="%s"/>`, hair)
	case "shaggy":
		return fmt.Sprintf(`<path d="M 102 92 Q 100 42 146 38 Q 192 42 190 92 Q 184 68 168 60 L 158 74 L 146 56 L 132 74 L 122 60 Q 108 68 102 92 Z" fill="%s"/>`, hair)
	case "tidy":
		return fmt.Sprintf(`<path d="M 104 90 Q 106 44 146 40 Q 186 44 188 90 Q 178 66 146 64 Q 114 66 104 90 Z" fill="%s"/>`, hair)
	case "tonsured":
		return fmt.Sprintf(`<circle cx="146" cy="44" r="20" fill="%s"/><path d="M 104 92 Q 110 60 146 56 Q 182 60 188 92 Q 180 72 146 70 Q 112 72 104 92 Z" fill="%s"/>`, hair, hair)
	case "braided":
		return fmt.Sprintf(`<path d="M 104 92 Q 106 44 146 40 Q 186 44 188 92 Q 178 68 146 66 Q 114 68 104 92 Z" fill="%s"/><path d="M 100 96 q -8 40 -2 66" fill="none" stroke="%s" stroke-width="7" stroke-linecap="round"/><path d="M 192 96 q 8 40 2 66" fill="none" stroke="%s" stroke-width="7" stroke-linecap="round"/>`, hair, hair, hair)
	case "styled":
		return fmt.Sprintf(`<path d="M 102 90 Q 104 40 146 38 Q 188 40 190 90 Q 182 60 146 58 Q 110 60 102 90 Z" fill="%s"/><path d="M 146 38 q 26 -6 34 12" fill="none" stroke="%s" stroke-width="6" stroke-linecap="round"/>`, hair, hair)
	case "wrapped":
		return fmt.Sprintf(`<path d="M 100 92 Q 100 46 146 42 Q 192 46 192 92 Q 186 66 146 64 Q 106 66 100 92 Z" fill="%s"/><rect x="98" y="62" width="96" height="12" rx="4" fill="%s" opacity="0.85"/>`, hair, shade(hair, 0.8))
	case "hooded":
		return fmt.Sprintf(`<path d="M 92 128 Q 88 36 146 32 Q 204 36 200 128 Q 196 70 146 62 Q 96 70 92 128 Z" fill="%s" opacity="0.95"/>`, shade(hair, 0.8))
	default:
		return fmt.Sprintf(`<path d="M 106 86 Q 108 46 146 42 Q 184 46 186 86 Q 176 64 146 62 Q 116 64 106 86 Z" fill="%s"/>`, hair)
	}
}

// drawCollar renders the archetype's collar at the shoulders.
func drawCollar(style, garment, skinShade string) string {
	switch style {
	case "high":
		return fmt.Sprintf(`<path d="M 118 164 L 122 128 L 146 152 L 170 128 L 174 164 Z" fill="%s"/>`, lighten(garment, 0.2))
	case "hooded":
		return fmt.Sprintf(`<path d="M 110 172 Q 120 150 146 158 Q 172 150 182 172 Q 164 162 146 162 Q 128 162 110 172 Z" fill="%s"/>`, shade(garment, 0.7))
	case "clerical":
		return fmt.Sprintf(`<rect x="132" y="140" width="28" height="24" rx="3" fill="%s"/>`, lighten(garment, 0.35))
	case "flat":
		return fmt.Sprintf(`<path d="M 120 166 L 146 150 L 172 166 L 172 172 L 120 172 Z" fill="%s"/>`, lighten(garment, 0.25))
	case "cloak":
		return fmt.Sprintf(`<path d="M 104 176 Q 124 152 146 160 Q 168 152 188 176 Q 166 168 146 168 Q 126 168 104 176 Z" fill="%s"/>`, shade(garment, 0.65))
	case "ruff":
		return fmt.Sprintf(`<ellipse cx="146" cy="166" rx="34" ry="12" fill="%s"/>`, lighten(garment, 0.4))
	case "gorget":
		return fmt.Sprintf(`<path d="M 122 164 Q 146 178 170 164 L 170 174 Q 146 188 122 174 Z" fill="%s"/>`, lighten(garment, 0.15))
	case "open":
		return fmt.Sprintf(`<path d="M 122 166 L 146 156 L 170 166 L 166 178 L 146 170 L 126 178 Z" fill="%s"/>`, shade(garment, 0.8))
	default:
		return fmt.Sprintf(`<path d="M 122 166 Q 146 176 170 166 L 170 174 Q 146 184 122 174 Z" fill="%s"/>`, skinShade)
	}
}

// drawAccessory renders the one accessory an archetype carries, at the lower
// right so it reads at portrait size.
func drawAccessory(kind, garment string) string {
	switch kind {
	case "sword":
		return fmt.Sprintf(`<path d="M 208 196 L 236 92 L 242 96 L 216 200 Z" fill="#cbd5e1"/><rect x="198" y="192" width="26" height="7" rx="2" fill="%s" transform="rotate(20 211 195)"/>`, lighten(garment, 0.3))
	case "staff":
		return fmt.Sprintf(`<rect x="206" y="120" width="7" height="136" rx="3" fill="#6b4f3a"/><circle cx="209" cy="112" r="13" fill="%s" opacity="0.9"/>`, lighten(garment, 0.45))
	case "dagger":
		return fmt.Sprintf(`<path d="M 214 198 L 236 148 L 241 151 L 220 202 Z" fill="#d6d3d1"/><rect x="206" y="196" width="20" height="6" rx="2" fill="%s" transform="rotate(20 216 199)"/>`, shade(garment, 0.7))
	case "tome":
		return fmt.Sprintf(`<rect x="196" y="186" width="52" height="38" rx="4" fill="%s"/><rect x="200" y="190" width="44" height="30" rx="2" fill="#f5f5f4"/>`, lighten(garment, 0.2))
	case "circlet":
		return fmt.Sprintf(`<path d="M 112 52 Q 146 34 180 52" fill="none" stroke="%s" stroke-width="5" stroke-linecap="round"/><circle cx="146" cy="40" r="6" fill="%s"/>`, lighten(garment, 0.4), lighten(garment, 0.55))
	case "bow":
		return fmt.Sprintf(`<path d="M 220 84 Q 244 148 220 212" fill="none" stroke="%s" stroke-width="6"/><path d="M 220 84 L 220 212" stroke="#d6d3d1" stroke-width="2"/>`, shade(garment, 0.6))
	case "signet":
		return fmt.Sprintf(`<circle cx="196" cy="182" r="10" fill="%s"/><circle cx="196" cy="182" r="5" fill="%s"/>`, lighten(garment, 0.5), lighten(garment, 0.75))
	case "satchel":
		return fmt.Sprintf(`<rect x="186" y="188" width="46" height="40" rx="6" fill="%s"/><path d="M 186 196 L 146 168" stroke="%s" stroke-width="7"/>`, shade(garment, 0.75), shade(garment, 0.9))
	default:
		return fmt.Sprintf(`<circle cx="200" cy="190" r="9" fill="%s"/>`, lighten(garment, 0.35))
	}
}
