#!/usr/bin/env bash
set -euo pipefail

# Benchmark gomdlint against markdownlint-cli on the repositories in
# bench/repos.txt, cloned by clone-repos.sh. Writes a JSON result file and
# prints a summary. Any linter failure stops the run with an error, so a
# result file only ever holds complete measurements.
#
# Environment:
#   GOMDLINT_BIN       gomdlint binary under test (default: gomdlint on PATH)
#   MARKDOWNLINT_BIN   markdownlint-cli binary (default: markdownlint on PATH)
#   BENCH_RUNS         timed runs per tool and repository, median kept (default: 3)
#   BENCH_REPOS        org/repo entries from repos.txt to run, separated by
#                      spaces or commas (default: all of them)
#   BENCH_GOMAXPROCS   GOMAXPROCS and --jobs for gomdlint (default: 8)
#   BENCH_PROFILE      1 to write gomdlint profiles in an extra, untimed run (default: 1)
#   BENCH_RESULTS_DIR  where results go (default: bench/results)
#   GOMDLINT_COMPARE_CACHE  repository cache (default: ~/.cache/gomdlint-compare)
#
# Both tools lint the same files. gomdlint lints the repository directory,
# as it is usually run, and so walks it and reads its .gitignore files.
# markdownlint lints the list of the Markdown files (.md and .markdown, in
# any case) that git tracks, without the hidden files and directories and
# the node_modules and vendor directories that gomdlint skips. The script
# fails unless gomdlint linted exactly the files on that list. Both run
# from an empty directory with an empty HOME, so neither reads a config
# file from the repository or the user, and both are told to ignore the
# config files in the directories above, so both use their default rules.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=bench/scripts/lib.sh
source "$SCRIPT_DIR/lib.sh"

RESULTS_DIR="${BENCH_RESULTS_DIR:-$SCRIPT_DIR/../results}"
# gomdlint runs from another directory and writes its profiles here, so the
# path must not be relative to this one.
case "$RESULTS_DIR" in
    /*) ;;
    *) RESULTS_DIR="$PWD/$RESULTS_DIR" ;;
esac
RUNS="${BENCH_RUNS:-3}"
PROCS="${BENCH_GOMAXPROCS:-8}"
PROFILE="${BENCH_PROFILE:-1}"
GOMDLINT_BIN="${GOMDLINT_BIN:-gomdlint}"
MARKDOWNLINT_BIN="${MARKDOWNLINT_BIN:-markdownlint}"

[[ "$RUNS" =~ ^[1-9][0-9]*$ ]] || bench_die "BENCH_RUNS must be a positive integer, not '$RUNS'"
[[ "$PROCS" =~ ^[1-9][0-9]*$ ]] || bench_die "BENCH_GOMAXPROCS must be a positive integer, not '$PROCS'"
[[ "$PROFILE" =~ ^[01]$ ]] || bench_die "BENCH_PROFILE must be 0 or 1, not '$PROFILE'"

# resolve_bin prints the absolute path of a command, since the linters run
# from another directory.
resolve_bin() {
    local path
    path=$(command -v "$1") || bench_die "$2 not found: $1"
    case "$path" in
        /*) ;;
        *) path="$PWD/$path" ;;
    esac
    printf '%s\n' "$path"
}

GOMDLINT_BIN=$(resolve_bin "$GOMDLINT_BIN" "gomdlint (set GOMDLINT_BIN)")
MARKDOWNLINT_BIN=$(resolve_bin "$MARKDOWNLINT_BIN" \
    "markdownlint (set MARKDOWNLINT_BIN, or npm install --prefix <dir> markdownlint-cli)")
command -v jq >/dev/null || bench_die "jq not found"
command -v git >/dev/null || bench_die "git not found"

# Settings in the environment or in system config files would change the
# rules a linter runs, so neither tool may see any. markdownlint-cli's
# run-con matches its markdownlint_ prefix in any case, so both prefixes
# are matched that way. export -n keeps the script's own variables, such
# as GOMDLINT_BIN.
exported=$(compgen -e)
shopt -s nocasematch
while read -r name; do
    if [[ "$name" == gomdlint_* || "$name" == markdownlint_* ]]; then
        # shellcheck disable=SC2163 # name holds the name of the variable
        export -n "$name"
    fi
done <<<"$exported"
shopt -u nocasematch
for path in /etc/gomdlint /etc/markdownlint /etc/markdownlintrc; do
    [ ! -e "$path" ] || bench_die "$path would configure a linter; the benchmark needs default rules"
done

# Memory needs GNU time, whose -f %M reports the peak resident set size.
# Without it, wall time comes from the shell's clock and memory is not
# measured.
GNU_TIME=""
for candidate in gtime "$(type -P time || true)"; do
    [ -n "$candidate" ] || continue
    if "$candidate" --version 2>&1 | grep -q GNU; then
        GNU_TIME=$(command -v "$candidate")
        break
    fi
done

# now_us sets NOW_US to the current time in microseconds, without a
# subshell when the shell has EPOCHREALTIME (bash 5).
now_us() {
    if [ -n "${EPOCHREALTIME:-}" ]; then
        NOW_US=${EPOCHREALTIME/[.,]/}
    else
        NOW_US=$(date +%s%N)
        [[ "$NOW_US" =~ ^[0-9]+$ ]] || bench_die "no clock with sub-second resolution: use bash 5 or GNU date"
        NOW_US=$((NOW_US / 1000))
    fi
}
now_us

WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/gomdlint-bench.XXXXXX")
trap 'rm -rf "$WORK_DIR"' EXIT
LINT_CWD="$WORK_DIR/cwd"
LINT_HOME="$WORK_DIR/home"
mkdir -p "$LINT_CWD" "$LINT_HOME"
LINT_ENV=(env HOME="$LINT_HOME" XDG_CONFIG_HOME="$LINT_HOME/.config")

# An empty working directory is not enough: gomdlint looks for a project
# config in the directories above it too, and markdownlint-cli, through
# run-con, for a .markdownlintrc. gomdlint gets an empty config file with
# --config, which takes the place of the project config. markdownlint-cli
# gets --configPointer naming a key that no config file holds, so its rules
# get an empty configuration, the defaults, whatever files run-con finds.
LINT_GOMDLINT_CONFIG="$WORK_DIR/gomdlint.yml"
: >"$LINT_GOMDLINT_CONFIG"
LINT_MARKDOWNLINT_POINTER=/gomdlint-bench-default-rules

# Versions
GOMDLINT_VERSION=$("$GOMDLINT_BIN" version) || bench_die "$GOMDLINT_BIN version failed"
GOMDLINT_REVISION=$(go version -m "$GOMDLINT_BIN" 2>/dev/null |
    awk -F'\t' '$2 == "build" && $3 ~ /^vcs.revision=/ { sub(/^vcs.revision=/, "", $3); print $3 }' || true)
MARKDOWNLINT_VERSION=$("$MARKDOWNLINT_BIN" --version) || bench_die "$MARKDOWNLINT_BIN --version failed"
MARKDOWNLINT_LIB_VERSION="unknown"
cli_dir=$(dirname "$(readlink -f "$MARKDOWNLINT_BIN")")
for pkg in "$cli_dir/node_modules/markdownlint/package.json" "$cli_dir/../markdownlint/package.json"; do
    if [ -f "$pkg" ]; then
        MARKDOWNLINT_LIB_VERSION=$(jq -r .version "$pkg")
        break
    fi
done
NODE_VERSION=$(node --version 2>/dev/null || echo "unknown")

# A command substitution, so that an error in the list stops the script.
repo_list=$(bench_repo_list)
REPOS=()
while read -r repo; do
    REPOS+=("$repo")
done <<<"$repo_list"
[ ${#REPOS[@]} -gt 0 ] || bench_die "no repositories selected"

RUN_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
RESULT_FILE="$RESULTS_DIR/raw/$RUN_TIMESTAMP.json"
PROFILE_DIR="$RESULTS_DIR/profiles/$RUN_TIMESTAMP"

echo "gomdlint vs markdownlint comparison"
echo "===================================="
echo "gomdlint:         $GOMDLINT_VERSION"
echo "  binary:         $GOMDLINT_BIN"
echo "  revision:       ${GOMDLINT_REVISION:-unknown}"
echo "  GOMAXPROCS:     $PROCS (--jobs $PROCS)"
echo "markdownlint-cli: $MARKDOWNLINT_VERSION (markdownlint $MARKDOWNLINT_LIB_VERSION, node $NODE_VERSION)"
echo "  binary:         $MARKDOWNLINT_BIN"
if [ -n "$GNU_TIME" ]; then
    echo "Timing:           wall clock; peak memory from $GNU_TIME"
else
    echo "Timing:           wall clock; memory unavailable (no GNU time)"
fi
echo "Runs per tool:    $RUNS (median)"
echo "Repositories:     ${REPOS[*]}"
echo ""

# timed_run runs a command from LINT_CWD with its output in OUT_FILE and
# ERR_FILE, and sets RUN_MS, RUN_KB (empty without GNU time) and RUN_EXIT.
OUT_FILE="$WORK_DIR/out"
ERR_FILE="$WORK_DIR/err"
TIME_FILE="$WORK_DIR/time"
timed_run() {
    local start status=0
    RUN_KB=""
    now_us
    start=$NOW_US
    if [ -n "$GNU_TIME" ]; then
        (cd "$LINT_CWD" && "$GNU_TIME" -f '%M' -o "$TIME_FILE" "$@") >"$OUT_FILE" 2>"$ERR_FILE" || status=$?
    else
        (cd "$LINT_CWD" && "$@") >"$OUT_FILE" 2>"$ERR_FILE" || status=$?
    fi
    now_us
    RUN_MS=$(((NOW_US - start + 500) / 1000))
    RUN_EXIT=$status
    if [ -n "$GNU_TIME" ]; then
        # GNU time puts "Command exited with non-zero status" first.
        RUN_KB=$(tail -n 1 "$TIME_FILE")
        [[ "$RUN_KB" =~ ^[0-9]+$ ]] || bench_die "GNU time wrote no peak memory: $(cat "$TIME_FILE")"
    fi
}

# fail_run stops the benchmark after a linter failed, showing its output.
fail_run() {
    echo "" >&2
    echo "Error: $1" >&2
    echo "--- stderr (last 20 lines) ---" >&2
    tail -n 20 "$ERR_FILE" >&2 || true
    echo "--- stdout (first 20 lines) ---" >&2
    head -n 20 "$OUT_FILE" >&2 || true
    exit 1
}

# median prints the median of its arguments (the upper one for an even count).
median() {
    printf '%s\n' "$@" | sort -n | sed -n "$(($# / 2 + 1))p"
}

# Exit codes that mean the linter ran to the end: 0 for no errors and 1 for
# errors found, for both gomdlint (internal/cli/exitcodes.go; 2 needs
# --strict) and markdownlint-cli. Anything else is a failure.
run_gomdlint() {
    timed_run "${LINT_ENV[@]}" GOMAXPROCS="$PROCS" "$GOMDLINT_BIN" lint \
        --config "$LINT_GOMDLINT_CONFIG" --format json --jobs "$PROCS" "$@" -- "$REPO_REAL"
    case "$RUN_EXIT" in
        0 | 1) ;;
        *) fail_run "gomdlint failed on $repo with exit status $RUN_EXIT" ;;
    esac
    local summary
    summary=$(jq -e -c '.summary
        | select([.filesChecked, .totalIssues, .filesWithIssues, .filesErrored, .ruleErrors]
            | all(type == "number"))' "$OUT_FILE" 2>/dev/null) ||
        fail_run "gomdlint wrote no JSON report with the summary this script reads for $repo"
    G_FILES=$(jq -r '.filesChecked' <<<"$summary")
    G_ISSUES=$(jq -r '.totalIssues' <<<"$summary")
    G_FILES_WITH=$(jq -r '.filesWithIssues' <<<"$summary")
    local errored rule_errors
    errored=$(jq -r '.filesErrored' <<<"$summary")
    rule_errors=$(jq -r '.ruleErrors' <<<"$summary")
    [ "$G_FILES" -gt 0 ] || fail_run "gomdlint linted no files in $repo"
    [ "$errored" -eq 0 ] || fail_run "gomdlint could not lint $errored files in $repo"
    [ "$rule_errors" -eq 0 ] || fail_run "$rule_errors gomdlint rule failures in $repo"

    # The report lists every file linted, by its absolute path, as the
    # repository lies outside the working directory.
    jq -r --arg root "$REPO_REAL/" '.files[].path | ltrimstr($root)' "$OUT_FILE" |
        LC_ALL=C sort >"$WORK_DIR/linted.list"
    if ! cmp -s "$WORK_DIR/expected.list" "$WORK_DIR/linted.list"; then
        echo "Files markdownlint gets (<) and files gomdlint linted (>), first 20 differences:" >&2
        diff "$WORK_DIR/expected.list" "$WORK_DIR/linted.list" | grep '^[<>]' | head -n 20 >&2 || true
        fail_run "gomdlint linted $G_FILES files in $repo, not the $FILE_COUNT on the list markdownlint gets"
    fi
}

run_markdownlint() {
    # The report must come from this run, so that a run that stops before
    # writing one cannot pass for a run without findings.
    rm -f "$OUT_FILE.json"
    timed_run "${LINT_ENV[@]}" "$MARKDOWNLINT_BIN" --json --output "$OUT_FILE.json" \
        --configPointer "$LINT_MARKDOWNLINT_POINTER" -- "${FILES[@]}"
    case "$RUN_EXIT" in
        0 | 1) ;;
        *) fail_run "markdownlint failed on $repo with exit status $RUN_EXIT" ;;
    esac
    # markdownlint-cli writes the report even when it finds nothing, as an
    # empty file. It lints every file it is given, as none is a directory or
    # a glob and the working directory has no .markdownlintignore.
    [ -f "$OUT_FILE.json" ] ||
        fail_run "markdownlint wrote no report for $repo (exit status $RUN_EXIT)"
    if [ -s "$OUT_FILE.json" ]; then
        M_ISSUES=$(jq -e 'if type == "array" then length else error("not an array") end' "$OUT_FILE.json") ||
            fail_run "markdownlint wrote no JSON report for $repo"
        M_FILES_WITH=$(jq '[.[].fileName] | unique | length' "$OUT_FILE.json")
    else
        M_ISSUES=0
        M_FILES_WITH=0
    fi
    rm -f "$OUT_FILE.json"
    # markdownlint-cli exits 1 for findings, and also when it rejects its
    # command line, so exit 1 must come with findings.
    if [ "$RUN_EXIT" -eq 1 ] && [ "$M_ISSUES" -eq 0 ]; then
        fail_run "markdownlint exited with status 1 and reported no findings for $repo"
    fi
    M_FILES=$FILE_COUNT
}

# The Markdown files, as git pathspecs. gomdlint matches the .md and
# .markdown extensions in any case, so the pathspecs do too.
MARKDOWN_PATHSPECS=(':(icase)*.md' ':(icase)*.markdown')

# prepare_repo sets repo_path and REPO_REAL for the repository in $repo,
# points the "repo" link in LINT_CWD at it, and sets FILES to its Markdown
# files as paths through that link, which keeps markdownlint's command line
# short. It also writes the sorted list of the files to expected.list.
prepare_repo() {
    repo_path=$(bench_repo_dir "$repo")
    [ -d "$repo_path/.git" ] || bench_die "$repo is not cloned at $repo_path; run clone-repos.sh"
    REPO_REAL=$(cd "$repo_path" && pwd -P)
    ln -sfn "$REPO_REAL" "$LINT_CWD/repo"

    # The tracked files, and those of them that a .gitignore matches, which
    # gomdlint skips. gomdlint reads only .gitignore files, not the global
    # excludes file or .git/info/exclude, so neither may drop a file from
    # the list. Both lists are NUL-separated, as git quotes unusual names
    # in its other output, and go through files, so that a git failure
    # stops the script.
    local tracked_list="$WORK_DIR/tracked.list0" ignored_list="$WORK_DIR/ignored.list0"
    git -C "$REPO_REAL" ls-files -z -- "${MARKDOWN_PATHSPECS[@]}" >"$tracked_list"
    git -C "$REPO_REAL" ls-files -z -c -i --exclude-per-directory=.gitignore \
        -- "${MARKDOWN_PATHSPECS[@]}" >"$ignored_list"

    local file ignored=() next=0
    while IFS= read -r -d '' file; do
        ignored+=("$file")
    done <"$ignored_list"

    # git lists both in the order of its index, so the ignored files come
    # up in the tracked list in their own order, and one pass finds them.
    local links=()
    FILES=()
    while IFS= read -r -d '' file; do
        if [ "$next" -lt "${#ignored[@]}" ] && [ "$file" = "${ignored[$next]}" ]; then
            next=$((next + 1))
            continue
        fi
        case "/$file" in
            */.* | */node_modules/* | */vendor/*) continue ;;
        esac
        # gomdlint lints a link to a file, and skips a broken link.
        if [ ! -f "$REPO_REAL/$file" ]; then
            continue
        elif [ -L "$REPO_REAL/$file" ]; then
            links+=("$file")
        else
            FILES+=("repo/$file")
        fi
    done <"$tracked_list"
    [ "$next" -eq "${#ignored[@]}" ] ||
        bench_die "git listed ${ignored[$next]} of $repo as ignored, but not in the order of the tracked files"

    # gomdlint lints a file once however many of its paths it finds. Of a
    # file and the links to it, it keeps the file, and of links alone, the
    # first in sorted order (dropAliases in pkg/runner/discovery.go). git
    # lists the links in that order. A tracked file never lies below a
    # link, so the path of a file on the list is its real path.
    local regular=${#FILES[@]} targets=() target alias i j
    for ((i = 0; i < ${#links[@]}; i++)); do
        target=$(readlink -f -- "$REPO_REAL/${links[$i]}") ||
            bench_die "cannot resolve the link ${links[$i]} in $repo"
        targets+=("$target")
        alias=0
        if [[ "$target" == "$REPO_REAL/"* ]]; then
            for ((j = 0; j < regular; j++)); do
                if [ "${FILES[$j]}" = "repo/${target#"$REPO_REAL/"}" ]; then
                    alias=1
                    break
                fi
            done
        fi
        for ((j = 0; j < i; j++)); do
            [ "${targets[$j]}" != "$target" ] || alias=1
        done
        [ "$alias" -eq 1 ] || FILES+=("repo/${links[$i]}")
    done
    FILE_COUNT=${#FILES[@]}
    [ "$FILE_COUNT" -gt 0 ] || bench_die "$repo has no Markdown files"
    printf '%s\n' "${FILES[@]#repo/}" | LC_ALL=C sort >"$WORK_DIR/expected.list"

    local arg_bytes env_bytes limit
    arg_bytes=$(printf '%s\0' "${FILES[@]}" | wc -c)
    env_bytes=$(env | wc -c)
    limit=$(($(getconf ARG_MAX) - env_bytes - 65536))
    [ "$arg_bytes" -lt "$limit" ] ||
        bench_die "the $FILE_COUNT file paths of $repo need $arg_bytes bytes, over the $limit the command line allows"
}

REPO_RESULTS="$WORK_DIR/repos.jsonl"
: >"$REPO_RESULTS"

# Check every repository with one untimed gomdlint run before timing any,
# so that a failure or a file set that differs stops the run in minutes
# instead of after hours of markdownlint runs.
echo "Checking that both tools get the same files..."
for repo in "${REPOS[@]}"; do
    prepare_repo
    run_gomdlint
    printf "  %-26s %7d files\n" "$repo" "$FILE_COUNT"
done
echo ""

printf "%-26s %7s %10s %13s %8s %10s %13s\n" \
    "Repository" "Files" "gomdlint" "markdownlint" "Speedup" "gomdlint" "markdownlint"
printf "%-26s %7s %10s %13s %8s %10s %13s\n" \
    "" "" "time" "time" "" "findings" "findings"
printf "%-26s %7s %10s %13s %8s %10s %13s\n" \
    "----------" "-----" "--------" "------------" "-------" "--------" "------------"

speedups=()

for repo in "${REPOS[@]}"; do
    prepare_repo

    g_times=() g_mems=() m_times=() m_mems=()
    for ((i = 1; i <= RUNS; i++)); do
        run_gomdlint
        g_times+=("$RUN_MS")
        g_mems+=("$RUN_KB")
        if [ "$i" -gt 1 ] && [ "$G_ISSUES" != "$g_first_issues" ]; then
            bench_die "gomdlint found $g_first_issues and then $G_ISSUES issues in $repo"
        fi
        g_first_issues=$G_ISSUES

        run_markdownlint
        m_times+=("$RUN_MS")
        m_mems+=("$RUN_KB")
        if [ "$i" -gt 1 ] && [ "$M_ISSUES" != "$m_first_issues" ]; then
            bench_die "markdownlint found $m_first_issues and then $M_ISSUES issues in $repo"
        fi
        m_first_issues=$M_ISSUES
    done

    g_time=$(median "${g_times[@]}")
    m_time=$(median "${m_times[@]}")
    g_mem=null
    m_mem=null
    if [ -n "$GNU_TIME" ]; then
        g_mem=$(median "${g_mems[@]}")
        m_mem=$(median "${m_mems[@]}")
    fi

    speedup=$(awk -v m="$m_time" -v g="$g_time" 'BEGIN { if (g > 0) printf "%.1f", m / g; else print "N/A" }')
    [ "$speedup" = "N/A" ] || speedups+=("$speedup")

    printf "%-26s %7d %10s %13s %7sx %10d %13d\n" "$repo" "$FILE_COUNT" \
        "$(awk -v t="$g_time" 'BEGIN { printf "%.2fs", t / 1000 }')" \
        "$(awk -v t="$m_time" 'BEGIN { printf "%.2fs", t / 1000 }')" \
        "$speedup" "$G_ISSUES" "$M_ISSUES"
    printf "  files seen: gomdlint %d, markdownlint %d; files with findings: gomdlint %d, markdownlint %d\n" \
        "$G_FILES" "$M_FILES" "$G_FILES_WITH" "$M_FILES_WITH"
    if [ -n "$GNU_TIME" ]; then
        printf "  peak memory: gomdlint %d KB, markdownlint %d KB\n" "$g_mem" "$m_mem"
    fi

    jq -n -c \
        --arg repo "$repo" \
        --argjson file_count "$FILE_COUNT" \
        --argjson g_time "$g_time" --argjson g_mem "$g_mem" \
        --argjson g_issues "$G_ISSUES" --argjson g_files "$G_FILES" --argjson g_with "$G_FILES_WITH" \
        --argjson m_time "$m_time" --argjson m_mem "$m_mem" \
        --argjson m_issues "$M_ISSUES" --argjson m_files "$M_FILES" --argjson m_with "$M_FILES_WITH" \
        --argjson g_runs "$(printf '%s\n' "${g_times[@]}" | jq -s -c .)" \
        --argjson m_runs "$(printf '%s\n' "${m_times[@]}" | jq -s -c .)" \
        '{($repo): {
            file_count: $file_count,
            gomdlint: {time_ms: $g_time, memory_kb: $g_mem, issues: $g_issues,
                       files_linted: $g_files, files_with_issues: $g_with, run_times_ms: $g_runs},
            markdownlint: {time_ms: $m_time, memory_kb: $m_mem, issues: $m_issues,
                           files_linted: $m_files, files_with_issues: $m_with, run_times_ms: $m_runs}
        }}' >>"$REPO_RESULTS"

    if [ "$PROFILE" = 1 ]; then
        repo_profile="$PROFILE_DIR/${repo//\//__}"
        mkdir -p "$repo_profile"
        run_gomdlint --cpuprofile="$repo_profile/cpu.pprof" \
            --memprofile="$repo_profile/mem.pprof" --trace="$repo_profile/trace.out"
        for profile in cpu.pprof mem.pprof trace.out; do
            [ -s "$repo_profile/$profile" ] || bench_die "gomdlint wrote no $profile for $repo"
        done
    fi
done

rm -f "$LINT_CWD/repo"

mkdir -p "$RESULTS_DIR/raw"
jq -n \
    --arg timestamp "$TIMESTAMP" \
    --arg gomdlint_version "$GOMDLINT_VERSION" \
    --arg gomdlint_revision "${GOMDLINT_REVISION:-unknown}" \
    --arg markdownlint_version "$MARKDOWNLINT_VERSION" \
    --arg markdownlint_library_version "$MARKDOWNLINT_LIB_VERSION" \
    --arg node_version "$NODE_VERSION" \
    --arg platform "$(uname -sm)" \
    --argjson cpus "$(getconf _NPROCESSORS_ONLN)" \
    --argjson gomaxprocs "$PROCS" \
    --arg memory "$([ -n "$GNU_TIME" ] && echo gnu-time || echo unavailable)" \
    --argjson runs "$RUNS" \
    --slurpfile repos "$REPO_RESULTS" \
    '{timestamp: $timestamp, gomdlint_version: $gomdlint_version,
      gomdlint_revision: $gomdlint_revision, markdownlint_version: $markdownlint_version,
      markdownlint_library_version: $markdownlint_library_version, node_version: $node_version,
      platform: $platform, cpus: $cpus, gomaxprocs: $gomaxprocs, timing: "wall-clock",
      memory: $memory, runs: $runs, repos: ($repos | add)}' >"$RESULT_FILE"

echo ""
if [ ${#speedups[@]} -gt 0 ]; then
    avg_speedup=$(printf '%s\n' "${speedups[@]}" | awk '{ s += $1 } END { printf "%.1f", s / NR }')
    echo "Average speedup: ${avg_speedup}x"
fi

echo ""
echo "Results saved to: $RESULT_FILE"
echo "$RESULT_FILE" >"$RESULTS_DIR/latest.txt"

if [ "$PROFILE" = 1 ]; then
    echo "Profiles saved to: $PROFILE_DIR"
    echo ""
    "$SCRIPT_DIR/generate-profiles.sh" "$PROFILE_DIR"
fi
