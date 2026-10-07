package content_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
)

func TestContentDigestIsStable(t *testing.T) {
	m := content.Manifest{Files: []content.FileEntry{
		{Path: "b", SHA256: "2", Size: 2},
		{Path: "a", SHA256: "1", Size: 1},
	}}
	reordered := content.Manifest{Files: []content.FileEntry{
		{Path: "a", SHA256: "1", Size: 1},
		{Path: "b", SHA256: "2", Size: 2},
	}}
	if m.ContentDigest() != reordered.ContentDigest() {
		t.Fatal("the digest must not depend on file order")
	}
	if m.ContentDigest() == (content.Manifest{Files: []content.FileEntry{{Path: "a", SHA256: "9", Size: 1}}}).ContentDigest() {
		t.Fatal("a changed checksum must change the digest")
	}
}

func TestContentDigestExcludesSignature(t *testing.T) {
	m := content.Manifest{Files: []content.FileEntry{
		{Path: "a", SHA256: "1", Size: 1},
	}}
	withSig := content.Manifest{Files: []content.FileEntry{
		{Path: "package.sig", SHA256: "999", Size: 100},
		{Path: "a", SHA256: "1", Size: 1},
	}}
	if m.ContentDigest() != withSig.ContentDigest() {
		t.Fatal("package.sig should be excluded from ContentDigest")
	}
}

func TestContentDigestGolden(t *testing.T) {
	m := content.Manifest{Files: []content.FileEntry{
		{Path: "a.txt", SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Size: 0},
		{Path: "b.txt", SHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Size: 3},
	}}
	const expected = "2001ee63e483eb55e5479a2e9bc0c07e219ad604decbf17f67485166aebff766"
	got := m.ContentDigest()
	if got != expected {
		t.Fatalf("ContentDigest = %q, want %q", got, expected)
	}
}
