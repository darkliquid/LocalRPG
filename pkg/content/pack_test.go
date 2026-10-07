package content_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func writeFixtureWorld(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	worldYAML := `id: ashen_reach
name: Ashen Reach
description: A blasted caldera
`
	if err := os.WriteFile(filepath.Join(dir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}
	entitiesDir := filepath.Join(dir, "entities")
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	entity := `---
name: Gatehouse
type: location
---
The rusted gatehouse stands watch.
`
	if err := os.WriteFile(filepath.Join(entitiesDir, "gatehouse.md"), []byte(entity), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPackProducesManifestAndMembers(t *testing.T) {
	dir := writeFixtureWorld(t)
	var buf bytes.Buffer
	m, err := content.Pack(dir, "world", content.ManifestMeta{}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "ashen_reach" || len(m.Files) == 0 {
		t.Fatalf("manifest = %+v", m)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	firstHdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar next: %v", err)
	}
	if firstHdr.Name != "package.yaml" {
		t.Fatalf("first member = %s, want package.yaml", firstHdr.Name)
	}

	manifestBytes, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("read package.yaml: %v", err)
	}
	parsedM, err := content.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse package.yaml from tar: %v", err)
	}
	if parsedM.ID != "ashen_reach" {
		t.Fatalf("parsed manifest ID = %s, want ashen_reach", parsedM.ID)
	}
}

func TestPackRejectsInvalidSemver(t *testing.T) {
	dir := writeFixtureWorld(t)
	var buf bytes.Buffer
	_, err := content.Pack(dir, "world", content.ManifestMeta{Version: "not-a-semver"}, &buf)
	if err == nil {
		t.Fatal("expected error packing with invalid semver, got nil")
	}
}
