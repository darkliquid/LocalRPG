package systemtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxFunctionLines is the budget set by RD-11 for non-test functions.
const maxFunctionLines = 80

// exemptFunctions names non-test functions permitted to exceed the budget,
// mapped to the architectural rationale why each remains over eighty lines.
// When an exempt function is refactored below eighty lines, its entry must be
// removed so exemptions do not rot.
var exemptFunctions = map[string]string{
	"pkg/engine/orchestrator.go:runGenerationLoop": "Waiting on the pkg/engine batch of RD-11 (finding R-12): the generation loop's boundaries are a decision about provider rounds and tool calls, so it is worked down beside them rather than here.",
}

// TestFunctionLengthGate enforces that every non-test function in the repository
// stays within the eighty-line budget, unless explicitly exempt with a recorded reason.
func TestFunctionLengthGate(t *testing.T) {
	root := findRepoRoot(t)

	type longFunc struct {
		lines int
		path  string
		name  string
		line  int
	}

	var violations []longFunc
	foundExempt := make(map[string]bool)

	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "frontend", "website", "styles", "dist", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)

		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			startLine := fset.Position(fd.Pos()).Line
			endLine := fset.Position(fd.End()).Line
			lines := endLine - startLine + 1
			if lines > maxFunctionLines {
				key := fmt.Sprintf("%s:%s", relSlash, fd.Name.Name)
				if _, ok := exemptFunctions[key]; ok {
					foundExempt[key] = true
				} else {
					violations = append(violations, longFunc{
						lines: lines,
						path:  relSlash,
						name:  fd.Name.Name,
						line:  startLine,
					})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for _, v := range violations {
		t.Errorf("%s:%d %s has %d lines (budget: %d). Break it into named phases or document an exemption.",
			v.path, v.line, v.name, v.lines, maxFunctionLines)
	}

	for key := range exemptFunctions {
		if !foundExempt[key] {
			t.Errorf("exemption %q is no longer over %d lines; remove it from exemptFunctions in function_length_test.go", key, maxFunctionLines)
		}
	}
}

func TestFunctionLengthScannerFlagsLongFunction(t *testing.T) {
	src := "package dummy\n\nfunc longFunction() {\n" + strings.Repeat("\tprintln(\"line\")\n", 85) + "}\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "dummy.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var lines int
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			return true
		}
		lines = fset.Position(fd.End()).Line - fset.Position(fd.Pos()).Line + 1
		return true
	})

	if lines <= maxFunctionLines {
		t.Fatalf("expected dummy function to exceed %d lines, got %d", maxFunctionLines, lines)
	}
}

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
