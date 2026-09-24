package imagecli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/darkliquid/localrpg/pkg/media"
)

// NewCLIImageClient builds the command-line image client.
func NewCLIImageClient(command string, args []string) media.ImageClient {
	return &cliImageClient{command: command, args: args}
}

type cliImageClient struct {
	command string
	args    []string
}

func (c *cliImageClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, append(c.args, prompt)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli image error: %w", err)
	}
	return out.Bytes(), nil
}
