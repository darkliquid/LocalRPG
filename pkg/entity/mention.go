package entity

// Mention kinds describe how an entity was involved in a turn.
const (
	MentionPlayer    = "player"
	MentionLocation  = "location"
	MentionWikilink  = "wikilink"
	MentionExtracted = "extracted"
	MentionSpeech    = "speech"
	// MentionProse records a character whose name the turn's prose contains, found
	// by a deterministic scan rather than by a link, a spoken line, or a model.
	MentionProse = "prose"
)

// Mention records how one entity was involved in a turn.
type Mention struct {
	ID   string `json:"id"`
	Kind string `json:"mention"`
}
