/*
do-dyndns is a simple dynamic DNS client for DigitalOcean.
It updates one or more DNS records with the current public IP address.
It is intended to be run as a cron job or a systemd service.
*/
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// customLogHandler implements a custom slog.Handler that formats logs as:
// [TIME] [LEVEL] [PROG] message.
type customLogHandler struct {
	out         io.Writer
	programName string
	level       slog.Level
	mu          sync.Mutex
}

// Enabled implements slog.Handler.
func (h *customLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler.
func (h *customLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format: [TIME] [LEVEL] [PROG] message
	timeStr := fmt.Sprintf("[%s]", r.Time.UTC().Format("2006-01-02T15:04:05.000Z"))
	levelStr := fmt.Sprintf("[%s]", strings.ToUpper(r.Level.String()))
	progStr := fmt.Sprintf("[%s]", h.programName)

	// Format the message.
	msg := r.Message

	// Write the formatted log entry.
	_, err := fmt.Fprintf(h.out, "%s %s %s %s\n", timeStr, levelStr, progStr, msg)

	return err
}

// WithAttrs implements slog.Handler.
func (h *customLogHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	// We don't use attributes in our format, so just return the same handler
	return h
}

// WithGroup implements slog.Handler.
func (h *customLogHandler) WithGroup(_ string) slog.Handler {
	// We don't use groups in our format, so just return the same handler
	return h
}

// LogFile name.
const LogFile = "out.log"

// initLogger initializes slog with a bracketed format.
func initLogger(logfile string) (err error) {
	var logDir string

	// If logfile is explicitly set, use it.
	if logfile != "" {
		logDir = filepath.Dir(logfile)
	} else {
		// Otherwise, use the user cache directory.
		// On Linux, this is $HOME/.cache.
		var userCacheDir string
		userCacheDir, err = os.UserCacheDir()
		if err != nil {
			return err
		}
		logDir = filepath.Join(userCacheDir, Prog)
		logfile = filepath.Join(logDir, LogFile)
	}

	// Create the log directory if it doesn't exist.
	if _, err = os.Stat(logDir); err != nil {
		if err = os.MkdirAll(logDir, 0755); err != nil {
			return err
		}
	}

	// Create or open the log file.
	f, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Create a custom handler
	handler := &customLogHandler{
		out:         f,
		programName: Prog,
		level:       slog.LevelInfo,
	}

	// Create logger with the custom handler.
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
