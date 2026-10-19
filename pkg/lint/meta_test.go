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

// TestCategoryTitles covers the titles that rule listings and documentation
// give the categories: each category has its own, and a category that
// gomdlint does not define is titled by its key.
func TestCategoryTitles(t *testing.T) {
	titled := make(map[string]Category)
	for _, category := range Categories() {
		title := category.Title()
		assert.NotEmpty(t, title, "category %q", category)
		if other, taken := titled[title]; taken {
			t.Errorf("categories %q and %q share the title %q", other, category, title)
		}
		titled[title] = category
	}

	assert.Equal(t, "Headings", CategoryHeadings.Title())
	assert.Equal(t, "Whitespace and layout", CategoryLayout.Title())
	assert.Equal(t, "Links and images", CategoryLinks.Title())
	assert.Equal(t, "HTML", CategoryHTML.Title())
	assert.Equal(t, "custom", Category("custom").Title())
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
