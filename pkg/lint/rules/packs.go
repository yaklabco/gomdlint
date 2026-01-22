package rules

import "github.com/yaklabco/gomdlint/pkg/config"

// Pack describes a named group of rule defaults for a particular use case.
// Packs are configuration fragments that can be used as starting points
// for .gomdlint.yml files.
type Pack struct {
	// Name is the short identifier for the pack (e.g., "core", "strict").
	Name string

	// Description explains the purpose and characteristics of the pack.
	Description string

	// Rules contains rule configurations keyed by rule ID.
	Rules map[string]config.RuleConfig
}

// CorePack returns the core pack with essential rules for clean Markdown.
// This pack includes whitespace cleanup and basic structural rules.
func CorePack() Pack {
	return Pack{
		Name:        "core",
		Description: "Essential rules for clean Markdown: whitespace, basic structure",
		Rules: map[string]config.RuleConfig{
			"MD009": enabled("warning"), // no-trailing-spaces
			"MD010": enabled("warning"), // no-hard-tabs
			"MD012": enabled("warning"), // no-multiple-blanks
			"MD047": enabled("warning"), // single-trailing-newline
			"MD001": enabled("warning"), // heading-increment
			"MD003": enabled("info"),    // heading-style
			"MD004": enabled("info"),    // ul-style
			"MD013": enabled("info"),    // line-length
			"MD031": enabled("info"),    // blanks-around-fences
			"MD032": enabled("info"),    // blanks-around-lists
		},
	}
}

// StrictPack returns the strict pack with all core rules elevated to errors,
// plus additional structural and HTML rules for maximum consistency.
func StrictPack() Pack {
	return Pack{
		Name:        "strict",
		Description: "Strict pack: all core rules as errors, plus HTML restrictions and structural rules",
		Rules: map[string]config.RuleConfig{
			// Whitespace (errors).
			"MD009": enabled("error"), // no-trailing-spaces
			"MD010": enabled("error"), // no-hard-tabs
			"MD012": enabled("error"), // no-multiple-blanks
			"MD022": enabled("error"), // blanks-around-headings
			"MD047": enabled("error"), // single-trailing-newline

			// Headings (errors).
			"MD001": enabled("error"), // heading-increment
			"MD003": enabled("error"), // heading-style
			"MD018": enabled("error"), // no-missing-space-atx
			"MD019": enabled("error"), // no-multiple-space-atx
			"MD023": enabled("error"), // heading-start-left
			"MD024": enabled("error"), // no-duplicate-heading
			"MD025": enabled("error"), // single-h1
			"MD026": enabled("error"), // no-trailing-punctuation
			"MD041": enabled("error"), // first-line-heading

			// Lists (errors).
			"MD004": enabled("error"), // ul-style
			"MD005": enabled("error"), // list-indent
			"MD007": enabled("error"), // ul-indent
			"MD029": enabled("error"), // ol-prefix
			"MD030": enabled("error"), // list-marker-space
			"MD032": enabled("error"), // blanks-around-lists

			// Line length (warning - too strict as error).
			"MD013": enabled("warning"), // line-length

			// Links (errors).
			"MD034": enabled("error"), // no-bare-urls
			"MD042": enabled("error"), // no-empty-links
			"MD045": enabled("error"), // no-alt-text

			// Code blocks (errors).
			"MD031": enabled("error"), // blanks-around-fences
			"MD038": enabled("error"), // no-space-in-code
			"MD040": enabled("error"), // fenced-code-language
			"MD048": enabled("error"), // code-fence-style

			// Emphasis (errors).
			"MD037": enabled("error"), // no-space-in-emphasis
			"MD049": enabled("error"), // emphasis-style
			"MD050": enabled("error"), // strong-style

			// HR (errors).
			"MD035": enabled("error"), // hr-style

			// HTML (error).
			"MD033": enabled("error"), // no-inline-html
		},
	}
}

// RelaxedPack returns a relaxed pack with minimal noise,
// suitable for loose style guides or legacy codebases.
func RelaxedPack() Pack {
	return Pack{
		Name:        "relaxed",
		Description: "Relaxed pack: only essential whitespace rules, minimal noise",
		Rules: map[string]config.RuleConfig{
			"MD009": enabled("info"), // no-trailing-spaces
			"MD047": enabled("info"), // single-trailing-newline
		},
	}
}

// GFMAuthoringPack returns rules tuned for GitHub-flavored Markdown authoring,
// including table validation, task lists, and stricter link checking.
func GFMAuthoringPack() Pack {
	return Pack{
		Name:        "gfm",
		Description: "GFM authoring pack: tables, task lists, links, optimized for GitHub",
		Rules: map[string]config.RuleConfig{
			// Core whitespace.
			"MD009": enabled("warning"), // no-trailing-spaces
			"MD047": enabled("warning"), // single-trailing-newline

			// Headings.
			"MD001": enabled("warning"), // heading-increment
			"MD022": enabled("info"),    // blanks-around-headings

			// GFM tables.
			"MDL002": enabled("warning"), // table-column-count
			"MDL003": enabled("warning"), // table-alignment
			"MDL004": enabled("info"),    // table-blank-lines

			// Links and images.
			"MD042": enabled("warning"), // no-empty-links
			"MD045": enabled("warning"), // no-alt-text
			"MD039": enabled("info"),    // no-space-in-links

			// Code blocks.
			"MD040": enabled("info"), // fenced-code-language
			"MD048": enabled("info"), // code-fence-style
		},
	}
}

// Packs returns all built-in rule packs.
func Packs() []Pack {
	return []Pack{
		CorePack(),
		StrictPack(),
		RelaxedPack(),
		GFMAuthoringPack(),
	}
}

// PackByName returns a pack by name, or nil if not found.
func PackByName(name string) *Pack {
	for _, p := range Packs() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// PackNames returns the names of all available packs.
func PackNames() []string {
	packs := Packs()
	names := make([]string, len(packs))
	for i, p := range packs {
		names[i] = p.Name
	}
	return names
}

// enabled creates a RuleConfig with the rule enabled and the given severity.
func enabled(sev string) config.RuleConfig {
	enabled := true
	return config.RuleConfig{
		Enabled:  &enabled,
		Severity: &sev,
	}
}
