package media

import (
	"context"
	"testing"
)

func TestSherpaTTSClientListVoices(t *testing.T) {
	client := NewSherpaTTSClient(t.TempDir())
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) != 11 {
		t.Fatalf("expected 11 voices, got %d", len(voices))
	}

	foundBella := false
	for _, v := range voices {
		if v.ID == "af_bella" {
			foundBella = true
			if v.Name != "Bella (American Female)" {
				t.Errorf("expected name 'Bella (American Female)', got %q", v.Name)
			}
			if v.Gender != "female" {
				t.Errorf("expected gender 'female', got %q", v.Gender)
			}
		}
	}
	if !foundBella {
		t.Errorf("did not find 'af_bella' in listed voices")
	}
}
