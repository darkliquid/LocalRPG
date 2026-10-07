package content_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func writeFixtureSystem(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sysYAML := "id: fixture_sys\nname: Fixture Sys\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "system.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mechanics.js"), []byte("// mechanics"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompts", "rules.md"), []byte("# Rules\nSome rules"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func TestBehaviouralDigestIgnoresProse(t *testing.T) {
	dir := writeFixtureSystem(t)
	a, err := content.BehaviouralDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendToFile(t, filepath.Join(dir, "prompts", "rules.md"), "\nmore prose\n")
	b, err := content.BehaviouralDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("a prose edit should not change the behavioural digest")
	}
	appendToFile(t, filepath.Join(dir, "mechanics.js"), "\n// x\n")
	c, err := content.BehaviouralDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatal("a mechanics.js edit should change the digest")
	}
}

func TestLockSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "content.lock.yaml")
	lock := content.ContentLock{
		App: "localrpg",
		Entries: []content.LockEntry{
			{
				Type:    "system",
				ID:      "d20",
				Version: "1.0.0",
				SHA256:  "abc123def456",
			},
			{
				Type:    "world",
				ID:      "realm",
				Version: "1.2.0",
				SHA256:  "789xyz",
			},
		},
	}
	if err := lock.Save(lockPath); err != nil {
		t.Fatalf("Save lock: %v", err)
	}
	loaded, err := content.LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if loaded.App != "localrpg" || len(loaded.Entries) != 2 {
		t.Fatalf("unexpected loaded lock: %+v", loaded)
	}
	entry, ok := loaded.FindEntry("system", "d20")
	if !ok || entry.Version != "1.0.0" || entry.SHA256 != "abc123def456" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}
