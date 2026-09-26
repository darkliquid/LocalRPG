package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestFilterNoise(t *testing.T) {
	in := strings.NewReader("first\n" + gpuFallbackNoise + "\nsecond\n")
	var out bytes.Buffer
	filterNoise(in, &out, gpuFallbackNoise)
	got := out.String()
	if strings.Contains(got, gpuFallbackNoise) {
		t.Fatalf("noise leaked through: %q", got)
	}
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("other lines dropped: %q", got)
	}
}
