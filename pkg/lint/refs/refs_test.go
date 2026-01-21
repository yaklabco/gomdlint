package refs

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "foo", "foo"},
		{"uppercase", "FOO", "foo"},
		{"mixed case", "FoO BaR", "foo bar"},
		{"extra spaces", "foo  bar", "foo bar"},
		{"leading spaces", "  foo", "foo"},
		{"trailing spaces", "foo  ", "foo"},
		{"tabs", "foo\tbar", "foo bar"},
		{"newlines", "foo\nbar", "foo bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeLabel(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeLabel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractFragment(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"no fragment", "https://example.com", ""},
		{"with fragment", "https://example.com#section", "#section"},
		{"only fragment", "#section", "#section"},
		{"empty fragment", "https://example.com#", "#"},
		{"relative with fragment", "page.md#heading", "#heading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFragment(tt.url)
			if got != tt.expected {
				t.Errorf("ExtractFragment(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestIsGitHubLineReference(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"L20", true},
		{"L1", true},
		{"L19C5", true},
		{"L19C5-L21C11", true},
		{"L19-L21", true},
		{"l20", true},      // lowercase
		{"heading", false}, // not a line reference
		{"L", false},       // no number
		{"", false},        // empty
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isGitHubLineReference(tt.id)
			if got != tt.expected {
				t.Errorf("isGitHubLineReference(%q) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestContext_ValidateFragment(t *testing.T) {
	ctx := NewContext(nil)

	// Add some anchors
	ctx.Anchors.Add(&Anchor{ID: "heading-one", Source: AnchorFromHeading})
	ctx.Anchors.Add(&Anchor{ID: "custom-id", Source: AnchorFromHTMLID})

	tests := []struct {
		name     string
		fragment string
		expected bool
	}{
		{"empty fragment", "", true},
		{"just hash", "#", true},
		{"special top", "#top", true},
		{"special TOP", "#TOP", true},
		{"github line ref", "#L20", true},
		{"valid anchor", "#heading-one", true},
		{"valid html anchor", "#custom-id", true},
		{"invalid anchor", "#nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.ValidateFragment(tt.fragment)
			if got != tt.expected {
				t.Errorf("ValidateFragment(%q) = %v, want %v", tt.fragment, got, tt.expected)
			}
		})
	}
}

func TestContext_ResolveLabel(t *testing.T) {
	ctx := NewContext(nil)

	// Add a definition
	def := &ReferenceDefinition{
		Label:           "Example",
		NormalizedLabel: "example",
		Destination:     "https://example.com",
	}
	ctx.Definitions["example"] = def

	tests := []struct {
		name     string
		label    string
		expected bool
	}{
		{"exact match", "example", true},
		{"different case", "Example", true},
		{"uppercase", "EXAMPLE", true},
		{"not found", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ctx.ResolveLabel(tt.label)
			if (got != nil) != tt.expected {
				t.Errorf("ResolveLabel(%q) found = %v, want %v", tt.label, got != nil, tt.expected)
			}
		})
	}
}

func TestContext_UnusedDefinitions(t *testing.T) {
	ctx := NewContext(nil)

	// Add definitions
	used := &ReferenceDefinition{Label: "used", NormalizedLabel: "used", UsageCount: 1}
	unused := &ReferenceDefinition{Label: "unused", NormalizedLabel: "unused", UsageCount: 0}
	duplicate := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", UsageCount: 0, IsDuplicate: true}

	ctx.AllDefinitions = []*ReferenceDefinition{used, unused, duplicate}

	got := ctx.UnusedDefinitions()
	if len(got) != 1 {
		t.Errorf("UnusedDefinitions() returned %d items, want 1", len(got))
	}
	if len(got) > 0 && got[0] != unused {
		t.Errorf("UnusedDefinitions() returned wrong definition")
	}
}

func TestContext_DuplicateDefinitions(t *testing.T) {
	ctx := NewContext(nil)

	// Add definitions
	first := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", IsDuplicate: false}
	duplicate := &ReferenceDefinition{Label: "dup", NormalizedLabel: "dup", IsDuplicate: true}
	unique := &ReferenceDefinition{Label: "unique", NormalizedLabel: "unique", IsDuplicate: false}

	ctx.AllDefinitions = []*ReferenceDefinition{first, duplicate, unique}

	got := ctx.DuplicateDefinitions()
	if len(got) != 1 {
		t.Errorf("DuplicateDefinitions() returned %d items, want 1", len(got))
	}
	if len(got) > 0 && got[0] != duplicate {
		t.Errorf("DuplicateDefinitions() returned wrong definition")
	}
}

func TestAnchorMap_GenerateAnchor(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{"simple", "Hello World", "hello-world"},
		{"uppercase", "API Reference", "api-reference"},
		{"numbers", "Version 1.0.0", "version-100"},
		{"punctuation", "Don't Panic!", "dont-panic"},
		{"c++", "C++ Guide", "c-guide"},
		{"underscores", "foo_bar_baz", "foo_bar_baz"},
		{"multiple spaces", "hello   world", "hello-world"},
		{"leading trailing spaces", "  hello  ", "hello"},
		{"emoji", "Hello World 🎉", "hello-world"},
		{"only special", "!!!???", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewAnchorMap() // Fresh map for each test
			got := m.GenerateAnchor(tt.text)
			if got != tt.expected {
				t.Errorf("GenerateAnchor(%q) = %q, want %q", tt.text, got, tt.expected)
			}
		})
	}
}

func TestAnchorMap_DuplicateHandling(t *testing.T) {
	m := NewAnchorMap()

	// Generate same heading multiple times
	first := m.GenerateAnchor("Hello World")
	second := m.GenerateAnchor("Hello World")
	third := m.GenerateAnchor("Hello World")

	if first != "hello-world" {
		t.Errorf("first anchor = %q, want %q", first, "hello-world")
	}
	if second != "hello-world-1" {
		t.Errorf("second anchor = %q, want %q", second, "hello-world-1")
	}
	if third != "hello-world-2" {
		t.Errorf("third anchor = %q, want %q", third, "hello-world-2")
	}
}

func TestAnchorMap_Lookup(t *testing.T) {
	anchorMap := NewAnchorMap()

	pos := mdast.SourcePosition{StartLine: 1, EndLine: 1}
	anchorMap.AddFromHeading("Hello World", pos)

	// Test Has
	if !anchorMap.Has("hello-world") {
		t.Error("Has('hello-world') = false, want true")
	}
	if anchorMap.Has("nonexistent") {
		t.Error("Has('nonexistent') = true, want false")
	}

	// Test HasIgnoreCase
	if !anchorMap.HasIgnoreCase("HELLO-WORLD") {
		t.Error("HasIgnoreCase('HELLO-WORLD') = false, want true")
	}

	// Test Lookup
	anchor := anchorMap.Lookup("hello-world")
	if anchor == nil {
		t.Fatal("Lookup('hello-world') = nil")
	}
	if anchor.Text != "Hello World" {
		t.Errorf("anchor.Text = %q, want %q", anchor.Text, "Hello World")
	}

	// Test LookupIgnoreCase
	anchor = anchorMap.LookupIgnoreCase("HELLO-WORLD")
	if anchor == nil {
		t.Fatal("LookupIgnoreCase('HELLO-WORLD') = nil")
	}
}

func TestAnchorMap_Count(t *testing.T) {
	anchorMap := NewAnchorMap()

	pos := mdast.SourcePosition{StartLine: 1, EndLine: 1}
	anchorMap.AddFromHeading("First", pos)
	anchorMap.AddFromHeading("Second", pos)
	anchorMap.AddFromHeading("First", pos) // duplicate

	if anchorMap.Count() != 3 { // first, second, first-1
		t.Errorf("Count() = %d, want 3", anchorMap.Count())
	}
}
