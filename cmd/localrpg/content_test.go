package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func createTestWorldDir(t *testing.T, baseDir, id, name string) {
	t.Helper()
	worldDir := filepath.Join(baseDir, "worlds", id)
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: " + id + "\nname: " + name + "\ndescription: A test world\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestContentExportCLI(t *testing.T) {
	tmpDir := t.TempDir()
	createTestWorldDir(t, tmpDir, "dusk_realm", "Dusk Realm")

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	outPath := filepath.Join(tmpDir, "dusk.lrpgpack")
	var stdout, stderr bytes.Buffer
	code := runContentCommand([]string{"export", "world", "dusk_realm", "--out", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export returned %d, stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output file %s not created: %v", outPath, err)
	}

	// Test export to stdout
	var stdoutBuf, stderrBuf bytes.Buffer
	code = runContentCommand([]string{"export", "world", "dusk_realm"}, &stdoutBuf, &stderrBuf)
	if code != 0 {
		t.Fatalf("export to stdout returned %d, stderr: %s", code, stderrBuf.String())
	}
	if stdoutBuf.Len() == 0 {
		t.Fatal("expected package bytes in stdout")
	}
}

func TestContentImportCLIRequiresYes(t *testing.T) {
	tmpDir := t.TempDir()
	createTestWorldDir(t, tmpDir, "src_world", "Source World")

	var pkgBuf bytes.Buffer
	if _, err := content.Pack(filepath.Join(tmpDir, "worlds", "src_world"), "world", content.ManifestMeta{}, &pkgBuf); err != nil {
		t.Fatal(err)
	}
	pkgFile := filepath.Join(tmpDir, "test.lrpgpack")
	if err := os.WriteFile(pkgFile, pkgBuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	targetDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(targetDir)
	defer func() { _ = os.Chdir(oldCwd) }()

	// Without --yes
	var stdout, stderr bytes.Buffer
	code := runContentCommand([]string{"import", pkgFile}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected import without --yes to fail")
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Fatalf("stderr does not mention --yes: %s", stderr.String())
	}

	// With --yes
	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"import", pkgFile, "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("import with --yes failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "src_world") {
		t.Fatalf("stdout did not confirm import: %s", stdout.String())
	}
}
