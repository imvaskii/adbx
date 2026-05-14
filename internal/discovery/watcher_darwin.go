//go:build darwin

package discovery

import (
	"bufio"
	"context"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// subprocessRunner abstracts exec.CommandContext so tests can inject fake
// subprocess output without spawning real dns-sd processes.
// It starts the named program, returns its stdout as an io.Reader, and
// arranges for the process to be reaped when ctx is cancelled.
type subprocessRunner func(ctx context.Context, name string, args ...string) (io.Reader, error)

// defaultSubprocessRunner is the production subprocessRunner: it runs the
// real binary via exec.CommandContext, pipes stdout, and reaps the process
// in the background so the caller only needs to read and discard the reader.
func defaultSubprocessRunner(ctx context.Context, name string, args ...string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go cmd.Wait() //nolint:errcheck
	return stdout, nil
}

// watchForPairing continuously watches for _adb-tls-pairing._tcp using
// dns-sd (macOS mDNSResponder). It pipelines browse → resolve → address
// lookup for each discovered service. The channel is closed when ctx is
// cancelled or the browse process exits.
func watchForPairing(ctx context.Context, targetIP net.IP) <-chan Device {
	out := make(chan Device, 4)
	go func() {
		defer close(out)
		adbxlog.Info("discovery: watchForPairing started", "targetIP", targetIP)
		dnsBrowse(ctx, servicePairing, DevicePairing, targetIP, out, defaultSubprocessRunner)
	}()
	return out
}

// scanForConnect does a short targeted browse for _adb-tls-connect._tcp.
// Returns the first match or nil if ctx expires first.
func scanForConnect(ctx context.Context, targetIP net.IP) *Device {
	adbxlog.Info("discovery: scanForConnect started", "targetIP", targetIP)
	out := make(chan Device, 4)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go dnsBrowse(ctx, serviceConnect, DeviceConnect, targetIP, out, defaultSubprocessRunner)
	select {
	case dev, ok := <-out:
		if !ok {
			return nil
		}
		adbxlog.Info("discovery: scanForConnect found device", "ip", dev.IP, "port", dev.Port)
		return &dev
	case <-ctx.Done():
		adbxlog.Info("discovery: scanForConnect timed out, no device found")
		return nil
	}
}

// dnsBrowse runs `dns-sd -B <serviceType> local` and for each service
// addition spawns a resolve goroutine that pipes into out.
// run is injected so tests can supply fake subprocess output.
func dnsBrowse(ctx context.Context, serviceType string, dtype DeviceType, targetIP net.IP, out chan<- Device, run subprocessRunner) {
	stdout, err := run(ctx, "dns-sd", "-B", serviceType, "local")
	if err != nil {
		adbxlog.Info("discovery: dnsBrowse subprocess error", "err", err)
		return
	}

	var wg sync.WaitGroup
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		adbxlog.Debug("discovery: dnsBrowse line", "raw", line)
		name, ok := parseBrowseLine(line)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(svcName string) {
			defer wg.Done()
			dev := dnsResolve(ctx, svcName, serviceType, dtype, targetIP, run)
			if dev == nil {
				return
			}
			select {
			case out <- *dev:
			case <-ctx.Done():
			}
		}(name)
	}
	wg.Wait()
}

// parseBrowseLine parses a `dns-sd -B` output line.
// Format (after header lines):
//
//	Browsing for _adb-tls-pairing._tcp
//	 HH:MM:SS.mmm  Add  flags  iface  domain  type  instance-name
//
// We only care about "Add" events and extract the last field (instance name).
func parseBrowseLine(line string) (name string, ok bool) {
	fields := strings.Fields(line)
	// Minimum: timestamp, Add, flags, iface, domain, type, name...
	if len(fields) < 7 {
		return "", false
	}
	if fields[1] != "Add" {
		return "", false
	}
	// Instance name may contain spaces — rejoin from field 6 onward.
	name = strings.Join(fields[6:], " ")
	return name, true
}

// dnsResolve runs `dns-sd -L <name> <type> local` to get host:port,
// then `dns-sd -G v4 <host>` to resolve the IP.
// run is injected so tests can supply fake subprocess output.
func dnsResolve(ctx context.Context, name, serviceType string, dtype DeviceType, targetIP net.IP, run subprocessRunner) *Device {
	rCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Use a separate child context for the dns-sd -L subprocess so we can
	// stop it as soon as we have the host:port without cancelling rCtx.
	// Cancelling rCtx early would break the subsequent dnsGetAddr call which
	// also depends on rCtx being alive.
	lookupCtx, lookupCancel := context.WithCancel(rCtx)
	defer lookupCancel()

	stdout, err := run(lookupCtx, "dns-sd", "-L", name, serviceType, "local")
	if err != nil {
		return nil
	}

	var host string
	var port int

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		adbxlog.Debug("discovery: dnsResolve lookup line", "raw", line)
		h, p, ok := parseLookupLine(line)
		if !ok {
			continue
		}
		host = h
		port = p
		lookupCancel() // stop dns-sd -L only; rCtx remains valid for dnsGetAddr
		break
	}

	if host == "" || port == 0 {
		return nil
	}

	ip := dnsGetAddr(rCtx, host, run)
	if ip == nil {
		return nil
	}
	if targetIP != nil && !ip.Equal(targetIP.To4()) {
		return nil
	}

	return &Device{
		Name: name,
		Host: host,
		IP:   ip,
		Port: port,
		Type: dtype,
	}
}

// parseLookupLine parses a `dns-sd -L` output line.
// Relevant line format:
//
//	HH:MM:SS.mmm  <instance>._adb-tls-pairing._tcp.local. can be reached at <host>:<port> (...)
func parseLookupLine(line string) (host string, port int, ok bool) {
	// Look for "can be reached at"
	const marker = "can be reached at "
	idx := strings.Index(line, marker)
	if idx < 0 {
		return "", 0, false
	}
	rest := line[idx+len(marker):]
	// rest is like "MyPhone.local.:37263 (interface 7)"
	// strip anything after a space
	if sp := strings.Index(rest, " "); sp >= 0 {
		rest = rest[:sp]
	}
	// split host:port — port is after the last ':'
	lastColon := strings.LastIndex(rest, ":")
	if lastColon < 0 {
		return "", 0, false
	}
	hostPart := strings.TrimSuffix(rest[:lastColon], ".")
	portPart := rest[lastColon+1:]
	p, err := strconv.Atoi(portPart)
	if err != nil || p == 0 {
		return "", 0, false
	}
	return hostPart, p, true
}

// dnsGetAddr runs `dns-sd -G v4 <hostname>` and returns the first IPv4 address.
// run is injected so tests can supply fake subprocess output.
func dnsGetAddr(ctx context.Context, hostname string, run subprocessRunner) net.IP {
	rCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	stdout, err := run(rCtx, "dns-sd", "-G", "v4", hostname)
	if err != nil {
		return nil
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		adbxlog.Debug("discovery: dnsGetAddr line", "raw", line)
		ip := parseGetAddrLine(line)
		if ip == nil {
			continue
		}
		cancel()
		return ip
	}
	return nil
}

// parseGetAddrLine parses a `dns-sd -G v4` output line.
// Actual format emitted by dns-sd on macOS:
//
//	HH:MM:SS.mmm  Add  flags  iface  hostname  address  ttl
//
// Example:
//
//	23:18:34.499  Add  40000002  14  Android_EFGAH8N4.local.  192.168.1.12  120
func parseGetAddrLine(line string) net.IP {
	fields := strings.Fields(line)
	// need at least: timestamp, Add/Rmv, flags, iface, hostname, address
	if len(fields) < 6 {
		return nil
	}
	if fields[1] != "Add" {
		return nil
	}
	ip := net.ParseIP(fields[5])
	if ip == nil {
		return nil
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return nil
}
