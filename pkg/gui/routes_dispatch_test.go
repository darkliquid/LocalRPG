package gui

// The three dispatchers this file guards each replaced a chain of shape guards,
// and the chains matched only some of the time: a /entity with no entity id, a
// scene-image with the wrong method, a five-segment turn path. Those
// fall-throughs are behaviour, and a restructure that tidied them away would
// still compile and still pass a suite that never asked for them. They are
// pinned here.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// functionLines returns the line span of one function declaration, and whether it
// was found at all.
func functionLines(t *testing.T, filename, name string) (int, bool) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name {
			continue
		}
		return fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1, true
	}
	return 0, false
}

// TestRouteDispatchersStaySmall is the property the split exists to establish: a
// dispatcher parses the path and hands off, and does not grow a body again.
func TestRouteDispatchersStaySmall(t *testing.T) {
	cases := []struct{ file, fn string }{
		{"routes_world.go", "handleWorldRoutes"},
		{"routes_game_turn.go", "handleGameTurnRoute"},
		{"routes_game_turn.go", "handleTurnActionRoute"},
		{"routes_game_entity.go", "handleGameEntityRoute"},
	}
	// Eighty is not tight: each is a table of arms plus the path it parsed. The
	// bound catches a body growing back, it does not constrain the table.
	const limit = 80
	for _, tc := range cases {
		lines, ok := functionLines(t, tc.file, tc.fn)
		if !ok {
			t.Errorf("%s was not declared in %s", tc.fn, tc.file)
			continue
		}
		if lines > limit {
			t.Errorf("%s is %d lines; a dispatcher must stay under %d", tc.fn, lines, limit)
		}
	}
}

// TestWorldRouteDispatcherHandsOff checks that every arm of the world dispatcher
// is a single call to a server method, so the dispatcher cannot grow a body back
// one arm at a time. The arm count is pinned because an arm is a route, and a
// route arriving or leaving without this table moving is the drift being caught.
func TestWorldRouteDispatcherHandsOff(t *testing.T) {
	const wantArms = 8
	declared := declaredServerMethods(t)
	file := parsePackageFile(t, "routes_world.go")

	arms := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "handleWorldRoutes" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			// The dispatcher's switch is a guard switch: its conditions are the
			// shapes, so it has no tag.
			if !ok || sw.Tag != nil {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				arms++
				method := calledServerMethod(t, clause.Body)
				if method != "" && !declared[method] {
					t.Errorf("an arm calls %s, which is not a method on *Server", method)
				}
			}
			return false
		})
	}

	switch {
	case arms == 0:
		t.Fatal("no dispatch switch found in handleWorldRoutes")
	case arms != wantArms:
		t.Errorf("handleWorldRoutes has %d arms, want %d", arms, wantArms)
	}
}

// TestRouteFallThroughsAreUnchanged pins the shapes that name no sub-resource.
// Each was let through to a weaker handler by the chain this batch replaced, and
// each is behaviour rather than an accident of the restructure.
func TestRouteFallThroughsAreUnchanged(t *testing.T) {
	gameID, svc := turnFixture(t)
	server := NewServer(svc, http.NotFoundHandler())

	world, err := svc.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{
			name:   "a world entity route with no entity id is the world",
			method: http.MethodGet,
			path:   "/api/world/" + world.ID + "/entity",
			want:   http.StatusOK,
		},
		{
			name:   "a world entities route that is not accept is the world",
			method: http.MethodGet,
			path:   "/api/world/" + world.ID + "/entities/pending",
			want:   http.StatusOK,
		},
		{
			name:   "a scene-image with a method it has no answer for is not found",
			method: http.MethodPut,
			path:   "/api/game/" + gameID + "/turn/1/scene-image",
			want:   http.StatusNotFound,
		},
		{
			name:   "a turn play without POST is not found",
			method: http.MethodGet,
			path:   "/api/game/" + gameID + "/turn/1/play",
			want:   http.StatusNotFound,
		},
		{
			name:   "a resolve-check without POST is not found",
			method: http.MethodGet,
			path:   "/api/game/" + gameID + "/turn/1/resolve-check",
			want:   http.StatusNotFound,
		},
		{
			name:   "a ledger refuses a method it has no answer for",
			method: http.MethodDelete,
			path:   "/api/game/" + gameID + "/turn/1/ledger",
			want:   http.StatusMethodNotAllowed,
		},
		{
			name:   "a five-segment turn path is not found",
			method: http.MethodPost,
			path:   "/api/game/" + gameID + "/turn/1/segment/1",
			want:   http.StatusNotFound,
		},
		{
			name:   "a segment play with a trailing segment is not found",
			method: http.MethodPost,
			path:   "/api/game/" + gameID + "/turn/1/segment/1/play/extra",
			want:   http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("%s %s status = %d, want %d: %s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
			}
			// A route the dispatcher refused answers with the not-found page the
			// standard library writes, where a request that reached a handler and
			// was refused answers with the API's error envelope. The difference is
			// what tells a missing route from a missing record.
			if tc.want == http.StatusNotFound && !strings.Contains(rec.Body.String(), "404 page not found") {
				t.Errorf("%s %s answered 404 with %q, want the route's own not-found page", tc.method, tc.path, rec.Body.String())
			}
			if tc.want == http.StatusOK && !strings.Contains(rec.Body.String(), world.ID) {
				t.Errorf("%s %s answered %q, want the world", tc.method, tc.path, rec.Body.String())
			}
		})
	}
}
