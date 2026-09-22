package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestStderrLoggerPrintsOneLinePerEvent(t *testing.T) {
	var out bytes.Buffer
	logger := &trace.StderrLogger{}
	logger.SetWriter(&out)
	logger.SetLevel(trace.LevelSummary)
	logger.SetGame("test-campaign")

	logger.Event("turn.begin", map[string]interface{}{"number": 2, "prompt": "not at summary"})

	line := out.String()
	if !strings.Contains(line, "trace turn.begin") {
		t.Errorf("expected a named trace line, got %q", line)
	}
	if !strings.Contains(line, "test-campaign") {
		t.Errorf("expected the campaign stamped, got %q", line)
	}
	if strings.Contains(line, "not at summary") {
		t.Errorf("summary level must not print payloads, got %q", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("expected exactly one line, got %q", line)
	}
}

func TestStderrLoggerIsSilentWhenOff(t *testing.T) {
	var out bytes.Buffer
	logger := &trace.StderrLogger{}
	logger.SetWriter(&out)
	logger.SetLevel(trace.LevelOff)

	logger.Event("turn.begin", map[string]interface{}{"number": 1})

	if out.Len() != 0 {
		t.Errorf("expected no output at level off, got %q", out.String())
	}
}

func TestTraceFlagDefaultsToFullWhenBare(t *testing.T) {
	cases := []struct {
		args       []string
		configured string
		want       string
		wantSet    bool
	}{
		{args: []string{"gui"}, configured: "off", want: "off", wantSet: false},
		{args: []string{"gui", "--trace"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--trace", "summary"}, configured: "off", want: "summary", wantSet: true},
		{args: []string{"gui", "--trace=full"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--trace", "--headless"}, configured: "off", want: "full", wantSet: true},
		{args: []string{"gui", "--port", "8080"}, configured: "summary", want: "summary", wantSet: false},
	}

	for _, testCase := range cases {
		got, set := traceFlagLevel(testCase.args, testCase.configured)
		if got != testCase.want || set != testCase.wantSet {
			t.Errorf("traceFlagLevel(%v, %q) = %q, %v; want %q, %v",
				testCase.args, testCase.configured, got, set, testCase.want, testCase.wantSet)
		}
	}
}
