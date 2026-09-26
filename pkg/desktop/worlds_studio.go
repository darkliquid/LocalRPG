package desktop

import (
	"context"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Injected world operations; nil outside a live app.
var (
	saveWorld        func(ctx context.Context, svc *gui.Service, req gui.CreateWorldRequestDTO) error
	saveWorldEntity  func(ctx context.Context, svc *gui.Service, worldID, entityID, markdown string) error
	deleteWorldEntity func(ctx context.Context, svc *gui.Service, worldID, entityID string) error
)

func worldTab() string {
	if appState.WorldTab == "" {
		return "lore"
	}
	return appState.WorldTab
}

func openWorldsStudio() {
	appState.Screen = ScreenWorldsStudio
	appState.WorldTab = "lore"
	refreshWorlds()
}

func refreshWorlds() {
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		worlds := loadWorlds(context.Background(), svc)
		WithFrameLock(func() { appState.Worlds = worlds })
		RequestNextFrame()
	}()
}

func newWorldDraft() {
	appState.Studio = Selection{Kind: "draft"}
	appState.World = nil
	appState.WorldSaved = false
	appState.StudioDirty = false
	appState.FormWorldName = ""
	appState.FormWorldSlug = ""
	appState.FormWorldGenre = ""
	appState.FormWorldSys = ""
	appState.FormWorldStyle = ""
	appState.FormWorldTags = ""
	appState.FormWorldDesc = ""
	appState.FormWorldLore = ""
	appState.WorldEntities = nil
	appState.WorldEntityID = ""
	appState.WorldMarkdown = ""
}

func loadWorldIntoForm(id string) {
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		detail := loadWorldDetail(context.Background(), svc, id)
		WithFrameLock(func() {
			appState.World = detail
			appState.Studio = Selection{Kind: "saved", ID: id}
			appState.WorldSaved = true
			appState.StudioDirty = false
			if detail != nil {
				appState.FormWorldName = detail.Name
				appState.FormWorldSlug = detail.ID
				appState.FormWorldGenre = detail.Genre
				appState.FormWorldSys = detail.DefaultSystem
				appState.FormWorldStyle = detail.ArtStyle
				appState.FormWorldTags = strings.Join(detail.Tags, ", ")
				appState.FormWorldDesc = detail.Description
				appState.FormWorldLore = detail.LorePrompt
				appState.WorldEntities = detail.Entities
			}
		})
		RequestNextFrame()
	}()
}

// worldPayload builds the save request; a draft omits its ID.
func worldPayload() gui.CreateWorldRequestDTO {
	id := ""
	if appState.WorldSaved {
		id = appState.FormWorldSlug
	}
	var tags []string
	for _, tag := range strings.Split(appState.FormWorldTags, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return gui.CreateWorldRequestDTO{
		ID:            id,
		Name:          appState.FormWorldName,
		Description:   appState.FormWorldDesc,
		Genre:         appState.FormWorldGenre,
		DefaultSystem: appState.FormWorldSys,
		ArtStyle:      appState.FormWorldStyle,
		Tags:          tags,
		LorePrompt:    appState.FormWorldLore,
	}
}

func worldsStudioView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(20), Gap(10)), func() {
		ScrollOnInput()
		ScrollBars()
		studioHeader("Worlds Studio", newWorldDraft, func() { appState.Screen = ScreenLauncher })

		Container(Attrs(Row, Wrap, Gap(6)), func() {
			for i := range appState.Worlds {
				world := appState.Worlds[i]
				selected := appState.Studio.Kind == "saved" && appState.Studio.ID == world.ID
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Panel)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("worlds.item." + world.ID)
					if PressAction() {
						loadWorldIntoForm(world.ID)
					}
					AssignAccess()
					Label(world.Name, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		Container(Attrs(Row, Gap(4)), func() {
			for _, tab := range []struct{ key, label string }{
				{"lore", "Lore"}, {"prompt", "lore.md"}, {"entities", "Entities"},
			} {
				tab := tab
				selected := worldTab() == tab.key
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("worlds.tab." + tab.key)
					if PressAction() {
						appState.WorldTab = tab.key
					}
					AssignAccess()
					Label(tab.label, FontSize(11), TextColorVec(p.Text))
				})
			}
		})

		switch worldTab() {
		case "prompt":
			TextArea(&appState.FormWorldLore)
		case "entities":
			worldEntitiesEditor(p)
		default:
			Label("Name", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormWorldName)
			Label("Genre", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormWorldGenre)
			Label("Slug", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormWorldSlug)
			Label("Default system", FontSize(12), TextColorVec(p.Muted))
			MenuButton(NoIcon, worldSystemLabel(), func() {
				for _, system := range appState.Systems {
					system := system
					if MenuItem(NoIcon, system.Name) {
						appState.FormWorldSys = system.ID
					}
				}
			})
			Label("Art style", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormWorldStyle)
			Label("Tags (comma-separated)", FontSize(12), TextColorVec(p.Muted))
			TextInput(&appState.FormWorldTags)
			Label("Description", FontSize(12), TextColorVec(p.Muted))
			TextArea(&appState.FormWorldDesc)
		}

		Container(Attrs(Row, Gap(8)), func() {
			NextAccessName("worlds.save")
			if Button(NoIcon, "Save") {
				svc := liveService
				req := worldPayload()
				if saveWorld != nil && svc != nil {
					go func() {
						if err := saveWorld(context.Background(), svc, req); err == nil {
							WithFrameLock(func() {
								appState.WorldSaved = true
								appState.StudioDirty = false
							})
							refreshWorlds()
						}
					}()
				}
			}
			AssignAccess()
		})
	})
}

func worldSystemLabel() string {
	if appState.FormWorldSys == "" {
		return "(default system)"
	}
	return appState.FormWorldSys
}

func worldEntitiesEditor(p ui.Palette) {
	if !appState.WorldSaved {
		Label("Save the world before editing its entities.", FontSize(12), TextColorVec(p.Muted))
		return
	}
	Container(Attrs(Row, Wrap, Gap(6)), func() {
		for _, entity := range appState.WorldEntities {
			id := entity.ID
			selected := appState.WorldEntityID == id
			Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Panel)), func() {
				if selected {
					ModAttrs(BackgroundVec(p.Accent))
				}
				NextAccessName("worlds.entity." + id)
				if PressAction() {
					openWorldEntity(id)
				}
				AssignAccess()
				Label(entity.Name, FontSize(12), TextColorVec(p.Text))
			})
		}
	})
	if appState.WorldEntityID == "" {
		return
	}
	TextArea(&appState.WorldMarkdown)
	Container(Attrs(Row, Gap(8)), func() {
		NextAccessName("worlds.entity.save")
		if Button(NoIcon, "Save entity") {
			svc := liveService
			worldID := appState.Studio.ID
			entityID := appState.WorldEntityID
			markdown := appState.WorldMarkdown
			if saveWorldEntity != nil && svc != nil {
				go func() {
					_ = saveWorldEntity(context.Background(), svc, worldID, entityID, markdown)
					loadWorldIntoForm(worldID)
				}()
			}
		}
		AssignAccess()
		NextAccessName("worlds.entity.delete")
		if Button(NoIcon, "Delete entity") {
			svc := liveService
			worldID := appState.Studio.ID
			entityID := appState.WorldEntityID
			if deleteWorldEntity != nil && svc != nil {
				go func() {
					_ = deleteWorldEntity(context.Background(), svc, worldID, entityID)
					WithFrameLock(func() { appState.WorldEntityID = "" })
					loadWorldIntoForm(worldID)
				}()
			}
		}
		AssignAccess()
	})
}

func openWorldEntity(id string) {
	svc := liveService
	if svc == nil {
		return
	}
	worldID := appState.Studio.ID
	appState.WorldEntityID = id
	go func() {
		markdown, _ := loadWorldEntity(context.Background(), svc, worldID, id)
		WithFrameLock(func() { appState.WorldMarkdown = markdown })
		RequestNextFrame()
	}()
}
