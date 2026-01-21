package rules

import (
	"context"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
)

// testHelper parses markdown and runs a rule.
func testHelper(t *testing.T, rule lint.Rule, markdown string) []lint.Diagnostic {
	t.Helper()

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", []byte(markdown))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := &config.Config{}
	ruleCtx := lint.NewRuleContext(context.Background(), file, cfg, nil)

	diags, err := rule.Apply(ruleCtx)
	if err != nil {
		t.Fatalf("Rule.Apply failed: %v", err)
	}

	return diags
}

// testHelperWithOptions parses markdown and runs a rule with custom options.
func testHelperWithOptions(t *testing.T, rule lint.Rule, markdown string, options map[string]interface{}) []lint.Diagnostic {
	t.Helper()

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", []byte(markdown))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := &config.Config{}
	ruleCfg := &config.RuleConfig{Options: options}
	ruleCtx := lint.NewRuleContext(context.Background(), file, cfg, ruleCfg)

	diags, err := rule.Apply(ruleCtx)
	if err != nil {
		t.Fatalf("Rule.Apply failed: %v", err)
	}

	return diags
}

func TestLinkFragmentsRule(t *testing.T) {
	rule := NewLinkFragmentsRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "valid fragment",
			markdown: `# Hello World

[link](#hello-world)
`,
			want: 0,
		},
		{
			name: "invalid fragment",
			markdown: `# Hello World

[link](#nonexistent)
`,
			want: 1,
		},
		{
			name: "multiple headings",
			markdown: `# First Heading

## Second Heading

[first](#first-heading)
[second](#second-heading)
[invalid](#bad-fragment)
`,
			want: 1,
		},
		{
			name: "special top fragment",
			markdown: `# Heading

[back to top](#top)
`,
			want: 0,
		},
		{
			name: "github line reference",
			markdown: `# Heading

[see line 20](#L20)
[see lines 19-21](#L19-L21)
`,
			want: 0,
		},
		{
			name: "no fragment links",
			markdown: `# Heading

[external](https://example.com)
[relative](page.md)
`,
			want: 0,
		},
		{
			name: "empty document",
			markdown: `[link](#somewhere)
`,
			want: 1,
		},
		{
			name: "duplicate headings generate unique anchors",
			markdown: `# Hello

## Hello

[first](#hello)
[second](#hello-1)
[invalid](#hello-2)
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkFragmentsRule_IgnorePattern(t *testing.T) {
	rule := NewLinkFragmentsRule()

	markdown := `# Heading

[fig](#figure-1)
[tbl](#table-2)
`

	// Without ignore pattern - should report 2 issues
	diags := testHelper(t, rule, markdown)
	if len(diags) != 2 {
		t.Errorf("without pattern: got %d diagnostics, want 2", len(diags))
	}

	// With ignore pattern - should report 0 issues
	diags = testHelperWithOptions(t, rule, markdown, map[string]interface{}{
		"ignored_pattern": "^(figure|table)-\\d+$",
	})
	if len(diags) != 0 {
		t.Errorf("with pattern: got %d diagnostics, want 0", len(diags))
	}
}

func TestReferenceLinkImagesRule(t *testing.T) {
	rule := NewReferenceLinkImagesRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "defined reference",
			markdown: `[link][example]

[example]: https://example.com
`,
			want: 0,
		},
		{
			// Note: goldmark doesn't create Link nodes for undefined references,
			// so we can only test that defined references work correctly.
			name: "inline link no issue",
			markdown: `[link](https://example.com)
`,
			want: 0,
		},
		{
			name: "autolink no issue",
			markdown: `<https://example.com>
`,
			want: 0,
		},
		{
			name: "ignored label x",
			markdown: `- [x] Task item
`,
			want: 0, // [x] is in default ignored list
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkImageRefDefsRule(t *testing.T) {
	rule := NewLinkImageRefDefsRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "used definition",
			markdown: `[link][example]

[example]: https://example.com
`,
			want: 0,
		},
		{
			name: "unused definition",
			markdown: `Some text without links.

[unused]: https://example.com
`,
			want: 1,
		},
		{
			name: "duplicate definition",
			markdown: `[link][example]

[example]: https://first.com
[example]: https://second.com
`,
			want: 1, // second is duplicate
		},
		{
			name: "ignored definition",
			markdown: `Some text.

[//]: # "This is a comment"
`,
			want: 0, // // is in default ignored list
		},
		{
			name: "mixed used and unused",
			markdown: `[link][used]

[used]: https://used.com
[unused]: https://unused.com
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkImageStyleRule(t *testing.T) {
	rule := NewLinkImageStyleRule()

	tests := []struct {
		name     string
		markdown string
		options  map[string]interface{}
		want     int
	}{
		{
			name:     "all styles allowed by default",
			markdown: `[inline](url) and <https://autolink.com>`,
			options:  nil,
			want:     0,
		},
		{
			name:     "inline disallowed",
			markdown: `[inline](url)`,
			options:  map[string]interface{}{"inline": false},
			want:     1,
		},
		{
			name:     "autolink disallowed",
			markdown: `<https://example.com>`,
			options:  map[string]interface{}{"autolink": false},
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags []lint.Diagnostic
			if tt.options != nil {
				diags = testHelperWithOptions(t, rule, tt.markdown, tt.options)
			} else {
				diags = testHelper(t, rule, tt.markdown)
			}
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestDescriptiveLinkTextRule(t *testing.T) {
	rule := NewDescriptiveLinkTextRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name:     "descriptive text",
			markdown: `[Visit the documentation](url)`,
			want:     0,
		},
		{
			name:     "click here",
			markdown: `[click here](url)`,
			want:     1,
		},
		{
			name:     "here",
			markdown: `[here](url)`,
			want:     1,
		},
		{
			name:     "read more",
			markdown: `[Read More](url)`,
			want:     1, // case insensitive
		},
		{
			name:     "link text",
			markdown: `[link](url)`,
			want:     1,
		},
		{
			name:     "images not checked",
			markdown: `![click here](image.png)`,
			want:     0, // images have alt text, different concern
		},
		{
			name:     "multiple generic links",
			markdown: `[here](url1) and [click](url2) and [more](url3)`,
			want:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestDescriptiveLinkTextRule_CustomPatterns(t *testing.T) {
	rule := NewDescriptiveLinkTextRule()

	markdown := `[info](url) and [details](url)`

	// Default patterns - should not flag "info" or "details"
	diags := testHelper(t, rule, markdown)
	if len(diags) != 0 {
		t.Errorf("default patterns: got %d diagnostics, want 0", len(diags))
	}

	// Custom patterns including "info" and "details"
	diags = testHelperWithOptions(t, rule, markdown, map[string]interface{}{
		"patterns": []interface{}{"info", "details"},
	})
	if len(diags) != 2 {
		t.Errorf("custom patterns: got %d diagnostics, want 2", len(diags))
	}
}

func TestRuleMetadata(t *testing.T) {
	tests := []struct {
		rule         lint.Rule
		expectedID   string
		expectedName string
	}{
		{NewLinkFragmentsRule(), "MD051", "link-fragments"},
		{NewReferenceLinkImagesRule(), "MD052", "reference-links-images"},
		{NewLinkImageRefDefsRule(), "MD053", "link-image-reference-definitions"},
		{NewLinkImageStyleRule(), "MD054", "link-image-style"},
		{NewDescriptiveLinkTextRule(), "MD059", "descriptive-link-text"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedID, func(t *testing.T) {
			if tt.rule.ID() != tt.expectedID {
				t.Errorf("ID() = %q, want %q", tt.rule.ID(), tt.expectedID)
			}
			if tt.rule.Name() != tt.expectedName {
				t.Errorf("Name() = %q, want %q", tt.rule.Name(), tt.expectedName)
			}
		})
	}
}

func TestOptionalRulesDefaultDisabled(t *testing.T) {
	// These rules should be disabled by default
	optionalRules := []lint.Rule{
		NewLinkImageStyleRule(),
		NewDescriptiveLinkTextRule(),
	}

	for _, rule := range optionalRules {
		if rule.DefaultEnabled() {
			t.Errorf("%s.DefaultEnabled() = true, want false", rule.ID())
		}
	}

	// These rules should be enabled by default
	enabledRules := []lint.Rule{
		NewLinkFragmentsRule(),
		NewReferenceLinkImagesRule(),
		NewLinkImageRefDefsRule(),
	}

	for _, rule := range enabledRules {
		if !rule.DefaultEnabled() {
			t.Errorf("%s.DefaultEnabled() = false, want true", rule.ID())
		}
	}
}
