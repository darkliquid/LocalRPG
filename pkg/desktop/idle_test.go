package desktop

import (
	"testing"

	"go.hasen.dev/shirei"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// TestFormsIdleWhenSettled guards against a form that never stops requesting
// frames. A text field that grabs focus on mount animates its caret and asks
// the host for a frame every frame for five seconds, so switching between
// settings tabs or studio forms redraws continuously and flickers. Every form
// screen must reach an idle frame (no follow-up requested) within a couple of
// passes.
func TestFormsIdleWhenSettled(t *testing.T) {
	host := shirei.GetHost()
	host.WindowSize = shirei.Vec2{1000, 700}
	host.WindowScale = 1
	if host.ComfortScale == 0 {
		host.ComfortScale = 1
	}

	systems := &State{
		Loaded:      true,
		Screen:      ScreenSystemsStudio,
		Systems:     []gui.SystemSummaryDTO{{ID: "blades", Name: "Blades in the Dark"}},
		Studio:      Selection{Kind: "saved", ID: "blades"},
		SystemTab:   "manifest",
		FormSysName: "Blades in the Dark",
	}
	worlds := &State{
		Loaded:        true,
		Screen:        ScreenWorldsStudio,
		Worlds:        []gui.WorldSummaryDTO{{ID: "sundered", Name: "The Sundered Realm"}},
		Studio:        Selection{Kind: "saved", ID: "sundered"},
		WorldTab:      "lore",
		FormWorldName: "The Sundered Realm",
	}

	screens := []struct {
		name  string
		state *State
	}{
		{"settings.paths", settingsStateWithTab("paths")},
		{"settings.providers", settingsStateWithTab("providers")},
		{"settings.agents", settingsStateWithTab("agents")},
		{"settings.media", settingsStateWithTab("media")},
		{"settings.preferences", settingsStateWithTab("preferences")},
		{"settings.debug", settingsStateWithTab("debug")},
		{"studio.systems", systems},
		{"studio.worlds", worlds},
		{"new_campaign", &State{Loaded: true, Screen: ScreenNewCampaign}},
	}

	for _, screen := range screens {
		screen := screen
		t.Run(screen.name, func(t *testing.T) {
			shirei.ResetInputSession()
			appState = screen.state
			for i := 0; i < 4; i++ {
				out := shirei.RunFrameFn(shirei.FrameFn(RootView))
				if !out.NextFrameRequested {
					return
				}
			}
			t.Fatalf("%s never went idle: a mounted field is requesting frames every frame (flicker)", screen.name)
		})
	}
}
