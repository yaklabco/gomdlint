package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

func TestRulesCommand_RuleFormatFlag(t *testing.T) {
	cmd := newRulesCommand(lint.NewRegistry())
	flag := cmd.Flags().Lookup("rule-format")
	assert.NotNil(t, flag)
}
