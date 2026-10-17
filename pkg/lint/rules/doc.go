// Package rules provides the built-in lint rules for gomdlint.
//
// # Rule Domains
//
// The rules are grouped by the Markdown construct they check, as their
// catalog entries record (see Catalog).
//
// Headings:
//
//   - MD001: heading-increment
//   - MD003: heading-style
//   - MD018: no-missing-space-atx
//   - MD019: no-multiple-space-atx
//   - MD020: no-missing-space-closed-atx
//   - MD021: no-multiple-space-closed-atx
//   - MD022: heading-blank-lines
//   - MD023: heading-start-left
//   - MD024: no-duplicate-heading
//   - MD025: single-h1
//   - MD026: no-trailing-punctuation
//   - MD036: no-emphasis-as-heading
//   - MD041: first-line-heading
//   - MD043: required-headings
//
// Lists:
//
//   - MD004: unordered-list-style
//   - MD005: list-indent
//   - MD007: ul-indent
//   - MD029: ol-prefix
//   - MD030: list-marker-space
//   - MD032: blanks-around-lists
//
// Whitespace and layout:
//
//   - MD009: no-trailing-spaces
//   - MD010: no-hard-tabs
//   - MD012: no-multiple-blank-lines
//   - MD013: line-length
//   - MD035: hr-style
//   - MD047: single-trailing-newline
//
// Code:
//
//   - MD014: commands-show-output
//   - MD031: blanks-around-fences
//   - MD038: no-space-in-code
//   - MD040: fenced-code-language
//   - MD046: code-block-style
//   - MD048: code-fence-style
//
// Links and images:
//
//   - MD011: no-reversed-links
//   - MD034: no-bare-urls
//   - MD039: no-space-in-links
//   - MD042: no-empty-links
//   - MD045: no-alt-text
//   - MD051: link-fragments
//   - MD052: reference-links-images
//   - MD053: link-image-reference-definitions
//   - MD054: link-image-style
//   - MD059: descriptive-link-text
//   - MDL001: link-destination-style
//
// Tables (GFM):
//
//   - MD055: table-pipe-style
//   - MD056: table-column-count
//   - MD058: blanks-around-tables
//   - MD060: table-column-style
//   - MDL003: table-alignment
//
// Emphasis:
//
//   - MD037: no-space-in-emphasis
//   - MD049: emphasis-style
//   - MD050: strong-style
//
// Blockquotes:
//
//   - MD027: no-multiple-space-blockquote
//   - MD028: no-blanks-blockquote
//
// HTML:
//
//   - MD033: no-inline-html
//
// Spelling:
//
//   - MD044: proper-names
//
// # Rule IDs
//
// Rule IDs follow the markdownlint MDxxx convention for compatibility.
// Rules unique to gomdlint use the MDLxxx namespace:
//
//   - MD001-MD060: markdownlint-compatible rules
//   - MDL001-MDLxxx: gomdlint-specific rules
//
// # Rule Packs
//
// Rule packs are configuration presets for common use cases:
//
//   - core: Essential rules for clean Markdown (whitespace, basic structure)
//   - strict: All core rules as errors plus comprehensive checks
//   - relaxed: Minimal noise, only essential whitespace rules
//   - gfm: GFM authoring with tables, links, and code blocks
//
// Use PackByName or Packs to access pack definitions programmatically.
//
// # Registration
//
// Rules are registered with the default registry via RegisterAll, together
// with their catalog entries. Catalog describes every rule: its ID, name,
// markdownlint aliases, category and markdownlint tags, and whether it can
// fix what it finds. The registry resolves rule names and aliases, and
// inline directives resolve tags, through these entries. Each
// catalog_<category>.go file lists the rules of one category.
//
// Each rule follows the lint.Rule interface and uses the RuleContext,
// DiagnosticBuilder, and EditBuilder infrastructure.
package rules
