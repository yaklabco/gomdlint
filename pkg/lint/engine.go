package lint

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Errors wrapped by RuleError to classify rule failures.
var (
	// ErrRulePanic marks a rule that panicked while linting a file.
	ErrRulePanic = errors.New("rule panicked")

	// ErrInvalidFix marks a rule that proposed fix edits that do not fit the
	// file: out of range, overlapping each other, or splitting a line ending
	// or a UTF-8 character. The finding is still reported, without its fix.
	ErrInvalidFix = errors.New("invalid fix")
)

// RuleError records a failure of one rule on one file. A rule with an error
// may have reported incomplete findings for that file.
type RuleError struct {
	// RuleID is the ID of the rule that failed.
	RuleID string

	// RuleName is the human-readable name of the rule that failed.
	RuleName string

	// Err is the underlying error. It wraps ErrRulePanic for a recovered
	// panic and ErrInvalidFix for invalid fix edits; otherwise it is the
	// error the rule returned.
	Err error

	// Stack is the stack trace captured when the rule panicked, or nil.
	Stack []byte
}

// Error implements error.
func (e *RuleError) Error() string {
	return fmt.Sprintf("rule %s (%s): %v", e.RuleID, e.RuleName, e.Err)
}

// Unwrap returns the underlying error.
func (e *RuleError) Unwrap() error {
	return e.Err
}

// SkippedEdit is a fix edit that was proposed but not applied because its fix
// conflicted with an earlier fix.
type SkippedEdit struct {
	// RuleID identifies the rule that proposed the edit.
	RuleID string

	// Edit is the proposed edit.
	Edit fix.TextEdit
}

// FileResult contains the results of linting a single file.
type FileResult struct {
	// Snapshot is the parsed file.
	Snapshot *mdast.FileSnapshot

	// Diagnostics contains all issues found.
	Diagnostics []Diagnostic

	// Edits contains validated, sorted, conflict-free edits for auto-fix.
	// Empty if no fixes are available or --fix was not requested.
	Edits []fix.TextEdit

	// SkippedEdits contains the edits of fixes that were not applied because
	// they conflicted with an earlier fix. A fix is applied whole or not at
	// all, so every edit of a skipped fix is listed. The multi-pass pipeline
	// proposes a skipped fix again on the next pass.
	SkippedEdits []SkippedEdit

	// EditConflicts is true if any edits were skipped due to conflicts.
	EditConflicts bool

	// RuleErrors maps a rule ID to that rule's failure on this file: an error
	// the rule returned, a recovered panic (ErrRulePanic) or invalid fix edits
	// (ErrInvalidFix). Every value is a *RuleError.
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

// HasRuleErrors reports whether any rule failed on this file.
func (fr *FileResult) HasRuleErrors() bool {
	return len(fr.RuleErrors) > 0
}

// RuleErrorList returns the rule errors sorted by rule ID.
func (fr *FileResult) RuleErrorList() []*RuleError {
	if len(fr.RuleErrors) == 0 {
		return nil
	}

	ids := slices.Sorted(maps.Keys(fr.RuleErrors))
	list := make([]*RuleError, 0, len(ids))
	for _, id := range ids {
		var ruleErr *RuleError
		if !errors.As(fr.RuleErrors[id], &ruleErr) {
			ruleErr = &RuleError{RuleID: id, Err: fr.RuleErrors[id]}
		}
		list = append(list, ruleErr)
	}
	return list
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
//
// A rule that returns an error, panics, or proposes invalid fix edits does
// not stop the others: the failure is recorded in FileResult.RuleErrors and
// only that rule's results (or only the invalid fix) are dropped. The
// newlines in fix text are written with the file's dominant line ending.
func (e *Engine) LintFile(
	ctx context.Context,
	path string,
	content []byte,
	cfg *config.Config,
) (*FileResult, error) {
	return e.lintFile(ctx, path, content, cfg, fix.DetectLineEnding(content))
}

// lintFile lints content, writing the newlines in fix text as lineEnding.
func (e *Engine) lintFile(
	ctx context.Context,
	path string,
	content []byte,
	cfg *config.Config,
	lineEnding string,
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
		Snapshot:   snapshot,
		RuleErrors: make(map[string]error),
	}

	validator := fix.NewValidator(content)
	var fixes []pendingFix

	// Run each rule.
	for _, rule := range e.Registry.Rules() {
		rr := resolveRule(rule, cfg)
		ruleStates := states[rule.ID()]
		if !slices.Contains(ruleStates, true) {
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
		ruleCtx.lineEnding = lineEnding

		diags, err := applyRule(ctx, rule, ruleCtx, path)
		if err != nil {
			result.RuleErrors[rule.ID()] = err
			continue
		}

		var invalid []error
		for _, diag := range diags {
			line := diag.StartLine
			if line > 0 && line < len(ruleStates) && !ruleStates[line] {
				continue
			}
			// Apply resolved severity.
			diag.Severity = rr.Severity

			// Ensure file path is set.
			if diag.FilePath == "" {
				diag.FilePath = path
			}

			// Ensure rule name is set for human-readable output.
			if diag.RuleName == "" {
				diag.RuleName = rule.Name()
			}

			if len(diag.FixEdits) > 0 {
				edits, err := validator.Validate(diag.FixEdits)
				if err != nil {
					// Report the finding without its broken fix.
					invalid = append(invalid, fmt.Errorf("fix for line %d: %w", diag.StartLine, err))
					diag.FixEdits = nil
				} else {
					diag.FixEdits = convertNewlines(edits, lineEnding)
					// Collect the fix if auto-fix is enabled for this rule.
					if rr.AutoFix {
						fixes = append(fixes, pendingFix{ruleID: rule.ID(), edits: diag.FixEdits})
					}
				}
			}
			result.Diagnostics = append(result.Diagnostics, diag)
		}

		if len(invalid) > 0 {
			result.RuleErrors[rule.ID()] = &RuleError{
				RuleID:   rule.ID(),
				RuleName: rule.Name(),
				Err:      fmt.Errorf("%w: %w", ErrInvalidFix, errors.Join(invalid...)),
			}
		}
	}

	result.Edits, result.SkippedEdits = resolveFixes(mergeBlankLineInsertions(fixes))
	result.EditConflicts = len(result.SkippedEdits) > 0

	return result, nil
}

// applyRule runs one rule. A returned error or a panic becomes a *RuleError;
// a panic's stack is written to the debug log.
func applyRule(ctx context.Context, rule Rule, ruleCtx *RuleContext, path string) ([]Diagnostic, error) {
	var (
		diags    []Diagnostic
		applyErr error
		panicErr *RuleError
	)

	func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			stack := debug.Stack()
			logging.FromContext(ctx).Debug("rule panicked",
				"rule", rule.ID(), "path", path, "panic", recovered, "stack", string(stack))

			cause, ok := recovered.(error)
			if !ok {
				cause = errors.New(fmt.Sprint(recovered))
			}
			panicErr = &RuleError{
				RuleID:   rule.ID(),
				RuleName: rule.Name(),
				Err:      fmt.Errorf("%w: %w", ErrRulePanic, cause),
				Stack:    stack,
			}
		}()
		diags, applyErr = rule.Apply(ruleCtx)
	}()

	if panicErr != nil {
		return nil, panicErr
	}
	if applyErr != nil {
		return nil, &RuleError{RuleID: rule.ID(), RuleName: rule.Name(), Err: applyErr}
	}
	return diags, nil
}

// pendingFix is the fix of one diagnostic, waiting for conflict resolution.
type pendingFix struct {
	ruleID string
	edits  []fix.TextEdit
}

// resolveFixes picks the fixes to apply and lists the edits of the others.
func resolveFixes(fixes []pendingFix) ([]fix.TextEdit, []SkippedEdit) {
	if len(fixes) == 0 {
		return nil, nil
	}

	groups := make([][]fix.TextEdit, len(fixes))
	for idx, pending := range fixes {
		groups[idx] = pending.edits
	}
	accepted, skippedFixes := fix.ResolveFixes(groups)

	var skipped []SkippedEdit
	for _, idx := range skippedFixes {
		for _, edit := range fixes[idx].edits {
			skipped = append(skipped, SkippedEdit{RuleID: fixes[idx].ruleID, Edit: edit})
		}
	}
	return accepted, skipped
}

// convertNewlines writes the newlines in the text of edits as lineEnding, in
// place. The engine only passes its own copies from fix.Validator.
func convertNewlines(edits []fix.TextEdit, lineEnding string) []fix.TextEdit {
	for idx := range edits {
		edits[idx].NewText = fix.ConvertNewlines(edits[idx].NewText, lineEnding)
	}
	return edits
}

// mergeBlankLineInsertions combines formatting requirements at the same
// boundary: of several single-edit fixes that insert only blank lines at
// one offset, the one asking for the most blank lines is kept (see
// moreBlankLines). It runs after suppression so disabled findings cannot
// increase an enabled fix.
func mergeBlankLineInsertions(fixes []pendingFix) []pendingFix {
	result := make([]pendingFix, 0, len(fixes))
	boundaries := make(map[int]int)
	for _, pending := range fixes {
		if len(pending.edits) == 1 && isBlankLineInsertion(pending.edits[0]) {
			offset := pending.edits[0].StartOffset
			if idx, ok := boundaries[offset]; ok {
				if moreBlankLines(pending.edits[0].NewText, result[idx].edits[0].NewText) {
					result[idx] = pending
				}
				continue
			}
			boundaries[offset] = len(result)
		}
		result = append(result, pending)
	}
	return result
}

// moreBlankLines reports whether blank-line text a asks for more than b:
// more line breaks, or as many with a longer container prefix (so ">\n"
// wins over "\n" inside a blockquote).
func moreBlankLines(a, b string) bool {
	if linesA, linesB := strings.Count(a, "\n"), strings.Count(b, "\n"); linesA != linesB {
		return linesA > linesB
	}
	return len(a) > len(b)
}

// isBlankLineInsertion reports whether edit only inserts blank lines: text
// that ends with a line break and whose lines hold nothing but container
// markers (spaces, tabs and '>'), such as "\n", ">\n" or "  > >\r\n". Rules
// insert such lines to add a blank line inside a blockquote or list item.
func isBlankLineInsertion(edit fix.TextEdit) bool {
	if edit.StartOffset != edit.EndOffset || !strings.HasSuffix(edit.NewText, "\n") {
		return false
	}
	for _, line := range strings.SplitAfter(edit.NewText, "\n") {
		if strings.Trim(line, " \t>\r\n") != "" {
			return false
		}
	}
	return true
}
