---
name: golden-test-verification
description: Use when verifying golden test output files after generation. Ensures fix correctness, diagnostic accuracy, round-trip idempotency, and no accidental side effects. Must be invoked after every batch of golden file generation.
---

# Golden Test Verification

## Overview

Verify that generated golden test files are correct. This is the critical quality gate — the golden files ARE the source of truth for fix behavior, so they must be manually verified before being committed.

## Verification Checklist

For each newly generated test case, verify all 4 files:

### 1. Verify `.golden.md` (Fix Output)

**For fixable rules with violations:**
- [ ] The fix addresses the violation correctly (e.g., trailing spaces removed, blank lines collapsed)
- [ ] The fix does NOT modify content outside the violation (no collateral damage)
- [ ] The fix preserves document structure (headings, lists, code blocks intact)
- [ ] Multi-byte characters are not corrupted
- [ ] Line endings are preserved (LF stays LF, CRLF stays CRLF)
- [ ] Indentation is preserved where it should be

**For `clean` cases:**
- [ ] `.golden.md` is byte-identical to `.input.md`

**For `code_block_immunity` cases:**
- [ ] `.golden.md` is byte-identical to `.input.md`

**For non-fixable rules:**
- [ ] `.golden.md` is byte-identical to `.input.md` (no fixes should be applied)

### 2. Verify `.diags.json` (Diagnostics)

- [ ] Each diagnostic has the correct `rule` ID matching the directory name
- [ ] Each diagnostic has the correct `line` number (1-indexed)
- [ ] Each diagnostic has the correct `column` number (1-indexed)
- [ ] The `message` accurately describes the violation
- [ ] The `severity` is correct (usually `"warning"`)
- [ ] The `fixable` field matches the rule's `CanFix()` return value
- [ ] For `clean` cases: array is `[]`
- [ ] For `code_block_immunity` cases: array is `[]`
- [ ] Diagnostic count matches the number of violations in the input

### 3. Verify `.diags.txt` (Human-Readable)

- [ ] Each line follows format: `<filename>:<line>:<col> <severity> <message> (<rule-name>) [fixable]`
- [ ] Content is consistent with `.diags.json`

### 4. Run Round-Trip Test

```bash
go test ./pkg/lint/rules/... -run "TestGoldenRoundTrip/<RULE_ID>/<scenario>"
```

- [ ] Test passes (zero fixable diagnostics remain after applying fixes)
- [ ] No warnings about skipped or merged edits (unless testing overlapping edit scenarios)

## Batch Verification Commands

After generating golden files for a rule:

```bash
# Run all golden tests for a specific rule
go test -v ./pkg/lint/rules/... -run "TestGoldenPerRule/<RULE_ID>"

# Run round-trip tests for a specific rule
go test -v ./pkg/lint/rules/... -run "TestGoldenRoundTrip/<RULE_ID>"

# Run ALL golden tests (after completing a batch)
go test -v ./pkg/lint/rules/... -run "TestGolden"
```

## Red Flags — Investigate Before Committing

| Red Flag | Possible Cause |
|----------|----------------|
| `.golden.md` differs from `.input.md` for a `clean` case | Input accidentally contains violations |
| `.diags.json` is non-empty for `code_block_immunity` | Rule not properly checking `IsLineInCodeBlock()` |
| Round-trip test fails | Fix introduces new violations or is non-idempotent |
| Diagnostic line/column seems off by one | Rule's position calculation may have a bug |
| Fix corrupts content after multi-byte characters | Rule's byte-offset calculation doesn't handle UTF-8 |
| CRLF test produces mixed line endings | Fix replaces `\r\n` with `\n` in some places |
| `.golden.md` has unexpected changes far from violation | Fix has incorrect byte range (too wide) |
| Multiple violations but only some are fixed | Edit conflict resolution is dropping valid edits |
| `diags.json` has `"name": ""` | Rule implementation may not set the name field — check if this is the existing convention |

## When a Golden File Looks Wrong

If the generated golden output is incorrect, the problem is in the **rule implementation**, not the test infrastructure. Do NOT manually edit golden files to make tests pass.

Instead:
1. Document the issue (what's wrong and why)
2. File a bug against the rule
3. Skip that specific test case for now with a comment in the input file
4. Fix the rule first, then regenerate the golden file

## Verification Order

When verifying a batch of new test cases:

1. **`clean` cases first** — fastest to verify (golden = input, diags = [])
2. **`code_block_immunity` cases** — same check (golden = input, diags = [])
3. **`basic` cases** — core violation/fix behavior
4. **Edge cases** — each one individually
5. **Round-trip for all** — single batch run

## Final Gate

Before committing new golden test files:

```bash
# Full test suite must pass
go test ./pkg/lint/rules/... -run "TestGolden"

# Verify no unintended files were modified
git diff --name-only
```

All new files should be under `pkg/lint/rules/testdata/<RULE_ID>/` only.
