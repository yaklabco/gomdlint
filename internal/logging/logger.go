// Package logging provides a structured logging wrapper around charmbracelet/log.
package logging

import (
	"os"
	"strings"
	"sync"

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
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
		ReportCaller:    false,
	})

	setLoggerLevel(logger, level)

	return logger
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
