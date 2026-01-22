package cli_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/stretchr/testify/assert"
)

func TestLintCommand_RuleFormatFlag(t *testing.T) {
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

	// Check flag exists
	flag := lintCmd.Flags().Lookup("rule-format")
	assert.NotNil(t, flag, "rule-format flag should exist")
	assert.Equal(t, "name", flag.DefValue, "default value should be 'name'")
}

func TestLintCommand_SummaryOrderFlag(t *testing.T) {
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

	// Check summary-order flag exists
	flag := lintCmd.Flags().Lookup("summary-order")
	assert.NotNil(t, flag, "summary-order flag should exist")
	assert.Equal(t, "rules", flag.DefValue, "default value should be 'rules'")

	// Check format flag includes "summary"
	formatFlag := lintCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag, "format flag should exist")
	assert.Contains(t, formatFlag.Usage, "summary", "format flag help should include 'summary'")
}
