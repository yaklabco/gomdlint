package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// emphasisCatalog lists the rules that check emphasis and strong emphasis.
func emphasisCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD037", Name: "no-space-in-emphasis", Tags: []string{"whitespace", "emphasis"}, Fixable: true,
			Description: "Emphasis markers touch the text they wrap, as in **bold**.",
			Rationale:   "With a space inside, the markers do not make emphasis and show as literal * or _ characters.",
			Fix: "The fix deletes the spaces inside the markers. gomdlint reports a pair without a fix " +
				"when the markers may be meant literally, such as single asterisks with spaces on both sides.",
		},
		{
			ID: "MD049", Name: "emphasis-style", Tags: []string{"emphasis"}, Fixable: true,
			Description: "All italic text uses one marker, *text* or _text_, except * inside a word.",
			Rationale: "One italic marker throughout makes the source easier to read. " +
				"Underscores do not mark emphasis inside a word, so * stays allowed there.",
			Fix: "The fix swaps the markers. gomdlint reports italic text without a fix " +
				"when the new marker could pair with characters around it, as in *snake_case*. " +
				"It does the same inside square brackets that are, or could become, the label of a reference link.",
		},
		{
			ID: "MD050", Name: "strong-style", Tags: []string{"emphasis"}, Fixable: true,
			Description: "All bold text uses one marker, **text** or __text__, except ** inside a word.",
			Rationale: "One bold marker throughout makes the source easier to read. " +
				"Underscores do not mark emphasis inside a word, so ** stays allowed there.",
			Fix: "The fix swaps the markers. gomdlint reports bold text without a fix " +
				"when the new marker could pair with characters around it, as in **snake_case**. " +
				"It does the same inside square brackets that are, or could become, the label of a reference link.",
		},
	}
}
