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

## Rule Categories

**Headings** - Enforce heading level increments (no jumping from H1 to H3),
a consistent style (ATX or setext), spacing around headings and after their
hashes, unique heading text, a single top-level heading, no trailing
punctuation, a heading on the first line, and an optional required heading
structure. Emphasis used as a heading becomes a real heading, one level
below the heading before it. Most heading issues auto-fix.

**Lists** - Ensure consistent bullet markers, proper indentation at each
nesting level, correct ordered list numbering, and appropriate spacing.
List formatting issues auto-fix to your configured style, and a fix never
changes the number a list starts with.

**Layout** - Remove trailing spaces, convert tabs to spaces, collapse
multiple blank lines, keep a consistent horizontal rule style, and ensure
files end with a newline. Tabs in code text are reported but never changed.
Line length is enforced with word wrapping that applies to paragraph text
only, and keeps blockquote and list prefixes, code spans, inline HTML and
link destinations intact.

**Code** - Require language identifiers on fenced code blocks, enforce a
consistent code block and fence style, and ensure blank lines around fences,
no spaces inside code spans, and no `$` prompts before commands without
output. A missing language is added only when the whole block shows which
one it is: a shebang line, JSON, an XML document, an HTML page with a
doctype, a PHP opening tag, a complete unified diff, or a Dockerfile. When
`allowed_languages` is set, the language must also be one it names, with
the same case.

**Links** - Detect reversed link syntax, bare URLs, spaces inside link text,
empty links, missing image alt text, broken link fragments, undefined and
unused reference definitions, inconsistent link and image style, and
non-descriptive link text such as "click here". gomdlint's own
`link-destination-style` (MDL001) keeps relative links and local `file:`
URLs from being mixed. It never reports a link to another host, which
cannot be relative. gomdlint cannot tell an absolute URL of the document's
own site from one of another host, so it never reports those either.
Reversed links, spaces in link text, unused definitions and bare URLs
auto-fix.

**Tables** (GFM) - Validate table structure including consistent column
counts, pipe style, column spacing and surrounding blank lines. gomdlint's
own `table-alignment` (MDL003) formats the delimiter row.

**Emphasis** - Detect spaces inside emphasis markers and inconsistent
emphasis and strong style. Emphasis issues auto-fix, except where changing a
marker could change which markers pair up.

**Blockquotes** - Detect multiple spaces after `>` and blank lines inside
blockquotes.

**HTML** - Report raw HTML, with an optional list of allowed elements.

**Spelling** - Enforce the capitalization of the proper names you configure.

Every rule has an ID and a name. markdownlint's own names for five rules
also work: `ul-style` (MD004), `no-multiple-blanks` (MD012),
`blanks-around-headings` (MD022), `single-title` (MD025) and `first-line-h1`
(MD041). `gomdlint rules` lists every rule by category, with its status and
a one-line description of what correct Markdown looks like, and
`gomdlint explain <rule>` describes one rule in full (see
[Commands](#commands)).

| ID | Name | Category | Fix |
| --- | --- | --- | --- |
| MD001 | `heading-increment` | headings | no |
| MD003 | `heading-style` | headings | yes |
| MD004 | `unordered-list-style` | lists | yes |
| MD005 | `list-indent` | lists | yes |
| MD007 | `ul-indent` | lists | yes |
| MD009 | `no-trailing-spaces` | layout | yes |
| MD010 | `no-hard-tabs` | layout | yes |
| MD011 | `no-reversed-links` | links | yes |
| MD012 | `no-multiple-blank-lines` | layout | yes |
| MD013 | `line-length` | layout | yes |
| MD014 | `commands-show-output` | code | yes |
| MD018 | `no-missing-space-atx` | headings | yes |
| MD019 | `no-multiple-space-atx` | headings | yes |
| MD020 | `no-missing-space-closed-atx` | headings | yes |
| MD021 | `no-multiple-space-closed-atx` | headings | yes |
| MD022 | `heading-blank-lines` | headings | yes |
| MD023 | `heading-start-left` | headings | yes |
| MD024 | `no-duplicate-heading` | headings | no |
| MD025 | `single-h1` | headings | no |
| MD026 | `no-trailing-punctuation` | headings | yes |
| MD027 | `no-multiple-space-blockquote` | blockquotes | yes |
| MD028 | `no-blanks-blockquote` | blockquotes | no |
| MD029 | `ol-prefix` | lists | yes |
| MD030 | `list-marker-space` | lists | yes |
| MD031 | `blanks-around-fences` | code | yes |
| MD032 | `blanks-around-lists` | lists | yes |
| MD033 | `no-inline-html` | html | no |
| MD034 | `no-bare-urls` | links | yes |
| MD035 | `hr-style` | layout | yes |
| MD036 | `no-emphasis-as-heading` | headings | yes |
| MD037 | `no-space-in-emphasis` | emphasis | yes |
| MD038 | `no-space-in-code` | code | yes |
| MD039 | `no-space-in-links` | links | yes |
| MD040 | `fenced-code-language` | code | yes |
| MD041 | `first-line-heading` | headings | no |
| MD042 | `no-empty-links` | links | no |
| MD043 | `required-headings` | headings | no |
| MD044 | `proper-names` | spelling | yes |
| MD045 | `no-alt-text` | links | no |
| MD046 | `code-block-style` | code | no |
| MD047 | `single-trailing-newline` | layout | yes |
| MD048 | `code-fence-style` | code | yes |
| MD049 | `emphasis-style` | emphasis | yes |
| MD050 | `strong-style` | emphasis | yes |
| MD051 | `link-fragments` | links | no |
| MD052 | `reference-links-images` | links | no |
| MD053 | `link-image-reference-definitions` | links | yes |
| MD054 | `link-image-style` | links | no |
| MD055 | `table-pipe-style` | tables | no |
| MD056 | `table-column-count` | tables | no |
| MD058 | `blanks-around-tables` | tables | yes |
| MD059 | `descriptive-link-text` | links | no |
| MD060 | `table-column-style` | tables | no |
| MDL001 | `link-destination-style` | links | no |
| MDL003 | `table-alignment` | tables | yes |

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

Columns count characters, so a finding after multibyte text points at its
target. JSON output reports the same character columns, while the offsets of
its fixes are byte offsets. Every format except SARIF shows a path relative
to the working directory when the file lies inside it.

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
reads the first name in this list and ignores the others:

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
`/etc/gomdlint`, or `%ProgramData%\gomdlint` on Windows.

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
Nine options have a gomdlint name that differs, and gomdlint accepts both
spellings:

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
an error in a config file. A `markdownlint-configure-file` comment that
sets both reads markdownlint's spelling, as markdownlint does, and warns
that gomdlint's spelling is ignored. Options that markdownlint lacks keep
markdownlint's behaviour by default.

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
`.markdownlint.yml`. Whatever the file's name, gomdlint skips a leading
byte order mark, reads the file as JSON with comments and trailing commas,
and reads it as YAML when that fails. So a `.markdownlint.yaml` file that
holds JSON loads, and a key that the JSON writes twice takes its last value
in its first place. A file that holds a list, a string, a number or nothing
loads as an empty config, so every rule keeps markdownlint's defaults. A
value other than an object gets a warning.

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
fixes. Comments inside code or frontmatter do not change lint settings,
while markdownlint also reads comments inside code blocks. As in
markdownlint, whitespace or `-->` must follow the comment's kind, so a
comment such as `<!-- markdownlint-disable-next-lines MD009 -->` is plain
text and does nothing. Unlike markdownlint, gomdlint also reads a comment
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
An option that the rule does not read changes nothing. Of two spellings of
one option, markdownlint's applies. Each of these gets a warning on stderr,
in JSON `notices` and in SARIF `toolConfigurationNotifications`, and none
changes the findings or the exit code. As in markdownlint, the keys apply
in the order they are written, and the first `default` key, in any case,
sets every rule's baseline. A key that turns a rule on gives it the options
that it sets and markdownlint's defaults for the others, whatever the
config gives the rule. The configure-file comments of one file act as
one object. A key that a later comment writes again keeps its first place
and takes the later value. A comment without a `-->` of its own runs on to
the `-->` of a later one, as in markdownlint, so it holds the comments
after it.

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
  them. The rules are `unordered-list-style` (MD004), `list-indent`
  (MD005), `ul-indent` (MD007), `no-trailing-spaces` (MD009),
  `no-hard-tabs` (MD010), `no-multiple-blank-lines` (MD012), `line-length`
  (MD013), `no-multiple-space-blockquote` (MD027), `ol-prefix` (MD029),
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
- markdownlint-cli2 reads a `.markdownlint.json` or `.jsonc` file that
  starts with a byte order mark as YAML, because its JSON parser rejects the
  mark. YAML reads each comment in such a file together with the key after
  it as one key that names nothing, so that setting does not apply there.
  gomdlint skips the mark and reads the file as JSON with comments.

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
    display/           # Character columns and paths for reporters
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
