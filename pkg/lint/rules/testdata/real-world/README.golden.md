# gomdlint

A blisteringly fast, self-fixing Markdown linter written in Go.

## Features

- CommonMark + GFM support
- Rich rule system (syntax + style)
- Maximally safe auto-fix with conflict detection
- Modern CLI UX with beautiful output
- Fast parallel processing
- Human-readable rule names in output

## Usage

### Basic Linting

```bash
gomdlint lint                    # Lint current directory
gomdlint lint docs/              # Lint docs directory
gomdlint lint README.md          # Lint single file
gomdlint lint --fix              # Lint and auto-fix issues
```

### Rule Identifier Format

gomdlint uses human-readable rule names by default for clearer output. You can control the format with `--rule-format`:

```bash
# Human-readable names (default)
gomdlint lint file.md
# Output: no-trailing-spaces: Trailing whitespace

# Traditional markdownlint IDs
gomdlint lint --rule-format id file.md
# Output: MD009: Trailing whitespace

# Combined format (ID/name)
gomdlint lint --rule-format combined file.md
# Output: MD009/no-trailing-spaces: Trailing whitespace
```

### Output Formats

gomdlint supports multiple output formats:

```bash
# Text format (default) - human-readable with colors
gomdlint lint file.md

# Table format - columnar output
gomdlint lint --format table file.md

# JSON format - machine-readable
gomdlint lint --format json file.md

# SARIF format - for CI/CD integration
gomdlint lint --format sarif file.md

# Diff format - unified diff for fixes
gomdlint lint --format diff --fix --dry-run file.md

# Summary format - aggregated tables by rule and file
gomdlint lint --format summary file.md
```

The summary format shows aggregated statistics:

```
Rules Summary
──────────────────────────────────────────────────────────────────────
Rule                           Count   Errors Warnings  Fixable
──────────────────────────────────────────────────────────────────────
no-trailing-spaces                 5       0        5        ✓
no-hard-tabs                       2       0        2        ✓

Files Summary
────────────────────────────────────────────────────────────
File                                Count   Errors Warnings
────────────────────────────────────────────────────────────
README.md                               3       0        3
docs/guide.md                           4       0        4

Total: 7 issues (7 warnings) in 2 files
```

Control table order with `--summary-order`:

```bash
# Rules table first (default)
gomdlint lint --format summary --summary-order rules file.md

# Files table first
gomdlint lint --format summary --summary-order files file.md
```

### List Available Rules

```bash
gomdlint rules                      # List rules with names (default)
gomdlint rules --rule-format id     # List rules with IDs
```

### Configuration File

Both rule IDs and human-readable names work in configuration files:

```yaml
# .gomdlint.yaml
rules:
  # Using human-readable name
  no-trailing-spaces:
    enabled: false

  # Using traditional ID (also works)
  MD010:
    severity: error
    options:
      code_blocks: true
```

## Development

This project uses [stave](https://github.com/yaklabco/stave) for build automation.

### Prerequisites

- Go 1.25+ (tested with 1.25.4)
- [golangci-lint](https://golangci-lint.run/usage/install/) (latest version)
- [stave](https://github.com/yaklabco/stave) - `go install github.com/yaklabco/stave@latest`

Install prerequisites:

```bash

# Install Go (if not already installed)
# See https://go.dev/doc/install

# Install golangci-lint
brew install golangci-lint  # macOS
# or
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install stave
go install github.com/yaklabco/stave@latest
```

### Build Tasks

List all available tasks:

```bash

stave -l
```

Common tasks:

```bash

# Build the project
stave build

# Run CI gate checks (fmt, vet, lint, build)
stave cigate

# Run tests
stave test

# Run linters
stave lint

# Format code
stave format

# Run all checks (format, lint, test)
stave check

# Install to GOPATH/bin
stave install

# Clean build artifacts
stave clean

# Download and tidy dependencies
stave deps

# Generate coverage report
stave coverage

# Run benchmarks
stave bench
```

### Project Structure

```

gomdlint/
  cmd/
    gomdlint/            # main.go, Cobra root command
  internal/
    cli/                 # CLI wiring, flag handling
    configloader/        # Viper + XDG config resolution
    ui/
      pretty/            # Lipgloss-based pretty printer
    logging/             # charmbracelet/log setup
  pkg/
    mdast/               # FileSnapshot, Tokens, AST, Parser interface
    lint/                # Rule interfaces, registry, engine
    fix/                 # TextEdit, EditBuilder, ApplyEdits, validation
    config/              # Core config types
    parser/
      goldmark/          # goldmark-based Parser implementation
    runner/              # multi-file runner, concurrency, globs, ignore
    reporter/            # text, JSON, SARIF, diff reports
```

## Architecture

See [design-docs.md](design-docs.md) for the complete architectural design.

See [implementation-plan.md](implementation-plan.md) for the phased implementation plan.

## Status

Currently in Phase 0: Project skeleton and core plumbing.

## License

MIT License - see [LICENSE](LICENSE) for details.
