package adb

import (
	"context"
	"testing"
)

// fakeRunner returns a runner that always produces the given output and error.
func fakeRunner(out string, err error) runner {
	return func(_ context.Context, _ ...string) (string, error) {
		return out, err
	}
}

// ---- Connect output parsing --------------------------------------------------

// A1: "connected to <addr>" → success
func TestConnect_SuccessMessage(t *testing.T) {
	r := connect(context.Background(), fakeRunner("connected to 192.168.1.42:37001\n", nil), "192.168.1.42", 37001)
	if !r.Success {
		t.Fatalf("expected success, got failure: %q", r.Message)
	}
}

// A2: "already connected" → success (idempotent reconnect)
func TestConnect_AlreadyConnected(t *testing.T) {
	r := connect(context.Background(), fakeRunner("already connected to 192.168.1.42:37001\n", nil), "192.168.1.42", 37001)
	if !r.Success {
		t.Fatalf("expected success for already-connected, got: %q", r.Message)
	}
}

// A3: "failed to connect" → failure
func TestConnect_ConnectionRefused(t *testing.T) {
	out := "failed to connect to '192.168.1.42:37001': Connection refused\n"
	r := connect(context.Background(), fakeRunner(out, nil), "192.168.1.42", 37001)
	if r.Success {
		t.Fatalf("expected failure on refused connection")
	}
	if r.NeedsPairing {
		t.Fatalf("NeedsPairing should be false for connection refused")
	}
}

// A4: "failed to authenticate" → failure + NeedsPairing
func TestConnect_NotPaired_SetsNeedsPairing(t *testing.T) {
	out := "failed to authenticate to 192.168.1.42:37001\n"
	r := connect(context.Background(), fakeRunner(out, nil), "192.168.1.42", 37001)
	if r.Success {
		t.Fatalf("expected failure for unauthenticated connect")
	}
	if !r.NeedsPairing {
		t.Fatalf("expected NeedsPairing=true when adb reports authentication failure")
	}
}

// A4b: bare "failed to connect to <addr>" (no reason suffix) → NeedsPairing
// This is the actual output Android 11+ produces when the device rejects the
// TLS handshake because this host has not been paired yet.
func TestConnect_BareFailedToConnect_SetsNeedsPairing(t *testing.T) {
	out := "failed to connect to 192.168.1.12:45323\n"
	r := connect(context.Background(), fakeRunner(out, nil), "192.168.1.12", 45323)
	if r.Success {
		t.Fatalf("expected failure")
	}
	if !r.NeedsPairing {
		t.Fatalf("expected NeedsPairing=true for bare failed-to-connect (Android TLS rejection)")
	}
}

// ---- Pair output parsing -----------------------------------------------------

// A5: "Successfully paired" → success
func TestPair_SuccessMessage(t *testing.T) {
	out := "Successfully paired to 192.168.1.42:40123 [guid=adb-XXXX]\n"
	r := pair(context.Background(), fakeRunner(out, nil), "192.168.1.42", 40123, "482910")
	if !r.Success {
		t.Fatalf("expected success, got: %q", r.Message)
	}
}

// A6: "incorrect pin" → failure with IncorrectPin=true
func TestPair_WrongCode_ContainsHint(t *testing.T) {
	out := "Failed to pair with device: incorrect pin\n"
	r := pair(context.Background(), fakeRunner(out, nil), "192.168.1.42", 40123, "000000")
	if r.Success {
		t.Fatalf("expected failure for wrong pairing code")
	}
	if r.Message == "" {
		t.Fatalf("expected non-empty message with hint")
	}
	if !r.IncorrectPin {
		t.Fatalf("expected IncorrectPin=true for wrong pin output")
	}
	if r.WindowClosed {
		t.Fatalf("expected WindowClosed=false for wrong pin output")
	}
}

// A7: pairing window closed (protocol fault) → failure with WindowClosed=true
func TestPair_WindowClosed_ContainsHint(t *testing.T) {
	out := "error: protocol fault (couldn't read status message): Success\n"
	r := pair(context.Background(), fakeRunner(out, nil), "192.168.1.42", 40123, "482910")
	if r.Success {
		t.Fatalf("expected failure when pairing window closed")
	}
	if !r.WindowClosed {
		t.Fatalf("expected WindowClosed=true for protocol fault output")
	}
	if r.IncorrectPin {
		t.Fatalf("expected IncorrectPin=false for protocol fault output")
	}
}

// ---- Daemon startup noise ----------------------------------------------------

// A8: adb daemon startup lines prepended to a bare failed-to-connect message.
// This happens when the daemon was not running; adb starts it automatically,
// then attempts the connect. The daemon lines must be stripped so the parser
// can still detect NeedsPairing from the actual result line.
func TestConnect_DaemonStartup_NeedsPairing(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\n* daemon started successfully\nfailed to connect to 192.168.1.12:45951\n"
	r := connect(context.Background(), fakeRunner(out, nil), "192.168.1.12", 45951)
	if r.Success {
		t.Fatalf("expected failure")
	}
	if !r.NeedsPairing {
		t.Fatalf("expected NeedsPairing=true — daemon noise must not mask the TLS rejection line")
	}
	if !r.DaemonStarted {
		t.Fatalf("expected DaemonStarted=true when daemon startup lines are present")
	}
}

// A9: daemon startup followed by a successful connect (daemon was down, adb
// started it, then connected immediately).
func TestConnect_DaemonStartup_Success(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\n* daemon started successfully\nconnected to 192.168.1.42:37001\n"
	r := connect(context.Background(), fakeRunner(out, nil), "192.168.1.42", 37001)
	if !r.Success {
		t.Fatalf("expected success after daemon startup: %q", r.Message)
	}
	if !r.DaemonStarted {
		t.Fatalf("expected DaemonStarted=true when daemon startup lines are present")
	}
}

// A10: stripDaemonLines with no daemon lines returns the input unchanged and
// daemonStarted=false.
func TestStripDaemonLines_NoDaemonLines(t *testing.T) {
	in := "connected to 192.168.1.42:37001\n"
	cleaned, started := stripDaemonLines(in)
	if started {
		t.Fatalf("expected daemonStarted=false when no daemon lines present")
	}
	if cleaned != "connected to 192.168.1.42:37001" {
		t.Fatalf("unexpected cleaned output: %q", cleaned)
	}
}

// A11: daemon startup prepended to a successful pair response.
// parsePairOutput used to receive raw output without stripping — latent bug
// when the daemon was down. pair() must now strip before parsing and set
// DaemonStarted on the result.
func TestPair_DaemonStartup_SetsDaemonStarted(t *testing.T) {
	out := "* daemon not running; starting now at tcp:5037\n* daemon started successfully\nSuccessfully paired to 192.168.1.42:40123 [guid=adb-XXXX]\n"
	r := pair(context.Background(), fakeRunner(out, nil), "192.168.1.42", 40123, "482910")
	if !r.Success {
		t.Fatalf("expected success after daemon startup: %q", r.Message)
	}
	if !r.DaemonStarted {
		t.Fatalf("expected DaemonStarted=true when daemon startup lines are present")
	}
}
