package ui

import (
	"net"
	"testing"

	"github.com/imvaskii/adbx/internal/discovery"
)

func TestResolveHost_NonNilIP(t *testing.T) {
	dev := &discovery.Device{IP: net.ParseIP("192.168.1.1"), Host: "myphone.local"}
	got := resolveHost(dev)
	if got != "192.168.1.1" {
		t.Fatalf("expected IP string, got %q", got)
	}
}

func TestResolveHost_NilIP_UsesHost(t *testing.T) {
	dev := &discovery.Device{IP: nil, Host: "myphone.local"}
	got := resolveHost(dev)
	if got != "myphone.local" {
		t.Fatalf("expected host, got %q", got)
	}
}

func TestResolveHost_NilIP_TrailingDotStripped(t *testing.T) {
	dev := &discovery.Device{IP: nil, Host: "myphone.local."}
	got := resolveHost(dev)
	if got != "myphone.local" {
		t.Fatalf("expected trailing dot stripped, got %q", got)
	}
}

func TestAddrStr_WithNilIP_UsesHost(t *testing.T) {
	got := addrStr(nil, "myphone.local", 37000)
	if got != "myphone.local:37000" {
		t.Fatalf("expected host:port, got %q", got)
	}
}

func TestAddrStr_WithValidIP(t *testing.T) {
	ip := net.ParseIP("192.168.1.42").To4()
	got := addrStr(ip, "myphone.local", 37000)
	if got != "192.168.1.42:37000" {
		t.Fatalf("expected IP:port, got %q", got)
	}
}
