package ui

import (
	"context"
	"net"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/imvaskii/adbx/internal/adb"
	"github.com/imvaskii/adbx/internal/discovery"
)

// update is a test helper that runs one Update cycle and returns the new Model.
func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// updateWithCmd runs one Update cycle, executes the returned Cmd synchronously
// (if non-nil), and feeds the resulting message back through Update.
// Use this when the action under test returns a Cmd rather than a direct state change.
func updateWithCmd(m Model, msg tea.Msg) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		result := cmd()
		if result != nil {
			m = update(m, result)
		}
	}
	return m
}

// makeDevices returns a slice of n connect-mode devices for list tests.
func makeDevices(n int) []discovery.Device {
	devs := make([]discovery.Device, n)
	for i := range devs {
		devs[i] = discovery.Device{
			Name: "device",
			IP:   net.ParseIP("192.168.1.1"),
			Port: 37000 + i,
			Type: discovery.DeviceConnect,
		}
	}
	return devs
}

func makePairingDevice() discovery.Device {
	return discovery.Device{
		Name: "pixel",
		IP:   net.ParseIP("192.168.1.2"),
		Port: 40000,
		Type: discovery.DevicePairing,
	}
}

// ---- U1: app starts in scanning state ----------------------------------------

func TestModel_StartsInScanningState(t *testing.T) {
	m := New()
	if m.state != stateScanning {
		t.Fatalf("expected stateScanning on init, got %v", m.state)
	}
}

// ---- U2: scanDoneMsg with devices → device list ------------------------------

func TestModel_ScanDone_TransitionsToList(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(2)})
	if m.state != stateList {
		t.Fatalf("expected stateList after scanDoneMsg, got %v", m.state)
	}
	if len(m.list.devices) != 2 {
		t.Fatalf("expected 2 devices in list, got %d", len(m.list.devices))
	}
}

// ---- U3: scanDoneMsg with empty list → still device list ---------------------

func TestModel_ScanDone_EmptyList_StillTransitions(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: nil})
	if m.state != stateList {
		t.Fatalf("expected stateList even with empty scan results")
	}
}

// ---- U4: j moves cursor down -------------------------------------------------

func TestList_J_MovesCursorDown(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(3)})
	before := m.list.cursor
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.list.cursor != before+1 {
		t.Fatalf("expected cursor %d after j, got %d", before+1, m.list.cursor)
	}
}

// ---- U5: k clamps at top -----------------------------------------------------

func TestList_K_ClampsAtZero(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(3)})
	// cursor is already 0; pressing k should not go negative
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.list.cursor != 0 {
		t.Fatalf("expected cursor to clamp at 0, got %d", m.list.cursor)
	}
}

// ---- U6: G jumps to last device ----------------------------------------------

func TestList_G_JumpsToBottom(t *testing.T) {
	m := New()
	devs := makeDevices(5)
	m = update(m, scanDoneMsg{devices: devs})
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if m.list.cursor != 4 {
		t.Fatalf("expected cursor at 4 after G, got %d", m.list.cursor)
	}
}

// ---- U7: gg jumps to top -----------------------------------------------------

func TestList_GG_JumpsToTop(t *testing.T) {
	m := New()
	devs := makeDevices(5)
	m = update(m, scanDoneMsg{devices: devs})
	// Move to bottom first
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	// Now gg
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.list.cursor != 0 {
		t.Fatalf("expected cursor at 0 after gg, got %d", m.list.cursor)
	}
}

// ---- U8: enter on connect device triggers connectCmd → result ----------------

func TestList_Enter_ConnectDevice_TransitionsToResult(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	// enter → connectCmd fires; simulate the Cmd completing via actionDoneMsg
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	_ = cmd
	// Inject the result that the Cmd would have produced
	m = update(m, actionDoneMsg{result: adb.Result{Success: true, Message: "connected"}})
	if m.state != stateResult {
		t.Fatalf("expected stateResult after connect, got %v", m.state)
	}
}

// ---- U9: enter on pairing device transitions to pairing input ----------------

func TestList_Enter_PairingDevice_TransitionsToPairing(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != statePairing {
		t.Fatalf("expected statePairing after selecting pairing device, got %v", m.state)
	}
}

// ---- U10: r from list triggers rescan ----------------------------------------

func TestList_R_TriggersRescan(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.state != stateScanning {
		t.Fatalf("expected stateScanning after r, got %v", m.state)
	}
}

// ---- U11: pairing input rejects codes < 6 digits ----------------------------

func TestPairing_ShortCode_Rejected(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyEnter}) // enter pairing screen

	// Type only 4 digits then press enter
	for _, ch := range "1234" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Should stay in pairing state with an error
	if m.state != statePairing {
		t.Fatalf("expected to stay in statePairing on short code, got %v", m.state)
	}
	if m.pairing.err == "" {
		t.Fatalf("expected validation error message for short code")
	}
}

// ---- U12: pairing input accepts exactly 6 digits ----------------------------

func TestPairing_SixDigitCode_Submits(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyEnter}) // enter pairing screen

	for _, ch := range "482910" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// After enter, submitted flag causes pairCmd to fire; simulate result
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(m, actionDoneMsg{result: adb.Result{Success: true, Message: "Successfully paired"}})

	if m.state != stateResult {
		t.Fatalf("expected stateResult after successful pair, got %v", m.state)
	}
}

// ---- U13: esc from pairing returns to list -----------------------------------

func TestPairing_Esc_ReturnsToList(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateList {
		t.Fatalf("expected stateList after esc from pairing, got %v", m.state)
	}
}

// ---- U14: actionDoneMsg failure lands on result screen ----------------------

func TestModel_ActionDone_Failure_ShowsResult(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	m = update(m, actionDoneMsg{result: adb.Result{Success: false, Message: "failed to connect"}})
	if m.state != stateResult {
		t.Fatalf("expected stateResult on failed action, got %v", m.state)
	}
	if m.result.result.Success {
		t.Fatalf("expected failure result to be preserved")
	}
}

// ---- U15: r from result triggers rescan -------------------------------------

func TestResult_R_TriggersRescan(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	m = update(m, actionDoneMsg{result: adb.Result{Success: true, Message: "connected"}})
	m = updateWithCmd(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.state != stateScanning {
		t.Fatalf("expected stateScanning after r from result, got %v", m.state)
	}
}

// ---- B1: needsPairingMsg routes to stateAwaitingPairing ---------------------

// RED → GREEN: connectCmd emits needsPairingMsg when adb reports auth failure;
// the root model must route it to stateAwaitingPairing, not stateResult.
func TestModel_NeedsPairing_TransitionsToAwaitingPairing(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	dev := m.list.devices[0]
	m = update(m, needsPairingMsg{device: &dev})
	if m.state != stateAwaitingPairing {
		t.Fatalf("expected stateAwaitingPairing on needsPairingMsg, got %v", m.state)
	}
	if m.awaitingPairing.connectDevice == nil {
		t.Fatalf("expected connectDevice to be set in awaitingPairingModel")
	}
}

// ---- B2: pairingDeviceFoundMsg auto-advances to statePairing ----------------

func TestModel_PairingDeviceFound_TransitionsToPairing(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(1)})
	dev := m.list.devices[0]
	m = update(m, needsPairingMsg{device: &dev})

	pairingDev := discovery.Device{
		Name: "pixel-pairing",
		IP:   net.ParseIP("192.168.1.1"),
		Port: 40123,
		Type: discovery.DevicePairing,
	}
	m = update(m, pairingDeviceFoundMsg{device: pairingDev})
	if m.state != statePairing {
		t.Fatalf("expected statePairing after pairingDeviceFoundMsg, got %v", m.state)
	}
	if m.pairing.device == nil || m.pairing.device.Port != 40123 {
		t.Fatalf("expected pairing screen to hold the found device with port 40123")
	}
}

// ---- B3: esc from stateAwaitingPairing returns to device list ---------------

func TestAwaitingPairing_Esc_ReturnsToList(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: makeDevices(2)})
	dev := m.list.devices[0]
	m = update(m, needsPairingMsg{device: &dev})
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateList {
		t.Fatalf("expected stateList after esc from awaitingPairing, got %v", m.state)
	}
}

// ---- B5: actionDoneMsg with NeedsPairing=false after auto-connect shows result

func TestPairThenConnect_AutoConnect_ShowsResult(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = update(m, needsPairingMsg{device: &m.list.devices[0]})
	m = update(m, pairingDeviceFoundMsg{device: makePairingDevice()})
	// Simulate pairThenConnectCmd emitting a successful connect result
	// (as if ScanForConnect found the connect-mode entry and adb connected).
	m = update(m, actionDoneMsg{result: adb.Result{Success: true, Message: "connected to 192.168.1.12:45323"}})
	if m.state != stateResult {
		t.Fatalf("expected stateResult after auto-connect, got %v", m.state)
	}
	if !m.result.result.Success {
		t.Fatalf("expected success result after auto-connect")
	}
}

// ---- B6: fallback rescan message shown when ScanForConnect times out ---------

func TestPairThenConnect_Fallback_ShowsResult(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = update(m, needsPairingMsg{device: &m.list.devices[0]})
	m = update(m, pairingDeviceFoundMsg{device: makePairingDevice()})
	// ScanForConnect timed out — the real path emits connectAddrFoundMsg{nil},
	// which routes to the fallback result (not actionDoneMsg directly).
	m = update(m, connectAddrFoundMsg{device: nil})
	if m.state != stateResult {
		t.Fatalf("expected stateResult after fallback, got %v", m.state)
	}
	view := m.result.View()
	if view == "" {
		t.Fatal("expected non-empty result view")
	}
}

// ---- B4 (updated): incorrect pin stays on pairing screen with inline error ---

func TestPairCmd_Failure_ShowsResult(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = update(m, needsPairingMsg{device: &m.list.devices[0]})
	m = update(m, pairingDeviceFoundMsg{device: makePairingDevice()})
	for _, ch := range "000000" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	// Incorrect pin — should stay on pairing screen, not go to result.
	m = update(m, actionDoneMsg{result: adb.Result{Success: false, IncorrectPin: true, Message: "Failed to pair with device: incorrect pin"}})
	if m.state != statePairing {
		t.Fatalf("expected statePairing after incorrect pin, got %v", m.state)
	}
	if m.pairing.err == "" {
		t.Fatal("expected inline error message on pairing screen after wrong pin")
	}
	if m.pairing.submitting {
		t.Fatal("expected submitting=false after retry reset")
	}
}

// ---- B7: window closed routes back to awaitingPairing -----------------------

func TestPairCmd_WindowClosed_ReturnsToAwaitingPairing(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	dev := m.list.devices[0]
	m = update(m, needsPairingMsg{device: &dev})
	m = update(m, pairingDeviceFoundMsg{device: makePairingDevice()})
	for _, ch := range "123456" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	// Pairing window was closed on the device.
	m = update(m, actionDoneMsg{result: adb.Result{Success: false, WindowClosed: true, Message: "error: protocol fault"}})
	if m.state != stateAwaitingPairing {
		t.Fatalf("expected stateAwaitingPairing after window closed, got %v", m.state)
	}
}

// ---- B8: double enter while submitting is blocked ---------------------------

func TestPairing_DoubleEnter_Blocked(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: []discovery.Device{makePairingDevice()}})
	m = update(m, pairingDeviceFoundMsg{device: makePairingDevice()})
	for _, ch := range "123456" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter}) // first enter — submitting=true
	if !m.pairing.submitting {
		t.Fatal("expected submitting=true after first enter")
	}
	// Second enter should be ignored — submitted should not flip back on.
	m.pairing.submitted = false // simulate model reset as Update does
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pairing.submitted {
		t.Fatal("expected second enter to be blocked while submitting")
	}
}

// ---- U16: scanDoneMsg with error shows scanErr in list -----------------------

func TestModel_ScanDone_WithError_SetsScanErr(t *testing.T) {
	m := New()
	m = update(m, scanDoneMsg{devices: nil, err: net.ErrClosed})
	if m.state != stateList {
		t.Fatalf("expected stateList even on scan error, got %v", m.state)
	}
	if m.list.scanErr == nil {
		t.Fatal("expected scanErr to be set on listModel")
	}
}

// ---- Fake adapters ----------------------------------------------------------

// fakeConnector is a Connector that returns canned results without calling adb.
type fakeConnector struct {
	connectResult adb.Result
	pairResult    adb.Result
}

func (f *fakeConnector) Connect(_ context.Context, _ string, _ int) adb.Result { return f.connectResult }
func (f *fakeConnector) Pair(_ context.Context, _ string, _ int, _ string) adb.Result {
	return f.pairResult
}

// fakeScanner is a Scanner that returns canned results without touching the network.
type fakeScanner struct {
	scanDevices []discovery.Device
	scanErr     error
	watchCh     chan discovery.Device
	connectDev  *discovery.Device
}

func (f *fakeScanner) Scan(_ context.Context) ([]discovery.Device, error) {
	return f.scanDevices, f.scanErr
}

func (f *fakeScanner) WatchForPairing(_ context.Context, _ net.IP) <-chan discovery.Device {
	if f.watchCh == nil {
		ch := make(chan discovery.Device)
		close(ch)
		return ch
	}
	return f.watchCh
}

func (f *fakeScanner) ScanForConnect(_ context.Context, _ net.IP) *discovery.Device {
	return f.connectDev
}

// ---- W1: watchForPairingCmd batch delivers watchStartedMsg with cancel ------

func TestWatchForPairingCmd_BatchHasWatchStarted(t *testing.T) {
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 40000, Type: discovery.DevicePairing}
	// Closed channel → WatchForPairing returns immediately (channel-closed path).
	fs := &fakeScanner{}
	m := newWithDeps(&fakeConnector{}, fs)

	cmd := m.watchForPairingCmd(dev)
	batchResult := cmd()
	batch, ok := batchResult.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", batchResult)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 sub-cmds in batch, got %d", len(batch))
	}

	// First sub-cmd must deliver watchStartedMsg with a non-nil cancel.
	msg0 := batch[0]()
	started, ok := msg0.(watchStartedMsg)
	if !ok {
		t.Fatalf("expected watchStartedMsg from batch[0], got %T", msg0)
	}
	if started.cancel == nil {
		t.Fatal("expected non-nil cancel in watchStartedMsg")
	}
}

// ---- W2: watchForPairingCmd batch[1] delivers pairingDeviceFoundMsg ----------

func TestWatchForPairingCmd_DeviceFound_DeliversPairingDeviceFoundMsg(t *testing.T) {
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 40000, Type: discovery.DevicePairing}
	found := discovery.Device{IP: dev.IP, Port: 40001, Type: discovery.DevicePairing}
	watchCh := make(chan discovery.Device, 1)
	watchCh <- found
	fs := &fakeScanner{watchCh: watchCh}
	m := newWithDeps(&fakeConnector{}, fs)

	cmd := m.watchForPairingCmd(dev)
	batch := cmd().(tea.BatchMsg)

	msg1 := batch[1]()
	got, ok := msg1.(pairingDeviceFoundMsg)
	if !ok {
		t.Fatalf("expected pairingDeviceFoundMsg from batch[1], got %T", msg1)
	}
	if got.device.Port != 40001 {
		t.Fatalf("expected port 40001, got %d", got.device.Port)
	}
}

// ---- W3: watchForPairingCmd batch[1] returns nil when channel closes --------

func TestWatchForPairingCmd_ChannelClosed_ReturnsNil(t *testing.T) {
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 40000, Type: discovery.DevicePairing}
	// fakeScanner with nil watchCh → WatchForPairing returns a closed channel.
	fs := &fakeScanner{}
	m := newWithDeps(&fakeConnector{}, fs)

	cmd := m.watchForPairingCmd(dev)
	batch := cmd().(tea.BatchMsg)

	msg1 := batch[1]()
	if msg1 != nil {
		t.Fatalf("expected nil msg when channel closed, got %T: %v", msg1, msg1)
	}
}

// ---- C1: connectCmd delivers actionDoneMsg on success -----------------------

func TestConnectCmd_Success_DeliversActionDoneMsg(t *testing.T) {
	fc := &fakeConnector{connectResult: adb.Result{Success: true, Message: "connected"}}
	m := newWithDeps(fc, &fakeScanner{})
	cmd := m.connectCmd(&discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 37000})
	msg := cmd()
	got, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("expected actionDoneMsg, got %T", msg)
	}
	if !got.result.Success {
		t.Fatalf("expected success result, got %+v", got.result)
	}
}

// ---- C2: connectCmd delivers needsPairingMsg when NeedsPairing is set -------

func TestConnectCmd_NeedsPairing_DeliversNeedsPairingMsg(t *testing.T) {
	fc := &fakeConnector{connectResult: adb.Result{NeedsPairing: true}}
	m := newWithDeps(fc, &fakeScanner{})
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 37000}
	cmd := m.connectCmd(dev)
	msg := cmd()
	if _, ok := msg.(needsPairingMsg); !ok {
		t.Fatalf("expected needsPairingMsg, got %T", msg)
	}
}

// ---- C3: pairCmd delivers pairDoneMsg on success ----------------------------

func TestPairCmd_Success_DeliversPairDoneMsg(t *testing.T) {
	fc := &fakeConnector{pairResult: adb.Result{Success: true, Message: "Successfully paired"}}
	m := newWithDeps(fc, &fakeScanner{})
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.2"), Port: 40000}
	cmd := m.pairCmd(dev, "123456")
	msg := cmd()
	got, ok := msg.(pairDoneMsg)
	if !ok {
		t.Fatalf("expected pairDoneMsg, got %T", msg)
	}
	if !got.result.Success {
		t.Fatalf("expected success result")
	}
}

// ---- C4: pairCmd delivers actionDoneMsg on failure --------------------------

func TestPairCmd_Failure_DeliversActionDoneMsg(t *testing.T) {
	fc := &fakeConnector{pairResult: adb.Result{Success: false, IncorrectPin: true}}
	m := newWithDeps(fc, &fakeScanner{})
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.2"), Port: 40000}
	cmd := m.pairCmd(dev, "999999")
	msg := cmd()
	got, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("expected actionDoneMsg on pair failure, got %T", msg)
	}
	if !got.result.IncorrectPin {
		t.Fatalf("expected IncorrectPin=true")
	}
}

// ---- C5: scanForConnectCmd delivers connectAddrFoundMsg{nil} on timeout -----

func TestScanForConnectCmd_Timeout_DeliversNilDevice(t *testing.T) {
	fs := &fakeScanner{connectDev: nil}
	m := newWithDeps(&fakeConnector{}, fs)
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1")}
	cmd := m.scanForConnectCmd(dev)
	msg := cmd()
	got, ok := msg.(connectAddrFoundMsg)
	if !ok {
		t.Fatalf("expected connectAddrFoundMsg, got %T", msg)
	}
	if got.device != nil {
		t.Fatalf("expected nil device on timeout, got %v", got.device)
	}
}

// ---- C6: scanForConnectCmd delivers connectAddrFoundMsg with device on success

func TestScanForConnectCmd_Found_DeliversDevice(t *testing.T) {
	found := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Port: 45000, Type: discovery.DeviceConnect}
	fs := &fakeScanner{connectDev: found}
	m := newWithDeps(&fakeConnector{}, fs)
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1")}
	cmd := m.scanForConnectCmd(dev)
	msg := cmd()
	got, ok := msg.(connectAddrFoundMsg)
	if !ok {
		t.Fatalf("expected connectAddrFoundMsg, got %T", msg)
	}
	if got.device == nil || got.device.Port != 45000 {
		t.Fatalf("expected device with port 45000, got %v", got.device)
	}
}
