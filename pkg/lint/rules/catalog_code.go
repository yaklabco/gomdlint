package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// codeCatalog lists the rules that check code blocks and code spans.
func codeCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD014", Name: "commands-show-output", Tags: []string{"code"}, Fixable: true,
			Description: "Code blocks that show only commands leave out the $ prompt.",
			Rationale:   "Commands are easier to copy and paste without the $. A prompt helps only when the block also shows output.",
			Fix:         "The fix deletes each $ and the spaces after it.",
		},
		{
			ID: "MD031", Name: "blanks-around-fences", Tags: []string{"code", "blank_lines"}, Fixable: true,
			Description: "Fenced code blocks have a blank line above and below.",
			Rationale:   "Some parsers, such as kramdown, do not recognize a fenced code block without the blank lines.",
			Fix:         "The fix inserts the missing blank line, keeping any blockquote markers.",
		},
		{
			ID: "MD038", Name: "no-space-in-code", Tags: []string{"whitespace", "code"}, Fixable: true,
			Description: "Code spans have no space inside the backticks, or one space on each side.",
			Rationale:   "Extra spaces are usually accidental, and they can change how the code renders.",
			Fix:         "The fix deletes the extra spaces.",
		},
		{
			ID: "MD040", Name: "fenced-code-language", Tags: []string{"code", "language"}, Fixable: true,
			Description: "Fenced code blocks name their language after the opening fence.",
			Rationale:   "Renderers use the language to highlight the code.",
			Fix: "The fix adds a language only when gomdlint can detect it with certainty, such as from a shebang line or valid JSON. " +
				"gomdlint reports every other finding without a fix.",
		},
		{
			ID: "MD046", Name: "code-block-style", Tags: []string{"code"},
			Description: "Code blocks all use the first block's style, fenced or indented.",
			Rationale:   "One code block style makes the source easier to read.",
			Fix:         "There is no automatic fix.",
		},
		{
			ID: "MD048", Name: "code-fence-style", Tags: []string{"code"}, Fixable: true,
			Description: "Code fences all use the first fence's character, backtick or tilde.",
			Rationale:   "One fence character makes code blocks easy to recognize and to search for.",
			Fix: "The fix rewrites the opening and closing fences with the expected character. " +
				"gomdlint reports a block without a fix when the new fence would end it early or break its info string.",
		},
	}
}
