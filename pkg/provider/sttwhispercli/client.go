package sttwhispercli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
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
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cli stt error: %w (stderr: %s)", err, provider.TruncateDetailString(stderr.String()))
	}
	return out.String(), nil
}
