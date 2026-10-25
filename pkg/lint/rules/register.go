package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// RegisterAll registers all built-in rules with the given registry, with
// their catalog entries, so that the registry resolves each rule's aliases
// and tags. It records each entry before it registers the rule, so that a
// rule of another type that later replaces a built-in rule does not keep
// the built-in rule's entry (see lint.Registry.Register).
func RegisterAll(registry *lint.Registry) {
	for _, meta := range Catalog() {
		registry.Describe(meta)
	}

	// Whitespace rules
	registry.Register(NewTrailingWhitespaceRule()) // MD009
	registry.Register(NewHardTabsRule())           // MD010
	registry.Register(NewFinalNewlineRule())       // MD047
	registry.Register(NewMultipleBlankLinesRule()) // MD012
	registry.Register(NewHeadingBlankLinesRule())  // MD022

	// Heading rules
	registry.Register(NewHeadingIncrementRule())         // MD001
	registry.Register(NewHeadingStyleRule())             // MD003
	registry.Register(NewNoMissingSpaceATXRule())        // MD018
	registry.Register(NewNoMultipleSpaceATXRule())       // MD019
	registry.Register(NewNoMissingSpaceClosedATXRule())  // MD020
	registry.Register(NewNoMultipleSpaceClosedATXRule()) // MD021
	registry.Register(NewHeadingStartLeftRule())         // MD023
	registry.Register(NewNoDuplicateHeadingRule())       // MD024
	registry.Register(NewSingleH1Rule())                 // MD025
	registry.Register(NewNoTrailingPunctuationRule())    // MD026

	// List rules
	registry.Register(NewUnorderedListStyleRule())   // MD004
	registry.Register(NewListIndentRule())           // MD005
	registry.Register(NewULIndentRule())             // MD007
	registry.Register(NewOrderedListIncrementRule()) // MD029
	registry.Register(NewListMarkerSpaceRule())      // MD030
	registry.Register(NewBlanksAroundListsRule())    // MD032

	// Line length rule
	registry.Register(NewMaxLineLengthRule()) // MD013

	// Blockquote rules
	registry.Register(NewNoMultipleSpaceBlockquoteRule()) // MD027
	registry.Register(NewNoBlanksBlockquoteRule())        // MD028

	// Link and image rules
	registry.Register(NewReversedLinkRule())         // MD011
	registry.Register(NewNoBareURLsRule())           // MD034
	registry.Register(NewLinkSpacesRule())           // MD039
	registry.Register(NewEmptyLinkRule())            // MD042
	registry.Register(NewImageAltTextRule())         // MD045
	registry.Register(NewLinkDestinationStyleRule()) // MDL001

	// HR rules
	registry.Register(NewHRStyleRule()) // MD035

	// Emphasis rules
	registry.Register(NewNoEmphasisAsHeadingRule()) // MD036
	registry.Register(NewNoSpaceInEmphasisRule())   // MD037
	registry.Register(NewEmphasisStyleRule())       // MD049
	registry.Register(NewStrongStyleRule())         // MD050

	// Code block rules
	registry.Register(NewCommandsShowOutputRule()) // MD014
	registry.Register(NewBlanksAroundFencesRule()) // MD031
	registry.Register(NewNoSpaceInCodeRule())      // MD038
	registry.Register(NewCodeBlockLanguageRule())  // MD040
	registry.Register(NewCodeBlockStyleRule())     // MD046
	registry.Register(NewCodeFenceStyleRule())     // MD048

	// HTML rules
	registry.Register(NewInlineHTMLRule()) // MD033

	// Table rules (GFM)
	registry.Register(NewTablePipeStyleRule())   // MD055
	registry.Register(NewTableColumnCountRule()) // MD056
	registry.Register(NewTableBlankLinesRule())  // MD058
	registry.Register(NewTableColumnStyleRule()) // MD060
	registry.Register(NewTableAlignmentRule())   // MDL003

	// Metadata rules
	registry.Register(NewFirstLineHeadingRule()) // MD041
	registry.Register(NewRequiredHeadingsRule()) // MD043
	registry.Register(NewProperNamesRule())      // MD044

	// Reference link/image tracking rules
	registry.Register(NewLinkFragmentsRule())       // MD051
	registry.Register(NewReferenceLinkImagesRule()) // MD052
	registry.Register(NewLinkImageRefDefsRule())    // MD053
	registry.Register(NewLinkImageStyleRule())      // MD054
	registry.Register(NewDescriptiveLinkTextRule()) // MD059
}

// init registers all built-in rules with the default registry.
//
//nolint:gochecknoinits // Init is intentional for automatic rule registration
func init() {
	RegisterAll(lint.DefaultRegistry)
}
