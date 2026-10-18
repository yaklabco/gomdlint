package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// fixWithEngine lints input with the engine and the given rules, applies the
// fixes and repeats until no fix is left, the way the fix pipeline does. It
// returns the fixed text and the number of passes that applied fixes.
func fixWithEngine(t *testing.T, input string, rules ...lint.Rule) (string, int) {
	t.Helper()
	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	cfg := config.NewConfig()
	cfg.Fix = true
	content := []byte(input)
	passes := 0
	for range lint.DefaultMaxFixPasses {
		result, err := engine.LintFile(context.Background(), "test.md", content, cfg)
		require.NoError(t, err)
		if len(result.Edits) == 0 {
			break
		}
		content = fix.ApplyEdits(content, result.Edits)
		passes++
	}
	return string(content), passes
}

// TestListFixesKeepSuppressedItems checks that no list fix relies on the fix
// of an item whose diagnostic a directive suppresses.
func TestListFixesKeepSuppressedItems(t *testing.T) {
	tests := []struct {
		name  string
		rule  lint.Rule
		input string
		want  string
	}{
		{
			// Beta can only move together with Alpha, whose fix is
			// suppressed.
			name:  "suppressed parent",
			rule:  NewULIndentRule(),
			input: "<!-- markdownlint-disable-next-line MD007 -->\n  - Alpha\n    - Beta\n",
			want:  "<!-- markdownlint-disable-next-line MD007 -->\n  - Alpha\n    - Beta\n",
		},
		{
			// Alpha can move on its own, and Beta keeps its line.
			name:  "suppressed child",
			rule:  NewULIndentRule(),
			input: "  - Alpha\n    - Beta <!-- markdownlint-disable-line MD007 -->\n",
			want:  "- Alpha\n    - Beta <!-- markdownlint-disable-line MD007 -->\n",
		},
		{
			// Third can only move together with Nested, whose fix is
			// suppressed.
			name:  "suppressed middle item",
			rule:  NewULIndentRule(),
			input: "- Top\n    - Nested <!-- markdownlint-disable-line MD007 -->\n        - Third\n",
			want:  "- Top\n    - Nested <!-- markdownlint-disable-line MD007 -->\n        - Third\n",
		},
		{
			// Gamma and Delta can only move together with Beta.
			name:  "suppressed item with a sibling and a child",
			rule:  NewULIndentRule(),
			input: "- Alpha\n    - Beta <!-- markdownlint-disable-line MD007 -->\n    - Gamma\n        - Delta\n",
			want:  "- Alpha\n    - Beta <!-- markdownlint-disable-line MD007 -->\n    - Gamma\n        - Delta\n",
		},
		{
			// Replacing only the bullet of Gamma would split the nested list.
			name:  "suppressed bullet in a list",
			rule:  NewUnorderedListStyleRule(),
			input: "- Alpha\n  * Beta <!-- markdownlint-disable-line MD004 -->\n  * Gamma\n",
			want:  "- Alpha\n  * Beta <!-- markdownlint-disable-line MD004 -->\n  * Gamma\n",
		},
		{
			// Replacing both bullets would rewrite the suppressed line of Gamma.
			name:  "suppressed later bullet in a list",
			rule:  NewUnorderedListStyleRule(),
			input: "- Alpha\n  * Beta\n  * Gamma <!-- markdownlint-disable-line MD004 -->\n",
			want:  "- Alpha\n  * Beta\n  * Gamma <!-- markdownlint-disable-line MD004 -->\n",
		},
		{
			// A no-break space after "<!--" is whitespace to markdownlint, so the
			// comment still suppresses Gamma.
			name:  "suppressed later bullet after a no-break space",
			rule:  NewUnorderedListStyleRule(),
			input: "- Alpha\n  * Beta\n  * Gamma <!--\u00a0markdownlint-disable-line MD004 -->\n",
			want:  "- Alpha\n  * Beta\n  * Gamma <!--\u00a0markdownlint-disable-line MD004 -->\n",
		},
		{
			name:  "suppressed later bullet after a vertical tab",
			rule:  NewUnorderedListStyleRule(),
			input: "- Alpha\n  * Beta\n  * Gamma <!--\u000bmarkdownlint-disable-line MD004 -->\n",
			want:  "- Alpha\n  * Beta\n  * Gamma <!--\u000bmarkdownlint-disable-line MD004 -->\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed, _ := fixWithEngine(t, tt.input, tt.rule)
			assert.Equal(t, tt.want, fixed)
			assert.Equal(t, listShape(t, tt.input), listShape(t, fixed), "fixed:\n%s", fixed)
		})
	}
}

// TestListShiftFixesApplyInOnePass checks that moves that depend on each
// other are still fixed together in a single pass.
func TestListShiftFixesApplyInOnePass(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nested levels",
			input: "- Top\n    - Nested\n        - Third\n",
			want:  "- Top\n  - Nested\n    - Third\n",
		},
		{
			name:  "siblings",
			input: "- A\n    - B\n    - C\n",
			want:  "- A\n  - B\n  - C\n",
		},
		{
			name:  "nested levels in a blockquote",
			input: "> - Top\n>     - Nested\n>         - Third\n",
			want:  "> - Top\n>   - Nested\n>     - Third\n",
		},
		{
			name:  "two separate trees",
			input: "* a\n    * b\n        * c\n\ntext\n\n* d\n    * e\n    * f\n",
			want:  "* a\n  * b\n    * c\n\ntext\n\n* d\n  * e\n  * f\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed, passes := fixWithEngine(t, tt.input, NewULIndentRule())
			assert.Equal(t, tt.want, fixed)
			assert.Equal(t, 1, passes)
		})
	}
}

// TestListFixesAreSafeInAnySubset applies every subset of the fixes a rule
// reports in one pass and checks that the lists keep their structure.
// Directives, conflicts with other rules' edits and invalid edits can each
// drop some of a rule's fixes, so no fix may depend on another one being
// applied.
func TestListFixesAreSafeInAnySubset(t *testing.T) {
	tests := []struct {
		name    string
		rule    lint.Rule
		input   string
		options map[string]any
		// noFix is true when the only safe choice is to report without a
		// fix.
		noFix bool
	}{
		{name: "MD007 nested levels", rule: NewULIndentRule(), input: "- Top\n    - Nested\n        - Third\n"},
		{name: "MD007 siblings", rule: NewULIndentRule(), input: "- A\n    - B\n    - C\n"},
		{name: "MD007 indented list", rule: NewULIndentRule(), input: "  - Alpha\n    - Beta\n  - Gamma\n   - Delta\n"},
		{name: "MD007 blockquote", rule: NewULIndentRule(), input: "> - Top\n>     - Nested\n>         - Third\n"},
		{
			name:  "MD007 deep tree",
			rule:  NewULIndentRule(),
			input: "* a\n    * b\n        * c\n    * d\n        * e\n* f\n    * g\n",
		},
		{
			name:    "MD007 indented start",
			rule:    NewULIndentRule(),
			input:   "- a\n  - b\n- c\n",
			options: map[string]any{"start_indented": true},
		},
		{name: "MD004 nested list", rule: NewUnorderedListStyleRule(), input: "- first\n  * second\n  * third\n* fourth\n"},
		{
			name:  "MD004 two levels",
			rule:  NewUnorderedListStyleRule(),
			input: "- a\n\ntext\n\n* b\n  + c\n  + d\n* e\n",
		},
		{name: "MD005 ordered", rule: NewListIndentRule(), input: "1. a\n 2. b\n  3. c\n"},
		{name: "MD005 nested ordered", rule: NewListIndentRule(), input: "1. a\n   1. b\n    2. c\n     3. d\n"},
		{name: "MD030 spacing", rule: NewListMarkerSpaceRule(), input: "-   A\n    -   B\n        - C\n"},
		{
			// "- third" is a lazy continuation of the paragraph of second
			// only while the content column of first stays to its right.
			name:  "MD030 lazy line in a nested item",
			rule:  NewListMarkerSpaceRule(),
			input: ">  -   first\n>      - second\n>     - third\n",
			noFix: true,
		},
		{name: "MD027 quoted list", rule: NewNoMultipleSpaceBlockquoteRule(), input: ">   - a\n>     - b\n>   - c\n"},
		{
			name:  "MD027 lazy line in a nested item",
			rule:  NewNoMultipleSpaceBlockquoteRule(),
			input: ">  -   first\n>      - second\n>     - third\n",
			noFix: true,
		},
		{
			name:    "MD029 widened numbers",
			rule:    NewOrderedListIncrementRule(),
			input:   "1. a\n1. b\n1. c\n1. d\n1. e\n1. f\n1. g\n1. h\n1. i\n1. j\n   1. k\n1. l\n",
			options: map[string]any{"style": "ordered"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)
			var ruleCfg *config.RuleConfig
			if tt.options != nil {
				ruleCfg = &config.RuleConfig{Options: tt.options}
			}
			diags, err := tt.rule.Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), ruleCfg))
			require.NoError(t, err)
			fixable := filterFixableDiags(diags)
			if tt.noFix {
				assert.Empty(t, fixable)
				return
			}
			require.NotEmpty(t, fixable, "the input should have fixes")
			require.Less(t, len(fixable), 12, "too many fixes to try every subset")

			want := listShape(t, tt.input)
			for subset := 1; subset < 1<<len(fixable); subset++ {
				var edits []fix.TextEdit
				for idx, diag := range fixable {
					if subset&(1<<idx) != 0 {
						edits = append(edits, diag.FixEdits...)
					}
				}
				accepted, _, _, err := fix.PrepareEditsFiltered(edits, len(tt.input))
				require.NoError(t, err)
				fixed := string(fix.ApplyEdits([]byte(tt.input), accepted))
				assert.Equal(t, want, listShape(t, fixed), "subset %b, fixed:\n%s", subset, fixed)
			}
		})
	}
}
