# Performance: gomdlint vs markdownlint

This document compares the time gomdlint and markdownlint-cli take to lint
the Markdown of five open-source repositories, and describes how the
comparison is run.

## Summary

Measured on 2026-10-29 with gomdlint at `eb70b8b` and markdownlint-cli
0.49.0 (markdownlint 0.41.1), both with their default rules:

| Repository | Files | gomdlint | markdownlint | Speedup |
| --- | ---: | ---: | ---: | ---: |
| kubernetes/website | 8,339 | 13.01 s | 75.49 s | 5.8x |
| microsoft/vscode-docs | 842 | 3.35 s | 14.38 s | 4.3x |
| facebook/react | 2,005 | 3.27 s | 39.18 s | 12.0x |
| rust-lang/book | 476 | 0.77 s | 3.25 s | 4.2x |
| DavidAnson/markdownlint | 554 | 0.42 s | 1.72 s | 4.1x |

gomdlint took from a quarter to a twelfth of markdownlint's time, 6.1 times
less on average over the five repositories. gomdlint ran with 8 workers.
markdownlint-cli lints its files one after another, so part of the
difference is parallelism.

Both tools linted the same files and found about as many problems:

| Repository | gomdlint findings | markdownlint findings |
| --- | ---: | ---: |
| kubernetes/website | 479,269 | 473,574 |
| microsoft/vscode-docs | 60,640 | 60,626 |
| facebook/react | 43,367 | 43,367 |
| rust-lang/book | 4,641 | 4,643 |
| DavidAnson/markdownlint | 3,493 | 3,409 |

The counts differ where the rules still differ. The README lists the known
differences under Known limitations.

An earlier version of this document reported a 507x average speedup. Those
numbers were wrong: the script ran an older gomdlint that rejected the
profiling flags it was given and exited at once, and `|| true` hid the
failure, so every gomdlint time was the 0.02 s of a failed start.

## Environment

- Linux x86_64, 32 CPUs, shared with other work (load average about 11
  during the run)
- gomdlint built with Go 1.26 from `eb70b8b`, `GOMAXPROCS=8`, `--jobs 8`
- markdownlint-cli 0.49.0 with markdownlint 0.41.1 on Node.js v22.23.3
- One timed run per tool and repository (`BENCH_RUNS=1`)

golang/website is left out. markdownlint 0.41.1 stops with an error in
MD042 on `_content/doc/modules/publishing.md`, so markdownlint-cli cannot
lint the repository.

## Method

`bench/scripts/run-bench.sh` runs the comparison on the repositories in
`bench/repos.txt`, which `bench/scripts/clone-repos.sh` clones as shallow
clones into `~/.cache/gomdlint-compare/repos/<org>__<repo>`.

### The files each tool lints

gomdlint lints the repository directory, as it is usually run, so it walks
the tree and reads its `.gitignore` files. markdownlint gets the list of the
Markdown files (`.md` and `.markdown`) that git tracks, without hidden files
and directories, the `node_modules` and `vendor` directories, and the files
a `.gitignore` file ignores, which gomdlint skips. The list is one argument
list, and the script stops if it would exceed the system's limit.

Before timing anything, the script lints every repository once with
gomdlint and stops unless gomdlint linted exactly the files on the list.
A failure is found in minutes instead of after the slow markdownlint runs.

### Conditions

- Both tools run from an empty directory with an empty `HOME` and
  `XDG_CONFIG_HOME`, so neither reads a config file from the repository or
  the user, and both use their default rules.
- Neither reads a config file from the directories above the working
  directory either. gomdlint gets an empty config file with `--config`,
  which takes the place of the project config it would look for there.
  markdownlint-cli gets `--configPointer` with a key that no config file
  holds, so its rules get an empty configuration whatever `.markdownlintrc`
  it finds.
- Environment variables that start with `GOMDLINT_` or `markdownlint_`,
  in any case, are removed, since markdownlint-cli reads its prefix in any
  case. The run stops if `/etc/gomdlint` or a markdownlint config in `/etc`
  exists.
- Both tools write JSON, which the script reads for the counts of files
  and findings, so writing the report is part of each timed run.
- A run fails unless the tool exits 0 or 1 and its report shows the files
  linted. gomdlint's report must also show no rule failures. markdownlint
  must write a new report in each run, and an exit status of 1 must come
  with findings, since markdownlint-cli also exits 1 when it rejects its
  command line.

### Timing

The script times each run by the wall clock, from the shell's
`EPOCHREALTIME` or `date +%s%N`. With GNU time installed, it also records
each run's peak memory. Without it, memory is recorded as unavailable, as in
the run above.

Each run of a repository times gomdlint and then markdownlint, and the
median of the runs is kept. A run stops if a tool's count of findings
changes from one run to the next.

### Profiles

With `BENCH_PROFILE=1`, the default, the script runs gomdlint once more on
each repository, untimed, with `--cpuprofile`, `--memprofile` and `--trace`.
The profiles go to `bench/results/profiles/<timestamp>/<org>__<repo>/`.

## Running the comparison

```bash
stave cmp     # clone the repositories, compare, and plot the results
stave cmpf    # one run on DavidAnson/markdownlint only
```

Or run the scripts directly:

```bash
bench/scripts/clone-repos.sh
GOMDLINT_BIN=./gomdlint bench/scripts/run-bench.sh
```

Results go to `bench/results/raw/<timestamp>.json`, and
`bench/results/latest.txt` names the newest. The scripts read:

| Variable | Meaning | Default |
| --- | --- | --- |
| `GOMDLINT_BIN` | gomdlint binary under test | `gomdlint` on `PATH` |
| `MARKDOWNLINT_BIN` | markdownlint-cli binary | `markdownlint` on `PATH` |
| `BENCH_RUNS` | timed runs per tool and repository | 3 |
| `BENCH_REPOS` | repositories to run, from `repos.txt` | all |
| `BENCH_GOMAXPROCS` | `GOMAXPROCS` and `--jobs` for gomdlint | 8 |
| `BENCH_PROFILE` | 1 to write gomdlint profiles | 1 |
| `BENCH_RESULTS_DIR` | where results go | `bench/results` |
| `GOMDLINT_COMPARE_CACHE` | repository cache | `~/.cache/gomdlint-compare` |

To compare other repositories, add them to `bench/repos.txt` as `org/repo`.

## Limitations

- The machine was not idle, and each tool ran once per repository. Expect
  differences of a few percent between runs.
- The repositories were read from a warm file system cache: the untimed
  check reads every file first.
- gomdlint uses 8 workers and markdownlint-cli one process, so the speedup
  would be smaller on a machine with fewer cores.
- Rule behaviour is close but not identical, as the counts of findings
  show.

## Auto-fix coverage

37 of the 55 rules can fix what they find, by category:

| Category | Fixable | Rules |
| --- | ---: | ---: |
| Headings | 9 | 14 |
| Lists | 6 | 6 |
| Whitespace and layout | 6 | 6 |
| Code | 5 | 6 |
| Links and images | 4 | 11 |
| Tables | 2 | 5 |
| Emphasis | 3 | 3 |
| Blockquotes | 1 | 2 |
| HTML | 0 | 1 |
| Spelling | 1 | 1 |

`gomdlint rules --fixable` lists them, and `gomdlint explain <rule>` says
what a rule's fix changes and which findings it reports without a fix. The
README describes how gomdlint checks every fix before it applies it.
