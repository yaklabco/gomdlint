# Changelog

## [0.2.1] - 2026-02-06

No functional changes. Release process verification.

## [0.2.0] - 2026-02-06

### Added

- Warn when the multi-pass fix loop exhausts its pass limit without converging. Previously, gomdlint would silently stop fixing after 10 passes — now you'll see a warning if there were still edits pending.

### Fixed

- **MD032**: Now detects missing blank lines *after* lists, not just before. This was tricky because CommonMark's "lazy continuation" feature can absorb text into list items, but the rule now handles that correctly.
- **MD051**: No longer flags external URLs or code spans as broken internal links. The fragment validation was being too aggressive.
- **MD053 and others**: Fixed false positives where content inside fenced code blocks was incorrectly matched by line-scanning rules. Code block detection is now centralized and consistent across all rules.

## [0.1.1] - 2026-01-22

### Added

- Homebrew tap distribution: `brew install yaklabco/tap/gomdlint`
- Shell completions for bash, zsh, and fish bundled in releases

## [0.1.0] - 2026-01-22

Initial release of gomdlint - a fast Markdown linter written in Go.

### Features

- **55 lint rules** covering headings, lists, whitespace, code blocks, links, emphasis, blockquotes, and tables
- **Autofix support** for 37 rules (67% coverage)
- **Multiple output formats**: text, table, json, sarif, diff, summary
- **Parallel processing** for fast linting of large codebases
- **507x faster** than markdownlint on real-world repositories

### Commands

- `gomdlint lint` - Lint files with `--fix` for auto-correction
- `gomdlint rules` - List available rules
- `gomdlint init` - Generate config file
- `gomdlint migrate` - Convert markdownlint configs
- `gomdlint version` - Show version info

### Configuration

- YAML config via `.gomdlint.yml`
- Per-rule settings for severity, autofix, and options
- Include/exclude glob patterns
- Environment variable overrides (`GOMDLINT_*`)
- CommonMark and GFM flavor support

[0.2.1]: https://github.com/yaklabco/gomdlint/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/yaklabco/gomdlint/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/yaklabco/gomdlint/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/yaklabco/gomdlint/releases/tag/v0.1.0
