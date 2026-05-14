package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/imvaskii/adbx/internal/adb"
	adbxlog "github.com/imvaskii/adbx/internal/log"
	"github.com/imvaskii/adbx/internal/ui"
)

func main() {
	adbxlog.Init()
	if !adb.IsAvailable() {
		fmt.Fprintln(os.Stderr, "error: adb not found on PATH")
		fmt.Fprintln(os.Stderr, "Install with: sudo pacman -S android-tools")
		os.Exit(1)
	}

	// `adbx diag <ip:port>` — prints the exact raw output from adb connect
	// so we know which output string to parse against.
	if len(os.Args) == 3 && os.Args[1] == "diag" {
		runDiag(os.Args[2])
		return
	}

	p := tea.NewProgram(
		ui.New(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "adbx: %v\n", err)
		os.Exit(1)
	}
}

func runDiag(addr string) {
	fmt.Printf("=== adb connect %s ===\n", addr)
	out, err := runRaw("connect", addr)
	fmt.Printf("stdout+stderr:\n%q\n", out)
	fmt.Printf("exit err: %v\n\n", err)

	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 {
		host := parts[0]
		fmt.Printf("=== adb pair %s <code> ===\n", addr)
		fmt.Printf("(using dummy code 000000 — expected to fail)\n")
		out2, err2 := runRaw("pair", host+":"+parts[1], "000000")
		fmt.Printf("stdout+stderr:\n%q\n", out2)
		fmt.Printf("exit err: %v\n", err2)
	}
}

func runRaw(args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "adb", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
