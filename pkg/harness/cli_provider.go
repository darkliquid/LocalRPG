package harness

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/trace"
)

type CLIProvider struct {
	id      string
	command string
	args    []string
	opts    GenerationOptions
	logger  trace.Logger
}

// NewCLIProviderWithLogger is NewCLIProvider with a trace sink.
func NewCLIProviderWithLogger(id, command string, args []string, opts GenerationOptions, logger trace.Logger) *CLIProvider {
	provider := NewCLIProviderWithOptions(id, command, args, opts)
	provider.SetLogger(logger)
	return provider
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *CLIProvider) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

func NewCLIProvider(id, command string, args []string) *CLIProvider {
	return NewCLIProviderWithOptions(id, command, args, GenerationOptions{})
}

func NewCLIProviderWithOptions(id, command string, args []string, opts GenerationOptions) *CLIProvider {
	return &CLIProvider{
		id:      id,
		command: command,
		args:    args,
		opts:    opts,
	}
}

func (c *CLIProvider) ID() string {
	return c.id
}

func (c *CLIProvider) buildCmd(ctx context.Context, req GenerateRequest) *exec.Cmd {
	args := append([]string{}, c.args...)
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)

	env := cmd.Environ()
	if req.System != "" {
		env = append(env, "SYSTEM_PROMPT="+req.System)
	}
	if c.opts.MaxTokens > 0 {
		env = append(env, fmt.Sprintf("LOCALRPG_MAX_TOKENS=%d", c.opts.MaxTokens))
	}
	if c.opts.Temperature > 0 {
		env = append(env, fmt.Sprintf("LOCALRPG_TEMPERATURE=%g", c.opts.Temperature))
	}
	cmd.Env = env

	return cmd
}

func (c *CLIProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	c.logger = trace.OrNil(c.logger)
	c.logRequest(req, "generate")
	start := time.Now()

	cmd := c.buildCmd(ctx, req)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.logger.Event("provider.error", map[string]interface{}{"role": c.id, "error": err.Error()})
		return nil, fmt.Errorf("cli provider %q failed: %w (stderr: %s)", c.id, err, stderr.String())
	}

	text := strings.TrimSpace(stdout.String())
	c.logger.Event("provider.response", map[string]interface{}{
		"role":         c.id,
		"exit_code":    0,
		"stdout_chars": len([]rune(text)),
		"total_ms":     time.Since(start).Milliseconds(),
	})

	return &GenerateResponse{Text: text}, nil
}

func (c *CLIProvider) Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error {
	defer close(out)

	c.logger = trace.OrNil(c.logger)
	c.logRequest(req, "stream")
	start := time.Now()

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
		c.logger.Event("provider.error", map[string]interface{}{"role": c.id, "error": err.Error()})
		return fmt.Errorf("cli process finished with error: %w (stderr: %s)", err, stderr.String())
	}

	c.logger.Event("provider.response", map[string]interface{}{
		"role":          c.id,
		"exit_code":     0,
		"finish_reason": "stop",
		"total_ms":      time.Since(start).Milliseconds(),
	})

	out <- StreamChunk{Done: true, FinishReason: "stop"}
	return nil
}

// logRequest records the command without its final argument: that argument is the
// prompt, which is recorded once on context.assembled.
func (c *CLIProvider) logRequest(req GenerateRequest, call string) {
	c.logger.Event("provider.request", map[string]interface{}{
		"role":         c.id,
		"kind":         "cli",
		"call":         call,
		"command":      c.command,
		"arg_count":    len(c.args) + 1,
		"system_set":   req.System != "",
		"prompt_chars": len([]rune(req.Prompt)),
	})
}
