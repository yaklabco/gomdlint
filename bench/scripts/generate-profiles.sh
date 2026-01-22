#!/usr/bin/env bash
set -euo pipefail

# Generate reports from profile data
# Usage: generate-profiles.sh <profile-dir>

PROFILE_DIR="${1:-}"
if [ -z "$PROFILE_DIR" ]; then
    echo "Usage: generate-profiles.sh <profile-dir>"
    exit 1
fi

if [ ! -d "$PROFILE_DIR" ]; then
    echo "Error: Profile directory not found: $PROFILE_DIR"
    exit 1
fi

echo "Generating profile reports from: $PROFILE_DIR"
echo ""

# Process each repo's profiles
for repo_dir in "$PROFILE_DIR"/*/; do
    [ -d "$repo_dir" ] || continue
    repo_name=$(basename "$repo_dir")

    # Skip combined directory
    [ "$repo_name" = "combined" ] && continue

    echo "Processing $repo_name..."

    # Generate CPU flamegraph
    if [ -f "$repo_dir/cpu.pprof" ]; then
        go tool pprof -svg "$repo_dir/cpu.pprof" > "$repo_dir/cpu-flamegraph.svg" 2>/dev/null || true
    fi

    # Generate memory top functions
    if [ -f "$repo_dir/mem.pprof" ]; then
        go tool pprof -top "$repo_dir/mem.pprof" > "$repo_dir/mem-top.txt" 2>/dev/null || true
    fi

    # Generate summary
    {
        echo "Profile Summary: $repo_name"
        echo "=============================="
        echo ""
        if [ -f "$repo_dir/cpu.pprof" ]; then
            echo "Top 10 CPU functions:"
            go tool pprof -top -nodecount=10 "$repo_dir/cpu.pprof" 2>/dev/null | tail -n +5 || true
            echo ""
        fi
        if [ -f "$repo_dir/mem.pprof" ]; then
            echo "Top 10 memory allocations:"
            go tool pprof -top -nodecount=10 "$repo_dir/mem.pprof" 2>/dev/null | tail -n +5 || true
        fi
    } > "$repo_dir/summary.txt"
done

# Create combined profiles
echo "Creating combined profiles..."
COMBINED_DIR="$PROFILE_DIR/combined"
mkdir -p "$COMBINED_DIR"

# Merge CPU profiles
cpu_profiles=()
for f in "$PROFILE_DIR"/*/cpu.pprof; do
    [ -f "$f" ] && cpu_profiles+=("$f")
done

if [ ${#cpu_profiles[@]} -gt 0 ]; then
    go tool pprof -proto "${cpu_profiles[@]}" > "$COMBINED_DIR/cpu.pprof" 2>/dev/null || true
    if [ -f "$COMBINED_DIR/cpu.pprof" ]; then
        go tool pprof -svg "$COMBINED_DIR/cpu.pprof" > "$COMBINED_DIR/cpu-flamegraph.svg" 2>/dev/null || true
    fi
fi

# Print summary to terminal
echo ""
echo "=============================="
echo "Profile Report Complete"
echo "=============================="
echo ""

if [ -f "$COMBINED_DIR/cpu.pprof" ]; then
    echo "Top CPU hotspots (combined):"
    go tool pprof -top -nodecount=10 "$COMBINED_DIR/cpu.pprof" 2>/dev/null | tail -n +5 || true
    echo ""
fi

echo "Flamegraphs: $COMBINED_DIR/cpu-flamegraph.svg"
echo "Per-repo profiles: $PROFILE_DIR/<repo>/summary.txt"
