package storage

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

// canonicalEntityFields defines the canonical snake_case names of the 18 frontmatter fields
// that must be maintained in sync across EntityFrontmatter, ParseMarkdownEntity,
// SerializeMarkdown, SaveEntity, and GetEntity/unmarshalEntityMeta.
var canonicalEntityFields = []string{
	"age",
	"aliases",
	"appearance",
	"current_appearance",
	"extra",
	"faction",
	"gender",
	"history",
	"id",
	"location",
	"name",
	"portrait",
	"portrait_history",
	"portrait_version",
	"state",
	"tags",
	"type",
	"voice",
}

// TestEntityFieldDivergenceAST statically verifies via AST inspection that the
// eighteen entity frontmatter fields are enumerated identically across:
// 1. EntityFrontmatter struct declaration (pkg/entity/entity.go)
// 2. ParseMarkdownEntity construction (pkg/entity/entity.go)
// 3. SerializeMarkdown construction & exclusion switch (pkg/entity/entity.go)
// 4. SaveEntity index write (pkg/storage/store.go)
// 5. GetEntity & unmarshalEntityMeta index read (pkg/storage/store.go)
func TestEntityFieldDivergenceAST(t *testing.T) {
	root := findRepoRoot(t)
	entityPath := filepath.Join(root, "pkg", "entity", "entity.go")
	storePath := filepath.Join(root, "pkg", "storage", "store.go")

	fset := token.NewFileSet()
	entityFile, err := parser.ParseFile(fset, entityPath, nil, 0)
	if err != nil {
		t.Fatalf("parse entity.go: %v", err)
	}
	storeFile, err := parser.ParseFile(fset, storePath, nil, 0)
	if err != nil {
		t.Fatalf("parse store.go: %v", err)
	}

	fmFields := extractFrontmatterStructFields(t, entityFile)
	parseFields := extractParseMarkdownFields(t, entityFile)
	serFields, serExclusions := extractSerializeMarkdownFields(t, entityFile)
	saveFields := extractSaveEntityFields(t, storeFile)
	readFields := extractGetEntityFields(t, storeFile)

	wantCanonical := sortedCopy(canonicalEntityFields)

	assertFieldSet(t, "EntityFrontmatter struct", fmFields, wantCanonical)
	assertFieldSet(t, "ParseMarkdownEntity", parseFields, wantCanonical)
	assertFieldSet(t, "SerializeMarkdown assignments", serFields, wantCanonical)

	// In SerializeMarkdown, ExtraMeta is the catch-all container, so all
	// canonical fields except "extra" itself must be excluded from ExtraMeta.
	wantExclusions := filterOut(wantCanonical, "extra")
	assertFieldSet(t, "SerializeMarkdown ExtraMeta exclusions", serExclusions, wantExclusions)

	assertFieldSet(t, "SaveEntity (store write)", saveFields, wantCanonical)
	assertFieldSet(t, "GetEntity / unmarshalEntityMeta (store read)", readFields, wantCanonical)
}

// TestEntityAllFieldsRoundTripMarkdown verifies that an Entity with every frontmatter
// field populated round-trips losslessly through SerializeMarkdown -> ParseMarkdownEntity.
func TestEntityAllFieldsRoundTripMarkdown(t *testing.T) {
	ent := makeFullyPopulatedEntity(t)

	data, err := ent.SerializeMarkdown()
	if err != nil {
		t.Fatalf("SerializeMarkdown: %v", err)
	}

	reparsed, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		t.Fatalf("ParseMarkdownEntity: %v", err)
	}

	assertEntityFrontmatterEqual(t, "Markdown round-trip", ent, reparsed)

	// Verify standard fields did not leak into ExtraMeta
	for _, field := range canonicalEntityFields {
		if field == "extra" {
			continue
		}
		if _, leaked := reparsed.ExtraMeta[field]; leaked {
			t.Errorf("field %q leaked into ExtraMeta after markdown round-trip", field)
		}
	}
}

// TestEntityAllFieldsRoundTripStorage verifies that an Entity with every frontmatter
// field populated round-trips losslessly through SaveEntity -> GetEntity.
func TestEntityAllFieldsRoundTripStorage(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ent := makeFullyPopulatedEntity(t)

	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	loaded, err := store.GetEntity(ent.ID)
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}

	assertEntityFrontmatterEqual(t, "Storage round-trip", ent, loaded)

	// Also verify non-frontmatter storage fields
	if loaded.Body != ent.Body {
		t.Errorf("Body = %q, want %q", loaded.Body, ent.Body)
	}
	if loaded.Hash != ent.Hash {
		t.Errorf("Hash = %q, want %q", loaded.Hash, ent.Hash)
	}
	if loaded.Folder != ent.Folder {
		t.Errorf("Folder = %q, want %q", loaded.Folder, ent.Folder)
	}
}

// TestFixturePopulatesAllFields verifies that makeFullyPopulatedEntity does not leave
// any field in entity.Entity or entity.EntityFrontmatter at its zero value.
func TestFixturePopulatesAllFields(t *testing.T) {
	ent := makeFullyPopulatedEntity(t)
	val := reflect.ValueOf(ent).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		if isZero(fieldVal) {
			t.Errorf("makeFullyPopulatedEntity leaves Entity.%s as zero value; must populate all fields", field.Name)
		}
	}

	fmVal := reflect.ValueOf(entity.EntityFrontmatter{
		ID:                ent.ID,
		Name:              ent.Name,
		Type:              ent.Type,
		Tags:              ent.Tags,
		Voice:             ent.Voice,
		Portrait:          ent.Portrait,
		PortraitVersion:   ent.PortraitVersion,
		PortraitHistory:   ent.PortraitHistory,
		Location:          ent.Location,
		Appearance:        ent.Appearance,
		CurrentAppearance: ent.CurrentAppearance,
		Gender:            ent.Gender,
		Age:               ent.Age,
		Aliases:           ent.Aliases,
		Faction:           ent.Faction,
		History:           ent.History,
		State:             ent.State.Raw(),
		ExtraMeta:         ent.ExtraMeta,
	})
	fmTyp := fmVal.Type()
	for i := 0; i < fmTyp.NumField(); i++ {
		field := fmTyp.Field(i)
		fieldVal := fmVal.Field(i)
		if isZero(fieldVal) {
			t.Errorf("EntityFrontmatter.%s is zero in test fixture; must populate all fields", field.Name)
		}
	}
}

// makeFullyPopulatedEntity creates an Entity with distinct non-zero values for every field.
func makeFullyPopulatedEntity(t *testing.T) *entity.Entity {
	t.Helper()
	ent := &entity.Entity{
		ID:   "val-the-brave",
		Name: "Val the Brave",
		Type: "character",
		Tags: []string{"paladin", "champion"},
		Voice: &entity.VoiceConfig{
			Provider:   "kokoro",
			VoiceID:    "af_sarah",
			Pitch:      1.15,
			SpeechRate: 0.95,
			Options:    map[string]any{"volume": "high"},
		},
		Portrait:          "portraits/val.png",
		PortraitVersion:   4,
		PortraitHistory:   []string{"portraits/val_v1.png", "portraits/val_v2.png"},
		Location:          "high-sanctuary",
		Faction:           "order-of-light",
		Appearance:        "A tall warrior with burnished silver plate armor.",
		CurrentAppearance: "Silver armor dented on the left pauldron.",
		Gender:            "non-binary",
		Age:               "32",
		Aliases:           []string{"Val", "The Silver Shield"},
		History:           []int{1, 3, 7},
		ExtraMeta:         map[string]any{"custom_title": "Knight Protector"},
		Body:              "Val swore an oath to protect the sanctuary.",
		Wikilinks:         []string{"high-sanctuary", "order-of-light"},
		Hash:              "abc123hash",
		Folder:            "characters",
	}
	ent.InitState(map[string]any{
		"heroic": true,
		"title":  "Champion",
	})
	return ent
}

func assertEntityFrontmatterEqual(t *testing.T, context string, want, got *entity.Entity) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("[%s] ID = %q, want %q", context, got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("[%s] Name = %q, want %q", context, got.Name, want.Name)
	}
	if got.Type != want.Type {
		t.Errorf("[%s] Type = %q, want %q", context, got.Type, want.Type)
	}
	if !reflect.DeepEqual(got.Tags, want.Tags) {
		t.Errorf("[%s] Tags = %v, want %v", context, got.Tags, want.Tags)
	}
	if !reflect.DeepEqual(got.Voice, want.Voice) {
		t.Errorf("[%s] Voice = %+v, want %+v", context, got.Voice, want.Voice)
	}
	if got.Portrait != want.Portrait {
		t.Errorf("[%s] Portrait = %q, want %q", context, got.Portrait, want.Portrait)
	}
	if got.PortraitVersion != want.PortraitVersion {
		t.Errorf("[%s] PortraitVersion = %d, want %d", context, got.PortraitVersion, want.PortraitVersion)
	}
	if !reflect.DeepEqual(got.PortraitHistory, want.PortraitHistory) {
		t.Errorf("[%s] PortraitHistory = %v, want %v", context, got.PortraitHistory, want.PortraitHistory)
	}
	if got.Location != want.Location {
		t.Errorf("[%s] Location = %q, want %q", context, got.Location, want.Location)
	}
	if got.Faction != want.Faction {
		t.Errorf("[%s] Faction = %q, want %q", context, got.Faction, want.Faction)
	}
	if got.Appearance != want.Appearance {
		t.Errorf("[%s] Appearance = %q, want %q", context, got.Appearance, want.Appearance)
	}
	if got.CurrentAppearance != want.CurrentAppearance {
		t.Errorf("[%s] CurrentAppearance = %q, want %q", context, got.CurrentAppearance, want.CurrentAppearance)
	}
	if got.Gender != want.Gender {
		t.Errorf("[%s] Gender = %q, want %q", context, got.Gender, want.Gender)
	}
	if got.Age != want.Age {
		t.Errorf("[%s] Age = %q, want %q", context, got.Age, want.Age)
	}
	if !reflect.DeepEqual(got.Aliases, want.Aliases) {
		t.Errorf("[%s] Aliases = %v, want %v", context, got.Aliases, want.Aliases)
	}
	if !reflect.DeepEqual(got.History, want.History) {
		t.Errorf("[%s] History = %v, want %v", context, got.History, want.History)
	}

	wantState := map[string]any(nil)
	if want.State != nil {
		wantState = want.State.Raw()
	}
	gotState := map[string]any(nil)
	if got.State != nil {
		gotState = got.State.Raw()
	}
	if !reflect.DeepEqual(gotState, wantState) {
		t.Errorf("[%s] State = %v, want %v", context, gotState, wantState)
	}

	if !reflect.DeepEqual(got.ExtraMeta, want.ExtraMeta) {
		t.Errorf("[%s] ExtraMeta = %v, want %v", context, got.ExtraMeta, want.ExtraMeta)
	}
}

func extractFrontmatterStructFields(t *testing.T, file *ast.File) []string {
	t.Helper()
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "EntityFrontmatter" {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			tag := ""
			if f.Tag != nil {
				tag = reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("yaml")
			}
			if tag == ",inline" {
				fields = append(fields, "extra")
				continue
			}
			name := strings.TrimSpace(strings.Split(tag, ",")[0])
			if name != "" && name != "-" {
				fields = append(fields, name)
			}
		}
		return false
	})
	if len(fields) == 0 {
		t.Fatal("found no fields in EntityFrontmatter struct")
	}
	return fields
}

func extractParseMarkdownFields(t *testing.T, file *ast.File) []string {
	t.Helper()
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "ParseMarkdownEntity" {
			return true
		}
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			cl, ok := inner.(*ast.CompositeLit)
			if !ok {
				return true
			}
			ident, ok := cl.Type.(*ast.Ident)
			if !ok || ident.Name != "Entity" {
				return true
			}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				kIdent, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				canonical := entityFieldToCanonical(kIdent.Name)
				if canonical != "" {
					fields = append(fields, canonical)
				}
			}
			return false
		})
		return false
	})
	if len(fields) == 0 {
		t.Fatal("found no fields in ParseMarkdownEntity construction")
	}
	return fields
}

func extractSerializeMarkdownFields(t *testing.T, file *ast.File) ([]string, []string) {
	t.Helper()
	var assignedFields []string
	var exclusionFields []string

	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "SerializeMarkdown" {
			return true
		}
		// 1. Literal assignments
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			cl, ok := inner.(*ast.CompositeLit)
			if !ok {
				return true
			}
			ident, ok := cl.Type.(*ast.Ident)
			if !ok || ident.Name != "EntityFrontmatter" {
				return true
			}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				kIdent, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				canonical := entityFieldToCanonical(kIdent.Name)
				if canonical != "" {
					assignedFields = append(assignedFields, canonical)
				}
			}
			return true
		})

		// 2. Body assignments: fm.ExtraMeta and fm.State
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				xIdent, ok := sel.X.(*ast.Ident)
				if !ok || xIdent.Name != "fm" {
					continue
				}
				canonical := entityFieldToCanonical(sel.Sel.Name)
				if canonical != "" {
					assignedFields = append(assignedFields, canonical)
				}
			}
			return true
		})

		// 3. Switch exclusions
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			sw, ok := inner.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			for _, stmt := range sw.Body.List {
				cc, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range cc.List {
					lit, ok := expr.(*ast.BasicLit)
					if ok && lit.Kind == token.STRING {
						val := strings.Trim(lit.Value, `"`)
						exclusionFields = append(exclusionFields, val)
					}
				}
			}
			return true
		})
		return false
	})

	if len(assignedFields) == 0 {
		t.Fatal("found no assigned fields in SerializeMarkdown")
	}
	if len(exclusionFields) == 0 {
		t.Fatal("found no exclusion fields in SerializeMarkdown")
	}
	return assignedFields, exclusionFields
}

func extractSaveEntityFields(t *testing.T, file *ast.File) []string {
	t.Helper()
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "SaveEntity" {
			return true
		}
		// SQL columns for entity
		fields = append(fields, "id", "name", "type")

		// fmMeta map keys
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			cl, ok := inner.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				lit, ok := kv.Key.(*ast.BasicLit)
				if ok && lit.Kind == token.STRING {
					fields = append(fields, strings.Trim(lit.Value, `"`))
				}
			}
			return true
		})

		// fmMeta["state"] assignment
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			assign, ok := inner.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				idx, ok := lhs.(*ast.IndexExpr)
				if !ok {
					continue
				}
				xIdent, ok := idx.X.(*ast.Ident)
				if !ok || xIdent.Name != "fmMeta" {
					continue
				}
				lit, ok := idx.Index.(*ast.BasicLit)
				if ok && lit.Kind == token.STRING {
					fields = append(fields, strings.Trim(lit.Value, `"`))
				}
			}
			return true
		})
		return false
	})
	if len(fields) == 0 {
		t.Fatal("found no fields in SaveEntity")
	}
	return fields
}

func extractGetEntityFields(t *testing.T, file *ast.File) []string {
	t.Helper()
	var fields []string
	// SQL columns in GetEntity
	fields = append(fields, "id", "name", "type")

	// Lookups in unmarshalEntityMeta
	ast.Inspect(file, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "unmarshalEntityMeta" {
			return true
		}
		ast.Inspect(fd.Body, func(inner ast.Node) bool {
			idx, ok := inner.(*ast.IndexExpr)
			if !ok {
				return true
			}
			xIdent, ok := idx.X.(*ast.Ident)
			if !ok || xIdent.Name != "meta" {
				return true
			}
			lit, ok := idx.Index.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING {
				fields = append(fields, strings.Trim(lit.Value, `"`))
			}
			return true
		})
		return false
	})
	if len(fields) == 0 {
		t.Fatal("found no fields in unmarshalEntityMeta")
	}
	return fields
}

func entityFieldToCanonical(fieldName string) string {
	switch fieldName {
	case "ID":
		return "id"
	case "Name":
		return "name"
	case "Type":
		return "type"
	case "Tags":
		return "tags"
	case "Voice":
		return "voice"
	case "Portrait":
		return "portrait"
	case "PortraitVersion":
		return "portrait_version"
	case "PortraitHistory":
		return "portrait_history"
	case "Location":
		return "location"
	case "Faction":
		return "faction"
	case "Appearance":
		return "appearance"
	case "CurrentAppearance":
		return "current_appearance"
	case "Gender":
		return "gender"
	case "Age":
		return "age"
	case "Aliases":
		return "aliases"
	case "History":
		return "history"
	case "State":
		return "state"
	case "ExtraMeta":
		return "extra"
	default:
		// Body, Wikilinks, Hash, Folder are not frontmatter fields
		return ""
	}
}

func assertFieldSet(t *testing.T, siteName string, actual, expected []string) {
	t.Helper()
	for _, diff := range diffFieldSets(siteName, actual, expected) {
		t.Error(diff)
	}
}

func diffFieldSets(siteName string, actual, expected []string) []string {
	actualSet := make(map[string]bool)
	for _, f := range actual {
		actualSet[f] = true
	}
	expectedSet := make(map[string]bool)
	for _, f := range expected {
		expectedSet[f] = true
	}

	var diffs []string
	for f := range expectedSet {
		if !actualSet[f] {
			diffs = append(diffs, fmt.Sprintf("[%s] missing entity field %q", siteName, f))
		}
	}
	for f := range actualSet {
		if !expectedSet[f] {
			diffs = append(diffs, fmt.Sprintf("[%s] unexpected entity field %q", siteName, f))
		}
	}
	sort.Strings(diffs)
	return diffs
}

// TestEntityFieldDivergenceScannerDetectsMismatch asserts that the field-set diffing
// logic properly flags missing and unexpected fields when sets diverge.
func TestEntityFieldDivergenceScannerDetectsMismatch(t *testing.T) {
	actual := []string{"id", "name", "type", "rogue_field"}
	expected := []string{"id", "name", "type", "missing_field"}

	diffs := diffFieldSets("test_site", actual, expected)
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d: %v", len(diffs), diffs)
	}
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, `missing entity field "missing_field"`) {
		t.Errorf("expected missing field error, got %v", diffs)
	}
	if !strings.Contains(joined, `unexpected entity field "rogue_field"`) {
		t.Errorf("expected unexpected field error, got %v", diffs)
	}
}

func sortedCopy(s []string) []string {
	cp := make([]string, len(s))
	copy(cp, s)
	sort.Strings(cp)
	return cp
}

func filterOut(s []string, exclude string) []string {
	var res []string
	for _, item := range s {
		if item != exclude {
			res = append(res, item)
		}
	}
	return res
}

func isZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}
