package rules

import (
	"fmt"
	"sort"
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

// DefaultEnabled returns false - this rule is opt-in.
func (r *InlineHTMLRule) DefaultEnabled() bool {
	return false
}

// Apply reports every opening HTML element that is not allowed. HTML blocks
// are scanned for each element they contain, like markdownlint, which
// re-parses HTML blocks as inline content. Closing tags, comments,
// declarations, and CDATA sections are never reported.
func (r *InlineHTMLRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// markdownlint defaults allowed_elements to [], and table_allowed_elements
	// falls back to allowed_elements when it is not set.
	allowedElements := ctx.OptionStringSlice("allowed_elements", nil)
	allowed := lowercaseSet(allowedElements)
	tableAllowed := lowercaseSet(ctx.OptionStringSlice("table_allowed_elements", allowedElements))

	var diags []lint.Diagnostic
	report := func(construct htmlConstruct, allowedSet map[string]bool) {
		if !construct.isOpeningElement() || allowedSet[strings.ToLower(construct.Name)] {
			return
		}
		diags = append(diags, r.elementDiagnostic(ctx, construct, allowedSet))
	}

	for _, block := range ctx.HTMLBlocks() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		span := block.SourceRange()
		for _, construct := range scanHTMLConstructs(ctx.File.Content, span.StartOffset, span.EndOffset) {
			report(construct, allowed)
		}
	}

	for _, inline := range ctx.HTMLInlines() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		construct, ok := htmlNodeConstruct(inline)
		if !ok {
			continue
		}
		allowedSet := allowed
		if lint.IsLineInTable(ctx.File, ctx.Root, inline.SourcePosition().StartLine) {
			allowedSet = tableAllowed
		}
		report(construct, allowedSet)
	}

	sortDiagnosticsByPosition(diags)
	return diags, nil
}

// elementDiagnostic builds the diagnostic for a disallowed element. The range
// covers the element's opening tag up to the end of its first line.
func (r *InlineHTMLRule) elementDiagnostic(
	ctx *lint.RuleContext,
	construct htmlConstruct,
	allowedSet map[string]bool,
) lint.Diagnostic {
	end := construct.End
	if newline := strings.IndexAny(string(construct.text(ctx.File.Content)), "\r\n"); newline >= 0 {
		end = construct.Start + newline
	}
	startLine, startColumn := ctx.File.LineAt(construct.Start)
	endLine, endColumn := ctx.File.LineAt(end)
	pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}

	suggestion := "Remove HTML or use Markdown syntax"
	if len(allowedSet) > 0 {
		names := make([]string, 0, len(allowedSet))
		for name := range allowedSet {
			names = append(names, name)
		}
		sort.Strings(names)
		suggestion = "Allowed elements: " + strings.Join(names, ", ")
	}

	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
		fmt.Sprintf("HTML element '%s' is not allowed", construct.Name)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(suggestion).
		Build()
}

// lowercaseSet returns the set of lowercased values.
func lowercaseSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[strings.ToLower(value)] = true
	}
	return set
}
