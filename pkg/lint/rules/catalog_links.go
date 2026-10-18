package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// linksCatalog lists the rules that check links, images and reference
// definitions.
func linksCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD011", Name: "no-reversed-links", Tags: []string{"links"}, Fixable: true,
			Description: "Links put the text in brackets and the URL in parentheses, as in [text](url).",
			Rationale:   "The reversed form (text)[url] is not a link, so its brackets and parentheses show as text.",
			Fix:         "The fix swaps the two parts into the [text](url) form.",
		},
		{
			ID: "MD034", Name: "no-bare-urls", Tags: []string{"links", "url"}, Fixable: true,
			Description: "URLs and email addresses sit inside angle brackets or a link.",
			Rationale:   "Some Markdown parsers do not turn a bare URL or email address into a link.",
			Fix: "The fix wraps the URL in angle brackets. gomdlint reports a URL without a fix " +
				"when parsers may disagree about where it ends, such as in a table, inside emphasis or with an & in it. " +
				"It also reports a www. address without a fix, because angle brackets do not make it a link.",
		},
		{
			ID: "MD039", Name: "no-space-in-links", Tags: []string{"whitespace", "links"}, Fixable: true,
			Description: "Link text has no spaces just inside its brackets.",
			Rationale:   "The spaces end up inside the rendered link, where they serve no purpose.",
			Fix:         "The fix deletes the spaces.",
		},
		{
			ID: "MD042", Name: "no-empty-links", Tags: []string{"links"},
			Description: "Every link has a destination that is not empty or just #.",
			Rationale:   "An empty link looks clickable but leads nowhere.",
			Fix:         "There is no automatic fix. Only the author knows where the link should go.",
		},
		{
			ID: "MD045", Name: "no-alt-text", Tags: []string{"accessibility", "images"},
			Description: "Every image, in Markdown or HTML, has alt text.",
			Rationale:   "Alt text describes an image for people who cannot see it, such as screen reader users.",
			Fix:         "There is no automatic fix. Only the author can describe the image.",
		},
		{
			ID: "MD051", Name: "link-fragments", Tags: []string{"links"},
			Description: "Links to a #fragment point at a heading or anchor in the same file.",
			Rationale: "A heading's anchor changes when the heading is renamed or removed, " +
				"and a link to the old anchor goes nowhere.",
			Fix: "There is no automatic fix. Only the author knows the intended target.",
		},
		{
			ID: "MD052", Name: "reference-links-images", Tags: []string{"images", "links"},
			Description: "Reference links and images use labels that are defined.",
			Rationale:   "A reference with an undefined label renders as plain text in brackets.",
			Fix:         "There is no automatic fix. Only the author knows the missing destination.",
		},
		{
			ID: "MD053", Name: "link-image-reference-definitions", Tags: []string{"images", "links"}, Fixable: true,
			Description: "Every reference definition is used, and each label is defined once.",
			Rationale: "Definitions render nothing, so an unused one is dead text. " +
				"Only the first of duplicate definitions takes effect, so the others change nothing.",
			Fix: "The fix deletes the unused or duplicate definition.",
		},
		{
			ID: "MD054", Name: "link-image-style", Tags: []string{"images", "links"},
			Description: "Links and images use only the allowed styles. By default, all are allowed.",
			Rationale: "A project may prefer one way of writing links. Autolinks show raw URLs, " +
				"inline links take room in the source, and reference links need a definition elsewhere.",
			Fix: "There is no automatic fix.",
		},
		{
			ID: "MD059", Name: "descriptive-link-text", Tags: []string{"accessibility", "links"},
			Description: "Link text describes its target instead of saying \"click here\" or \"more\".",
			Rationale:   "Screen readers sometimes read a link without the text around it, so the link text alone has to make sense.",
			Fix:         "There is no automatic fix. Only the author knows what the link should say.",
		},
		{
			ID: "MDL001", Name: "link-destination-style", Tags: []string{"links", "url"},
			Description: "Links in a file all use relative paths or all use absolute URLs.",
			Rationale: "Relative and absolute links behave differently when a file moves or a repository is forked. " +
				"One kind keeps that behavior the same for every link.",
			Fix: "There is no automatic fix.",
		},
	}
}
