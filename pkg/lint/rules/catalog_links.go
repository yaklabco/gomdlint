package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// linksCatalog lists the rules that check links, images and reference
// definitions.
func linksCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD011", Name: "no-reversed-links", Tags: []string{"links"}, Fixable: true,
			Description: "Inline links put the text in brackets and the URL in parentheses: [text](url).",
			Rationale:   "The reversed form (text)[url] is not a link, so its brackets and parentheses show as text.",
			Fix:         "The fix swaps the two parts into the [text](url) form. " + noFixNearCheckbox,
		},
		{
			ID: "MD034", Name: "no-bare-urls", Tags: []string{"links", "url"}, Fixable: true,
			Description: "Outside code and HTML, URLs and email addresses sit in angle brackets or links.",
			Rationale:   "Some Markdown parsers do not turn a bare URL or email address into a link.",
			Fix: "The fix wraps the URL in angle brackets. gomdlint reports a URL without a fix " +
				"when parsers may disagree about where it ends, such as in a table, right after an emphasis marker or with an & in it. " +
				"It also reports a www. address without a fix, because angle brackets do not make it a link. " + noFixNearCheckbox,
		},
		{
			ID: "MD039", Name: "no-space-in-links", Tags: []string{"whitespace", "links"}, Fixable: true,
			Description: "Link text has no spaces just inside its brackets.",
			Rationale:   "The spaces end up inside the rendered link, where they serve no purpose.",
			Fix:         "The fix deletes the spaces. gomdlint reports some links without a fix, such as one whose text ends in a line break. " + noFixNearCheckbox,
		},
		{
			ID: "MD042", Name: "no-empty-links", Tags: []string{"links"},
			Description: "Inline links have a destination, and no link points at just #.",
			Rationale:   "An empty link looks clickable but leads nowhere.",
			Fix:         "There is no automatic fix. Only the author knows where the link should go.",
		},
		{
			ID: "MD045", Name: "no-alt-text", Tags: []string{"accessibility", "images"},
			Description: "Markdown images have alt text, and HTML images have alt or aria-hidden=\"true\".",
			Rationale: "Alt text describes an image for people who cannot see it, such as screen reader users. " +
				"In HTML, an empty alt or aria-hidden=\"true\" marks an image that screen readers skip.",
			Fix: "There is no automatic fix. Only the author can describe the image.",
		},
		{
			ID: "MD051", Name: "link-fragments", Tags: []string{"links"},
			Description: "Links to a #fragment point at a heading or anchor in the same file, or at #top.",
			Rationale: "A heading's anchor changes when the heading is renamed or removed, " +
				"and a link to the old anchor goes nowhere. In HTML, an element's id attribute, " +
				"or the name attribute of an a element, makes an anchor. Links to GitHub line numbers, such as #L20, pass too.",
			Fix: "There is no automatic fix. Only the author knows the intended target.",
		},
		{
			ID: "MD052", Name: "reference-links-images", Tags: []string{"images", "links"},
			Description: "Labels in [text][label] and [label][] references are defined, except x.",
			Rationale: "A reference with an undefined label renders as plain text in brackets. " +
				"Shortcut references such as [label] are not checked, because plain text in brackets looks the same. " +
				"The label x is ignored because task lists use [x].",
			Fix: "There is no automatic fix. Only the author knows the missing destination.",
		},
		{
			ID: "MD053", Name: "link-image-reference-definitions", Tags: []string{"images", "links"}, Fixable: true,
			Description: "Every reference definition is used and defined once, except [//] comments.",
			Rationale: "Definitions render nothing, so an unused one is dead text. " +
				"Only the first of duplicate definitions takes effect, so the others change nothing. " +
				"A definition labeled //, as in [//]: # (note), is a common way to write a comment, so it is ignored. " +
				"A GFM footnote definition such as [^1]: A note. counts too where gomdlint can tell that GitHub reads it as one: " +
				"at the start of a line outside lists and blockquotes, with a label of letters, digits and hyphens.",
			Fix: "The fix deletes the unused or duplicate definition. " +
				"gomdlint reports some definitions without a fix, such as one that other text follows on the next line, " +
				"or one whose label starts with ^, which may be a footnote whose text goes on in later lines. " + noFixNearCheckbox,
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
			Description: "Relative links and local file: URLs are not mixed in one document.",
			Rationale: "Relative and absolute links behave differently when a file moves or a repository is forked. " +
				"One kind keeps that behavior the same for every link that could take either form. " +
				"A link to another host, such as https://example.com, cannot be relative, so it never counts. " +
				"gomdlint cannot tell a URL of the document's own site from one of another host, " +
				"so the only absolute links that count are file: URLs that name no other host. " +
				"URIs such as mailto:, autolinks and links to a #fragment never count either.",
			Fix: "There is no automatic fix.",
		},
	}
}
