package ttspiper

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

// cliTTSClient runs the piper binary, feeding the text on stdin.
type cliTTSClient struct {
	command string
	args    []string
}

// NewCLITTSClient builds the command-line TTS client.
func NewCLITTSClient(command string, args []string) media.TTSClient {
	return &cliTTSClient{command: command, args: args}
}

func (c *cliTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Stdin = bytes.NewBufferString(text)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli tts error: %w (stderr: %s)", err, provider.TruncateDetailString(stderr.String()))
	}
	return out.Bytes(), nil
}
