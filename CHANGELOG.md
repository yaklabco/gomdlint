# Changelog

## [Unreleased]

Most rules now report what markdownlint 0.41 reports, with its defaults, and
every fix is checked before it is applied. Several defaults and output
formats change. The first entries under Changed may need action.

### Changed

- Text output prints each finding as location, severity, rule name and
  message, the order markdownlint-cli2 uses, as in
  `README.md:3:22  warning  no-trailing-spaces  Trailing whitespace`. The
  rule used to come last, in parentheses. Scripts and editor problem matchers
  that read text output need updating. JSON and SARIF output remain the
  stable machine formats.
- Text, table and summary output name rules by name. `--rule-format id` shows
  the ID instead, `--rule-format combined` shows both, and the
  `GOMDLINT_RULE_FORMAT` environment variable sets the default. The summary
  no longer cuts long rule names.
- Every rule runs by default, as in markdownlint. `no-inline-html` (MD033),
  `first-line-heading` (MD041), `required-headings` (MD043), `proper-names`
  (MD044), `link-image-style` (MD054), `descriptive-link-text` (MD059),
  `table-column-style` (MD060) and `link-destination-style` (MDL001) used to
  be off. MD043 and MD044 check nothing until they are given headings or
  names, and those options alone now turn them on. Turn any rule off by name
  or ID, with `no-inline-html: false` or `MD033: false` under `rules`, or
  with `--disable no-inline-html`.
- Rule options default to markdownlint 0.41's values: consistent styles for
  MD003, MD004, MD035, MD046, MD048, MD049 and MD050, `one_or_ordered` for
  MD029, `br_spaces: 2` for MD009, 80 characters for MD013, and no allowed
  elements for MD033. MD047 only asks for a final newline. Options that
  markdownlint lacks default to markdownlint's behaviour.
- The default flavor is `gfm`, as in markdownlint, so tables and autolink
  literals are parsed unless a config or `--flavor` chooses `commonmark`.
- A config file whose name ends in `.json` must hold JSON. Older versions of
  `init` and `migrate` could write YAML into such a file. It now fails to
  load, and the error says to rename it to `.yml`.
- `--color always` colours output that goes to a pipe or a file, and it
  overrides `NO_COLOR`, because the flag asks for colour explicitly.
- Exit codes tell failures apart: 1 for error findings, 2 for warnings with
  `--strict`, 64 for invalid usage, 65 for an invalid configuration, 70 for
  an internal error, a failing rule or an interrupted run, and 74 when a file
  or directory could not be read or written. Warnings alone still exit 0. A
  file that could not be read used to print "No issues found" and exit 0,
  and every other failure exited 1.
- Text, table, JSON and SARIF output list each file's findings by line, then
  column, rule ID and message. They used to follow rule ID order.
  markdownlint-cli2 also lists findings by line, but orders the findings on
  one line by rule ID.
- Table rows start with an `E`, `W` or `I` severity marker, and the legend
  lists only the markers that the rows show, so a table reads the same
  without colour.
- Columns count characters instead of bytes in text, table and JSON output,
  so a finding after multibyte text points at its target. JSON fix offsets
  stay byte offsets. Every human-readable format shows a path relative to the
  working directory when the file lies inside it.
- Fixes are checked before they are applied. Each fixing rule parses the
  document again with its fixes and withholds any fix that would change how
  the document renders in a way the rule does not intend. The finding is
  still reported. `no-bare-urls` (MD034) now fixes a URL only where GitHub,
  micromark and goldmark all link the same address, so URLs in tables,
  emphasis or quotes and next to character references are reported without
  a fix. `fenced-code-language` (MD040) adds a language only when the whole
  block shows it, such as a shebang, JSON or a complete diff.
- `no-hard-tabs` (MD010) reports tabs in code, as markdownlint does, but its
  fix never changes code text.
- In each fix pass, the fix engine applies the fixes of one rule per
  top-level block and leaves the other rules' fixes there for the next pass,
  so a file with many findings can take more passes. The pass limit grows
  from 10 to 10 plus one for each rule that can fix.
- Directory walks skip files that `.gitignore` files ignore, and the
  `node_modules` and `vendor` directories. `--no-gitignore`,
  `--no-default-ignores`, the `gitignore` and `default_ignores` settings, and
  their environment variables turn this off. Paths named on the command line
  are always linted.
- Ignore patterns follow `.gitignore` matching rules and are resolved against
  the directory of the config file that sets them, so `**/a.md` no longer
  matches `data.md`.
- `--config` replaces project config discovery, `--enable` overrides config
  files, and `--disable` wins over `--enable` for the same rule.
- When a project has only a markdownlint config, gomdlint translates it and
  applies it for the run, in CI too. The offer to save the translation
  appears on stderr, and only when stdin and stderr are both terminals.
- Inline markdownlint comments resolve tags as markdownlint 0.41 does.
  `bullet` and `ul` cover MD004, `blank_lines` covers MD012 and `language`
  covers MD040, while `whitespace` and `links` no longer cover MD022 and
  MD045. `url` and `table` also cover gomdlint's MDL001 and MDL003.
- Inline markdownlint comments follow markdownlint 0.41.1's grammar, so the
  comment's kind must be followed by whitespace or `-->`. A comment such as
  `<!-- markdownlint-disable-next-lines MD009 -->` is plain text, and no
  longer disables MD009 for the rest of the file.
- A `markdownlint-configure-file` key that is empty or holds whitespace is an
  unknown key and fails the file. `{"MD009 MD010": false}` used to disable
  both rules, and `{"": false}` every rule. markdownlint ignores both.
- Migrated markdownlint configs, and markdownlint configs that gomdlint
  applies for a run, match rule names and tags in any case, accept gomdlint's
  rule names, and skip unknown rule IDs with a warning instead of copying
  them.
- Go API: `lint.LookupRuleID`, `Registry.RegisterAlias`,
  `rules.RegisterLegacyAliases`, `RuleContext.Builder` and
  `analysis.Options.RuleFormat` are removed. `Registry.Lookup` resolves rule
  IDs, names and aliases, and `Registry.Resolve` now ignores case and
  surrounding whitespace too. The SARIF types in `pkg/reporter` changed shape
  to produce valid SARIF 2.1.0. `PipelineOptions.ReParseAfterFix` is
  deprecated and has no effect, and `pkg/lint` no longer builds for wasip1.

### Added

- Config discovery finds `.gomdlint.json` and `gomdlint.json` after the YAML
  names, and `config.json` in the user and system config directories. `init`
  and `migrate` write JSON for an output name that ends in `.json`.
- A bare `true` or `false` turns a rule on or off, as in markdownlint, such
  as `line-length: false` in YAML or `"MD013": false` in JSON.
- A catalog describes every rule: its ID, its name, the markdownlint names
  that differ from it, its category and its markdownlint tags. A rule can be
  named by ID, name or markdownlint name, in any case, in config files, in
  the rule flags, in inline comments and in `markdownlint-configure-file`.
- `--enable`, `--disable` and `--fix-rules` accept markdownlint tags, such as
  `whitespace`, which stand for every rule they cover.
- Unknown rule names get suggestions in the rule flags, config files,
  migrated configs and `markdownlint-configure-file`, as in
  `--disable: unknown rule "no-trailling-spaces" (did you mean
  "no-trailing-spaces"?)`. All bad flag values are reported together, and
  config warnings name the file and line.
- Shell completion completes rule names, IDs, markdownlint names and tags
  for `--enable`, `--disable` and `--fix-rules`.
- Warnings for words in markdownlint comments that name no rule or tag, with
  suggestions that use markdownlint's rule names, and for gomdlint's former
  tags `style`, `lists`, `layout`, `metadata`, `gfm` and `tables`. The former
  tags still work, but will stop working in a later release. The warnings go
  to stderr, JSON output lists them under each file's `notices`, and SARIF
  output records them as `toolConfigurationNotifications`. They change no
  finding or exit code.
- JSON output reports rules that failed, under `summary.ruleErrors` and each
  file's `ruleErrors`.
- `--format diff` previews the fixes without writing them, and
  `--fix --format diff` prints the diff of the fixes it wrote.
- `no-hard-tabs` (MD010) supports `ignore_code_languages`, matched against
  the first word of a fence's info string.
- `init --full` documents every rule with its default state and severity,
  and `init --format json` writes the same settings as the YAML template.

### Fixed

- **MD001**, **MD025**: A front matter title counts as the document's first
  heading, as in markdownlint.
- **MD003**: Recognizes setext headings, which it used to read as ATX.
- **MD018**, **MD020**: No longer edit `#notaheading` inside HTML comments,
  or turn the escaped hash in `## Learn C\#` into a stray backslash.
- **MD019**, **MD021**: No longer both report `##  Spaced heading  ##`, and
  they check headings in blockquotes and lists.
- **MD022**: No longer inserts a blank line between a setext heading and its
  underline, and withholds blank lines that would loosen a tight list.
- **MD023**: No longer moves a heading at a list item's content column out
  of the list.
- **MD026**: Decides from the heading's source, so `` ## Run `make.` `` keeps
  its closing backtick. It no longer empties a heading that would show
  nothing without its punctuation, such as `## .`. Such headings are reported
  without a fix.
- **MD035**: Edits only the thematic break, so a break no longer moves out
  of its list item, and it withholds a change that would turn a break under
  a paragraph into a setext heading underline.
- **MD036**: Keeps code and links when it turns emphasis into a heading.
- **MD037**: Reads the delimiter runs that the parser left unmatched, so it
  no longer pairs unrelated markers such as those in the list item
  `* spaced * text.`. Arithmetic such as `a * b * c` is reported without a
  fix.
- **MD041**: Accepts a leading HTML heading.
- **MD043**: Keeps checking after its list of headings is used up, using a
  port of markdownlint's matcher.
- **MD044**: No longer rewrites names inside link destinations, URLs and
  HTML attributes. Names in raw HTML are reported without a fix.
- **MD049**, **MD050**: Fix the style they report. They were listed as
  fixable but never fixed anything. Markers that could pair differently are
  reported without a fix.
- **MD004**: Its sublist style no longer produces an empty marker whose fix
  deleted the bullets, and it replaces all of a list's bullets in one fix.
- **MD005**, **MD007**, **MD029**, **MD030**: Find each item's marker from
  the parsed list, so they work in blockquotes and on lines that open
  several lists. MD005 no longer moves items out of their list, MD007 no
  longer replaces whole lines, MD029 no longer changes `)` to `.` or
  renumbers a list's first item, and MD030 checks lists in blockquotes.
- **MD009**: Reads line breaks from the parsed document, and normalizes a
  hard break instead of deleting it.
- **MD010**: Its fix no longer shifts the columns of tabs that indent
  blocks.
- **MD012**: No longer counts or deletes blank lines in code. At the end of
  a file it removes the extra blank lines, where it used to offer an empty
  edit that MD047 reported again.
- **MD013**: Measures characters instead of bytes, so 48 CJK characters
  measure 48, not 144. Its wrap fix wraps only paragraph text, carries
  blockquote and list prefixes onto new lines, keeps code spans, inline
  HTML, link destinations and task checkboxes whole, and no longer starts a
  line with a list marker, heading marker or fence. Lines that hold only a
  link, and reference definitions, are exempt, as in markdownlint.
- **MD027**: No longer reports the content indentation of list items in
  blockquotes, or pulls paragraphs out of them.
- **MD032**: Checks nested lists, and its fix adds the container's own blank
  line instead of splitting a blockquote in two.
- **MD014**: Checks every code block and reports each prompt. It used to
  check only fences whose info string named one of a few shells.
- **MD031**: Its fix adds the blockquote's own blank line instead of
  splitting the blockquote.
- **MD033**: Reports every disallowed tag at its own position. It used to
  check only the first tag of each HTML block, so a `<script>` after an
  allowed `<details>` passed.
- **MD034**: Reports the GFM autolink literals that markdownlint finds,
  except in tables that goldmark reads and markdownlint does not, which the
  README lists under Known Limitations. Its fix no longer wraps trailing
  punctuation or inserts angle brackets into link text, and it fixes a URL
  followed by an unbalanced `)`.
- **MD038**: Splits code spans as micromark does, so its fix no longer breaks
  a code span that holds backticks, and it reports the spaces on each side
  where they are.
- **MD039**, **MD053**: Fix what they report. MD053 also sees definitions in
  blockquotes and list items.
- **MD040**: No longer guesses a language from words in the block, which
  called `import React from 'react'` Python. It reports an empty fence at
  its line instead of 0:0. Its fix no longer inserts a detected language
  that `allowed_languages` leaves out. `allowed_languages` now matches
  exactly, as in markdownlint, so case matters. As in GFM, an info string
  ends at a carriage return or line feed, and splits into language and title
  at spaces and tabs only. `language_only` no longer hides an
  `allowed_languages` finding.
- **MD042**: Reports `[text](#)`.
- **MD045**: Reports `<img>` elements without alt text.
- **MD048**: Changes both fences of a block, where it used to change only
  the closing fence, and withholds a change that would not make a valid
  fence.
- **MD051**: Builds heading anchors as GitHub does, so `## A - B` has the
  anchor `#a---b`. Under GFM, it and **MD024** read closed ATX headings with
  spaces before the closing hashes.
- **MD052**: Reports references to undefined labels, such as
  `[missing][nope]`, which the parser leaves as text.
- **MD054**: `url_inline` decides as markdownlint does.
- **MD055**, **MD056**, **MD058**, **MD060**, **MDL003**: Read the rows of
  parsed tables, so they see tables in blockquotes. MD055 and MD056 report
  at markdownlint's columns, MD058's fix no longer moves a row out of its
  table, and MDL003's fix no longer strips list indentation until the table
  becomes a paragraph.
- **MD022**, **MD031**, **MD058**: A fix no longer inserts a blank line
  before a table inside a tight list item, which made the list loose. Such
  findings are reported without a fix.
- **MD058**: No longer adds a blank line before or after a table in a list
  where the line would separate two items or two blocks of an item. Such
  tables are reported without a fix.
- **MDL001**: Counts a destination as absolute when it starts with a URI
  scheme, after escapes and character references are resolved, or with
  `//`. Links to `sms:`, `urn:` or `vscode:` URIs and email autolinks used to
  count as relative.
- Fixes of two rules no longer combine to corrupt a file. MD014 removing
  prompts while MD048 changed the fences could close a code block early, and
  MD007 moving an item right while MD030 narrowed the item above could nest
  it.
- Node positions cover each construct's markers, so findings point at the
  start of a heading, list item or fence, and empty fences, thematic breaks
  and empty list items have positions. Fixes no longer rewrite `1)` lists
  as `1.` lists. A leading byte order mark no longer hides the first heading
  or the front matter, and concurrent parses no longer race.
- Fixing a file through a symbolic link fixes its target instead of
  replacing the link with a copy, hard links stay linked, and the file keeps
  its owner where the system allows. A fix is written only while the file
  still holds the content that was linted.
- Fixes in CRLF files write CRLF line endings, and JSON and SARIF output
  show the bytes a fix writes.
- One invalid fix no longer drops every fix in the file, and a panic in a
  rule, or anywhere else while a file is processed, no longer stops the run.
  Each is reported, and the run exits 70.
- A fix made of several edits is applied whole or not at all.
- The fix loop lints the result of its last pass, no longer rewrites a file
  whose bytes did not change, and stops when fixes undo each other.
- A backup never replaces an existing backup.
- The dry-run diff shows fixes that only add a final newline, prints
  `\ No newline at end of file`, and no longer needs memory that grows with
  the square of the file's length.
- Findings of rules excluded from fixing by `auto_fix: false` or
  `--fix-rules` no longer count as fixable, and the summary counts fixed
  findings instead of edits.
- SARIF output is valid SARIF 2.1.0. Artifact URIs are relative to the
  repository root, columns count UTF-16 code units, each fix is one artifact
  change, results reference their rule descriptor, the tool version is the
  real one, and unreadable files and failing rules appear as notifications.
- The diff output keeps changed lines that start with `---` or `+++`.
- Summary and table output pad and cut by display width, so non-ASCII
  paths, messages and rule names stay aligned and valid UTF-8.
- Counts read "1 error" and "1 issue" instead of "1 errors" and "1 issues".
- The summary lists rules and files with equal counts in rule ID and path
  order, where the order used to change from run to run.
- Help output follows `--color`.
- Config files can turn backups off, `severity_default` applies, and the
  rule flags accept rule names.
- A rule configured under both its ID and a name is an error that names both
  keys, where one of them used to win at random. Unknown config keys, such
  as `flavour`, and flag-only keys such as `rule_format` warn with the file
  and line.
- `GOMDLINT_FORMAT` sets the output format.
- `rules` and `version` print plain text, and the context caret lines up
  after wide characters and tabs.
- Large documents lint in time that grows linearly with their size, with
  the exceptions the README lists under Known limitations. Text output with
  source context no longer looks up each finding's line by splitting the
  whole document. MD018, MD020, MD022 and MD036 build their lookups once per
  document instead of scanning it again for each line or heading. Fix checks
  parse only the part of the document that an edit can affect.
- goldmark moves to v1.8.6, past GO-2026-5320, a flaw in its HTML renderer
  that gomdlint never calls. Releases build with Go 1.26.9, which govulncheck
  finds free of GO-2026-4602, reachable through `filepath.WalkDir` with Go
  1.25.4.
- A release runs the tests and govulncheck first, so a tag on a failing
  commit publishes nothing. The Homebrew formula declares the MIT license
  instead of Apache-2.0.

## [0.2.2] - 2026-10-14

### Added

- markdownlint comments: `disable`, `enable`, `disable-line`,
  `disable-next-line`, `disable-file`, `enable-file`, `capture` and
  `restore`, with rule IDs, aliases and tags. Suppressed findings produce no
  fixes, and comments inside code or front matter change nothing.
- `markdownlint-configure-file` applies a JSON object of markdownlint
  settings to one file. Invalid settings fail the file with an error at the
  comment's line, before any fix.

### Fixed

- Link labels formatted as code keep their visible text, and empty links are
  reported at their source positions.
- Markdown checks skip leading front matter, while positions still count its
  lines.
- Independent emphasis spans no longer cause false positives, and list and
  heading fixes stay safe.
- Migration translates markdownlint's rule options and defaults without
  enabling gomdlint-only rules or changing native defaults, including table
  alignment, HTML allowlists, ordered list styles, whitespace and line
  length. It does not evaluate JavaScript configs or resolve `extends`.
- **MD032**: No longer reports a list followed by a blank line and a fenced
  code block.
- The diff output keeps each file's subdirectory path instead of only its
  name.

## [0.2.1] - 2026-02-06

No functional changes. Release process verification.

## [0.2.0] - 2026-02-06

### Added

- Warn when the multi-pass fix loop exhausts its pass limit without
  converging. Previously, gomdlint would silently stop fixing after 10
  passes. Now you'll see a warning if there were still edits pending.

### Fixed

- **MD032**: Now detects missing blank lines *after* lists, not just before.
  This was tricky because CommonMark's "lazy continuation" feature can absorb
  text into list items, but the rule now handles that correctly.
- **MD051**: No longer flags external URLs or code spans as broken internal
  links. The fragment validation was being too aggressive.
- **MD053 and others**: Fixed false positives where content inside fenced
  code blocks was incorrectly matched by line-scanning rules. Code block
  detection is now centralized and consistent across all rules.

## [0.1.1] - 2026-01-22

### Added

- Homebrew tap distribution: `brew install yaklabco/tap/gomdlint`
- Shell completions for bash, zsh, and fish bundled in releases

## [0.1.0] - 2026-01-22

Initial release of gomdlint - a fast Markdown linter written in Go.

### Features

- **55 lint rules** covering headings, lists, whitespace, code blocks, links,
  emphasis, blockquotes, and tables
- **Autofix support** for 37 rules (67% coverage)
- **Multiple output formats**: text, table, json, sarif, diff, summary
- **Parallel processing** for fast linting of large codebases
- **507x faster** than markdownlint on real-world repositories

### Commands

- `gomdlint lint` - Lint files with `--fix` for auto-correction
- `gomdlint rules` - List available rules
- `gomdlint init` - Generate config file
- `gomdlint migrate` - Convert markdownlint configs
- `gomdlint version` - Show version info

### Configuration

- YAML config via `.gomdlint.yml`
- Per-rule settings for severity, autofix, and options
- Include/exclude glob patterns
- Environment variable overrides (`GOMDLINT_*`)
- CommonMark and GFM flavor support

[Unreleased]: https://github.com/yaklabco/gomdlint/compare/v0.2.2...HEAD
[0.2.2]: https://github.com/yaklabco/gomdlint/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/yaklabco/gomdlint/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/yaklabco/gomdlint/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/yaklabco/gomdlint/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/yaklabco/gomdlint/releases/tag/v0.1.0
