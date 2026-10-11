package entity

import "strings"

// appearanceSeparator introduces the current appearance inside a composed
// description. The baseline comes first so a note reads as itself, and the
// current comes last, where a model weights it as the qualifier.
const appearanceSeparator = "; now "

// LayerAppearance joins a baseline appearance with the current one for a note's
// circumstances. It is the shape every consumer reads, whether it composes a whole
// description or bounds each part itself. A current appearance with no baseline
// stands alone, because there is no context to layer it onto.
func LayerAppearance(baseline, current string) string {
	baseline = strings.TrimSpace(baseline)
	current = strings.TrimSpace(current)

	switch {
	case current == "":
		return baseline
	case baseline == "":
		return current
	default:
		return baseline + appearanceSeparator + current
	}
}

// DescribeAppearance composes a note's baseline appearance with the current one,
// bounded by limit. A limit of zero or less returns the whole text. It is the
// single rule for every consumer: an image prompt, an art cache key, and a sheet
// all show what this returns, so something authored and something play wrote can
// never be read apart.
func DescribeAppearance(ent *Entity, limit int) string {
	if ent == nil {
		return ""
	}
	composed := LayerAppearance(ent.Appearance, ent.CurrentAppearance)
	if limit > 0 && len(composed) > limit {
		composed = strings.TrimSpace(composed[:limit])
	}
	return composed
}
