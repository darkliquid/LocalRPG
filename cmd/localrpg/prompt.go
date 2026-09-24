package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider/clillm"
	"github.com/darkliquid/localrpg/pkg/provider/openaichat"
)

func handlePromptCommand(args []string) {
	fs := flag.NewFlagSet("prompt", flag.ExitOnError)
	cliCmd := fs.String("cmd", "", "CLI command harness to use (e.g. echo, claude, agy)")
	httpEndpoint := fs.String("endpoint", "", "HTTP endpoint for Ollama / OpenAI")
	model := fs.String("model", "llama3", "Model name for HTTP provider")

	fs.Parse(args)
	promptText := fs.Arg(0)
	if promptText == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg prompt [--cmd <command> | --endpoint <url>] <prompt text>")
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "Prompt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(res.Text)
}
