package lint_test

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// settingValues describes the keys and values of settings in order, with
// the Go syntax of each value, which tells a number from a string and
// prints -0, infinities and NaN.
func settingValues(settings *lint.MarkdownlintSettings) string {
	var parts []string
	for _, setting := range settings.Settings() {
		parts = append(parts, fmt.Sprintf("%q: %#v", setting.Key, setting.Value))
	}
	return strings.Join(parts, ", ")
}

// TestReadMarkdownlintConfigYAMLAsJSYAMLReads covers YAML that
// markdownlint-cli2 0.23.3 reads with js-yaml 5.4.1 and its default core
// schema. The core schema reads plain scalars as YAML 1.2 does, so 0b0,
// 0X1F, 1_000, yes and dates are strings, and a number too large for a
// float64 is a string too. It knows no tags beyond str, null, bool, int,
// float, seq and map, so !!binary, !!merge and local tags are errors, and a
// merge key is an ordinary key. Object keys are strings, so a numeric or
// null key becomes its JavaScript string, and two keys with the same string
// are duplicates, which are errors anywhere in the document. A %YAML
// directive of any version 1.x is accepted, and an unknown directive is
// ignored. Each expected value is what markdownlint-cli2 reads.
func TestReadMarkdownlintConfigYAMLAsJSYAMLReads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text, want string
	}{
		{text: "MD013: 0b0\n", want: `"MD013": "0b0"`},
		{text: "MD013: 0o7\n", want: `"MD013": 7`},
		{text: "MD013: 0x1F\n", want: `"MD013": 31`},
		{text: "MD013: 0X1F\n", want: `"MD013": "0X1F"`},
		{text: "MD013: 1_000\n", want: `"MD013": "1_000"`},
		{text: "MD013: yes\n", want: `"MD013": "yes"`},
		{text: "MD013: .inf\n", want: `"MD013": +Inf`},
		{text: "MD013: 1e400\n", want: `"MD013": "1e400"`},
		{text: "MD013: -0\n", want: `"MD013": -0`},
		{text: "MD013: 012\n", want: `"MD013": 12`},
		{text: "MD013: 2001-12-14\n", want: `"MD013": "2001-12-14"`},
		{text: "MD013: !!int 0b101\n", want: `"MD013": 5`},
		{text: "MD013: ~\nMD009: Null\n", want: `"MD013": <nil>, "MD009": <nil>`},
		{text: "MD013: {line_length: 200, 0: ignored}\n", want: `"MD013": map[string]interface {}{"0":"ignored", "line_length":200}`},
		{text: "null: x\nMD009: false\n", want: `"null": "x", "MD009": false`},
		{text: "1.0: x\n0x10: y\n.inf: z\n", want: `"1": "x", "16": "y", "Infinity": "z"`},
		{text: "%YAML 1.2\n---\nMD013: false\n", want: `"MD013": false`},
		{text: "%YAML 1.10\n---\nMD013: false\n", want: `"MD013": false`},
		{text: "%FOO bar\n---\nMD013: false\n", want: `"MD013": false`},
		{text: "<<: {MD013: false}\n", want: `"<<": map[string]interface {}{"MD013":false}`},
		{
			text: "base: &b {MD013: false}\n<<: *b\n",
			want: `"base": map[string]interface {}{"MD013":false}, "<<": map[string]interface {}{"MD013":false}`,
		},
		{text: "__proto__: x\nMD013: false\n", want: `"__proto__": "x", "MD013": false`},
		{text: "!!map\n", want: ``},
		// js-yaml counts no indentation on the first line, which no line
		// break precedes, so a directive or a "---" there may follow
		// spaces, as in the parameter of a configure-file comment.
		{text: " %YAML 1.2\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "  %FOO\n---\nMD009: false\n", want: `"MD009": false`},
		{text: " ---\nMD009: false\n", want: `"MD009": false`},
		{text: " MD009: false\n MD013: false\n", want: `"MD009": false, "MD013": false`},
		// The non-specific tag ! makes a plain scalar a string. It may follow
		// the anchor after spaces, line breaks and comments, and precede it.
		{text: "MD009: &a\n  ! false\n", want: `"MD009": "false"`},
		{text: "MD009: &a # note\n  ! false\n", want: `"MD009": "false"`},
		{text: "MD009: &a\n# note\n  ! false\n", want: `"MD009": "false"`},
		{text: "MD009: &a\n\n  ! false\n", want: `"MD009": "false"`},
		{text: "MD009: &a\r\n  ! false\r\n", want: `"MD009": "false"`},
		{text: "MD009: &a\t! 7\n", want: `"MD009": "7"`},
		{text: "MD009: &a !\tnull\n", want: `"MD009": "null"`},
		{text: "MD009: !\n  false\n", want: `"MD009": "false"`},
		{text: "MD009: ! &a\n  false\n", want: `"MD009": "false"`},
		{text: "MD009: &a\n  ! false\nMD010: *a\n", want: `"MD009": "false", "MD010": "false"`},
		{text: "MD009:\n  - &a\n    ! 5\n", want: `"MD009": []interface {}{"5"}`},
		{text: "MD009: [&a\n  ! 5, 6]\n", want: `"MD009": []interface {}{"5", 6}`},
		{text: "? &k\n  ! .inf\n: x\n", want: `".inf": "x"`},
		// An empty value takes a ! on a later line only while it is indented
		// under the value's key. The next key's ! is its own.
		{text: "MD009: &a\n  !\nMD010: x\n", want: `"MD009": "", "MD010": "x"`},
		{text: "MD009: &a\n! MD010: x\n", want: `"MD009": <nil>, "MD010": "x"`},
		{text: "MD009: &a\nMD010: ! false\n", want: `"MD009": <nil>, "MD010": "false"`},
		// js-yaml skips one byte order mark at the start of a line where a
		// document may start: after the end marker "...", and the spaces,
		// comments and line breaks after it.
		{text: "MD009: false\n...\n\ufeff\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n\ufeff", want: `"MD009": false`},
		{text: "MD009: false\n...\n\ufeff# note\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n\ufeff # note\n", want: `"MD009": false`},
		{text: "MD009: false\n... # note\n\ufeff\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n# note\n\ufeff\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n\n\ufeff\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n\ufeff...\n", want: `"MD009": false`},
		{text: "MD009: false\n...\n\ufeff\n...\n\ufeff\n", want: `"MD009": false`},
		{text: "MD009: false\r\n...\r\n\ufeff\r\n", want: `"MD009": false`},
		{text: "---\nMD009: false\n...\n\ufeff\n", want: `"MD009": false`},
		// A directive's name runs to a space or a tab, and a directive other
		// than YAML and TAG is ignored, whatever its name holds.
		{text: "%#foo\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%FOO#bar baz\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%YAML\u00a01.2\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%YAML#c 1.2\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%YAML\t1.2\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%YAML 1.2 #c\n---\nMD009: false\n", want: `"MD009": false`},
		{text: "%YAML 1.2\t#c\n---\nMD009: false\n", want: `"MD009": false`},
		{text: " %#foo\n---\nMD009: false\n", want: `"MD009": false`},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			t.Parallel()

			source, err := lint.ReadMarkdownlintConfig([]byte(tt.text))
			require.NoError(t, err)

			assert.Equal(t, lint.MarkdownlintYAML, source.Syntax)
			assert.Equal(t, tt.want, settingValues(source.Settings))
		})
	}
}

// TestReadMarkdownlintConfigRejectsWhatJSYAMLRejects covers YAML that
// js-yaml 5.4.1 rejects with its default core schema, so markdownlint-cli2
// fails to load it: unknown tags, an explicit tag whose value it cannot
// read, complex keys, keys that are duplicates once JavaScript turns them
// into strings, anywhere in the document, a %YAML directive of another
// version, and directives without the "---" that must follow them.
func TestReadMarkdownlintConfigRejectsWhatJSYAMLRejects(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"!unknown {MD013: false}\n",
		"MD013: !!binary aGk=\n",
		"MD013: !!bool yes\n",
		"MD013: !unknown x\n",
		"base: &b {MD013: false}\n!!merge <<: *b\n",
		"!!str {a: 1}\n",
		"? [a, b]\n: x\n",
		"0x10: a\n16: b\n",
		"null: a\n~: b\n",
		"- MD013: true\n  MD013: false\n",
		"MD013:\n  - {x: 1, x: 2}\n",
		"%YAML 2.0\n---\nMD013: false\n",
		"%FOO bar\nMD013: false\n",
		"MD013: false\n---\nMD009: false\n",
		" %YAML 1.2\nMD009: false\n",
		"\n %YAML 1.2\n---\nMD009: false\n",
		" MD009: false\nMD013: false\n",
		// A byte order mark that js-yaml does not skip is text, which
		// starts a second document.
		"MD009: false\n...\n\ufeffMD010: false\n",
		"MD009: false\n...\n\ufeff\n\ufeff\n",
		"MD009: false\n...\n\ufeff---\nMD010: false\n",
		"MD009: false\n...\n  \ufeff\n",
		"MD009: false\n\ufeff\n",
		// Directives that js-yaml rejects: two arguments for YAML, as the
		// no-break space joins no words, no name, and an ill-formed version.
		"%YAML 1.2\u00a0# c\n---\nMD009: false\n",
		"% YAML 1.2\n---\nMD009: false\n",
		"%YAML 1.2#c\n---\nMD009: false\n",
		"%YAML 1.1 extra\n---\nMD009: false\n",
		"%#foo\nMD009: false\n",
	} {
		_, err := lint.ReadMarkdownlintConfig([]byte(text))

		require.Error(t, err, "%q", text)
	}
}

// nestedBlockItems returns a block sequence nested levels deep, each item
// indented two spaces further than the one before, whose innermost item is
// empty, or last when it is not empty.
func nestedBlockItems(levels int, last string) string {
	var text strings.Builder
	for idx := range levels {
		text.WriteString(strings.Repeat("  ", idx) + "-\n")
	}
	if last != "" {
		text.WriteString(strings.Repeat("  ", levels) + "- " + last + "\n")
	}
	return text.String()
}

// TestReadMarkdownlintConfigReadsDeepNesting covers YAML nested deeply,
// which loads however deep it is, as it does on gomdlint's main line, as
// far as yaml.v3's parser reads: 10,000 levels of flow collections and
// 10,000 of indentation. js-yaml stops at a depth of 100, which it counts
// its own way, so markdownlint-cli2 loads the first config and fails the
// second, and gomdlint does not follow it there. The converter recurses as
// deep as the nesting, as yaml.v3's parser does.
func TestReadMarkdownlintConfigReadsDeepNesting(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, text string }{
		{name: "block items 100 deep", text: "MD009: false\nignored:\n" + nestedBlockItems(99, "")},
		{name: "block items 100 deep with an alias", text: "a: &a x\nMD009: false\nignored:\n" + nestedBlockItems(98, "*a")},
		{name: "an alias 101 deep", text: "a: &a x\nMD009: false\nignored: " + strings.Repeat("[", 99) + "*a" + strings.Repeat("]", 99) + "\n"},
		{name: "flow sequences 9,999 deep", text: "MD009: false\nignored: " + strings.Repeat("[", 9999) + strings.Repeat("]", 9999) + "\n"},
		{
			name: "flow sequences in block sequences, 18,000 deep",
			text: "MD009: false\nignored:\n" + strings.Repeat("- ", 9000) + strings.Repeat("[", 9000) + "x" + strings.Repeat("]", 9000) + "\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, err := lint.ReadMarkdownlintConfig([]byte(tt.text))

			require.NoError(t, err)
			assert.Contains(t, settingValues(source.Settings), `"MD009": false`)
		})
	}
}

// TestReadMarkdownlintConfigSharesAliases covers aliases, which js-yaml
// reads as the value of their anchor itself, so values that alias each
// other in many levels take no more work than they hold. An alias inside
// the node it names, which js-yaml reads as a value that holds itself,
// reads as an empty collection of the node's kind, so that the config
// loads, as in markdownlint-cli2, and its value holds no cycle.
func TestReadMarkdownlintConfigSharesAliases(t *testing.T) {
	t.Parallel()

	var text strings.Builder
	text.WriteString("a0: &a0 [x, x]\n")
	for level := 1; level <= 40; level++ {
		fmt.Fprintf(&text, "a%d: &a%d [*a%d, *a%d]\n", level, level, level-1, level-1)
	}
	source, err := lint.ReadMarkdownlintConfig([]byte(text.String()))
	require.NoError(t, err)
	assert.Len(t, source.Settings.Settings(), 41)

	source, err = lint.ReadMarkdownlintConfig([]byte("MD013: &r [*r]\nMD009: &m {x: *m}\n"))
	require.NoError(t, err)
	assert.Equal(t, `"MD013": []interface {}{[]interface {}{}}, "MD009": map[string]interface {}{"x":map[string]interface {}{}}`,
		settingValues(source.Settings))
}

// TestReadMarkdownlintConfigFindsDuplicatesInLinearTime covers a mapping
// with many keys: the check for duplicates takes time in proportion to the
// number of keys, where comparing every pair took about 7 seconds for these.
func TestReadMarkdownlintConfigFindsDuplicatesInLinearTime(t *testing.T) {
	t.Parallel()

	keys, limit := 50000, 3*time.Second
	if raceEnabled {
		keys, limit = 20000, 20*time.Second
	}
	var text strings.Builder
	for idx := range keys {
		fmt.Fprintf(&text, "key_%d: %d\n", idx, idx)
	}

	start := time.Now()
	source, err := lint.ReadMarkdownlintConfig([]byte(text.String()))
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, source.Settings.Settings(), keys)
	assert.Less(t, elapsed, limit)

	text.WriteString("key_0: again\n")
	_, err = lint.ReadMarkdownlintConfig([]byte(text.String()))
	require.Error(t, err, "the last key repeats the first")
}

// TestReadMarkdownlintConfigReadsYAMLInLinearTime guards the YAML reader
// against work that grows faster than the text, as finding each plain
// scalar's place in its line did: a flow sequence of 16,000 items on one
// line took almost a second, and a 96 KB config seven times as long as
// yaml.v3's decoding.
//
// As TestManyClosingBracketsParseInLinearTime in pkg/parser/goldmark does,
// it compares two measurements of the same amount of text: one read of a
// long input against a batch of reads of an input an eighth as long. A
// linear reader takes about as long either way, and a quadratic one about
// eight times as long for the long input. The measurements run in turns,
// the fastest of each counts, and garbage collection is paused while they
// are timed.
func TestReadMarkdownlintConfigReadsYAMLInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	const (
		small  = 2000
		factor = 8
		rounds = 5
		// A linear reader gives a ratio near one and a quadratic one near
		// eight. The bound leaves room for a busy machine.
		maxRatio = 3
	)
	items := func(prefix string, item func(idx int) string) func(int) string {
		return func(count int) string {
			var text strings.Builder
			text.WriteString(prefix)
			for idx := range count {
				if idx > 0 {
					text.WriteString(", ")
				}
				text.WriteString(item(idx))
			}
			text.WriteString("]}\n")
			return text.String()
		}
	}
	inputs := map[string]func(int) string{
		"words":                                  items("MD013: {unused: [", func(int) string { return "word" }),
		"numbers":                                items("MD013: {unused: [", strconv.Itoa),
		"numbers after a character beyond ASCII": items("MD013: {é: [", strconv.Itoa),
		"anchored numbers": items("MD013: {unused: [", func(idx int) string {
			return fmt.Sprintf("&a%d %d", idx, idx)
		}),
		"keys on their own lines": func(count int) string {
			var text strings.Builder
			for idx := range count {
				fmt.Fprintf(&text, "key_%d: %d\n", idx, idx)
			}
			return text.String()
		},
	}
	for name, generate := range inputs {
		t.Run(name, func(t *testing.T) {
			short, long := []byte(generate(small)), []byte(generate(factor*small))
			runtime.GC()
			defer debug.SetGCPercent(debug.SetGCPercent(-1))

			// Warm up both inputs before timing them.
			readDuration(t, short)
			readDuration(t, long)

			bestBatch, bestLong := time.Duration(1<<63-1), time.Duration(1<<63-1)
			for range rounds {
				runtime.GC()
				var batch time.Duration
				for range factor {
					batch += readDuration(t, short)
				}
				bestBatch = min(bestBatch, batch)
				runtime.GC()
				bestLong = min(bestLong, readDuration(t, long))
			}
			ratio := float64(bestLong) / float64(bestBatch)
			t.Logf("%d reads of %d items in %v, one read of %d in %v, ratio %.2f",
				factor, small, bestBatch, factor*small, bestLong, ratio)
			require.Less(t, ratio, float64(maxRatio), "reading time grows faster than the text")
		})
	}
}

// readDuration reads text, a markdownlint config in YAML, and returns how
// long the reading took.
func readDuration(t *testing.T, text []byte) time.Duration {
	t.Helper()
	start := time.Now()
	source, err := lint.ReadMarkdownlintConfig(text)
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, lint.MarkdownlintYAML, source.Syntax)
	return elapsed
}
