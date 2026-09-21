package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func handleTTSCommand(args []string) {
	fs := flag.NewFlagSet("tts", flag.ContinueOnError)
	voice := fs.String("voice", "", "Voice ID override")
	pitch := fs.Float64("pitch", 0, "Pitch override")
	rate := fs.Float64("rate", 0, "Speech rate override")
	if err := fs.Parse(args); err != nil {
		return
	}

	text := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if text == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg tts [--voice id] [--pitch n] [--rate n] <text>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	client, err := media.NewTTSClient(cfg.Media.TTS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building TTS client: %v\n", err)
		os.Exit(1)
	}

	voiceCfg := &entity.VoiceConfig{
		VoiceID:    cfg.Media.TTS.DefaultVoice,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
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

	pipeline := media.NewTTSPipeline(client, media.NewContentCache(cfg.Paths.Cache))
	path, err := pipeline.SynthesizeUtterance(context.Background(), "cli", voiceCfg, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error synthesizing: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(path)
}

func handleImageCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg image <prompt>")
		os.Exit(1)
	}

	prompt := args[0]
	fmt.Printf("Generating image: %s\n", prompt)
}
