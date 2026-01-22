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
