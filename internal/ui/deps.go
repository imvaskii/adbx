package ui

import (
	"context"
	"net"

	"github.com/imvaskii/adbx/internal/adb"
	"github.com/imvaskii/adbx/internal/discovery"
)

// connector is the seam between the UI and the adb binary wrapper.
// Callers supply a host string and port; the implementation handles
// subprocess execution, timeout, and output parsing.
type connector interface {
	Connect(ctx context.Context, host string, port int) adb.Result
	Pair(ctx context.Context, host string, port int, code string) adb.Result
}

// scanner is the seam between the UI and mDNS / avahi-browse discovery.
// Scan returns all devices visible on the LAN, surfacing any error so the
// UI can distinguish "scan failed" from "no devices found".
// WatchForPairing blocks until a pairing-mode entry for the given IP appears
// (or the context is cancelled). ScanForConnect does a short targeted scan
// for the connect-mode entry after pairing.
type scanner interface {
	Scan(ctx context.Context) ([]discovery.Device, error)
	WatchForPairing(ctx context.Context, ip net.IP) <-chan discovery.Device
	ScanForConnect(ctx context.Context, ip net.IP) *discovery.Device
}

// realConnector adapts the adb package to the connector interface.
type realConnector struct{}

func (realConnector) Connect(ctx context.Context, host string, port int) adb.Result {
	return adb.Connect(ctx, host, port)
}

func (realConnector) Pair(ctx context.Context, host string, port int, code string) adb.Result {
	return adb.Pair(ctx, host, port, code)
}

// realScanner adapts the discovery package to the scanner interface.
type realScanner struct{}

func (realScanner) Scan(ctx context.Context) ([]discovery.Device, error) {
	return discovery.Scan(ctx)
}

func (realScanner) WatchForPairing(ctx context.Context, ip net.IP) <-chan discovery.Device {
	return discovery.WatchForPairing(ctx, ip)
}

func (realScanner) ScanForConnect(ctx context.Context, ip net.IP) *discovery.Device {
	return discovery.ScanForConnect(ctx, ip)
}
