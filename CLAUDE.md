# gomdlint Project Instructions

## Project Overview

gomdlint is a high-performance, self-fixing Markdown linter written in Go. It implements 55 lint rules (37 auto-fixable) with multi-pass fix application, conflict detection, and merge.

## Project Structure

```
cmd/gomdlint/          # CLI entry point
internal/cli/           # CLI command implementation
internal/configloader/  # Config file discovery and loading
internal/ui/pretty/     # Terminal output formatting
pkg/lint/               # Core lint engine, pipeline, registry
pkg/lint/rules/         # All 55 rule implementations
pkg/lint/rules/testdata/ # Golden test fixtures
pkg/fix/                # Edit model, validation, application, diff
pkg/runner/             # Multi-file parallel processing
pkg/reporter/           # Output formatters (text, table, JSON, SARIF, diff, summary)
pkg/parser/goldmark/    # Markdown parser (goldmark-based)
pkg/mdast/              # AST node types and walking
pkg/config/             # Configuration model
pkg/fsutil/             # File system utilities (atomic write, backup)
pkg/langdetect/         # Code language detection for MD040
pkg/analysis/           # Result analysis and statistics
```

## Golden Test Conventions

Golden tests are the primary regression safety net for autofix correctness.

### File Layout
```
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.input.md   # Hand-crafted input
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.golden.md   # Generated expected output
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.diags.json  # Generated expected diagnostics
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.diags.txt   # Generated expected diagnostics (text)
```

### Generating Golden Files
```bash
go test -update ./pkg/lint/rules/... -run TestGoldenPerRule/<RULE_ID>
```

### Running Golden Tests
```bash
go test ./pkg/lint/rules/... -run TestGolden
```

### Key Principle
Only create `.input.md` files by hand. The other 3 files are generated with `-update` and then verified for correctness. Never manually edit `.golden.md`, `.diags.json`, or `.diags.txt`.

## Available Project Skills

| Skill | When to Use |
|-------|-------------|
| `golden-test-gap-analysis` | Before creating golden tests — analyze which rules need coverage |
| `golden-test-authoring` | When creating golden test input files — enforces conventions |
| `golden-test-verification` | After generating golden files — verify correctness |

## Common Gotchas When Writing Test Input Files

- Always end files with a trailing newline (unless testing MD047)
- Use spaces not tabs (unless testing MD010)
- No trailing whitespace (unless testing MD009)
- Use single blank lines (unless testing MD012)
- Start with `# Heading` (unless testing MD041)
- Use ATX heading style consistently (unless testing MD003)
- Use proper heading hierarchy (unless testing MD001)
