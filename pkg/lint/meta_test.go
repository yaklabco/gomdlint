package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuleDescription(t *testing.T) {
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"} // Describes itself as "mock".

	assert.Equal(t, "Lines end cleanly.", RuleDescription(rule, RuleMeta{ID: "MD009", Description: "Lines end cleanly."}),
		"the catalog description wins")
	assert.Equal(t, "mock", RuleDescription(rule, RuleMeta{ID: "MD009", Rationale: "An entry without a description."}),
		"an entry without a description leaves the rule's own")
	assert.Equal(t, "mock", RuleDescription(rule, RuleMeta{}), "a rule without an entry describes itself")
}

func TestIsGomdlintOnlyRule(t *testing.T) {
	for id, want := range map[string]bool{
		"MDL001": true, "MDL003": true,
		"MD009": false, "MD060": false,
		"CUSTOM001": false, "": false,
	} {
		assert.Equal(t, want, IsGomdlintOnlyRule(id), "rule ID %q", id)
	}
}
