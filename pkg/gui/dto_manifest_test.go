package gui

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var updateDTO = flag.Bool("update-dto", false, "rewrite testdata/dto.json from the DTO structs")

const dtoManifestPath = "testdata/dto.json"

// dtoField is one JSON field of a DTO, as the wire sees it.
type dtoField struct {
	JSON     string `json:"json"`
	Go       string `json:"go"`
	Optional bool   `json:"optional"`
}

// dtoType is one DTO as the client has to mirror it.
type dtoType struct {
	Fields []dtoField `json:"fields"`
}

// dtoManifest is the whole generated document. The Go side owns the field names
// and their optionality; the TypeScript side is compared against it.
type dtoManifest struct {
	Types map[string]dtoType `json:"types"`
}

// buildDTOManifest parses this package's own source, so the manifest cannot fall
// behind the structs it describes: a registry would be one more list to forget.
func buildDTOManifest(t *testing.T) dtoManifest {
	t.Helper()

	// Files are parsed one at a time rather than with ParseDir, which is deprecated
	// and does not consider build tags; the package's own sources are all this
	// needs, and a test file's fixtures are not part of the contract.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	fset := token.NewFileSet()
	manifest := dtoManifest{Types: map[string]dtoType{}}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		{
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok || !spec.Name.IsExported() {
					return true
				}
				st, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				if fields := dtoFields(st); len(fields) > 0 {
					manifest.Types[spec.Name.Name] = dtoType{Fields: fields}
				}
				return true
			})
		}
	}
	return manifest
}

// dtoFields reads a struct's JSON tags. A field is optional when the tag says
// omitempty or the field is a pointer, which is the two ways the encoder can leave
// it out.
func dtoFields(st *ast.StructType) []dtoField {
	var fields []dtoField
	for _, field := range st.Fields.List {
		if field.Tag == nil || len(field.Names) == 0 {
			continue
		}
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		_, isPointer := field.Type.(*ast.StarExpr)
		fields = append(fields, dtoField{
			JSON:     name,
			Go:       field.Names[0].Name,
			Optional: isPointer || strings.Contains(tag, "omitempty"),
		})
	}
	return fields
}

// encodeDTOManifest writes the manifest in a stable order, so a regeneration that
// changes nothing produces the same bytes.
func encodeDTOManifest(t *testing.T, manifest dtoManifest) []byte {
	t.Helper()

	names := make([]string, 0, len(manifest.Types))
	for name := range manifest.Types {
		names = append(names, name)
	}
	sort.Strings(names)

	ordered := struct {
		Types map[string]dtoType `json:"types"`
	}{Types: make(map[string]dtoType, len(names))}
	for _, name := range names {
		ordered.Types[name] = manifest.Types[name]
	}

	data, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	return append(data, '\n')
}

// TestDTOManifestIsCurrent keeps the checked-in mirror of the wire contract in
// step with the structs that produce it, so a renamed tag is a test failure with
// a diff rather than an undefined in the browser.
func TestDTOManifestIsCurrent(t *testing.T) {
	got := encodeDTOManifest(t, buildDTOManifest(t))

	if *updateDTO {
		if err := os.MkdirAll(filepath.Dir(dtoManifestPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dtoManifestPath, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(dtoManifestPath)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with go test ./pkg/gui -update-dto)", dtoManifestPath, err)
	}
	if string(want) != string(got) {
		t.Fatalf("dto manifest is stale; regenerate with go test ./pkg/gui -update-dto\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestDTOManifestFindsTheKnownFields fails when the walk stops working, which a
// comparison against a silently empty manifest would otherwise hide.
func TestDTOManifestFindsTheKnownFields(t *testing.T) {
	manifest := buildDTOManifest(t)
	if len(manifest.Types) < 100 {
		t.Fatalf("manifest holds %d types; the walk is not finding the DTOs", len(manifest.Types))
	}

	player, ok := manifest.Types["PlayerDTO"]
	if !ok {
		t.Fatal("PlayerDTO is not in the manifest")
	}
	var found *dtoField
	for i := range player.Fields {
		if player.Fields[i].JSON == "state" {
			found = &player.Fields[i]
		}
	}
	if found == nil {
		t.Fatal("PlayerDTO has no state field in the manifest")
	}
	if found.Go != "State" {
		t.Errorf("PlayerDTO.state comes from %q, want State", found.Go)
	}
	if found.Optional {
		t.Error("PlayerDTO.state is reported optional; the tag has no omitempty")
	}

	// A pointer field is optional whatever the tag says, and the projection site
	// depends on it: the client sends no voice for a character without one.
	for _, field := range player.Fields {
		if field.JSON == "voice" && !field.Optional {
			t.Error("PlayerDTO.voice is a pointer and must be reported optional")
		}
	}
}

// TestDTOManifestIsDeterministic covers the ordering: two walks over unchanged
// source must produce identical bytes, or every CI run would report a diff.
func TestDTOManifestIsDeterministic(t *testing.T) {
	first := encodeDTOManifest(t, buildDTOManifest(t))
	second := encodeDTOManifest(t, buildDTOManifest(t))
	if string(first) != string(second) {
		t.Fatal("two walks of the same source produced different manifests")
	}
	if !strings.HasSuffix(string(first), "}\n") {
		t.Fatalf("manifest does not end with a single newline: %q", string(first[len(first)-4:]))
	}
}
