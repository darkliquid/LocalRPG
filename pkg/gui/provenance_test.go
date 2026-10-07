package gui

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestImportReportsTrustState(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, "config")
	t.Setenv("LOCALRPG_CONFIG_DIR", cfgDir)

	svc := NewService(tmpDir)

	// 1. Test unsigned package
	srcDir := filepath.Join(tmpDir, "src-unsigned")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: world_unsigned\nname: World Unsigned\nversion: 1.0.0\ndescription: Unsigned test\n"
	if err := os.WriteFile(filepath.Join(srcDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	var unsignedBuf bytes.Buffer
	if _, err := content.Pack(srcDir, "world", content.ManifestMeta{}, &unsignedBuf); err != nil {
		t.Fatal(err)
	}

	resUnsigned, err := svc.ImportContent(context.Background(), &unsignedBuf, "refuse")
	if err != nil {
		t.Fatalf("import unsigned failed: %v", err)
	}
	if resUnsigned.Trust.State != "unsigned" {
		t.Fatalf("expected state 'unsigned', got %q", resUnsigned.Trust.State)
	}

	// 2. Test signed package with untrusted key
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pubFP := content.Fingerprint(pub)

	srcSigned := filepath.Join(tmpDir, "src-signed")
	if err := os.MkdirAll(srcSigned, 0755); err != nil {
		t.Fatal(err)
	}
	signedYAML := "id: world_signed\nname: World Signed\nversion: 1.0.0\ndescription: Signed test\n"
	if err := os.WriteFile(filepath.Join(srcSigned, "world.yaml"), []byte(signedYAML), 0644); err != nil {
		t.Fatal(err)
	}

	if err := content.SignDirectory(srcSigned, priv, "Alice"); err != nil {
		t.Fatalf("sign directory failed: %v", err)
	}

	var signedBuf bytes.Buffer
	if _, err := content.Pack(srcSigned, "world", content.ManifestMeta{}, &signedBuf); err != nil {
		t.Fatal(err)
	}

	// Make a copy for trusted test
	signedBytes := signedBuf.Bytes()

	resSignedUnknown, err := svc.ImportContent(context.Background(), bytes.NewReader(signedBytes), "refuse")
	if err != nil {
		t.Fatalf("import signed untrusted failed: %v", err)
	}
	if resSignedUnknown.Trust.State != "unknown_key" {
		t.Fatalf("expected state 'unknown_key', got %q", resSignedUnknown.Trust.State)
	}
	if resSignedUnknown.Trust.Fingerprint != pubFP {
		t.Fatalf("expected fingerprint %q, got %q", pubFP, resSignedUnknown.Trust.Fingerprint)
	}

	// 3. Test signed package with trusted publisher
	cfg := svc.configMgr.Get()
	if cfg.Publishers == nil {
		cfg.Publishers = make(map[string]string)
	}
	cfg.Publishers[pubFP] = "Alice"
	if err := svc.configMgr.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	resSignedTrusted, err := svc.ImportContent(context.Background(), bytes.NewReader(signedBytes), "rename")
	if err != nil {
		t.Fatalf("import signed trusted failed: %v", err)
	}
	if resSignedTrusted.Trust.State != "verified" {
		t.Fatalf("expected state 'verified', got %q", resSignedTrusted.Trust.State)
	}
	if resSignedTrusted.Trust.Publisher != "Alice" {
		t.Fatalf("expected publisher 'Alice', got %q", resSignedTrusted.Trust.Publisher)
	}

	// 4. Test tampered package fails
	tamperedBytes := append([]byte(nil), signedBytes...)
	tamperedBytes[len(tamperedBytes)-10] ^= 0xff
	_, err = svc.ImportContent(context.Background(), bytes.NewReader(tamperedBytes), "refuse")
	if err == nil {
		t.Fatal("expected error importing tampered package, got nil")
	}
}
