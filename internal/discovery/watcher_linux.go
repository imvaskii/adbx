//go:build linux

package discovery

import (
	"bufio"
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"

	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// watchForPairing continuously watches for _adb-tls-pairing._tcp entries
// matching targetIP using avahi-browse as a subprocess. This avoids the
// multicast socket conflict between zeroconf and the avahi daemon.
// The channel is closed when ctx is cancelled or avahi exits.
func watchForPairing(ctx context.Context, targetIP net.IP) <-chan Device {
	out := make(chan Device, 4)
	go func() {
		defer close(out)
		adbxlog.Info("discovery: watchForPairing started", "targetIP", targetIP)
		cmd := exec.CommandContext(ctx, "avahi-browse", "-r", "-p", servicePairing)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			adbxlog.Info("discovery: watchForPairing StdoutPipe error", "err", err)
			return
		}
		if err := cmd.Start(); err != nil {
			adbxlog.Info("discovery: watchForPairing avahi-browse start error", "err", err)
			return
		}
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			adbxlog.Debug("discovery: watchForPairing avahi line", "raw", line)
		dev, ok := parseAvahiLine(line, targetIP, DevicePairing)
			if !ok {
				continue
			}
			adbxlog.Info("discovery: watchForPairing matched device", "ip", dev.IP, "port", dev.Port)
			select {
			case out <- dev:
			case <-ctx.Done():
				adbxlog.Info("discovery: watchForPairing context cancelled")
				return
			}
		}
		adbxlog.Info("discovery: watchForPairing avahi-browse exited")
		_ = cmd.Wait()
	}()
	return out
}

// scanForConnect does a short targeted browse for _adb-tls-connect._tcp
// entries matching targetIP using avahi-browse. Returns the first match
// or nil if ctx expires first.
func scanForConnect(ctx context.Context, targetIP net.IP) *Device {
	adbxlog.Info("discovery: scanForConnect started", "targetIP", targetIP)
	cmd := exec.CommandContext(ctx, "avahi-browse", "-r", "-p", serviceConnect)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		adbxlog.Info("discovery: scanForConnect StdoutPipe error", "err", err)
		return nil
	}
	if err := cmd.Start(); err != nil {
		adbxlog.Info("discovery: scanForConnect avahi-browse start error", "err", err)
		return nil
	}
	defer cmd.Wait() //nolint:errcheck

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		adbxlog.Debug("discovery: scanForConnect avahi line", "raw", line)
		dev, ok := parseAvahiLine(line, targetIP, DeviceConnect)
		if !ok {
			continue
		}
		adbxlog.Info("discovery: scanForConnect found device", "ip", dev.IP, "port", dev.Port)
		return &dev
	}
	adbxlog.Info("discovery: scanForConnect timed out, no device found")
	return nil
}

// parseAvahiLine parses a line from `avahi-browse -r -p` output.
// Format:
//
//	=;iface;proto;name;type;domain;hostname;address;port[;txt...]
//
// dtype controls the DeviceType set on the returned Device — callers pass
// DevicePairing or DeviceConnect based on which service they are browsing,
// eliminating the post-hoc Type fixup that scanForConnect used to apply.
// Returns the Device and true only for fully-resolved IPv4 lines that
// match targetIP (or any IP if targetIP is nil).
func parseAvahiLine(line string, targetIP net.IP, dtype DeviceType) (Device, bool) {
	if !strings.HasPrefix(line, "=;") {
		return Device{}, false
	}
	parts := strings.SplitN(line, ";", 10)
	if len(parts) < 9 {
		return Device{}, false
	}
	if parts[2] != "IPv4" {
		return Device{}, false
	}
	name := parts[3]
	hostname := parts[6]
	address := parts[7]
	portStr := parts[8]

	ip := net.ParseIP(address)
	if ip == nil {
		return Device{}, false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if targetIP != nil && !ip.Equal(targetIP.To4()) {
		return Device{}, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return Device{}, false
	}
	return Device{
		Name: name,
		Host: hostname,
		IP:   ip,
		Port: port,
		Type: dtype,
	}, true
}
