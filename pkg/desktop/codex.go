package desktop

import (
	"context"
	"fmt"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// saveEntity is injected by Run for a live service.
var saveEntity func(ctx context.Context, svc *gui.Service, gameID, entityID, markdown string) error

// filteredEntities returns indices into appState.Entities matching the current
// query and type facet.
func filteredEntities() []int {
	query := strings.ToLower(strings.TrimSpace(appState.EntityQuery))
	var out []int
	for i := range appState.Entities {
		entity := &appState.Entities[i]
		if appState.EntityType != "" && appState.EntityType != "all" && entity.Type != appState.EntityType {
			continue
		}
		if query != "" {
			match := strings.Contains(strings.ToLower(entity.Name), query) ||
				strings.Contains(strings.ToLower(entity.ID), query)
			if !match {
				for _, tag := range entity.Tags {
					if strings.Contains(strings.ToLower(tag), query) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, i)
	}
	return out
}

// openEntity loads one entity note into the editor.
func openEntity(id string) {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		entity, markdown := loadEntity(context.Background(), svc, gameID, id)
		WithFrameLock(func() {
			appState.Entity = entity
			appState.EntityMarkdown = markdown
		})
		RequestNextFrame()
	}()
}

// refreshEntities reloads the campaign's entity list.
func refreshEntities() {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		entities := loadEntities(context.Background(), svc, gameID)
		WithFrameLock(func() { appState.Entities = entities })
		RequestNextFrame()
	}()
}

func codexDrawer(p ui.Palette) {
	Container(Attrs(Gap(8)), func() {
		TextInput(&appState.EntityQuery)

		types := map[string]bool{}
		for _, entity := range appState.Entities {
			if entity.Type != "" {
				types[entity.Type] = true
			}
		}
		chips := []string{"all"}
		for t := range types {
			chips = append(chips, t)
		}
		Container(Attrs(Row, Wrap, Gap(4)), func() {
			for _, t := range chips {
				typ := t
				selected := appState.EntityType == typ || (typ == "all" && appState.EntityType == "")
				Container(Attrs(Pad2(2, 8), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("codex.type." + typ)
					if PressAction() {
						appState.EntityType = typ
					}
					AssignAccess()
					Label(typ, FontSize(11), TextColorVec(p.Text))
				})
			}
		})

		idxs := filteredEntities()
		VirtualListView("codex-list", len(idxs),
			func(i int) any { return appState.Entities[idxs[i]].ID },
			func(int, float32) float32 { return 36 },
			func(i int, width float32) {
				entity := appState.Entities[idxs[i]]
				Container(Attrs(Expand, FixHeight(32), Pad2(4, 8), Corners(6)), func() {
					if IsHovered() {
						ModAttrs(BackgroundVec(p.Bg))
					}
					NextAccessName("codex.entity." + entity.ID)
					if PressAction() {
						openEntity(entity.ID)
					}
					AssignAccess()
					Label(entity.Name+" · "+entity.Type, FontSize(12), TextColorVec(p.Text))
				})
			})

		if appState.Entity == nil {
			Label("Choose a note from the codex.", FontSize(12), TextColorVec(p.Muted))
			return
		}

		Label(appState.Entity.Name, FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
		if appState.Entity.ParseError {
			Label("This note has a frontmatter parse error.", FontSize(11), TextColorVec(p.Danger))
		}
		TextArea(&appState.EntityMarkdown)
		Container(Attrs(Row, Gap(8)), func() {
			NextAccessName("codex.save")
			if Button(NoIcon, "Save") {
				svc := liveService
				gameID := appState.OpenGame
				id := appState.Entity.ID
				markdown := appState.EntityMarkdown
				if saveEntity != nil && svc != nil {
					go func() {
						_ = saveEntity(context.Background(), svc, gameID, id, markdown)
						refreshEntities()
					}()
				}
			}
			AssignAccess()
		})

		if len(appState.Entity.Backlinks) > 0 {
			Label("Backlinks", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
			Container(Attrs(Row, Wrap, Gap(4)), func() {
				for _, link := range appState.Entity.Backlinks {
					id := link
					NextAccessName("codex.backlink." + id)
					if Button(NoIcon, id) {
						openEntity(id)
					}
					AssignAccess()
				}
			})
		}
		if len(appState.Entity.History) > 0 {
			Label("Appears in", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Muted))
			Container(Attrs(Row, Wrap, Gap(4)), func() {
				for _, turn := range appState.Entity.History {
					Label(fmt.Sprintf("t%d", turn), FontSize(11), TextColorVec(p.Muted))
				}
			})
		}
	})
}
