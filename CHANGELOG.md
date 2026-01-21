# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/spec/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-01-20

### Added

- `lint` command for linting Markdown files and directories
- `rules` command to list available lint rules
- `init` command to generate configuration files
- `migrate` command for markdownlint config conversion
- `version` command with build metadata
- `--fix` flag for automatic issue correction
- `--dry-run` flag to preview fixes without applying
- `--format` flag with text, table, json, sarif, diff, and summary outputs
- `--rule-format` flag for name, id, or combined rule identifiers
- `--summary-order` flag to control table ordering in summary format
- `--jobs` flag for parallel worker configuration
- `--ignore` flag for glob-based file exclusion
- `--enable` and `--disable` flags for rule control
- `--fix-rules` flag to limit auto-fixing to specific rules
- `--no-backups` flag to disable backup creation during fix
- `--flavor` flag for CommonMark or GFM mode
- `--strict` flag to treat warnings as errors
- `--config` flag to specify configuration file path
- `--color` flag with auto, always, and never modes
- `--compact` and `--no-context` flags for minimal output
- `--per-file` flag for grouped table output
- YAML configuration via `.gomdlint.yml` or `.gomdlint.yaml`
- Hierarchical config discovery (project, user, system)
- Per-rule configuration for enabled, severity, auto_fix, and options
- Include/exclude glob patterns in configuration
- Environment variable overrides via `GOMDLINT_*`
- 12 heading rules: MD001, MD003, MD018-MD026, MD041
- 6 list rules: MD004, MD005, MD007, MD029, MD030, MD032
- 4 whitespace rules: MD009, MD010, MD012, MD047
- 6 code block rules: MD014, MD031, MD038, MD040, MD046, MD048
- 8 link rules: MD011, MD034, MD039, MD042, MD045, MD051-MD054
- 4 emphasis rules: MD036, MD037, MD049, MD050
- 2 blockquote rules: MD027, MD028
- 4 table rules (GFM): MD055, MD056, MD058, MD060
- 4 other rules: MD013, MD033, MD035, MD043, MD044
- Autofix support for 30+ rules
- Multi-pass fixing for cascading issues
- Conflict detection for incompatible edits
- Automatic backup creation with `.gomdlint.bak` suffix
- Language detection for code blocks (10 languages)
- SARIF 2.1.0 output for GitHub code scanning
- Parallel file processing with deterministic ordering
- Relative path display in output

[Unreleased]: https://github.com/jamesainslie/gomdlint/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/jamesainslie/gomdlint/releases/tag/v0.1.0
