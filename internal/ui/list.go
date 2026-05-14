package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imvaskii/adbx/internal/discovery"
)

// deviceChosenMsg is emitted when the user selects a device from the list.
// It replaces the chosen flag side-channel.
type deviceChosenMsg struct{ device *discovery.Device }

// rescanMsg is emitted when the user presses 'r' on the list or result screen.
// It replaces the rescan flag side-channel.
type rescanMsg struct{}

type listModel struct {
	devices []discovery.Device
	cursor  int
	width   int
	height  int
	lastG   bool
	scanErr error // non-nil when the last scan failed
}

func newListModel(devices []discovery.Device, w, h int) listModel {
	return listModel{devices: devices, width: w, height: h}
}

func (m listModel) Init() tea.Cmd { return nil }

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		if key == "g" {
			if m.lastG {
				m.cursor = 0
				m.lastG = false
				return m, nil
			}
			m.lastG = true
			return m, nil
		}
		m.lastG = false

		switch key {
		case "j", "down":
			if m.cursor < len(m.devices)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "G":
			if len(m.devices) > 0 {
				m.cursor = len(m.devices) - 1
			}
		case "r":
			return m, func() tea.Msg { return rescanMsg{} }
		case "enter":
			if len(m.devices) > 0 {
				d := m.devices[m.cursor]
				return m, func() tea.Msg { return deviceChosenMsg{device: &d} }
			}
		}
	}
	return m, nil
}

func (m listModel) View() string {
	rows := []string{
		headerStyle.Render("adbx — Android Wireless Debug"),
		"",
	}

	if m.scanErr != nil {
		rows = append(rows,
			errorStyle.Render("Scan failed: "+m.scanErr.Error()),
			emptyStyle.Render("Press r to try again."),
		)
	} else if len(m.devices) == 0 {
		rows = append(rows,
			emptyStyle.Render("No Android devices found on the network."),
			emptyStyle.Render("Make sure Wireless Debugging is enabled on your device."),
		)
	} else {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("Found %d device(s)", len(m.devices))), "")
		for i, dev := range m.devices {
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("▶ ")
			}

			badge := connectBadge.Render("[connect]")
			if dev.Type == discovery.DevicePairing {
				badge = pairingBadge.Render("[pairing]")
			}

			addr := addrStr(dev.IP, dev.Host, dev.Port)
			info := fmt.Sprintf("%-20s  %s", addr, dev.Name)

			var row string
			if i == m.cursor {
				row = cursor + badge + "  " + selectedStyle.Render(info)
			} else {
				row = cursor + badge + "  " + normalStyle.Render(info)
			}
			rows = append(rows, row)
		}
	}

	rows = append(rows, "", mutedStyle.Render("j/k  navigate  •  enter  select  •  r  rescan  •  q  quit"))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
