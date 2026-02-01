---
rule_id: MD049
status: fixed
severity: blocking
discovered_by: golden-test-authoring
discovered_during: basic scenario
affected_files:
  - pkg/lint/rules/emphasis.go
also_affects:
  - MD050
related_test_files:
  - pkg/lint/rules/testdata/MD049/basic.input.md
  - pkg/lint/rules/testdata/MD050/basic.input.md
---

# MD049/MD050: SourcePosition points to emphasis content, not marker

## Symptom

Running MD049 against a file with mixed emphasis styles (`*text*` and `_text_`) produces zero
diagnostics. Same for MD050 with mixed strong styles (`**text**` and `__text__`). Both
`basic.diags.json` files are `[]`.

## Root Cause

`detectEmphasisStyle()` at `emphasis.go:343` reads `lineContent[pos.StartColumn-1]` expecting
to find the marker character (`*` or `_`). However, `SourcePosition()` for emphasis/strong nodes
points to the **content** inside the markers, not the markers themselves.

For `*italic*`, `pos.StartColumn` points to the `i` in "italic", not the `*`. So
`lineContent[pos.StartColumn-1]` reads the character before `i`, which is `*` only by accident
if the content starts at the right position — but in practice it reads the wrong character and
returns `""`.

The same issue affects `detectStrongStyle()` at `emphasis.go:443`.

## Evidence

```
$ go test -v ./pkg/lint/rules/... -run "TestEmphasisStyleRule_Fix/style_mismatch"
=== RUN   TestEmphasisStyleRule_Fix/style_mismatch_-_consistent_mode_(parser_limitation_-_no_detection)
--- PASS: (wantDiags: 0, got: 0 — passes because it expects the broken behavior)
```

The unit test at `emphasis_test.go:367-370` documents: `wantDiags: 0 // Should be 1 when parser provides marker positions`.

## Proposed Fix

Scan backward from `pos.StartColumn-1` to find the marker character. For emphasis (MD049),
look one position back. For strong (MD050), look two positions back for consecutive `**` or `__`.

Note: `buildStyleFix()` currently returns `nil` for both rules. Detection fix is the priority;
autofix implementation is a follow-up.

## Resolution

Fixed `detectEmphasisStyle()` and `detectStrongStyle()` in `pkg/lint/rules/emphasis.go` to
correctly account for `SourcePosition()` pointing to the emphasis content rather than the marker.

- `detectEmphasisStyle()`: Changed from `lineContent[pos.StartColumn-1]` to
  `lineContent[pos.StartColumn-2]` (one position back from content to reach the single marker).
- `detectStrongStyle()`: Changed to check `lineContent[pos.StartColumn-2]` and
  `lineContent[pos.StartColumn-3]` for the two consecutive marker characters.
- Updated boundary checks accordingly (`< 2` for emphasis, `< 3` for strong).
- Updated unit tests to expect correct diagnostic counts (1 instead of 0 for mismatch cases).
- Removed outdated "parser limitation" comments from tests.
- Regenerated golden files: `basic.diags.json` for both MD049 and MD050 now correctly report
  style mismatches instead of empty arrays.

Note: `buildStyleFix()` still returns nil for both rules -- autofix is a separate follow-up task.
Diagnostics are correctly emitted with `fixable: false`.
