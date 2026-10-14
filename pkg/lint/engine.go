package lint

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// FileResult contains the results of linting a single file.
type FileResult struct {
	// Snapshot is the parsed file.
	Snapshot *mdast.FileSnapshot

	// Diagnostics contains all issues found.
	Diagnostics []Diagnostic

	// Edits contains validated, sorted edits for auto-fix.
	// Empty if no fixes are available or --fix was not requested.
	Edits []fix.TextEdit

	// SkippedEdits contains edits that were skipped due to conflicts.
	// When multiple edits overlap, earlier edits (by start position) take precedence.
	SkippedEdits []fix.TextEdit

	// EditConflicts is true if any edits were skipped due to conflicts.
	EditConflicts bool

	// RuleErrors contains any errors from rule execution.
	RuleErrors map[string]error
}

// HasIssues returns true if any diagnostics were found.
func (fr *FileResult) HasIssues() bool {
	return len(fr.Diagnostics) > 0
}

// HasFixes returns true if any fixes are available.
func (fr *FileResult) HasFixes() bool {
	return len(fr.Edits) > 0
}

// IssueCount returns the total number of diagnostics.
func (fr *FileResult) IssueCount() int {
	return len(fr.Diagnostics)
}

// FixableCount returns the number of diagnostics with fixes.
func (fr *FileResult) FixableCount() int {
	count := 0
	for _, d := range fr.Diagnostics {
		if d.HasFix() {
			count++
		}
	}
	return count
}

// Engine coordinates parsing and rule execution for linting.
type Engine struct {
	// Parser parses Markdown files into FileSnapshots.
	Parser Parser

	// Registry holds all available rules.
	Registry *Registry
}

// NewEngine creates a new Engine with the given parser and registry.
func NewEngine(parser Parser, registry *Registry) *Engine {
	return &Engine{
		Parser:   parser,
		Registry: registry,
	}
}

// LintFile parses and lints a single file.
func (e *Engine) LintFile(
	ctx context.Context,
	path string,
	content []byte,
	cfg *config.Config,
) (*FileResult, error) {
	// Parse the file.
	snapshot, err := e.Parser.Parse(ctx, path, content)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Resolve which rules to run.
	cfg, err = configureFile(collectDirectives(snapshot, e.Registry), e.Registry, cfg)
	if err != nil {
		return nil, fmt.Errorf("file configuration: %w", err)
	}
	states := directiveStates(snapshot, e.Registry, cfg)

	result := &FileResult{
		Snapshot:    snapshot,
		Diagnostics: nil,
		Edits:       nil,
		RuleErrors:  make(map[string]error),
	}

	// Collect all edits for validation.
	var allEdits []fix.TextEdit

	// Run each rule.
	for _, rule := range e.Registry.Rules() {
		rr := resolveRule(rule, cfg)
		if !slices.Contains(states[rule.ID()], true) {
			continue
		}
		// Check for cancellation.
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("linting cancelled: %w", ctx.Err())
		default:
		}

		// Create rule context.
		ruleCtx := NewRuleContext(ctx, snapshot, cfg, rr.Config)
		ruleCtx.Registry = e.Registry

		// Execute rule.
		diags, err := rr.Rule.Apply(ruleCtx)
		if err != nil {
			result.RuleErrors[rr.Rule.ID()] = err
			continue
		}

		// Process diagnostics.
		for diagIdx := range diags {
			line := diags[diagIdx].StartLine
			if line > 0 && line < len(states[rule.ID()]) && !states[rule.ID()][line] {
				continue
			}
			// Apply resolved severity.
			diags[diagIdx].Severity = rr.Severity

			// Ensure file path is set.
			if diags[diagIdx].FilePath == "" {
				diags[diagIdx].FilePath = path
			}

			// Ensure rule name is set for human-readable output.
			if diags[diagIdx].RuleName == "" {
				diags[diagIdx].RuleName = rr.Rule.Name()
			}

			// Collect edits if auto-fix is enabled for this rule.
			if rr.AutoFix && len(diags[diagIdx].FixEdits) > 0 {
				allEdits = append(allEdits, diags[diagIdx].FixEdits...)
			}
			result.Diagnostics = append(result.Diagnostics, diags[diagIdx])
		}
	}

	// Validate and prepare edits, merging deletions and filtering conflicts.
	if len(allEdits) > 0 {
		allEdits = mergeBlankLineInsertions(allEdits)
		accepted, skipped, _, err := fix.PrepareEditsFiltered(allEdits, len(content))
		if err != nil {
			// Validation error (not conflicts - those are filtered).
			// Still include diagnostics but clear edits.
			result.Edits = nil
			result.SkippedEdits = nil
			result.EditConflicts = true
		} else {
			result.Edits = accepted
			result.SkippedEdits = skipped
			result.EditConflicts = len(skipped) > 0
		}
	}

	return result, nil
}

// mergeBlankLineInsertions combines formatting requirements at the same boundary.
// It runs after suppression so disabled findings cannot increase an enabled fix.
func mergeBlankLineInsertions(edits []fix.TextEdit) []fix.TextEdit {
	result := make([]fix.TextEdit, 0, len(edits))
	boundaries := make(map[int]int)
	for _, edit := range edits {
		if edit.StartOffset == edit.EndOffset && edit.NewText != "" && strings.Trim(edit.NewText, "\n") == "" {
			if idx, ok := boundaries[edit.StartOffset]; ok {
				if len(edit.NewText) > len(result[idx].NewText) {
					result[idx] = edit
				}
				continue
			}
			boundaries[edit.StartOffset] = len(result)
		}
		result = append(result, edit)
	}
	return result
}
