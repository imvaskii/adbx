package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imvaskii/adbx/internal/discovery"
)

type pairingModel struct {
	device     *discovery.Device
	input      textinput.Model
	spinner    spinner.Model
	submitted  bool
	submitting bool // true while waiting for adb pair response — blocks input
	cancelled  bool
	err        string
}

func newPairingModel(dev *discovery.Device) pairingModel {
	ti := textinput.New()
	ti.Placeholder = "000000"
	ti.CharLimit = 6
	ti.Width = 10
	ti.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return pairingModel{device: dev, input: ti, spinner: s}
}

// retry resets the model for another attempt, keeping the same device but
// clearing the input and showing an inline error hint.
func (m pairingModel) retry(errMsg string) pairingModel {
	m.input.SetValue("")
	m.submitted = false
	m.submitting = false
	m.err = errMsg
	return m
}

func (m pairingModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pairingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While waiting for the adb response, ignore all keypresses.
	if m.submitting {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			c := m.code()
			if len(c) != 6 {
				m.err = "Pairing code must be exactly 6 digits"
				return m, nil
			}
			m.err = ""
			m.submitted = true
			m.submitting = true
			m.input.Blur()
			return m, m.spinner.Tick
		case "esc":
			m.cancelled = true
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m pairingModel) code() string {
	return strings.TrimSpace(m.input.Value())
}

func (m pairingModel) View() string {
	addr := addrStr(m.device.IP, m.device.Host, m.device.Port)

	rows := []string{
		headerStyle.Render("adbx — Pair Device"),
		"",
		inputLabelStyle.Render(fmt.Sprintf("Device:  %s", addr)),
		inputLabelStyle.Render(fmt.Sprintf("Host:    %s", m.device.Name)),
		"",
		inputLabelStyle.Render("Enter the 6-digit pairing code shown on your device:"),
		"",
		inputBoxStyle.Render(m.input.View()),
		"",
	}

	if m.submitting {
		rows = append(rows, m.spinner.View()+"  "+watchStyle.Render("Pairing…"), "")
	} else if m.err != "" {
		rows = append(rows, errorStyle.Render("✗  "+m.err), "")
	}

	if !m.submitting {
		rows = append(rows, mutedStyle.Render("enter  confirm  •  esc  back"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
