package entity

// Mention kinds describe how an entity was involved in a turn.
const (
	MentionPlayer    = "player"
	MentionLocation  = "location"
	MentionWikilink  = "wikilink"
	MentionExtracted = "extracted"
	MentionSpeech    = "speech"
)

// Mention records how one entity was involved in a turn.
type Mention struct {
	ID   string `json:"id"`
	Kind string `json:"mention"`
}
