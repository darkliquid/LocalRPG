package entity

import "testing"

func TestValidateMemory(t *testing.T) {
	ok := &Memory{Turn: 3, Kind: MemoryEvent, EntityRefs: []string{"kae"}, Text: "Crossed the bridge.", Importance: 3}
	if err := ValidateMemory(ok); err != nil {
		t.Fatalf("valid memory rejected: %v", err)
	}
	empty := &Memory{Turn: 3, Kind: MemoryEvent, Text: "x", Importance: 3}
	if err := ValidateMemory(empty); err == nil {
		t.Fatal("memory with no entity refs should be invalid")
	}
	noText := &Memory{Turn: 3, Kind: MemoryEvent, EntityRefs: []string{"kae"}, Importance: 3}
	if err := ValidateMemory(noText); err == nil {
		t.Fatal("memory with no text should be invalid")
	}
	badImportance := &Memory{Turn: 3, Kind: MemoryEvent, EntityRefs: []string{"kae"}, Text: "x", Importance: 9}
	if err := ValidateMemory(badImportance); err == nil {
		t.Fatal("memory with importance 9 should be invalid")
	}
}
