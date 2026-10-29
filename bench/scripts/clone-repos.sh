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

# update_repo moves a cached clone to the newest commit of its remote's
# default branch. A shallow clone lacks the history that a fast-forward
# needs once upstream has moved on, so it fetches the new tip alone and
# resets the checkout to it. The clone is disposable: local changes and
# untracked files go.
update_repo() {
    git -C "$1" fetch --depth 1 origin HEAD &&
        git -C "$1" reset --hard FETCH_HEAD &&
        git -C "$1" clean -ffdx
}

for repo in "${REPOS[@]}"; do
    repo_path=$(bench_repo_dir "$repo")

    if [ -d "$repo_path/.git" ]; then
        echo "Updating $repo..."
        if ! update_repo "$repo_path"; then
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
