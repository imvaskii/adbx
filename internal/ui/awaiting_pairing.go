package ui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imvaskii/adbx/internal/discovery"
)

type awaitingPairingModel struct {
	connectDevice *discovery.Device
	watching      bool
	spinner       spinner.Model
	cancelled     bool
	// cancelWatch is set when watchStartedMsg arrives from watchForPairingCmd.
	// It is a plain value — no pointer aliasing across goroutine boundaries.
	cancelWatch context.CancelFunc
}

func newAwaitingPairingModel(dev *discovery.Device) awaitingPairingModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	return awaitingPairingModel{
		connectDevice: dev,
		spinner:       s,
		cancelWatch:   func() {}, // safe no-op until watchStartedMsg arrives
	}
}

func (m awaitingPairingModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		return startWatchMsg{}
	})
}

// startWatchMsg triggers the watch cmd from within Init without a keypress.
type startWatchMsg struct{}

// beginWatchMsg is emitted by awaitingPairingModel after startWatchMsg is processed.
// The parent model handles this to launch watchForPairingCmd, replacing the
// startWatch flag side-channel.
type beginWatchMsg struct{}

// watchStartedMsg carries the cancel function for the background watch goroutine.
// Delivering it as a message keeps the cancel token on the Update path, avoiding
// pointer aliasing across goroutine boundaries.
type watchStartedMsg struct{ cancel context.CancelFunc }

func (m awaitingPairingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startWatchMsg:
		m.watching = true
		return m, func() tea.Msg { return beginWatchMsg{} }

	case watchStartedMsg:
		m.cancelWatch = msg.cancel
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.cancelWatch()
			m.cancelled = true
			return m, nil
		}
	}

	if m.watching {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m awaitingPairingModel) View() string {
	addr := addrStr(m.connectDevice.IP, m.connectDevice.Host, m.connectDevice.Port)

	rows := []string{
		headerStyle.Render("adbx — Pairing Required"),
		"",
		mutedStyle.Render("Device: " + addr),
		"",
		warnStyle.Render("This device has not been paired with this machine yet."),
		"",
		stepNumStyle.Render("Step 1"),
		stepStyle.Render("  On your Android: Settings → Developer Options → Wireless Debugging"),
		"",
		stepNumStyle.Render("Step 2"),
		stepStyle.Render("  Tap ") + boldStyle.Render("\"Pair device with pairing code\""),
		"",
		stepNumStyle.Render("Step 3"),
		stepStyle.Render("  adbx will detect the dialog and ask for the code automatically."),
		"",
	}

	if m.watching {
		rows = append(rows,
			m.spinner.View()+"  "+watchStyle.Render(fmt.Sprintf("Watching for pairing mode on %s…", m.connectDevice.IP)),
			"",
		)
	}

	rows = append(rows, mutedStyle.Render("esc  back to list"))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
