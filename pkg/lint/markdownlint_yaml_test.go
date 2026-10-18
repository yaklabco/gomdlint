package lint_test

import (
	"fmt"
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
	} {
		_, err := lint.ReadMarkdownlintConfig([]byte(text))

		require.Error(t, err, "%q", text)
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
