package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// traceFlagLevel reads --trace from the argument list without registering it with
// flag, so a bare --trace can mean "full" rather than swallowing the next
// argument. The returned bool reports whether the flag was present at all.
func traceFlagLevel(args []string, configured string) (string, bool) {
	level := configured
	seen := false

	for index, arg := range args {
		if arg == "--trace" {
			seen = true
			level = "full"
			if index+1 < len(args) {
				next := args[index+1]
				if !strings.HasPrefix(next, "-") {
					if _, err := trace.ParseLevel(next); err == nil {
						level = next
					}
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "--trace=") {
			seen = true
			level = strings.TrimPrefix(arg, "--trace=")
		}
	}
	return level, seen
}

// buildTraceLogger creates the sink a command should write to, under the cache
// directory the resolver owns. It returns a no-op logger when tracing is off, and
// never fails the command: a trace that cannot be opened is reported and dropped.
func buildTraceLogger(cfg *config.Config, args []string, cacheDir string) trace.Logger {
	if cfg == nil {
		return trace.Nop()
	}

	levelName, explicit := traceFlagLevel(args, cfg.TraceLevel())
	if !explicit && levelName == "off" {
		return trace.Nop()
	}

	level, err := trace.ParseLevel(levelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unknown trace level %q, tracing is off\n", levelName)
		return trace.Nop()
	}
	if level == trace.LevelOff {
		return trace.Nop()
	}

	sink, err := trace.NewFileSink(filepath.Join(cacheDir, "trace", "trace.jsonl"), trace.FileOptions{
		Level:        level,
		MaxBytes:     cfg.TraceMaxBytes(),
		MaxFiles:     cfg.TraceMaxFiles(),
		RotateCheck:  cfg.TraceRotateCheck(),
		PayloadChars: cfg.TracePayloadChars(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open the trace file: %v\n", err)
		return trace.Nop()
	}

	fmt.Fprintf(os.Stderr, "Tracing at level %q to %s\n", level.String(), sink.Path())
	if explicit {
		return trace.Multi(sink, trace.NewStderr(level))
	}
	return sink
}
