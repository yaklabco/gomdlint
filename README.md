# gomdlint

A blisteringly fast, self-fixing Markdown linter written in Go.

Inspired by [markdownlint](https://github.com/DavidAnson/markdownlint) by
David Anson, which has set the standard for markdown linting. Without this
epic work, this project would not have been possible.

## Why gomdlint?

| Feature | gomdlint | markdownlint |
| --- | --- | --- |
| **Performance** | **507x faster** average | baseline |
| **Auto-fixable** | 67% (37 of 55 rules) | Limited |
| **Language** | Go | Node.js |

gomdlint is **dramatically faster** than markdownlint. Benchmarks run on
real-world repositories in January 2026 showed an average **507x speedup**,
with improvements ranging from 72x to over 1100x depending on repository
size. 37 of 55 rules support automatic fixing. Built in Go with no runtime
dependencies.

See [bench/PERFORMANCE.md](bench/PERFORMANCE.md) for detailed benchmark
methodology and results.

## Features

- **55 lint rules**: markdownlint's 53 rules with its defaults, and two of
  gomdlint's own
- **Automatic fixing** for 37 rules, with every fix checked against the
  document's structure before it is applied
- **Multiple output formats** including text, table, JSON, SARIF, diff, and
  summary
- **CommonMark and GFM support** with flavor-specific rules
- **Flexible configuration** via YAML or JSON files with hierarchical
  discovery
- **Fast parallel processing** with deterministic ordering
- **Human-readable rule names** with optional traditional ID format

## Installation

### Homebrew (macOS/Linux)

```bash
brew install yaklabco/tap/gomdlint
```

### Go

```bash
go install github.com/yaklabco/gomdlint/cmd/gomdlint@latest
```

### Binary releases

Download from [GitHub Releases](https://github.com/yaklabco/gomdlint/releases).

## Quick Start

```bash
gomdlint lint                    # Lint current directory
gomdlint lint docs/              # Lint specific directory
gomdlint lint README.md          # Lint single file
gomdlint lint --fix              # Lint and auto-fix issues
gomdlint lint --fix --dry-run    # Preview fixes without applying
```

## Linting and Fixing

Lint Markdown files with 55 rules. Run `gomdlint lint` on files or
directories to detect issues, with parallel processing for large codebases.
Directory walks skip files that `.gitignore` files ignore, and the
`node_modules` and `vendor` directories. `--no-gitignore` and
`--no-default-ignores` lint those files too. Files named on the command line
are always linted.

Automatically fix most issues with `--fix`. Fixing runs in passes. Each pass
applies the fixes of one rule to each top-level block, such as a list or a
paragraph, and then lints the result again, until nothing is left to fix or
the pass limit is reached. gomdlint backs up each file it changes as
`<file>.gomdlint.bak` and never replaces an existing backup. Disable backups
with `--no-backups` if your files are under version control.

Preview changes before applying them with `--dry-run`, or show them as a
unified diff with `--format diff`, which writes nothing unless you add
`--fix`. Like `diff` and `patch`, the diff ends lines only at line feeds, so
a file whose lines end with carriage returns alone shows as one line.

A line ends with a line feed, a carriage return and a line feed, or a
carriage return alone, as in CommonMark and markdownlint. In a file with a
carriage return alone, fixes keep each line's own line ending. A line that
a fix adds takes the file's most common line ending, and a fix that would
merge a carriage return and a line feed after it into one line ending is
withheld. In any other file, every newline that a fix writes takes the
file's most common line ending, so a fix that rewrites lines of a file
mixing LF and CRLF writes them all with that ending.

Limit auto-fixing to specific rules with `--fix-rules`, which takes rule
names, IDs and markdownlint tags.

### Fix Safety

A fix must not change what a document means. Every fixing rule applies its
fixes, parses the result again with the same flavor, and compares it with
the original: block kinds and nesting, heading levels, lists, code, raw HTML,
link destinations, and the text a renderer shows. A fix that would change
anything other than what the rule intends is withheld, and the finding is
still reported without a fix. Fixes are checked one at a time and together,
because the fix engine may apply only some of them.

Some fixes that markdownlint makes are withheld for this reason. For
example, `no-bare-urls` (MD034) fixes a URL only where GitHub, micromark and
goldmark all link the same address, so URLs in tables, emphasis or quotes are
reported without a fix. Fixes that markdownlint does not offer, such as
those of `line-length` (MD013), `no-emphasis-as-heading` (MD036) and
`fenced-code-language` (MD040), are held to the same check. MD040 adds a
language only when the whole code block shows which one it is. See
[Known Limitations](#known-limitations) for the cases where gomdlint
withholds more fixes than it needs to.

## Rules

Every rule has an ID, such as MD001, and a name, such as
`heading-increment`. Config keys, rule flags and markdownlint comments take
either, and markdownlint's names for a rule work too, such as `ul-style` for
`unordered-list-style` (MD004). `gomdlint rules` lists every rule by
category, with its status under your configuration, and
`gomdlint explain <rule>` describes one rule in full (see
[Commands](#commands)). The [rule reference](docs/rules/README.md) has a page
for each rule: what it checks and why, its options, what its fix does, and an
example.

The rule list below and the rule reference are generated from the rule
catalog. Run `go test ./internal/ruledocs -update` to regenerate them.

<!-- BEGIN GENERATED RULES -->

<!-- Generated by: go test ./internal/ruledocs -update. Do not edit. -->

gomdlint has 55 rules, and 37 of them can fix what they find.

### Headings

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`first-line-heading`](docs/rules/first-line-heading.md) | MD041 | no | Comments aside, the file starts with a top-level heading or front matter title. |
| [`heading-blank-lines`](docs/rules/heading-blank-lines.md) | MD022 | yes | Headings have a blank or comment line above and below. |
| [`heading-increment`](docs/rules/heading-increment.md) | MD001 | no | Each heading is at most one level deeper than the heading or title before it. |
| [`heading-start-left`](docs/rules/heading-start-left.md) | MD023 | yes | Headings start at the beginning of their line, list item or blockquote. |
| [`heading-style`](docs/rules/heading-style.md) | MD003 | yes | Headings all use the first heading's style: \# Title, \# Title \#, or underlined. |
| [`no-duplicate-heading`](docs/rules/no-duplicate-heading.md) | MD024 | no | No two headings have exactly the same text. |
| [`no-emphasis-as-heading`](docs/rules/no-emphasis-as-heading.md) | MD036 | yes | Outside lists and blockquotes, a bold or italic line is not a stand-in heading. |
| [`no-missing-space-atx`](docs/rules/no-missing-space-atx.md) | MD018 | yes | A space follows the \# marks that open a heading at the start of a line. |
| [`no-missing-space-closed-atx`](docs/rules/no-missing-space-closed-atx.md) | MD020 | yes | A closed heading at the start of a line has a space inside each run of \#. |
| [`no-multiple-space-atx`](docs/rules/no-multiple-space-atx.md) | MD019 | yes | Only one space follows the \# marks that open a heading. |
| [`no-multiple-space-closed-atx`](docs/rules/no-multiple-space-closed-atx.md) | MD021 | yes | A closed heading has only one space inside each run of \#. |
| [`no-trailing-punctuation`](docs/rules/no-trailing-punctuation.md) | MD026 | yes | Headings do not end with a period, comma, semicolon, colon or exclamation mark. |
| [`required-headings`](docs/rules/required-headings.md) | MD043 | no | Headings follow the configured outline in order. No outline is set by default. |
| [`single-h1`](docs/rules/single-h1.md) | MD025 | no | A document that starts with a title has no other top-level heading. |

### Lists

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`blanks-around-lists`](docs/rules/blanks-around-lists.md) | MD032 | yes | Lists outside other lists have a blank or comment line above and below. |
| [`list-indent`](docs/rules/list-indent.md) | MD005 | yes | Items in a list start at the same column. Ordered numbers may right-align. |
| [`list-marker-space`](docs/rules/list-marker-space.md) | MD030 | yes | One space, or one tab, separates each list marker from the item's text. |
| [`ol-prefix`](docs/rules/ol-prefix.md) | MD029 | yes | Ordered list items are numbered 1, 1, 1, or 1, 2, 3, or 0, 1, 2, and so on. |
| [`ul-indent`](docs/rules/ul-indent.md) | MD007 | yes | Outside ordered lists, bullets sit at the margin and indent 2 spaces per level. |
| [`unordered-list-style`](docs/rules/unordered-list-style.md) | MD004 | yes | Every bullet uses the same marker as the first bullet in the file. |

### Whitespace and layout

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`hr-style`](docs/rules/hr-style.md) | MD035 | yes | Every horizontal rule is written the same way as the first one, such as ---. |
| [`line-length`](docs/rules/line-length.md) | MD013 | yes | Lines fit in 80 characters, with exemptions such as lone links and definitions. |
| [`no-hard-tabs`](docs/rules/no-hard-tabs.md) | MD010 | yes | Lines contain no tab characters, including inside code. |
| [`no-multiple-blank-lines`](docs/rules/no-multiple-blank-lines.md) | MD012 | yes | Outside code, no two blank lines follow each other, except after front matter. |
| [`no-trailing-spaces`](docs/rules/no-trailing-spaces.md) | MD009 | yes | Outside code blocks, lines end in no spaces or tabs, or in exactly 2 of them. |
| [`single-trailing-newline`](docs/rules/single-trailing-newline.md) | MD047 | yes | The file ends with a newline, or a last line of only whitespace, \> or comments. |

### Code

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`blanks-around-fences`](docs/rules/blanks-around-fences.md) | MD031 | yes | Fenced code blocks have a blank or comment line above and below. |
| [`code-block-style`](docs/rules/code-block-style.md) | MD046 | no | Code blocks all use the first block's style, fenced or indented. |
| [`code-fence-style`](docs/rules/code-fence-style.md) | MD048 | yes | Code fences all use the first fence's character, backtick or tilde. |
| [`commands-show-output`](docs/rules/commands-show-output.md) | MD014 | yes | Code blocks that show only commands leave out the \$ prompt. |
| [`fenced-code-language`](docs/rules/fenced-code-language.md) | MD040 | yes | Fenced code blocks name their language after the opening fence. |
| [`no-space-in-code`](docs/rules/no-space-in-code.md) | MD038 | yes | Code span text has no spaces around it, except one each side or by a backtick. |

### Links and images

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`descriptive-link-text`](docs/rules/descriptive-link-text.md) | MD059 | no | Link text describes its target instead of saying "click here" or "more". |
| [`link-destination-style`](docs/rules/link-destination-style.md) | MDL001 | no | Relative links and local file: URLs are not mixed in one document. |
| [`link-fragments`](docs/rules/link-fragments.md) | MD051 | no | Links to a \#fragment point at a heading or anchor in the same file, or at \#top. |
| [`link-image-reference-definitions`](docs/rules/link-image-reference-definitions.md) | MD053 | yes | Every reference definition is used and defined once, except \[//\] comments. |
| [`link-image-style`](docs/rules/link-image-style.md) | MD054 | no | Links and images use only the allowed styles. By default, all are allowed. |
| [`no-alt-text`](docs/rules/no-alt-text.md) | MD045 | no | Markdown images have alt text, and HTML images have alt or aria-hidden="true". |
| [`no-bare-urls`](docs/rules/no-bare-urls.md) | MD034 | yes | Outside code and HTML, URLs and email addresses sit in angle brackets or links. |
| [`no-empty-links`](docs/rules/no-empty-links.md) | MD042 | no | Inline links have a destination, and no link points at just \#. |
| [`no-reversed-links`](docs/rules/no-reversed-links.md) | MD011 | yes | Inline links put the text in brackets and the URL in parentheses: \[text\](url). |
| [`no-space-in-links`](docs/rules/no-space-in-links.md) | MD039 | yes | Link text has no spaces just inside its brackets. |
| [`reference-links-images`](docs/rules/reference-links-images.md) | MD052 | no | Labels in \[text\]\[label\] and \[label\]\[\] references are defined, except x. |

### Tables

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`blanks-around-tables`](docs/rules/blanks-around-tables.md) | MD058 | yes | Tables have a blank or comment line above and below. |
| [`table-alignment`](docs/rules/table-alignment.md) | MDL003 | yes | Each cell of a table's delimiter row has at least 3 dashes. |
| [`table-column-count`](docs/rules/table-column-count.md) | MD056 | no | Every table row has as many cells as the header row. |
| [`table-column-style`](docs/rules/table-column-style.md) | MD060 | no | Each table follows one style: aligned pipes, one space of padding, or none. |
| [`table-pipe-style`](docs/rules/table-pipe-style.md) | MD055 | no | Table rows use leading and trailing pipes the same way throughout. |

### Emphasis

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`emphasis-style`](docs/rules/emphasis-style.md) | MD049 | yes | All italic text uses one marker, \*text\* or \_text\_, except \* inside a word. |
| [`no-space-in-emphasis`](docs/rules/no-space-in-emphasis.md) | MD037 | yes | Emphasis markers touch the text they wrap, as in \*\*bold\*\*. |
| [`strong-style`](docs/rules/strong-style.md) | MD050 | yes | All bold text uses one marker, \*\*text\*\* or \_\_text\_\_, except \*\* inside a word. |

### Blockquotes

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`no-blanks-blockquote`](docs/rules/no-blanks-blockquote.md) | MD028 | no | No blank line sits between two blockquotes. |
| [`no-multiple-space-blockquote`](docs/rules/no-multiple-space-blockquote.md) | MD027 | yes | Outside code and list item text, at most one space follows a blockquote's \>. |

### HTML

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`no-inline-html`](docs/rules/no-inline-html.md) | MD033 | no | Documents contain no raw HTML elements other than allowed ones. |

### Spelling

| Name | ID | Fix | Description |
| --- | --- | --- | --- |
| [`proper-names`](docs/rules/proper-names.md) | MD044 | yes | Outside URLs, proper names match the configured capitals, none by default. |

<!-- END GENERATED RULES -->

## Output Formats

Choose the output format that fits your workflow with `--format`:

```bash
gomdlint lint file.md                      # text (default) - human-readable with colors
gomdlint lint --format table file.md       # table - columnar output
gomdlint lint --format summary file.md     # summary - aggregated statistics
gomdlint lint --format json file.md        # json - machine-readable
gomdlint lint --format sarif file.md       # sarif - GitHub code scanning
gomdlint lint --format diff file.md        # diff - the fixes --fix would make
gomdlint lint --format diff --fix file.md  # diff - the fixes --fix made
```

Text output lists each finding with its location, severity, rule and
message, the order markdownlint-cli2 uses, followed by the source line and a
suggestion. `--no-context` leaves out the source lines. After the summary, a
last line names `gomdlint explain`, which describes each rule.

```text
README.md (3 issues)
  README.md:3:1   warning  heading-increment        Heading level jumped from H1 to H3
        ### Details
        ^
    Suggestion: Use H2 instead
  README.md:5:5   warning  no-bare-urls             Bare URL used
        See http://example.com for more.
            ^
    Suggestion: Wrap the URL/email in angle brackets
  README.md:7:11  warning  no-trailing-punctuation  Heading ends with trailing punctuation ":"
        ## Install:
                  ^
    Suggestion: Remove trailing punctuation from the heading

3 issues (3 warnings), in 1 file, 2 fixable
Run 'gomdlint explain <rule>' to see what a rule checks and how to configure it.
```

Columns count UTF-16 code units, as markdownlint and markdownlint-cli2 count
them, so a finding after multibyte text points at its target and an emoji
counts two columns. The caret under the source line sits under the target
whatever the width of the text before it. JSON and SARIF output report the
same columns, while the offsets of JSON fixes are byte offsets. Every format
except SARIF shows a path relative to the working directory when the file
lies inside it.

Findings that one change fixes carry the same fix. `no-space-in-emphasis`
(MD037), `emphasis-style` (MD049) and `strong-style` (MD050) report each side
or marker of an emphasis, as markdownlint does, with the fix that changes
both, and the blank line that a rule asks for on both sides of one boundary
is one insertion. JSON and SARIF reports show such a fix on each finding,
so a consumer that applies every reported fix must apply it once, as
markdownlint's own fix application does.

The summary format shows aggregated statistics:

```text
Rules Summary
──────────────────────────────────────────────────────────────────────────────────────────
Rule                 Count  Errors Warnings  Fixable
──────────────────────────────────────────────────────────────────────────────────────────
no-trailing-spaces       5       0        5        ✓
no-hard-tabs             2       0        2        ✓

Files Summary
──────────────────────────────────────────────────────────────────────────────────────────
File                                                           Count  Errors Warnings
──────────────────────────────────────────────────────────────────────────────────────────
docs/guide.md                                                      4       0        4
README.md                                                          3       0        3

Total: 7 issues (7 warnings) in 2 files
```

Control table order with `--summary-order rules` (default) or `--summary-order files`.

Text, table, JSON and SARIF output list findings file by file, in path
order. Within a file, findings follow line order, as in markdownlint-cli2,
then column, rule ID and message. On one line, markdownlint-cli2 orders
findings by rule ID instead of column. The summary format counts findings
instead of listing them, and orders its rules and files by those counts,
highest first, with ties in rule ID and path order.

SARIF output follows SARIF 2.1.0. Its file locations are relative to the
root of the Git working tree, as GitHub code scanning expects, and its
columns count UTF-16 code units. Each rule descriptor gives the rule's
one-line description as `shortDescription`, and the description, the reason
for the rule and what its fix does as `fullDescription`.

### Color

Text, table, summary and diff output, and help, use color on a terminal.
`--color` controls it:

- `auto` (default): color only when output goes to a terminal and
  `NO_COLOR` is not set.
- `always`: color even when output goes to a pipe or a file, and even when
  `NO_COLOR` is set, because the flag asks for color explicitly. The colors
  are the 16 basic ANSI colors, which every color terminal and `less -R`
  show.
- `never`: no color.

JSON and SARIF output never contain color. Table rows start with a severity
marker, `E`, `W` or `I`, which the legend under the table explains, so a
table reads the same without color. A `+` at the end of a row marks a
finding that `--fix` would fix.

## Rule Identifier Format

Text, table and summary output name each rule by its name. Use
`--rule-format` to show its ID, or both:

```bash
gomdlint lint file.md                        # name (default): no-trailing-spaces
gomdlint lint --rule-format id file.md       # id: MD009
gomdlint lint --rule-format combined file.md # combined: MD009/no-trailing-spaces
```

To change the default for `gomdlint lint` and for the text listings of
`gomdlint rules` and `gomdlint explain`, set `GOMDLINT_RULE_FORMAT`, for
example to `id`. The flag wins over the variable. JSON and SARIF output
always carry both the ID and the name.

## Configuration

Configure gomdlint with a YAML or JSON file. The tool searches project,
user, and system directories, merging configurations hierarchically.

For the project config, gomdlint looks in the current directory and then in
each parent directory, up to the repository root or your home directory. It
uses the first directory that holds one of these files. In that directory it
reads the first name in this list and ignores the others, each with a
warning that names the file it reads:

1. `.gomdlint.yml`
2. `.gomdlint.yaml`
3. `gomdlint.yml`
4. `gomdlint.yaml`
5. `.gomdlint.json`
6. `gomdlint.json`

`gomdlint init` writes `.gomdlint.yml`, or `.gomdlint.json` with
`--format json`. A file whose name ends in `.json` must hold JSON, so
`init --output` and `migrate --output` write JSON for such a name.

The user config is `config.yaml`, `config.yml` or `config.json`, in that
order of preference, in `$XDG_CONFIG_HOME/gomdlint` (by default
`~/.config/gomdlint`). The system config uses the same names in
`/etc/gomdlint`, or `%ProgramData%\gomdlint` on Windows. A file that
another name there shadows gets a warning too.

[docs/schema/gomdlint.schema.json](docs/schema/gomdlint.schema.json) is a
JSON Schema of the configuration, generated from the rule catalog. Editors
that read JSON Schemas complete rule names and options from it, show what
each rule checks, and flag unknown rules and options. With the YAML
language server, as in VS Code's YAML extension, a first line such as this
one points a file at it:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/yaklabco/gomdlint/main/docs/schema/gomdlint.schema.json
```

```yaml
# .gomdlint.yaml
flavor: gfm

rules:
  # Using human-readable name
  no-trailing-spaces:
    enabled: true
    severity: warning
    auto_fix: true

  # Using traditional ID (also works)
  MD013:
    enabled: true
    severity: warning
    options:
      max: 120
      ignore_code_blocks: true

  # A bare true or false turns a rule on or off, as in markdownlint
  no-inline-html: false

  # A bare severity turns the rule on at that severity, as in markdownlint
  no-hard-tabs: error

ignore:
  - "vendor/**"
  - "node_modules/**"
```

Generate a starter configuration with `gomdlint init` or a comprehensive
template that documents every rule with `gomdlint init --full`.

Ignore patterns are matched like `.gitignore` patterns. Patterns in a
project config, or in a file named with `--config`, are relative to that
file's directory. Patterns from a user or system config, the environment or
`--ignore` are relative to the working directory. Run-time settings such as
`--fix`, `--format` and `--jobs` are command-line flags, and a config file
that sets one gets a warning.

### Defaults

Without a config file, gomdlint runs every rule, as markdownlint does, with
markdownlint 0.41's option defaults and the `gfm` flavor. For example,
`heading-style` (MD003) and `unordered-list-style` (MD004) expect a
consistent style, `ol-prefix` (MD029) accepts `one_or_ordered` numbering,
`no-trailing-spaces` (MD009) allows two-space hard breaks, and `line-length`
(MD013) allows 80 characters. Where a rule has an option that markdownlint
lacks, its default keeps markdownlint's behaviour.

Every rule reports warnings. Warnings fail a run only with `--strict`. Set
`severity_default: error`, or a rule's `severity`, to fail on findings
without it.

`required-headings` (MD043) and `proper-names` (MD044) check nothing until
they are given headings or names, and `link-image-style` (MD054) reports
nothing until an option disallows a style. Turn any rule off by name or ID,
under `rules` or with `--disable`.

### Naming Rules

Wherever a rule can be named, use its ID (`MD013`), its name
(`line-length`) or one of markdownlint's names for it (`ul-style` for MD004),
in any case: under `rules`, in `--enable`, `--disable` and `--fix-rules`, in
markdownlint comments, and in `markdownlint-configure-file`. The flags, the
comments and `markdownlint-configure-file` also accept markdownlint's tags,
such as `whitespace`, for every rule a tag covers. Keys under `rules` take
rules only.

A name that matches no rule gets an error or a warning that suggests the
closest rules, such as `unknown rule "no-trailling-spaces" (did you mean
"no-trailing-spaces"?)`. In a config file, the warning names the file and
line. Shell completion offers rule names, IDs, markdownlint's names and
tags for the three flags and for `gomdlint explain`, with each rule's
one-line description. It completes categories and tags for the
`--category` and `--tag` filters of `gomdlint rules`.

### Rule Settings

A rule's value is a mapping of its settings, `enabled`, `severity`,
`auto_fix`, `reset_options` and `options`, or one of markdownlint's
shorthands. `true` and `false` turn the rule on and off. A severity turns
the rule on at that severity, as in markdownlint: `error` or `warning`, or
gomdlint's `info`. So `line-length: warning` means
`line-length: {enabled: true, severity: warning}`. Any other string is an
invalid severity, which fails as it does under `severity`.

Under `rules`, the shorthand turns the rule on even when `rule_defaults` or
a lower layer, such as a user config, turns it off. It keeps the options
and `auto_fix` setting that they give the rule. The long form differs:
`line-length: {severity: warning}` sets the severity alone, so a rule that
`rule_defaults` or a lower layer turns off stays off. Under `rule_defaults`,
the shorthand is a default like any other there, so a rule that `rules`
turns off, in any layer, stays off.

Under `rules`, `reset_options: true` drops the options that lower layers
give the rule under `rules`, and those under `rule_defaults` in every
layer, so every option that the rule's own settings do not set takes its
default. Migration writes it for each rule that a markdownlint config turns
on, since there a rule key gives the rule its own options and no others.
Under `rule_defaults`, it drops only the options that lower layers give the
rule under `rule_defaults`. `reset_options: false` is the same as leaving
the setting out, so it cannot undo the reset of a lower layer.

A key written twice in one mapping is an error in any part of a config
file that gomdlint reads, such as a rule under `rules`, a setting under a
rule or an option under `options`. A key that a YAML alias writes a second
time gets the same error, as if the key were written out. gomdlint does not
read the value of an unknown top-level key, or of an option that a rule
does not have when it is written directly under the rule key, so it does
not check those values for keys written twice.

Every key in a config file must be a scalar, such as a name or a number.
A sequence or a mapping written as a key, or an alias of one, is an error
anywhere in the file, even in a value that gomdlint does not read. The
error names the key's line, as in `line 3: mapping key must be a scalar`.

### Rule Options

A rule's options go under its `options` key:

```yaml
rules:
  line-length:
    options:
      max: 100
      ignore_tables: true
```

They can also go directly under the rule key, as markdownlint writes them.
This example sets the same options under markdownlint's names:

```yaml
rules:
  line-length:
    line_length: 100
    tables: false
```

Under a rule key, `enabled`, `severity`, `auto_fix`, `reset_options` and
`options` are the rule's settings, and every other key is an option. A
rule can mix settings, options under the rule key and options under
`options`, and all of them apply. Setting one option in both places is an
error. `init` and `migrate` write options under `options`.

Every option that markdownlint documents works under markdownlint's name, so
markdownlint's documentation of a rule's parameters holds for gomdlint too.
Nine options have a gomdlint name that differs, and gomdlint's own config
files accept both spellings:

| Rule | gomdlint name | markdownlint name |
| --- | --- | --- |
| `no-trailing-spaces` (MD009) | `ignore_code_blocks` | `code_blocks`, inverted |
| `no-multiple-blank-lines` (MD012) | `max_consecutive` | `maximum` |
| `line-length` (MD013) | `max` | `line_length` |
| `line-length` (MD013) | `heading_max` | `heading_line_length` |
| `line-length` (MD013) | `code_block_max` | `code_block_line_length` |
| `line-length` (MD013) | `ignore_code_blocks` | `code_blocks`, inverted |
| `line-length` (MD013) | `ignore_tables` | `tables`, inverted |
| `line-length` (MD013) | `ignore_headings` | `headings`, inverted |
| `descriptive-link-text` (MD059) | `patterns` | `prohibited_texts` |

An inverted name holds the opposite value, so `tables: false` means
`ignore_tables: true`. Setting both spellings of one option for a rule is
an error in a gomdlint config file. Options that markdownlint lacks keep
markdownlint's behaviour by default.

A markdownlint config, and a `markdownlint-configure-file` comment, read
options as markdownlint does, under markdownlint's names only. gomdlint's
name for one of the options above, such as `max`, and an option that only
gomdlint has, such as `ignore_urls`, are options that the rule does not
read there. They change nothing, and get a warning, which under a rule's
own key offers markdownlint's name where there is one. Set them in a
gomdlint config file. The options of gomdlint's own rules,
`link-destination-style` (MDL001) and `table-alignment` (MDL003), which
markdownlint lacks, keep their names.

A config file gets a warning, with its path and line, for an option that
the rule does not read, with the closest names, such as `unknown option
"stirct" for line-length (did you mean "strict"?)`. A key under the rule
that looks like a misspelled setting, such as `severty`, gets a warning that
suggests the setting instead. A config file also gets a warning for a
value of the wrong type, which counts as unset, so the option keeps the
value that an earlier layer gives it, or its default, and for a value
outside the option's allowed values, such as a `style` that the rule does
not know. A list with an item of the wrong type, such as
`lines_above: [1, 2.5]`, counts as a value of the wrong type: each such
item gets a warning, and the whole list counts as unset. A markdownlint
config that gomdlint migrates or translates for a run gets these warnings
too, with the line of each option.

### Precedence

Settings apply in this order, each layer overriding the ones before it:

1. Built-in defaults
2. The system config, then the user config
3. The project config, or a markdownlint config translated for the run
4. A config file named with `--config`, which replaces the project config
5. Environment variables
6. Command-line flags

A rule's own severity, under `rules` or `rule_defaults` in any layer,
overrides `severity_default` from every layer, `GOMDLINT_SEVERITY_DEFAULT`
included. A rule's settings under `rules`, in any layer, override its
settings under `rule_defaults`, in every layer. `--disable` wins over
`--enable` for the same rule.

These environment variables set the matching settings: `GOMDLINT_FLAVOR`,
`GOMDLINT_SEVERITY_DEFAULT`, `GOMDLINT_FIX`, `GOMDLINT_DRY_RUN`,
`GOMDLINT_JOBS`, `GOMDLINT_FORMAT`, `GOMDLINT_RULE_FORMAT`,
`GOMDLINT_BACKUPS_ENABLED`, `GOMDLINT_BACKUPS_MODE`, `GOMDLINT_NO_BACKUPS`,
`GOMDLINT_IGNORE` (a comma-separated list), `GOMDLINT_GITIGNORE` and
`GOMDLINT_DEFAULT_IGNORES`.

### Migrating from markdownlint

Migrate existing markdownlint configurations with `gomdlint migrate`. When a
project has no gomdlint config, gomdlint translates a markdownlint config
that it finds and applies it for the run. It offers to save the translation
only when stdin and stderr are both terminals.

Migration writes markdownlint's rule defaults and translated options into
`rule_defaults`, with the settings of the source file under `rules`. It
writes no other setting, so `ignore`, `flavor`, `backups` and the other
settings of a user or system config still apply after migration, as they do
while gomdlint translates the markdownlint config for each run. Options are
written under gomdlint's names, such as `max` for `line_length`, and an
option that the rule does not read is left out with a warning. A rule that
the source turns on gets `reset_options: true` (see
[Rule Settings](#rule-settings)), so that it takes markdownlint's defaults
for the options that the source does not set. As in
markdownlint, a migrated config makes every finding an error, so findings
fail the run without `--strict`, and it keeps gomdlint's own rules,
`link-destination-style` (MDL001) and `table-alignment` (MDL003), off.
Migration does not evaluate JavaScript configuration or resolve `extends`.

gomdlint reads a markdownlint config file as markdownlint-cli2 0.23 reads
one that it finds. In a directory with several, it uses the first of
`.markdownlint.jsonc`, `.markdownlint.json`, `.markdownlint.yaml` and
`.markdownlint.yml`, and warns about the others. Whatever the file's name,
gomdlint skips a leading byte order mark, reads the file as JSON with
comments and trailing commas, and reads it as YAML when that fails. So a
`.markdownlint.yaml` file that holds JSON loads, and a key that the JSON
writes twice takes its last value in its first place. A file that holds a
list, a string, a number or nothing loads as an empty config, so every rule
keeps markdownlint's defaults. A value other than an object gets a warning.

gomdlint also reads the `config` key of a markdownlint-cli2 options file,
`.markdownlint-cli2.jsonc` or `.markdownlint-cli2.yaml`, as a markdownlint
config. It looks for both kinds of file upward from the working directory
and uses the nearest directory that holds a markdownlint config file or an
options file that sets `config`. As in markdownlint-cli2, an options file
without `config` passes the search on to the directories above, and one
that sets it to `null`, `false`, `0` or an empty string does too. Without
such a directory, gomdlint reads the nearest options file, so that
markdownlint's defaults apply. In a directory with both kinds of file, a
markdownlint config file wins over the options file's `config` key, with
a warning, as in markdownlint-cli2, except in a parent directory of an
options file that the search passed, where markdownlint-cli2 merges the
options first, so their `config` key wins. `.markdownlint-cli2.jsonc` wins
over `.markdownlint-cli2.yaml`. An options file is read as its name says,
JSON with comments and trailing commas or YAML, and markdownlint-cli2
fails on one that its parser rejects, so it fails the run when it holds
the configuration. Its other keys, such as `globs`, `ignores` and
`customRules`, configure markdownlint-cli2 itself, which gomdlint does not
read, so each gets a warning, in the options file that applies and in a
nearer one that the search passed. gomdlint cannot run a JavaScript options
file, `.markdownlint-cli2.cjs` or `.markdownlint-cli2.mjs`, which gets a
warning too. markdownlint-cli2 merges the options files of the directories
between the working directory and each file, `config` keys included, and
gomdlint reads the `config` key of only one of them.

YAML reads as js-yaml 5 reads it for markdownlint-cli2, with the core
schema of YAML 1.2. So `yes`, `off` and `0b1` are strings, which turn a
rule on, while `0o7` and `0x1F` are numbers. Every key is a string, so a
numeric option such as `0` is an option that the rule does not read. A key
written twice is an error anywhere in the file, even when two spellings
such as `16` and `0x10` only become the same string, and so is a tag that
the core schema lacks, such as `!!binary` or `!unknown`. A `%YAML 1.2`
directive is accepted. A merge key (`<<`) is an ordinary key, which names
no rule, so the settings it would merge do not apply. None of this applies
to gomdlint's own config files.

gomdlint applies a markdownlint config as markdownlint 0.41 does. It
compares keys by their JavaScript upper case, as it does the words of
markdownlint comments. Keys apply in the order they are written, and a
later key overrides an earlier one for the rules both name. For example,
`{"MD009": true, "whitespace": false}` turns `no-trailing-spaces` (MD009)
off. The first `default` key, in any case, sets every rule's baseline
before the other keys apply. A value that JavaScript reads as false, such
as `0`, `""` or `null`, turns rules off. A rule's object turns the rule off
only when its `enabled` key holds such a value, and an object as the
default always turns the rules on. A rule's `severity` gives warnings when
it is `warning` and errors for any other value. So gomdlint's `info` gives
errors in a markdownlint config, while gomdlint's own config files keep it.

A numeric option reads a numeric string, such as `"20"`, `"2e1"` or
`"0x14"`, as the number that JavaScript's `Number` reads from it, as
markdownlint's rules read their numeric options. The items of a list, such
as `lines_above: ["2"]`, stay strings, which count as of the wrong type,
and options of other types read no numbers. An infinite `line_length`,
`heading_line_length`, `code_block_line_length` or `maximum`, such as
YAML's `.inf`, lifts the limit, as in markdownlint. Any other option
ignores an infinite value with a warning, where markdownlint fails or
never finishes.

A key that turns a rule on, with `true`, a severity or an object, gives the
rule the options that it sets and markdownlint's defaults for the others,
as in markdownlint. So `{"MD013": true}` over a user config that sets
`line-length` options gives the rule markdownlint's defaults, where `true`
in a gomdlint config keeps them. A key that turns a rule off, and the
`default` key, leave the options of a user or system config in place,
apart from a few that gomdlint has always set to markdownlint's defaults.

### markdownlint Comments

Markdownlint HTML comments support `disable`, `enable`, `disable-line`,
`disable-next-line`, `disable-file`, `enable-file`, `capture`, and
`restore`, with rule IDs, aliases, or tags. Suppressed findings produce no
fixes, and a finding on another line keeps no fix that would change the
text of a line where its rule is disabled, such as the fix that MD049 gives
both markers of an emphasis that spans two lines. markdownlint fixes the
enabled marker alone there and leaves the emphasis unbalanced. A blank line
inserted next to such a line leaves it as it was and stays. Comments inside
code or frontmatter do not change lint settings, while markdownlint also
reads comments inside code blocks. As in markdownlint, whitespace or `-->`
must follow the comment's kind, so a comment such as
`<!-- markdownlint-disable-next-lines MD009 -->` is plain text and does
nothing. Unlike markdownlint, gomdlint also reads a comment
split over several lines, and it reads every comment after one that lacks
its closing `-->`, where markdownlint can skip some.

A word in such a comment that names no rule or tag does nothing, as in
markdownlint. gomdlint warns about it, and names the rules and tags that the
word most likely means when some are close. gomdlint's former tags `style`,
`lists`, `layout`, `metadata`, `gfm` and `tables` still work, but markdownlint
does not know them. Each use gets a warning that names the rules the tag
covers, and the tags will stop working in a later release. These warnings name
rules as markdownlint does, such as `ul-style` for `unordered-list-style`, so
that the comment works in markdownlint too. The warnings go to stderr in every
output format. JSON output also lists them under each file's `notices`, and
SARIF output as `toolConfigurationNotifications`. They never change findings
or the exit code, even with `--strict`.

`markdownlint-configure-file` accepts an object of markdownlint settings and
applies it to the whole file. As in markdownlint-cli2, the object may be
written as JSON with comments and trailing commas, or as YAML, which reads
as in a markdownlint config file. Like markdownlint, gomdlint never fails a
file over such a comment. A comment that holds no object changes nothing. A
key that names no rule configures nothing, and the other keys still apply.
An option that the rule does not read changes nothing, and that includes
gomdlint's spelling of an option, as in a markdownlint config. A value of
the wrong type, such as `"yes"` for a boolean or a list with a number
among strings, counts as unset, and a string that the option does not
allow applies as markdownlint applies it, as in a config file. Each of
these gets a warning on stderr, in JSON `notices` and in SARIF
`toolConfigurationNotifications`, and none changes the findings or the exit
code. As in markdownlint, the keys apply in the order they are written, and
the first `default` key, in any case, sets every rule's baseline. A key
that turns a rule on gives it the options that it sets and markdownlint's
defaults for the others, whatever the config gives the rule. The
configure-file comments of one file act as one object. A key that a later
comment writes again keeps its first place and takes the later value. A
comment without a `-->` of its own runs on to the `-->` of a later one, as
in markdownlint, so it holds the comments after it.

## Markdown Support

Supports both CommonMark and GitHub Flavored Markdown (GFM) via `--flavor`
or the `flavor` setting. The default is `gfm`, as in markdownlint, which
parses tables, task lists, strikethrough and autolink literals. With
`commonmark`, a table is a paragraph, so the table rules find nothing.

## CI Integration

Use `--strict` to treat warnings as errors for CI pipelines. JSON and SARIF
output formats integrate with analysis tools and GitHub's code scanning. The
exit code tells a CI job what happened:

| Code | Meaning |
| --- | --- |
| 0 | No errors, and no warnings with `--strict` |
| 1 | Error findings |
| 2 | Warning findings with `--strict` |
| 64 | Invalid command-line usage |
| 65 | Invalid configuration |
| 70 | An internal error, a rule that failed, or an interrupted run |
| 74 | A file or directory could not be read or written |

When several apply, 74 comes first, then 70, then the codes for findings.
Info findings never change the exit code, even with `--strict`. gomdlint
names each file it could not lint and each rule that failed on stderr.

```yaml
# GitHub Actions example
- name: Lint Markdown
  run: gomdlint lint --strict --format sarif > results.sarif

- name: Upload SARIF
  if: always()
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

The upload step runs even when the lint step fails on findings, and it needs
the `security-events: write` permission.

## Commands

| Command | Description |
| --- | --- |
| `gomdlint lint [paths...]` | Lint files and directories |
| `gomdlint rules [words...]` | List the rules by category |
| `gomdlint explain <rule>` | Explain what a rule checks and how to configure it |
| `gomdlint init` | Generate configuration file |
| `gomdlint migrate` | Convert markdownlint config |
| `gomdlint completion <shell>` | Generate a shell completion script |
| `gomdlint version` | Show version information |

`gomdlint rules` lists the rules by category, sorted by name. Each row shows
a rule's name, ID, whether it can fix what it finds, its status and what it
checks. The status is `off` or the severity of the rule's findings, under
the configuration that `gomdlint lint` would load in the current directory
or from `--config`. `--defaults` shows the built-in settings instead.

Words select the rules that hold each of them in their name, ID,
markdownlint name, category, tags or description, ignoring case.
`--category`, `--tag`, `--fixable`, `--enabled` and `--disabled` filter
further, and a rule must pass every filter. `--rule-format` names rules as
for `lint`. `--format json` describes every rule with its options and their
defaults, and `--format markdown` prints a Markdown table for each category.

```bash
gomdlint rules table                 # Rules that mention tables
gomdlint rules --tag whitespace      # The rules of a markdownlint tag
gomdlint rules --disabled            # The rules this project turns off
gomdlint rules --format json         # Every rule and its options, as JSON
```

`gomdlint explain` takes a rule's ID, name or markdownlint name, in any
case. It shows what the rule checks and why, its options with their
defaults and markdownlint's spellings, what its fix does and which findings
it reports without a fix, and how to configure the rule and silence it on
one line. A markdownlint tag, such as `whitespace`, or a category, such as
`layout`, lists its rules instead. `--format markdown` prints the rule's
documentation page. `--format json` describes the rule as
`gomdlint rules --format json` does.

```text
$ gomdlint explain MD009
no-trailing-spaces (MD009), Whitespace and layout
On by default, severity warning. Fixable.

Outside code blocks, lines end in no spaces or tabs, or in exactly 2 of them.

Why: Trailing whitespace in text does not change the output, except where 2 or
more spaces make a hard line break. In code blocks it is part of the code.
...
```

`gomdlint completion` writes a script for bash, zsh, fish or powershell.
Release archives and the Homebrew formula include the bash, zsh and fish
completions.

## Known Limitations

gomdlint parses Markdown with goldmark, which reads a few constructs
differently from GitHub and from micromark, the parser behind markdownlint.
Some of its fix checks are also stricter than they need to be.

- Twelve list and whitespace rules withhold every fix in a document that
  holds any raw HTML other than well-formed comments and start tags of
  `br`, `wbr`, `hr` and `img` with only presentational attributes, because
  other HTML can change how the whole page shows. The attributes allowed
  are `src`, `alt`, `title`, `width`, `height`, `align`, `border`,
  `loading`, `decoding`, `srcset` and `sizes` on `img`, `clear` on `br`,
  and `align`, `width`, `size` and `noshade` on `hr`, each used once. So
  `<br>` keeps the fixes, while `<br onclick="noop()">`, an end tag such as
  `</br>`, a comment with `--` inside it, or one inline `<kbd>` withholds
  them. A document with a `<` that may start a tag, one followed by a
  letter, `/`, `!` or `?`, gets no fix either when it also holds `[^`
  anywhere, even with only `<br>` tags. GitHub writes the HTML of GFM
  footnotes after the document, and gomdlint does not read footnotes as
  GitHub does, so it treats any `[^` as a possible footnote, including one
  in a code span or an undefined reference. The same holds for a document
  with TOML or JSON front matter, which GitHub renders as Markdown, and
  such a `<` anywhere. The rules are `unordered-list-style` (MD004),
  `list-indent` (MD005), `ul-indent` (MD007),
  `no-trailing-spaces` (MD009), `no-hard-tabs` (MD010),
  `no-multiple-blank-lines` (MD012), `line-length` (MD013),
  `no-multiple-space-blockquote` (MD027), `ol-prefix` (MD029),
  `list-marker-space` (MD030), `blanks-around-lists` (MD032) and
  `single-trailing-newline` (MD047). Their findings are still reported.
- `blanks-around-tables` (MD058) reports a table in a list without a fix
  when an item of the list holds an HTML block, or a fenced code block
  without a closing fence, outside a blockquote. goldmark can then read a
  tight list as loose, so the fix is withheld even in a list that is loose.
- goldmark reads `> -  first item` followed by `> <tab>- second item`, where
  `<tab>` is a tab character, as one list item that continues with the text
  `- second item`. CommonMark reads two items, so the list rules see one
  item where markdownlint sees two.
- Inside a blockquote or list item, goldmark takes a following line that
  lacks the container's marker into a table. For `> | a | b |`, `> | - | - |`
  and then `| 1 | 2 | 3 |`, gomdlint reports `table-column-count` (MD056)
  where markdownlint reports `blanks-around-tables` (MD058).
- goldmark reads some tables that GitHub and markdownlint read as
  paragraphs, such as a header row with fewer cells than its delimiter row.
  There `no-bare-urls` (MD034) reads cells, so a `www.` address right after
  a pipe is reported, and an address that runs across a pipe is reported as
  two. MD034 never fixes a URL in such a block.
- A file that ends with `> foo` and then `--`, without a final newline, gets
  a `single-trailing-newline` (MD047) finding without a fix. With the
  newline, goldmark reads the two lines as a table, while GitHub reads a
  paragraph.
- Two kinds of document still lint slowly. Removing a long run of unused
  reference definitions with `link-image-reference-definitions` (MD053)
  takes time that grows with the square of the run's length. Many fixes
  inside one large block cost a parse of that block each, unless the block
  is a top-level blockquote, or a list or table at the top level or in a
  top-level blockquote. Such blocks include a long paragraph, a table read
  with the `commonmark` flavor, which has no tables, and a blockquote
  inside another blockquote or a list item.
- goldmark parses no math, so no rule skips it. `no-reversed-links`
  (MD011), `no-inline-html` (MD033), `no-alt-text` (MD045) and
  `reference-links-images` (MD052) report in a `$$` math block and in
  inline math such as `$(a)[b]$`, which markdownlint skips, and
  `no-duplicate-heading` (MD024) and `descriptive-link-text` (MD059) read
  a tag or code span in inline math as HTML or code.
- `reference-links-images` (MD052) reads GFM footnotes as markdownlint does
  only where micromark is certain to read the definition: a `[^1]:` line at
  column 0, at the top level outside code blocks, HTML blocks and front
  matter, at the start of the document, after a blank line or right after
  another such definition. No line before it may start with `$$` or `:::`,
  each code fence before it must start at column 0 after a blank line, where
  goldmark reads it at the top level, and no HTML block before it that may
  hold blank lines, such as a comment or a `<pre>` block, may go on past its
  first line. Only a link whose label after the `^` holds nothing but ASCII
  letters, digits and hyphens, such as `[^1]` or `[^my-note]`, calls such a
  definition, and only one with the same label in any case, after white
  space is collapsed and trimmed. Every other reference is read as goldmark
  reads it, and reported when no link reference definition matches, even
  where markdownlint reads a footnote call. That covers an image such as
  `![^1]`, a label such as `[^my_note]`, `[^a\*b]` or `[^É]`, and a call of
  a definition in a list item, a blockquote or a footnote. With
  `shortcut_syntax`, `[text][^1]` is reported, even when markdownlint finds
  `text` defined, unless a link reference definition such as `[^1]: /url`
  defines its label, `^1` is one of `ignored_labels`, or `text` is blank or
  holds nothing but ASCII letters, digits, spaces and tabs and, lowercased
  with its white space collapsed, is one of `ignored_labels`. No definition
  counts in a document whose front matter markdownlint may read differently,
  or where the text markdownlint gives micromark starts with a byte order
  mark, as with two at the start of the file or one right after front
  matter.
- `link-image-reference-definitions` (MD053) reads GFM footnote definitions,
  such as `[^1]: A note.`, only where `reference-links-images` (MD052) counts
  them, and only when the label holds nothing but ASCII letters, digits and
  hyphens. It reports such a definition as unused only when `^` and its label
  appear nowhere else in the file, in any case and not even in code, so it
  never reports a footnote that markdownlint finds called. It reports the
  second definition of one label as a duplicate. So it misses unused
  footnotes defined in a list item or a blockquote, with a label such as
  `[^my_note]`, or mentioned only in code, which markdownlint reports. MD053
  deletes no definition whose label starts with `^`, which GitHub reads as a
  footnote whose note may go on in indented lines after a blank line, and
  reports it without a fix.
- `link-fragments` (MD051) accepts the anchors that GitHub gives repeated
  headings by github-slugger's rules. After `## Release`, `## Release` and
  `## Release 1`, GitHub's anchors are `#release`, `#release-1` and
  `#release-1-1`, because the second heading takes `#release-1` first.
  markdownlint gives the third heading `#release-1` as well, so it reports a
  link to `#release-1-1`.
- `reference-links-images` (MD052) and `link-image-reference-definitions`
  (MD053) match a reference with its definition by CommonMark's Unicode case
  folding, as GitHub and goldmark do, so `[Straße][]` uses
  `[STRASSE]: /url`. markdownlint lowercases labels instead, so it reports
  `[Straße][]` as undefined and the definition as unused.
- gomdlint reads a markdownlint comment split over two lines, which
  markdownlint ignores, and ignores markdownlint comments inside code
  blocks, which markdownlint honours.
- markdownlint merges `markdownlint-configure-file` comments into its own
  configuration object, where a key that the configuration holds already
  keeps its earlier place. Without a configuration file, that object is
  `{"default": true}`, so there a comment's `Default` key, written in
  another case, changes nothing. gomdlint applies the comments over its own
  settings, so their keys always apply in the order the comments write them.
- markdownlint reads every `markdownlint-configure-file` comment in full,
  so comments without a `-->` of their own, each of which holds every
  comment after it up to a shared `-->`, take time that grows with the
  square of their number: about 51 seconds for 8,000. gomdlint reads such
  comments only while their text comes to at most four times the file's
  length and 1 MiB more, and ignores the rest, each with a warning. A
  comment with its own `-->` is always read.
- gomdlint parses YAML with yaml.v3, which follows libyaml, and reads what
  it parses as js-yaml 5 does. A few constructs parse differently. js-yaml
  parses an empty value with only a tag, such as `[!!str]`, an empty key,
  such as `: x`, and the escapes `\/` and `\ud83d` in double-quoted
  strings, which fail in gomdlint. gomdlint parses `[-]` and `&a: x`, which
  fail in js-yaml. yaml.v3 ends a line at U+2028 and U+2029, which js-yaml
  reads as text. A config that fails to parse fails to load, and a
  `markdownlint-configure-file` comment that fails configures nothing, with
  a warning. An alias inside the value it names, such as `&r [*r]`, gives
  js-yaml a value that holds itself, and gomdlint an empty list or object in
  place of the alias.
- markdownlint-cli2 also tries TOML, between JSON and YAML, when it reads a
  markdownlint config or a `markdownlint-configure-file` comment. gomdlint
  does not read TOML. A config written in TOML gets a warning or a config
  error, and a comment written in TOML configures nothing, with a warning.
- markdownlint-cli2 reads markdownlint config files and options files only
  in the directory it runs from and the directories below it. gomdlint
  also looks for them upward from the working directory, so a
  `.markdownlint-cli2.jsonc` file in a parent directory configures a run
  from a subdirectory in gomdlint but not in markdownlint-cli2.
- markdownlint-cli2 reads a `.markdownlint.json` or `.jsonc` file that
  starts with a byte order mark as YAML, because its JSON parser rejects the
  mark. YAML reads each comment in such a file together with the key after
  it as one key that names nothing, so that setting does not apply there.
  gomdlint skips the mark and reads the file as JSON with comments.
- Most of markdownlint's rules read the number 0 for an option such as
  `line_length` or `maximum` as unset, and a numeric string that reads as
  0, such as `"0"`, as 0 itself. In a markdownlint config, gomdlint reads
  such a string for `line-length` as -1, which turns the length check off
  as 0 does in markdownlint. For `maximum` and the spacings of
  `list-marker-space`, where markdownlint then reports every blank line or
  every list item, gomdlint has no such value, so it ignores the string
  with a warning and uses the default.
- markdownlint reads an empty string for `no-trailing-spaces`'
  `br_spaces`, `no-hard-tabs`' `spaces_per_tab` and
  `blanks-around-headings`' `lines_above` and `lines_below` as 0. gomdlint
  ignores it with a warning and uses the default.
- markdownlint's `no-multiple-blanks` reports every line of a document for
  a `maximum` below 0, such as -1 or `-Infinity`. gomdlint reads any
  maximum below 1 as 1. markdownlint compares lengths and counts with a
  fractional limit, such as `"line_length": 100.5`, as it is, and gomdlint
  ignores it with a warning.
- markdownlint rejects a set of custom rules in which two IDs or names, or
  a name and a tag, are equal in upper case. gomdlint's Go API accepts
  such rules. An ID wins over a name. Of two rules with the same name,
  the one registered last keeps it, and of two names equal only in upper
  case, the spelling that sorts first wins. A word that is a rule's name
  and a tag of a catalog entry names the rule and the tag's rules, while a
  tag that a rule reports itself yields to the name.
- markdownlint compares a comment's word with a custom rule's whole name,
  so no word names a rule registered with whitespace around its name.
  gomdlint ignores that whitespace, as its config files do, and suggests
  the rule's ID, which both read.
- markdownlint's rules read options of other types with JavaScript's
  conversions too, such as `"strict": "false"`, which reads as true, and a
  number in a list of strings, which reads as its text. gomdlint reads only
  numeric strings for numeric options this way, and ignores other values of
  the wrong type with a warning.

## Development

This project uses [stave](https://github.com/yaklabco/stave) for build
automation.

### Prerequisites

- Go 1.25.4 or later. `go.mod` selects the Go 1.26.9 toolchain for this
  repository, which the `go` command downloads on first use.
- [golangci-lint](https://golangci-lint.run/usage/install/) v2.11.4, or
  another release built with Go 1.26 or later
- [stave](https://github.com/yaklabco/stave):
  `go install github.com/yaklabco/stave@latest`

### Build Tasks

```bash
stave build      # Build bin/gomdlint
stave test       # Run tests with the race detector
stave lint       # Run golangci-lint and apply its fixes
stave ci:gate    # Run the CI checks, apart from the Markdown lint
stave check      # Format, lint and test
stave install    # Install to GOPATH/bin
stave -l         # List all available tasks
```

CI also lints the repository's Markdown, which `stave ci:gate` does not.
To run that check locally:

```bash
go run ./cmd/gomdlint lint --strict README.md CHANGELOG.md CI_CD.md docs CLAUDE.md AGENTS.md
```

[CI_CD.md](CI_CD.md) describes the CI workflows and the release process.

### Project Structure

```text
gomdlint/
  cmd/gomdlint/        # CLI entry point
  internal/
    cli/               # Cobra commands and flag handling
    configloader/      # Config discovery, loading, merging and migration
    display/           # UTF-16 columns and paths for reporters
    jstext/            # UTF-16 lengths of text, as markdownlint measures them
    editdistance/      # Edit distance for rule name suggestions
    ignore/            # gitignore matching for directory walks
    logging/           # Structured logging setup
    ui/pretty/         # Lipgloss-based styled output
  pkg/
    analysis/          # Result aggregation and statistics
    config/            # Core config types
    fix/               # Text edits, fix resolution, line endings, diffs
    fsutil/            # Pinned file writes and backups
    langdetect/        # Code language detection for MD040
    lint/              # Rule interface, catalog types, registry, engine
    lint/mdscan/       # Scanners for inline HTML and autolink literals
    lint/refs/         # Reference definitions, usages and heading anchors
    lint/rules/        # The 55 built-in rules, their catalog, fix checks
    mdast/             # AST types, source spans, FileSnapshot
    parser/goldmark/   # Goldmark-based parser with exact source spans
    reporter/          # Formatters: text, table, JSON, SARIF, diff, summary
    runner/            # File discovery and the concurrent runner
```

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the complete architectural
design, including layer diagrams, extension points, and design patterns.

## License

MIT License - see [LICENSE](LICENSE) for details.
