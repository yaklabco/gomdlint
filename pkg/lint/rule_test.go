package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnostic_HasRuleName(t *testing.T) {
	diag := Diagnostic{
		RuleID:   "MD009",
		RuleName: "no-trailing-spaces",
		Message:  "trailing spaces found",
	}
	assert.Equal(t, "MD009", diag.RuleID)
	assert.Equal(t, "no-trailing-spaces", diag.RuleName)
	assert.Equal(t, "trailing spaces found", diag.Message)
}
