// Package main provides the Terminal User Interface for the Market Intelligence Aggregator.
// Built with Bubble Tea and Lip Gloss for a beautiful, interactive experience.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// model represents the TUI application state
type model struct {
	ready bool
}

func initialModel() model {
	return model{
		ready: false,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.ready = true
	}
	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	return `
  Market Intelligence Aggregator - TUI
  =====================================
  
  [TUI Implementation pending - Phase 4]
  
  Press 'q' to quit.
`
}
