---
name: golden-test-bug-fixing
description: Use when fixing rule implementation bugs discovered during golden test creation. Guides investigation, fix implementation, test updates, and verification. Must be invoked before regenerating golden files for a rule with an open bug report.
---

# Golden Test Bug Fixing

## Overview

Fix production code bugs in gomdlint rule implementations that were discovered during golden test creation. These bugs cause rules to produce incorrect results — zero diagnostics for violations, wrong positions, infinite fix loops.

The golden test process should never baseline broken behavior. If a rule is broken, fix the rule first, then generate golden files that capture correct behavior.

## Prerequisites

Before starting a fix:

1. Read the bug report at `.claude/bug-reports/<RULE_ID>-<slug>.md`
2. Update bug report status to `fixing`
3. Read the rule implementation source
4. Read the relevant parser/AST code if the bug involves source positions

## Investigation Protocol

### Step 1: Reproduce the Bug

Create or update a Go test that demonstrates the failure:

```go
func TestBugRepro_<RULE_ID>(t *testing.T) {
    parser := goldmark.New(string(config.FlavorCommonMark))
    snapshot, err := parser.Parse(context.Background(), "test.md", []byte(input))
    require.NoError(t, err)

    rule := New<RuleName>Rule()
    cfg := config.NewConfig()
    ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
    diags, err := rule.Apply(ruleCtx)
    require.NoError(t, err)

    // This assertion MUST FAIL before the fix:
    assert.Greater(t, len(diags), 0, "rule should detect violations")
}
```

Run the test. Confirm it fails. This is the regression test.

### Step 2: Trace the Root Cause

Depending on the bug category:

**Source position bugs** (e.g., AST nodes with invalid positions):

1. Check if `node.SourcePosition().IsValid()` returns false
2. If so, check whether the token stream has tokens of the relevant `TokenKind`
3. Tokens are always populated by the tokenizer regardless of goldmark's AST
4. Use `ctx.File.Tokens` + `ctx.File.LineAt(tok.StartOffset)` to bypass AST positions

**Style detection bugs** (e.g., position points to wrong character):

1. Check what `SourcePosition()` actually returns
2. Verify whether the position points to the marker or the content
3. Scan backward or forward from the reported position to find the actual marker
4. Check if relevant marker tokens (e.g., `TokEmphasisMarker`, `TokThematicBreak`) are available in the token stream

**Fix idempotency bugs** (e.g., regex re-matches after fix):

1. Apply the fix once, capture the output
2. Re-run the rule on the fixed output
3. Check if the regex/detection still matches on already-fixed content
4. Trace through the skip logic to find where it fails

### Step 3: Implement the Fix

**Rules:**

- Fix the rule implementation, not the test harness
- Prefer using the token stream over AST positions when AST positions are unreliable
- Maintain the rule's existing API contract (same rule ID, same diagnostic messages)
- Preserve existing `CanFix()` behavior unless the fix fundamentally changes it

**Common fix patterns:**

Token-stream bypass (for missing AST positions):

```go
for _, tok := range ctx.File.Tokens {
    if tok.Kind == mdast.TokXxx { // Use the appropriate TokenKind constant
        line, col := ctx.File.LineAt(tok.StartOffset)
        // ... use line/col directly instead of node.SourcePosition()
    }
}
```

Backward/forward scan (for incorrect position offsets):

```go
// When SourcePosition points to content rather than syntax markers,
// scan from the reported position to find the actual syntax character.
idx := pos.StartColumn - 2 // look before the reported content start
if idx >= 0 && isSyntaxMarker(lineContent[idx]) {
    // found the marker
}
```

### Step 4: Update Unit Tests

1. Update `wantDiags` values that were set to 0 due to the bug
2. Update `wantFix` values to reflect the actual corrected output
3. Remove "parser limitation" comments
4. Ensure idempotency checks are enabled (not skipped or commented out)

### Step 5: Regenerate Golden Files

```bash
# Delete old broken golden files (keep .input.md)
rm pkg/lint/rules/testdata/<RULE_ID>/*.golden.md
rm pkg/lint/rules/testdata/<RULE_ID>/*.diags.json
rm pkg/lint/rules/testdata/<RULE_ID>/*.diags.txt

# Regenerate from fixed rule
go test -update ./pkg/lint/rules/... -run TestGoldenPerRule/<RULE_ID>
```

### Step 6: Verify

1. Read each regenerated `.diags.json` — confirm it is no longer `[]` for violation scenarios
2. Read each `.golden.md` — confirm the fix is correct
3. Run round-trip test:
   ```bash
   go test ./pkg/lint/rules/... -run "TestGoldenRoundTrip/<RULE_ID>"
   ```
4. Run full golden suite:
   ```bash
   go test ./pkg/lint/rules/... -run "TestGolden"
   ```
5. Run the full rule test suite:
   ```bash
   go test ./pkg/lint/rules/...
   ```

### Step 7: Update Bug Report

Update `.claude/bug-reports/<RULE_ID>-<slug>.md`:

- Set `status: fixed`
- Add a `## Resolution` section describing what was changed and why
- List all files modified

## Verification Criteria

A bug fix is complete when:

- [ ] The reproduction test now passes (detects violations correctly)
- [ ] All unit tests pass with updated expected values
- [ ] Golden files regenerated with correct, non-empty diagnostics
- [ ] Round-trip test passes (fixes are idempotent)
- [ ] Full `go test ./pkg/lint/rules/...` passes
- [ ] Bug report updated to `status: fixed`

## Orchestration Notes

Bug fixes are batched into a dedicated phase between test authoring and golden file generation. The parent agent:

1. Collects bug reports from authoring agents
2. Groups bugs by shared root cause (e.g., two rules that share the same position lookup bug)
3. Dispatches one fixer agent per bug cluster
4. After fixes, the authoring agents re-run `-update` on affected rules
5. The verification agent independently confirms the fix

Fixer agents do initial self-verification (Step 6), but the verification agent does independent confirmation because the fixer may have confirmation bias.
