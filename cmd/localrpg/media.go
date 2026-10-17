package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/paths"
)

// handleTTSCommand synthesizes an utterance to speech and outputs the resulting audio file path.
func handleTTSCommand(args []string) {
	code := runTTSCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

// runTTSCommand executes the tts command or delegates to tts batch.
func runTTSCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "batch" {
		return runTTSBatchCommand(args[1:], stdout, stderr)
	}

	fs := flag.NewFlagSet("tts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	voice := fs.String("voice", "", "Voice ID override")
	pitch := fs.Float64("pitch", 0, "Pitch override")
	rate := fs.Float64("rate", 0, "Speech rate override")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		fmt.Fprintln(stderr, "Usage: localrpg tts [--voice id] [--pitch n] [--rate n] <text>")
		return 1
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	client, err := media.NewTTSClientWithSharedKey(cfg.Media.TTS, cfg.Providers.Gemini.APIKey)
	if err != nil {
		fmt.Fprintf(stderr, "Error building TTS client: %v\n", err)
		return 1
	}

	voiceCfg := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
		Options:    cfg.Media.TTS.Options,
	}
	if *voice != "" {
		voiceCfg.VoiceID = *voice
	}
	if *pitch != 0 {
		voiceCfg.Pitch = *pitch
	}
	if *rate != 0 {
		voiceCfg.SpeechRate = *rate
	}

	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	pipeline := media.NewTTSPipeline(client, media.NewContentCache(dirs.Cache))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())
	path, err := pipeline.SynthesizeUtterance(context.Background(), "cli", voiceCfg, text)
	if err != nil {
		fmt.Fprintf(stderr, "Error synthesizing: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, path)
	return 0
}

// handleImageCommand generates a scene or character image from a text prompt.
func handleImageCommand(args []string) {
	code := runImageCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runImageCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: localrpg image <prompt>")
		return 1
	}

	prompt := args[0]
	fmt.Fprintf(stdout, "Generating image: %s\n", prompt)
	return 0
}
