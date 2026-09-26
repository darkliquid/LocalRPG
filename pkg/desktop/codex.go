package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Injected codex write operations; nil outside a live app.
var (
	saveEntity         func(ctx context.Context, svc *gui.Service, gameID, entityID, markdown string) error
	regeneratePortrait func(ctx context.Context, svc *gui.Service, gameID, characterID string) error
	mergeEntities      func(ctx context.Context, svc *gui.Service, gameID, sourceID, targetID string) error
	portraitPath       func(ctx context.Context, svc *gui.Service, gameID, characterID string) (string, error)
)

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

// mergeTargets lists the entities the open note could merge into.
func mergeTargets() []gui.EntitySummaryDTO {
	var out []gui.EntitySummaryDTO
	if appState.Entity == nil {
		return out
	}
	for _, entity := range appState.Entities {
		if entity.ID != appState.Entity.ID {
			out = append(out, entity)
		}
	}
	return out
}

// openEntity loads one entity note, its memories, and a character portrait.
func openEntity(id string) {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		ctx := context.Background()
		entity, markdown := loadEntity(ctx, svc, gameID, id)
		memories := []gui.MemoryDTO{}
		portrait := ""
		if entity != nil {
			memories = loadMemories(svc, gameID, id)
			if entity.Type == "character" && portraitPath != nil {
				if path, err := portraitPath(ctx, svc, gameID, id); err == nil {
					portrait = path
				}
			}
		}
		WithFrameLock(func() {
			appState.Entity = entity
			appState.EntityMarkdown = markdown
			appState.Memories = memories
			appState.PortraitPath = portrait
			appState.CodexTab = "notes"
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

		if appState.Entity.Type == "character" {
			Container(Attrs(Row, CrossMid, Gap(8)), func() {
				if appState.PortraitPath != "" {
					artTile(p, appState.PortraitPath, "?", 56)
				}
				NextAccessName("codex.portrait.regenerate")
				if Button(NoIcon, "Regenerate portrait") {
					svc := liveService
					gameID := appState.OpenGame
					id := appState.Entity.ID
					if regeneratePortrait != nil && svc != nil {
						go func() {
							_ = regeneratePortrait(context.Background(), svc, gameID, id)
							openEntity(id)
						}()
					}
				}
				AssignAccess()
			})
		}

		Container(Attrs(Row, Gap(4)), func() {
			for _, tab := range []struct{ key, label string }{{"notes", "Notes"}, {"memories", "Memories"}} {
				tab := tab
				selected := appState.CodexTab == tab.key || (tab.key == "notes" && appState.CodexTab == "")
				Container(Attrs(Pad2(2, 8), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("codex.tab." + tab.key)
					if PressAction() {
						appState.CodexTab = tab.key
					}
					AssignAccess()
					Label(tab.label, FontSize(11), TextColorVec(p.Text))
				})
			}
		})

		if appState.CodexTab == "memories" {
			if len(appState.Memories) == 0 {
				Label("No memories yet.", FontSize(12), TextColorVec(p.Muted))
			}
			for _, memory := range appState.Memories {
				Label(fmt.Sprintf("t%d · %s", memory.Turn, memory.Text), FontSize(11), TextColorVec(p.Text))
			}
			return
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
			NextAccessName("codex.merge")
			if Button(NoIcon, "Merge note…") {
				appState.MergeOpen = true
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

		if appState.MergeOpen {
			mergeModal(p)
		}
	})
}

func mergeModal(p ui.Palette) {
	stage := ModalStyle{Background: p.Panel, Text: p.Text, Scrim: Vec4{0, 0, 0, 0.6}}
	ModalStyled(520, func() { appState.MergeOpen = false }, stage, func() {
		Label("Merge into", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
		for _, entity := range mergeTargets() {
			id := entity.ID
			NextAccessName("codex.merge.target." + id)
			if Button(NoIcon, entity.Name) {
				svc := liveService
				gameID := appState.OpenGame
				source := appState.Entity.ID
				if mergeEntities != nil && svc != nil {
					go func() {
						_ = mergeEntities(context.Background(), svc, gameID, source, id)
						refreshEntities()
					}()
				}
				appState.MergeOpen = false
				appState.Entity = nil
			}
			AssignAccess()
		}
		NextAccessName("codex.merge.cancel")
		if Button(NoIcon, "Cancel") {
			appState.MergeOpen = false
		}
		AssignAccess()
	})
}

// writePortrait fetches a character portrait and writes it to a temp file.
func writePortrait(ctx context.Context, svc *gui.Service, gameID, characterID string) (string, error) {
	data, ext, err := svc.GetCharacterPortrait(ctx, gameID, characterID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(os.TempDir(), "localrpg-portrait-"+characterID+"."+ext)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
