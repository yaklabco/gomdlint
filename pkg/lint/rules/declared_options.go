package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// declaredOptions returns the options that the catalog declares for the
// built-in rule with the given ID (see catalogOptions), in a list that
// shares nothing with any other.
func declaredOptions(id string) []lint.OptionSpec {
	return catalogOptions()[id]
}

// Each built-in rule declares its options itself (lint.OptionDeclarer), so
// that a registry that holds the rule without its catalog entry, as a
// program that registers rules one by one does, still rewrites
// markdownlint's spellings of them.

// DeclaredOptions returns the options of MD001, as the catalog declares them.
func (r *HeadingIncrementRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD003, as the catalog declares them.
func (r *HeadingStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD004, as the catalog declares them.
func (r *UnorderedListStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD005, as the catalog declares them.
func (r *ListIndentRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD007, as the catalog declares them.
func (r *ULIndentRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD009, as the catalog declares them.
func (r *TrailingWhitespaceRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD010, as the catalog declares them.
func (r *HardTabsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD011, as the catalog declares them.
func (r *ReversedLinkRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD012, as the catalog declares them.
func (r *MultipleBlankLinesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD013, as the catalog declares them.
func (r *MaxLineLengthRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD014, as the catalog declares them.
func (r *CommandsShowOutputRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD018, as the catalog declares them.
func (r *NoMissingSpaceATXRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD019, as the catalog declares them.
func (r *NoMultipleSpaceATXRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD020, as the catalog declares them.
func (r *NoMissingSpaceClosedATXRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD021, as the catalog declares them.
func (r *NoMultipleSpaceClosedATXRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD022, as the catalog declares them.
func (r *HeadingBlankLinesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD023, as the catalog declares them.
func (r *HeadingStartLeftRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD024, as the catalog declares them.
func (r *NoDuplicateHeadingRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD025, as the catalog declares them.
func (r *SingleH1Rule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD026, as the catalog declares them.
func (r *NoTrailingPunctuationRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD027, as the catalog declares them.
func (r *NoMultipleSpaceBlockquoteRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD028, as the catalog declares them.
func (r *NoBlanksBlockquoteRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD029, as the catalog declares them.
func (r *OrderedListIncrementRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD030, as the catalog declares them.
func (r *ListMarkerSpaceRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD031, as the catalog declares them.
func (r *BlanksAroundFencesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD032, as the catalog declares them.
func (r *BlanksAroundListsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD033, as the catalog declares them.
func (r *InlineHTMLRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD034, as the catalog declares them.
func (r *NoBareURLsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD035, as the catalog declares them.
func (r *HRStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD036, as the catalog declares them.
func (r *NoEmphasisAsHeadingRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD037, as the catalog declares them.
func (r *NoSpaceInEmphasisRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD038, as the catalog declares them.
func (r *NoSpaceInCodeRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD039, as the catalog declares them.
func (r *LinkSpacesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD040, as the catalog declares them.
func (r *CodeBlockLanguageRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD041, as the catalog declares them.
func (r *FirstLineHeadingRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD042, as the catalog declares them.
func (r *EmptyLinkRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD043, as the catalog declares them.
func (r *RequiredHeadingsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD044, as the catalog declares them.
func (r *ProperNamesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD045, as the catalog declares them.
func (r *ImageAltTextRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD046, as the catalog declares them.
func (r *CodeBlockStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD047, as the catalog declares them.
func (r *FinalNewlineRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD048, as the catalog declares them.
func (r *CodeFenceStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD049, as the catalog declares them.
func (r *EmphasisStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD050, as the catalog declares them.
func (r *StrongStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD051, as the catalog declares them.
func (r *LinkFragmentsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD052, as the catalog declares them.
func (r *ReferenceLinkImagesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD053, as the catalog declares them.
func (r *LinkImageRefDefsRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD054, as the catalog declares them.
func (r *LinkImageStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD055, as the catalog declares them.
func (r *TablePipeStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD056, as the catalog declares them.
func (r *TableColumnCountRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD058, as the catalog declares them.
func (r *TableBlankLinesRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD059, as the catalog declares them.
func (r *DescriptiveLinkTextRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MD060, as the catalog declares them.
func (r *TableColumnStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MDL001, as the catalog declares them.
func (r *LinkDestinationStyleRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}

// DeclaredOptions returns the options of MDL003, as the catalog declares them.
func (r *TableAlignmentRule) DeclaredOptions() []lint.OptionSpec {
	return declaredOptions(r.ID())
}
