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

FAILED_REPOS=()
SUCCESS_COUNT=0

# Read repos, skip comments and empty lines
while read -r repo; do
    repo_name=$(basename "$repo")
    repo_path="$REPOS_DIR/$repo_name"

    if [ -d "$repo_path" ]; then
        echo "Updating $repo..."
        git -C "$repo_path" pull --ff-only --depth 1 2>/dev/null || true
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "Cloning $repo..."
        if git clone --depth 1 "https://github.com/$repo.git" "$repo_path" 2>&1; then
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        else
            echo "  Warning: Failed to clone $repo (will skip in benchmark)"
            FAILED_REPOS+=("$repo")
        fi
    fi
done < <(grep -v '^#' "$REPOS_FILE" | grep -v '^$')

echo ""
if [ ${#FAILED_REPOS[@]} -gt 0 ]; then
    echo "Warning: ${#FAILED_REPOS[@]} repo(s) failed to clone: ${FAILED_REPOS[*]}"
fi

if [ "$SUCCESS_COUNT" -eq 0 ]; then
    echo "Error: No repos available for benchmarking"
    exit 1
fi

echo "$SUCCESS_COUNT repo(s) ready for benchmarking."
