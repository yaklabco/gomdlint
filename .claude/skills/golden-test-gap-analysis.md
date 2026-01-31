---
name: golden-test-gap-analysis
description: Use when analyzing which gomdlint rules lack golden test coverage and determining which edge-case categories apply to each rule. Triggers before creating any new golden test files.
---

# Golden Test Gap Analysis

## Overview

Analyze gomdlint rules to determine which lack golden test coverage and which edge-case scenarios are applicable to each rule based on what the rule checks, whether it modifies content, and what markdown contexts it operates in.

## Context

Golden tests live in `pkg/lint/rules/testdata/<RULE_ID>/`. Each test case consists of 4 files:
- `<name>.input.md` — the markdown input with violations
- `<name>.golden.md` — the expected output after fixes are applied
- `<name>.diags.json` — the expected diagnostics as JSON
- `<name>.diags.txt` — the expected diagnostics as human-readable text

The test harness auto-discovers these via `TestGoldenPerRule` in `golden_test.go`.

## Edge Case Categories

For each rule, determine which of these categories are applicable. Not every category applies to every rule — skip categories that are meaningless for a given rule.

| Category | File Name | Description | When Applicable |
|----------|-----------|-------------|-----------------|
| Basic violation | `basic` | Standard violation case | All rules |
| Clean file | `clean` | No violations present | All rules |
| YAML frontmatter | `frontmatter` | Violations after `---` frontmatter block | Rules that check line-level content or headings |
| Code block immunity | `code_block_immunity` | Violations inside fenced/indented code blocks that must NOT be flagged | Rules that scan line content and must respect code blocks |
| Unicode content | `unicode` | Multi-byte characters (emoji, CJK, accented) near violation site | Rules that do byte-offset edits or column calculations |
| Nested structures | `nested` | Violations inside blockquotes, list items | Rules whose violations can appear in nested contexts |
| CRLF line endings | `crlf` | Same violations with `\r\n` line endings | Rules that do line-ending-sensitive edits |
| Multiple violations | `multiple` | Many violations in a single file | All fixable rules — tests batch fix correctness |
| Adjacent violations | `adjacent` | Two violations on same/consecutive lines | Fixable rules where offset shifts could conflict |
| Empty file | `empty_file` | Completely empty file or whitespace-only | Rules that check document-level properties (MD041, MD047) |
| Large content | `large_content` | Violation buried after 200+ lines | Fixable rules — stress tests offset math |
| Mixed HTML | `html_mixed` | Violations near inline HTML elements | Rules that scan inline content |
| Table context | `table_context` | Violations inside GFM table cells | Rules whose violations could appear in tables |
| Link context | `link_context` | Violations inside link text or destinations | Rules that scan inline content near links |
| Indented context | `indented_context` | Content indented 4+ spaces (potential code block ambiguity) | Rules sensitive to indentation parsing |
| Escaped characters | `escaped_chars` | Violations near backslash-escaped markdown | Rules that modify inline content |
| Consecutive blanks | `consecutive_blanks` | Many blank lines around violation | Rules that interact with MD012 blank line behavior |
| Trailing content | `trailing_content` | No trailing newline at EOF | Rules that interact with MD047 |

## Analysis Process

For each of the 55 rules:

1. **Read the rule implementation** in `pkg/lint/rules/` to understand:
   - What markdown construct it checks (headings, lists, code blocks, links, etc.)
   - Whether it produces fix edits (`CanFix() == true`)
   - What contexts it skips (e.g., `ctx.IsLineInCodeBlock()`)
   - What byte-level operations its fixes perform

2. **Check existing coverage** — list what test cases already exist in `testdata/<RULE_ID>/`

3. **Determine applicable categories** — for each edge-case category, decide yes/no/already-covered based on the rule's behavior

4. **Output a structured report** with this format per rule:

```
## MD009 (no-trailing-spaces) — Fixable: Yes
Existing cases: basic, clean
Missing cases: frontmatter, unicode, crlf, multiple, adjacent, large_content
Not applicable: table_context (trailing spaces in table cells are structural)
```

## Output

Produce a single markdown file at `docs/plans/golden-test-gap-analysis.md` containing:
- Summary table: rule ID, fixable, existing count, missing count, applicable categories
- Detailed per-rule analysis as shown above
- Priority ordering: fixable rules with zero coverage first, then fixable rules needing edge cases, then non-fixable rules
