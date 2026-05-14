package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type scanModel struct {
	spinner spinner.Model
}

func newScanModel() scanModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return scanModel{spinner: s}
}

func (m scanModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m scanModel) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("adbx — Android Wireless Debug"),
		"",
		m.spinner.View()+" Scanning for Android devices on the network…",
		"",
		mutedStyle.Render("Waiting up to 4 seconds for mDNS responses"),
	)
}
