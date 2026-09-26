package desktop

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/ui"
)

// inspectTTS is injected by Run for a live service.
var inspectTTS func(ctx context.Context, svc *gui.Service, req gui.TTSInspectRequestDTO) (*gui.TTSInspectResponseDTO, error)

// inspectSig returns a stable signature of a TTS config.
func inspectSig(cfg config.TTSConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return cfg.Type + "|" + cfg.BuiltinName
	}
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}

// maybeInspectTTS re-inspects the TTS config only when it changed.
func maybeInspectTTS() {
	cfg := appState.Config
	svc := liveService
	if cfg == nil || svc == nil || inspectTTS == nil {
		return
	}
	sig := inspectSig(cfg.Media.TTS)
	if sig == appState.InspectSig {
		return
	}
	appState.InspectSig = sig
	ttsCfg := cfg.Media.TTS
	go func() {
		res, err := inspectTTS(context.Background(), svc, gui.TTSInspectRequestDTO{Config: ttsCfg})
		if err != nil {
			return
		}
		WithFrameLock(func() { appState.Inspect = res })
		RequestNextFrame()
	}()
}

// voiceOptionsControl renders provider-declared tunables.
func voiceOptionsControl(p ui.Palette, schema []media.VoiceOption, values map[string]any, onChange func(key string, value any)) {
	if values == nil {
		return
	}
	for _, opt := range schema {
		Label(opt.Label, FontSize(11), TextColorVec(p.Muted))
		switch opt.Kind {
		case "float", "int":
			v := float32(toFloat(values[opt.Key], opt.Default))
			Slider(&v, SliderAttrs{Min: float32(opt.Min), Max: float32(opt.Max), Step: 0.1, Width: 260})
			onChange(opt.Key, float64(v))
		case "bool":
			v := toBool(values[opt.Key], opt.Default)
			CheckBox(&v, opt.Label)
			onChange(opt.Key, v)
		case "enum":
			current := toString(values[opt.Key], opt.Default)
			MenuButton(NoIcon, current, func() {
				for _, choice := range opt.Options {
					choice := choice
					if MenuItem(NoIcon, choice) {
						onChange(opt.Key, choice)
					}
				}
			})
		default:
			s := toString(values[opt.Key], opt.Default)
			TextInput(&s)
			onChange(opt.Key, s)
		}
	}
}

func settingsMedia(p ui.Palette) {
	cfg := appState.Config
	maybeInspectTTS()

	Label("TTS Engine", FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("Engine", FontSize(12), TextColorVec(p.Muted))
	MenuButton(NoIcon, engineLabel(cfg.Media.TTS), func() {
		for _, desc := range providersForFamily(provider.FamilyTTS) {
			for _, preset := range desc.Presets {
				preset := preset
				if MenuItem(NoIcon, preset.Label) {
					applyTTSPreset(&cfg.Media.TTS, preset)
					appState.InspectSig = ""
				}
			}
		}
		MenuItem(NoIcon, "Disabled")
		if MenuItem(NoIcon, "Configure disabled") {
			cfg.Media.TTS.Type = "disabled"
		}
	})

	CheckBox(&cfg.Media.TTS.AutoPlay, "Auto-play narration")
	Label("Markdown handling", FontSize(12), TextColorVec(p.Muted))
	OptionGroup(&cfg.Media.TTS.Markdown, func() {
		OptionButton("Auto", "auto")
		OptionButton("Strip", "strip")
		OptionButton("Keep", "keep")
	})
	Label("Endpoint", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.TTS.Endpoint)
	Label("Model", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.TTS.Model)
	Label("Command", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.TTS.Command)
	Label("Built-in name", FontSize(12), TextColorVec(p.Muted))
	TextInput(&cfg.Media.TTS.BuiltinName)
	Label("API key", FontSize(12), TextColorVec(p.Muted))
	PasswordInput(&cfg.Media.TTS.APIKey)

	vol := float32(cfg.Media.TTS.MasterVolume)
	Label(fmt.Sprintf("Master volume %.2f", vol), FontSize(12), TextColorVec(p.Muted))
	Slider(&vol, SliderAttrs{Min: 0, Max: 1, Step: 0.05, Width: 260})
	cfg.Media.TTS.MasterVolume = float64(vol)

	if appState.Inspect != nil {
		if appState.Inspect.Error != "" {
			Label("Inspect: "+appState.Inspect.Error, FontSize(11), TextColorVec(p.Danger))
		}
		Label(fmt.Sprintf("provider: %s · metered: %v", appState.Inspect.ProviderKey, appState.Inspect.Metered),
			FontSize(11), TextColorVec(p.Muted))
		voiceOptionsControl(p, appState.Inspect.Options, cfg.Media.TTS.Options, func(key string, value any) {
			if cfg.Media.TTS.Options == nil {
				cfg.Media.TTS.Options = map[string]any{}
			}
			cfg.Media.TTS.Options[key] = value
		})
		if appState.Inspect.Catalog.Available && len(appState.Inspect.Catalog.Voices) > 0 {
			Label("Default voice", FontSize(12), TextColorVec(p.Muted))
			MenuButton(NoIcon, voiceLabel(cfg.Media.TTS.DefaultVoice), func() {
				for _, voice := range appState.Inspect.Catalog.Voices {
					voice := voice
					if MenuItem(NoIcon, voice.Name) {
						cfg.Media.TTS.DefaultVoice = voice.ID
					}
				}
			})
		}
	}

	CheckBox(&cfg.Media.TTS.SpeechCues.Enabled, "Speak speech cues")

	settingsSTT(p)
	settingsImage(p)
	modelsSection(p)
}

func engineLabel(cfg config.TTSConfig) string {
	if cfg.BuiltinName != "" {
		return cfg.Type + ":" + cfg.BuiltinName
	}
	if cfg.Type == "" {
		return "disabled"
	}
	return cfg.Type
}

func voiceLabel(id string) string {
	if id == "" {
		return "(default)"
	}
	return id
}

// applyTTSPreset copies a preset's config values into a TTS config.
func applyTTSPreset(cfg *config.TTSConfig, preset provider.Preset) {
	if preset.Config == nil {
		return
	}
	if v, ok := preset.Config["type"].(string); ok {
		cfg.Type = v
	}
	if v, ok := preset.Config["builtin_name"].(string); ok {
		cfg.BuiltinName = v
	}
	if v, ok := preset.Config["endpoint"].(string); ok {
		cfg.Endpoint = v
	}
	if v, ok := preset.Config["model"].(string); ok {
		cfg.Model = v
	}
	if v, ok := preset.Config["default_voice"].(string); ok {
		cfg.DefaultVoice = v
	}
}

func toFloat(v any, fallback any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	}
	switch n := fallback.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

func toBool(v any, fallback any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if b, ok := fallback.(bool); ok {
		return b
	}
	return false
}

func toString(v any, fallback any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if s, ok := fallback.(string); ok {
		return s
	}
	return ""
}
