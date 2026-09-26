package desktop

import (
	"context"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Injected studio operations; nil outside a live app.
var (
	saveSystem   func(ctx context.Context, svc *gui.Service, req gui.CreateSystemRequestDTO) error
	generateText func(ctx context.Context, svc *gui.Service, req gui.GenerateTextRequest) (*gui.GenerateTextResponse, error)
)

func systemTab() string {
	if appState.SystemTab == "" {
		return "manifest"
	}
	return appState.SystemTab
}

func openSystemsStudio() {
	appState.Screen = ScreenSystemsStudio
	appState.SystemTab = "manifest"
	refreshSystems()
}

func refreshSystems() {
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		systems := loadSystems(context.Background(), svc)
		WithFrameLock(func() { appState.Systems = systems })
		RequestNextFrame()
	}()
}

func newSystemDraft() {
	appState.Studio = Selection{Kind: "draft"}
	appState.System = nil
	appState.SystemSaved = false
	appState.StudioDirty = false
	appState.FormSysName = ""
	appState.FormSysVersion = "1.0.0"
	appState.FormSysSlug = ""
	appState.FormSysDesc = ""
	appState.FormSysRules = ""
	appState.FormSysScript = ""
	appState.FormSysPrelude = ""
	appState.FormSysFields = nil
}

func loadSystemIntoForm(id string) {
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		detail := loadSystemDetail(context.Background(), svc, id)
		WithFrameLock(func() {
			appState.System = detail
			appState.Studio = Selection{Kind: "saved", ID: id}
			appState.SystemSaved = true
			appState.StudioDirty = false
			if detail != nil {
				appState.FormSysName = detail.Name
				appState.FormSysVersion = detail.Version
				appState.FormSysSlug = detail.ID
				appState.FormSysDesc = detail.Description
				appState.FormSysRules = detail.RulesPrompt
				appState.FormSysScript = detail.Script
				appState.FormSysPrelude = detail.CharacterCreation.Preamble
				appState.FormSysFields = detail.CharacterCreation.Fields
			}
		})
		RequestNextFrame()
	}()
}

// systemPayload builds the save request; a draft omits its ID so the service
// slugifies the name.
func systemPayload() gui.CreateSystemRequestDTO {
	id := ""
	if appState.SystemSaved {
		id = appState.FormSysSlug
	}
	return gui.CreateSystemRequestDTO{
		ID:          id,
		Name:        appState.FormSysName,
		Version:     appState.FormSysVersion,
		Description: appState.FormSysDesc,
		RulesPrompt: appState.FormSysRules,
		Script:      appState.FormSysScript,
		CharacterCreation: core.CharacterCreationSpec{
			Preamble: appState.FormSysPrelude,
			Fields:   appState.FormSysFields,
		},
	}
}

func systemsStudioView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(20), Gap(10)), func() {
		ScrollOnInput()
		ScrollBars()
		studioHeader("Systems Studio", newSystemDraft, func() { appState.Screen = ScreenLauncher })

		Container(Attrs(Row, Wrap, Gap(6)), func() {
			for i := range appState.Systems {
				system := appState.Systems[i]
				selected := appState.Studio.Kind == "saved" && appState.Studio.ID == system.ID
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("systems.item." + system.ID)
					if PressAction() {
						loadSystemIntoForm(system.ID)
					}
					AssignAccess()
					Label(system.Name, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		Container(Attrs(Row, Gap(4)), func() {
			for _, tab := range []struct{ key, label string }{
				{"manifest", "Manifest"}, {"rules", "rules.md"}, {"script", "mechanics.js"},
			} {
				tab := tab
				selected := systemTab() == tab.key
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("systems.tab." + tab.key)
					if PressAction() {
						appState.SystemTab = tab.key
					}
					AssignAccess()
					Label(tab.label, FontSize(11), TextColorVec(p.Text))
				})
			}
		})

		switch systemTab() {
		case "rules":
			TextArea(&appState.FormSysRules)
		case "script":
			TextArea(&appState.FormSysScript)
		default:
			Label("Name", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormSysName)
			Label("Version", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormSysVersion)
			Label("Slug", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormSysSlug)
			Label("Description", FontSize(12), TextColorVec(p.Muted))
			TextArea(&appState.FormSysDesc)
			Label("Creation preamble", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormSysPrelude)
			characterFieldsEditor(p)
		}

		Container(Attrs(Row, Gap(8)), func() {
			NextAccessName("systems.save")
			if Button(NoIcon, "Save") {
				svc := liveService
				req := systemPayload()
				if saveSystem != nil && svc != nil {
					go func() {
						if err := saveSystem(context.Background(), svc, req); err == nil {
							WithFrameLock(func() {
								appState.SystemSaved = true
								appState.StudioDirty = false
							})
							refreshSystems()
						}
					}()
				}
			}
			AssignAccess()
		})
	})
}

func characterFieldsEditor(p ui.Palette) {
	Label("Character creation fields", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
	for i := range appState.FormSysFields {
		field := &appState.FormSysFields[i]
		Container(Attrs(Row, CrossMid, Gap(6)), func() {
			Container(Attrs(FixWidth(120)), func() { TextInput(&field.ID) })
			Container(Attrs(FixWidth(160)), func() { TextInput(&field.Label) })
			Container(Attrs(FixWidth(240)), func() { TextInput(&field.Prompt) })
			CheckBox(&field.Generatable, "gen")
		})
	}
	NextAccessName("systems.field.add")
	if Button(NoIcon, "Add field") {
		appState.FormSysFields = append(appState.FormSysFields, core.CharacterCreationField{ID: "field", Label: "Field"})
	}
	AssignAccess()
}
