package registry

import "testing"

func TestNormalizeSource(t *testing.T) {
	valid := []string{
		"https://example.org/index.json",
		"http://example.org/index.json",
		"git+https://example.org/repo.git",
		"git+ssh://git@example.org/repo.git",
		"git+file:///srv/registry",
	}
	for _, in := range valid {
		out, err := NormalizeSource(in)
		if err != nil {
			t.Errorf("NormalizeSource(%q) error = %v", in, err)
			continue
		}
		if out != in {
			t.Errorf("NormalizeSource(%q) = %q, want unchanged", in, out)
		}
	}

	invalid := []string{"", "   ", "ftp://example.org", "example.org", "github.com/foo/bar", "git+"}
	for _, in := range invalid {
		if out, err := NormalizeSource(in); err == nil {
			t.Errorf("NormalizeSource(%q) = %q, want an error", in, out)
		}
	}

	out, err := NormalizeSource("  https://example.org/index.json  ")
	if err != nil || out != "https://example.org/index.json" {
		t.Errorf("NormalizeSource trimmed = %q, %v; want the URL trimmed", out, err)
	}
}