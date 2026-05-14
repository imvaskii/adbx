// Package log provides a lightweight debug logger that always writes to
// /tmp/adbx.log (appended). It is backed by log/slog with a text handler.
// All calls are no-ops until Init is called.
package log

import (
	"log/slog"
	"os"
)

const defaultPath = "/tmp/adbx.log"

var logger *slog.Logger

// Init opens (or creates) the log file at path in append mode and installs
// a DEBUG-level slog text handler. Call once from main before tea.NewProgram.
func Init() {
	f, err := os.OpenFile(defaultPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// Can't open log file — silently degrade, don't crash the TUI.
		return
	}
	logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	logger.Info("adbx started", "log", defaultPath)
}

// Debug logs a debug message with optional key-value pairs.
func Debug(msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs.
func Info(msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.Info(msg, args...)
}
