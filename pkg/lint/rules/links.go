package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// ReversedLinkRule detects reversed link syntax: (text)[url] instead of [text](url).
type ReversedLinkRule struct {
	lint.BaseRule
}

// NewReversedLinkRule creates a new reversed link rule.
func NewReversedLinkRule() *ReversedLinkRule {
	return &ReversedLinkRule{
		BaseRule: lint.NewBaseRule(
			"MD011",
			"no-reversed-links",
			"Reversed link syntax (text)[url] should be [text](url)",
			[]string{"links"},
			true,
		),
	}
}

// reversedLinkPattern matches (text)[url] patterns.
var reversedLinkPattern = regexp.MustCompile(`\(([^)]*)\)\[([^\]]*)\]`)

// reversedLinkMatchIndices is the minimum number of submatch indices required.
// Pattern has 2 capture groups: full (0:1), text (2:3), url (4:5).
const reversedLinkMatchIndices = 6

// Apply checks for reversed link syntax in the file content.
func (r *ReversedLinkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Skip lines in code blocks.
		if lint.IsLineInCodeBlock(ctx.File, ctx.Root, lineNum) {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		matches := reversedLinkPattern.FindAllSubmatchIndex(lineContent, -1)

		for _, match := range matches {
			if len(match) < reversedLinkMatchIndices {
				continue
			}

			// match[0]:match[1] = full match
			// match[2]:match[3] = text (inside parens)
			// match[4]:match[5] = url (inside brackets)
			text := string(lineContent[match[2]:match[3]])
			url := string(lineContent[match[4]:match[5]])

			line := ctx.File.Lines[lineNum-1]
			startOffset := line.StartOffset + match[0]
			endOffset := line.StartOffset + match[1]

			// Build fix: convert (text)[url] to [text](url)
			builder := fix.NewEditBuilder()
			newText := fmt.Sprintf("[%s](%s)", text, url)
			builder.ReplaceRange(startOffset, endOffset, newText)

			pos := mdast.SourcePosition{
				StartLine:   lineNum,
				StartColumn: match[0] + 1,
				EndLine:     lineNum,
				EndColumn:   match[1],
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				"Reversed link syntax detected").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use [text](url) instead of (text)[url]").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// LinkSpacesRule detects spaces inside link text: [ text ] instead of [text].
type LinkSpacesRule struct {
	lint.BaseRule
}

// NewLinkSpacesRule creates a new link spaces rule.
func NewLinkSpacesRule() *LinkSpacesRule {
	return &LinkSpacesRule{
		BaseRule: lint.NewBaseRule(
			"MD039",
			"no-space-in-links",
			"Link text should not have leading or trailing spaces",
			[]string{"links", "whitespace"},
			true,
		),
	}
}

// Apply checks for spaces inside link text.
func (r *LinkSpacesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	links := lint.Links(ctx.Root)
	var diags []lint.Diagnostic

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		text := lint.LinkText(link)
		if text == "" {
			continue
		}

		trimmed := strings.TrimSpace(text)
		if text == trimmed {
			continue
		}

		// Text has leading or trailing spaces.
		hasLeading := len(text) > 0 && (text[0] == ' ' || text[0] == '\t')
		hasTrailing := len(text) > 0 && (text[len(text)-1] == ' ' || text[len(text)-1] == '\t')

		var msg string
		switch {
		case hasLeading && hasTrailing:
			msg = "Link text has leading and trailing spaces"
		case hasLeading:
			msg = "Link text has leading spaces"
		default:
			msg = "Link text has trailing spaces"
		}

		diag := lint.NewDiagnostic(r.ID(), link, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove spaces from link text").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// EmptyLinkRule detects links with empty destination or text.
type EmptyLinkRule struct {
	lint.BaseRule
}

// NewEmptyLinkRule creates a new empty link rule.
func NewEmptyLinkRule() *EmptyLinkRule {
	return &EmptyLinkRule{
		BaseRule: lint.NewBaseRule(
			"MD042",
			"no-empty-links",
			"Links should have both text and destination",
			[]string{"links"},
			false, // Not auto-fixable.
		),
	}
}

// Apply checks for empty links.
func (r *EmptyLinkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	links := lint.Links(ctx.Root)
	var diags []lint.Diagnostic

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		dest := lint.LinkDestination(link)
		text := lint.LinkText(link)

		emptyDest := dest == ""
		emptyText := len(bytes.TrimSpace([]byte(text))) == 0

		if !emptyDest && !emptyText {
			continue
		}

		var msg string
		switch {
		case emptyDest && emptyText:
			msg = "Link has empty text and destination"
		case emptyDest:
			msg = "Link has empty destination"
		default:
			msg = "Link has empty text"
		}

		diag := lint.NewDiagnostic(r.ID(), link, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Provide both link text and destination").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// ImageAltTextRule checks that images have alt text.
type ImageAltTextRule struct {
	lint.BaseRule
}

// NewImageAltTextRule creates a new image alt text rule.
func NewImageAltTextRule() *ImageAltTextRule {
	return &ImageAltTextRule{
		BaseRule: lint.NewBaseRule(
			"MD045",
			"no-alt-text",
			"Images should have alt text",
			[]string{"links", "images", "accessibility"},
			false, // Not auto-fixable.
		),
	}
}

// Apply checks that all images have non-empty alt text.
func (r *ImageAltTextRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	images := lint.Images(ctx.Root)
	var diags []lint.Diagnostic

	for _, img := range images {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		alt := lint.ImageAlt(img)
		if len(bytes.TrimSpace([]byte(alt))) > 0 {
			continue
		}

		diag := lint.NewDiagnostic(r.ID(), img, "Image is missing alt text").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add descriptive alt text to the image").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// LinkDestinationStyleRule enforces link destination style (relative vs absolute).
type LinkDestinationStyleRule struct {
	lint.BaseRule
}

// NewLinkDestinationStyleRule creates a new link destination style rule.
func NewLinkDestinationStyleRule() *LinkDestinationStyleRule {
	return &LinkDestinationStyleRule{
		BaseRule: lint.NewBaseRule(
			"MDL001",
			"link-destination-style",
			"Link destination style should be consistent",
			[]string{"links", "style"},
			false, // Not auto-fixable.
		),
	}
}

// LinkDestStyle represents the style of link destinations.
type LinkDestStyle string

const (
	// LinkDestRelative requires relative URLs.
	LinkDestRelative LinkDestStyle = "relative"
	// LinkDestAbsolute requires absolute URLs.
	LinkDestAbsolute LinkDestStyle = "absolute"
	// LinkDestConsistent uses whatever style is first encountered.
	LinkDestConsistent LinkDestStyle = "consistent"
)

// DefaultEnabled returns false for this optional rule.
func (r *LinkDestinationStyleRule) DefaultEnabled() bool {
	return false
}

// Apply checks link destination style consistency.
func (r *LinkDestinationStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	configStyle := LinkDestStyle(ctx.OptionString("style", string(LinkDestConsistent)))
	effectiveStyle := configStyle
	if configStyle == LinkDestConsistent {
		effectiveStyle = "" // Will be set from first link.
	}

	links := lint.Links(ctx.Root)
	var diags []lint.Diagnostic

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		dest := lint.LinkDestination(link)
		if dest == "" {
			continue
		}

		// Skip fragment-only links (#anchor).
		if strings.HasPrefix(dest, "#") {
			continue
		}

		isAbsolute := isAbsoluteURL(dest)
		detectedStyle := LinkDestRelative
		if isAbsolute {
			detectedStyle = LinkDestAbsolute
		}

		// Set consistent style from first link.
		if effectiveStyle == "" {
			effectiveStyle = detectedStyle
			continue
		}

		// Check for style mismatch.
		if detectedStyle != effectiveStyle {
			var msg string
			if effectiveStyle == LinkDestAbsolute {
				msg = "Link uses relative URL, but absolute URLs are expected"
			} else {
				msg = "Link uses absolute URL, but relative URLs are expected"
			}

			diag := lint.NewDiagnostic(r.ID(), link, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %s URLs consistently", effectiveStyle)).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// isAbsoluteURL returns true if the URL is absolute (has a scheme).
func isAbsoluteURL(url string) bool {
	// Check for common schemes.
	return strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "ftp://") ||
		strings.HasPrefix(url, "mailto:") ||
		strings.HasPrefix(url, "tel:") ||
		strings.HasPrefix(url, "file://") ||
		strings.Contains(url, "://")
}
