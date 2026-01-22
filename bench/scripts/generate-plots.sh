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
