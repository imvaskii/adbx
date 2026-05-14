package discovery

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

// ---- fake mdnsBrowser for browse() / Scan() unit tests --------------------

// fakeBrowser sends a fixed set of entries and then blocks until ctx is
// cancelled — mirroring real zeroconf behaviour where entries is closed on
// ctx expiry.
type fakeBrowser struct {
	entries []*zeroconf.ServiceEntry
}

func (f *fakeBrowser) Browse(ctx context.Context, _ string, _ string, out chan<- *zeroconf.ServiceEntry) error {
	go func() {
		for _, e := range f.entries {
			select {
			case out <- e:
			case <-ctx.Done():
				close(out)
				return
			}
		}
		<-ctx.Done()
		close(out)
	}()
	return nil
}

func makeEntry(ip string, port int) *zeroconf.ServiceEntry {
	e := zeroconf.NewServiceEntry("test", "_adb-tls-connect._tcp", "local.")
	e.AddrIPv4 = []net.IP{net.ParseIP(ip).To4()}
	e.Port = port
	return e
}

// scanWithFakes exercises the same logic as Scan but with a short 200ms
// timeout and injected fake browsers — no network required.
func scanWithFakes(ctx context.Context, connectEntries, pairingEntries []*zeroconf.ServiceEntry) ([]Device, error) {
	results := make(chan Device, 32)
	errc := make(chan error, 2)

	scanCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		browse(scanCtx, &fakeBrowser{connectEntries}, serviceConnect, DeviceConnect, results, errc)
	}()
	go func() {
		defer wg.Done()
		browse(scanCtx, &fakeBrowser{pairingEntries}, servicePairing, DevicePairing, results, errc)
	}()
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
		}
	}
	var errs []error
	for err := range errc {
		errs = append(errs, err)
	}
	return devices, errors.Join(errs...)
}

func TestScan_ReturnsDevicesFromFakeBrowser(t *testing.T) {
	connectEntries := []*zeroconf.ServiceEntry{makeEntry("192.168.1.10", 44444)}
	pairingEntries := []*zeroconf.ServiceEntry{makeEntry("192.168.1.11", 55555)}

	devices, err := scanWithFakes(context.Background(), connectEntries, pairingEntries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestScan_DeduplicatesSameDevice(t *testing.T) {
	entry := makeEntry("192.168.1.10", 44444)
	devices, err := scanWithFakes(context.Background(),
		[]*zeroconf.ServiceEntry{entry},
		[]*zeroconf.ServiceEntry{entry},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after dedup, got %d", len(devices))
	}
}

func TestScan_WaitsFullTimeout(t *testing.T) {
	start := time.Now()
	_, _ = scanWithFakes(context.Background(), nil, nil)
	elapsed := time.Since(start)
	if elapsed < 190*time.Millisecond {
		t.Fatalf("scan returned too fast (%v); expected ~200ms", elapsed)
	}
}

func TestScan_EmptyWhenNoEntries(t *testing.T) {
	devices, err := scanWithFakes(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}
