#!/usr/bin/env bash
set -euo pipefail

# Clone or update the benchmark repositories as shallow clones in
# ~/.cache/gomdlint-compare/repos/<org>__<repo> (GOMDLINT_COMPARE_CACHE
# moves the cache). BENCH_REPOS restricts it to some of the repositories in
# bench/repos.txt, for example BENCH_REPOS=DavidAnson/markdownlint.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=bench/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

# A command substitution, so that an error in the list stops the script.
repo_list=$(bench_repo_list)
REPOS=()
while read -r repo; do
    REPOS+=("$repo")
done <<<"$repo_list"
[ ${#REPOS[@]} -gt 0 ] || bench_die "no repositories selected"

mkdir -p "$BENCH_REPOS_DIR"

echo "Benchmark repo cache: $BENCH_REPOS_DIR"
echo ""

FAILED_REPOS=()

for repo in "${REPOS[@]}"; do
    repo_path=$(bench_repo_dir "$repo")

    if [ -d "$repo_path/.git" ]; then
        echo "Updating $repo..."
        if ! git -C "$repo_path" pull --ff-only --depth 1; then
            echo "  Warning: failed to update $repo"
            FAILED_REPOS+=("$repo")
        fi
    else
        echo "Cloning $repo..."
        if ! git clone --depth 1 "https://github.com/$repo.git" "$repo_path"; then
            echo "  Warning: failed to clone $repo"
            FAILED_REPOS+=("$repo")
        fi
    fi
done

echo ""
if [ ${#FAILED_REPOS[@]} -gt 0 ]; then
    # run-bench.sh benchmarks every selected repository, so a missing one
    # must stop the run here instead of silently shrinking the results.
    bench_die "${#FAILED_REPOS[@]} repo(s) could not be cloned or updated: ${FAILED_REPOS[*]}"
fi

echo "${#REPOS[@]} repo(s) ready for benchmarking."
