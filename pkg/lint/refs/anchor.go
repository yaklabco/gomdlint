package refs

import (
	"strings"
	"unicode"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// AnchorSource indicates the origin of an anchor.
type AnchorSource int

const (
	// AnchorFromHeading is generated from a Markdown heading.
	AnchorFromHeading AnchorSource = iota

	// AnchorFromHTMLID is from an HTML element's id attribute.
	AnchorFromHTMLID

	// AnchorFromHTMLName is from an HTML anchor's name attribute.
	AnchorFromHTMLName

	// AnchorFromCustomID is from {#custom-id} syntax (not yet supported).
	AnchorFromCustomID
)

// Anchor represents a valid link target within the document.
type Anchor struct {
	// ID is the anchor identifier (e.g., "heading-name").
	ID string

	// Source indicates how the anchor was generated.
	Source AnchorSource

	// Position of the anchor source (heading, HTML element, etc.).
	Position mdast.SourcePosition

	// Text is the original text (for headings).
	Text string
}

// AnchorMap provides efficient anchor lookup.
type AnchorMap struct {
	// anchors maps anchor IDs to their definitions.
	// Multiple anchors can have the same ID (duplicates from repeated headings).
	anchors map[string][]*Anchor

	// anchorLower maps lowercase IDs for case-insensitive lookup.
	anchorLower map[string]string

	// seenCounts tracks how many times each base anchor has been seen,
	// used for generating duplicate suffixes.
	seenCounts map[string]int
}

// NewAnchorMap creates an empty AnchorMap.
func NewAnchorMap() *AnchorMap {
	return &AnchorMap{
		anchors:     make(map[string][]*Anchor),
		anchorLower: make(map[string]string),
		seenCounts:  make(map[string]int),
	}
}

// Add adds an anchor to the map.
func (m *AnchorMap) Add(anchor *Anchor) {
	id := anchor.ID
	m.anchors[id] = append(m.anchors[id], anchor)
	m.anchorLower[strings.ToLower(id)] = id
}

// AddFromHeading generates and adds an anchor from heading text.
// Returns the generated anchor ID.
func (m *AnchorMap) AddFromHeading(text string, pos mdast.SourcePosition) string {
	id := m.GenerateAnchor(text)

	anchor := &Anchor{
		ID:       id,
		Source:   AnchorFromHeading,
		Position: pos,
		Text:     text,
	}
	m.Add(anchor)

	return id
}

// GenerateAnchor converts heading text to a GitHub-compatible anchor.
// This method handles duplicate detection and suffix generation.
func (m *AnchorMap) GenerateAnchor(text string) string {
	base := generateAnchorBase(text)

	// Handle duplicates with -1, -2 suffix
	count := m.seenCounts[base]
	m.seenCounts[base] = count + 1

	if count == 0 {
		return base
	}
	return base + "-" + itoa(count)
}

// generateAnchorBase converts heading text to a base anchor ID.
// Algorithm (GitHub-compatible):
//  1. Convert to lowercase
//  2. Remove punctuation (except hyphens and underscores)
//  3. Replace spaces with hyphens
//  4. Collapse multiple hyphens
//  5. Trim leading/trailing hyphens
func generateAnchorBase(text string) string {
	var buf strings.Builder
	buf.Grow(len(text))

	prevHyphen := false

	for _, ch := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(ch) || unicode.IsNumber(ch):
			buf.WriteRune(ch)
			prevHyphen = false
		case ch == '-' || ch == '_':
			buf.WriteRune(ch)
			prevHyphen = (ch == '-')
		case ch == ' ':
			// Replace space with hyphen, but avoid consecutive hyphens
			if !prevHyphen && buf.Len() > 0 {
				_ = buf.WriteByte('-') // strings.Builder.WriteByte never fails
				prevHyphen = true
			}
		}
		// Other punctuation is silently dropped
	}

	result := buf.String()

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	// Collapse multiple consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	return result
}

// itoa is a simple int-to-string without importing strconv.
func itoa(num int) string {
	if num == 0 {
		return "0"
	}
	var buf [20]byte
	idx := len(buf)
	for num > 0 {
		idx--
		buf[idx] = byte('0' + num%10)
		num /= 10
	}
	return string(buf[idx:])
}

// Has returns true if the anchor ID exists.
func (m *AnchorMap) Has(id string) bool {
	_, ok := m.anchors[id]
	return ok
}

// HasIgnoreCase returns true if the anchor ID exists (case-insensitive).
func (m *AnchorMap) HasIgnoreCase(id string) bool {
	_, ok := m.anchorLower[strings.ToLower(id)]
	return ok
}

// Lookup returns the first anchor with the given ID, or nil.
func (m *AnchorMap) Lookup(id string) *Anchor {
	anchors := m.anchors[id]
	if len(anchors) == 0 {
		return nil
	}
	return anchors[0]
}

// LookupIgnoreCase returns the first anchor matching case-insensitively.
func (m *AnchorMap) LookupIgnoreCase(id string) *Anchor {
	canonicalID, ok := m.anchorLower[strings.ToLower(id)]
	if !ok {
		return nil
	}
	return m.Lookup(canonicalID)
}

// LookupAll returns all anchors with the given ID.
func (m *AnchorMap) LookupAll(id string) []*Anchor {
	return m.anchors[id]
}

// All returns all anchors in the map.
func (m *AnchorMap) All() []*Anchor {
	// Calculate total capacity needed
	total := 0
	for _, anchors := range m.anchors {
		total += len(anchors)
	}
	all := make([]*Anchor, 0, total)
	for _, anchors := range m.anchors {
		all = append(all, anchors...)
	}
	return all
}

// Count returns the total number of unique anchor IDs.
func (m *AnchorMap) Count() int {
	return len(m.anchors)
}
