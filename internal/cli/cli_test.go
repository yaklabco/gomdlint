package cli_test

import (
	"bytes"
	"testing"

	"github.com/yaklabco/gomdlint/internal/cli"
)

func TestNewRootCommand(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test-version",
		Commit:  "test-commit",
		Date:    "test-date",
	}

	cmd := cli.NewRootCommand(info)

	if cmd == nil {
		t.Fatal("NewRootCommand returned nil")
	}

	if cmd.Use != "gomdlint" {
		t.Errorf("expected Use to be 'mdlint', got %q", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short description to be set")
	}

	if cmd.Long == "" {
		t.Error("expected Long description to be set")
	}
}

func TestRootCommandHasSubcommands(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	expectedSubcommands := []string{"lint", "rules", "init", "version"}

	for _, name := range expectedSubcommands {
		subCmd, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Errorf("expected subcommand %q to exist, got error: %v", name, err)
			continue
		}

		if subCmd.Name() != name {
			t.Errorf("expected subcommand name %q, got %q", name, subCmd.Name())
		}
	}
}

func TestLintCommandFlags(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)
	lintCmd, _, err := cmd.Find([]string{"lint"})
	if err != nil {
		t.Fatalf("lint command not found: %v", err)
	}

	expectedFlags := []string{
		"fix",
		"dry-run",
		"format",
		"jobs",
		"ignore",
		"enable",
		"disable",
		"fix-rules",
		"no-backups",
		"flavor",
	}

	for _, flagName := range expectedFlags {
		flag := lintCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q to exist on lint command", flagName)
		}
	}
}

func TestGlobalFlags(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	expectedFlags := []string{"debug", "config"}

	for _, flagName := range expectedFlags {
		flag := cmd.PersistentFlags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected global flag %q to exist", flagName)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "1.2.3",
		Commit:  "abc123",
		Date:    "2024-01-01",
	}

	cmd := cli.NewRootCommand(info)
	cmd.SetArgs([]string{"version"})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	// Version command uses charmbracelet/log which writes to stdout directly,
	// so we just verify it doesn't error.
}

func TestLintCommandAcceptsArbitraryArgs(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)
	lintCmd, _, err := cmd.Find([]string{"lint"})
	if err != nil {
		t.Fatalf("lint command not found: %v", err)
	}

	// Test that lint command accepts arbitrary args (file paths).
	err = lintCmd.Args(lintCmd, []string{"file1.md", "file2.md", "docs/"})
	if err != nil {
		t.Errorf("lint command should accept arbitrary args, got error: %v", err)
	}
}
