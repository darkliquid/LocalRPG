package main

import (
	"flag"
	"fmt"
	"os"
)

func handleTTSCommand(args []string) {
	fs := flag.NewFlagSet("tts", flag.ExitOnError)
	speaker := fs.String("speaker", "narrator", "Speaker or character name")
	fs.Parse(args)

	text := fs.Arg(0)
	if text == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg tts [--speaker <name>] <text to speak>")
		os.Exit(1)
	}

	fmt.Printf("Synthesizing TTS: %s (speaker: %s)\n", text, *speaker)
}

func handleImageCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: localrpg image <prompt>")
		os.Exit(1)
	}

	prompt := args[0]
	fmt.Printf("Generating image: %s\n", prompt)
}
