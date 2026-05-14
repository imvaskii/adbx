// Package discovery implements pure-Go mDNS scanning for Android wireless
// debugging services. Android 10+ advertises two mDNS service types when
// wireless debugging is enabled:
//
//   - _adb-tls-connect._tcp  — device is ready to accept connections
//   - _adb-tls-pairing._tcp  — device is in pairing mode (shows a code)
package discovery

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// DeviceType distinguishes how to interact with a discovered device.
type DeviceType int

const (
	DeviceConnect DeviceType = iota // already paired, just connect
	DevicePairing                   // needs pairing code first
)

// Device holds all information resolved from mDNS for a single Android device.
type Device struct {
	Name string
	Host string
	IP   net.IP
	Port int
	Type DeviceType
}

const (
	serviceConnect = "_adb-tls-connect._tcp"
	servicePairing = "_adb-tls-pairing._tcp"
	scanTimeout    = 4 * time.Second
)

// mdnsBrowser is the interface around zeroconf.Resolver.Browse.
// Extracted so Scan can be tested without a real network by injecting a fake.
type mdnsBrowser interface {
	Browse(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error
}

// realBrowser wraps a *zeroconf.Resolver to satisfy mdnsBrowser.
type realBrowser struct{ r *zeroconf.Resolver }

func (b realBrowser) Browse(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
	return b.r.Browse(ctx, service, domain, entries)
}

// newRealBrowser creates a zeroconf resolver and wraps it.
// Returns an error if the underlying multicast socket cannot be opened.
func newRealBrowser() (mdnsBrowser, error) {
	r, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}
	return realBrowser{r}, nil
}

// Scan performs a concurrent mDNS browse for both Android wireless debug
// service types and returns all discovered devices within the timeout window.
// Returns the first browse error encountered, if any; a non-nil error is
// distinct from finding zero devices.
func Scan(ctx context.Context) ([]Device, error) {
	adbxlog.Info("discovery: Scan started")
	results := make(chan Device, 32)
	errc := make(chan error, 2)

	scanCtx, cancel := context.WithTimeout(ctx, scanTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		b, err := newRealBrowser()
		if err != nil {
			errc <- err
			return
		}
		browse(scanCtx, b, serviceConnect, DeviceConnect, results, errc)
	}()
	go func() {
		defer wg.Done()
		b, err := newRealBrowser()
		if err != nil {
			errc <- err
			return
		}
		browse(scanCtx, b, servicePairing, DevicePairing, results, errc)
	}()

	// Close results only after both goroutines have finished writing.
	go func() {
		wg.Wait()
		close(results)
		close(errc)
	}()

	var devices []Device
	seen := make(map[string]bool)
	for d := range results {
		key := d.IP.String() + ":" + itoa(d.Port)
		if !seen[key] {
			seen[key] = true
			devices = append(devices, d)
			adbxlog.Debug("discovery: Scan found device", "name", d.Name, "ip", d.IP, "port", d.Port, "type", d.Type)
		}
	}

	// Collect any browse errors (errc is closed by the goroutine above).
	var errs []error
	for err := range errc {
		errs = append(errs, err)
	}

	adbxlog.Info("discovery: Scan complete", "count", len(devices))
	return devices, errors.Join(errs...)
}

// browse calls b.Browse and then blocks, draining the entries channel into out
// until zeroconf closes it (which happens when ctx expires). This ensures the
// caller's WaitGroup only completes after the full scan window has elapsed.
func browse(ctx context.Context, b mdnsBrowser, service string, dtype DeviceType, out chan<- Device, errc chan<- error) {
	entries := make(chan *zeroconf.ServiceEntry)
	if err := b.Browse(ctx, service, "local.", entries); err != nil {
		errc <- err
		return
	}
	// Block here: zeroconf closes entries when ctx is cancelled/expired.
	for entry := range entries {
		ip := pickIP(entry.AddrIPv4)
		if ip == nil {
			ip = pickIP6(entry.AddrIPv6)
		}
		if ip == nil {
			continue
		}
		out <- Device{
			Name: entry.ServiceInstanceName(),
			Host: entry.HostName,
			IP:   ip,
			Port: entry.Port,
			Type: dtype,
		}
	}
}

// WatchForPairing continuously watches for _adb-tls-pairing._tcp entries
// matching targetIP. The implementation is platform-specific (avahi-browse
// on Linux, dns-sd on macOS). The channel is closed when ctx is cancelled.
func WatchForPairing(ctx context.Context, targetIP net.IP) <-chan Device {
	return watchForPairing(ctx, targetIP)
}

// ScanForConnect does a short targeted browse for _adb-tls-connect._tcp
// entries matching targetIP. Returns the first match or nil if ctx expires.
// The implementation is platform-specific (avahi-browse on Linux, dns-sd on macOS).
func ScanForConnect(ctx context.Context, targetIP net.IP) *Device {
	return scanForConnect(ctx, targetIP)
}

func pickIP(ips []net.IP) net.IP {
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4
		}
	}
	return nil
}

func pickIP6(ips []net.IP) net.IP {
	for _, ip := range ips {
		if ip != nil {
			return ip
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [10]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
