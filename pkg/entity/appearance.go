package entity

import "strings"

// appearanceSeparator introduces the current appearance inside a composed
// description. The baseline comes first so a note reads as itself, and the
// current comes last, where a model weights it as the qualifier.
const appearanceSeparator = "; now "

// DescribeAppearance composes a note's baseline appearance with the current one
// for its circumstances. It is the single rule for every consumer: an image
// prompt, an art cache key, and a sheet all show what this returns, so something
// authored and something play wrote can never be read apart. A limit of zero or
// less returns the whole text.
func DescribeAppearance(ent *Entity, limit int) string {
	if ent == nil {
		return ""
	}
	baseline := strings.TrimSpace(ent.Appearance)
	current := strings.TrimSpace(ent.CurrentAppearance)

	composed := baseline
	if current != "" {
		if baseline == "" {
			// Nothing authored a baseline, so there is no context to layer the
			// current one onto and it stands alone.
			composed = current
		} else {
			composed = baseline + appearanceSeparator + current
		}
	}
	if limit > 0 && len(composed) > limit {
		composed = strings.TrimSpace(composed[:limit])
	}
	return composed
}
