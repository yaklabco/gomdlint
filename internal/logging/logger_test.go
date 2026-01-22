package logging_test

import (
	"testing"

	"github.com/charmbracelet/log"

	"github.com/jamesainslie/gomdlint/internal/logging"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		level    string
		expected log.Level
	}{
		{"debug level", "debug", log.DebugLevel},
		{"info level", "info", log.InfoLevel},
		{"warn level", "warn", log.WarnLevel},
		{"warning level", "warning", log.WarnLevel},
		{"error level", "error", log.ErrorLevel},
		{"invalid defaults to info", "invalid", log.InfoLevel},
		{"empty defaults to info", "", log.InfoLevel},
		{"case insensitive DEBUG", "DEBUG", log.DebugLevel},
		{"case insensitive Info", "Info", log.InfoLevel},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			logger := logging.New(testCase.level)
			if logger == nil {
				t.Fatal("New returned nil logger")
			}

			if logger.GetLevel() != testCase.expected {
				t.Errorf("expected level %v, got %v", testCase.expected, logger.GetLevel())
			}
		})
	}
}

func TestDefault(t *testing.T) {
	t.Parallel()

	logger := logging.Default()
	if logger == nil {
		t.Fatal("Default returned nil logger")
	}
}

func TestSetLevel(t *testing.T) {
	// Not parallel because it modifies global state.

	// Save original and restore after test.
	original := logging.Default()
	defer logging.SetDefault(original)

	// Create a fresh logger for testing.
	testLogger := logging.New("info")
	logging.SetDefault(testLogger)

	logging.SetLevel("debug")
	if logging.Default().GetLevel() != log.DebugLevel {
		t.Error("SetLevel to debug failed")
	}

	logging.SetLevel("error")
	if logging.Default().GetLevel() != log.ErrorLevel {
		t.Error("SetLevel to error failed")
	}
}

func TestSetDefault(t *testing.T) {
	// Not parallel because it modifies global state.

	original := logging.Default()
	defer logging.SetDefault(original)

	newLogger := logging.New("error")
	logging.SetDefault(newLogger)

	if logging.Default() != newLogger {
		t.Error("SetDefault did not change the default logger")
	}
}

func TestNewInteractive(t *testing.T) {
	t.Parallel()

	logger := logging.NewInteractive()
	if logger == nil {
		t.Fatal("NewInteractive returned nil logger")
	}

	// Interactive loggers should default to info level
	if logger.GetLevel() != log.InfoLevel {
		t.Errorf("expected info level, got %v", logger.GetLevel())
	}
}
