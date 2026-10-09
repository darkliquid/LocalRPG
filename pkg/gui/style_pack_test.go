package gui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

// TestApplyStylePackIsolates the pack from the developer's own config: NewService
// with a project root keeps the styles folder inside the test's temp directory.
func TestApplyStylePack(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	t.Cleanup(func() {
		svc.Close()
		media.SetActiveTables(media.Tables{})
	})

	stylesDir := svc.configMgr.StylesDir()
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pack := "id: ashen\nscene_palettes:\n  fantasy:\n    sky_top: \"#000000\"\n"
	if err := os.WriteFile(filepath.Join(stylesDir, "ashen.yaml"), []byte(pack), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := *svc.configMgr.Get()
	cfg.Styles.Pack = "ashen"
	if err := svc.configMgr.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if warnings := svc.applyStylePack(); len(warnings) != 0 {
		t.Fatalf("a valid pack warned: %v", warnings)
	}
	if media.ActivePackID() != "ashen" {
		t.Fatalf("active pack = %q", media.ActivePackID())
	}
}

func TestInvalidStylePackWarnsAndIsIgnored(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	t.Cleanup(func() {
		svc.Close()
		media.SetActiveTables(media.Tables{})
	})

	stylesDir := svc.configMgr.StylesDir()
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "id: bad\nscene_palettes:\n  fantasy:\n    sky_top: not-a-colour\n"
	if err := os.WriteFile(filepath.Join(stylesDir, "bad.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := *svc.configMgr.Get()
	cfg.Styles.Pack = "bad"
	if err := svc.configMgr.Save(&cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	warnings := svc.applyStylePack()
	if len(warnings) == 0 {
		t.Fatal("an invalid pack should warn")
	}
	if media.ActivePackID() != "" {
		t.Fatalf("an invalid pack should leave the built-in look, got %q", media.ActivePackID())
	}
	if len(svc.styleWarnings()) == 0 {
		t.Fatal("the warning should be recorded for the settings to show")
	}
}

func TestListStylePacksEndpoint(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	t.Cleanup(func() {
		svc.Close()
		media.SetActiveTables(media.Tables{})
	})

	stylesDir := svc.configMgr.StylesDir()
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stylesDir, "ashen.yaml"), []byte("id: ashen\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dto, err := svc.ListStylePacks(t.Context())
	if err != nil {
		t.Fatalf("ListStylePacks: %v", err)
	}
	if len(dto.Packs) != 1 || dto.Packs[0].ID != "ashen" {
		t.Fatalf("packs = %+v", dto.Packs)
	}
}
