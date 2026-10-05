package gui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEntityTypeCatalogKeysResolve(t *testing.T) {
	catalog := buildEntityTypeCatalog()
	if len(catalog.Types) == 0 {
		t.Fatal("the catalogue lists no types")
	}
	for _, spec := range catalog.Types {
		if spec.ID == "" || spec.Label == "" || spec.Description == "" {
			t.Errorf("type %q is missing an id, label or description", spec.ID)
		}
		if len(spec.Keys) == 0 {
			t.Errorf("type %q scaffolds no keys", spec.ID)
		}
		seen := map[string]bool{}
		for _, key := range spec.Keys {
			if strings.TrimSpace(key.Description) == "" {
				t.Errorf("type %q key %q has no description", spec.ID, key.Name)
			}
			if strings.TrimSpace(key.Type) == "" {
				t.Errorf("type %q key %q has no type", spec.ID, key.Name)
			}
			if seen[key.Name] {
				t.Errorf("type %q lists key %q twice", spec.ID, key.Name)
			}
			seen[key.Name] = true
		}
		for _, required := range []string{"id", "name", "type", "state"} {
			if !seen[required] {
				t.Errorf("type %q does not scaffold the %q key", spec.ID, required)
			}
		}
	}
}

func TestEntityTypeCatalogTypeIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range buildEntityTypeCatalog().Types {
		if seen[spec.ID] {
			t.Errorf("type %q is listed twice", spec.ID)
		}
		seen[spec.ID] = true
	}
}

func TestEntityTypeCatalogTypeValuesMatchTypes(t *testing.T) {
	catalog := buildEntityTypeCatalog()
	ids := make([]string, 0, len(catalog.Types))
	for _, spec := range catalog.Types {
		ids = append(ids, spec.ID)
	}
	got := frontmatterKeyValues["type"]
	if strings.Join(got, ",") != strings.Join(ids, ",") {
		t.Errorf("frontmatterKeyValues[type] = %v, want the catalogue ids %v", got, ids)
	}
}

func TestEntityTypeCatalogVoiceAndAppearance(t *testing.T) {
	byID := map[string]EntityTypeSpec{}
	for _, spec := range buildEntityTypeCatalog().Types {
		byID[spec.ID] = spec
	}
	has := func(id, key string) bool {
		for _, candidate := range byID[id].Keys {
			if candidate.Name == key {
				return true
			}
		}
		return false
	}
	if !has("character", "voice") {
		t.Error("character must offer the voice key")
	}
	for _, id := range []string{"character", "location", "faction", "item"} {
		if !has(id, "appearance") {
			t.Errorf("%s must offer the appearance key", id)
		}
	}
	if has("location", "voice") {
		t.Error("location must not offer the voice key")
	}
}

func TestEntityTypeCatalogMarshals(t *testing.T) {
	encoded, err := json.Marshal(buildEntityTypeCatalog())
	if err != nil {
		t.Fatalf("marshal catalogue: %v", err)
	}
	var decoded EntityTypeCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal catalogue: %v", err)
	}
	if len(decoded.BaseKeys) == 0 || len(decoded.Types) == 0 {
		t.Fatal("the round-tripped catalogue is empty")
	}
}
