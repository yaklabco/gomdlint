# Performance Comparison Harness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a test harness to empirically compare gomdlint vs markdownlint performance at scale.

**Architecture:** Shell scripts handle repo cloning, benchmarking, and gnuplot chart generation. Stave targets (`Compare`, `CompareFast`) orchestrate everything. Results stored as JSON with terminal output and PNG charts.

**Tech Stack:** Bash scripts, gnuplot, GNU time (gtime on macOS), jq for JSON processing, GitHub Actions for CI.

---

## Task 1: Create Directory Structure

**Files:**
- Create: `bench/repos.txt`
- Create: `bench/.gitkeep` (for empty directories)
- Modify: `.gitignore`

**Step 1: Create bench directory structure**

```bash
cd /Volumes/Development/gomdlint/.worktrees/compare
mkdir -p bench/scripts bench/gnuplot bench/results
touch bench/results/.gitkeep
```

**Step 2: Create repos.txt**

Create `bench/repos.txt`:
```
# Real-world markdown-heavy repositories
# Format: org/repo

# Large documentation sites
kubernetes/website
microsoft/vscode-docs
golang/website

# Popular projects with docs
facebook/react
rust-lang/book

# Small reference (markdownlint itself)
DavidAnson/markdownlint
```

**Step 3: Add results to .gitignore**

Add to `.gitignore`:
```
# Benchmark results (generated)
bench/results/raw/
bench/results/plots/
bench/results/latest.txt
```

**Step 4: Commit**

```bash
git add bench/ .gitignore
git commit -m "feat(bench): add directory structure and repo list"
```

---

## Task 2: Create Clone Script

**Files:**
- Create: `bench/scripts/clone-repos.sh`

**Step 1: Write clone-repos.sh**

Create `bench/scripts/clone-repos.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Clone or update benchmark repositories
# Repos are stored in ~/.cache/gomdlint-compare/repos/

CACHE_DIR="${GOMDLINT_COMPARE_CACHE:-$HOME/.cache/gomdlint-compare}"
REPOS_DIR="$CACHE_DIR/repos"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOS_FILE="$SCRIPT_DIR/../repos.txt"

mkdir -p "$REPOS_DIR"

echo "Benchmark repo cache: $REPOS_DIR"
echo ""

# Read repos, skip comments and empty lines
grep -v '^#' "$REPOS_FILE" | grep -v '^$' | while read -r repo; do
    repo_name=$(basename "$repo")
    repo_path="$REPOS_DIR/$repo_name"

    if [ -d "$repo_path" ]; then
        echo "Updating $repo..."
        git -C "$repo_path" pull --ff-only --depth 1 2>/dev/null || true
    else
        echo "Cloning $repo..."
        git clone --depth 1 "https://github.com/$repo.git" "$repo_path"
    fi
done

echo ""
echo "All repos ready."
```

**Step 2: Make executable and test**

```bash
chmod +x bench/scripts/clone-repos.sh
./bench/scripts/clone-repos.sh
```

Expected: Repos clone to `~/.cache/gomdlint-compare/repos/`

**Step 3: Commit**

```bash
git add bench/scripts/clone-repos.sh
git commit -m "feat(bench): add clone-repos script"
```

---

## Task 3: Create Benchmark Runner Script

**Files:**
- Create: `bench/scripts/run-bench.sh`

**Step 1: Write run-bench.sh**

Create `bench/scripts/run-bench.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Run benchmarks comparing gomdlint vs markdownlint
# Outputs JSON results and terminal summary

CACHE_DIR="${GOMDLINT_COMPARE_CACHE:-$HOME/.cache/gomdlint-compare}"
REPOS_DIR="$CACHE_DIR/repos"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/../results"
RUNS="${BENCH_RUNS:-3}"

# Detect GNU time
if command -v gtime &>/dev/null; then
    TIME_CMD="gtime"
elif /usr/bin/time --version 2>&1 | grep -q GNU; then
    TIME_CMD="/usr/bin/time"
else
    echo "Error: GNU time required. Install with: brew install gnu-time (macOS)"
    exit 1
fi

# Check tools exist
command -v gomdlint &>/dev/null || { echo "Error: gomdlint not found"; exit 1; }
command -v markdownlint &>/dev/null || { echo "Error: markdownlint not found. Install: npm i -g markdownlint-cli"; exit 1; }
command -v jq &>/dev/null || { echo "Error: jq not found"; exit 1; }

# Setup results directory
mkdir -p "$RESULTS_DIR/raw" "$RESULTS_DIR/plots"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
RESULT_FILE="$RESULTS_DIR/raw/$(date +%Y%m%d-%H%M%S).json"

# Get versions
GOMDLINT_VERSION=$(gomdlint --version 2>/dev/null | head -1 || echo "unknown")
MARKDOWNLINT_VERSION=$(markdownlint --version 2>/dev/null || echo "unknown")

echo "gomdlint vs markdownlint comparison"
echo "===================================="
echo "gomdlint version: $GOMDLINT_VERSION"
echo "markdownlint version: $MARKDOWNLINT_VERSION"
echo "Runs per tool: $RUNS"
echo ""

# Function to run and time a linter
run_linter() {
    local cmd="$1"
    local repo_path="$2"
    local tmp_time=$(mktemp)

    # Run with GNU time, capture memory and time
    $TIME_CMD -f '%e %M' -o "$tmp_time" $cmd "$repo_path"/**/*.md 2>/dev/null || true

    read -r time_sec mem_kb < "$tmp_time"
    rm -f "$tmp_time"

    # Convert to milliseconds
    local time_ms=$(echo "$time_sec * 1000" | bc | cut -d. -f1)
    echo "$time_ms $mem_kb"
}

# Function to get median of runs
median_run() {
    local cmd="$1"
    local repo_path="$2"
    local times=()
    local mems=()

    for ((i=1; i<=RUNS; i++)); do
        result=$(run_linter "$cmd" "$repo_path")
        times+=($(echo "$result" | cut -d' ' -f1))
        mems+=($(echo "$result" | cut -d' ' -f2))
    done

    # Sort and get median
    IFS=$'\n' sorted_times=($(sort -n <<<"${times[*]}")); unset IFS
    IFS=$'\n' sorted_mems=($(sort -n <<<"${mems[*]}")); unset IFS

    local mid=$((RUNS / 2))
    echo "${sorted_times[$mid]} ${sorted_mems[$mid]}"
}

# Count issues for a linter
count_issues() {
    local cmd="$1"
    local repo_path="$2"
    $cmd "$repo_path"/**/*.md 2>/dev/null | wc -l | tr -d ' '
}

# Initialize JSON
echo "{" > "$RESULT_FILE"
echo "  \"timestamp\": \"$TIMESTAMP\"," >> "$RESULT_FILE"
echo "  \"gomdlint_version\": \"$GOMDLINT_VERSION\"," >> "$RESULT_FILE"
echo "  \"markdownlint_version\": \"$MARKDOWNLINT_VERSION\"," >> "$RESULT_FILE"
echo "  \"runs\": $RUNS," >> "$RESULT_FILE"
echo "  \"repos\": {" >> "$RESULT_FILE"

# Print header
printf "%-30s %8s %12s %12s %8s\n" "Repository" "Files" "gomdlint" "markdownlint" "Speedup"
printf "%-30s %8s %12s %12s %8s\n" "----------" "-----" "--------" "------------" "-------"

first_repo=true
total_speedup=0
repo_count=0

# Process each repo
for repo_path in "$REPOS_DIR"/*/; do
    repo_name=$(basename "$repo_path")

    # Count markdown files
    file_count=$(find "$repo_path" -name "*.md" -type f | wc -l | tr -d ' ')

    if [ "$file_count" -eq 0 ]; then
        continue
    fi

    # Run benchmarks
    gomdlint_result=$(median_run "gomdlint lint" "$repo_path")
    gomdlint_time=$(echo "$gomdlint_result" | cut -d' ' -f1)
    gomdlint_mem=$(echo "$gomdlint_result" | cut -d' ' -f2)
    gomdlint_issues=$(count_issues "gomdlint lint" "$repo_path")

    markdownlint_result=$(median_run "markdownlint" "$repo_path")
    markdownlint_time=$(echo "$markdownlint_result" | cut -d' ' -f1)
    markdownlint_mem=$(echo "$markdownlint_result" | cut -d' ' -f2)
    markdownlint_issues=$(count_issues "markdownlint" "$repo_path")

    # Calculate speedup
    if [ "$gomdlint_time" -gt 0 ]; then
        speedup=$(echo "scale=1; $markdownlint_time / $gomdlint_time" | bc)
    else
        speedup="N/A"
    fi

    # Format times for display
    gomdlint_display=$(echo "scale=2; $gomdlint_time / 1000" | bc)s
    markdownlint_display=$(echo "scale=2; $markdownlint_time / 1000" | bc)s

    printf "%-30s %8d %12s %12s %7sx\n" "$repo_name" "$file_count" "$gomdlint_display" "$markdownlint_display" "$speedup"

    # Add to JSON
    if [ "$first_repo" = true ]; then
        first_repo=false
    else
        echo "," >> "$RESULT_FILE"
    fi

    cat >> "$RESULT_FILE" <<EOF
    "$repo_name": {
      "file_count": $file_count,
      "gomdlint": { "time_ms": $gomdlint_time, "memory_kb": $gomdlint_mem, "issues": $gomdlint_issues },
      "markdownlint": { "time_ms": $markdownlint_time, "memory_kb": $markdownlint_mem, "issues": $markdownlint_issues }
    }
EOF

    if [ "$speedup" != "N/A" ]; then
        total_speedup=$(echo "$total_speedup + $speedup" | bc)
        repo_count=$((repo_count + 1))
    fi
done

# Close JSON
echo "" >> "$RESULT_FILE"
echo "  }" >> "$RESULT_FILE"
echo "}" >> "$RESULT_FILE"

# Print summary
echo ""
if [ "$repo_count" -gt 0 ]; then
    avg_speedup=$(echo "scale=1; $total_speedup / $repo_count" | bc)
    echo "Average speedup: ${avg_speedup}x"
fi

echo ""
echo "Results saved to: $RESULT_FILE"

# Update latest symlink
echo "$RESULT_FILE" > "$RESULTS_DIR/latest.txt"
```

**Step 2: Make executable**

```bash
chmod +x bench/scripts/run-bench.sh
```

**Step 3: Commit**

```bash
git add bench/scripts/run-bench.sh
git commit -m "feat(bench): add benchmark runner script"
```

---

## Task 4: Create Gnuplot Chart Templates

**Files:**
- Create: `bench/gnuplot/time-comparison.gp`
- Create: `bench/gnuplot/memory-comparison.gp`

**Step 1: Write time-comparison.gp**

Create `bench/gnuplot/time-comparison.gp`:
```gnuplot
# Time comparison bar chart
# Usage: gnuplot -e "datafile='data.dat'; outfile='chart.png'" time-comparison.gp

set terminal pngcairo size 800,500 enhanced font 'Arial,12'
set output outfile

set title "Linting Time Comparison" font 'Arial,14'
set xlabel "Repository"
set ylabel "Time (seconds)"

set style data histogram
set style histogram cluster gap 1
set style fill solid border -1
set boxwidth 0.9

set xtics rotate by -45
set key top left

set grid ytics

plot datafile using 2:xtic(1) title "gomdlint" linecolor rgb "#4CAF50", \
     '' using 3 title "markdownlint" linecolor rgb "#2196F3"
```

**Step 2: Write memory-comparison.gp**

Create `bench/gnuplot/memory-comparison.gp`:
```gnuplot
# Memory comparison bar chart
# Usage: gnuplot -e "datafile='data.dat'; outfile='chart.png'" memory-comparison.gp

set terminal pngcairo size 800,500 enhanced font 'Arial,12'
set output outfile

set title "Peak Memory Usage Comparison" font 'Arial,14'
set xlabel "Repository"
set ylabel "Memory (MB)"

set style data histogram
set style histogram cluster gap 1
set style fill solid border -1
set boxwidth 0.9

set xtics rotate by -45
set key top left

set grid ytics

plot datafile using ($2/1024):xtic(1) title "gomdlint" linecolor rgb "#4CAF50", \
     '' using ($3/1024) title "markdownlint" linecolor rgb "#2196F3"
```

**Step 3: Commit**

```bash
git add bench/gnuplot/
git commit -m "feat(bench): add gnuplot chart templates"
```

---

## Task 5: Create Chart Generation Script

**Files:**
- Create: `bench/scripts/generate-plots.sh`

**Step 1: Write generate-plots.sh**

Create `bench/scripts/generate-plots.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Generate gnuplot charts from benchmark results

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/../results"
GNUPLOT_DIR="$SCRIPT_DIR/../gnuplot"

command -v gnuplot &>/dev/null || { echo "Error: gnuplot not found"; exit 1; }
command -v jq &>/dev/null || { echo "Error: jq not found"; exit 1; }

# Find latest results
if [ ! -f "$RESULTS_DIR/latest.txt" ]; then
    echo "Error: No results found. Run benchmarks first."
    exit 1
fi

RESULT_FILE=$(cat "$RESULTS_DIR/latest.txt")
if [ ! -f "$RESULT_FILE" ]; then
    echo "Error: Result file not found: $RESULT_FILE"
    exit 1
fi

TIMESTAMP=$(basename "$RESULT_FILE" .json)
PLOTS_DIR="$RESULTS_DIR/plots"
mkdir -p "$PLOTS_DIR"

echo "Generating charts from: $RESULT_FILE"

# Generate time data file
TIME_DATA=$(mktemp)
jq -r '.repos | to_entries[] | "\(.key) \(.value.gomdlint.time_ms / 1000) \(.value.markdownlint.time_ms / 1000)"' "$RESULT_FILE" > "$TIME_DATA"

# Generate memory data file
MEM_DATA=$(mktemp)
jq -r '.repos | to_entries[] | "\(.key) \(.value.gomdlint.memory_kb) \(.value.markdownlint.memory_kb)"' "$RESULT_FILE" > "$MEM_DATA"

# Generate time chart
TIME_CHART="$PLOTS_DIR/time-$TIMESTAMP.png"
gnuplot -e "datafile='$TIME_DATA'; outfile='$TIME_CHART'" "$GNUPLOT_DIR/time-comparison.gp"
echo "Created: $TIME_CHART"

# Generate memory chart
MEM_CHART="$PLOTS_DIR/memory-$TIMESTAMP.png"
gnuplot -e "datafile='$MEM_DATA'; outfile='$MEM_CHART'" "$GNUPLOT_DIR/memory-comparison.gp"
echo "Created: $MEM_CHART"

# Update latest symlinks
ln -sf "time-$TIMESTAMP.png" "$PLOTS_DIR/latest-time.png"
ln -sf "memory-$TIMESTAMP.png" "$PLOTS_DIR/latest-memory.png"

# Cleanup
rm -f "$TIME_DATA" "$MEM_DATA"

echo ""
echo "Charts generated in: $PLOTS_DIR"
```

**Step 2: Make executable**

```bash
chmod +x bench/scripts/generate-plots.sh
```

**Step 3: Commit**

```bash
git add bench/scripts/generate-plots.sh
git commit -m "feat(bench): add chart generation script"
```

---

## Task 6: Add Stave Targets

**Files:**
- Modify: `stavefile.go`

**Step 1: Add Compare and CompareFast functions**

Add to `stavefile.go` before the helper functions:

```go
// Compare benchmarks gomdlint against markdownlint.
func Compare(ctx context.Context) error {
	fmt.Println("Running gomdlint vs markdownlint comparison...")

	// Ensure gomdlint is built
	if err := Build(ctx); err != nil {
		return fmt.Errorf("build gomdlint: %w", err)
	}

	// Check dependencies
	if err := checkCompareDepends(); err != nil {
		return err
	}

	// Clone repos if needed
	if err := sh(ctx, "bench/scripts/clone-repos.sh"); err != nil {
		return fmt.Errorf("clone repos: %w", err)
	}

	// Run benchmarks
	if err := sh(ctx, "bench/scripts/run-bench.sh"); err != nil {
		return fmt.Errorf("run benchmarks: %w", err)
	}

	// Generate charts
	if err := sh(ctx, "bench/scripts/generate-plots.sh"); err != nil {
		return fmt.Errorf("generate charts: %w", err)
	}

	return nil
}

// CompareFast runs quick comparison on smallest repos only.
func CompareFast(ctx context.Context) error {
	fmt.Println("Running quick gomdlint vs markdownlint comparison...")

	// Ensure gomdlint is built
	if err := Build(ctx); err != nil {
		return fmt.Errorf("build gomdlint: %w", err)
	}

	// Check dependencies
	if err := checkCompareDepends(); err != nil {
		return err
	}

	// Clone repos if needed
	if err := sh(ctx, "bench/scripts/clone-repos.sh"); err != nil {
		return fmt.Errorf("clone repos: %w", err)
	}

	// Run benchmarks with reduced runs
	cmd := exec.CommandContext(ctx, "bench/scripts/run-bench.sh")
	cmd.Env = append(os.Environ(), "BENCH_RUNS=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("→ BENCH_RUNS=1 bench/scripts/run-bench.sh")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run benchmarks: %w", err)
	}

	return nil
}

// checkCompareDepends verifies required tools are installed.
func checkCompareDepends() error {
	tools := []struct {
		name    string
		check   string
		install string
	}{
		{"markdownlint", "markdownlint --version", "npm install -g markdownlint-cli"},
		{"jq", "jq --version", "brew install jq"},
		{"gnuplot", "gnuplot --version", "brew install gnuplot"},
	}

	// Check for GNU time
	hasGtime := exec.Command("which", "gtime").Run() == nil
	hasGnuTime := false
	if !hasGtime {
		out, _ := exec.Command("/usr/bin/time", "--version").CombinedOutput()
		hasGnuTime = strings.Contains(string(out), "GNU")
	}
	if !hasGtime && !hasGnuTime {
		return fmt.Errorf("GNU time required. Install with: brew install gnu-time")
	}

	for _, tool := range tools {
		if err := exec.Command("sh", "-c", tool.check).Run(); err != nil {
			return fmt.Errorf("%s not found. Install with: %s", tool.name, tool.install)
		}
	}

	return nil
}
```

**Step 2: Add aliases**

Update the `Aliases` map:

```go
var Aliases = map[string]interface{}{
	"b":     Build,
	"t":     Test,
	"l":     Lint,
	"c":     Check,
	"i":     Install,
	"cmp":   Compare,
	"cmpf":  CompareFast,
}
```

**Step 3: Verify it compiles**

```bash
go build -tags stave -o /dev/null stavefile.go
```

**Step 4: Commit**

```bash
git add stavefile.go
git commit -m "feat(bench): add Compare and CompareFast stave targets"
```

---

## Task 7: Add GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/compare.yml`

**Step 1: Write compare.yml**

Create `.github/workflows/compare.yml`:
```yaml
name: Performance Comparison

on:
  workflow_dispatch:
  schedule:
    - cron: '0 3 * * 0'  # Weekly on Sunday at 3am UTC

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

      - name: Install system dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y gnuplot jq

      - name: Install markdownlint-cli
        run: npm install -g markdownlint-cli

      - name: Cache benchmark repos
        uses: actions/cache@v4
        with:
          path: ~/.cache/gomdlint-compare
          key: compare-repos-v1

      - name: Build gomdlint
        run: go build -o gomdlint ./cmd/gomdlint

      - name: Add gomdlint to PATH
        run: echo "$PWD" >> $GITHUB_PATH

      - name: Clone benchmark repos
        run: ./bench/scripts/clone-repos.sh

      - name: Run comparison
        run: ./bench/scripts/run-bench.sh

      - name: Generate charts
        run: ./bench/scripts/generate-plots.sh

      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: comparison-results-${{ github.run_number }}
          path: bench/results/
          retention-days: 90
```

**Step 2: Commit**

```bash
git add .github/workflows/compare.yml
git commit -m "ci: add performance comparison workflow"
```

---

## Task 8: Test End-to-End

**Step 1: Install dependencies (if needed)**

```bash
# macOS
brew install gnu-time gnuplot jq
npm install -g markdownlint-cli
```

**Step 2: Run full comparison**

```bash
stave compare
```

Expected: Repos clone, benchmarks run, charts generate, terminal shows comparison table.

**Step 3: Verify outputs**

```bash
ls bench/results/raw/
ls bench/results/plots/
cat bench/results/latest.txt
```

**Step 4: Run quick comparison**

```bash
stave compareFast
```

Expected: Same flow but faster (1 run instead of 3).

---

## Task 9: Final Cleanup and PR

**Step 1: Run all checks**

```bash
stave check
```

**Step 2: Push branch**

```bash
git push -u origin feature/compare-harness
```

**Step 3: Create PR**

```bash
gh pr create --title "feat: add performance comparison harness" --body "$(cat <<'EOF'
## Summary
- Add benchmark harness comparing gomdlint vs markdownlint
- Shell scripts for cloning repos, running benchmarks, generating gnuplot charts
- Stave targets: `compare` (full) and `compareFast` (quick)
- GitHub Actions workflow for on-demand + weekly scheduled runs

## Test plan
- [ ] Run `stave compare` locally
- [ ] Verify charts generated in `bench/results/plots/`
- [ ] Trigger workflow manually in Actions tab

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
