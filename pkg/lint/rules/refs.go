// Package rules provides reference link/image tracking rules.
// These rules require document-wide analysis via the refs package.
package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// LinkFragmentsRule validates that link fragments reference valid anchors (MD051).
type LinkFragmentsRule struct {
	lint.BaseRule
}

// NewLinkFragmentsRule creates a new link fragments rule.
func NewLinkFragmentsRule() *LinkFragmentsRule {
	return &LinkFragmentsRule{
		BaseRule: lint.NewBaseRule(
			"MD051",
			"link-fragments",
			"Link fragments should be valid",
			[]string{"links"},
			false, // Not auto-fixable
		),
	}
}

// Apply checks that link fragments reference valid document anchors.
func (r *LinkFragmentsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	ignoreCase := ctx.OptionBool("ignore_case", false)
	ignoredPatternStr := ctx.OptionString("ignored_pattern", "")

	var ignoredPattern *regexp.Regexp
	if ignoredPatternStr != "" {
		var err error
		ignoredPattern, err = regexp.Compile(ignoredPatternStr)
		if err != nil {
			// Invalid pattern - ignore it
			ignoredPattern = nil
		}
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip if no fragment
		if usage.Fragment == "" {
			continue
		}

		// Only validate same-file fragments (e.g., "#heading").
		// External URLs have fragments pointing to external sites.
		// Local file paths (./file.md#anchor) would need cross-file validation.
		if !usage.IsSameFileFragment() {
			continue
		}

		fragment := strings.TrimPrefix(usage.Fragment, "#")

		// Skip if matches ignored pattern
		if ignoredPattern != nil && ignoredPattern.MatchString(fragment) {
			continue
		}

		// Validate fragment
		valid := refCtx.ValidateFragment(usage.Fragment)
		if !valid && ignoreCase {
			valid = refCtx.ValidateFragmentIgnoreCase(usage.Fragment)
		}

		if !valid {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
				fmt.Sprintf("Link fragment '#%s' does not match any heading", fragment)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use a valid heading anchor").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// ReferenceLinkImagesRule validates that reference labels are defined (MD052).
type ReferenceLinkImagesRule struct {
	lint.BaseRule
}

// NewReferenceLinkImagesRule creates a new reference links/images rule.
func NewReferenceLinkImagesRule() *ReferenceLinkImagesRule {
	return &ReferenceLinkImagesRule{
		BaseRule: lint.NewBaseRule(
			"MD052",
			"reference-links-images",
			"Reference links and images should use defined labels",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// Apply reports full and collapsed references, and shortcut references when
// shortcut_syntax is set, whose label matches no definition. The parser turns
// every reference with a defined label into a link or image, so only the
// reference syntax it left as text can be undefined.
func (r *ReferenceLinkImagesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	includeShortcut := ctx.OptionBool("shortcut_syntax", false)
	ignoredLabels := ctx.OptionStringSlice("ignored_labels", []string{"x"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, label := range ignoredLabels {
		ignoredSet[refs.NormalizeLabel(label)] = true
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if !usage.Unresolved || usage.Label == "" {
			continue
		}

		// Skip shortcut unless configured to check
		if usage.Style == refs.StyleShortcut && !includeShortcut {
			continue
		}

		// Skip ignored labels
		if ignoredSet[usage.NormalizedLabel] {
			continue
		}

		// A defined label can still stay text, for example when the link text
		// contains another link, so only report labels with no definition.
		if usage.ResolvedDefinition == nil {
			kind := "link"
			if usage.IsImage {
				kind = "image"
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
				fmt.Sprintf("Reference %s uses undefined label [%s]", kind, usage.Label)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Define the reference or use inline syntax").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// LinkImageRefDefsRule detects unused/duplicate reference definitions (MD053).
type LinkImageRefDefsRule struct {
	lint.BaseRule
}

// NewLinkImageRefDefsRule creates a new link/image reference definitions rule.
func NewLinkImageRefDefsRule() *LinkImageRefDefsRule {
	return &LinkImageRefDefsRule{
		BaseRule: lint.NewBaseRule(
			"MD053",
			"link-image-reference-definitions",
			"Link and image reference definitions should be needed",
			[]string{"links", "images"},
			true, // Auto-fixable (can remove unused/duplicate definitions)
		),
	}
}

// Apply checks for unused and duplicate reference definitions.
func (r *LinkImageRefDefsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	ignoredDefs := ctx.OptionStringSlice("ignored_definitions", []string{"//"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, def := range ignoredDefs {
		ignoredSet[refs.NormalizeLabel(def)] = true
	}

	var diags []lint.Diagnostic
	removal := newDefinitionRemoval(ctx, refCtx.AllDefinitions)

	for _, def := range refCtx.AllDefinitions {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip ignored definitions
		if ignoredSet[def.NormalizedLabel] {
			continue
		}

		var msg, suggestion string
		switch {
		case def.IsDuplicate:
			msg, suggestion = "Duplicate reference definition [%s]", "Remove duplicate definition"
		case def.UsageCount == 0:
			msg, suggestion = "Unused reference definition [%s]", "Remove unused definition or add a reference"
		default:
			continue
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, def.Position, fmt.Sprintf(msg, def.Label)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(suggestion)
		if builder := removal.fix(def); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// definitionRemoval builds the MD053 fix, which deletes a definition's lines.
type definitionRemoval struct {
	file *mdast.FileSnapshot
	// columns maps the first line of each definition to the column it starts at.
	columns map[int]int
	// absorbers are the lists and indented code blocks, which can take in
	// what follows them once a definition no longer separates them.
	absorbers []*mdast.Node
}

func newDefinitionRemoval(ctx *lint.RuleContext, defs []*refs.ReferenceDefinition) *definitionRemoval {
	columns := make(map[int]int, len(defs))
	for _, def := range defs {
		columns[def.Lines[0]] = def.Position.StartColumn
	}
	return &definitionRemoval{
		file:    ctx.File,
		columns: columns,
		absorbers: mdast.FindAll(ctx.Root, func(node *mdast.Node) bool {
			return node.Kind == mdast.NodeList || lint.IsIndentedCodeBlock(node)
		}),
	}
}

// fix deletes the lines of a definition, line endings included, when nothing
// else on them would be lost and the blocks around them parse the same
// without them. A byte order mark before the first line stays.
func (d *definitionRemoval) fix(def *refs.ReferenceDefinition) *fix.EditBuilder {
	first, last := def.Lines[0], def.Lines[1]
	if !def.Removable || first < 1 || last < first || last > len(d.file.Lines) || !d.keepsStructure(def) {
		return nil
	}
	builder := fix.NewEditBuilder()
	builder.Delete(d.file.Lines[first-1].StartOffset+lint.LineTextOffset(d.file, first), d.file.Lines[last-1].EndOffset)
	return builder
}

// keepsStructure reports whether the blocks around a definition parse the
// same once its lines are deleted. The line after it continues the
// definition's paragraph, so it must be blank or start another definition at
// the same column: anything else could start a different block once it
// follows a blank line (indented code, an ordered list not starting at 1)
// or leave its blockquote as a lazy line. After a list or an indented code
// block, the next non-blank line must also start such a definition, or the
// document must end, because the list could take in a list item or
// indented text and two indented code blocks would merge into one.
func (d *definitionRemoval) keepsStructure(def *refs.ReferenceDefinition) bool {
	first, last := def.Lines[0], def.Lines[1]
	startsSibling := func(line int) bool {
		column, ok := d.columns[line]
		return ok && column == def.Position.StartColumn
	}
	if next := last + 1; next <= len(d.file.Lines) && !d.isBlank(next) && !startsSibling(next) {
		return false
	}
	prev := first - 1
	for prev >= 1 && d.isBlank(prev) {
		prev--
	}
	if prev < 1 || !d.absorbs(prev, first) {
		return true
	}
	next := last + 1
	for next <= len(d.file.Lines) && d.isBlank(next) {
		next++
	}
	return next > len(d.file.Lines) || startsSibling(next)
}

// absorbs reports whether line prev belongs to a list or an indented code
// block that does not also hold line first.
func (d *definitionRemoval) absorbs(prev, first int) bool {
	for _, node := range d.absorbers {
		pos := node.SourcePosition()
		if !pos.IsValid() || prev < pos.StartLine || prev > pos.EndLine {
			continue
		}
		if first < pos.StartLine || first > pos.EndLine {
			return true
		}
	}
	return false
}

// isBlank reports whether a line holds nothing but spaces, tabs, and
// blockquote markers, so it is blank inside its blockquotes.
func (d *definitionRemoval) isBlank(line int) bool {
	return len(bytes.Trim(lint.LineContent(d.file, line), " \t\r>")) == 0
}

// LinkImageStyleRule enforces consistent link/image syntax styles (MD054).
type LinkImageStyleRule struct {
	lint.BaseRule
}

// NewLinkImageStyleRule creates a new link/image style rule.
func NewLinkImageStyleRule() *LinkImageStyleRule {
	return &LinkImageStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD054",
			"link-image-style",
			"Link and image style should be consistent",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// DefaultEnabled returns false for this optional rule.
func (r *LinkImageStyleRule) DefaultEnabled() bool {
	return false
}

// Apply checks link/image style consistency.
func (r *LinkImageStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()

	// Get allowed styles from config
	allowAutolink := ctx.OptionBool("autolink", true)
	allowInline := ctx.OptionBool("inline", true)
	allowFull := ctx.OptionBool("full", true)
	allowCollapsed := ctx.OptionBool("collapsed", true)
	allowShortcut := ctx.OptionBool("shortcut", true)
	allowURLInline := ctx.OptionBool("url_inline", true)

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Reference syntax the parser left as text is not a link or image.
		if usage.Unresolved {
			continue
		}

		allowed := true
		var msg string

		switch usage.Style {
		case refs.StyleAutolink:
			if !allowAutolink {
				allowed = false
				msg = "Autolinks are not allowed"
			}
		case refs.StyleInline:
			if !allowInline {
				allowed = false
				msg = "Inline links/images are not allowed"
			} else if !allowURLInline && isURLAsText(usage) {
				allowed = false
				msg = "URL as inline link text is not allowed; use autolink syntax"
			}
		case refs.StyleFull:
			if !allowFull {
				allowed = false
				msg = "Full reference syntax is not allowed"
			}
		case refs.StyleCollapsed:
			if !allowCollapsed {
				allowed = false
				msg = "Collapsed reference syntax is not allowed"
			}
		case refs.StyleShortcut:
			if !allowShortcut {
				allowed = false
				msg = "Shortcut reference syntax is not allowed"
			}
		}

		if !allowed {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use an allowed link/image style").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// isURLAsText checks if inline link has URL as both text and destination.
func isURLAsText(usage *refs.ReferenceUsage) bool {
	if usage.Style != refs.StyleInline {
		return false
	}
	// Check if text equals destination (URL as link text)
	return usage.Text == usage.Destination && isAbsoluteURL(usage.Destination)
}

// DescriptiveLinkTextRule checks for generic link text (MD059).
type DescriptiveLinkTextRule struct {
	lint.BaseRule
}

// NewDescriptiveLinkTextRule creates a new descriptive link text rule.
func NewDescriptiveLinkTextRule() *DescriptiveLinkTextRule {
	return &DescriptiveLinkTextRule{
		BaseRule: lint.NewBaseRule(
			"MD059",
			"descriptive-link-text",
			"Link text should be descriptive",
			[]string{"links", "accessibility"},
			false, // Not auto-fixable
		),
	}
}

// DefaultEnabled returns false for this optional rule.
func (r *DescriptiveLinkTextRule) DefaultEnabled() bool {
	return false
}

// getDefaultGenericPatterns returns markdownlint's default prohibited_texts.
func getDefaultGenericPatterns() []string {
	return []string{
		"click here",
		"here",
		"link",
		"more",
	}
}

// Apply checks for generic/non-descriptive link text. The prohibited texts
// come from prohibited_texts (markdownlint's name) or patterns. With the
// default markdownlint_compatible, link text is compared like markdownlint:
// punctuation is ignored, and links containing code or HTML are skipped.
func (r *DescriptiveLinkTextRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()

	// Get patterns from config, or use defaults
	patterns := ctx.OptionStringSlice("patterns", getDefaultGenericPatterns())
	if ctx.Option("prohibited_texts", nil) != nil {
		patterns = ctx.OptionStringSlice("prohibited_texts", patterns)
	}
	compatible := ctx.OptionBool("markdownlint_compatible", true)

	// Build pattern set (lowercase for case-insensitive matching)
	patternSet := make(map[string]bool)
	for _, p := range patterns {
		if compatible {
			p = normalizeMarkdownlintLinkText(p)
		}
		patternSet[strings.ToLower(strings.TrimSpace(p))] = true
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip images - they have alt text, not link text - and reference
		// syntax the parser left as text, which is not a link.
		if usage.IsImage || usage.Unresolved {
			continue
		}
		if compatible && (len(mdast.FindByKind(usage.Node, mdast.NodeCodeSpan)) > 0 ||
			len(mdast.FindByKind(usage.Node, mdast.NodeHTMLInline)) > 0) {
			continue
		}

		// Check if text is generic
		text := strings.ToLower(strings.TrimSpace(usage.Text))
		if compatible {
			text = normalizeMarkdownlintLinkText(text)
		}
		if text == "" {
			continue
		}

		if patternSet[text] {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
				fmt.Sprintf("Link text '%s' is not descriptive", usage.Text)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use descriptive text that explains the link destination").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func normalizeMarkdownlintLinkText(text string) string {
	text = strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			return char
		}
		return ' '
	}, text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
