# gomdlint

A blisteringly fast, self-fixing Markdown linter written in Go.

Inspired by [markdownlint](https://github.com/DavidAnson/markdownlint) by David Anson, which has set the standard for markdown linting. Without this epic work, this project would not have been possible.

## Why gomdlint?

|                    | gomdlint | markdownlint |
|--------------------|----------|--------------|
| **Performance**    | **507x faster** average | baseline |
| **Auto-fixable**   | 67% (37 of 55 rules) | Limited      |
| **Language**       | Go       | Node.js      |

gomdlint is **dramatically faster** than markdownlint. Benchmarks on real-world repositories show an average **507x speedup**, with improvements ranging from 72x to over 1100x depending on repository size. 37 of 55 rules support automatic fixing. Built in Go with no runtime dependencies.

See [bench/PERFORMANCE.md](bench/PERFORMANCE.md) for detailed benchmark methodology and results.

## Features

- **40+ lint rules** covering headings, lists, whitespace, code blocks, links, emphasis, blockquotes, and tables
- **Automatic fixing** for most issues with safe conflict detection
- **Multiple output formats** including text, table, JSON, SARIF, diff, and summary
- **CommonMark and GFM support** with flavor-specific rules
- **Flexible configuration** via YAML files with hierarchical discovery
- **Fast parallel processing** with deterministic ordering
- **Human-readable rule names** with optional traditional ID format

## Installation

### Homebrew (macOS/Linux)

```bash
brew install yaklabco/tap/gomdlint
```

### Go

```bash
go install github.com/yaklabco/gomdlint/cmd/gomdlint@latest
```

### Binary releases

Download from [GitHub Releases](https://github.com/yaklabco/gomdlint/releases).

## Quick Start

```bash
gomdlint lint                    # Lint current directory
gomdlint lint docs/              # Lint specific directory
gomdlint lint README.md          # Lint single file
gomdlint lint --fix              # Lint and auto-fix issues
gomdlint lint --fix --dry-run    # Preview fixes without applying
```

## Linting and Fixing

Lint Markdown files with over 40 rules. Run `gomdlint lint` on files or directories to detect issues, with parallel processing for large codebases.

Automatically fix most issues with `--fix`. The autofix system handles conflicting edits safely, supports multi-pass fixing for cascading issues, and creates backups by default. Preview changes before applying them with `--dry-run`, which shows a unified diff (when invoked with `--format diff`) of proposed fixes.

Limit auto-fixing to specific rules with `--fix-rules` when you want targeted corrections. Disable backups with `--no-backups` if your files are under version control.

## Rule Categories

**Headings** - Enforce heading level increments (no jumping from H1 to H3), consistent style (ATX or setext), proper spacing, unique heading text, single H1 per document, and no trailing punctuation. Most heading issues auto-fix.

**Lists** - Ensure consistent bullet markers, proper indentation at each nesting level, correct ordered list numbering, and appropriate spacing. List formatting issues auto-fix to your configured style.

**Whitespace** - Remove trailing spaces, convert tabs to spaces, collapse multiple blank lines, and ensure files end with a single newline. All whitespace issues auto-fix.

**Code Blocks** - Require language identifiers on fenced code blocks (with auto-detection for 10 languages including Go, Python, JavaScript, and Bash), enforce consistent fence style, and ensure proper blank lines around blocks. Missing language identifiers auto-fix based on content analysis.

**Links** - Detect reversed link syntax, bare URLs, empty links, invalid reference links, and missing image alt text. Reversed links and bare URLs auto-fix.

**Emphasis** - Detect bold text used as headings (converts to proper headings with intelligent level inference), spaces inside emphasis markers, and inconsistent emphasis/strong style. All emphasis issues auto-fix.

**Line Length** - Enforce maximum line length with intelligent word-wrapping that preserves list indentation, blockquote prefixes, and code blocks.

**Tables** (GFM) - Validate table structure including consistent column counts, pipe alignment, and surrounding blank lines.

## Output Formats

Choose the output format that fits your workflow with `--format`:

```bash
gomdlint lint file.md                      # text (default) - human-readable with colors
gomdlint lint --format table file.md       # table - columnar output
gomdlint lint --format summary file.md     # summary - aggregated statistics
gomdlint lint --format json file.md        # json - machine-readable
gomdlint lint --format sarif file.md       # sarif - GitHub code scanning
gomdlint lint --format diff --fix file.md  # diff - unified diff of fixes
```

The summary format shows aggregated statistics:

```text
Rules Summary
──────────────────────────────────────────────────────────────────────
Rule                 Count  Errors Warnings  Fixable
──────────────────────────────────────────────────────────────────────
no-trailing-spaces       5       0        5        ✓
no-hard-tabs             2       0        2        ✓

Files Summary
──────────────────────────────────────────────────────────────────────
File                                  Count  Errors Warnings
──────────────────────────────────────────────────────────────────────
docs/guide.md                             4       0        4
README.md                                 3       0        3

Total: 7 issues (7 warnings) in 2 files
```

Control table order with `--summary-order rules` (default) or `--summary-order files`.

Every format lists findings file by file, in path order. Within a file, as
in markdownlint-cli2, findings follow line order, then column, rule ID and
message.

### Color

Text, table, summary and diff output, and help, use color on a terminal.
`--color` controls it:

- `auto` (default): color only when output goes to a terminal and
  `NO_COLOR` is not set.
- `always`: color even when output goes to a pipe or a file, and even when
  `NO_COLOR` is set, because the flag asks for color explicitly. The colors
  are the 16 basic ANSI colors, which every color terminal and `less -R`
  show.
- `never`: no color.

JSON and SARIF output never contain color. Table rows start with a severity
marker, `E`, `W` or `I`, which the legend under the table explains, so a
table reads the same without color.

## Rule Identifier Format

Text, table and summary output name each rule by its name. Use
`--rule-format` to show its ID, or both:

```bash
gomdlint lint file.md                        # name (default): no-trailing-spaces
gomdlint lint --rule-format id file.md       # id: MD009
gomdlint lint --rule-format combined file.md # combined: MD009/no-trailing-spaces
```

To change the default for `gomdlint lint`, set `GOMDLINT_RULE_FORMAT`, for
example to `id`. The flag wins over the variable. JSON and SARIF output
always carry both the ID and the name.

## Configuration

Configure gomdlint with a YAML or JSON file. The tool searches project,
user, and system directories, merging configurations hierarchically.

For the project config, gomdlint looks in the current directory and then in
each parent directory, up to the repository root or your home directory. It
uses the first directory that holds one of these files. In that directory it
reads the first name in this list and ignores the others:

1. `.gomdlint.yml`
2. `.gomdlint.yaml`
3. `gomdlint.yml`
4. `gomdlint.yaml`
5. `.gomdlint.json`
6. `gomdlint.json`

`gomdlint init` writes `.gomdlint.yml`, or `.gomdlint.json` with
`--format json`. A file whose name ends in `.json` must hold JSON, so
`init --output` and `migrate --output` write JSON for such a name.

The user config is `config.yaml`, `config.yml` or `config.json`, in that
order of preference, in `$XDG_CONFIG_HOME/gomdlint` (by default
`~/.config/gomdlint`). The system config uses the same names in
`/etc/gomdlint`, or `%ProgramData%\gomdlint` on Windows.

```yaml
# .gomdlint.yaml
flavor: gfm

rules:
  # Using human-readable name
  no-trailing-spaces:
    enabled: true
    severity: warning
    auto_fix: true

  # Using traditional ID (also works)
  MD013:
    enabled: true
    severity: warning
    options:
      max: 120
      ignore_code_blocks: true

ignore:
  - "vendor/**"
  - "node_modules/**"
```

Generate a starter configuration with `gomdlint init` or a comprehensive template with `gomdlint init --full`.

Migrate existing markdownlint configurations with `gomdlint migrate`.

Migration writes markdownlint's rule defaults and translated options into
`rule_defaults`, with source overrides under `rules`. It keeps gomdlint-only rules
disabled. Native configurations retain gomdlint's defaults. For example, migrated
MD025 checks document titles, while native MD025 checks all H1 headings.

Markdownlint HTML comments support `disable`, `enable`, `disable-line`,
`disable-next-line`, `disable-file`, `enable-file`, `capture`, and `restore`, with
rule IDs, aliases, or tags. Suppressed findings produce no fixes. Comments inside
code or frontmatter do not change lint settings. As in markdownlint, whitespace
or `-->` must follow the comment's kind, so a comment such as
`<!-- markdownlint-disable-next-lines MD009 -->` is plain text and does nothing.
Unlike markdownlint, gomdlint also reads a comment split over several lines,
and it reads every comment after one that lacks its closing `-->`, where
markdownlint can skip some.

A word in such a comment that names no rule or tag does nothing, as in
markdownlint. gomdlint warns about it, and names the rules and tags that the
word most likely means when some are close. gomdlint's former tags `style`,
`lists`, `layout`, `metadata`, `gfm` and `tables` still work, but markdownlint
does not know them. Each use gets a warning that names the rules the tag
covers, and the tags will stop working in a later release. These warnings name
rules as markdownlint does, such as `ul-style` for `unordered-list-style`, so
that the comment works in markdownlint too. The warnings go to stderr in every
output format. JSON output also lists them under each file's `notices`, and
SARIF output as `toolConfigurationNotifications`. They never change findings
or the exit code, even with `--strict`.

`markdownlint-configure-file` accepts a JSON object of markdownlint settings and
applies it to the whole file. Invalid JSON and unknown rule keys fail that file.
Migration does not evaluate JavaScript configuration or resolve `extends`.

Override configuration via command line (`--enable`, `--disable`) or environment variables (`GOMDLINT_*`).

## Markdown Support

Supports both CommonMark and GitHub Flavored Markdown (GFM) via `--flavor`. GFM mode enables table rules and handles GFM-specific syntax like task lists and strikethrough.

## CI Integration

Use `--strict` to treat warnings as errors for CI pipelines. JSON and SARIF output formats integrate with analysis tools and GitHub's code scanning. Exit codes indicate whether issues were found.

```yaml
# GitHub Actions example
- name: Lint Markdown
  run: gomdlint lint --strict --format sarif > results.sarif

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

## Commands

| Command | Description |
|---------|-------------|
| `gomdlint lint [paths...]` | Lint files and directories |
| `gomdlint rules` | List all available rules |
| `gomdlint init` | Generate configuration file |
| `gomdlint migrate` | Convert markdownlint config |
| `gomdlint version` | Show version information |

## Development

This project uses [stave](https://github.com/yaklabco/stave) for build automation.

### Prerequisites

- Go 1.25+
- [golangci-lint](https://golangci-lint.run/usage/install/)
- [stave](https://github.com/yaklabco/stave) - `go install github.com/yaklabco/stave@latest`

### Build Tasks

```bash
stave build      # Build the project
stave test       # Run tests
stave lint       # Run linters
stave cigate     # Run CI gate checks (fmt, vet, lint, build)
stave check      # Run all checks (format, lint, test)
stave install    # Install to GOPATH/bin
stave -l         # List all available tasks
```

### Project Structure

```text
gomdlint/
  cmd/gomdlint/        # CLI entry point
  internal/
    cli/               # Cobra commands and flag handling
    configloader/      # Viper + XDG config resolution
    ui/pretty/         # Lipgloss-based styled output
    logging/           # Structured logging setup
  pkg/
    mdast/             # AST types, FileSnapshot, Parser interface
    lint/              # Rule interfaces, registry, engine
    lint/rules/        # 40+ built-in rules
    fix/               # TextEdit, EditBuilder, conflict detection
    config/            # Core config types
    parser/goldmark/   # Goldmark-based parser implementation
    runner/            # Multi-file runner with concurrency
    reporter/          # Output formatters (text, JSON, SARIF, diff, summary)
```

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the complete architectural design, including layer diagrams, extension points, and design patterns.

## License

MIT License - see [LICENSE](LICENSE) for details.
