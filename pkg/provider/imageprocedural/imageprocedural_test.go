package imageprocedural_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/imageprocedural"
)

func TestProceduralRegistrationAndDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyImageProceduralArt))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyImageProceduralArt)
	}
	desc := reg.Descriptor
	if desc.Family != provider.FamilyImage {
		t.Errorf("Family = %v, want %v", desc.Family, provider.FamilyImage)
	}
	if desc.Tier != provider.TierOfflineBasic {
		t.Errorf("Tier = %v, want %v", desc.Tier, provider.TierOfflineBasic)
	}
	if len(desc.Features) == 0 || desc.Features[0] != provider.FeatureOffline {
		t.Errorf("expected FeatureOffline, got %v", desc.Features)
	}
	if len(desc.Presets) == 0 || desc.Presets[0].ID != "procedural-art" {
		t.Errorf("unexpected presets: %+v", desc.Presets)
	}
}

func TestProceduralBuildAndGenerate(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyImageProceduralArt))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyImageProceduralArt)
	}

	built, err := reg.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	client, ok := built.(media.ImageClient)
	if !ok {
		t.Fatalf("expected media.ImageClient, got %T", built)
	}

	img, err := client.GenerateImage(context.Background(), "a gloomy mountain fortress")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !strings.Contains(string(img), "<svg") {
		t.Errorf("expected SVG output, got: %s", string(img))
	}
}
