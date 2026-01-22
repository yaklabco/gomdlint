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
GOMDLINT_VERSION=$(gomdlint version 2>/dev/null | head -1 || echo "unknown")
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
    local tmp_out=$(mktemp)

    # Build file list first
    local files
    files=$(find "$repo_path" -name "*.md" -type f)

    if [ -z "$files" ]; then
        echo "0 0"
        rm -f "$tmp_time" "$tmp_out"
        return
    fi

    # Run with GNU time, capture memory and time
    # shellcheck disable=SC2086
    $TIME_CMD -f '%e %M' -o "$tmp_time" $cmd $files >"$tmp_out" 2>&1 || true

    local time_sec mem_kb
    read -r time_sec mem_kb < "$tmp_time" || { time_sec=0; mem_kb=0; }
    rm -f "$tmp_time" "$tmp_out"

    # Handle empty or invalid values
    if [ -z "$time_sec" ] || [ "$time_sec" = "" ]; then
        time_sec=0
    fi
    if [ -z "$mem_kb" ] || [ "$mem_kb" = "" ]; then
        mem_kb=0
    fi

    # Convert to milliseconds (handle decimal)
    local time_ms
    time_ms=$(printf "%.0f" "$(echo "$time_sec * 1000" | bc)" 2>/dev/null || echo "0")
    echo "$time_ms $mem_kb"
}

# Function to get median of runs
median_run() {
    local cmd="$1"
    local repo_path="$2"
    local times_file=$(mktemp)
    local mems_file=$(mktemp)

    for ((i=1; i<=RUNS; i++)); do
        result=$(run_linter "$cmd" "$repo_path")
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
    local cmd="$1"
    local repo_path="$2"
    local files
    files=$(find "$repo_path" -name "*.md" -type f)

    if [ -z "$files" ]; then
        echo "0"
        return
    fi

    # shellcheck disable=SC2086
    $cmd $files 2>/dev/null | wc -l | tr -d ' '
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
