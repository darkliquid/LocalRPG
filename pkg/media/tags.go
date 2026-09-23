package media

import "strings"

// tagSynonyms maps the ways providers spell a trait onto one vocabulary, so the
// matcher scores an authored archetype and a catalog voice identically.
var tagSynonyms = map[string]string{
	"masculine":        "male",
	"feminine":         "female",
	"nonbinary":        "androgynous",
	"non-binary":       "androgynous",
	"american english": "american",
	"british english":  "british",
	"uk":               "british",
	"us":               "american",
	"united states":    "american",
	"united kingdom":   "british",
	"middle aged":      "middle-aged",
	"middleaged":       "middle-aged",
	"older":            "elder",
	"senior":           "elder",
	"youthful":         "young",
}

// NormaliseVoiceTags lowercases, trims, expands common synonyms, and drops
// empties, so one matcher scores authored and catalog voices without knowing
// where a profile came from.
func NormaliseVoiceTags(raw ...string) []string {
	seen := make(map[string]bool)
	tags := make([]string, 0, len(raw))
	for _, tag := range raw {
		normalised := normaliseTag(tag)
		if normalised == "" || seen[normalised] {
			continue
		}
		seen[normalised] = true
		tags = append(tags, normalised)
	}
	return tags
}

func normaliseTag(tag string) string {
	cleaned := strings.Join(strings.Fields(strings.ToLower(tag)), " ")
	if cleaned == "" {
		return ""
	}
	if synonym, ok := tagSynonyms[cleaned]; ok {
		return synonym
	}
	return strings.ReplaceAll(cleaned, " ", "-")
}
