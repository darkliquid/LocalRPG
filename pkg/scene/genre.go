package scene

import "strings"

// GenrePalette is the colour set for one genre: a gradient from the centre to the
// edge and an accent. The app, the export, and the showcase site read the same
// values, so a campaign looks the same in all three. The values are duplicated in
// frontend/src/lib/genre.ts, and a parity test over the shared fixture
// (testdata/genre-palettes.json) fails if the two drift.
type GenrePalette struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	From   string `json:"from"`
	To     string `json:"to"`
	Accent string `json:"accent"`
	Icon   string `json:"icon"`
}

// GenrePalettes is every genre this build knows, in a stable order. The neutral
// entry is the fallback and preserves the app's original look.
var GenrePalettes = []GenrePalette{
	{ID: "neutral", Label: "Neutral", From: "#261e1b", To: "#0c0a09", Accent: "#a855f7", Icon: "Dices"},
	{ID: "fantasy", Label: "Fantasy", From: "#1e1b4b", To: "#090a0f", Accent: "#a855f7", Icon: "Sword"},
	{ID: "cyberpunk", Label: "Cyberpunk", From: "#0e3b4a", To: "#050811", Accent: "#22d3ee", Icon: "Cpu"},
	{ID: "horror", Label: "Horror", From: "#3b1111", To: "#050505", Accent: "#ef4444", Icon: "Skull"},
	{ID: "scifi", Label: "Science Fiction", From: "#123055", To: "#050914", Accent: "#38bdf8", Icon: "Rocket"},
	{ID: "western", Label: "Western", From: "#4a2a12", To: "#140b05", Accent: "#f59e0b", Icon: "Flame"},
	{ID: "modern", Label: "Modern", From: "#2b3a45", To: "#0c1013", Accent: "#94a3b8", Icon: "Building2"},
	{ID: "historical", Label: "Historical", From: "#3a3220", To: "#0f0d08", Accent: "#d6b25e", Icon: "Shield"},
}

// genreAliases maps the names a world may use to a palette id, so "sci-fi",
// "space", and "cyber" resolve without a new palette.
var genreAliases = map[string]string{
	"fantasy": "fantasy", "high-fantasy": "fantasy", "dark-fantasy": "fantasy", "medieval": "fantasy",
	"cyberpunk": "cyberpunk", "cyber": "cyberpunk", "neon": "cyberpunk", "dystopian": "cyberpunk",
	"horror": "horror", "gothic": "horror", "grimdark": "horror", "lovecraftian": "horror",
	"scifi": "scifi", "sci-fi": "scifi", "space": "scifi", "science-fiction": "scifi", "space-opera": "scifi",
	"western": "western", "wildwest": "western", "wild-west": "western",
	"modern": "modern", "contemporary": "modern", "urban": "modern", "noir": "modern",
	"historical": "historical", "history": "historical", "steampunk": "historical", "victorian": "historical",
}

// GenrePaletteFor resolves a genre to its palette, case-insensitively and through
// the alias table. An unknown or absent genre is neutral, which preserves the
// app's original look.
func GenrePaletteFor(genre string) GenrePalette {
	key := strings.ToLower(strings.TrimSpace(genre))
	if key == "" {
		return GenrePalettes[0]
	}
	if id, ok := genreAliases[key]; ok {
		key = id
	}
	for _, palette := range GenrePalettes {
		if palette.ID == key {
			return palette
		}
	}
	return GenrePalettes[0]
}
