package rules

import (
	"fmt"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// InlineHTMLRule restricts the use of raw HTML in Markdown.
type InlineHTMLRule struct {
	lint.BaseRule
}

// NewInlineHTMLRule creates a new inline HTML rule.
func NewInlineHTMLRule() *InlineHTMLRule {
	return &InlineHTMLRule{
		BaseRule: lint.NewBaseRule(
			"MD033",
			"no-inline-html",
			"Inline HTML should be avoided or restricted to allowed elements",
			[]string{"html"},
			false, // Not auto-fixable.
		),
	}
}

// commonmarkAllowedHTMLElements returns the default allowed elements for CommonMark.
// CommonMark is strict - no HTML allowed by default.
func commonmarkAllowedHTMLElements() []string {
	return nil
}

// gfmAllowedHTMLElements returns the default allowed elements for GFM.
// Includes common formatting elements used in GitHub.
func gfmAllowedHTMLElements() []string {
	return []string{"br", "sup", "sub", "details", "summary", "kbd", "abbr"}
}

// DefaultEnabled returns false - this rule is opt-in.
func (r *InlineHTMLRule) DefaultEnabled() bool {
	return false
}

// Apply checks for inline HTML usage.
func (r *InlineHTMLRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	// Get allowed elements from config.
	allowedElements := r.getAllowedElements(ctx)
	allowedSet := make(map[string]bool)
	for _, el := range allowedElements {
		allowedSet[strings.ToLower(el)] = true
	}

	var diags []lint.Diagnostic

	// Check HTML blocks.
	htmlBlocks := ctx.HTMLBlocks()
	for _, block := range htmlBlocks {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		diag := r.checkHTMLNode(block, allowedSet, "HTML block")
		if diag != nil {
			diags = append(diags, *diag)
		}
	}

	// Check inline HTML.
	htmlInlines := ctx.HTMLInlines()
	for _, inline := range htmlInlines {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		diag := r.checkHTMLNode(inline, allowedSet, "Inline HTML")
		if diag != nil {
			diags = append(diags, *diag)
		}
	}

	return diags, nil
}

func (r *InlineHTMLRule) getAllowedElements(ctx *lint.RuleContext) []string {
	// Check for explicit configuration.
	if allowed := ctx.Option("allowed_elements", nil); allowed != nil {
		if list, ok := allowed.([]any); ok {
			result := make([]string, 0, len(list))
			for _, v := range list {
				if s, ok := v.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}

	// Use flavor-based defaults.
	if ctx.Config != nil && ctx.Config.Flavor == config.FlavorGFM {
		return gfmAllowedHTMLElements()
	}

	return commonmarkAllowedHTMLElements()
}

func (r *InlineHTMLRule) checkHTMLNode(
	node *mdast.Node,
	allowedSet map[string]bool,
	nodeType string,
) *lint.Diagnostic {
	if node == nil || node.File == nil {
		return nil
	}

	// Extract the HTML content.
	pos := node.SourcePosition()
	if !pos.IsValid() {
		return nil
	}

	// Get content from the source.
	var content []byte
	if pos.StartLine > 0 && pos.StartLine <= len(node.File.Lines) {
		line := node.File.Lines[pos.StartLine-1]
		startOffset := line.StartOffset + pos.StartColumn - 1
		endOffset := line.NewlineStart

		// For single-line nodes, use the end column if available.
		if pos.EndLine == pos.StartLine && pos.EndColumn > 0 {
			endOffset = line.StartOffset + pos.EndColumn
		}

		// Clamp offsets to valid range.
		if startOffset < 0 {
			startOffset = 0
		}
		if startOffset > len(node.File.Content) {
			startOffset = len(node.File.Content)
		}
		if endOffset > len(node.File.Content) {
			endOffset = len(node.File.Content)
		}

		if startOffset < endOffset {
			content = node.File.Content[startOffset:endOffset]
		}
	}

	if len(content) == 0 {
		return nil
	}

	tagName := lint.ExtractHTMLTagName(content)
	if tagName == "" {
		// Could be a comment or other HTML construct.
		diag := lint.NewDiagnostic(r.ID(), node,
			nodeType+" is not allowed").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove or replace with Markdown syntax").
			Build()
		return &diag
	}

	// Check if allowed.
	if allowedSet[tagName] {
		return nil
	}

	var suggestion string
	if len(allowedSet) > 0 {
		allowed := make([]string, 0, len(allowedSet))
		for k := range allowedSet {
			allowed = append(allowed, k)
		}
		suggestion = "Allowed elements: " + strings.Join(allowed, ", ")
	} else {
		suggestion = "Remove HTML or use Markdown syntax"
	}

	diag := lint.NewDiagnostic(r.ID(), node,
		fmt.Sprintf("HTML element '%s' is not allowed", tagName)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(suggestion).
		Build()
	return &diag
}
