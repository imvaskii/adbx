// Package adb wraps the adb command-line binary for wireless pairing and
// connecting to Android devices. It does not implement the ADB protocol
// itself — it delegates to the system adb binary and parses its output.
//
// The runner interface is the seam used in tests to inject fake adb output
// without requiring a real device or binary.
package adb

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	adbxlog "github.com/imvaskii/adbx/internal/log"
)

// Result holds the outcome of an adb operation.
type Result struct {
	Success bool
	Message string
	// NeedsPairing is set when a connect attempt fails due to missing TLS
	// credentials — the device has not been paired with this host yet.
	NeedsPairing bool
	// IncorrectPin is set when adb pair fails because the code was wrong.
	IncorrectPin bool
	// WindowClosed is set when adb pair fails because the pairing dialog was
	// dismissed on the device before the exchange completed.
	WindowClosed bool
	// DaemonStarted is set when the adb daemon was not running and had to be
	// started automatically before the operation could proceed.
	DaemonStarted bool
}

// runner abstracts shelling out to adb so tests can inject fake output.
type runner func(ctx context.Context, args ...string) (string, error)

// defaultRunner executes the real adb binary.
func defaultRunner(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "adb", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Connect runs `adb connect <host>:<port>` and returns the result.
func Connect(ctx context.Context, host string, port int) Result {
	return connect(ctx, defaultRunner, host, port)
}

// Pair runs `adb pair <host>:<port> <code>` and returns the result.
func Pair(ctx context.Context, host string, port int, code string) Result {
	return pair(ctx, defaultRunner, host, port, code)
}

// IsAvailable checks that the adb binary is reachable on PATH.
func IsAvailable() bool {
	_, err := exec.LookPath("adb")
	return err == nil
}

// devices returns the raw output of `adb devices` for diagnostic use.
func devices(ctx context.Context) (string, error) {
	return defaultRunner(ctx, "devices")
}

// ---- internal (injectable for tests) ----------------------------------------

func connect(ctx context.Context, run runner, host string, port int) Result {
	addr := fmt.Sprintf("%s:%d", host, port)
	adbxlog.Info("adb: connect", "addr", addr)
	out, err := run(ctx, "connect", addr)
	adbxlog.Debug("adb: connect raw output", "out", strings.TrimSpace(out), "err", err)
	cleaned, daemonStarted := stripDaemonLines(out)
	result := parseConnectOutput(cleaned)
	if daemonStarted {
		result.DaemonStarted = true
		if result.Success {
			result.Message += "\n\nNote: the ADB daemon was not running and was started automatically."
		}
	}
	adbxlog.Info("adb: connect result", "success", result.Success, "needsPairing", result.NeedsPairing, "msg", result.Message)
	return result
}

func pair(ctx context.Context, run runner, host string, port int, code string) Result {
	addr := fmt.Sprintf("%s:%d", host, port)
	adbxlog.Info("adb: pair", "addr", addr)
	out, err := run(ctx, "pair", addr, code)
	adbxlog.Debug("adb: pair raw output", "out", strings.TrimSpace(out), "err", err)
	cleaned, daemonStarted := stripDaemonLines(out)
	result := parsePairOutput(cleaned)
	result.DaemonStarted = daemonStarted
	adbxlog.Info("adb: pair result", "success", result.Success, "msg", result.Message)
	return result
}

// parseConnectOutput interprets the raw text from `adb connect`.
// The input must already have daemon startup lines stripped (see connect()).
//
// Known output patterns (adb 1.0.41):
//
//	"connected to <addr>"           — success (new connection)
//	"already connected to <addr>"   — success (was already connected)
//	"failed to connect to '<addr>': Connection refused"  — port closed / wrong port
//	"failed to connect to '<addr>': Connection timed out"
//	"failed to authenticate to <addr>"  — not paired (TLS cert missing)
//	"error: ..."                        — binary-level error
func parseConnectOutput(out string) Result {
	msg := strings.TrimSpace(out)
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "already connected") || strings.HasPrefix(lower, "connected to") {
		return Result{Success: true, Message: msg}
	}

	needsPairing := strings.Contains(lower, "failed to authenticate") ||
		strings.Contains(lower, "unauthorized") ||
		// Android 11+ TLS rejection: bare "failed to connect to <addr>" with
		// no trailing reason (no colon after the address). A connection-refused
		// or timeout failure always appends ": <reason>".
		(strings.HasPrefix(lower, "failed to connect to ") && !strings.Contains(lower, ": "))

	return Result{
		Success:      false,
		Message:      msg + hintForConnectFailure(lower),
		NeedsPairing: needsPairing,
	}
}

// stripDaemonLines removes adb daemon startup noise lines from raw adb output.
// When the daemon is not running, adb prints lines like:
//
//	* daemon not running; starting now at tcp:5037
//	* daemon started successfully
//
// before the actual command output. These lines all start with "* " and must
// be stripped before the output can be parsed for success/failure patterns.
// Returns the cleaned output and whether the daemon was freshly started.
func stripDaemonLines(out string) (cleaned string, daemonStarted bool) {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "* ") {
			if strings.Contains(trimmed, "daemon started successfully") {
				daemonStarted = true
			}
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), daemonStarted
}

func hintForConnectFailure(lower string) string {
	if strings.Contains(lower, "failed to authenticate") || strings.Contains(lower, "unauthorized") {
		return "\n\nThis device has not been paired with this machine.\nEnable 'Pair device with pairing code' on the device,\nthen rescan and select the [pairing] entry."
	}
	if strings.Contains(lower, "connection refused") {
		return "\n\nConnection refused — check that Wireless Debugging is still\nenabled on the device and rescan."
	}
	if strings.Contains(lower, "timed out") {
		return "\n\nConnection timed out — ensure the device is on the same\nWi-Fi network as this machine."
	}
	return ""
}

// parsePairOutput interprets the raw text from `adb pair`.
//
// Known output patterns (adb 1.0.41):
//
//	"Successfully paired to <addr> [guid=...]"  — success
//	"Failed to pair with device: incorrect pin"  — wrong code
//	"error: protocol fault ..."                  — pairing window closed
func parsePairOutput(out string) Result {
	msg := strings.TrimSpace(out)
	lower := strings.ToLower(msg)

	if strings.Contains(lower, "successfully paired") {
		return Result{
			Success: true,
			Message: msg + "\n\nPress 'r' to rescan — the device will now appear as [connect].",
		}
	}

	hint := ""
	incorrectPin := strings.Contains(lower, "incorrect pin") || strings.Contains(lower, "wrong pin")
	windowClosed := strings.Contains(lower, "protocol fault") || strings.Contains(lower, "couldn't read status")

	if incorrectPin {
		hint = "\n\nThe pairing code was incorrect. Re-open the pairing dialog\non your device for a new code."
	} else if windowClosed {
		hint = "\n\nThe pairing window was closed on the device before pairing\ncompleted. Re-open it and try again."
	}

	return Result{Success: false, Message: msg + hint, IncorrectPin: incorrectPin, WindowClosed: windowClosed}
}
