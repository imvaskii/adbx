package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// connectingModel is shown after a successful adb pair while we wait for
// ScanForConnect to locate the new connect-mode mDNS entry.
type connectingModel struct {
	spinner spinner.Model
}

func newConnectingModel() connectingModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return connectingModel{spinner: s}
}

func (m connectingModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m connectingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m connectingModel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render("adbx — Connecting"),
		"",
		successStyle.Render("✓  Paired successfully"),
		"",
		m.spinner.View()+" "+watchStyle.Render("Finding connect address…"),
		"",
		mutedStyle.Render("q  quit"),
	)
}
