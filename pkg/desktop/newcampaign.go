package desktop

import (
	"context"
	"os"
	"path/filepath"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// liveService is the service the boot path supplied, or nil for snapshots.
var liveService *gui.Service

// createGame creates a campaign and returns its summary. It is injected by Run
// for a live service and left nil for snapshot tests.
var createGame func(ctx context.Context, svc *gui.Service, req gui.CreateGameRequestDTO) (*gui.GameSummaryDTO, error)

// generatePreview renders a stateless banner/icon preview.
var generatePreview func(ctx context.Context, svc *gui.Service, req gui.GenerateAssetPreviewRequestDTO) ([]byte, string, error)

// saveAsset writes a generated banner/icon onto a campaign.
var saveAsset func(ctx context.Context, svc *gui.Service, gameID, kind string, data []byte, ext string) error

type newCampaignForm struct {
	Name       string
	PlayerName string
	Opening    string
	SystemID   string
}

var newForm = newCampaignForm{PlayerName: "Adventurer"}

func newCampaignView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24), Gap(12)), func() {
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Label("New Campaign", FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
			Filler(1)
			NextAccessName("new-campaign.cancel")
			if Button(NoIcon, "Cancel") {
				appState.Screen = ScreenLauncher
			}
			AssignAccess()
		})

		Label("World", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, Gap(8)), func() {
			for i := range appState.Worlds {
				world := &appState.Worlds[i]
				selected := world.ID == appState.PendingWorld
				Container(Attrs(Pad(10), Corners(8), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(p.Border))
					}
					NextAccessName("new-campaign.world." + world.ID)
					if PressAction() {
						appState.PendingWorld = world.ID
						if newForm.Name == "" {
							newForm.Name = "Chronicles of " + world.Name
						}
					}
					AssignAccess()
					Label(world.Name, FontSize(14), TextColorVec(p.Text))
				})
			}
		})

		Label("System", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, Gap(8)), func() {
			for i := range appState.Systems {
				system := &appState.Systems[i]
				selected := system.ID == newForm.SystemID
				Container(Attrs(Pad(10), Corners(8), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(p.Border))
					}
					NextAccessName("new-campaign.system." + system.ID)
					if PressAction() {
						newForm.SystemID = system.ID
					}
					AssignAccess()
					Label(system.Name, FontSize(14), TextColorVec(p.Text))
				})
			}
		})

		Label("Campaign name", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.Name)
		Label("Player name", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.PlayerName)
		Label("Opening prompt", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		TextInput(&newForm.Opening)

		Label("Artwork", FontSize(13), FontWeight(WeightBold), TextColorVec(p.Muted))
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			NextAccessName("new-campaign.ai-banner")
			if Button(NoIcon, "AI banner") {
				runPreview("banner")
			}
			AssignAccess()
			if appState.FormBannerPreview != "" {
				artTile(p, appState.FormBannerPreview, "B", 80)
			}
			NextAccessName("new-campaign.ai-icon")
			if Button(NoIcon, "AI icon") {
				runPreview("icon")
			}
			AssignAccess()
			if appState.FormIconPreview != "" {
				artTile(p, appState.FormIconPreview, "I", 48)
			}
		})

		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Filler(1)
			NextAccessName("new-campaign.create")
			if canCreate() && Button(NoIcon, "Create") {
				submitCreate()
			}
			AssignAccess()
		})
	})
}

func canCreate() bool {
	return appState.PendingWorld != "" && newForm.SystemID != "" && newForm.Name != ""
}

func runPreview(kind string) {
	svc := liveService
	if generatePreview == nil || svc == nil {
		return
	}
	go func() {
		_ = generateFormPreview(context.Background(), svc, kind)
		RequestNextFrame()
	}()
}

// generateFormPreview renders a banner/icon preview and writes it to a temp
// file so the form can draw it with shirei's path-based Image.
func generateFormPreview(ctx context.Context, svc *gui.Service, kind string) error {
	if generatePreview == nil {
		return nil
	}
	data, _, err := generatePreview(ctx, svc, gui.GenerateAssetPreviewRequestDTO{
		Kind:        kind,
		Name:        newForm.Name,
		Description: newForm.Opening,
	})
	if err != nil {
		return err
	}
	path := filepath.Join(os.TempDir(), "localrpg-"+kind+"-preview.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if kind == "banner" {
		appState.FormBannerPreview = path
	} else {
		appState.FormIconPreview = path
	}
	return nil
}

// saveFormAssets persists any generated previews onto the new campaign.
func saveFormAssets(ctx context.Context, svc *gui.Service, gameID string) {
	if saveAsset == nil || svc == nil {
		return
	}
	for kind, path := range map[string]string{
		"banner": appState.FormBannerPreview,
		"icon":   appState.FormIconPreview,
	} {
		if path == "" {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			_ = saveAsset(ctx, svc, gameID, kind, data, "png")
		}
	}
}

func submitCreate() {
	req := gui.CreateGameRequestDTO{
		Name:          newForm.Name,
		WorldID:       appState.PendingWorld,
		SystemID:      newForm.SystemID,
		PlayerName:    newForm.PlayerName,
		OpeningPrompt: newForm.Opening,
	}
	svc := liveService
	if createGame == nil || svc == nil {
		appState.Screen = ScreenLauncher
		return
	}
	appState.Screen = ScreenLauncher
	go func() {
		ctx := context.Background()
		created, err := createGame(ctx, svc, req)
		if err == nil && created != nil {
			saveFormAssets(ctx, svc, created.ID)
		}
		reload(ctx, svc)
	}()
}
