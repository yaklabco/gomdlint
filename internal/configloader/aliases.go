// Package configloader provides configuration loading and resolution.
package configloader

import "strings"

// ruleAliases maps markdownlint rule aliases to their canonical rule IDs.
// This enables compatibility with markdownlint configuration files that use
// either the rule ID (MD001) or the alias (heading-increment).
//
//nolint:gochecknoglobals // Read-only lookup table.
var ruleAliases = map[string]string{
	// Headings
	"heading-increment":            "MD001",
	"heading-style":                "MD003",
	"blanks-around-headings":       "MD022",
	"heading-start-left":           "MD023",
	"no-duplicate-heading":         "MD024",
	"single-title":                 "MD025",
	"single-h1":                    "MD025",
	"no-trailing-punctuation":      "MD026",
	"first-line-heading":           "MD041",
	"first-line-h1":                "MD041",
	"required-headings":            "MD043",
	"no-missing-space-atx":         "MD018",
	"no-multiple-space-atx":        "MD019",
	"no-missing-space-closed-atx":  "MD020",
	"no-multiple-space-closed-atx": "MD021",

	// Lists
	"ul-style":            "MD004",
	"list-indent":         "MD005",
	"ul-indent":           "MD007",
	"ol-prefix":           "MD029",
	"list-marker-space":   "MD030",
	"blanks-around-lists": "MD032",

	// Whitespace
	"no-trailing-spaces":      "MD009",
	"no-hard-tabs":            "MD010",
	"no-multiple-blanks":      "MD012",
	"line-length":             "MD013",
	"single-trailing-newline": "MD047",

	// Code
	"commands-show-output": "MD014",
	"blanks-around-fences": "MD031",
	"no-space-in-code":     "MD038",
	"fenced-code-language": "MD040",
	"code-block-style":     "MD046",
	"code-fence-style":     "MD048",

	// Links
	"no-reversed-links":                "MD011",
	"no-bare-urls":                     "MD034",
	"no-space-in-links":                "MD039",
	"no-empty-links":                   "MD042",
	"link-fragments":                   "MD051",
	"reference-links-images":           "MD052",
	"link-image-reference-definitions": "MD053",
	"link-image-style":                 "MD054",
	"descriptive-link-text":            "MD059",

	// Blockquote
	"no-multiple-space-blockquote": "MD027",
	"no-blanks-blockquote":         "MD028",

	// HTML
	"no-inline-html": "MD033",

	// HR
	"hr-style": "MD035",

	// Emphasis
	"no-emphasis-as-heading": "MD036",
	"no-space-in-emphasis":   "MD037",
	"emphasis-style":         "MD049",
	"strong-style":           "MD050",

	// Images
	"no-alt-text": "MD045",

	// Names/Spelling
	"proper-names": "MD044",

	// Tables
	"table-pipe-style":     "MD055",
	"table-column-count":   "MD056",
	"blanks-around-tables": "MD058",
	"table-column-style":   "MD060",
}

// ruleTags maps markdownlint tag names to the rule IDs they contain.
// Tags can be used in configuration to enable/disable groups of rules at once.
//
//nolint:gochecknoglobals // Read-only lookup table.
var ruleTags = map[string][]string{
	"accessibility": {"MD045", "MD059"},
	"atx":           {"MD018", "MD019"},
	"atx_closed":    {"MD020", "MD021"},
	"blank_lines":   {"MD012", "MD022", "MD031", "MD032", "MD047"},
	"blockquote":    {"MD027", "MD028"},
	"bullet":        {"MD004", "MD005", "MD007", "MD032"},
	"code":          {"MD014", "MD031", "MD038", "MD040", "MD046", "MD048"},
	"emphasis":      {"MD036", "MD037", "MD049", "MD050"},
	"hard_tab":      {"MD010"},
	"headings":      {"MD001", "MD003", "MD018", "MD019", "MD020", "MD021", "MD022", "MD023", "MD024", "MD025", "MD026", "MD036", "MD041", "MD043"},
	"hr":            {"MD035"},
	"html":          {"MD033"},
	"images":        {"MD045", "MD052", "MD053", "MD054"},
	"indentation":   {"MD005", "MD007", "MD027"},
	"language":      {"MD040"},
	"line_length":   {"MD013"},
	"links":         {"MD011", "MD034", "MD039", "MD042", "MD051", "MD052", "MD053", "MD054", "MD059"},
	"ol":            {"MD029", "MD030", "MD032"},
	"spaces":        {"MD018", "MD019", "MD020", "MD021", "MD023"},
	"spelling":      {"MD044"},
	"table":         {"MD055", "MD056", "MD058", "MD060"},
	"ul":            {"MD004", "MD005", "MD007", "MD030", "MD032"},
	"url":           {"MD034"},
	"whitespace":    {"MD009", "MD010", "MD012", "MD027", "MD028", "MD030", "MD037", "MD038", "MD039"},
}

// NormalizeRuleID converts a rule alias or ID to its canonical rule ID.
// Returns empty string if the key is not a recognized rule ID or alias.
func NormalizeRuleID(key string) string {
	// Check if already a rule ID (starts with MD)
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "MD") {
		return upper
	}

	// Check aliases
	if id, ok := ruleAliases[key]; ok {
		return id
	}

	return ""
}

// IsTag returns true if the key is a recognized tag name.
func IsTag(key string) bool {
	_, ok := ruleTags[key]
	return ok
}

// GetTagRules returns the rule IDs associated with a tag.
// Returns nil if the tag is not recognized.
func GetTagRules(tag string) []string {
	return ruleTags[tag]
}

// GetAllRuleIDs returns a slice of all known rule IDs.
func GetAllRuleIDs() []string {
	// Build a set of all rule IDs from aliases
	seen := make(map[string]struct{})
	for _, id := range ruleAliases {
		seen[id] = struct{}{}
	}

	// Convert to slice
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}

	return ids
}

// GetAliasesForRule returns all aliases for a given rule ID.
func GetAliasesForRule(ruleID string) []string {
	var aliases []string
	for alias, id := range ruleAliases {
		if id == ruleID {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}
