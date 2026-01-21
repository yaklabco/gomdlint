// Package refs provides reference link/image tracking infrastructure for linting.
// It collects reference definitions, link/image usages, and document anchors
// to support rules like MD051-MD054 that require document-wide analysis.
package refs

import (
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// ReferenceStyle indicates the syntax style of a link or image reference.
type ReferenceStyle string

const (
	// StyleInline represents inline links: [text](url) or ![alt](url).
	StyleInline ReferenceStyle = "inline"

	// StyleFull represents full reference links: [text][label] or ![alt][label].
	StyleFull ReferenceStyle = "full"

	// StyleCollapsed represents collapsed reference links: [label][] or ![label][].
	StyleCollapsed ReferenceStyle = "collapsed"

	// StyleShortcut represents shortcut reference links: [label] or ![label].
	StyleShortcut ReferenceStyle = "shortcut"

	// StyleAutolink represents autolinks: <https://example.com>.
	StyleAutolink ReferenceStyle = "autolink"
)

// ReferenceDefinition represents a link/image reference definition
// (e.g., [label]: https://example.com "Optional Title").
type ReferenceDefinition struct {
	// Label is the reference label as written in the source.
	Label string

	// NormalizedLabel is the lowercase, whitespace-collapsed label for matching.
	NormalizedLabel string

	// Destination is the URL/path.
	Destination string

	// Title is the optional title.
	Title string

	// Position in source.
	Position mdast.SourcePosition

	// LineNumber for quick access (1-based).
	LineNumber int

	// IsDuplicate indicates this is a duplicate definition (not the first).
	IsDuplicate bool

	// UsageCount tracks how many times this definition is referenced.
	UsageCount int
}

// ReferenceUsage represents a link or image in the document.
type ReferenceUsage struct {
	// Style indicates how the reference is written.
	Style ReferenceStyle

	// IsImage is true for images, false for links.
	IsImage bool

	// Text is the link text or image alt text.
	Text string

	// Label is the reference label (for full/collapsed/shortcut styles).
	// Empty for inline/autolink styles.
	Label string

	// NormalizedLabel for matching against definitions.
	NormalizedLabel string

	// Destination is the resolved URL.
	Destination string

	// Fragment is the URL fragment (e.g., "#heading-name").
	// Extracted from destination for validation.
	Fragment string

	// Position in source.
	Position mdast.SourcePosition

	// Node is the original AST node.
	Node *mdast.Node

	// ResolvedDefinition points to the matching definition (if any).
	ResolvedDefinition *ReferenceDefinition
}

// Context holds all reference-related data for a document.
// It is built once and shared across all reference-tracking rules.
type Context struct {
	// Definitions maps normalized labels to their first definitions.
	Definitions map[string]*ReferenceDefinition

	// AllDefinitions includes all definitions, including duplicates.
	AllDefinitions []*ReferenceDefinition

	// Usages is all link/image usages in document order.
	Usages []*ReferenceUsage

	// Anchors is the map of valid fragment targets.
	Anchors *AnchorMap

	// File is the source file snapshot.
	File *mdast.FileSnapshot
}

// NewContext creates an empty Context.
func NewContext(file *mdast.FileSnapshot) *Context {
	return &Context{
		Definitions:    make(map[string]*ReferenceDefinition),
		AllDefinitions: nil,
		Usages:         nil,
		Anchors:        NewAnchorMap(),
		File:           file,
	}
}

// ResolveLabel finds the definition for a normalized label.
func (c *Context) ResolveLabel(label string) *ReferenceDefinition {
	normalized := NormalizeLabel(label)
	return c.Definitions[normalized]
}

// ValidateFragment checks if a fragment references a valid anchor.
func (c *Context) ValidateFragment(fragment string) bool {
	if fragment == "" {
		return true // No fragment is always valid
	}

	// Remove leading #
	id := strings.TrimPrefix(fragment, "#")

	// Empty after trimming # is valid (just "#")
	if id == "" {
		return true
	}

	// Special case: #top is always valid (HTML standard)
	if strings.EqualFold(id, "top") {
		return true
	}

	// GitHub line number syntax: #L20 or #L19C5-L21C11
	if isGitHubLineReference(id) {
		return true
	}

	// Check against anchor map
	return c.Anchors.Has(id)
}

// UnusedDefinitions returns definitions with zero usage count.
func (c *Context) UnusedDefinitions() []*ReferenceDefinition {
	var unused []*ReferenceDefinition
	for _, def := range c.AllDefinitions {
		if !def.IsDuplicate && def.UsageCount == 0 {
			unused = append(unused, def)
		}
	}
	return unused
}

// DuplicateDefinitions returns all duplicate definitions.
func (c *Context) DuplicateDefinitions() []*ReferenceDefinition {
	var dups []*ReferenceDefinition
	for _, def := range c.AllDefinitions {
		if def.IsDuplicate {
			dups = append(dups, def)
		}
	}
	return dups
}

// UnresolvedUsages returns usages that reference undefined labels.
func (c *Context) UnresolvedUsages() []*ReferenceUsage {
	var unresolved []*ReferenceUsage
	for _, usage := range c.Usages {
		if usage.Label != "" && usage.ResolvedDefinition == nil {
			unresolved = append(unresolved, usage)
		}
	}
	return unresolved
}

// NormalizeLabel normalizes a reference label for matching.
// Per CommonMark: case-insensitive, collapse whitespace.
func NormalizeLabel(label string) string {
	// Lowercase
	label = strings.ToLower(label)
	// Collapse whitespace
	label = strings.Join(strings.Fields(label), " ")
	return label
}

// isGitHubLineReference checks for GitHub's line/column reference syntax.
func isGitHubLineReference(id string) bool {
	// Quick check for common prefix
	if len(id) < 2 || (id[0] != 'L' && id[0] != 'l') {
		return false
	}

	// Matches patterns like: L20, L19C5, L19C5-L21C11, L19-L21
	// Simplified check: starts with L followed by digit
	if len(id) >= 2 && (id[0] == 'L' || id[0] == 'l') {
		for i := 1; i < len(id); i++ {
			ch := id[i]
			if ch >= '0' && ch <= '9' {
				return true // Has at least one digit after L
			}
			if ch != 'C' && ch != 'c' && ch != '-' {
				return false
			}
		}
	}
	return false
}

// ExtractFragment extracts the fragment from a URL.
// Returns empty string if no fragment.
func ExtractFragment(url string) string {
	idx := strings.Index(url, "#")
	if idx == -1 {
		return ""
	}
	return url[idx:]
}
