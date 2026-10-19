package ruledocs

import (
	"cmp"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// otherTitle titles the group of rules without a category.
const otherTitle = "Other"

// categoryGroup is the rules of one category, in listing order.
type categoryGroup struct {
	category lint.Category
	entries  []Entry
}

// title returns the title of the group's category.
func (g categoryGroup) title() string {
	if g.category == "" {
		return otherTitle
	}
	return g.category.Title()
}

// groups returns entries by category, in the order of lint.Categories, then
// any other categories by key, then the rules without a category. Within a
// category, rules are in order of the identifier that ruleFormat shows
// first: the name, or the ID for the id and combined formats.
func groups(entries []Entry, ruleFormat config.RuleFormat) []categoryGroup {
	byCategory := make(map[lint.Category][]Entry)
	for _, entry := range entries {
		byCategory[entry.Category] = append(byCategory[entry.Category], entry)
	}

	order := lint.Categories()
	var others []lint.Category
	for category := range byCategory {
		if category != "" && !slices.Contains(order, category) {
			others = append(others, category)
		}
	}
	slices.Sort(others)
	order = append(append(order, others...), "")

	byID := ruleFormat == config.RuleFormatID || ruleFormat == config.RuleFormatCombined
	listed := make([]categoryGroup, 0, len(byCategory))
	for _, category := range order {
		members := byCategory[category]
		if len(members) == 0 {
			continue
		}
		slices.SortFunc(members, func(left, right Entry) int {
			if byID {
				return cmp.Or(cmp.Compare(left.ID, right.ID), cmp.Compare(left.Name, right.Name))
			}
			return cmp.Or(cmp.Compare(left.Name, right.Name), cmp.Compare(left.ID, right.ID))
		})
		listed = append(listed, categoryGroup{category: category, entries: members})
	}
	return listed
}

// ordered returns entries in listing order: by category, then by name.
func ordered(entries []Entry) []Entry {
	listed := make([]Entry, 0, len(entries))
	for _, group := range groups(entries, config.RuleFormatName) {
		listed = append(listed, group.entries...)
	}
	return listed
}

// Filter selects the rules of a listing. A rule must pass every filter that
// is set, and, for a filter with several values, match one of them.
type Filter struct {
	// Words must each appear, ignoring case, in the rule's name, an alias,
	// its ID, its category's key, a tag, or its description.
	Words []string

	// Categories are the categories whose rules to list.
	Categories []lint.Category

	// Tags are the markdownlint tags whose rules to list, in any case, and
	// with any whitespace around them.
	Tags []string

	// Fixable lists only the rules that can fix what they find.
	Fixable bool

	// Enabled lists only the rules that run, and Disabled only those that
	// do not.
	Enabled  bool
	Disabled bool
}

// Matches reports whether entry passes the filter.
func (f *Filter) Matches(entry Entry) bool {
	switch {
	case f.Fixable && !entry.Fixable,
		f.Enabled && !entry.State.Enabled,
		f.Disabled && entry.State.Enabled,
		len(f.Categories) > 0 && !slices.Contains(f.Categories, entry.Category),
		len(f.Tags) > 0 && !slices.ContainsFunc(f.Tags, func(tag string) bool { return hasTag(entry, tag) }):
		return false
	}

	fields := searchFields(entry)
	for _, word := range f.Words {
		word = strings.ToLower(word)
		if !slices.ContainsFunc(fields, func(field string) bool { return strings.Contains(field, word) }) {
			return false
		}
	}
	return true
}

// Select returns the entries that pass the filter, in their order.
func (f *Filter) Select(entries []Entry) []Entry {
	var selected []Entry
	for _, entry := range entries {
		if f.Matches(entry) {
			selected = append(selected, entry)
		}
	}
	return selected
}

// hasTag reports whether entry lists tag, ignoring case and the whitespace
// around tag.
func hasTag(entry Entry, tag string) bool {
	tag = strings.TrimSpace(tag)
	return slices.ContainsFunc(entry.Tags, func(own string) bool { return strings.EqualFold(own, tag) })
}

// searchFields returns the texts of entry that the words of a filter look
// in, lower-cased.
func searchFields(entry Entry) []string {
	own := [...]string{entry.ID, entry.Name, string(entry.Category), entry.Description}
	fields := make([]string, 0, len(own)+len(entry.Aliases)+len(entry.Tags))
	fields = append(fields, own[:]...)
	fields = append(fields, entry.Aliases...)
	fields = append(fields, entry.Tags...)
	for idx, field := range fields {
		fields[idx] = strings.ToLower(field)
	}
	return fields
}
