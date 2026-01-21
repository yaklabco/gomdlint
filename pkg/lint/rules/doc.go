// Package rules provides the built-in lint rules for gomdlint.
//
// # Rule Domains
//
// This package contains rule implementations across several domains:
//
//   - Whitespace and layout:
//
//   - MD009: no-trailing-spaces - Lines should not have trailing spaces
//
//   - MD010: no-hard-tabs - Hard tabs should not be used
//
//   - MD012: no-multiple-blanks - Multiple consecutive blank lines
//
//   - MD047: single-trailing-newline - Files should end with a single newline
//
//   - Headings:
//
//   - MD001: heading-increment - Heading levels should only increment by one
//
//   - MD003: heading-style - Heading style should be consistent
//
//   - MD018: no-missing-space-atx - No space after hash on ATX headings
//
//   - MD019: no-multiple-space-atx - Multiple spaces after hash on ATX headings
//
//   - MD020: no-missing-space-closed-atx - No space inside closed ATX headings
//
//   - MD021: no-multiple-space-closed-atx - Multiple spaces inside closed ATX headings
//
//   - MD022: blanks-around-headings - Headings should be surrounded by blank lines
//
//   - MD023: heading-start-left - Headings must start at beginning of line
//
//   - MD024: no-duplicate-heading - Multiple headings with same content
//
//   - MD025: single-h1 - Multiple top-level headings in the same document
//
//   - MD026: no-trailing-punctuation - Trailing punctuation in heading
//
//   - MD041: first-line-heading - First line should be a top-level heading
//
//   - Lists:
//
//   - MD004: ul-style - Unordered list style should be consistent
//
//   - MD005: list-indent - Inconsistent indentation for list items
//
//   - MD007: ul-indent - Unordered list indentation
//
//   - MD029: ol-prefix - Ordered list item prefix
//
//   - MD030: list-marker-space - Spaces after list markers
//
//   - MD032: blanks-around-lists - Lists should be surrounded by blank lines
//
//   - Blockquotes:
//
//   - MD027: no-multiple-space-blockquote - Multiple spaces after blockquote symbol
//
//   - MD028: no-blanks-blockquote - Blank line inside blockquote
//
//   - Line length:
//
//   - MD013: line-length - Line length should not exceed configured maximum
//
//   - Links and images:
//
//   - MD011: no-reversed-links - Reversed link syntax
//
//   - MD034: no-bare-urls - Bare URL used
//
//   - MD039: no-space-in-links - Spaces inside link text
//
//   - MD042: no-empty-links - Empty links
//
//   - MD045: no-alt-text - Images should have alternative text
//
//   - MDL001: link-destination-style - Link destination style
//
//   - Code blocks:
//
//   - MD031: blanks-around-fences - Fenced code blocks should have blank lines around them
//
//   - MD038: no-space-in-code - Spaces inside code span elements
//
//   - MD040: fenced-code-language - Fenced code blocks should have language info
//
//   - MD046: code-block-style - Code block style should be consistent
//
//   - MD048: code-fence-style - Code fence style should be consistent
//
//   - Emphasis:
//
//   - MD036: no-emphasis-as-heading - Emphasis used instead of heading
//
//   - MD037: no-space-in-emphasis - Spaces inside emphasis markers
//
//   - MD049: emphasis-style - Emphasis style should be consistent
//
//   - MD050: strong-style - Strong style should be consistent
//
//   - Horizontal rules:
//
//   - MD035: hr-style - Horizontal rule style should be consistent
//
//   - HTML:
//
//   - MD033: no-inline-html - Inline HTML should be avoided
//
//   - Tables (GFM):
//
//   - MDL002: table-column-count - Table column count should be consistent
//
//   - MDL003: table-alignment - Table alignment should be consistent
//
//   - MDL004: table-blank-lines - Tables should be surrounded by blank lines
//
// # Rule IDs
//
// Rule IDs follow the markdownlint MDxxx convention for compatibility.
// Rules unique to mdlint use the MDLxxx namespace:
//
//   - MD001-MD060: markdownlint-compatible rules
//   - MDL001-MDLxxx: mdlint-specific rules
//
// # Rule Packs
//
// Rule packs are configuration presets for common use cases:
//
//   - core: Essential rules for clean Markdown (whitespace, basic structure)
//   - strict: All core rules as errors plus comprehensive checks
//   - relaxed: Minimal noise, only essential whitespace rules
//   - gfm: GFM authoring with tables, task lists, and links
//
// Use PackByName or Packs to access pack definitions programmatically.
//
// # Registration
//
// Rules are registered with the default registry via RegisterAll.
// Each rule follows the lint.Rule interface and uses the RuleContext,
// DiagnosticBuilder, and EditBuilder infrastructure.
package rules
