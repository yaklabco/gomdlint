# gomdlint Project Instructions

## Project Overview

gomdlint is a high-performance, self-fixing Markdown linter written in Go.
It implements 55 lint rules (37 auto-fixable) with multi-pass fix
application, a safety check on every fix, conflict detection, and merge.
Every rule runs by default, with markdownlint 0.41's option defaults and the
`gfm` flavor.

## Project Structure

```text
cmd/gomdlint/            # CLI entry point
internal/cli/            # CLI command implementation and exit codes
internal/configloader/   # Config file discovery, loading and migration
internal/display/        # Character columns and display paths for reporters
internal/editdistance/   # Edit distance for rule name suggestions
internal/ignore/         # gitignore matching for directory walks
internal/logging/        # Structured logging
internal/ui/pretty/      # Terminal output formatting
pkg/lint/                # Core lint engine, pipeline, registry, fix scheduling
pkg/lint/mdscan/         # Scanners for inline HTML and autolink literals
pkg/lint/refs/           # Reference definitions, usages and heading anchors
pkg/lint/rules/          # All 55 rule implementations
pkg/lint/rules/catalog*.go # Rule catalog: IDs, names, aliases, tags, categories
pkg/lint/rules/fix*.go   # Fix checks (fixcheck.go, fix_check.go, fix_safety.go) and region checks
pkg/lint/rules/testdata/ # Golden test fixtures
pkg/fix/                 # Edit model, validation, fix resolution, diff
pkg/runner/              # Multi-file parallel processing
pkg/reporter/            # Output formatters (text, table, JSON, SARIF, diff, summary)
pkg/parser/goldmark/     # Markdown parser (goldmark-based) with exact source spans
pkg/mdast/               # AST node types, span contract and walking
pkg/config/              # Configuration model
pkg/fsutil/              # File system utilities (pinned writes, backup)
pkg/langdetect/          # Code language detection for MD040
pkg/analysis/            # Result analysis and statistics
```

## Golden Test Conventions

Golden tests are the primary regression safety net for autofix correctness.

### File Layout

```text
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.input.md    # Hand-crafted input
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.config.yml  # Optional hand-written sidecar config
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.golden.md   # Generated expected output
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.diags.json  # Generated expected diagnostics
pkg/lint/rules/testdata/<RULE_ID>/<scenario>.diags.txt   # Generated expected diagnostics (text)
```

`testdata/real-world/` uses the same layout and runs every rule.

### Sidecar Configs

A scenario that needs a flavor or rule options gets a hand-written
`<scenario>.config.yml` next to its input. Without one, the scenario runs
with `config.NewConfig()`: the `gfm` flavor and no rule options. A sidecar is
a strict subset of the `.gomdlint.yml` schema and may hold only:

- `flavor`: `commonmark` or `gfm`
- `rules.<RULE_ID>.options`: options for one rule, keyed by the rule ID

```yaml
rules:
  MD043:
    options:
      headings:
        - "# Project"
        - "## Usage"
```

The harness (`pkg/lint/rules/golden_config_test.go`) fails the scenario
instead of ignoring part of a sidecar. It rejects unknown keys (a misspelled
`flavour`, a per-rule `severity` or `enabled`), an invalid flavor, an unknown
rule, a rule name used instead of its ID, a rule entry without options, an
empty sidecar, a sidecar with more than one YAML document, and options for a
rule the scenario does not run. A per-rule scenario runs only its
directory's rule. `TestGoldenTestdataFiles` also fails for a `.config.yaml`
suffix or a sidecar without a matching `.input.md`. Option names are not
checked, so confirm that each option changes the generated output.

### How Scenarios Run

Each scenario is linted through `lint.Engine.LintFile` with only its rules
enabled. Other rules are inert stand-ins with their real IDs, names, tags
and catalog entries, so inline directives resolve as in the product. A
per-rule scenario's `golden.md` applies one engine pass, which holds every
fix of its one rule. Real-world scenarios and `TestGoldenRoundTrip` fix
through `lint.Pipeline`, pass by pass as `--fix` does, and round trips also
require the passes to converge.

### Generating Golden Files

```bash
go test ./pkg/lint/rules/ -run 'TestGoldenPerRule/<RULE_ID>' -update
go test ./pkg/lint/rules/ -run TestGoldenRealWorld -update
go test ./internal/cli/ -run Golden -update   # CLI output goldens
```

Put `-update` after the package path. `go test` stops reading package paths
at the first flag it does not know, so `go test -update ./pkg/lint/rules/...`
tests the repository root instead.

### Running Golden Tests

```bash
go test ./pkg/lint/rules/ -run TestGolden
```

### Key Principle

Only create `.input.md` files, and a `.config.yml` sidecar when a scenario
needs one, by hand. The other 3 files are generated with `-update` and then
verified for correctness. Never manually edit `.golden.md`, `.diags.json`,
or `.diags.txt`.

## Available Project Skills

| Skill | When to Use |
| --- | --- |
| `golden-test-gap-analysis` | Before creating golden tests, to analyze which rules need coverage |
| `golden-test-authoring` | When creating golden test input files. Enforces conventions and detects bugs |
| `golden-test-verification` | After generating golden files, to verify correctness and check bug reports |
| `golden-test-bug-fixing` | When fixing rule bugs discovered during golden test creation |

## Bug Report Convention

When golden test agents discover rule implementation bugs:

- Reports are stored in `.claude/bug-reports/<RULE_ID>-<slug>.md`
- Reports follow a structured format with YAML frontmatter (rule_id, status,
  severity, affected_files)
- Status lifecycle: `open` -> `fixing` -> `fixed` -> `verified`
- Blocking bugs prevent golden file generation for the affected rule
- Bug fixes must be completed before golden files can be baselined
- The authoring skill's Bug Detection Protocol defines when and how to file
  reports

## Common Gotchas When Writing Test Input Files

Every rule runs by default, MD041 included, so the real-world fixtures and
the CLI report any of these. A per-rule fixture runs only its own rule, but
keep inputs clean so they test only what they mean to test.

- Always end files with a trailing newline (unless testing MD047)
- Use spaces not tabs (unless testing MD010)
- No trailing whitespace (unless testing MD009)
- Use single blank lines (unless testing MD012)
- Start with `# Heading` (unless testing MD041)
- Use ATX heading style consistently (unless testing MD003)
- Use proper heading hierarchy (unless testing MD001)
- Keep lines to 80 characters, the markdownlint default (unless testing
  MD013)
- Give fenced code blocks a language (unless testing MD040)
- Avoid raw HTML (unless testing MD033)
- Avoid link text such as "click here", "here", "link" or "more" (unless
  testing MD059)
- Format each table in one style: aligned, compact or tight (unless testing
  MD060)
- Don't mix relative and absolute link destinations (unless testing MDL001)
