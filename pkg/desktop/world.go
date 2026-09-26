package desktop

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// refreshWorld reloads the campaign state and recap.
func refreshWorld() {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		state, recap := loadWorld(context.Background(), svc, gameID)
		WithFrameLock(func() {
			appState.GameState = state
			appState.Recap = recap
		})
		RequestNextFrame()
	}()
}

// progressFraction clamps a value/max ratio to [0,1].
func progressFraction(value, max int) float64 {
	if max <= 0 {
		return 0
	}
	frac := float64(value) / float64(max)
	if frac < 0 {
		return 0
	}
	if frac > 1 {
		return 1
	}
	return frac
}

// bar draws a rounded progress track and fill.
func bar(p ui.Palette, frac float64, colour Vec4) {
	Container(Attrs(Expand, FixHeight(6), Corners(3), BackgroundVec(p.Bg)), func() {
		if frac <= 0 {
			return
		}
		Container(Attrs(Grow(float32(frac)), FixHeight(6), Corners(3), BackgroundVec(colour)), func() {})
	})
}

func worldDrawer(p ui.Palette) {
	settingsSmallButton("world.refresh", "Refresh", refreshWorld)

	if appState.Recap != nil {
		settingsSubTitle(TypTime, "Story So Far")
		if appState.Recap.Summary != "" {
			Label(appState.Recap.Summary, FontSize(12), TextColorVec(ui.TextMuted))
		} else if !appState.Recap.Enabled {
			settingsHint("Recap is disabled.")
		}
		for _, thread := range appState.Recap.Threads {
			Label(fmt.Sprintf("%s · %s · t%d", thread.Name, thread.Status, thread.LastAdvanced),
				FontSize(11), TextColorVec(ui.TextMuted))
		}
	}

	if appState.GameState == nil {
		return
	}

	if len(appState.GameState.Arcs) > 0 {
		settingsSubTitle(TypChartLine, "Narrative Arcs")
		for _, arc := range appState.GameState.Arcs {
			Label(fmt.Sprintf("%s (%d/%d)", arc.Name, arc.Progress, arc.MaxProgress), FontSize(11), TextColorVec(ui.TextMain))
			bar(p, progressFraction(arc.Progress, arc.MaxProgress), ui.Accent)
		}
	}

	if len(appState.GameState.Clocks) > 0 {
		settingsSubTitle(TypTime, "Faction Clocks")
		for _, clock := range appState.GameState.Clocks {
			Label(fmt.Sprintf("%s · %s (%d/%d)", clock.Name, clock.Faction, clock.Ticks, clock.MaxTicks),
				FontSize(11), TextColorVec(ui.TextMain))
			bar(p, progressFraction(clock.Ticks, clock.MaxTicks), ui.Danger)
		}
	}
}

func characterDrawer(p ui.Palette) {
	state := appState.GameState
	if state == nil {
		settingsHint("No character loaded.")
		return
	}
	player := state.Player
	settingsSubTitle(TypUser, player.Name)
	level := 1
	if v, ok := numericField(player.State, "level"); ok {
		level = v
	}
	Label(fmt.Sprintf("level %d · %s", level, player.Type), FontSize(12), TextColorVec(ui.TextMuted))

	hp, _ := numericField(player.State, "hp")
	maxHP, ok := numericField(player.State, "max_hp")
	if !ok {
		maxHP = 20
	}
	Label(fmt.Sprintf("HP %d/%d", hp, maxHP), FontSize(12), TextColorVec(ui.TextMain))
	bar(p, progressFraction(hp, maxHP), ui.Accent)

	if player.Appearance != "" {
		Label(player.Appearance, FontSize(11), TextColorVec(ui.TextMuted))
	}
	if player.Voice != nil {
		name := player.Voice.Name
		if name == "" {
			name = player.Voice.VoiceID
		}
		Label("voice: "+name, FontSize(11), TextColorVec(ui.TextMuted))
	}
	for key, value := range player.State {
		switch key {
		case "hp", "max_hp", "level":
			continue
		}
		Label(fmt.Sprintf("%s: %v", key, value), FontSize(11), TextColorVec(ui.TextMuted))
	}
}

// numericField reads an integer-ish value out of an arbitrary state map.
func numericField(state map[string]any, key string) (int, bool) {
	value, ok := state[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	default:
		return 0, false
	}
}
