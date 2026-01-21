package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRulesCommand_RuleFormatFlag(t *testing.T) {
	cmd := newRulesCommand()
	flag := cmd.Flags().Lookup("rule-format")
	assert.NotNil(t, flag)
}
