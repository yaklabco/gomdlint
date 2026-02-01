---
rule_id: MD035
status: fixed
severity: blocking
discovered_by: golden-test-authoring
discovered_during: basic scenario
affected_files:
  - pkg/lint/rules/hr.go
related_test_files:
  - pkg/lint/rules/testdata/MD035/basic.input.md
---

# MD035: ThematicBreak nodes have no source position

## Symptom

Running MD035 against a file with mixed HR styles (`---` and `***`) produces zero diagnostics.
The generated `basic.diags.json` is `[]`. The golden output is identical to the input — no fixes applied.

## Root Cause

Goldmark's `ast.ThematicBreak` node has `Lines.Len() == 0`. The mapper at
`pkg/parser/goldmark/mapper.go` creates `NodeThematicBreak` nodes, but `getNodeByteRange()`
returns `(-1, -1)` because there are no lines to extract a byte range from. This means
`assignTokenRanges` never sets `FirstToken`/`LastToken`, so `SourcePosition()` returns an
invalid zero struct.

In `hr.go:58-61`, the rule checks `pos.IsValid()` and skips every thematic break node.

## Evidence

```
$ go test -v ./pkg/lint/rules/... -run "TestHRStyleRule_Fix/single_violation"
=== RUN   TestHRStyleRule_Fix/single_violation_-_consistent_mode_(parser_limitation_-_no_positions)
--- PASS: (wantDiags: 0, got: 0 — passes because it expects the broken behavior)
```

The unit test at `hr_test.go:97-100` explicitly documents this with `wantDiags: 0 // Should be 1 when parser is fixed`.

## Proposed Fix

Use the token stream instead of AST node positions. `TokThematicBreak` tokens are correctly
emitted by the tokenizer with accurate byte offsets. Iterate `ctx.File.Tokens`, filter for
`TokThematicBreak`, use `ctx.File.LineAt(tok.StartOffset)` to convert to line/column, and
construct diagnostics with `lint.NewDiagnosticAt()`.

## Resolution

Rewrote `pkg/lint/rules/hr.go` to iterate `ctx.File.Tokens` instead of using AST node positions.
For each `TokThematicBreak` token, the rule now:
- Extracts the HR text via `tok.Text(ctx.File.Content)` and trims whitespace
- Converts byte offset to line/col via `ctx.File.LineAt(tok.StartOffset)`
- Skips lines in code blocks via `ctx.IsLineInCodeBlock(line)`
- Constructs diagnostics with explicit `mdast.SourcePosition` via `lint.NewDiagnosticAt()`
- Builds fix edits using line info from `ctx.File.Lines[line-1]`

Updated `pkg/lint/rules/hr_test.go` to remove all "parser limitation" workarounds and
set correct expected diagnostic counts and fix outputs. Regenerated golden files for MD035
which now correctly show diagnostics and fix output.
