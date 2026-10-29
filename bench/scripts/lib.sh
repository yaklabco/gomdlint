# shellcheck shell=bash
# Shared helpers for the benchmark scripts. Source it, do not run it.

BENCH_CACHE_DIR="${GOMDLINT_COMPARE_CACHE:-$HOME/.cache/gomdlint-compare}"
BENCH_REPOS_DIR="$BENCH_CACHE_DIR/repos"
BENCH_REPOS_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/repos.txt"

# bench_die prints an error to stderr and exits.
bench_die() {
    echo "Error: $*" >&2
    exit 1
}

# bench_repo_list prints the repositories to benchmark, one org/repo per
# line. BENCH_REPOS, a list of org/repo separated by spaces or commas,
# restricts the run to those repositories. Each must be listed in
# repos.txt, so that a typo fails instead of cloning something else.
bench_repo_list() {
    local listed
    listed=$(grep -v '^[[:space:]]*#' "$BENCH_REPOS_FILE" | grep -v '^[[:space:]]*$' | tr -d '[:blank:]') ||
        bench_die "no repositories in $BENCH_REPOS_FILE"

    if [ -z "${BENCH_REPOS:-}" ]; then
        printf '%s\n' "$listed"
        return
    fi

    local repo
    for repo in ${BENCH_REPOS//,/ }; do
        grep -qxF "$repo" <<<"$listed" ||
            bench_die "BENCH_REPOS names $repo, which is not in $BENCH_REPOS_FILE"
        printf '%s\n' "$repo"
    done
}

# bench_repo_dir prints the clone directory of an org/repo. The org is part
# of the name because several repositories share a name, such as
# kubernetes/website and golang/website.
bench_repo_dir() {
    printf '%s/%s\n' "$BENCH_REPOS_DIR" "${1//\//__}"
}
