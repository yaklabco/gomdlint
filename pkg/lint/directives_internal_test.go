package lint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// resolveOne resolves the words of <!-- markdownlint-disable parameter -->,
// a comment on line 7, as collectDirectives resolves them.
func resolveOne(registry *Registry, parameter string) inlineDirective {
	text := "<!-- markdownlint-disable " + parameter + " -->"
	matches := findDirectives(text)
	directives := make([]inlineDirective, len(matches))
	for idx, match := range matches {
		directives[idx] = inlineDirective{line: 7, action: match.kind, parameter: text[match.kindEnd:match.close]}
	}
	(&wordResolver{registry: registry}).resolveComments(text, matches, directives)
	return directives[0]
}

// TestWordResolverRuleList pins how the words of a markdownlint comment
// resolve to rules, as before notices existed, and which words get notices.
func TestWordResolverRuleList(t *testing.T) {
	// withCustomTag adds a rule that tags itself acme, outside the catalog.
	withCustomTag := func() *Registry {
		reg := describedRegistry()
		reg.Register(&mockRule{id: "ACME001", name: "acme-rule", tags: []string{"acme"}})
		return reg
	}
	// withoutCatalog holds rules that tag themselves, without catalog
	// entries.
	withoutCatalog := func() *Registry {
		reg := NewRegistry()
		reg.Register(&mockRule{id: "MD004", name: "unordered-list-style", tags: []string{"lists", "style"}})
		reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces", tags: []string{"whitespace"}})
		return reg
	}
	every := []string{"MD004", "MD009", "MD026", "MD043", "MD047", "MD055"}

	type noticed struct {
		kind  NoticeKind
		name  string
		rules []string
	}
	tests := []struct {
		name      string
		registry  func() *Registry
		parameter string
		rules     []string
		notices   []noticed
	}{
		{name: "no words name every rule", registry: describedRegistry, parameter: "", rules: every},
		{name: "whitespace names every rule", registry: describedRegistry, parameter: " \t\n ", rules: every},
		{
			name: "an unknown word names nothing", registry: describedRegistry, parameter: " frobnicate ",
			notices: []noticed{{kind: NoticeUnknownRule, name: "frobnicate"}},
		},
		{
			name: "an unknown word leaves the others", registry: describedRegistry, parameter: "frobnicate MD009",
			rules: []string{"MD009"}, notices: []noticed{{kind: NoticeUnknownRule, name: "frobnicate"}},
		},
		{
			name: "repeats in any case", registry: describedRegistry, parameter: "md009 No-Trailing-Spaces MD009 FROB frob",
			rules: []string{"MD009"}, notices: []noticed{{kind: NoticeUnknownRule, name: "FROB"}},
		},
		{name: "an alias and a catalog tag", registry: describedRegistry, parameter: "ul-style bullet", rules: []string{"MD004"}},
		{name: "catalog words of a rule that is not registered", registry: describedRegistry, parameter: "phantom-alias MD998 phantom-rule"},
		{
			name: "a former tag", registry: describedRegistry, parameter: "lists LISTS",
			rules:   []string{"MD004"},
			notices: []noticed{{kind: NoticeDeprecatedTag, name: "lists", rules: []string{"unordered-list-style"}}},
		},
		{name: "the tag of a rule outside the catalog", registry: withCustomTag, parameter: "acme", rules: []string{"ACME001"}},
		{name: "a markdownlint tag without the catalog", registry: withoutCatalog, parameter: "whitespace", rules: []string{"MD009"}},
		{
			name: "a former tag without the catalog", registry: withoutCatalog, parameter: "style",
			rules:   []string{"MD004"},
			notices: []noticed{{kind: NoticeDeprecatedTag, name: "style", rules: []string{"unordered-list-style"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directive := resolveOne(tt.registry(), tt.parameter)

			if len(tt.rules) == 0 {
				assert.Empty(t, directive.rules)
			} else {
				assert.Equal(t, tt.rules, directive.rules)
			}
			var got []noticed
			for _, notice := range directive.notices {
				assert.Equal(t, 7, notice.Line)
				assert.Equal(t, "markdownlint-disable", notice.Directive)
				got = append(got, noticed{kind: notice.Kind, name: notice.Name, rules: notice.Rules})
			}
			assert.Equal(t, tt.notices, got)
		})
	}
}

// TestFindDirectives covers where comments start, their kinds, and where
// their parameters end: at the next "-->" after the kind, which a comment
// inside another comment's parameter shares. The search resumes right after
// the kind, so it finds that inner comment, and it stops at a comment
// without a closing "-->".
func TestFindDirectives(t *testing.T) {
	text := "x <!-- markdownlint-disable a <!-- MARKDOWNLINT-ENABLE b --> y <!-- markdownlint-capture--> " +
		"<!-- markdownlint-disablex --> <!-- markdownlint-restore z"

	var got []string
	for _, match := range findDirectives(text) {
		got = append(got, fmt.Sprintf("%d %s %q", match.start, match.kind, text[match.kindEnd:match.close]))
	}

	assert.Equal(t, []string{
		`2 disable " a <!-- MARKDOWNLINT-ENABLE b "`,
		`30 enable " b "`,
		`63 capture ""`,
	}, got)
}

// TestJSWhitespaceAgreesWithItsPattern checks that isJSWhitespace and the
// jsWhitespace pattern hold the same characters, every one of them.
func TestJSWhitespaceAgreesWithItsPattern(t *testing.T) {
	pattern := regexp.MustCompile(`^` + jsWhitespace + `$`)
	for char := rune(0); char <= utf8.MaxRune; char++ {
		if !utf8.ValidRune(char) {
			continue
		}
		if pattern.MatchString(string(char)) != isJSWhitespace(char) {
			t.Fatalf("U+%04X: the pattern says %v", char, pattern.MatchString(string(char)))
		}
	}
}

// TestNestedDirectivesCostLinearWork reads a comment that holds many
// comment starts before its one "-->". Each start is a comment whose
// parameter runs to that "-->", so the parameters overlap. Reading them
// must cost memory in proportion to the text, not to the square of the
// number of comments.
func TestNestedDirectivesCostLinearWork(t *testing.T) {
	registry := describedRegistry()
	allocated := func(starts int) uint64 {
		content := "# T\n\n" + strings.Repeat("<!-- markdownlint-disable a\n", starts) + "-->\n"
		snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "doc.md", []byte(content))
		require.NoError(t, err)

		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		directives := collectDirectives(snapshot, registry)
		runtime.ReadMemStats(&after)

		require.Len(t, directives, starts)
		return after.TotalAlloc - before.TotalAlloc
	}

	small, large := allocated(1000), allocated(4000)

	assert.Less(t, large, 8*small, "four times the comments may cost about four times the memory, not sixteen")
}

// blankLineRule removes one blank line from the first run of two or more.
type blankLineRule struct {
	BaseRule
}

func (r *blankLineRule) Apply(ctx *RuleContext) ([]Diagnostic, error) {
	idx := bytes.Index(ctx.File.Content, []byte("\n\n\n"))
	if idx < 0 {
		return nil, nil
	}
	return []Diagnostic{{
		RuleID: r.ID(), Message: "blank lines", StartLine: bytes.Count(ctx.File.Content[:idx], []byte("\n")) + 2, StartColumn: 1,
		FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1}},
	}}, nil
}

// TestPipeline_RefusedWriteReportsTheNoticesOfTheFileAsItIs covers a fix
// that is not written because another program saved the file first. The
// result describes the file as it is, so its notices carry the lines of
// the content that was read, not of the fixed content.
func TestPipeline_RefusedWriteReportsTheNoticesOfTheFileAsItIs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	content := "# Title\n\n\n\n<!-- markdownlint-disable frobnicate -->\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	registry := NewRegistry()
	registry.Register(&blankLineRule{BaseRule: NewBaseRule("TEST010", "blank-lines", "", nil, true)})
	pipeline := NewPipeline(NewEngine(goldmark.New(goldmark.FlavorGFM), registry))
	pipeline.beforeWrite = func() {
		require.NoError(t, os.WriteFile(path, []byte(content+"Saved meanwhile.\n"), 0o644))
	}
	cfg := config.NewConfig()
	cfg.Fix = true

	result, err := pipeline.ProcessFile(context.Background(), path, cfg, withBackups(root))
	require.NoError(t, err)

	requireRefused(t, result)
	require.Len(t, result.Notices, 1)
	assert.Equal(t, 5, result.Notices[0].Line, "the comment's line in the file as it is; the fixes would move it to line 3")
}
