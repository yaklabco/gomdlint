# Performance Analysis: gomdlint vs markdownlint

This document provides a rigorous analysis of gomdlint's performance characteristics compared to markdownlint, the de facto standard Markdown linter.

## Executive Summary

Results pending re-run after performance fixes. Run `./bench/scripts/run-bench.sh` to generate current numbers.

## Methodology Correction

Previous versions of this document reported an "average 507.3x speedup" with gomdlint completing in 0.02s for every repository regardless of size. Those results were invalid — gomdlint was likely exiting early (no files found or immediate error) rather than doing real work.

Additionally, the benchmark script was measuring **lint + full report formatting + I/O**, not just lint time. Profiling revealed that 51% of CPU time was spent in write syscalls for diagnostic output, 6% in repeated file content parsing for source line context, and 23% in GC from string allocations — leaving only 9% for actual linting.

The benchmark script now uses `--quiet` to measure pure lint time without reporting overhead. The reporter has also been optimized with buffered I/O and O(1) source line lookup.

## Test Environment

### Hardware

- **Architecture:** Apple Silicon (arm64)
- **Platform:** Darwin (macOS)

### Software Versions

- **Go:** 1.25.6
- **Node.js:** v25.2.1
- **gomdlint:** development build
- **markdownlint-cli:** latest npm version

## Methodology

### Repository Selection

Test repositories were selected to represent real-world Markdown usage patterns:

1. **rust-lang/book** (478 files) — Technical documentation with structured content
2. **DavidAnson/markdownlint** (515 files) — The markdownlint project itself, chosen to eliminate bias
3. **facebook/react** (1917 files) — Large, actively maintained open-source project
4. **microsoft/vscode-docs** (744 files) — Professional documentation with varied formatting
5. **golang/website** (7340 files) — Large-scale documentation site

This selection provides:
- Diversity in file counts (478–7340 files)
- Variety in document types (tutorials, API docs, READMEs, changelogs)
- Real-world complexity (nested lists, code blocks, tables, links)
- Inclusion of markdownlint's own repository to ensure fairness

### Measurement Protocol

#### Timing Method

All measurements use GNU `time` (`gtime` on macOS) for high-precision wall-clock timing:

```bash
gtime -f '%e %M' <command>
```

This captures:
- **Wall-clock time** (seconds, millisecond precision)
- **Peak memory usage** (kilobytes)

#### Statistical Approach

Each measurement follows this protocol:

1. **Multiple runs:** 3 runs per tool per repository (configurable via `BENCH_RUNS`)
2. **Median selection:** The median time is used to reduce impact of outliers
3. **Cold cache mitigation:** Repositories are cloned once; subsequent runs benefit from OS file caching (equally for both tools)

The median was chosen over mean to provide robustness against:
- System interrupts and background processes
- JIT warm-up variance (markdownlint/Node.js)
- Garbage collection pauses

#### Execution Conditions

- **gomdlint:** Runs with `--quiet` to suppress output formatting, measuring pure lint time
- **markdownlint:** `find ... -name '*.md' | xargs markdownlint` with output captured to temp file

Both tools:
- Process all Markdown files recursively
- Run with default rule sets enabled

### Profiling Integration

Each gomdlint benchmark run generates CPU profiles, memory profiles, and execution traces:

```
bench/results/profiles/<timestamp>/<repo>/
├── cpu.pprof    # CPU profile (go tool pprof compatible)
├── mem.pprof    # Memory allocation profile
└── trace.out    # Execution trace (go tool trace compatible)
```

These profiles enable post-hoc analysis of performance characteristics.

## Analysis

### Architectural Advantages

#### Node.js Overhead

markdownlint incurs per-file overhead:
- V8 JIT compilation
- Garbage collection pauses
- String-heavy intermediate representations

#### gomdlint Optimizations

Key optimizations contributing to performance:

1. **Single-pass AST walking:** NodeCache builds all node type indices in one traversal
2. **Pre-allocation:** Slice capacities tuned to typical document structures
3. **Parallel file processing:** Bounded goroutine pool processes files concurrently
4. **Direct byte operations:** Avoids string allocations where possible
5. **Buffered output:** All reporters use 64KB buffered writers to minimize syscalls
6. **O(1) source line lookup:** Pre-indexed line offsets avoid re-parsing file content

## Reproducibility

### Running Benchmarks

```bash
# Clone test repositories
./bench/scripts/clone-repos.sh

# Run comparison benchmark
./bench/scripts/run-bench.sh

# Results written to bench/results/raw/<timestamp>.json
```

### Configuration

- `BENCH_RUNS=<n>`: Number of runs per measurement (default: 3)
- `GOMDLINT_COMPARE_CACHE`: Custom cache directory for repositories

### Adding Repositories

Edit `bench/repos.txt` to add or remove test repositories:

```text
# Format: org/repo
kubernetes/website
microsoft/vscode-docs
```

## Limitations and Caveats

### Rule Parity

gomdlint implements equivalent functionality for most markdownlint rules but is not a 1:1 port. Some markdownlint rules have no gomdlint equivalent, and vice versa. Benchmarks run both tools with their default rule sets.

### Platform Variance

Benchmarks were conducted on Apple Silicon (arm64). Performance characteristics may vary on:
- x86_64 architectures
- Linux vs macOS
- Different Go/Node.js versions

### Warm Cache Effect

Both tools benefit from OS file cache after the first run. The median-of-3 approach mitigates first-run cold-cache effects but does not eliminate them entirely.

### Output Suppression

gomdlint benchmarks use the `--quiet` flag to suppress all diagnostic output, measuring pure lint time. markdownlint output is captured to a temp file. Real-world usage with formatted output will have additional overhead from report formatting and I/O.

## Auto-Fix Coverage

Beyond raw performance, gomdlint provides comprehensive automatic fixing capabilities.

### Coverage Statistics

| Metric | Value |
|--------|-------|
| Total rules | 55 |
| Auto-fixable rules | 37 (67%) |
| Estimated issue coverage | ~90% |

The "estimated issue coverage" reflects that fixable rules address the most common Markdown issues encountered in practice (whitespace, formatting, style consistency).

### Auto-Fixable Rule Categories

**Whitespace (100% fixable)**
- `no-trailing-spaces` — Remove trailing whitespace
- `no-hard-tabs` — Convert tabs to spaces
- `no-multiple-blank-lines` — Collapse consecutive blank lines
- `single-trailing-newline` — Ensure single newline at EOF

**Heading Formatting (83% fixable)**
- `heading-style` — Enforce ATX/setext consistency
- `no-missing-space-atx` — Add space after `#`
- `no-multiple-space-atx` — Collapse multiple spaces
- `heading-blank-lines` — Add surrounding blank lines
- `heading-start-left` — Remove leading whitespace
- `no-trailing-punctuation` — Remove trailing punctuation

**List Formatting (100% fixable)**
- `unordered-list-style` — Consistent bullet markers
- `list-indent` — Fix indentation levels
- `ul-indent` — Enforce indentation rules
- `ol-prefix` — Fix ordered list prefixes
- `list-marker-space` — Correct spacing after markers
- `blanks-around-lists` — Add surrounding blank lines

**Code Blocks (75% fixable)**
- `fenced-code-language` — Add detected language identifiers
- `blanks-around-fences` — Add surrounding blank lines
- `code-fence-style` — Enforce fence style consistency
- `no-space-in-code` — Remove internal spaces

**Links and Emphasis (70% fixable)**
- `no-reversed-links` — Fix `(text)[url]` → `[text](url)`
- `no-bare-urls` — Wrap URLs in angle brackets
- `no-emphasis-as-heading` — Convert to proper headings
- `no-space-in-emphasis` — Remove internal spaces
- `no-space-in-links` — Trim link text
- `emphasis-style` / `strong-style` — Enforce consistency

**Other Formatting**
- `hr-style` — Consistent horizontal rules
- `no-multiple-space-blockquote` — Fix blockquote spacing
- `proper-names` — Fix capitalization
- `blanks-around-tables` — Add surrounding blank lines
- `table-alignment` — Fix delimiter row formatting

### Non-Fixable Rules

Some rules cannot be auto-fixed because they require human judgment:

- **Semantic rules:** `heading-increment`, `no-duplicate-heading`, `single-h1`
- **Content rules:** `no-alt-text`, `descriptive-link-text`, `first-line-heading`
- **Structural rules:** `no-empty-links`, `required-headings`, `reference-links-images`
- **Validation rules:** `link-fragments`, `table-column-count`

### Fix Safety

gomdlint's auto-fix system includes safety mechanisms:

1. **Conflict detection:** Overlapping edits are safely merged or deferred
2. **Multi-pass fixing:** Cascading issues are resolved iteratively
3. **Backup by default:** Original files are preserved (disable with `--no-backups`)
4. **Dry-run mode:** Preview changes with `--fix --dry-run --format diff`

## Conclusion

gomdlint's performance advantages come from native compilation, goroutine parallelism, AST caching, and efficient byte operations. Run `./bench/scripts/run-bench.sh` to generate current benchmark numbers for your hardware.
