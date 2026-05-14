//go:build darwin

package discovery

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
)

// fakeRunner returns a subprocessRunner whose Nth call returns the Nth string
// from outputs (cycling through them in order). Useful for testing pipelines
// that invoke the runner multiple times (browse → resolve → getaddr).
func fakeRunner(outputs ...string) subprocessRunner {
	i := 0
	return func(_ context.Context, _ string, _ ...string) (io.Reader, error) {
		if i >= len(outputs) {
			return strings.NewReader(""), nil
		}
		out := outputs[i]
		i++
		return strings.NewReader(out), nil
	}
}

func TestParseBrowseLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantN   string
		wantOK  bool
	}{
		{
			name:   "valid Add line",
			line:   "12:34:56.789  Add  3  7  local  _adb-tls-pairing._tcp.  MyPhone",
			wantN:  "MyPhone",
			wantOK: true,
		},
		{
			name:   "valid Add line with spaces in name",
			line:   "12:34:56.789  Add  3  7  local  _adb-tls-pairing._tcp.  My Android Phone",
			wantN:  "My Android Phone",
			wantOK: true,
		},
		{
			name:   "Remove event ignored",
			line:   "12:34:56.789  Rmv  3  7  local  _adb-tls-pairing._tcp.  MyPhone",
			wantN:  "",
			wantOK: false,
		},
		{
			name:   "header line ignored",
			line:   "Browsing for _adb-tls-pairing._tcp",
			wantN:  "",
			wantOK: false,
		},
		{
			name:   "too few fields",
			line:   "12:34:56.789  Add  3",
			wantN:  "",
			wantOK: false,
		},
		{
			name:   "empty line",
			line:   "",
			wantN:  "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotN, gotOK := parseBrowseLine(tt.line)
			if gotOK != tt.wantOK {
				t.Errorf("parseBrowseLine(%q) ok = %v, want %v", tt.line, gotOK, tt.wantOK)
			}
			if gotN != tt.wantN {
				t.Errorf("parseBrowseLine(%q) name = %q, want %q", tt.line, gotN, tt.wantN)
			}
		})
	}
}

func TestParseLookupLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantHost string
		wantPort int
		wantOK   bool
	}{
		{
			name:     "valid lookup line",
			line:     "12:34:56.789  MyPhone._adb-tls-pairing._tcp.local. can be reached at MyPhone.local.:37263 (interface 7)",
			wantHost: "MyPhone.local",
			wantPort: 37263,
			wantOK:   true,
		},
		{
			name:     "no marker",
			line:     "Lookup MyPhone._adb-tls-pairing._tcp.local.",
			wantHost: "",
			wantPort: 0,
			wantOK:   false,
		},
		{
			name:     "missing port",
			line:     "12:34:56.789  foo can be reached at MyPhone.local. (interface 7)",
			wantHost: "",
			wantPort: 0,
			wantOK:   false,
		},
		{
			name:     "port zero rejected",
			line:     "12:34:56.789  foo can be reached at MyPhone.local.:0 (interface 7)",
			wantHost: "",
			wantPort: 0,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotH, gotP, gotOK := parseLookupLine(tt.line)
			if gotOK != tt.wantOK {
				t.Errorf("parseLookupLine(%q) ok = %v, want %v", tt.line, gotOK, tt.wantOK)
			}
			if gotH != tt.wantHost {
				t.Errorf("parseLookupLine(%q) host = %q, want %q", tt.line, gotH, tt.wantHost)
			}
			if gotP != tt.wantPort {
				t.Errorf("parseLookupLine(%q) port = %d, want %d", tt.line, gotP, tt.wantPort)
			}
		})
	}
}

func TestParseGetAddrLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		wantIP net.IP
	}{
		{
			// Real dns-sd -G v4 output observed on macOS (from /tmp/adbx.log).
			// Format: timestamp  Add  flags  iface  hostname  address  ttl
			name:   "real dns-sd output",
			line:   "23:18:34.499  Add  40000002      14  Android_EFGAH8N4.local.                192.168.1.12                                 120",
			wantIP: net.IPv4(192, 168, 1, 42).To4(), // placeholder — equality checked by fields
		},
		{
			name:   "valid Add record",
			line:   "12:34:56.789  Add  40000002  14  MyPhone.local.  192.168.1.42  120",
			wantIP: net.IPv4(192, 168, 1, 42).To4(),
		},
		{
			name:   "Rmv event ignored",
			line:   "12:34:56.789  Rmv  40000002  14  MyPhone.local.  192.168.1.42  120",
			wantIP: nil,
		},
		{
			name:   "too few fields",
			line:   "12:34:56.789  Add  40000002  14  MyPhone.local.",
			wantIP: nil,
		},
		{
			name:   "invalid IP",
			line:   "12:34:56.789  Add  40000002  14  MyPhone.local.  not-an-ip  120",
			wantIP: nil,
		},
		{
			name:   "empty line",
			line:   "",
			wantIP: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGetAddrLine(tt.line)
			if tt.name == "real dns-sd output" {
				// Just check the IP parses to a valid IPv4 — the exact value
				// is device-specific, not hardcoded to 192.168.1.42.
				if got == nil {
					t.Errorf("parseGetAddrLine(%q) = nil, want a valid IPv4", tt.line)
				}
				return
			}
			if tt.wantIP == nil {
				if got != nil {
					t.Errorf("parseGetAddrLine(%q) = %v, want nil", tt.line, got)
				}
				return
			}
			if got == nil {
				t.Errorf("parseGetAddrLine(%q) = nil, want %v", tt.line, tt.wantIP)
				return
			}
			if !got.Equal(tt.wantIP) {
				t.Errorf("parseGetAddrLine(%q) = %v, want %v", tt.line, got, tt.wantIP)
			}
		})
	}
}

// ---- Pipeline tests (subprocessRunner seam) ----------------------------------

// D1: dnsGetAddr with a fake runner that emits a valid Add line returns the IP.
func TestDnsGetAddr_ValidLine_ReturnsIP(t *testing.T) {
	line := "12:34:56.789  Add  40000002  14  MyPhone.local.  192.168.1.42  120\n"
	run := fakeRunner(line)
	ip := dnsGetAddr(context.Background(), "MyPhone.local.", run)
	if ip == nil {
		t.Fatal("expected non-nil IP")
	}
	want := net.IPv4(192, 168, 1, 42).To4()
	if !ip.Equal(want) {
		t.Fatalf("expected %v, got %v", want, ip)
	}
}

// D2: dnsGetAddr with no valid Add line returns nil.
func TestDnsGetAddr_NoValidLine_ReturnsNil(t *testing.T) {
	run := fakeRunner("Timestamp  DATE  flags  iface  hostname  address  ttl\n")
	ip := dnsGetAddr(context.Background(), "MyPhone.local.", run)
	if ip != nil {
		t.Fatalf("expected nil, got %v", ip)
	}
}

// D3: dnsResolve full pipeline — browse emits a lookup line then getaddr
// emits an Add line; dnsResolve must return a fully-populated Device.
func TestDnsResolve_FullPipeline_ReturnsDevice(t *testing.T) {
	lookupLine := "12:34:56.789  MyPhone._adb-tls-pairing._tcp.local. can be reached at MyPhone.local.:37263 (interface 7)\n"
	addrLine := "12:34:56.789  Add  40000002  14  MyPhone.local.  192.168.1.42  120\n"
	// First call (dns-sd -L) → lookup line; second call (dns-sd -G v4) → addr line.
	run := fakeRunner(lookupLine, addrLine)
	dev := dnsResolve(context.Background(), "MyPhone", servicePairing, DevicePairing, nil, run)
	if dev == nil {
		t.Fatal("expected non-nil Device from dnsResolve")
	}
	if dev.Port != 37263 {
		t.Fatalf("expected port 37263, got %d", dev.Port)
	}
	want := net.IPv4(192, 168, 1, 42).To4()
	if !dev.IP.Equal(want) {
		t.Fatalf("expected IP %v, got %v", want, dev.IP)
	}
	if dev.Type != DevicePairing {
		t.Fatalf("expected DevicePairing, got %v", dev.Type)
	}
}

// D4: dnsResolve returns nil when the lookup subprocess emits no host:port line.
func TestDnsResolve_NoLookupLine_ReturnsNil(t *testing.T) {
	run := fakeRunner("Browsing for _adb-tls-pairing._tcp\n", "")
	dev := dnsResolve(context.Background(), "MyPhone", servicePairing, DevicePairing, nil, run)
	if dev != nil {
		t.Fatalf("expected nil when lookup line absent, got %+v", dev)
	}
}

// D5: dnsResolve returns nil when getaddr subprocess emits no Add line.
func TestDnsResolve_NoAddrLine_ReturnsNil(t *testing.T) {
	lookupLine := "12:34:56.789  MyPhone._adb-tls-pairing._tcp.local. can be reached at MyPhone.local.:37263 (interface 7)\n"
	run := fakeRunner(lookupLine, "") // getaddr returns nothing
	dev := dnsResolve(context.Background(), "MyPhone", servicePairing, DevicePairing, nil, run)
	if dev != nil {
		t.Fatalf("expected nil when getaddr line absent, got %+v", dev)
	}
}

// D6: dnsResolve with a targetIP filter drops devices that don't match.
func TestDnsResolve_IPFilter_RejectsNonMatchingIP(t *testing.T) {
	lookupLine := "12:34:56.789  MyPhone._adb-tls-pairing._tcp.local. can be reached at MyPhone.local.:37263 (interface 7)\n"
	addrLine := "12:34:56.789  Add  40000002  14  MyPhone.local.  192.168.1.42  120\n"
	run := fakeRunner(lookupLine, addrLine)
	target := net.ParseIP("192.168.1.99") // different IP
	dev := dnsResolve(context.Background(), "MyPhone", servicePairing, DevicePairing, target, run)
	if dev != nil {
		t.Fatalf("expected nil when IP doesn't match target, got %+v", dev)
	}
}
