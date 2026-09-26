package desktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// Injected audio/art operations; nil outside a live app.
var (
	playTurnAudio    func(ctx context.Context, svc *gui.Service, gameID string, turn int, force bool) error
	playSegmentAudio func(ctx context.Context, svc *gui.Service, gameID string, turn, index int, force bool) error
	stopAudioFn      func(svc *gui.Service)
	audioPlayingFn   func(svc *gui.Service) bool
	locationArt      func(ctx context.Context, svc *gui.Service, gameID, locationID string) (string, error)
)

// openCampaign switches to the chronicle and loads the campaign's turns, plus
// the portraits and location art they reference.
func openCampaign(gameID string) {
	appState.OpenGame = gameID
	appState.Screen = ScreenChronicle
	appState.Turns = []gui.TurnDTO{}
	appState.Prose = ""
	appState.AudioState = ""
	appState.PrologueDismissed = false
	svc := liveService
	if svc == nil {
		return
	}
	go func() {
		ctx := context.Background()
		turns := loadChronicle(ctx, svc, gameID)
		entities := loadEntities(ctx, svc, gameID)
		portraits := map[string]string{}
		art := map[string]string{}
		for i := range turns {
			for _, seg := range turns[i].Segments {
				if seg.SpeakerID == "" {
					continue
				}
				if _, ok := portraits[seg.SpeakerID]; ok {
					continue
				}
				if portraitPath != nil {
					if p, err := portraitPath(ctx, svc, gameID, seg.SpeakerID); err == nil {
						portraits[seg.SpeakerID] = p
					}
				}
			}
			if id := turns[i].LocationID; id != "" && locationArt != nil {
				if _, ok := art[id]; !ok {
					if p, err := locationArt(ctx, svc, gameID, id); err == nil {
						art[id] = p
					}
				}
			}
		}
		WithFrameLock(func() {
			appState.Turns = turns
			appState.Entities = entities
			appState.Portraits = portraits
			appState.SceneArt = art
		})
		RequestNextFrame()
	}()
}

func chronicleView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		Container(Attrs(Row, Expand, Grow(1), Clip), func() {
			Container(Attrs(Viewport, Grow(1)), func() {
				ScrollOnInput()
				ScrollBars()
				Container(Attrs(Expand, Pad(32), Gap(24)), func() {
					chronicleHeader()
					if len(appState.Turns) == 0 && !appState.PrologueDismissed {
						prologuePanel()
						return
					}
					if len(appState.Turns) == 0 {
						Label("The chronicle awaits your first action...", FontSize(14), TextColorVec(ui.TextFaint))
					}

					prevLocation := ""
					for i := range appState.Turns {
						turn := &appState.Turns[i]
						isSceneChange := turn.LocationID != "" && turn.LocationID != prevLocation
						prevLocation = turn.LocationID
						turnView(turn, isSceneChange, i == len(appState.Turns)-1)
					}
					if appState.TurnInFlight {
						inFlightView()
					}
				})
			})
		})
	})
	drawerPanel()
	actionConsole()
}

func chronicleHeader() {
	showLabels := GetContentWidth() > 1120
	Container(Attrs(Row, CrossMid, Gap(14), Expand, Corners(16), BackgroundVec(ui.PanelBG),
		BorderWidth(1), BorderColorVec(ui.Hairline), Pad2(10, 16), BoxShadow(22)), func() {
		Container(Attrs(Row, CrossMid, Gap(6), Corners(12), Pad2(6, 12), BackgroundVec(ui.HoverFill),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			if IsHovered() {
				ModAttrs(BackgroundVec(ui.AccentSoft))
			}
			NextAccessName("chronicle.home")
			if PressAction() {
				appState.Screen = ScreenLauncher
				appState.CampaignGalleryOpen = false
			}
			AssignAccess()
			Icon(TypCompass, FontSize(14), TextColorVec(ui.TextMain))
			Label("Campaigns", Fonts(ui.SansStack...), FontSize(12), TextColorVec(ui.TextMain))
		})
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Element(Attrs(FixSize(8, 8), Corners(4), BackgroundVec(ui.Accent), BoxShadow(8)))
			Label(appState.GameName(), Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		})
		Filler(1)
		drawerToolbar(ui.DefaultPalette(), showLabels)
		Container(Attrs(Row, CrossMid, Gap(6), Corners(12), Pad2(8, 10)), func() {
			if appState.Screen == ScreenTheater {
				ModAttrs(BackgroundVec(ui.AccentBtn))
			} else if IsHovered() {
				ModAttrs(BackgroundVec(ui.HoverFill))
			}
			NextAccessName("chronicle.theater")
			if PressAction() {
				openTheater()
			}
			AssignAccess()
			Icon(TypFilm, FontSize(14), TextColorVec(ui.TextMain))
			if showLabels {
				Label("Theater", Fonts(ui.SansStack...), FontSize(12), TextColorVec(ui.TextMain))
			}
		})
	})
}

func sceneArtPath(turn *gui.TurnDTO) string {
	if turn.LocationID == "" {
		return ""
	}
	return appState.SceneArt[turn.LocationID]
}

func hasTurnAudio(turn *gui.TurnDTO) bool {
	for _, seg := range turn.Segments {
		if seg.AudioURL != "" {
			return true
		}
	}
	return false
}

func turnView(turn *gui.TurnDTO, isSceneChange, isLast bool) {
	Container(Attrs(Expand, Gap(14)), func() {
		if turn.InputText != "" && !hasPlayerSegment(turn) {
			Container(Attrs(Row, Pad(14), Gap(12), Corners(12), BackgroundVec(ui.InputBG),
				BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
				Label("["+modeLabel(turn.Mode)+"]", Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.Accent))
				Label(turn.InputText, FontSize(14), FontStyle(StyleItalic), TextColorVec(ui.TextMuted))
				Filler(1)
				if turn.Outcome != "" {
					Label(turn.Outcome, Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextFaint))
				}
			})
		}

		if isSceneChange && sceneArtPath(turn) != "" {
			sceneArt(turn)
		}

		segmentsView(turn)

		if turn.Rejected {
			reason := ""
			if turn.Verdict != nil {
				reason = turn.Verdict.Reason
			}
			note("That action was impossible: "+reason, ui.Amber)
		}
		if turn.Recovery == "trimmed" {
			note("The narrator's reply ended mid-thought; the unfinished tail was dropped.", ui.Accent)
		}
		if len(turn.ToolCalls) > 0 {
			names := ""
			for i, call := range turn.ToolCalls {
				if i > 0 {
					names += ", "
				}
				names += fmt.Sprintf("%s (%d)", call.Name, call.ResultChars)
			}
			note("Looked up: "+names, ui.TextFaint)
		}
		if turn.Truncated {
			note("The narrator's reply could not be completed. Raise the response limit for the gm role in Settings, or check the provider.", ui.Accent)
		}
		if len(turn.ContextNotes) > 0 {
			note("Context trimmed to fit the prompt budget: "+strings.Join(turn.ContextNotes, ", ")+".", ui.TextFaint)
		}

		if len(turn.EntitiesHit) > 0 {
			Container(Attrs(Row, Wrap, Gap(8)), func() {
				for _, id := range turn.EntitiesHit {
					entityID := id
					NextAccessName("turn.entity." + entityID)
					if entityPill(entityID) {
						openEntity(entityID)
						appState.Drawer = "codex"
					}
					AssignAccess()
				}
			})
		}

		if hasTurnAudio(turn) {
			turnAudioControls(turn)
		}

		if !isLast {
			Element(Attrs(Expand, FixHeight(1), BackgroundVec(ui.Hairline)))
		}
	})
}

func sceneArt(turn *gui.TurnDTO) {
	path := sceneArtPath(turn)
	Container(Attrs(Expand, Corners(12), Clip, BorderWidth(1), BorderColorVec(ui.Hairline), BoxShadow(22)), func() {
		Container(Attrs(Expand, FixHeight(320), Clip), func() {
			if IsClicked() {
				appState.LightboxPath = path
			}
			coverImage("scene:"+turn.LocationID, path, GetContentWidth(), 320)
		})
		if turn.LocationName != "" {
			Container(Attrs(Expand, Pad2(10, 14), BackgroundVec(ui.InputBG)), func() {
				Label(turn.LocationName, Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.TextMuted))
			})
		}
	})
}

func segmentsView(turn *gui.TurnDTO) {
	checks := make(map[string]harness.CheckResult, len(turn.Checks))
	for _, check := range turn.Checks {
		checks[check.CheckID] = check
	}
	used := map[string]bool{}
	Container(Attrs(Expand, Gap(12)), func() {
		for i := range turn.Segments {
			seg := &turn.Segments[i]
			if seg.CheckRef != "" {
				if check, ok := checks[seg.CheckRef]; ok && !used[check.CheckID] {
					used[check.CheckID] = true
					checkCard(check)
				}
			}
			segmentView(turn, seg, i)
		}
		for _, check := range turn.Checks {
			if !used[check.CheckID] {
				checkCard(check)
			}
		}
	})
}

func segmentView(turn *gui.TurnDTO, seg *gui.SegmentDTO, index int) {
	if seg.Kind == "speech" {
		tone := ui.Accent
		if seg.Player {
			tone = ui.PlayerTone
		}
		Container(Attrs(Row, Expand, Clip, BackgroundVec(ui.CardBG), Corners(12)), func() {
			Element(Attrs(FixWidth(4), Expand, BackgroundVec(tone)))
			Container(Attrs(Grow(1), Pad(14), Gap(8)), func() {
				Container(Attrs(Row, CrossMid, Gap(12)), func() {
					if path := appState.Portraits[seg.SpeakerID]; path != "" {
						Container(Attrs(FixSize(36, 36), Corners(18), Clip, BorderWidth(2), BorderColorVec(tone)), func() {
							if IsClicked() {
								appState.LightboxPath = path
							}
							coverImage("portrait:"+seg.SpeakerID, path, 36, 36)
						})
					}
					Label(speakerLabel(seg), Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(tone))
					Filler(1)
					if seg.AudioURL != "" {
						NextAccessName(fmt.Sprintf("segment.play.%d.%d", turn.TurnNumber, index))
						if segmentPlayButton() {
							playSegment(turn.TurnNumber, index, false)
						}
						AssignAccess()
					}
				})
				proseBlocks("\u201c"+seg.Text+"\u201d", Fonts(ui.SerifStack...), FontSize(17), FontStyle(StyleItalic), TextColorVec(ui.TextMain))
			})
		})
		return
	}
	proseBlocks(seg.Text, Fonts(ui.SerifStack...), FontSize(19), TextColorVec(ui.TextMain))
}

func segmentPlayButton() bool {
	clicked := false
	Container(Attrs(FixSize(26, 26), Corners(13), Center, BackgroundVec(ui.RaisedBG), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		if PressAction() {
			clicked = true
		}
		Icon(SymPlay, FontSize(12), TextColorVec(ui.TextMain))
	})
	return clicked
}

func speakerLabel(seg *gui.SegmentDTO) string {
	name := seg.Speaker
	if name == "" {
		name = "UNKNOWN"
	}
	if seg.Player {
		name += "  (you)"
	}
	return name
}

func checkCard(check harness.CheckResult) {
	toneColor := outcomeColor(check.Outcome)
	notation := "check"
	total := 0
	successes := 0
	size := "?"
	rollCount := 1
	if check.Roll != nil {
		notation = check.Roll.Notation
		total = check.Roll.Total
		successes = check.Roll.Successes
		size = dieSize(check.Roll.Notation)
		if check.Roll.RollCount > 0 {
			rollCount = check.Roll.RollCount
		}
	}
	stakes := check.Stakes
	if stakes == "" {
		switch {
		case check.Actor != "" || check.Target != "":
			stakes = strings.TrimSpace(check.Actor + " vs " + check.Target)
		default:
			stakes = check.CheckKind
		}
	}

	Container(Attrs(Row, Expand, Clip, BackgroundVec(ui.InputBG), Corners(12)), func() {
		Element(Attrs(FixWidth(4), Expand, BackgroundVec(toneColor)))
		Container(Attrs(Grow(1), Pad(12), Gap(4)), func() {
			Container(Attrs(Row, Wrap, CrossMid, Gap(10)), func() {
				Container(Attrs(Row, Gap(4)), func() {
					shown := rollCount
					if shown > 6 {
						shown = 6
					}
					for d := 0; d < shown; d++ {
						dieCard(size)
					}
					if rollCount > shown {
						Label(fmt.Sprintf("+%d", rollCount-shown), Fonts(Monospace...), FontSize(10), TextColorVec(ui.TextFaint))
					}
				})
				Label(notation, Fonts(Monospace...), FontSize(12), TextColorVec(ui.TextMain))
				Label(fmt.Sprintf("→ %d", total), Fonts(Monospace...), FontSize(12), TextColorVec(ui.TextMuted))
				if successes > 0 {
					Label(fmt.Sprintf("%d successes", successes), Fonts(Monospace...), FontSize(12), TextColorVec(ui.TextMuted))
				}
				Label(strings.ToUpper(check.Outcome), Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(toneColor))
			})
			if stakes != "" {
				Label(stakes, FontSize(11), TextColorVec(ui.TextMuted))
			}
		})
	})
}

func dieCard(size string) {
	Container(Attrs(FixSize(20, 20), Corners(5), Center, BackgroundVec(ui.RaisedBG), BorderWidth(1), BorderColorVec(ui.TextFaint)), func() {
		Label(size, Fonts(Monospace...), FontSize(9), TextColorVec(ui.TextMain))
	})
}

func inFlightView() {
	Container(Attrs(Expand, Gap(14)), func() {
		if appState.PendingAction != "" {
			Container(Attrs(Row, Pad(14), Gap(12), Corners(12), BackgroundVec(ui.InputBG),
				BorderWidth(1), BorderColorVec(ui.AccentSoft)), func() {
				Label("["+modeLabel(consoleMode())+"]", Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.Accent))
				Label(appState.PendingAction, FontSize(14), FontStyle(StyleItalic), TextColorVec(ui.TextMuted))
			})
		}
		if appState.Prose != "" {
			proseBlocks(appState.Prose, Fonts(ui.SerifStack...), FontSize(19), TextColorVec(ui.TextMain))
			return
		}
		Container(Attrs(Row, CrossMid, Gap(12), Pad(16), Corners(12), BackgroundVec(ui.CardBG),
			BorderWidth(1), BorderColorVec(ui.AccentSoft)), func() {
			Icon(SymStar, FontSize(16), TextColorVec(ui.Accent))
			Container(Attrs(Gap(2)), func() {
				Label("THE NARRATOR IS DRAFTING THE SCENE...", Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.Accent))
				Label("Weaving your action into the chronicle.", FontSize(12), TextColorVec(ui.TextMuted))
			})
		})
	})
}

func turnAudioControls(turn *gui.TurnDTO) {
	playing := appState.AudioState == "playing" && appState.AudioTurn == turn.TurnNumber
	generating := appState.AudioState == "generating" && appState.AudioTurn == turn.TurnNumber
	Container(Attrs(Row, Wrap, CrossMid, Gap(8)), func() {
		if generating {
			Label("Generating speech…", FontSize(11), TextColorVec(ui.Accent))
			BusyDots()
		} else {
			NextAccessName(fmt.Sprintf("turn.play.%d", turn.TurnNumber))
			if audioChip("Play turn", SymPlay, ui.TextMuted) {
				playTurn(turn.TurnNumber, false)
			}
			AssignAccess()
		}
		if playing {
			NextAccessName(fmt.Sprintf("turn.stop.%d", turn.TurnNumber))
			if audioChip("Stop", TypMediaStop, ui.Danger) {
				stopTurnAudio()
			}
			AssignAccess()
		}
		if !generating && !playing {
			NextAccessName(fmt.Sprintf("turn.regen.%d", turn.TurnNumber))
			if audioChip("Regenerate", SymRefresh, ui.TextMuted) {
				playTurn(turn.TurnNumber, true)
			}
			AssignAccess()
		}
		if appState.AudioState == "error" && appState.AudioTurn == turn.TurnNumber && appState.AudioMessage != "" {
			Label("Error: "+appState.AudioMessage, FontSize(11), TextColorVec(ui.Danger))
		}
	})
}

func audioChip(label string, icon IconGlyph, colour Vec4) bool {
	clicked := false
	Container(Attrs(Row, CrossMid, Gap(6), Pad2(6, 12), Corners(10), BackgroundVec(ui.RaisedBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		if PressAction() {
			clicked = true
		}
		Icon(icon, FontSize(12), TextColorVec(colour))
		Label(label, Fonts(ui.SansStack...), FontSize(11), TextColorVec(colour))
	})
	return clicked
}

func playTurn(turnNumber int, force bool) {
	svc := liveService
	if playTurnAudio == nil || svc == nil {
		return
	}
	gameID := appState.OpenGame
	appState.AudioTurn = turnNumber
	appState.AudioState = "generating"
	appState.AudioMessage = ""
	RequestNextFrame()
	go func() {
		if err := playTurnAudio(context.Background(), svc, gameID, turnNumber, force); err != nil {
			WithFrameLock(func() {
				appState.AudioState = "error"
				appState.AudioMessage = err.Error()
			})
			RequestNextFrame()
			return
		}
		WithFrameLock(func() { appState.AudioState = "playing" })
		RequestNextFrame()
		for i := 0; i < 3000; i++ {
			time.Sleep(100 * time.Millisecond)
			if audioPlayingFn == nil || !audioPlayingFn(svc) {
				break
			}
		}
		WithFrameLock(func() { appState.AudioState = "idle" })
		RequestNextFrame()
	}()
}

func playSegment(turnNumber, index int, force bool) {
	svc := liveService
	if playSegmentAudio == nil || svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() { _ = playSegmentAudio(context.Background(), svc, gameID, turnNumber, index, force) }()
}

func stopTurnAudio() {
	svc := liveService
	if stopAudioFn != nil && svc != nil {
		stopAudioFn(svc)
	}
	appState.AudioState = "idle"
}

func note(text string, colour Vec4) {
	Label(text, Fonts(Monospace...), FontSize(11), TextColorVec(colour))
}

func entityPill(id string) bool {
	clicked := false
	Container(Attrs(Pad2(4, 12), Corners(999), BackgroundVec(ui.HoverFill), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		if PressAction() {
			clicked = true
		}
		Label(id, Fonts(ui.SansStack...), FontSize(12), TextColorVec(ui.TextMuted))
	})
	return clicked
}

func hasPlayerSegment(turn *gui.TurnDTO) bool {
	for _, seg := range turn.Segments {
		if seg.Player {
			return true
		}
	}
	return false
}

func modeLabel(mode string) string {
	if mode == "" {
		return "Action"
	}
	return mode
}

func outcomeColor(outcome string) Vec4 {
	value := strings.ToLower(outcome)
	switch {
	case strings.Contains(value, "partial"), strings.Contains(value, "mixed"),
		strings.Contains(value, "cost"), strings.Contains(value, "complication"):
		return ui.PartialTone
	case strings.Contains(value, "fail"):
		return ui.Danger
	case strings.Contains(value, "success"), strings.Contains(value, "pass"),
		strings.Contains(value, "critical"), strings.Contains(value, "succeed"):
		return ui.Success
	default:
		return ui.TextMuted
	}
}

func dieSize(notation string) string {
	lower := strings.ToLower(notation)
	if idx := strings.IndexByte(lower, 'd'); idx >= 0 {
		end := idx + 1
		for end < len(lower) && lower[end] >= '0' && lower[end] <= '9' {
			end++
		}
		if end > idx+1 {
			return lower[idx+1 : end]
		}
	}
	return "?"
}
