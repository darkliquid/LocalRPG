package content_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	m := content.Manifest{Files: []content.FileEntry{{Path: "a", SHA256: "1", Size: 1}}}
	sig, err := content.Sign(m, priv, "someone")
	if err != nil {
		t.Fatal(err)
	}

	fp := content.Fingerprint(pub)
	tr, err := content.Verify(m, sig, map[string]string{fp: "someone"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.State != "verified" || tr.Publisher != "someone" || tr.Fingerprint != fp {
		t.Fatalf("trust = %+v, want verified by someone (%s)", tr, fp)
	}

	// Untrusted key
	trUntrusted, err := content.Verify(m, sig, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trUntrusted.State != "unknown_key" || trUntrusted.Fingerprint != fp {
		t.Fatalf("untrusted key state = %q, want unknown_key", trUntrusted.State)
	}
}

func TestVerifyRejectsTamperedManifest(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := content.Manifest{Files: []content.FileEntry{{Path: "a", SHA256: "1", Size: 1}}}
	sig, err := content.Sign(m, priv, "someone")
	if err != nil {
		t.Fatal(err)
	}

	// Tampered manifest: file checksum changed
	tampered := content.Manifest{Files: []content.FileEntry{{Path: "a", SHA256: "tampered", Size: 1}}}
	tr, err := content.Verify(tampered, sig, nil)
	if err == nil {
		t.Fatal("expected error on tampered manifest, got nil")
	}
	if tr.State != "invalid" {
		t.Fatalf("tr.State = %q, want invalid", tr.State)
	}
}

func TestVerifyUnsigned(t *testing.T) {
	m := content.Manifest{Files: []content.FileEntry{{Path: "a", SHA256: "1", Size: 1}}}
	tr, err := content.Verify(m, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.State != "unsigned" {
		t.Fatalf("tr.State = %q, want unsigned", tr.State)
	}

	trEmpty, err := content.Verify(m, []byte{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trEmpty.State != "unsigned" {
		t.Fatalf("tr.State = %q, want unsigned", trEmpty.State)
	}
}
