//go:build !linux && !darwin

package discovery

import (
	"context"
	"net"

	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// watchForPairing is not supported on this platform.
// Returns a channel that is immediately closed.
func watchForPairing(ctx context.Context, targetIP net.IP) <-chan Device {
	adbxlog.Info("discovery: watchForPairing not supported on this platform")
	out := make(chan Device)
	close(out)
	return out
}

// scanForConnect is not supported on this platform.
func scanForConnect(_ context.Context, _ net.IP) *Device {
	adbxlog.Info("discovery: scanForConnect not supported on this platform")
	return nil
}
