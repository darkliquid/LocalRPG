package registry

import (
	"testing"
)

func TestParseIndex(t *testing.T) {
	in := []byte(`{
		"name": "R",
		"homepage": "https://example.org",
		"packages": [
			{
				"type": "world",
				"id": "w",
				"name": "World W",
				"version": "1.0.0",
				"download": "https://x/w.lrpgpack",
				"sha256": "ab1234",
				"publisher": "pubfp",
				"requires": [
					{"type": "system", "id": "sys1", "version": ">=1.0.0"}
				]
			}
		]
	}`)
	idx, err := ParseIndex(in)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Name != "R" || len(idx.Packages) != 1 || idx.Packages[0].ID != "w" {
		t.Fatalf("index = %+v", idx)
	}
	p := idx.Packages[0]
	if p.Type != "world" || p.Download != "https://x/w.lrpgpack" || p.SHA256 != "ab1234" || p.Publisher != "pubfp" {
		t.Fatalf("package = %+v", p)
	}
	if len(p.Requires) != 1 || p.Requires[0].ID != "sys1" {
		t.Fatalf("requires = %+v", p.Requires)
	}
}

func TestParseIndexInvalid(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"invalid json", `{"name":`},
		{"missing name", `{"packages":[]}`},
		{"missing download", `{"name":"R","packages":[{"type":"world","id":"w","version":"1.0.0","sha256":"ab"}]}`},
		{"missing sha256", `{"name":"R","packages":[{"type":"world","id":"w","version":"1.0.0","download":"https://x"}]}`},
		{"invalid type", `{"name":"R","packages":[{"type":"game","id":"w","version":"1.0.0","download":"https://x","sha256":"ab"}]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseIndex([]byte(tc.json))
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
