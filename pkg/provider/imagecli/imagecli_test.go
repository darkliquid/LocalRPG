package imagecli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/imagecli"
)

func TestImageCLIRegistrationAndDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyImageCLI))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyImageCLI)
	}
	desc := reg.Descriptor
	if desc.Family != provider.FamilyImage {
		t.Errorf("Family = %v, want %v", desc.Family, provider.FamilyImage)
	}
	if desc.Tier != provider.TierLocalServer {
		t.Errorf("Tier = %v, want %v", desc.Tier, provider.TierLocalServer)
	}
	if len(desc.Presets) == 0 || desc.Presets[0].ID != "sd-cli" {
		t.Errorf("unexpected presets: %+v", desc.Presets)
	}
}

func TestImageCLIBuild(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyImageCLI))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyImageCLI)
	}

	built, err := reg.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build with nil failed: %v", err)
	}
	if _, ok := built.(media.ImageClient); !ok {
		t.Fatalf("expected media.ImageClient, got %T", built)
	}

	payload := media.ImageBuildPayload{
		Config: config.ImageConfig{
			Command: "echo",
			Args:    []string{"--test"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	built, err = reg.Build(context.Background(), raw)
	if err != nil {
		t.Fatalf("Build with valid payload failed: %v", err)
	}
	if _, ok := built.(media.ImageClient); !ok {
		t.Fatalf("expected media.ImageClient, got %T", built)
	}

	_, err = reg.Build(context.Background(), []byte("not-json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
}

func TestCLIImageClientGenerateSuccess(t *testing.T) {
	client := imagecli.NewCLIImageClient("sh", []string{"-c", `printf 'rendered-image:%s' "$1"`, "--"})
	out, err := client.GenerateImage(context.Background(), "a dragon in a cave")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	want := "rendered-image:a dragon in a cave"
	if string(out) != want {
		t.Errorf("GenerateImage = %q, want %q", string(out), want)
	}
}

func TestCLIImageClientGenerateError(t *testing.T) {
	client := imagecli.NewCLIImageClient("sh", []string{"-c", "echo 'out of vram' >&2; exit 2", "--"})
	_, err := client.GenerateImage(context.Background(), "a dragon")
	if err == nil {
		t.Fatal("expected GenerateImage to fail, got nil")
	}
	if !strings.Contains(err.Error(), "cli image error") {
		t.Errorf("error = %v, want substring 'cli image error'", err)
	}
}
