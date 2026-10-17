package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// linksCatalog lists the rules that check links, images and reference
// definitions.
func linksCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD011", Name: "no-reversed-links", Tags: []string{"links"}, Fixable: true},
		{ID: "MD034", Name: "no-bare-urls", Tags: []string{"links", "url"}, Fixable: true},
		{ID: "MD039", Name: "no-space-in-links", Tags: []string{"whitespace", "links"}, Fixable: true},
		{ID: "MD042", Name: "no-empty-links", Tags: []string{"links"}},
		{ID: "MD045", Name: "no-alt-text", Tags: []string{"accessibility", "images"}},
		{ID: "MD051", Name: "link-fragments", Tags: []string{"links"}},
		{ID: "MD052", Name: "reference-links-images", Tags: []string{"images", "links"}},
		{ID: "MD053", Name: "link-image-reference-definitions", Tags: []string{"images", "links"}, Fixable: true},
		{ID: "MD054", Name: "link-image-style", Tags: []string{"images", "links"}},
		{ID: "MD059", Name: "descriptive-link-text", Tags: []string{"accessibility", "links"}},
		{ID: "MDL001", Name: "link-destination-style", Tags: []string{"links", "url"}},
	}
}
