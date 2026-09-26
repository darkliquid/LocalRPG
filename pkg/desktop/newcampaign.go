package desktop

import (
	"context"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// liveService is the service the boot path supplied, or nil for snapshots.
var liveService *gui.Service

// createGame creates a campaign. It is injected by Run for a live service and
// left nil for snapshot tests, so rendering never needs a backend.
var createGame func(ctx context.Context, svc *gui.Service, req gui.CreateGameRequestDTO) error

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
		_ = createGame(context.Background(), svc, req)
		reload(context.Background(), svc)
	}()
}
