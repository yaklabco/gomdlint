// Package logging provides a structured logging wrapper around charmbracelet/log.
package logging

import (
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

// defaultLogger is the package-level default logger instance.
//
//nolint:gochecknoglobals // Package-level logger is intentional for convenience
var (
	defaultLogger     *log.Logger
	defaultLoggerOnce sync.Once
)

func getDefaultLogger() *log.Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = New("info")
	})
	return defaultLogger
}

// New creates a new logger with the specified level.
// Valid levels: "debug", "info", "warn", "error".
func New(level string) *log.Logger {
	return NewWithWriter(os.Stderr, level)
}

// NewWithWriter creates a logger with the specified level that writes to
// writer.
func NewWithWriter(writer io.Writer, level string) *log.Logger {
	logger := newLogger(writer)
	setLoggerLevel(logger, level)
	return logger
}

// NewInteractive creates a logger for interactive CLI commands.
// It writes to stdout (not stderr) and is suitable for user-facing output
// from commands like init and migrate.
func NewInteractive() *log.Logger {
	logger := newLogger(os.Stdout)
	logger.SetLevel(log.InfoLevel)
	return logger
}

// newLogger creates a logger on writer that never queries the terminal.
//
// charmbracelet/log builds its renderer with termenv's color cache, which
// asks a terminal for its foreground and background colors (OSC 10 and 11,
// each fenced by a cursor position report) as soon as the logger exists.
// The queries ignore NO_COLOR and stall for seconds on terminals that do
// not answer. Hiding the terminal behind opaqueWriter avoids them, and the
// color profile then comes from termenv's environment checks, which honour
// NO_COLOR, CLICOLOR_FORCE and TERM without talking to the terminal.
func newLogger(writer io.Writer) *log.Logger {
	// A value, not a pointer: charm keeps a renderer per writer in a global
	// registry, and equal values reuse one entry instead of adding another
	// for every logger.
	logger := log.NewWithOptions(opaqueWriter{Writer: writer}, log.Options{
		ReportTimestamp: false,
		ReportCaller:    false,
	})
	logger.SetColorProfile(lipgloss.NewRenderer(writer).ColorProfile())
	return logger
}

// opaqueWriter forwards writes while hiding the file behind them, so termenv
// cannot detect a terminal to query.
type opaqueWriter struct {
	io.Writer
}

func setLoggerLevel(logger *log.Logger, level string) {
	switch strings.ToLower(level) {
	case "debug":
		logger.SetLevel(log.DebugLevel)
	case "info":
		logger.SetLevel(log.InfoLevel)
	case "warn", "warning":
		logger.SetLevel(log.WarnLevel)
	case "error":
		logger.SetLevel(log.ErrorLevel)
	default:
		logger.SetLevel(log.InfoLevel)
	}
}

// Default returns the package-level default logger.
func Default() *log.Logger {
	return getDefaultLogger()
}

// SetDefault sets the package-level default logger.
func SetDefault(logger *log.Logger) {
	defaultLogger = logger
}

// SetLevel updates the log level of the default logger.
func SetLevel(level string) {
	setLoggerLevel(getDefaultLogger(), level)
}
