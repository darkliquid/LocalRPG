package desktop

import (
	"context"
	"fmt"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// refreshInspectTTS re-inspects the TTS config, forcing the provider to
// re-enumerate its voice catalogue.
func refreshInspectTTS() {
	cfg := appState.Config
	svc := liveService
	if cfg == nil || svc == nil || inspectTTS == nil {
		return
	}
	ttsCfg := cfg.Media.TTS
	go func() {
		res, err := inspectTTS(context.Background(), svc, gui.TTSInspectRequestDTO{Config: ttsCfg, Refresh: true})
		if err != nil {
			WithFrameLock(func() { appState.VoiceError = err.Error() })
			RequestNextFrame()
			return
		}
		WithFrameLock(func() {
			appState.Inspect = res
			appState.VoiceError = res.Error
		})
		RequestNextFrame()
	}()
}

// searchVoicesNow searches the provider's extended voice library.
func searchVoicesNow() {
	cfg := appState.Config
	svc := liveService
	if cfg == nil || svc == nil || searchTTSVoices == nil {
		return
	}
	cfgCopy := cfg.Media.TTS
	query := appState.VoiceQuery
	WithFrameLock(func() { appState.VoiceSearching = true })
	go func() {
		res, err := searchTTSVoices(context.Background(), svc, gui.VoiceSearchRequestDTO{Config: cfgCopy, Query: query})
		WithFrameLock(func() {
			appState.VoiceSearching = false
			if err != nil {
				appState.VoiceError = err.Error()
				appState.VoiceResults = nil
				return
			}
			appState.VoiceError = res.Error
			appState.VoiceResults = res.Voices
		})
		RequestNextFrame()
	}()
}

// fetchModelsNow lists the models a Gemini key can reach.
func fetchModelsNow() {
	svc := liveService
	if svc == nil || listModels == nil {
		return
	}
	key := ""
	if appState.Config != nil {
		key = appState.Config.Providers.Gemini.APIKey
	}
	WithFrameLock(func() { appState.FetchingModels = true })
	go func() {
		res, err := listModels(context.Background(), svc, gui.ModelCatalogueRequestDTO{APIKey: key})
		WithFrameLock(func() {
			appState.FetchingModels = false
			if err != nil {
				appState.ModelListErr = err.Error()
				return
			}
			appState.ModelList = res.Models
			appState.ModelListErr = res.Error
		})
		RequestNextFrame()
	}()
}

// settingsSmallButton is a compact secondary action used beside section heads.
func settingsSmallButton(name, label string, action func()) {
	Container(Attrs(Row, CrossMid, Gap(6), Corners(10), Pad2(5, 10), BackgroundVec(ui.PillBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Label(label, Fonts(ui.SansStack...), FontSize(11), TextColorVec(ui.TextMain))
	})
}

// runProviderTest probes one media/LLM provider and records the result.
func runProviderTest(category string, providerCfg any) {
	svc := liveService
	if testProvider == nil || svc == nil {
		return
	}
	go func() {
		res, err := testProvider(context.Background(), svc, gui.TestProviderRequestDTO{
			Category: category, Provider: providerCfg,
		})
		if err != nil {
			res = &gui.TestProviderResponseDTO{Message: err.Error()}
		}
		WithFrameLock(func() { appState.TestResult = res })
		RequestNextFrame()
	}()
}

// providerTestRow is a test action with its latest result beside it.
func providerTestRow(key, label, category string, providerCfg any) {
	Container(Attrs(Row, CrossMid, Gap(10), Wrap), func() {
		settingsSmallButton(key, label, func() { runProviderTest(category, providerCfg) })
		if appState.TestResult == nil {
			return
		}
		colour := ui.Success
		msg := appState.TestResult.Message
		if !appState.TestResult.Success {
			colour = ui.Danger
		} else if appState.TestResult.LatencyMS > 0 {
			msg = fmt.Sprintf("%s (%dms)", msg, appState.TestResult.LatencyMS)
		}
		Label(msg, FontSize(11), Fonts(Monospace...), TextColorVec(colour))
	})
}

// voiceMatches reports whether a voice satisfies the current search query.
func voiceMatches(v media.ProviderVoice, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	fields := []string{v.ID, v.Name, v.Language, v.Gender, v.Accent, v.Description}
	fields = append(fields, v.Tags...)
	fields = append(fields, v.Categories...)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// voiceCatalogueSection browses the provider's voice catalogue and picks the
// default voice, with an optional extended-library search.
func voiceCatalogueSection(p ui.Palette) {
	cfg := appState.Config
	settingsSubTitle(TypVolumeUp, "Voice Catalogue")

	if appState.Inspect != nil {
		cat := appState.Inspect.Catalog
		status := "no catalogue available"
		if cat.Available {
			status = fmt.Sprintf("%d voices", len(cat.Voices))
			if cat.Stale {
				status += " (stale; showing the last copy)"
			}
			if !cat.FetchedAt.IsZero() {
				status += " · checked " + cat.FetchedAt.Format("15:04")
			}
		}
		Container(Attrs(Row, CrossMid, Gap(8), Expand), func() {
			Label(status, Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
			Filler(1)
			settingsSmallButton("settings.voices.refresh", "Refresh Catalogue", refreshInspectTTS)
		})
	}
	if appState.VoiceError != "" {
		Label(appState.VoiceError, FontSize(11), TextColorVec(ui.Danger))
	}

	settingsHint("Fallback voice used for turn narration and unvoiced characters.")
	studioFieldLabel("Current default voice")
	Label(voiceLabel(cfg.Media.TTS.DefaultVoice), Fonts(Monospace...), FontSize(12), TextColorVec(ui.Accent))

	settingsSmallButton("settings.voices.search", "Search Provider Library", searchVoicesNow)

	voices := catalogueVoices()
	if len(voices) == 0 {
		settingsHint("No voices reported by the active provider.")
		return
	}
	studioFieldLabel("Search catalogue")
	TextInputExt(&appState.VoiceQuery, TextInputAttrs{Placeholder: "name, accent, gender…", NoAutoFocus: true})

	shown := 0
	Container(Attrs(Row, Wrap, Gap(6)), func() {
		for _, v := range voices {
			if shown >= 48 {
				return
			}
			if !voiceMatches(v, appState.VoiceQuery) {
				continue
			}
			shown++
			voice := v
			selected := voice.ID == cfg.Media.TTS.DefaultVoice
			studioPill("settings.voice."+voice.ID, voice.Name, selected, func() {
				cfg.Media.TTS.DefaultVoice = voice.ID
			})
		}
	})
	if shown == 0 {
		settingsHint("No catalogue voices match that query.")
	}
}

// catalogueVoices merges the inspected catalogue with any extended search
// results, de-duplicated by ID.
func catalogueVoices() []media.ProviderVoice {
	seen := map[string]bool{}
	var out []media.ProviderVoice
	add := func(v media.ProviderVoice) {
		if v.ID == "" || seen[v.ID] {
			return
		}
		seen[v.ID] = true
		out = append(out, v)
	}
	if appState.Inspect != nil {
		for _, v := range appState.Inspect.Catalog.Voices {
			add(v)
		}
	}
	for _, v := range appState.VoiceResults {
		add(v)
	}
	return out
}

// voiceProfilesSection manages the NPC voice-profile library: a list of
// authored archetypes the GM matches characters against.
func voiceProfilesSection(p ui.Palette) {
	cfg := appState.Config
	profiles := &cfg.Media.TTS.VoiceProfiles
	settingsSubTitle(TypUser, "NPC Voice Profiles Library")
	settingsHint("The GM and world extractor match character descriptions against these archetypes and tags to assign speech parameters automatically.")

	Container(Attrs(Row, Wrap, CrossMid, Gap(8)), func() {
		Label(fmt.Sprintf("%d archetypes", len(*profiles)), Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
		Filler(1)
		if isSherpaOrKokoro(cfg) {
			settingsSmallButton("settings.profiles.kokoro", "Load Kokoro Voices", func() {
				*profiles = append([]config.VoiceProfile(nil), config.KokoroVoiceProfiles...)
			})
		}
		settingsSmallButton("settings.profiles.defaults", "Load Fantasy Defaults", func() {
			*profiles = append([]config.VoiceProfile(nil), config.DefaultConfig().Media.TTS.VoiceProfiles...)
		})
		settingsSmallButton("settings.profiles.add", "Add Profile", func() {
			*profiles = append(*profiles, config.VoiceProfile{
				ID:          fmt.Sprintf("npc_voice_%d", len(*profiles)+1),
				Name:        "New Archetype",
				VoiceID:     cfg.Media.TTS.DefaultVoice,
				Pitch:       1.0,
				SpeechRate:  1.0,
				Tags:        []string{"npc"},
				Description: "Distinctive voice description for automatic GM matching.",
			})
		})
	})

	remove := -1
	for i := range *profiles {
		profile := &(*profiles)[i]
		Container(Attrs(Expand, Gap(8), Corners(10), Pad(12), BackgroundVec(ui.PillBG),
			BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			Container(Attrs(Row, CrossMid, Gap(8)), func() {
				Label(profile.Name, Fonts(ui.SansStack...), FontSize(12), FontWeight(WeightBold), TextColorVec(ui.TextMain))
				Filler(1)
				settingsSmallButton("settings.profiles.remove."+profile.ID, "Remove", func() { remove = i })
			})
			studioFieldLabel("ID")
			FieldInput(&profile.ID)
			studioFieldLabel("Name")
			FieldInput(&profile.Name)
			studioFieldLabel("Voice ID")
			FieldInput(&profile.VoiceID)

			pitch := float32(profile.Pitch)
			Label(fmt.Sprintf("Pitch %.2f", pitch), FontSize(11), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
			Slider(&pitch, SliderAttrs{Min: 0.5, Max: 1.5, Step: 0.01, Width: 260})
			profile.Pitch = float64(pitch)

			rate := float32(profile.SpeechRate)
			Label(fmt.Sprintf("Speech rate %.2f", rate), FontSize(11), Fonts(ui.SansStack...), TextColorVec(ui.TextMuted))
			Slider(&rate, SliderAttrs{Min: 0.5, Max: 2.0, Step: 0.01, Width: 260})
			profile.SpeechRate = float64(rate)

			studioFieldLabel("Tags (comma-separated)")
			if appState.ProfileTagsEdit == nil {
				appState.ProfileTagsEdit = map[string]string{}
			}
			raw, ok := appState.ProfileTagsEdit[profile.ID]
			if !ok {
				raw = strings.Join(profile.Tags, ", ")
			}
			FieldInput(&raw)
			appState.ProfileTagsEdit[profile.ID] = raw
			profile.Tags = splitTags(raw)
			studioFieldLabel("Description")
			FieldArea(&profile.Description)
		})
	}
	if remove >= 0 {
		*profiles = append((*profiles)[:remove], (*profiles)[remove+1:]...)
	}

	if len(*profiles) == 0 {
		settingsHint("No voice profiles yet. Load the defaults or add an archetype.")
	}
}

// splitTags normalises a comma-separated tag list.
func splitTags(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// isSherpaOrKokoro reports whether the active TTS engine is one of the built-in
// Kokoro-family voices that ships a matching profile set.
func isSherpaOrKokoro(cfg *config.Config) bool {
	tts := cfg.Media.TTS
	return tts.Type == "builtin" && (tts.BuiltinName == "sherpa-onnx" || tts.BuiltinName == "kokoro")
}

// speechCueCapabilities explains what the active provider can do with vocal cues.
func speechCueCapabilities() {
	if appState.Inspect == nil {
		return
	}
	caps := appState.Inspect.SpeechCues
	switch {
	case caps.AudioTags:
		preview := ""
		if len(caps.SupportedTags) > 0 {
			preview = " (" + strings.Join(prefixTags(caps.SupportedTags, 5), ", ") + ")"
		}
		settingsHint("Provider supports bracketed vocal cues" + preview + ".")
	case caps.MarkdownEmphasis:
		settingsHint("Provider supports Markdown emphasis for delivery.")
	default:
		settingsHint("Plain text only; vocal tags are stripped before synthesis.")
	}
}

// prefixTags formats bracketed tags, capped at max.
func prefixTags(tags []string, max int) []string {
	out := make([]string, 0, max)
	for i, tag := range tags {
		if i >= max {
			break
		}
		out = append(out, "["+tag+"]")
	}
	return out
}
