package lint

import (
	"bytes"
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

// SkippedEdit is a fix edit that was proposed but not applied in this pass:
// its fix conflicted with an earlier fix (FileResult.SkippedEdits), or was
// deferred to the next pass (FileResult.DeferredEdits).
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

	// Edits contains the validated, sorted, conflict-free edits to apply in
	// this pass. Each top-level block gets the fixes of one rule; see
	// DeferredEdits. Empty if no fixes are available or --fix was not
	// requested.
	Edits []fix.TextEdit

	// AcceptedFixes is the number of diagnostics whose fixes Edits holds. It
	// can differ from len(Edits): one fix may take several edits, and the
	// blank-line fixes of several diagnostics of one rule at one boundary
	// share one.
	AcceptedFixes int

	// SkippedEdits contains the edits of fixes that were not applied because
	// they conflicted with an earlier fix of the same rule. A fix is applied
	// whole or not at all, so every edit of a skipped fix is listed. The
	// multi-pass pipeline proposes a skipped fix again on the next pass.
	SkippedEdits []SkippedEdit

	// EditConflicts is true if any edits were skipped due to conflicts.
	EditConflicts bool

	// DeferredEdits contains the edits of fixes held back for the next pass
	// because another rule's fixes are applied to the same top-level block
	// (the front matter, or a child of the document root) in this pass.
	// Fixes of different rules can each be safe and still corrupt a block
	// together, so the multi-pass pipeline lints the fixed content and each
	// rule proposes its fixes again. Deferred fixes are not conflicts.
	DeferredEdits []SkippedEdit

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

// RuleErrorList returns the rule errors sorted by rule ID, skipping rules
// recorded without an error.
func (fr *FileResult) RuleErrorList() []*RuleError {
	if len(fr.RuleErrors) == 0 {
		return nil
	}

	ids := slices.Sorted(maps.Keys(fr.RuleErrors))
	list := make([]*RuleError, 0, len(ids))
	for _, id := range ids {
		err := fr.RuleErrors[id]
		if err == nil {
			continue
		}
		var ruleErr *RuleError
		if !errors.As(err, &ruleErr) {
			ruleErr = &RuleError{RuleID: id, Err: err}
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
// The fixes of one rule per top-level block are returned as Edits, and the
// other rules' fixes in those blocks as DeferredEdits.
//
// A diagnostic keeps its fix edits only when --fix would apply them (see
// ResolvedRule.Fixable), whether or not cfg has Fix set, so FixableCount
// counts the findings that --fix would fix.
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

			// A fix that --fix would not apply (auto_fix: false, --fix-rules,
			// or a rule that declares no fixes) is not offered, so the finding
			// does not count as fixable.
			if !rr.Fixable {
				diag.FixEdits = nil
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
						fixes = append(fixes, pendingFix{ruleID: rule.ID(), edits: diag.FixEdits, diagnostics: 1})
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

	selectEdits(result, fixes, content, newTopLevelBlocks(snapshot))
	return result, nil
}

// selectEdits decides which fixes this pass applies to content and records
// the outcome in result. Each rule's blank-line insertions at one boundary
// are merged first. Then each top-level block keeps the fixes of one rule,
// and the other rules' fixes there are deferred (see selectFixes). Finally
// the fixes taken are resolved against each other, so a fix that overlaps
// another fix of the same rule is skipped.
//
// Fixes that change nothing must not keep another rule's fixes out of a
// block, or the pass would end without progress while those wait. A fix
// that changes nothing by itself takes no part, and neither does a rule's
// group of fixes that changes nothing together (see selectFixes). If the
// selection as a whole still changes nothing while fixes wait, the rules
// it holds sit out the pass and the fixes are chosen again, so a pass whose
// edits change nothing never defers a fix. Fixes that take no part are
// offered only when nothing else can be applied, and only if together they
// change nothing, so the pipeline sees that the remaining fixes make no
// progress without applying two rules' fixes to one block. Their conflicts
// are reported as skipped, like those of any fixes the pass applies.
func selectEdits(result *FileResult, fixes []pendingFix, content []byte, blocks *topLevelBlocks) {
	changes, idle := splitNoops(fixes, content)
	merged, conflicting := mergeBlankLineInsertions(changes)
	for idx := range conflicting {
		conflicting[idx].mergeConflict = true
	}
	candidates := slices.Concat(merged, conflicting)

	var selection fixSelection
	var conflicts []pendingFix
	for {
		selection = selectFixes(candidates, blocks, content)
		var accepted []pendingFix
		accepted, conflicts = splitMergeConflicts(selection.selected)
		result.Edits, result.SkippedEdits, result.AcceptedFixes = resolveFixes(accepted)
		if len(selection.deferred) == 0 || changesContent(content, result.Edits) {
			break
		}
		// Every rule's fixes here change content, but together they do not.
		// Each round removes at least one rule, so the loop ends.
		held := make(map[string]bool)
		for _, pending := range selection.selected {
			held[pending.ruleID] = true
		}
		kept := candidates[:0:0]
		for _, pending := range candidates {
			if held[pending.ruleID] {
				idle = append(idle, pending)
			} else {
				kept = append(kept, pending)
			}
		}
		candidates = kept
	}
	idle = append(idle, selection.idle...)

	if len(result.Edits) == 0 {
		// A blank-line insertion that conflicts with another at its boundary
		// is never applied, so it is reported, not offered.
		offered, idleConflicts := splitMergeConflicts(idle)
		if edits, skipped, count := resolveFixes(offered); !changesContent(content, edits) {
			result.Edits, result.AcceptedFixes = edits, count
			result.SkippedEdits = append(result.SkippedEdits, skipped...)
			result.SkippedEdits = appendEditsOf(result.SkippedEdits, idleConflicts)
		}
	}
	result.SkippedEdits = appendEditsOf(result.SkippedEdits, conflicts)
	result.EditConflicts = len(result.SkippedEdits) > 0
	result.DeferredEdits = appendEditsOf(nil, selection.deferred)
}

// splitMergeConflicts separates the selected fixes the pass applies from
// the blank-line insertions that conflict with another insertion at their
// boundary, which the pass skips.
func splitMergeConflicts(selected []pendingFix) ([]pendingFix, []pendingFix) {
	var accepted, conflicts []pendingFix
	for _, pending := range selected {
		if pending.mergeConflict {
			conflicts = append(conflicts, pending)
		} else {
			accepted = append(accepted, pending)
		}
	}
	return accepted, conflicts
}

// splitNoops separates the fixes that change content from the fixes that,
// applied, would leave it as it is.
func splitNoops(fixes []pendingFix, content []byte) ([]pendingFix, []pendingFix) {
	var changes, noops []pendingFix
	for _, pending := range fixes {
		if changesContent(content, pending.edits) {
			changes = append(changes, pending)
		} else {
			noops = append(noops, pending)
		}
	}
	return changes, noops
}

// changesContent reports whether applying edits, sorted and without
// overlaps as fix.Validator returns them, would change content. Edits can
// change nothing each, or only together, as when one deletes a character
// and the next inserts it again where it was.
//
// Content that keeps its length matches the fixed content before the first
// edit and after the last one, so only the text in between is compared,
// piece by piece, and the comparison stops at the first difference.
func changesContent(content []byte, edits []fix.TextEdit) bool {
	delta := 0
	for _, edit := range edits {
		delta += len(edit.NewText) - (edit.EndOffset - edit.StartOffset)
	}
	if delta != 0 {
		return true
	}
	if len(edits) == 0 {
		return false
	}

	// pos is the offset in content that the next piece of fixed text must
	// match; kept is where the content the edits keep resumes.
	pos, kept := edits[0].StartOffset, edits[0].StartOffset
	for _, edit := range edits {
		unchanged := content[kept:edit.StartOffset]
		if pos+len(unchanged) > len(content) || !bytes.Equal(content[pos:pos+len(unchanged)], unchanged) {
			return true
		}
		pos += len(unchanged)
		if pos+len(edit.NewText) > len(content) || string(content[pos:pos+len(edit.NewText)]) != edit.NewText {
			return true
		}
		pos += len(edit.NewText)
		kept = edit.EndOffset
	}
	return false
}

// appendEditsOf appends every edit of fixes, with its rule.
func appendEditsOf(edits []SkippedEdit, fixes []pendingFix) []SkippedEdit {
	for _, pending := range fixes {
		for _, edit := range pending.edits {
			edits = append(edits, SkippedEdit{RuleID: pending.ruleID, Edit: edit})
		}
	}
	return edits
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

	// diagnostics is the number of diagnostics the fix fixes: one, or more
	// for one rule's blank-line insertions merged into one fix.
	diagnostics int

	// mergeConflict marks a blank-line insertion for a different container
	// than another insertion at the same boundary.
	mergeConflict bool
}

// resolveFixes picks the fixes to apply and lists the edits of the others.
// It also returns the number of diagnostics the accepted fixes fix.
func resolveFixes(fixes []pendingFix) ([]fix.TextEdit, []SkippedEdit, int) {
	if len(fixes) == 0 {
		return nil, nil, 0
	}

	groups := make([][]fix.TextEdit, len(fixes))
	fixed := 0
	for idx, pending := range fixes {
		groups[idx] = pending.edits
		fixed += pending.diagnostics
	}
	accepted, skippedFixes := fix.ResolveFixes(groups)

	skipped := make([]pendingFix, 0, len(skippedFixes))
	for _, idx := range skippedFixes {
		fixed -= fixes[idx].diagnostics
		skipped = append(skipped, fixes[idx])
	}
	return accepted, appendEditsOf(nil, skipped), fixed
}

// convertNewlines writes the newlines in the text of edits as lineEnding, in
// place. The engine only passes its own copies from fix.Validator.
func convertNewlines(edits []fix.TextEdit, lineEnding string) []fix.TextEdit {
	for idx := range edits {
		edits[idx].NewText = fix.ConvertNewlines(edits[idx].NewText, lineEnding)
	}
	return edits
}

// mergeBlankLineInsertions combines one rule's formatting requirements at
// the same boundary: several single-edit fixes of that rule that insert
// only blank lines at one offset become one insertion (see
// mergeBlankLines), which would otherwise insert the blank lines twice. It
// runs after suppression so disabled findings cannot increase an enabled
// fix. Fixes whose blank lines belong to a different container than an
// earlier fix of the rule at the same offset are returned separately, as
// conflicts.
//
// Insertions of different rules are not merged. Two insertions at one
// offset touch the same top-level blocks, so selectFixes applies one
// rule's and defers the other's, and the next pass lints the content with
// the blank line in place.
func mergeBlankLineInsertions(fixes []pendingFix) ([]pendingFix, []pendingFix) {
	result := make([]pendingFix, 0, len(fixes))
	var conflicting []pendingFix
	boundaries := make(map[blankLineBoundary]int)
	for _, pending := range fixes {
		if len(pending.edits) == 1 && isBlankLineInsertion(pending.edits[0]) {
			offset := pending.edits[0].StartOffset
			key := blankLineBoundary{ruleID: pending.ruleID, offset: offset}
			if idx, ok := boundaries[key]; ok {
				text, ok := mergeBlankLines(result[idx].edits[0].NewText, pending.edits[0].NewText)
				if !ok {
					conflicting = append(conflicting, pending)
					continue
				}
				// The merged fix fixes both diagnostics. A changed edit is
				// replaced, not changed in place: it belongs to a diagnostic.
				result[idx].diagnostics += pending.diagnostics
				if text != result[idx].edits[0].NewText {
					result[idx].edits = []fix.TextEdit{{StartOffset: offset, EndOffset: offset, NewText: text}}
				}
				continue
			}
			boundaries[key] = len(result)
		}
		result = append(result, pending)
	}
	return result, conflicting
}

// blankLineBoundary is the offset at which a rule inserts blank lines.
type blankLineBoundary struct {
	ruleID string
	offset int
}

// blankLines is a blank-line insertion: count lines, each the container
// prefix followed by the line ending.
type blankLines struct {
	prefix string
	ending string
	count  int
}

// parseBlankLines splits a blank-line insertion into its lines. It reports
// false when the lines do not share one prefix and one line ending.
func parseBlankLines(text string) (blankLines, bool) {
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1] // text ends with "\n", so the last part is empty

	var parsed blankLines
	for idx, line := range lines {
		body := strings.TrimSuffix(line, "\n")
		ending := "\n"
		if trimmed, ok := strings.CutSuffix(body, "\r"); ok {
			body, ending = trimmed, "\r\n"
		}
		if idx == 0 {
			parsed.prefix, parsed.ending = body, ending
			continue
		}
		if body != parsed.prefix || ending != parsed.ending {
			return blankLines{}, false
		}
	}
	parsed.count = len(lines)
	return parsed, true
}

// mergeBlankLines combines two blank-line insertions at one boundary into
// one that satisfies both, without losing container markers:
//   - for the same container (prefixes equal apart from trailing spaces),
//     the insertion with more lines is kept as it is;
//   - a bare insertion (no '>' markers) adopts the other's container
//     prefix, with the larger line count, so ">\n" and "\n\n" give ">\n>\n";
//   - two different containers cannot both be right, so it reports false.
func mergeBlankLines(kept, next string) (string, bool) {
	first, okFirst := parseBlankLines(kept)
	second, okSecond := parseBlankLines(next)
	if !okFirst || !okSecond {
		return "", false
	}

	firstMarkers := strings.TrimRight(first.prefix, " \t")
	secondMarkers := strings.TrimRight(second.prefix, " \t")
	switch {
	case firstMarkers == secondMarkers:
		if second.count > first.count {
			return next, true
		}
		return kept, true
	case firstMarkers == "":
		return strings.Repeat(second.prefix+second.ending, max(first.count, second.count)), true
	case secondMarkers == "":
		return strings.Repeat(first.prefix+first.ending, max(first.count, second.count)), true
	default:
		return "", false
	}
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
