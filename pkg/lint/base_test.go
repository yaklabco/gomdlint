package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBaseRuleFromMeta(t *testing.T) {
	meta := RuleMeta{
		ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"},
		Category: CategoryLists, Tags: []string{"bullet", "ul"}, RuleTags: []string{"lists", "style"},
		Fixable: true, Summary: "Unordered list style should be consistent",
		Description: "Every bullet in a list is the same marker.",
	}

	rule := NewBaseRuleFromMeta(meta)

	assert.Equal(t, "MD004", rule.ID())
	assert.Equal(t, "unordered-list-style", rule.Name())
	assert.Equal(t, "Unordered list style should be consistent", rule.Description(),
		"the rule describes itself by the entry's summary")
	assert.Equal(t, []string{"lists", "style"}, rule.Tags(), "the rule reports the entry's own tags")
	assert.True(t, rule.CanFix())

	tags := rule.Tags()
	tags[0] = overwritten
	assert.Equal(t, []string{"lists", "style"}, meta.RuleTags, "the rule keeps its own copy of the tags")
}

func TestNewBaseRuleFromMetaWithoutRuleTags(t *testing.T) {
	meta := RuleMeta{ID: "MD035", Name: "hr-style", Tags: []string{"hr"}, Summary: "Horizontal rule style"}

	rule := NewBaseRuleFromMeta(meta)

	assert.Equal(t, []string{"hr"}, rule.Tags(), "an entry without RuleTags gives the rule its Tags")
	assert.False(t, rule.CanFix())

	tags := rule.Tags()
	tags[0] = overwritten
	assert.Equal(t, []string{"hr"}, meta.Tags, "the rule keeps its own copy of the tags")
}
