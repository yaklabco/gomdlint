# Golden Test Methodology (With a twist)

## Introduction

This document describes how we built comprehensive golden test coverage for gomdlint's 55 lint rules — and how we used AI agents to do most of the work.

Golden testing is a well-known technique: you capture the output of your program, commit it, and fail the build if the output ever changes unexpectedly. What makes this project interesting is the scale and the process. We needed ~170 carefully constructed markdown files, each designed to trigger exactly one rule while avoiding false positives from the other 54. Writing those files requires detailed knowledge of every rule's behavior, its edge cases, and the dozen or so ways you can accidentally create a bad test input. That's a lot of domain knowledge to hold in your head across 55 rules.

So we encoded that knowledge into three reusable [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) skills — structured instruction files that agents load before doing work. The [gap analysis skill](../.claude/skills/golden-test-gap-analysis.md) tells the agent how to audit the codebase and figure out what's missing. The [authoring skill](../.claude/skills/golden-test-authoring.md) tells it exactly how to construct test inputs for each edge-case category (frontmatter, unicode, code blocks, etc.) and what mistakes to avoid. The [verification skill](../.claude/skills/golden-test-verification.md) gives it a checklist to run after generating golden files. With those skills in place, we could dispatch 5 [sub-agents](https://docs.anthropic.com/en/docs/claude-code/sub-agents) in parallel, each working on a different group of rules, and get consistent results across all of them.

The output speaks for itself: 477 golden tests covering all 55 rules, a round-trip idempotency guarantee on every fixable rule, and three real bugs discovered in rule implementations along the way. The whole thing — from gap analysis to committed PR — ran in a single session.

This document covers the test system itself (how golden files work, what they verify, why the round-trip test matters) and the agent-driven process we used to create them (how the skills work, how we parallelized the effort, what went right and what we learned).

## The Problem

gomdlint has 55 lint rules, 37 of which can auto-fix violations. When you change a rule's detection logic or fix generation, how do you know you haven't broken something? Unit tests can check individual cases, but they don't scale well — and they don't catch the subtle interaction between detection, fix generation, edit merging, and re-parsing that makes autofix tricky.

We needed a test strategy that:

- Catches regressions in both diagnostics and fix output
- Verifies fixes don't corrupt documents
- Scales to hundreds of scenarios without writing hundreds of test functions
- Makes it obvious when behavior changes (intentionally or not)

And then we needed to actually create ~170 hand-crafted test input files across all 55 rules — each one requiring knowledge of the specific rule's behavior, what markdown constructs trigger it, and what pitfalls to avoid (like accidentally triggering MD009 when you're trying to test MD012). Doing that by hand would be tedious and error-prone. So we taught AI agents how to do it.

## How the Golden Tests Work

The golden test system is built on a simple file convention:

```
testdata/MD009/
├── basic.input.md          ← you write this
├── basic.golden.md         ← generated: expected output after fix
├── basic.diags.json        ← generated: expected diagnostics
└── basic.diags.txt         ← generated: human-readable diagnostics
```

You only ever write `.input.md` files by hand. Everything else is generated.

### The Generate-Then-Lock Workflow

When you create a new test case or change a rule:

```bash
# Regenerate golden files for a rule
go test ./pkg/lint/rules/... -run TestGoldenPerRule/MD009 -args -update
```

This runs the rule against the input, captures the diagnostics and fixed output, and writes them to disk. You review the generated files — are the diagnostics correct? Does the fixed output look right? — and commit them.

From that point on, the golden files are locked. Any future change that produces different diagnostics or different fix output will fail the test. You either fix the regression or deliberately re-generate with `-update`.

### What Gets Tested

**TestGoldenPerRule** runs a single rule against each input file in its directory. It checks two things:

1. The diagnostics match `diags.json` exactly — same rule ID, line, column, message, severity, and fixability
2. After applying all fixes, the output matches `golden.md` byte-for-byte

**TestGoldenRoundTrip** goes further. It applies the fixes, then re-parses the fixed document and runs the rule again. If any fixable diagnostics remain, the test fails. This catches a class of bugs that are otherwise hard to detect:

- Fixes that shift byte offsets and break subsequent edits
- Fixes that introduce new violations
- Fixes that partially apply, leaving the document in a state that still triggers the rule

The round-trip guarantee means: **if the tool says it can fix something, applying the fix actually fixes it.**

### Test Discovery

The test harness discovers cases automatically. Drop an `.input.md` file into `testdata/MD009/` and it gets picked up on the next test run. No registration, no test function to write. The directory name determines which rule runs — `testdata/MD009/` runs MD009, `testdata/real-world/` runs all enabled rules.

### Fix Application

The fix pipeline is the same one users get. Diagnostics produce `TextEdit` values (byte offset ranges + replacement text). The edits go through:

1. **Validation** — bounds checking against document length
2. **Sorting** — by start offset
3. **Merge/conflict resolution** — overlapping deletions get merged into a single edit; other overlaps are resolved greedily (earlier edit wins)
4. **Application** — cursor-based splice of the original content

This means the golden tests exercise the real fix pipeline, not a simplified test double. If there's a bug in edit merging or offset calculation, a golden test will catch it.

## How We Created 170 Test Files With Agents

We started with 23 rules that had golden tests and needed to get to 55 — plus add edge-case coverage for all of them. That's roughly 170 input files, each needing to be carefully constructed markdown that triggers exactly one rule without accidentally tripping others. Getting that right requires knowing things like: always end with a newline (or you trigger MD047), don't use tabs (MD010), no trailing whitespace (MD009), use proper heading hierarchy (MD001), and so on.

We used Claude Code agents to do this, but the interesting part is *how* we made it reliable.

### The Skills

We wrote three project-specific skills (instruction files that agents load before doing work) that encode the rules for golden test creation:

**golden-test-gap-analysis** — Tells the agent how to analyze the 55 rules, read each implementation, check what test cases exist, and produce a coverage report. It defines the edge-case categories (frontmatter, code_block_immunity, unicode, etc.) and when each applies. The agent reads every rule's source code, checks which ones call `ctx.IsLineInCodeBlock()`, which ones are fixable, and maps out what's missing.

**golden-test-authoring** — The detailed rulebook for creating input files. It specifies the file naming convention, content structure, size guidelines, and templates for each edge-case category. Critically, it includes a "common mistakes" table: don't accidentally use tabs, don't leave trailing whitespace, always start with a heading, etc. These are the kinds of things that cause cascading test failures when you get them wrong, and encoding them in a skill means agents don't have to learn them the hard way.

**golden-test-verification** — The checklist agents follow after generating golden files. Read the `.diags.json`, check the line numbers make sense, verify `clean` cases produce zero diagnostics, verify `code_block_immunity` cases produce zero diagnostics, run the round-trip test. It also lists red flags: if a clean case produces diagnostics, the input has accidental violations; if a round-trip test fails, the fix is non-idempotent.

### The Phased Approach

We broke the work into phases, each dispatching multiple agents in parallel:

**Phase 1 — Gap Analysis.** One agent read all 55 rule implementations, catalogued existing test coverage, and produced a coverage matrix. This told us exactly what was missing: 32 rules had zero golden tests, and even the 23 that had tests were missing edge-case categories.

**Phase 2 — Basic + Clean.** Five agents ran in parallel, each handling a group of rules (heading rules, list rules, link rules, etc.). Each agent created `basic.input.md` and `clean.input.md` for its assigned rules, generated the golden files with `-update`, read the output to verify correctness, and ran the tests. By the end, all 55 rules had test directories.

**Phase 3 — Edge Cases for Fixable Rules.** Five more agents in parallel. Each one took a group of fixable rules and added frontmatter, code_block_immunity, unicode, multiple, adjacent, and nested cases as applicable. This is where the authoring skill really paid off — an agent working on MD009 (trailing whitespace) needs to create a `unicode.input.md` with trailing spaces after multi-byte characters like `你好` and `🎉`, and the skill tells it exactly how to structure that.

**Phase 4 — Edge Cases for Non-Fixable Rules.** Three agents covered the remaining rules. Non-fixable rules need fewer edge cases (no offset shifting to worry about), so this phase focused on frontmatter and code_block_immunity scenarios.

**Phase 5 — Full Verification.** Ran the complete test suite (477 golden tests + the rest of the project's 2041 tests), confirmed everything passes, committed.

### Why Agents + Skills Worked Here

The key insight is that writing golden test input files is a task with very precise rules that are easy to get wrong in subtle ways. A human writing `frontmatter.input.md` for MD012 (no-multiple-blanks) might accidentally leave trailing whitespace on a line, which would cause the generated diagnostics to include MD009 violations — and the test would still pass (it only runs MD012), but the input wouldn't be testing what you think it's testing.

The skills encode all of these constraints in one place. Every agent loads the same authoring rules and verification checklist. That consistency is hard to maintain across 170 files if you're writing them by hand over multiple sessions.

The parallel dispatch is the other big win. Five agents creating test files for different rule groups simultaneously, with no file conflicts because each rule has its own directory. The total wall-clock time for creating all the test files was a fraction of what sequential work would have taken.

The agents also caught real bugs during the process:

- **MD034 (bare URLs):** The autofix wraps bare emails in angle brackets (`<user@example.com>`), but the regex still partially matches the `<`, causing an infinite fix loop. The agent discovered this during round-trip testing and worked around it by only testing URL patterns.
- **MD035 (HR style):** Goldmark doesn't assign source positions to thematic break AST nodes, so the rule produces zero diagnostics despite valid input. The agent documented this and kept the test as a regression baseline.
- **MD049/MD050 (emphasis style):** `detectEmphasisStyle()` fails because the parser's `SourcePosition` points to emphasis content rather than the marker character. Again, documented and baselined.

These are the kind of things you find when you systematically run every rule against carefully constructed edge cases. The agents did the tedious part; the generated golden files captured the actual behavior for future regression detection.

### Trust Model

The agents don't decide what's correct — the tool does. The workflow is:

1. Agent creates a `.input.md` file (following the skill's rules)
2. Agent runs `go test -update` to generate golden files from the *actual rule implementation*
3. Agent reads the generated diagnostics and fix output to verify they make sense
4. Agent runs `TestGoldenRoundTrip` to verify fix idempotence

The golden files are a snapshot of the tool's real behavior. If that behavior is wrong, the golden test locks it in — and you fix the rule, not the test. The agents verify that the snapshot is internally consistent (diagnostics match what the code produces, fixes are idempotent), but they don't invent expected output.

## Edge Case Categories

Each rule gets test cases from a set of categories based on what's applicable:

| Category | What It Tests |
|----------|--------------|
| `basic` | The rule's core detection and fix behavior |
| `clean` | A file with no violations — should produce zero diagnostics |
| `frontmatter` | Content after YAML frontmatter (`---` blocks) — catches off-by-N line counting |
| `code_block_immunity` | Rule trigger patterns inside fenced code blocks — should produce zero diagnostics for rules that skip code blocks |
| `unicode` | Multi-byte characters (CJK, Cyrillic, emoji) — catches byte vs. rune offset bugs |
| `multiple` | Several violations in one file — tests batch fix application with offset shifting |
| `adjacent` | Violations on consecutive lines — tests that fixes don't interfere with each other |
| `nested` | Content inside blockquotes or nested lists |
| `empty_file` | Zero-byte input — tests early-return paths |

Not every category applies to every rule. A rule that doesn't skip code blocks doesn't need `code_block_immunity`. A non-fixable rule doesn't need `multiple` (since there's no offset shifting to worry about). The categories are assigned per-rule based on the rule's implementation.

## Current Coverage

477 golden tests across all 55 rules. Every rule has at least `basic` and `clean` cases. Fixable rules have edge-case coverage for frontmatter, code block immunity, unicode, and batch fix scenarios. Non-fixable rules have frontmatter and (where applicable) code block immunity cases.

```bash
# Run all golden tests
go test ./pkg/lint/rules/... -run TestGolden

# Run tests for a specific rule
go test ./pkg/lint/rules/... -run TestGoldenPerRule/MD009

# Regenerate after changing a rule
go test ./pkg/lint/rules/... -run TestGoldenPerRule/MD009 -args -update
```
