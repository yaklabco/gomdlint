package lint_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

func TestRuleLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "no-trailing-spaces (MD009)", lint.RuleLabel("MD009", "no-trailing-spaces"))
	assert.Equal(t, "MD009", lint.RuleLabel("MD009", ""), "a rule without a name")
}

func TestRuleErrorLeadsWithRuleName(t *testing.T) {
	t.Parallel()

	cause := errors.New("frobnicator missing")

	assert.Equal(t, "rule no-trailing-spaces (MD009): frobnicator missing",
		(&lint.RuleError{RuleID: "MD009", RuleName: "no-trailing-spaces", Err: cause}).Error())
	assert.Equal(t, "rule MD009: frobnicator missing",
		(&lint.RuleError{RuleID: "MD009", Err: cause}).Error(), "a rule without a name")
}

func TestUnknownRuleText(t *testing.T) {
	t.Parallel()

	var (
		spaces      = lint.Suggestion{Name: "no-trailing-spaces", RuleID: "MD009"}
		punctuation = lint.Suggestion{Name: "no-trailing-punctuation", RuleID: "MD026"}
		newline     = lint.Suggestion{Name: "single-trailing-newline", RuleID: "MD047"}
		table       = lint.Suggestion{Name: "table"}
	)

	tests := []struct {
		name        string
		key         string
		suggestions []lint.Suggestion
		want        string
	}{
		{name: "no suggestion", key: "MD999", want: `unknown rule "MD999"`},
		{name: "a rule", key: "no-trailling-spaces", suggestions: []lint.Suggestion{spaces},
			want: `unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?)`},
		{name: "a tag", key: "tables", suggestions: []lint.Suggestion{table},
			want: `unknown rule "tables" (did you mean the tag "table"?)`},
		{name: "two", key: "trailing", suggestions: []lint.Suggestion{punctuation, spaces},
			want: `unknown rule "trailing" (did you mean "no-trailing-punctuation" or "no-trailing-spaces"?)`},
		{name: "three with a tag", key: "x", suggestions: []lint.Suggestion{punctuation, table, newline},
			want: `unknown rule "x" (did you mean "no-trailing-punctuation", the tag "table" or "single-trailing-newline"?)`},
		{name: "a key that needs quoting", key: "a \"b\"", want: `unknown rule "a \"b\""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, lint.UnknownRuleText(tt.key, tt.suggestions))
		})
	}
}
