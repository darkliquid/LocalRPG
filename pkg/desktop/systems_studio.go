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
				studioPill("systems.item."+system.ID, system.Name, selected, func() {
					loadSystemIntoForm(system.ID)
				})
			}
		})

		studioTabStrip("systems.tab", []studioTab{
			{"manifest", "Manifest"}, {"rules", "rules.md"}, {"script", "mechanics.js"},
		}, systemTab(), func(key string) { appState.SystemTab = key })

		studioForm(func() {
			switch systemTab() {
			case "rules":
				FieldArea(&appState.FormSysRules)
			case "script":
				FieldArea(&appState.FormSysScript)
			default:
				studioFieldLabel("Name")
				FieldInput(&appState.FormSysName)
				studioFieldLabel("Version")
				FieldInput(&appState.FormSysVersion)
				studioFieldLabel("Slug")
				FieldInput(&appState.FormSysSlug)
				studioFieldLabel("Description")
				FieldArea(&appState.FormSysDesc)
				studioFieldLabel("Creation preamble")
				FieldInput(&appState.FormSysPrelude)
				characterFieldsEditor(p)
			}

			Container(Attrs(Row, Gap(8)), func() {
				studioPrimaryButton("systems.save", "Save", func() {
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
				})
			})
		})
	})
}

func characterFieldsEditor(p ui.Palette) {
	Label("Character creation fields", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
	for i := range appState.FormSysFields {
		field := &appState.FormSysFields[i]
		Container(Attrs(Row, CrossMid, Gap(6)), func() {
			Container(Attrs(FixWidth(120)), func() { FieldInput(&field.ID) })
			Container(Attrs(FixWidth(160)), func() { FieldInput(&field.Label) })
			Container(Attrs(FixWidth(240)), func() { FieldInput(&field.Prompt) })
			CheckBox(&field.Generatable, "gen")
		})
	}
	NextAccessName("systems.field.add")
	if Button(NoIcon, "Add field") {
		appState.FormSysFields = append(appState.FormSysFields, core.CharacterCreationField{ID: "field", Label: "Field"})
	}
	AssignAccess()
}
