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
RUN_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
mkdir -p "$RESULTS_DIR/raw" "$RESULTS_DIR/plots"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
RESULT_FILE="$RESULTS_DIR/raw/$RUN_TIMESTAMP.json"
PROFILE_DIR="$RESULTS_DIR/profiles/$RUN_TIMESTAMP"
mkdir -p "$PROFILE_DIR"

# Get versions
GOMDLINT_VERSION=$(gomdlint version 2>/dev/null | grep -oE 'version=[^ ]+' | cut -d= -f2 || echo "unknown")
MARKDOWNLINT_VERSION=$(markdownlint --version 2>/dev/null || echo "unknown")

echo "gomdlint vs markdownlint comparison"
echo "===================================="
echo "gomdlint version: $GOMDLINT_VERSION"
echo "markdownlint version: $MARKDOWNLINT_VERSION"
echo "Runs per tool: $RUNS"
echo ""

# Function to run and time a linter
run_linter() {
    local linter="$1"
    local repo_path="$2"
    local repo_name="$3"
    local tmp_time=$(mktemp)
    local tmp_out=$(mktemp)

    # Run with GNU time
    # Note: gomdlint accepts directories, markdownlint needs file list via find
    if [ "$linter" = "gomdlint" ]; then
        # Create profile directory for this repo
        local profile_base="$PROFILE_DIR/$repo_name"
        mkdir -p "$profile_base"
        $TIME_CMD -f '%e %M' -o "$tmp_time" gomdlint lint \
            --cpuprofile="$profile_base/cpu.pprof" \
            --memprofile="$profile_base/mem.pprof" \
            --trace="$profile_base/trace.out" \
            "$repo_path" >"$tmp_out" 2>&1 || true
    else
        # markdownlint: use find to get recursive file list
        $TIME_CMD -f '%e %M' -o "$tmp_time" bash -c "find \"$repo_path\" -name '*.md' -type f -print0 | xargs -0 markdownlint" >"$tmp_out" 2>&1 || true
    fi

    local time_sec mem_kb time_ms
    if [ -s "$tmp_time" ]; then
        # GNU time writes timing on last line (error message on first line if command failed)
        read -r time_sec mem_kb < <(tail -1 "$tmp_time") || { time_sec=0; mem_kb=0; }
    else
        time_sec=0
        mem_kb=0
    fi
    rm -f "$tmp_time" "$tmp_out"

    # Handle empty values
    : "${time_sec:=0}"
    : "${mem_kb:=0}"

    # Convert to milliseconds (use awk for robustness)
    time_ms=$(echo "$time_sec" | awk '{printf "%.0f", $1 * 1000}' 2>/dev/null || echo "0")
    echo "$time_ms $mem_kb"
}

# Function to get median of runs
median_run() {
    local linter="$1"
    local repo_path="$2"
    local repo_name="$3"
    local times_file=$(mktemp)
    local mems_file=$(mktemp)

    for ((i=1; i<=RUNS; i++)); do
        result=$(run_linter "$linter" "$repo_path" "$repo_name")
        echo "$result" | cut -d' ' -f1 >> "$times_file"
        echo "$result" | cut -d' ' -f2 >> "$mems_file"
    done

    # Sort and get median
    local sorted_time sorted_mem
    sorted_time=$(sort -n "$times_file" | head -n $((RUNS / 2 + 1)) | tail -1)
    sorted_mem=$(sort -n "$mems_file" | head -n $((RUNS / 2 + 1)) | tail -1)

    rm -f "$times_file" "$mems_file"

    # Default to 0 if empty
    echo "${sorted_time:-0} ${sorted_mem:-0}"
}

# Count issues for a linter
count_issues() {
    local linter="$1"
    local repo_path="$2"

    # Use subshell to isolate pipefail (linters exit non-zero when they find issues)
    if [ "$linter" = "gomdlint" ]; then
        (gomdlint lint "$repo_path" 2>/dev/null || true) | wc -l | tr -d ' '
    else
        (find "$repo_path" -name '*.md' -type f -print0 | xargs -0 markdownlint 2>/dev/null || true) | wc -l | tr -d ' '
    fi
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
    gomdlint_result=$(median_run "gomdlint" "$repo_path" "$repo_name")
    gomdlint_time=$(echo "$gomdlint_result" | cut -d' ' -f1)
    gomdlint_mem=$(echo "$gomdlint_result" | cut -d' ' -f2)
    gomdlint_issues=$(count_issues "gomdlint" "$repo_path")

    markdownlint_result=$(median_run "markdownlint" "$repo_path" "$repo_name")
    markdownlint_time=$(echo "$markdownlint_result" | cut -d' ' -f1)
    markdownlint_mem=$(echo "$markdownlint_result" | cut -d' ' -f2)
    markdownlint_issues=$(count_issues "markdownlint" "$repo_path")

    # Ensure numeric values (default to 0)
    gomdlint_time=${gomdlint_time:-0}
    gomdlint_mem=${gomdlint_mem:-0}
    markdownlint_time=${markdownlint_time:-0}
    markdownlint_mem=${markdownlint_mem:-0}

    # Calculate speedup (use awk for robustness)
    if [ "${gomdlint_time:-0}" -gt 0 ] 2>/dev/null; then
        speedup=$(awk "BEGIN {printf \"%.1f\", $markdownlint_time / $gomdlint_time}")
    else
        speedup="N/A"
    fi

    # Format times for display
    gomdlint_display=$(awk "BEGIN {printf \"%.2fs\", ${gomdlint_time:-0} / 1000}")
    markdownlint_display=$(awk "BEGIN {printf \"%.2fs\", ${markdownlint_time:-0} / 1000}")

    printf "%-30s %8d %12s %12s %7sx\n" "$repo_name" "$file_count" "$gomdlint_display" "$markdownlint_display" "$speedup"

    # Add to JSON
    if [ "$first_repo" = true ]; then
        first_repo=false
    else
        echo "," >> "$RESULT_FILE"
    fi

    # Ensure issue counts are numeric
    gomdlint_issues=${gomdlint_issues:-0}
    markdownlint_issues=${markdownlint_issues:-0}

    cat >> "$RESULT_FILE" <<EOF
    "$repo_name": {
      "file_count": $file_count,
      "gomdlint": { "time_ms": ${gomdlint_time:-0}, "memory_kb": ${gomdlint_mem:-0}, "issues": ${gomdlint_issues:-0} },
      "markdownlint": { "time_ms": ${markdownlint_time:-0}, "memory_kb": ${markdownlint_mem:-0}, "issues": ${markdownlint_issues:-0} }
    }
EOF

    if [ "$speedup" != "N/A" ]; then
        total_speedup=$(awk "BEGIN {print $total_speedup + $speedup}")
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
    avg_speedup=$(awk "BEGIN {printf \"%.1f\", $total_speedup / $repo_count}")
    echo "Average speedup: ${avg_speedup}x"
fi

echo ""
echo "Results saved to: $RESULT_FILE"
echo "Profiles saved to: $PROFILE_DIR"

# Update latest symlink
echo "$RESULT_FILE" > "$RESULTS_DIR/latest.txt"
