package rules

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// lineEndingInputs are documents beyond the golden inputs whose rules read
// lines on their own.
var lineEndingInputs = map[string]string{
	"html block with an unclosed fence": "# Title\n\n<div>\n```\n<b>x</b>\n</div>\n",
	"names in an html block":            "# Title\n\n<div>\n<p>javascript</p>\njavascript\n</div>\n",
	"front matter":                      "---\ntitle: x\n---\n# Title\nText   \n\n\n\nMore\n",
	"toml front matter":                 "+++\ntitle = \"x\"\n+++\n\n## Title\n",
	"blank lines in containers":         "# Title\n> - a\n>\n>\n> - b\n\n1. x\n\n\n   y\n",
	"code in lists":                     "# Title\n\n- item\n\n      code\n\n- ```\n  fence\n  ```\n",
	"footnotes":                         "# T\n\nUse [^n] and [x][^n].\n\n[^n]: Note text.\n",
	"definitions":                       "# T\n\n[used]: /u\n[unused]: /v\n\nSee [used].\n",
	"table":                             "# T\n| a | b |\n|---|---|\n| 1 | 2 |\nText\n",
	"heading text over lines":           "Some *heading\ntext*.\n===\n\n# Title #\n",
	"code span over lines":              "# T\n\nA `` code\nspan `` here and ` x\n` there.\n",
	"long lines":                        "# T\n\n" + string(bytes.Repeat([]byte("word "), 30)) + "\n> " + string(bytes.Repeat([]byte("quoted "), 20)) + "\n",
}

// TestRulesReadCarriageReturnsAsLineFeeds checks every rule, in both
// flavors, on the golden inputs and lineEndingInputs written with carriage
// returns alone and with a mix of carriage returns and line feeds. A
// carriage return alone ends a line as a line feed does, and the documents
// have the same offsets, so every finding must be the same document's with
// line feeds: its position, message and fix, with the fix's text read with
// line feeds.
func TestRulesReadCarriageReturnsAsLineFeeds(t *testing.T) {
	inputs := lineFeedInputs(t)
	rules := lint.DefaultRegistry.Rules()
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		for name, lf := range inputs {
			want := ruleFindings(t, rules, flavor, lf)
			variants := map[string][]byte{
				"cr":    lineFeedsAsCarriageReturns(lf, func(int) bool { return true }),
				"mixed": lineFeedsAsCarriageReturns(lf, func(idx int) bool { return idx%2 == 0 }),
			}
			for variant, content := range variants {
				t.Run(fmt.Sprintf("%s/%s/%s", flavor, variant, name), func(t *testing.T) {
					got := ruleFindings(t, rules, flavor, content)
					for _, rule := range rules {
						assert.Equal(t, want[rule.ID()], got[rule.ID()], "%s", rule.ID())
					}
				})
			}
		}
	}
}

// TestCarriageReturnLinesAsMarkdownlintReadsThem checks a carriage return
// alone inside a document: it ends a line, so the heading is "# [" and the
// link is on line 4, where markdownlint 0.41.1 reports MD059 and no MD026.
func TestCarriageReturnLinesAsMarkdownlintReadsThem(t *testing.T) {
	input := "- # [\r].\n\n[link](/url)\n"

	descriptive := applyRuleWithOptions(t, NewDescriptiveLinkTextRule(), input, nil)
	require.Len(t, descriptive, 1)
	assert.Equal(t, mdast.Position{Line: 4, Column: 2}, mdast.Position{Line: descriptive[0].StartLine, Column: descriptive[0].StartColumn})

	assert.Empty(t, applyRuleWithOptions(t, NewNoTrailingPunctuationRule(), input, nil))
}

// TestFix_RewrittenLinesKeepTheirEndings runs --fix on a file whose lines
// end with a carriage return alone or a line feed, where MD004 rewrites every
// bullet of a list with one edit that copies the lines between them: each
// line keeps its own ending.
func TestFix_RewrittenLinesKeepTheirEndings(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.Rules = map[string]config.RuleConfig{"MD004": {Options: map[string]any{"style": "dash"}}}
	pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(string(cfg.Flavor)), lint.DefaultRegistry))
	input := "# Title\r\r* a\n* b\r* c\r"
	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte(input), cfg, lint.PipelineOptions{Fix: true})
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	assert.Equal(t, "# Title\r\r- a\n- b\r- c\r", string(result.ModifiedContent))
}

// TestRulesWriteNoCarriageReturnAlone checks that no rule writes a carriage
// return that no line feed follows into fix text for a document without
// one, the golden inputs with line feeds and with CRLF pairs. The engine
// keeps the line endings of such documents as they were (see
// fix.LineEndingWriter), which holds only while their fixes bring in no
// such carriage return.
func TestRulesWriteNoCarriageReturnAlone(t *testing.T) {
	rules := lint.DefaultRegistry.Rules()
	for name, lf := range lineFeedInputs(t) {
		for _, content := range [][]byte{lf, bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))} {
			for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
				file, parseErr := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", content)
				require.NoError(t, parseErr)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				for _, rule := range rules {
					diags, applyErr := rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
					require.NoError(t, applyErr, rule.ID())
					for _, diag := range diags {
						for _, edit := range diag.FixEdits {
							assert.False(t, mdast.HasBareCarriageReturn([]byte(edit.NewText)),
								"%s %s %s writes %q", name, flavor, rule.ID(), edit.NewText)
						}
					}
				}
			}
		}
	}
}

// lineFeedInputs returns lineEndingInputs and the golden inputs, the ones
// that hold no carriage return, by name.
func lineFeedInputs(t *testing.T) map[string][]byte {
	t.Helper()
	inputs := make(map[string][]byte)
	for name, input := range lineEndingInputs {
		inputs[name] = []byte(input)
	}
	err := filepath.WalkDir("testdata", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".input.md") {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.IndexByte(content, '\r') < 0 {
			inputs[path] = content
		}
		return nil
	})
	require.NoError(t, err)
	return inputs
}

// TestFix_WithholdsAFixThatJoinsLineEndings runs --fix where MD053 would
// delete an unused definition between a line that ends with a carriage
// return and an empty line that ends with a line feed: the two would make
// one CRLF line ending, so the fix is withheld and the finding stays. With
// line feeds alone, the fix applies.
func TestFix_WithholdsAFixThatJoinsLineEndings(t *testing.T) {
	process := func(input string) (*lint.PipelineResult, []lint.Diagnostic) {
		t.Helper()
		cfg := config.NewConfig()
		cfg.Fix = true
		pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(string(cfg.Flavor)), lint.DefaultRegistry))
		result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte(input), cfg, lint.PipelineOptions{Fix: true})
		require.NoError(t, err)
		require.Empty(t, result.RuleErrors)
		var unused []lint.Diagnostic
		for _, diag := range result.Diagnostics {
			if diag.RuleID == "MD053" {
				unused = append(unused, diag)
			}
		}
		return result, unused
	}

	lf, unused := process("# Title\n\nSee [used].\n\n[used]: /u\n[unused]: /v\n\nText\n")
	assert.Equal(t, "# Title\n\nSee [used].\n\n[used]: /u\n\nText\n", string(lf.ModifiedContent))
	assert.Empty(t, unused)

	mixed, unused := process("# Title\n\nSee [used].\r\r[used]: /u\r[unused]: /v\n\nText\n")
	assert.False(t, mixed.Modified)
	require.Len(t, unused, 1)
	assert.Equal(t, 6, unused[0].StartLine)
	assert.False(t, unused[0].HasFix())
}

// TestFix_KeepsLineEndingsApartInLaterPasses runs --fix on a file whose
// lines end mostly with carriage returns, where the first pass deletes every
// line that ends that way. The later passes still write the file's dominant
// ending, a carriage return, so they must keep it from joining a line feed
// after it: MD022's second blank line below the heading is written as a
// line feed, and is not lost in a CRLF pair.
func TestFix_KeepsLineEndingsApartInLaterPasses(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.Rules = map[string]config.RuleConfig{
		"MD022": {Options: map[string]any{"lines_below": 2}},
		"MD012": {Options: map[string]any{"maximum": 2}},
	}
	pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(string(cfg.Flavor)), lint.DefaultRegistry))
	input := "#T\n\nText\n\n[aa]: /a\r[bb]: /b\r[cc]: /c\r[dd]: /d\r[ee]: /e\r[ff]: /f\r"
	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte(input), cfg, lint.PipelineOptions{Fix: true})
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	assert.Equal(t, "# T\n\n\nText\n\n", string(result.ModifiedContent))
}

// ruleFinding is what TestRulesReadCarriageReturnsAsLineFeeds compares of a
// diagnostic: its position, message and fix edits, their text read with line
// feeds.
type ruleFinding struct {
	start, end mdast.Position
	message    string
	edits      []string
}

// ruleFindings runs each rule with its defaults on content and returns its
// findings by rule ID.
func ruleFindings(t *testing.T, rules []lint.Rule, flavor config.Flavor, content []byte) map[string][]ruleFinding {
	t.Helper()
	file, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", content)
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	found := make(map[string][]ruleFinding, len(rules))
	for _, rule := range rules {
		diags, applyErr := rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
		require.NoError(t, applyErr, rule.ID())
		for _, diag := range diags {
			finding := ruleFinding{
				start:   mdast.Position{Line: diag.StartLine, Column: diag.StartColumn},
				end:     mdast.Position{Line: diag.EndLine, Column: diag.EndColumn},
				message: diag.Message,
			}
			for _, edit := range diag.FixEdits {
				finding.edits = append(finding.edits, fmt.Sprintf("%d:%d:%q", edit.StartOffset, edit.EndOffset, mdast.LineFeedText(edit.NewText)))
			}
			found[rule.ID()] = append(found[rule.ID()], finding)
		}
	}
	return found
}

// lineFeedsAsCarriageReturns returns content with the line feeds whose index
// among the line feeds convert reports true written as carriage returns. A
// line feed stays when the next one does and follows it at once, so that no
// carriage return and line feed pair forms.
func lineFeedsAsCarriageReturns(content []byte, convert func(idx int) bool) []byte {
	out := bytes.Clone(content)
	lineFeed := 0
	for idx, char := range out {
		if char != '\n' {
			continue
		}
		if convert(lineFeed) && (idx+1 == len(out) || out[idx+1] != '\n' || convert(lineFeed+1)) {
			out[idx] = '\r'
		}
		lineFeed++
	}
	return out
}
