package content

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// SignDirectory generates a manifest for dir, computes package.sig using key,
// and writes package.sig into dir.
func SignDirectory(dir string, key ed25519.PrivateKey, publisher string) error {
	m, err := BuildManifest(dir, "", ManifestMeta{})
	if err != nil {
		return fmt.Errorf("build manifest for %q: %w", dir, err)
	}

	sigBytes, err := Sign(m, key, publisher)
	if err != nil {
		return fmt.Errorf("sign manifest: %w", err)
	}

	target := filepath.Join(dir, "package.sig")
	if err := os.WriteFile(target, sigBytes, 0644); err != nil {
		return fmt.Errorf("write package.sig: %w", err)
	}
	return nil
}

// SignPackageFile signs an existing .lrpgpack archive by parsing its package.yaml,
// generating package.sig, and rewriting the archive to include package.sig.
func SignPackageFile(archivePath string, key ed25519.PrivateKey, publisher string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("read gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	type member struct {
		hdr  tar.Header
		data []byte
	}

	var manifest *Manifest
	var manifestData []byte
	var otherMembers []member

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		cleanName := filepath.ToSlash(filepath.Clean(hdr.Name))
		if cleanName == "package.sig" {
			// discard previous signature when re-signing
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("read member %q: %w", hdr.Name, err)
		}

		if cleanName == "package.yaml" {
			m, err := ParseManifest(data)
			if err != nil {
				return fmt.Errorf("parse package.yaml: %w", err)
			}
			manifest = &m
			manifestData = data
		} else {
			otherMembers = append(otherMembers, member{
				hdr:  *hdr,
				data: data,
			})
		}
	}

	if manifest == nil {
		return errors.New("archive is missing package.yaml")
	}

	sigBytes, err := Sign(*manifest, key, publisher)
	if err != nil {
		return fmt.Errorf("sign manifest: %w", err)
	}

	// Write to temporary file in the same directory, then rename
	dir := filepath.Dir(archivePath)
	tmpFile, err := os.CreateTemp(dir, ".lrpgpack-sign-*")
	if err != nil {
		return fmt.Errorf("create temp archive: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	gzw := gzip.NewWriter(tmpFile)
	gzw.ModTime = time.Unix(0, 0)
	gzw.OS = 255
	tw := tar.NewWriter(gzw)

	// 1. package.yaml
	pkgHdr := &tar.Header{
		Name:     "package.yaml",
		Mode:     0644,
		Size:     int64(len(manifestData)),
		ModTime:  time.Unix(0, 0),
		Format:   tar.FormatPAX,
	}
	if err := tw.WriteHeader(pkgHdr); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write package.yaml header: %w", err)
	}
	if _, err := tw.Write(manifestData); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write package.yaml content: %w", err)
	}

	// 2. package.sig
	sigHdr := &tar.Header{
		Name:     "package.sig",
		Mode:     0644,
		Size:     int64(len(sigBytes)),
		ModTime:  time.Unix(0, 0),
		Format:   tar.FormatPAX,
	}
	if err := tw.WriteHeader(sigHdr); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write package.sig header: %w", err)
	}
	if _, err := tw.Write(sigBytes); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write package.sig content: %w", err)
	}

	// 3. other files
	for _, m := range otherMembers {
		if err := tw.WriteHeader(&m.hdr); err != nil {
			_ = tmpFile.Close()
			return fmt.Errorf("write %q header: %w", m.hdr.Name, err)
		}
		if _, err := tw.Write(m.data); err != nil {
			_ = tmpFile.Close()
			return fmt.Errorf("write %q content: %w", m.hdr.Name, err)
		}
	}

	if err := tw.Close(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("close tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("close gzip: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	_ = f.Close()
	if err := os.Rename(tmpName, archivePath); err != nil {
		return fmt.Errorf("replace archive: %w", err)
	}

	return nil
}

// ReadPackageManifestAndSig reads the manifest and optional signature bytes from a .lrpgpack file or directory.
// When reading a .lrpgpack, it also verifies that every archive member matches the manifest's declared size and sha256 checksum.
func ReadPackageManifestAndSig(targetPath string) (Manifest, []byte, error) {
	fi, err := os.Stat(targetPath)
	if err != nil {
		return Manifest{}, nil, err
	}

	if fi.IsDir() {
		m, err := BuildManifest(targetPath, "", ManifestMeta{})
		if err != nil {
			return Manifest{}, nil, err
		}
		sigBytes, _ := os.ReadFile(filepath.Join(targetPath, "package.sig"))
		return m, sigBytes, nil
	}

	f, err := os.Open(targetPath)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// First pass / header: package.yaml
	firstHdr, err := tr.Next()
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read first archive member: %w", err)
	}
	if filepath.ToSlash(filepath.Clean(firstHdr.Name)) != "package.yaml" {
		return Manifest{}, nil, fmt.Errorf("first archive member must be package.yaml, got %q", firstHdr.Name)
	}

	manifestBytes, err := io.ReadAll(tr)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read package.yaml: %w", err)
	}
	m, err := ParseManifest(manifestBytes)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("parse package.yaml: %w", err)
	}

	declared := make(map[string]FileEntry, len(m.Files))
	for _, fe := range m.Files {
		declared[filepath.ToSlash(filepath.Clean(fe.Path))] = fe
	}

	var sigBytes []byte
	extracted := make(map[string]bool, len(m.Files))

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read archive entry: %w", err)
		}

		cleanRel := filepath.ToSlash(filepath.Clean(hdr.Name))
		if cleanRel == "package.sig" {
			sig, err := io.ReadAll(tr)
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("read package.sig: %w", err)
			}
			sigBytes = sig
			continue
		}

		fe, isDeclared := declared[cleanRel]
		if !isDeclared {
			return Manifest{}, nil, fmt.Errorf("archive member %q not declared in manifest", cleanRel)
		}
		if extracted[cleanRel] {
			return Manifest{}, nil, fmt.Errorf("duplicate archive member %q", cleanRel)
		}
		if hdr.Size != fe.Size {
			return Manifest{}, nil, fmt.Errorf("archive member %q size %d does not match manifest size %d", cleanRel, hdr.Size, fe.Size)
		}

		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, tr)
		if copyErr != nil {
			return Manifest{}, nil, fmt.Errorf("read %q: %w", cleanRel, copyErr)
		}
		if written != fe.Size {
			return Manifest{}, nil, fmt.Errorf("read %q: got %d bytes, expected %d", cleanRel, written, fe.Size)
		}

		computedSum := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(computedSum, fe.SHA256) {
			return Manifest{}, nil, fmt.Errorf("checksum mismatch for %q: got %s, want %s", cleanRel, computedSum, fe.SHA256)
		}
		extracted[cleanRel] = true
	}

	if len(extracted) != len(declared) {
		return Manifest{}, nil, errors.New("archive is missing declared files")
	}

	// Drain any remaining bytes in the gzip stream to verify CRC32
	if _, err := io.Copy(io.Discard, gzr); err != nil {
		return Manifest{}, nil, fmt.Errorf("archive stream corrupt: %w", err)
	}

	return m, sigBytes, nil
}
