package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// codeCatalog lists the rules that check code blocks and code spans.
func codeCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD014", Name: "commands-show-output", Tags: []string{"code"}, Fixable: true,
			Summary:     "Dollar signs used before commands without showing output",
			Description: "Code blocks that show only commands leave out the $ prompt.",
			Rationale:   "Commands are easier to copy and paste without the $. A prompt helps only when the block also shows output.",
			Fix:         "The fix deletes each $ and the spaces after it. " + noFixNearCheckbox,
		},
		{
			ID: "MD031", Name: "blanks-around-fences", Tags: []string{"code", "blank_lines"}, Fixable: true,
			RuleTags:    []string{"blank_lines", "code"},
			Summary:     "Fenced code blocks should be surrounded by blank lines",
			Description: "Fenced code blocks have a blank or comment line above and below.",
			Rationale:   "Some parsers, such as kramdown, do not recognize a fenced code block without the blank lines.",
			Fix: "The fix inserts the missing blank line, keeping any blockquote markers. " +
				"gomdlint reports some fences without a fix, such as a fence in a tight list when the blank line would make the list loose. " +
				noFixNearCheckbox,
		},
		{
			ID: "MD038", Name: "no-space-in-code", Tags: []string{"whitespace", "code"}, Fixable: true,
			RuleTags:    []string{"code", "whitespace"},
			Summary:     "Spaces inside code span elements",
			Description: "Code span text has no spaces around it, except one each side or by a backtick.",
			Rationale: "Extra spaces are usually accidental, and they can change how the code renders. " +
				"One space on each side is padding that renderers remove, " +
				"and a space beside a backtick in the code keeps it apart from the delimiter.",
			Fix: "The fix deletes the extra spaces. " + noFixNearCheckbox,
		},
		{
			ID: "MD040", Name: "fenced-code-language", Tags: []string{"code", "language"}, Fixable: true,
			RuleTags:    []string{"code"},
			Summary:     "Fenced code blocks should have a language specified",
			Description: "Fenced code blocks name their language after the opening fence.",
			Rationale:   "Renderers use the language to highlight the code.",
			Fix: "The fix adds a language only when gomdlint can detect it with certainty, such as from a shebang line or valid JSON, " +
				"and when allowed_languages, if set, names it exactly. gomdlint reports every other finding without a fix. " + noFixNearCheckbox,
		},
		{
			ID: "MD046", Name: "code-block-style", Tags: []string{"code"},
			RuleTags:    []string{"code", "style"},
			Summary:     "Code block style should be consistent",
			Description: "Code blocks all use the first block's style, fenced or indented.",
			Rationale:   "One code block style makes the source easier to read.",
			Fix:         "There is no automatic fix.",
		},
		{
			ID: "MD048", Name: "code-fence-style", Tags: []string{"code"}, Fixable: true,
			RuleTags:    []string{"code", "style"},
			Summary:     "Code fence style should be consistent",
			Description: "Code fences all use the first fence's character, backtick or tilde.",
			Rationale:   "One fence character makes code blocks easy to recognize and to search for.",
			Fix: "The fix rewrites the opening and closing fences with the expected character. " +
				"gomdlint reports a block without a fix when the new fence would end it early or break its info string. " + noFixNearCheckbox,
		},
	}
}
