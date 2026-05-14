package ui

import (
	"fmt"
	"net"

	"github.com/imvaskii/adbx/internal/discovery"
)

// addrStr returns a human-readable host:port string.
// If ip is nil or its string form is "<nil>", the host name is used instead.
func addrStr(ip net.IP, host string, port int) string {
	h := ip.String()
	if h == "<nil>" || h == "" {
		h = host
	}
	return fmt.Sprintf("%s:%d", h, port)
}

// resolveHost returns the best string form of a device's address for passing
// to adb. When dev.IP is set it is preferred; otherwise dev.Host is used with
// any trailing dot removed (dns-sd returns fully-qualified names like
// "MyPhone.local." that adb does not understand).
func resolveHost(dev *discovery.Device) string {
	if dev.IP != nil {
		return dev.IP.String()
	}
	h := dev.Host
	if len(h) > 0 && h[len(h)-1] == '.' {
		h = h[:len(h)-1]
	}
	return h
}
