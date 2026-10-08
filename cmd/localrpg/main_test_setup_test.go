package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

// TestMain points the configuration at a temporary directory for every test in
// this package.
//
// The CLI resolves the user's configuration from XDG, and xdg caches its
// directories when the package is first used, so a test that sets HOME and
// expects to be redirected is not: it writes the developer's real config.yaml.
// TestOfflineConfigCommands did exactly that, replacing a working provider setup
// with the offline preset on every `go test ./...`.
//
// LOCALRPG_CONFIG_DIR is read on each call rather than cached, so setting it here
// redirects every resolution in the package, whether a test asks for it or not.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "localrpg-config-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create a temporary config dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("LOCALRPG_CONFIG_DIR", dir); err != nil {
		fmt.Fprintf(os.Stderr, "cannot redirect the config dir: %v\n", err)
		os.Exit(1)
	}

	// A resolution that still escapes the temporary directory would write the
	// developer's config, so refuse to run at all rather than damage it.
	if read, write := config.DetectConfigFile(); read != dir+"/config.yaml" || write != dir+"/config.yaml" {
		fmt.Fprintf(os.Stderr, "refusing to run: the config resolves to %s, not %s\n", read, dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
