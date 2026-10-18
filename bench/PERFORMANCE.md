# Performance Analysis: gomdlint vs markdownlint

This document provides a rigorous analysis of gomdlint's performance characteristics compared to markdownlint, the de facto standard Markdown linter.

> **Note (2026-10-18):** These numbers come from a development build measured in January 2026, before the changes listed under Unreleased in [CHANGELOG.md](../CHANGELOG.md), which turn every rule on by default and check every fix before it is applied. They have not been re-measured since.

## Executive Summary

Benchmarks on real-world, open-source repositories demonstrate that gomdlint achieves an **average 507.3x speedup** over markdownlint, with individual repository improvements ranging from 72x to 1100x.

| Repository | Files | gomdlint | markdownlint | Speedup |
|------------|-------|----------|--------------|---------|
| book | 478 | 0.02s | 1.44s | 72.0x |
| markdownlint | 515 | 0.02s | 3.10s | 155.0x |
| react | 1917 | 0.02s | 18.60s | 930.0x |
| vscode-docs | 744 | 0.02s | 5.58s | 279.0x |
| website | 7340 | 0.02s | 22.01s | 1100.5x |
| **Average** | | | | **507.3x** |

## Test Environment

### Hardware

- **Architecture:** Apple Silicon (arm64)
- **Platform:** Darwin (macOS)

### Software Versions

- **Go:** 1.25.6
- **Node.js:** v25.2.1
- **gomdlint:** development build (post-NodeCache optimization)
- **markdownlint-cli:** latest npm version

## Methodology

### Repository Selection

Test repositories were selected to represent real-world Markdown usage patterns:

1. **rust-lang/book** (478 files), technical documentation with structured content
2. **DavidAnson/markdownlint** (515 files), the markdownlint project itself, chosen to eliminate bias
3. **facebook/react** (1917 files), large, actively maintained open-source project
4. **microsoft/vscode-docs** (744 files), professional documentation with varied formatting
5. **golang/website** (7340 files), large-scale documentation site

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

- **gomdlint:** Direct directory path argument
- **markdownlint:** `find ... -name '*.md' | xargs markdownlint` (required for recursive processing)

Both tools:
- Process all Markdown files recursively
- Run with default rule sets enabled
- Output to `/dev/null` (timing only, not output formatting)

### Profiling Integration

Each gomdlint benchmark run generates CPU profiles, memory profiles, and execution traces, in these files:

- `bench/results/profiles/<timestamp>/<repo>/cpu.pprof`: CPU profile, readable with `go tool pprof`
- `bench/results/profiles/<timestamp>/<repo>/mem.pprof`: memory allocation profile
- `bench/results/profiles/<timestamp>/<repo>/trace.out`: execution trace, readable with `go tool trace`

These profiles enable post-hoc analysis of performance characteristics.

## Analysis

### Speedup Characteristics

The observed speedup shows strong correlation with repository size:

| File Count | Speedup Range |
|------------|---------------|
| ~500 | 72–155x |
| ~750 | 279x |
| ~1900 | 930x |
| ~7300 | 1100x |

This super-linear scaling advantage stems from:

1. **No interpreter startup:** Go compiles to native code; Node.js requires V8 initialization
2. **Efficient parallelism:** gomdlint uses Go's goroutines for file-level parallelism
3. **AST caching:** NodeCache provides O(1) node type lookups vs O(n) walks per rule
4. **Memory efficiency:** Pre-allocated slices and zero-copy byte operations

### Architectural Advantages

#### Node.js Overhead

markdownlint incurs per-file overhead:
- V8 JIT compilation
- Garbage collection pauses
- String-heavy intermediate representations

This overhead becomes dominant as file counts increase.

#### gomdlint Optimizations

Key optimizations contributing to performance:

1. **Single-pass AST walking:** NodeCache builds all node type indices in one traversal
2. **Pre-allocation:** Slice capacities tuned to typical document structures
3. **Parallel file processing:** Bounded goroutine pool processes files concurrently
4. **Direct byte operations:** Avoids string allocations where possible

### Memory Efficiency

While this report focuses on execution time, profiling data shows gomdlint maintains consistent memory usage regardless of repository size, while markdownlint exhibits linear memory growth proportional to file count.

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

gomdlint has a rule for each of markdownlint 0.41's 53 rules, plus two of its own, MDL001 and MDL003, but it is not a 1:1 port. It parses with goldmark instead of micromark, so a few constructs read differently (see [Known Limitations](../README.md#known-limitations) in the README), and it fixes a different set of rules. It fixes some that markdownlint leaves unfixed, such as `line-length` (MD013), and leaves unfixed some that markdownlint fixes, such as `link-fragments` (MD051). Benchmarks run both tools with their default rule sets.

### Platform Variance

Benchmarks were conducted on Apple Silicon (arm64). Performance characteristics may vary on:
- x86_64 architectures
- Linux vs macOS
- Different Go/Node.js versions

### Warm Cache Effect

Both tools benefit from OS file cache after the first run. The median-of-3 approach mitigates first-run cold-cache effects but does not eliminate them entirely.

### Output Suppression

Benchmarks suppress output to measure pure processing time. Real-world usage with formatted output will have slightly different characteristics, though the relative advantage remains.

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
- `no-trailing-spaces`, remove trailing whitespace
- `no-hard-tabs`, convert tabs to spaces
- `no-multiple-blank-lines`, collapse consecutive blank lines
- `single-trailing-newline`, ensure single newline at EOF

**Heading Formatting (83% fixable)**
- `heading-style`, enforce ATX/setext consistency
- `no-missing-space-atx`, add space after `#`
- `no-multiple-space-atx`, collapse multiple spaces
- `heading-blank-lines`, add surrounding blank lines
- `heading-start-left`, remove leading whitespace
- `no-trailing-punctuation`, remove trailing punctuation

**List Formatting (100% fixable)**
- `unordered-list-style`, consistent bullet markers
- `list-indent`, fix indentation levels
- `ul-indent`, enforce indentation rules
- `ol-prefix`, fix ordered list prefixes
- `list-marker-space`, correct spacing after markers
- `blanks-around-lists`, add surrounding blank lines

**Code Blocks (75% fixable)**
- `fenced-code-language`, add detected language identifiers
- `blanks-around-fences`, add surrounding blank lines
- `code-fence-style`, enforce fence style consistency
- `no-space-in-code`, remove internal spaces

**Links and Emphasis (70% fixable)**
- `no-reversed-links`, fix `(text)[url]` → `[text](url)`
- `no-bare-urls`, wrap URLs in angle brackets
- `no-emphasis-as-heading`, convert to proper headings
- `no-space-in-emphasis`, remove internal spaces
- `no-space-in-links`, trim link text
- `emphasis-style` / `strong-style`, enforce consistency

**Other Formatting**
- `hr-style`, consistent horizontal rules
- `no-multiple-space-blockquote`, fix blockquote spacing
- `proper-names`, fix capitalization
- `blanks-around-tables`, add surrounding blank lines
- `table-alignment`, fix delimiter row formatting

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

gomdlint demonstrates substantial performance advantages over markdownlint across diverse real-world repositories. The average 507x speedup makes gomdlint particularly suitable for:

- CI/CD pipelines where lint time impacts developer feedback loops
- Large documentation projects with thousands of Markdown files
- Pre-commit hooks where sub-second response time improves developer experience
- Editor integrations requiring responsive diagnostics

The performance advantage is not merely a constant factor but scales favorably with repository size, making gomdlint increasingly advantageous for larger projects.
