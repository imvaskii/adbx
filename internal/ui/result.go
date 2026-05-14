package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imvaskii/adbx/internal/adb"
)

type resultModel struct {
	result adb.Result
}

func newResultModel(r adb.Result) resultModel {
	return resultModel{result: r}
}

func (m resultModel) Init() tea.Cmd { return nil }

func (m resultModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "r" {
			return m, func() tea.Msg { return rescanMsg{} }
		}
	}
	return m, nil
}

func (m resultModel) View() string {
	var statusLine string
	if m.result.Success {
		statusLine = successStyle.Render("✓  Success")
	} else {
		statusLine = failStyle.Render("✗  Failed")
	}

	rows := []string{
		headerStyle.Render("adbx — Result"),
		"",
		statusLine,
		"",
		msgStyle.Render(m.result.Message),
		"",
		mutedStyle.Render("r  scan again  •  q  quit"),
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
