# Performance Comparison Harness Design

## Overview

A test harness to empirically compare markdown linting performance between gomdlint and markdownlint at scale. The goal is to validate performance claims with rigorous, reproducible benchmarks.

## Goals

- Validate the "3x faster" claim in README with real data
- Track performance over time to detect regressions
- Generate visual charts for documentation
- Run in CI (on-demand + scheduled weekly)

## Directory Structure

```
bench/
├── repos.txt              # List of repos to clone (one per line)
├── results/               # Output directory (gitignored)
│   ├── raw/               # JSON results from each run
│   ├── plots/             # Generated gnuplot charts
│   └── latest.txt         # Summary of most recent run
├── scripts/
│   ├── clone-repos.sh     # Clone/update repos from repos.txt
│   ├── run-bench.sh       # Execute benchmarks, collect metrics
│   ├── generate-plots.sh  # Run gnuplot to create charts
│   └── report.sh          # Print terminal summary
└── gnuplot/
    ├── time-comparison.gp # Wall-clock time chart
    ├── memory-usage.gp    # Memory comparison chart
    └── scaling.gp         # Files vs time scaling chart
```

Repos are cloned to `~/.cache/gomdlint-compare/repos/` to avoid bloating the repository.

## Test Corpus

Real-world markdown-heavy repositories providing a range of sizes:

| Repository | Approx Files | Purpose |
|------------|--------------|---------|
| kubernetes/website | ~4,000 | Very large documentation site |
| microsoft/vscode-docs | ~2,500 | Large documentation |
| golang/website | ~500 | Medium size |
| facebook/react | ~200 | Smaller project docs |
| rust-lang/book | ~100 | Small-medium |
| DavidAnson/markdownlint | ~50 | Small (reference implementation) |

Repos are shallow-cloned (`--depth 1`) to minimize disk usage.

## Metrics Collected

For each repository, both linters run 3 times with median taken:

- **Wall-clock time** (milliseconds)
- **Peak memory** (KB) - via `/usr/bin/time -v` or `gtime` on macOS
- **CPU time** (user + system)
- **Issue count** (informational, to verify comparable work)

### Scaling Analysis

Additionally, benchmarks run on file subsets (10%, 25%, 50%, 100%) to generate scaling curves.

### Output Format

JSON per run:

```json
{
  "timestamp": "2025-01-21T15:30:00Z",
  "gomdlint_version": "v0.1.0",
  "markdownlint_version": "0.39.0",
  "repos": {
    "kubernetes/website": {
      "file_count": 4012,
      "gomdlint": { "time_ms": 1200, "memory_kb": 45000, "issues": 342 },
      "markdownlint": { "time_ms": 3800, "memory_kb": 180000, "issues": 338 }
    }
  }
}
```

## Terminal Output

```
gomdlint comparison results (2025-01-21 15:30:00)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Repository              Files   gomdlint    markdownlint   Speedup
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
kubernetes/website       4012     1.20s         3.80s        3.2x
microsoft/vscode-docs    2534     0.78s         2.41s        3.1x
golang/website            487     0.15s         0.48s        3.2x
facebook/react            203     0.06s         0.19s        3.2x
DavidAnson/markdownlint    52     0.02s         0.06s        3.0x
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Average speedup: 3.1x
```

## Gnuplot Charts

Generated automatically after each run:

1. **Bar chart** - Side-by-side time comparison per repo
2. **Line chart** - Scaling curve (file count vs time)
3. **Memory chart** - Peak memory comparison

Output to `bench/results/plots/` with timestamps and `latest-*.png` symlinks.

## Stave Integration

New targets in `stavefile.go`:

```go
// Compare benchmarks gomdlint against markdownlint.
func Compare(ctx context.Context) error { ... }

// CompareFast runs quick comparison (~30 sec).
func CompareFast(ctx context.Context) error { ... }
```

Usage:

```bash
stave compare      # Full suite (~5 min)
stave compareFast  # Quick check (~30 sec)
```

Setup (cloning repos) is automatic on first run.

## CI Integration

GitHub Actions workflow at `.github/workflows/compare.yml`:

- **Trigger**: Manual (`workflow_dispatch`) + weekly schedule (Sunday 3am UTC)
- **Caching**: Benchmark repos cached between runs
- **Artifacts**: Results uploaded for historical tracking

```yaml
name: Performance Comparison

on:
  workflow_dispatch:
  schedule:
    - cron: '0 3 * * 0'

jobs:
  compare:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
      - name: Install markdownlint-cli
        run: npm install -g markdownlint-cli
      - name: Cache benchmark repos
        uses: actions/cache@v4
        with:
          path: ~/.cache/gomdlint-compare
          key: compare-repos-v1
      - name: Run comparison
        run: stave compare
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: comparison-results
          path: bench/results/
```

## Dependencies

- `gnuplot` - for chart generation
- `gtime` (macOS) or `/usr/bin/time` (Linux) - for memory profiling
- `markdownlint-cli` - the comparison target
- `jq` - for JSON processing in scripts

## Open Questions

None - design is complete.
