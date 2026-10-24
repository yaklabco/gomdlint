package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRuleIDSets pins the sizes of the sets of markdownlint's rules and
// gomdlint's own, which TestCatalogNamesMarkdownlintsRules in pkg/lint/rules
// checks against the catalog, so that neither holds an ID the catalog lacks.
func TestRuleIDSets(t *testing.T) {
	t.Parallel()

	assert.Len(t, markdownlintRuleIDs, 53)
	assert.Len(t, gomdlintRuleIDs, 2)
}
