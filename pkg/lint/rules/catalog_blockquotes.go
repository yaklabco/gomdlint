package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// blockquotesCatalog lists the rules that check blockquotes.
func blockquotesCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD027", Name: "no-multiple-space-blockquote", Tags: []string{"blockquote", "whitespace", "indentation"}, Fixable: true,
			Description: "At most one space follows the > of a blockquote.",
			Rationale: "Extra spaces after > make the quote harder to read. " +
				"At the start of a block, five spaces after > make an indented code block.",
			Fix: "The fix deletes the extra spaces. gomdlint reports fence lines and HTML lines inside the quote without a fix.",
		},
		{
			ID: "MD028", Name: "no-blanks-blockquote", Tags: []string{"blockquote", "whitespace"},
			Description: "No blank line sits between two blockquotes.",
			Rationale: "Parsers disagree about whether a blank line ends a blockquote, " +
				"so the quotes may render as one or as two.",
			Fix: "There is no automatic fix. Put > on the blank line to join the quotes, or text between them to keep them apart.",
		},
	}
}
