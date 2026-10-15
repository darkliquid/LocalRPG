package tui

import (
	"fmt"

	"github.com/charmbracelet/glamour"
)

// RenderMarkdown renders a guide article or a line of narration as terminal text,
// wrapped to wordWrap columns.
func RenderMarkdown(content string, wordWrap int) (string, error) {
	if wordWrap <= 0 {
		wordWrap = 80
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		return "", fmt.Errorf("new term renderer: %w", err)
	}

	out, err := r.Render(content)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}

	return out, nil
}
