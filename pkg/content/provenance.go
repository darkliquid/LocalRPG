package content

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Trust is the outcome of a provenance check.
type Trust struct {
	State       string `json:"state"`                 // "verified" | "unknown_key" | "unsigned" | "invalid"
	Publisher   string `json:"publisher,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// PackageSig describes the contents of package.sig.
type PackageSig struct {
	Algorithm string `yaml:"algorithm"`
	Key       string `yaml:"key"`                  // the SHA-256 fingerprint of the public key (hex)
	PublicKey string `yaml:"public_key,omitempty"` // base64-encoded public key
	Signature string `yaml:"signature"`            // base64-encoded Ed25519 signature
	Publisher string `yaml:"publisher,omitempty"`  // display name
}

// Fingerprint returns the hex-encoded SHA-256 digest of an Ed25519 public key.
func Fingerprint(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

// Sign returns the package.sig YAML bytes for a manifest and a private key.
func Sign(m Manifest, key ed25519.PrivateKey, publisher string) ([]byte, error) {
	pub := key.Public().(ed25519.PublicKey)
	fp := Fingerprint(pub)
	digest := m.ContentDigest()
	sig := ed25519.Sign(key, []byte(digest))

	ps := PackageSig{
		Algorithm: "ed25519",
		Key:       fp,
		PublicKey: base64.StdEncoding.EncodeToString(pub),
		Signature: base64.StdEncoding.EncodeToString(sig),
		Publisher: publisher,
	}

	data, err := yaml.Marshal(ps)
	if err != nil {
		return nil, fmt.Errorf("marshal package.sig: %w", err)
	}
	return data, nil
}

// Verify checks a package.sig against a manifest and returns the trust state.
func Verify(m Manifest, sigBytes []byte, trusted map[string]string) (Trust, error) {
	if len(sigBytes) == 0 {
		return Trust{State: "unsigned"}, nil
	}

	var ps PackageSig
	if err := yaml.Unmarshal(sigBytes, &ps); err != nil {
		return Trust{State: "invalid"}, fmt.Errorf("parse package.sig: %w", err)
	}

	if ps.Algorithm != "ed25519" {
		return Trust{State: "invalid", Fingerprint: ps.Key}, fmt.Errorf("unsupported signature algorithm: %q", ps.Algorithm)
	}

	pubBytes, err := base64.StdEncoding.DecodeString(ps.PublicKey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return Trust{State: "invalid", Fingerprint: ps.Key}, fmt.Errorf("invalid public key: %w", err)
	}
	pub := ed25519.PublicKey(pubBytes)

	computedFP := Fingerprint(pub)
	if ps.Key != "" && ps.Key != computedFP {
		return Trust{State: "invalid", Fingerprint: computedFP}, fmt.Errorf("key fingerprint mismatch")
	}

	sig, err := base64.StdEncoding.DecodeString(ps.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return Trust{State: "invalid", Fingerprint: computedFP}, fmt.Errorf("invalid signature encoding")
	}

	digest := m.ContentDigest()
	if !ed25519.Verify(pub, []byte(digest), sig) {
		return Trust{State: "invalid", Fingerprint: computedFP}, fmt.Errorf("signature verification failed")
	}

	// Signature is mathematically valid. Check trust list.
	if trusted != nil {
		if pubName, ok := trusted[computedFP]; ok {
			return Trust{
				State:       "verified",
				Publisher:   pubName,
				Fingerprint: computedFP,
			}, nil
		}
	}

	return Trust{
		State:       "unknown_key",
		Publisher:   ps.Publisher,
		Fingerprint: computedFP,
	}, nil
}
