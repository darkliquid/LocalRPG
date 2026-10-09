package media

// GenerateProceduralBustSVG generates a deterministic 3/4 bust silhouette for a
// character with no generated portrait. It is the id, name, and gender only form
// of GenerateProceduralPortrait, kept for a caller that has nothing more to go
// on; a caller that has the entity's tags and state gets a portrait that reflects
// them.
func GenerateProceduralBustSVG(id, name, gender string) []byte {
	return GenerateProceduralPortrait(PortraitRequest{ID: id, Name: name, Gender: gender})
}
