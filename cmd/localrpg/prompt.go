package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider/clillm"
	"github.com/darkliquid/localrpg/pkg/provider/openaichat"
)

func handlePromptCommand(args []string) {
	code := runPromptCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runPromptCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cliCmd := fs.String("cmd", "", "CLI command harness to use (e.g. echo, claude, agy)")
	httpEndpoint := fs.String("endpoint", "", "HTTP endpoint for Ollama / OpenAI")
	model := fs.String("model", "llama3", "Model name for HTTP provider")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	promptText := fs.Arg(0)
	if promptText == "" {
		fmt.Fprintln(stderr, "Usage: localrpg prompt [--cmd <command> | --endpoint <url>] <prompt text>")
		return 1
	}

	var provider harness.ModelProvider
	if *cliCmd != "" {
		provider = clillm.NewCLIProvider("cli-harness", *cliCmd, []string{})
	} else if *httpEndpoint != "" {
		provider = openaichat.NewHTTPProvider("http-harness", *httpEndpoint, *model, "")
	} else {
		provider = clillm.NewCLIProvider("default-echo", "echo", []string{})
	}

	ctx := context.Background()
	res, err := provider.Generate(ctx, harness.GenerateRequest{Prompt: promptText})
	if err != nil {
		fmt.Fprintf(stderr, "Prompt failed: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, res.Text)
	return 0
}
