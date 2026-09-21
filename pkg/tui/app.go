package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/engine"
)

type turnResultMsg struct {
	turn *engine.Turn
	err  error
}

type AppModel struct {
	orchestrator *engine.TurnOrchestrator
	history      []engine.Turn
	mode         string   // "Do", "Say", "Story", "Roll", "GM"
	modes        []string // cycle list
	inputBuffer  string
	modalContent string
	showModal    bool
	width        int
	height       int
	isWaiting    bool
	statusMsg    string
}

func NewAppModel(orchestrator *engine.TurnOrchestrator, width, height int) *AppModel {
	return &AppModel{
		orchestrator: orchestrator,
		history:      make([]engine.Turn, 0),
		mode:         "Do",
		modes:        []string{"Do", "Say", "Story", "Roll", "GM"},
		inputBuffer:  "",
		width:        width,
		height:       height,
		isWaiting:    false,
		statusMsg:    "Ready. (Tab to cycle modes, Esc to clear, Enter to submit)",
	}
}

func (m *AppModel) Init() tea.Cmd {
	return nil
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case turnResultMsg:
		m.isWaiting = false
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		if msg.turn != nil {
			m.history = append(m.history, *msg.turn)
			m.statusMsg = fmt.Sprintf("Turn %d completed.", msg.turn.Number)
		}
		return m, nil

	case tea.KeyMsg:
		if m.showModal {
			if msg.Type == tea.KeyEsc || msg.String() == "q" {
				m.showModal = false
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyTab:
			// Cycle modes
			for i, mode := range m.modes {
				if mode == m.mode {
					m.mode = m.modes[(i+1)%len(m.modes)]
					break
				}
			}
			return m, nil

		case tea.KeyEsc:
			m.inputBuffer = ""
			m.showModal = false
			return m, nil

		case tea.KeyBackspace:
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
			return m, nil

		case tea.KeyEnter:
			if strings.TrimSpace(m.inputBuffer) == "" || m.isWaiting {
				return m, nil
			}
			userInput := m.inputBuffer
			currentMode := m.mode
			m.inputBuffer = ""
			m.isWaiting = true
			m.statusMsg = "GM is thinking..."

			return m, func() tea.Cmd {
				return func() tea.Msg {
					turn, err := m.orchestrator.ProcessAction(context.Background(), currentMode, userInput)
					return turnResultMsg{turn: turn, err: err}
				}
			}()

		case tea.KeyRunes:
			m.inputBuffer += string(msg.Runes)
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) View() string {
	var sb strings.Builder

	// Header
	header := TitleStyle.Render(" LocalRPG Chronicles ")
	sb.WriteString(header + "\n\n")

	// Story chronicle history
	for _, turn := range m.history {
		sb.WriteString(PromptStyle.Render(fmt.Sprintf("[%s] %s", turn.Mode, turn.Input)) + "\n")
		renderedStory, err := RenderMarkdown(turn.Prose(), m.width-4)
		if err == nil {
			sb.WriteString(renderedStory + "\n")
		} else {
			sb.WriteString(turn.Prose() + "\n\n")
		}
	}

	if m.showModal {
		sb.WriteString(ModalStyle.Render(m.modalContent) + "\n")
	}

	// Bottom Status & Action Bar
	sb.WriteString(StatusStyle.Render(m.statusMsg) + "\n")
	modeTag := fmt.Sprintf("[%s]", m.mode)
	sb.WriteString(PromptStyle.Render(modeTag+" > ") + m.inputBuffer)

	return sb.String()
}
