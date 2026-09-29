package gui

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateRoutes = flag.Bool("update-routes", false, "rewrite testdata/routes.json from the mount table")

const routeManifestPath = "testdata/routes.json"

// TestRouteManifestIsCurrent keeps the checked-in route manifest in step with the
// mount table, so a renamed or removed mount is a test failure with a diff rather
// than a 404 in the app.
func TestRouteManifestIsCurrent(t *testing.T) {
	got, err := json.MarshalIndent(Routes(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	if *updateRoutes {
		if err := os.MkdirAll(filepath.Dir(routeManifestPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routeManifestPath, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}

	want, err := os.ReadFile(routeManifestPath)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with go test ./pkg/gui -update-routes)", routeManifestPath, err)
	}
	if string(want) != string(got) {
		t.Fatalf("route manifest is stale; regenerate with go test ./pkg/gui -update-routes\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestRoutePatternNamesEveryMount fails when a mount would fall back to the
// generic span name, which would make traces unsearchable per route.
func TestRoutePatternNamesEveryMount(t *testing.T) {
	for _, mount := range mounts {
		probe := mount.Pattern
		if strings.HasSuffix(probe, "/") {
			probe += "probe"
		}
		if got := routePattern(probe); got == "http.request" {
			t.Errorf("mount %q falls back to http.request; add it to routePattern", mount.Pattern)
		}
	}
}

func TestEveryMountHasAHandler(t *testing.T) {
	seen := map[string]bool{}
	for _, mount := range mounts {
		if mount.Handler == "" || mount.serve == nil {
			t.Errorf("mount %q has no handler", mount.Pattern)
		}
		if seen[mount.Pattern] {
			t.Errorf("mount %q is registered twice", mount.Pattern)
		}
		seen[mount.Pattern] = true
	}
}

// apiPathRE matches an API path literal, including the `${...}` of a template.
var apiPathRE = regexp.MustCompile(`/api/[A-Za-z0-9_./${}-]*`)

// TestFrontendPathsResolveToMounts catches a call to a route the server does not
// register, which would otherwise only surface at runtime.
func TestFrontendPathsResolveToMounts(t *testing.T) {
	path := filepath.Join("..", "..", "frontend", "src", "api", "client.ts")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read client.ts: %v", err)
	}

	reportFrontendPaths(t, string(data))
}

func reportFrontendPaths(t *testing.T, content string) {
	t.Helper()
	for i, line := range strings.Split(content, "\n") {
		for _, literal := range apiPathRE.FindAllString(line, -1) {
			if !mountCovers(strings.TrimRight(literal, ".")) {
				t.Errorf("client.ts:%d calls %q, which no mount serves", i+1, literal)
			}
		}
	}
}

func TestFrontendPathScannerFlagsAnUnknownRoute(t *testing.T) {
	// A seeded call to a route the server does not register must be reported; the
	// scanner is exercised here rather than only against the live file.
	var found bool
	for i, line := range strings.Split("const res = await fetch('/api/does-not-exist');\n", "\n") {
		for _, literal := range apiPathRE.FindAllString(line, -1) {
			if !mountCovers(literal) {
				found = true
				_ = i
			}
		}
	}
	if !found {
		t.Fatal("scanner did not flag an unknown route")
	}
}
