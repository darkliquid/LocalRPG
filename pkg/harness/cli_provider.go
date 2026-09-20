package harness

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type CLIProvider struct {
	id      string
	command string
	args    []string
}

func NewCLIProvider(id, command string, args []string) *CLIProvider {
	return &CLIProvider{
		id:      id,
		command: command,
		args:    args,
	}
}

func (c *CLIProvider) ID() string {
	return c.id
}

func (c *CLIProvider) buildCmd(ctx context.Context, req GenerateRequest) *exec.Cmd {
	args := append([]string{}, c.args...)
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)
	if req.System != "" {
		cmd.Env = append(cmd.Environ(), "SYSTEM_PROMPT="+req.System)
	}
	return cmd
}

func (c *CLIProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	cmd := c.buildCmd(ctx, req)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cli provider %q failed: %w (stderr: %s)", c.id, err, stderr.String())
	}

	return &GenerateResponse{
		Text: strings.TrimSpace(stdout.String()),
	}, nil
}

func (c *CLIProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	cmd := c.buildCmd(ctx, req)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cli command: %w", err)
	}

	reader := bufio.NewReader(stdoutPipe)
	buf := make([]byte, 256)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			out <- StreamChunk{Text: string(buf[:n])}
		}
		if err != nil {
			if err != io.EOF {
				out <- StreamChunk{Error: err}
			}
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("cli process finished with error: %w (stderr: %s)", err, stderr.String())
	}

	out <- StreamChunk{Done: true}
	return nil
}
