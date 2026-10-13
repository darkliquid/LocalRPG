package gui

// Splitting handleGameRoutes into a dispatcher and one file per resource is a
// promise about where the route bodies live, and the compiler is equally happy
// with all of them back in one function. This test is the guard, and it is
// deliberately a table rather than a rule, because which resource a route
// belongs to is a judgement and the judgement should be legible. Adding an
// action without adding it here, or bodying a case instead of calling out to it,
// fails this until the table moves with it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// gameRouteHandlers is the handler each action under /api/game/ dispatches to.
// banner and icon share one handler because they are the same route with a
// different file, which is why the table maps action to method and not the other
// way round.
var gameRouteHandlers = map[string]string{
	"advance":        "handleGameAdvanceRoute",
	"audio":          "handleGameAudioRoute",
	"banner":         "handleGameAssetRoute",
	"character":      "handleGameCharacterRoute",
	"chronicle":      "handleGameChronicleRoute",
	"context":        "handleGameContextRoute",
	"entities":       "handleGameEntitiesRoute",
	"entity":         "handleGameEntityRoute",
	"findings":       "handleGameFindingsRoute",
	"folders":        "handleGameFoldersRoute",
	"generate-asset": "handleGameGenerateAssetRoute",
	"graph":          "handleGameGraphRoute",
	"icon":           "handleGameAssetRoute",
	"location":       "handleGameLocationRoute",
	"recap":          "handleGameRecapRoute",
	"restart":        "handleGameRestartRoute",
	"settings":       "handleGameSettingsRoute",
	"state":          "handleGameStateRoute",
	"tts":            "handleGameTTSRoute",
	"turn":           "handleGameTurnRoute",
	"usage":          "handleGameUsageRoute",
	"working-set":    "handleGameWorkingSetRoute",
}

// TestGameRouteDispatchIsComplete checks that the dispatcher and the table agree
// in both directions, so neither can gain an action alone.
func TestGameRouteDispatchIsComplete(t *testing.T) {
	dispatched := dispatchActions(t, "routes_game.go")

	for action, want := range gameRouteHandlers {
		got, ok := dispatched[action]
		if !ok {
			t.Errorf("action %q is in the table but not dispatched", action)
			continue
		}
		if got != want {
			t.Errorf("action %q dispatches to %s, the table says %s", action, got, want)
		}
	}
	for action, got := range dispatched {
		if _, ok := gameRouteHandlers[action]; !ok {
			t.Errorf("action %q dispatches to %s but is not in the table", action, got)
		}
	}
}

// TestGameRouteHandlersExist checks that every method the table names is
// declared on the server, so a rename cannot leave the dispatcher calling a
// method that a bodyless stub happens to satisfy.
func TestGameRouteHandlersExist(t *testing.T) {
	declared := declaredServerMethods(t)
	for action, method := range gameRouteHandlers {
		if !declared[method] {
			t.Errorf("action %q names %s, which is not a method on *Server", action, method)
		}
	}
}

// TestGameRouteDispatcherStaysSmall is the property the split exists to
// establish: the dispatcher parses the path and hands off, and does not grow a
// body again.
func TestGameRouteDispatcherStaysSmall(t *testing.T) {
	// Eighty is not tight: the switch is one line per action and there are
	// twenty-one. The bound is here to catch a body growing back, not to
	// constrain the table.
	const limit = 80
	data, err := os.ReadFile("routes_game.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "routes_game.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "handleGameRoutes" {
			continue
		}
		lines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
		if lines > limit {
			t.Errorf("handleGameRoutes is %d lines; the dispatcher must stay under %d", lines, limit)
		}
		return
	}
	t.Fatal("handleGameRoutes was not declared in routes_game.go")
}

// dispatchActions reads the action-to-handler map out of the switch inside
// handleGameRoutes.
func dispatchActions(t *testing.T, filename string) map[string]string {
	t.Helper()
	file := parsePackageFile(t, filename)
	dispatched := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "handleGameRoutes" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || !isPartsSegment(sw.Tag, 1) {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok || clause.List == nil {
					continue
				}
				method := calledServerMethod(t, clause.Body)
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Errorf("a case label is not a string literal: %T", expr)
						continue
					}
					action, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Errorf("unquote case label %s: %v", lit.Value, err)
						continue
					}
					dispatched[action] = method
				}
			}
			return false
		})
	}
	if len(dispatched) == 0 {
		t.Fatal("no dispatch switch found in handleGameRoutes")
	}
	return dispatched
}

// declaredServerMethods is the set of method names declared on *Server across
// the package's non-test files.
func declaredServerMethods(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, decl := range parsePackageFile(t, name).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !recvIsServer(fn.Recv) {
				continue
			}
			declared[fn.Name.Name] = true
		}
	}
	return declared
}

// calledServerMethod returns the name of the s.method(...) call a case body
// makes, and fails the test if the body does anything else, because handing off
// is the whole job of a case in the dispatcher.
func calledServerMethod(t *testing.T, body []ast.Stmt) string {
	t.Helper()
	if len(body) != 1 {
		t.Errorf("a case body has %d statements; the dispatcher hands off rather than doing work", len(body))
		return ""
	}
	expr, ok := body[0].(*ast.ExprStmt)
	if !ok {
		t.Errorf("a case body is %T, not a dispatch call", body[0])
		return ""
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		t.Errorf("a case body is not a call")
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		t.Errorf("a case body does not call a method")
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != "s" {
		t.Errorf("a case body calls something other than a server method")
		return ""
	}
	return sel.Sel.Name
}

// isPartsSegment reports whether expr is parts[i].
func isPartsSegment(expr ast.Expr, i int) bool {
	idx, ok := expr.(*ast.IndexExpr)
	if !ok {
		return false
	}
	ident, ok := idx.X.(*ast.Ident)
	if !ok || ident.Name != "parts" {
		return false
	}
	lit, ok := idx.Index.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == strconv.Itoa(i)
}

// recvIsServer reports whether a method receiver is *Server.
func recvIsServer(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Server"
}

// parsePackageFile parses one file of this package for the guards above.
func parsePackageFile(t *testing.T, filename string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	return file
}
