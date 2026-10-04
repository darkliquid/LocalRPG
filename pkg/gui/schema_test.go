package gui

import (
	"encoding/json"
	"strings"
	"testing"
)

const entityFrontmatterSchemaPath = "schema/entity-frontmatter.json"

func TestEntityFrontmatterSchemaIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, entityFrontmatterSchemaPath, renderEntityFrontmatterSchema())
}

func TestEveryFrontmatterKeyHasADescription(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if len(schema.Keys) == 0 {
		t.Fatal("the schema lists no keys")
	}
	for _, key := range schema.Keys {
		if strings.TrimSpace(key.Description) == "" {
			t.Errorf("frontmatter key %q has no description; add one to frontmatterKeyDescriptions", key.Name)
		}
		if strings.TrimSpace(key.Type) == "" {
			t.Errorf("frontmatter key %q has no type", key.Name)
		}
	}
}

func TestFrontmatterSchemaMarksRequiredKeys(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	required := map[string]bool{}
	for _, key := range schema.Keys {
		required[key.Name] = key.Required
	}
	for _, want := range []string{"id", "name", "type"} {
		if !required[want] {
			t.Errorf("%q must be required, because it has no omitempty tag", want)
		}
	}
	if required["tags"] {
		t.Error("tags is optional and must not be marked required")
	}
	if _, ok := required["extra"]; ok {
		t.Error("the inline catch-all must not appear as a key")
	}
}

func TestFrontmatterSchemaAllowsUnknownKeys(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if !schema.AllowUnknown {
		t.Fatal("the engine carries unknown keys in ExtraMeta, so allowUnknown must be true")
	}
}

func TestEmbeddedFrontmatterSchemaParses(t *testing.T) {
	schema := EntityFrontmatterSchema()
	if len(schema.Keys) == 0 {
		t.Fatal("the embedded schema is empty; regenerate with go test ./pkg/gui -update-docs")
	}
	if !schema.AllowUnknown {
		t.Error("the embedded schema must allow unknown keys")
	}
}

func TestFrontmatterSchemaTypes(t *testing.T) {
	var schema FrontmatterSchema
	if err := json.Unmarshal([]byte(renderEntityFrontmatterSchema()), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	types := map[string]string{}
	for _, key := range schema.Keys {
		types[key.Name] = key.Type
	}
	if types["tags"] != "[]string" {
		t.Errorf("tags type = %q, want []string", types["tags"])
	}
	if types["portrait_version"] != "int" {
		t.Errorf("portrait_version type = %q, want int", types["portrait_version"])
	}
	if !strings.HasPrefix(types["state"], "map<") {
		t.Errorf("state type = %q, want a map", types["state"])
	}
}
