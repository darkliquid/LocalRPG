package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestPublisherAddListRemove(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	var stdout, stderr bytes.Buffer

	// 1. Initially list should be empty or show no publishers
	code := runPublisherCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher list failed: %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "alice") {
		t.Fatalf("unexpected publisher in list: %s", stdout.String())
	}

	// 2. Add publisher by fingerprint directly
	dummyFP := strings.Repeat("a", 64)
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"add", dummyFP, "--name", "Alice"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher add failed: %d, stderr: %s", code, stderr.String())
	}

	// 3. List should show Alice
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher list failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Alice") || !strings.Contains(stdout.String(), dummyFP) {
		t.Fatalf("expected Alice with fp %s in list, got: %s", dummyFP, stdout.String())
	}

	// 4. Add publisher using a pubkey file
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubFP := content.Fingerprint(pub)
	pubKeyPath := filepath.Join(cfgDir, "bob.pub")
	if err := os.WriteFile(pubKeyPath, []byte(hex.EncodeToString(pub)), 0644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"add", pubKeyPath, "--name", "Bob"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher add with file failed: %d, stderr: %s", code, stderr.String())
	}

	// Verify Bob is now in list
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher list failed: %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Bob") || !strings.Contains(stdout.String(), pubFP) {
		t.Fatalf("expected Bob with fp %s in list, got: %s", pubFP, stdout.String())
	}

	// 5. Remove Alice
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"remove", dummyFP}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher remove failed: %d, stderr: %s", code, stderr.String())
	}

	// Verify Alice is gone but Bob remains
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher list failed: %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Alice") {
		t.Fatalf("expected Alice to be removed, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Bob") {
		t.Fatalf("expected Bob to remain, got: %s", stdout.String())
	}
}

func TestContentVerifyCLIReportsState(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	workDir := t.TempDir()
	createTestWorldDir(t, workDir, "sig_world", "Signed World")

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	packPath := filepath.Join(workDir, "test.lrpgpack")
	var stdout, stderr bytes.Buffer
	code := runContentCommand([]string{"export", "world", "sig_world", "--out", packPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("export returned %d, stderr: %s", code, stderr.String())
	}

	// 1. Verify unsigned package reports "unsigned" and exits 0
	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"verify", packPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify unsigned returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "unsigned") {
		t.Fatalf("expected 'unsigned' in output, got: %s", stdout.String())
	}

	// 2. Generate key and sign package
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(workDir, "publisher.key")
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(priv)), 0600); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"sign", packPath, "--key", keyPath, "--publisher", "Author"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("sign returned %d, stderr: %s", code, stderr.String())
	}

	// 3. Verify signed package with untrusted key reports "unknown_key" and exits 0
	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"verify", packPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify unknown_key returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "unknown_key") {
		t.Fatalf("expected 'unknown_key' in output, got: %s", stdout.String())
	}

	// 4. Trust the publisher
	pubFP := content.Fingerprint(pub)
	stdout.Reset()
	stderr.Reset()
	code = runPublisherCommand([]string{"add", pubFP, "--name", "Author"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("publisher add returned %d, stderr: %s", code, stderr.String())
	}

	// 5. Verify signed package now reports "verified" and exits 0
	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"verify", packPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("verify verified returned %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "verified") {
		t.Fatalf("expected 'verified' in output, got: %s", stdout.String())
	}

	// 6. Tamper with the package file -> verify reports "invalid" and exits non-zero
	packBytes, err := os.ReadFile(packPath)
	if err != nil {
		t.Fatal(err)
	}
	// Tamper some bytes in the middle
	packBytes[len(packBytes)-10] ^= 0xff
	if err := os.WriteFile(packPath, packBytes, 0644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = runContentCommand([]string{"verify", packPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code for tampered package, got 0; stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "invalid") && !strings.Contains(stderr.String(), "invalid") {
		t.Fatalf("expected 'invalid' in output/stderr, got stdout: %s, stderr: %s", stdout.String(), stderr.String())
	}
}
