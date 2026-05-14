// Package ui contains all Bubble Tea TUI components for adbx.
// The root Model drives a state machine across five screens:
//
//	Scanning → DeviceList → AwaitingPairing → PairingInput → Result
//	                      ↘                                 ↗
//	                       (connect mode, already paired)
package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/imvaskii/adbx/internal/adb"
	"github.com/imvaskii/adbx/internal/discovery"
	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// State machine

type state int

const (
	stateScanning state = iota
	stateList
	stateAwaitingPairing // connect failed with NeedsPairing; watching for pairing mode
	statePairing
	stateResult
)

// Messages

// scanDoneMsg is sent when mDNS scanning completes.
// err is non-nil when the scan itself failed (distinct from finding zero devices).
type scanDoneMsg struct {
	devices []discovery.Device
	err     error
}

// actionDoneMsg is sent when adb pair/connect finishes.
type actionDoneMsg struct{ result adb.Result }

// needsPairingMsg is sent when a connect attempt returns NeedsPairing=true.
// It carries the original device so the awaiting screen can watch by IP.
type needsPairingMsg struct{ device *discovery.Device }

// pairingDeviceFoundMsg is sent when WatchForPairing finds a matching entry.
type pairingDeviceFoundMsg struct{ device discovery.Device }

// pairDoneMsg is sent when adb pair completes successfully.
// It carries the paired device so the next step can scan for the connect port.
type pairDoneMsg struct {
	result adb.Result
	device *discovery.Device
}

// connectAddrFoundMsg is sent when ScanForConnect locates the connect-mode
// mDNS entry after pairing. It carries the device with the new port.
type connectAddrFoundMsg struct{ device *discovery.Device }

// ---- Root model --------------------------------------------------------------

// Model is the root Bubble Tea model. It embeds sub-models for each screen
// and delegates Init/Update/View to the active one.
type Model struct {
	state  state
	width  int
	height int

	connector connector
	scanner   scanner

	scanning        scanModel
	list            listModel
	awaitingPairing awaitingPairingModel
	pairing         pairingModel
	result          resultModel
}

func (s state) String() string {
	switch s {
	case stateScanning:
		return "scanning"
	case stateList:
		return "list"
	case stateAwaitingPairing:
		return "awaitingPairing"
	case statePairing:
		return "pairing"
	case stateResult:
		return "result"
	default:
		return "unknown"
	}
}

// transition logs a state change and returns the new state.
func (m *Model) transition(to state) {
	adbxlog.Info("ui: state transition", "from", m.state, "to", to)
	m.state = to
}

func New() Model {
	return newWithDeps(realConnector{}, realScanner{})
}

// newWithDeps constructs a Model with explicit connector and scanner adapters.
// Used in tests to inject fakes without touching real binaries or the network.
func newWithDeps(c connector, s scanner) Model {
	return Model{
		state:     stateScanning,
		scanning:  newScanModel(),
		connector: c,
		scanner:   s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanning.Init(), m.triggerScan())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.width = msg.Width
		m.list.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.state != statePairing && m.state != stateAwaitingPairing && msg.String() == "q" {
			return m, tea.Quit
		}

	case scanDoneMsg:
		adbxlog.Info("ui: scanDoneMsg", "deviceCount", len(msg.devices), "err", msg.err)
		m.list = newListModel(msg.devices, m.width, m.height)
		if msg.err != nil {
			m.list.scanErr = msg.err
		}
		m.transition(stateList)
		return m, m.list.Init()

	case needsPairingMsg:
		adbxlog.Info("ui: needsPairingMsg", "device", msg.device.IP)
		// Connect failed — device not paired. Move to the awaiting pairing screen
		// which will guide the user and watch for the pairing mDNS entry.
		m.awaitingPairing = newAwaitingPairingModel(msg.device)
		m.transition(stateAwaitingPairing)
		return m, m.awaitingPairing.Init()

	case beginWatchMsg:
		// awaitingPairingModel processed startWatchMsg and signalled that the
		// background watch should start. Launch it here so the cmd is owned by
		// the root model, eliminating the startWatch flag side-channel.
		adbxlog.Info("ui: beginWatchMsg, launching watchForPairingCmd")
		return m, m.watchForPairingCmd(m.awaitingPairing.connectDevice)

	case deviceChosenMsg:
		dev := msg.device
		adbxlog.Info("ui: device selected", "name", dev.Name, "ip", dev.IP, "port", dev.Port, "type", dev.Type)
		if dev.Type == discovery.DevicePairing {
			m.pairing = newPairingModel(dev)
			m.transition(statePairing)
			return m, m.pairing.Init()
		}
		return m, m.connectCmd(dev)

	case rescanMsg:
		adbxlog.Info("ui: rescan triggered")
		m.scanning = newScanModel()
		m.transition(stateScanning)
		return m, tea.Batch(m.scanning.Init(), m.triggerScan())

	case pairingDeviceFoundMsg:
		adbxlog.Info("ui: pairingDeviceFoundMsg", "ip", msg.device.IP, "port", msg.device.Port)
		// The pairing-mode mDNS entry appeared for our target device.
		// Auto-advance to the pairing code input.
		m.pairing = newPairingModel(&msg.device)
		m.transition(statePairing)
		return m, m.pairing.Init()

	case pairDoneMsg:
		adbxlog.Info("ui: pairDoneMsg", "success", msg.result.Success)
		if !msg.result.Success {
			m.result = newResultModel(msg.result)
			m.transition(stateResult)
			return m, nil
		}
		// Pair succeeded — scan for the connect-mode mDNS entry (new port).
		return m, m.scanForConnectCmd(msg.device)

	case connectAddrFoundMsg:
		if msg.device == nil {
			// ScanForConnect timed out — show result with manual rescan hint.
			adbxlog.Info("ui: ScanForConnect timed out, showing fallback rescan message")
			fallback := adb.Result{
				Success: true,
				Message: "Paired!\n\nPress 'r' to rescan and connect.",
			}
			m.result = newResultModel(fallback)
			m.transition(stateResult)
			return m, nil
		}
		adbxlog.Info("ui: connectAddrFoundMsg, auto-connecting", "ip", msg.device.IP, "port", msg.device.Port)
		return m, m.connectCmd(msg.device)

	case actionDoneMsg:
		adbxlog.Info("ui: actionDoneMsg", "success", msg.result.Success, "needsPairing", msg.result.NeedsPairing, "incorrectPin", msg.result.IncorrectPin, "windowClosed", msg.result.WindowClosed, "msg", msg.result.Message)
		if msg.result.NeedsPairing {
			return m, nil
		}
		if msg.result.IncorrectPin {
			// Wrong code — stay on pairing screen, clear input, show inline error.
			adbxlog.Info("ui: incorrect pin, retrying pairing")
			m.pairing = m.pairing.retry("Incorrect code — check the digits shown on your device and try again.")
			return m, textinput.Blink
		}
		if msg.result.WindowClosed {
			// Pairing dialog was closed — go back to awaiting screen so
			// WatchForPairing restarts when the user re-opens the dialog.
			adbxlog.Info("ui: pairing window closed, returning to awaitingPairing")
			m.awaitingPairing = newAwaitingPairingModel(m.pairing.device)
			m.transition(stateAwaitingPairing)
			return m, m.awaitingPairing.Init()
		}
		m.result = newResultModel(msg.result)
		m.transition(stateResult)
		return m, nil
	}

	// Delegate to the active screen.
	switch m.state {
	case stateScanning:
		nm, cmd := m.scanning.Update(msg)
		m.scanning = nm.(scanModel)
		return m, cmd

	case stateList:
		nm, cmd := m.list.Update(msg)
		m.list = nm.(listModel)
		return m, cmd

	case stateAwaitingPairing:
		nm, cmd := m.awaitingPairing.Update(msg)
		m.awaitingPairing = nm.(awaitingPairingModel)
		if m.awaitingPairing.cancelled {
			adbxlog.Info("ui: awaitingPairing cancelled, returning to list")
			m.list = newListModel(m.list.devices, m.width, m.height)
			m.transition(stateList)
			return m, m.list.Init()
		}
		return m, cmd

	case statePairing:
		nm, cmd := m.pairing.Update(msg)
		m.pairing = nm.(pairingModel)
		if m.pairing.submitted {
			dev := m.pairing.device
			code := m.pairing.code()
			adbxlog.Info("ui: pairing code submitted", "ip", dev.IP, "port", dev.Port)
			m.pairing.submitted = false
			return m, m.pairCmd(dev, code)
		}
		if m.pairing.cancelled {
			adbxlog.Info("ui: pairing cancelled, returning to list")
			m.list = newListModel(m.list.devices, m.width, m.height)
			m.transition(stateList)
			return m, m.list.Init()
		}
		return m, cmd

	case stateResult:
		nm, cmd := m.result.Update(msg)
		m.result = nm.(resultModel)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	var content string
	switch m.state {
	case stateScanning:
		content = m.scanning.View()
	case stateList:
		content = m.list.View()
	case stateAwaitingPairing:
		content = m.awaitingPairing.View()
	case statePairing:
		content = m.pairing.View()
	case stateResult:
		content = m.result.View()
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(content)
}

// ---- Commands ----------------------------------------------------------------

func (m Model) triggerScan() tea.Cmd {
	s := m.scanner
	return func() tea.Msg {
		devices, err := s.Scan(context.Background())
		return scanDoneMsg{devices: devices, err: err}
	}
}

// connectCmd attempts adb connect. If the device needs pairing it emits
// needsPairingMsg instead of actionDoneMsg so the state machine routes
// to the awaiting-pairing screen rather than the result screen.
func (m Model) connectCmd(dev *discovery.Device) tea.Cmd {
	c := m.connector
	return func() tea.Msg {
		host := resolveHost(dev)
		result := c.Connect(context.Background(), host, dev.Port)
		if result.NeedsPairing {
			return needsPairingMsg{device: dev}
		}
		return actionDoneMsg{result: result}
	}
}

// watchForPairingCmd returns two batched Cmds:
//  1. A fast Cmd that creates the context and immediately delivers watchStartedMsg
//     so the cancel function is stored on the Update path — no pointer aliasing.
//  2. A blocking Cmd that runs WatchForPairing and delivers pairingDeviceFoundMsg
//     (or nil if the context is cancelled before a device is found).
func (m Model) watchForPairingCmd(dev *discovery.Device) tea.Cmd {
	s := m.scanner
	watchCtx, watchCancel := context.WithCancel(context.Background())
	return tea.Batch(
		func() tea.Msg {
			return watchStartedMsg{cancel: watchCancel}
		},
		func() tea.Msg {
			defer watchCancel()
			ch := s.WatchForPairing(watchCtx, dev.IP)
			found, ok := <-ch
			if !ok {
				return nil // context cancelled, no device found
			}
			return pairingDeviceFoundMsg{device: found}
		},
	)
}

// pairCmd runs adb pair. On success it delivers pairDoneMsg so the state
// machine can proceed to scanForConnectCmd. On failure it delivers actionDoneMsg
// directly (IncorrectPin and WindowClosed are handled there).
func (m Model) pairCmd(dev *discovery.Device, code string) tea.Cmd {
	c := m.connector
	return func() tea.Msg {
		host := resolveHost(dev)
		result := c.Pair(context.Background(), host, dev.Port, code)
		if !result.Success {
			adbxlog.Info("ui: pair failed", "ip", dev.IP, "port", dev.Port)
			return actionDoneMsg{result: result}
		}
		adbxlog.Info("ui: pair succeeded, scanning for connect-mode entry", "ip", dev.IP)
		return pairDoneMsg{result: result, device: dev}
	}
}

// scanForConnectCmd waits for the connect-mode mDNS entry to appear after
// pairing. On Linux (avahi-browse) a few seconds is enough; on macOS the
// chain is dns-sd -B → dns-sd -L → dns-sd -G v4 and can take longer, so
// we allow up to 15 seconds. Delivers connectAddrFoundMsg{device} on success
// or connectAddrFoundMsg{nil} on timeout.
func (m Model) scanForConnectCmd(dev *discovery.Device) tea.Cmd {
	s := m.scanner
	return func() tea.Msg {
		scanCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		connectDev := s.ScanForConnect(scanCtx, dev.IP)
		return connectAddrFoundMsg{device: connectDev}
	}
}

// addrStr and resolveHost live in address.go.
