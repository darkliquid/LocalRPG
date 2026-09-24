package sttwhispercli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/darkliquid/localrpg/pkg/media"
)

// NewCLISTTClient builds the command-line transcription client.
func NewCLISTTClient(command string, args []string) media.STTClient {
	return &cliSTTClient{command: command, args: args}
}

type cliSTTClient struct {
	command string
	args    []string
}

func (c *cliSTTClient) Transcribe(ctx context.Context, audioData []byte) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewReader(audioData)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli stt error: %w", err)
	}
	return out.String(), nil
}
