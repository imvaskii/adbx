//go:build linux

package discovery

import (
	"net"
	"testing"
)

// Real avahi-browse -r -p output observed from a Pixel 8 Pro.
const sampleResolved = `=;wlan0;IPv4;adb-3B071FDJG000EH-a5UfMz;_adb-tls-pairing._tcp;local;Android_EFGAH8N4.local;192.168.1.12;43071;"v=1"`
const sampleIPv6 = `=;wlan0;IPv6;adb-3B071FDJG000EH-a5UfMz;_adb-tls-pairing._tcp;local;Android_EFGAH8N4.local;192.168.1.12;43071;"v=1"`
const samplePending = `+;wlan0;IPv4;adb-3B071FDJG000EH-a5UfMz;_adb-tls-pairing._tcp;local`

func TestParseAvahiLine_ResolvedIPv4(t *testing.T) {
	dev, ok := parseAvahiLine(sampleResolved, nil, DevicePairing)
	if !ok {
		t.Fatal("expected resolved IPv4 line to parse successfully")
	}
	if dev.Port != 43071 {
		t.Fatalf("expected port 43071, got %d", dev.Port)
	}
	if dev.IP.String() != "192.168.1.12" {
		t.Fatalf("expected IP 192.168.1.12, got %s", dev.IP)
	}
	if dev.Type != DevicePairing {
		t.Fatalf("expected DevicePairing type")
	}
}

func TestParseAvahiLine_IPv6Skipped(t *testing.T) {
	_, ok := parseAvahiLine(sampleIPv6, nil, DevicePairing)
	if ok {
		t.Fatal("expected IPv6 line to be skipped")
	}
}

func TestParseAvahiLine_PendingLineSkipped(t *testing.T) {
	_, ok := parseAvahiLine(samplePending, nil, DevicePairing)
	if ok {
		t.Fatal("expected pending (+) line to be skipped")
	}
}

func TestParseAvahiLine_IPFilterMatch(t *testing.T) {
	target := net.ParseIP("192.168.1.12")
	_, ok := parseAvahiLine(sampleResolved, target, DevicePairing)
	if !ok {
		t.Fatal("expected line to match target IP 192.168.1.12")
	}
}

// TestScanForConnect_TypeIsDeviceConnect verifies that parseAvahiLine sets the
// DeviceType from its dtype argument. When called with DeviceConnect (as
// scanForConnect does), the returned device must have Type == DeviceConnect
// without any post-hoc fixup at the call site.
func TestScanForConnect_TypeIsDeviceConnect(t *testing.T) {
	connectLine := `=;wlan0;IPv4;adb-3B071FDJG000EH-abc;_adb-tls-connect._tcp;local;Android.local;192.168.1.12;45323;"v=1"`
	dev, ok := parseAvahiLine(connectLine, nil, DeviceConnect)
	if !ok {
		t.Fatal("expected connect line to parse successfully")
	}
	if dev.Type != DeviceConnect {
		t.Fatalf("expected DeviceConnect type, got %v", dev.Type)
	}
	if dev.Port != 45323 {
		t.Fatalf("expected port 45323, got %d", dev.Port)
	}
}
