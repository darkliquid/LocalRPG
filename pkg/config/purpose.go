package config

// Purpose names a role a media provider plays.
type Purpose string

// The purposes a media provider can be assigned to: who narrates, who speaks for
// characters, who draws a scene and a portrait, and what stands in while an image is
// being made.
const (
	PurposeNarrator    Purpose = "narrator"
	PurposeNPC         Purpose = "npc"
	PurposeScene       Purpose = "scene"
	PurposePortrait    Purpose = "portrait"
	PurposePlaceholder Purpose = "placeholder"
)

// PurposeFamily reports which media family a purpose belongs to: "tts" or
// "image". An unknown purpose returns "".
func PurposeFamily(p Purpose) string {
	switch p {
	case PurposeNarrator, PurposeNPC:
		return "tts"
	case PurposeScene, PurposePortrait, PurposePlaceholder:
		return "image"
	default:
		return ""
	}
}

// KnownPurpose reports whether p is a purpose this build understands.
func KnownPurpose(p string) bool { return PurposeFamily(Purpose(p)) != "" }
