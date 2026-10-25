package lint_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// groupContent is a list whose nested bullets must change together: a
// different bullet starts a new list, so replacing one of them alone would
// split the nested list.
const groupContent = "- a\n  * b\n  * c\n- d\n"

// groupMember returns a diagnostic on line with its own edit and a fix group.
func groupMember(id string, line int, group int, edits ...fix.TextEdit) lint.Diagnostic {
	diag := fixDiag(id, edits...)
	diag.StartLine = line
	diag.FixGroup = group
	return diag
}

// groupRule reports diags on every pass.
func groupRule(id string, diags ...lint.Diagnostic) *funcRule {
	return newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return append([]lint.Diagnostic(nil), diags...), nil
	})
}

func TestEngine_AppliesAFixGroupAsOneFix(t *testing.T) {
	t.Parallel()

	first, second := edit(t, groupContent, "* b", "- b"), edit(t, groupContent, "* c", "- c")
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, first),
		groupMember("TEST001", 3, 1, second))

	result := lintMarkdown(t, groupContent, rule)

	require.Len(t, result.Diagnostics, 2)
	assert.Equal(t, []fix.TextEdit{first}, result.Diagnostics[0].FixEdits, "each finding keeps its own edit")
	assert.Equal(t, []fix.TextEdit{second}, result.Diagnostics[1].FixEdits)
	assert.Equal(t, 2, result.FixableCount())
	assert.Equal(t, 2, result.AcceptedFixes, "the fix fixes both findings")
	combined, ok := fix.CombineEdits([]byte(groupContent), []fix.TextEdit{first, second})
	require.True(t, ok)
	assert.Equal(t, []fix.TextEdit{combined}, result.Edits, "the group is applied as one edit")
	assert.Equal(t, "- a\n  - b\n  - c\n- d\n", string(fix.ApplyEdits([]byte(groupContent), result.Edits)))
}

// Without --fix the engine applies nothing, but a group's members still
// show their fixes.
func TestEngine_OffersAFixGroupWithoutFixing(t *testing.T) {
	t.Parallel()

	first, second := edit(t, groupContent, "* b", "- b"), edit(t, groupContent, "* c", "- c")
	cfg := fixConfig()
	cfg.Fix = false
	result, err := markdownEngine(groupRule("TEST001",
		groupMember("TEST001", 2, 1, first),
		groupMember("TEST001", 3, 1, second))).LintFile(context.Background(), "test.md", []byte(groupContent), cfg)
	require.NoError(t, err)

	assert.Equal(t, 2, result.FixableCount())
	assert.Empty(t, result.Edits)
	assert.Zero(t, result.AcceptedFixes)
}

// A group is fixed whole or not at all, so a member that has lost its fix
// takes the fix of every other member with it.
func TestEngine_WithholdsAFixGroupWithoutAllItsMembers(t *testing.T) {
	t.Parallel()

	const disabledContent = "- a\n  * b <!-- markdownlint-disable-line TEST001 -->\n  * c\n- d\n"
	tests := []struct {
		name    string
		content string
		diags   []lint.Diagnostic
		// invalid is true when the rule reports an invalid fix.
		invalid bool
	}{
		{
			name:    "member on a disabled line",
			content: disabledContent,
			diags: []lint.Diagnostic{
				groupMember("TEST001", 2, 1, edit(t, disabledContent, "* b", "- b")),
				groupMember("TEST001", 3, 1, edit(t, disabledContent, "* c", "- c")),
			},
		},
		{
			name:    "member with an invalid fix",
			content: groupContent,
			diags: []lint.Diagnostic{
				groupMember("TEST001", 2, 1, edit(t, groupContent, "* b", "- b")),
				groupMember("TEST001", 3, 1, fix.TextEdit{StartOffset: 10, EndOffset: 100, NewText: "-"}),
			},
			invalid: true,
		},
		{
			name:    "member without a fix",
			content: groupContent,
			diags: []lint.Diagnostic{
				groupMember("TEST001", 2, 1, edit(t, groupContent, "* b", "- b")),
				groupMember("TEST001", 3, 1),
			},
		},
		{
			name:    "members whose edits overlap",
			content: groupContent,
			diags: []lint.Diagnostic{
				groupMember("TEST001", 2, 1, edit(t, groupContent, "* b", "- b")),
				groupMember("TEST001", 2, 1, edit(t, groupContent, "b", "x")),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := lintMarkdown(t, tt.content, groupRule("TEST001", tt.diags...))

			for _, diag := range result.Diagnostics {
				assert.False(t, diag.HasFix(), "line %d", diag.StartLine)
			}
			assert.Empty(t, result.Edits)
			assert.Zero(t, result.AcceptedFixes)
			_, failed := result.RuleErrors["TEST001"]
			assert.Equal(t, tt.invalid, failed)
		})
	}
}

// The engine checks a group's fix as the one edit it applies, so a group
// whose edit rewrites a line where its rule is disabled is withheld, even
// when no member changes that line.
func TestEngine_ChecksAFixGroupAsOneEdit(t *testing.T) {
	t.Parallel()

	const content = "- a\n  * b\n  x <!-- markdownlint-disable-line TEST001 -->\n  * c\n- d\n"
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, edit(t, content, "* b", "- b")),
		groupMember("TEST001", 4, 1, edit(t, content, "* c", "- c")))

	result := lintMarkdown(t, content, rule)

	require.Len(t, result.Diagnostics, 2)
	for _, diag := range result.Diagnostics {
		assert.False(t, diag.HasFix(), "line %d", diag.StartLine)
	}
	assert.Empty(t, result.Edits)
}

// A group's fix takes the place of its first member among the rule's fixes.
// The engine resolves a conflict between two fixes that start and end in one
// place by their order, so the group wins or loses it as a fix that the
// first member carried would.
func TestEngine_PlacesAFixGroupAtItsFirstMember(t *testing.T) {
	t.Parallel()

	first, second := edit(t, groupContent, "* b", "- b"), edit(t, groupContent, "* c", "- c")
	combined, ok := fix.CombineEdits([]byte(groupContent), []fix.TextEdit{first, second})
	require.True(t, ok)
	rival := fix.TextEdit{StartOffset: combined.StartOffset, EndOffset: combined.EndOffset, NewText: "x"}

	t.Run("group first", func(t *testing.T) {
		t.Parallel()

		result := lintMarkdown(t, groupContent, groupRule("TEST001",
			groupMember("TEST001", 2, 1, first),
			fixDiag("TEST001", rival),
			groupMember("TEST001", 3, 1, second)))

		assert.Equal(t, []fix.TextEdit{combined}, result.Edits)
		assert.Equal(t, 2, result.AcceptedFixes)
		require.Len(t, result.SkippedEdits, 1)
		assert.Equal(t, rival, result.SkippedEdits[0].Edit)
	})

	t.Run("other fix first", func(t *testing.T) {
		t.Parallel()

		result := lintMarkdown(t, groupContent, groupRule("TEST001",
			fixDiag("TEST001", rival),
			groupMember("TEST001", 2, 1, first),
			groupMember("TEST001", 3, 1, second)))

		assert.Equal(t, []fix.TextEdit{rival}, result.Edits)
		assert.Equal(t, 1, result.AcceptedFixes)
		require.Len(t, result.SkippedEdits, 1)
		assert.Equal(t, combined, result.SkippedEdits[0].Edit)
	})
}

// Groups are numbered per rule: the same number in two rules names two
// groups.
func TestEngine_KeepsFixGroupsOfDifferentRulesApart(t *testing.T) {
	t.Parallel()

	const content = "- a\n  * b\n  * c\n\ntext\n\n- d\n  + e\n  + f\n"
	one := groupRule("TEST001",
		groupMember("TEST001", 2, 1, edit(t, content, "* b", "- b")),
		groupMember("TEST001", 3, 1, edit(t, content, "* c", "- c")))
	two := groupRule("TEST002",
		groupMember("TEST002", 8, 1, edit(t, content, "+ e", "- e")),
		groupMember("TEST002", 9, 1, edit(t, content, "+ f", "- f")))

	result := lintMarkdown(t, content, one, two)

	assert.Equal(t, 4, result.AcceptedFixes)
	assert.Equal(t, "- a\n  - b\n  - c\n\ntext\n\n- d\n  - e\n  - f\n", string(fix.ApplyEdits([]byte(content), result.Edits)))
}

// applyReported applies the edits that the diagnostics of result report, an
// edit that several of them carry once, as a consumer of a report does.
func applyReported(t *testing.T, content string, result *lint.FileResult) string {
	t.Helper()

	var edits []fix.TextEdit
	seen := make(map[fix.TextEdit]bool)
	for _, diag := range result.Diagnostics {
		for _, reported := range diag.FixEdits {
			if !seen[reported] {
				seen[reported] = true
				edits = append(edits, reported)
			}
		}
	}
	prepared, err := fix.PrepareEdits(edits, len(content))
	require.NoError(t, err)
	return string(fix.ApplyEdits([]byte(content), prepared))
}

// The edits that the members of a group report give, applied together,
// what --fix writes, even where the engine writes the text between them in
// other line endings than the file has there.
func TestEngine_ReportsTheEditsAFixGroupApplies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "line feeds",
			content: "# L\n\n- a\n  * b\n  * c\n- d\n",
			want:    "# L\n\n- a\n  - b\n  - c\n- d\n",
		},
		{
			name:    "carriage returns and line feeds",
			content: "# L\r\n\r\n- a\r\n  * b\r\n  * c\r\n- d\r\n",
			want:    "# L\r\n\r\n- a\r\n  - b\r\n  - c\r\n- d\r\n",
		},
		{
			// The engine writes the line feed after b, between the two
			// members, as the file's dominant ending.
			name:    "a line feed among carriage returns",
			content: "# L\r\n\r\n- a\r\n  * b\n  * c\r\n- d\r\n",
			want:    "# L\r\n\r\n- a\r\n  - b\r\n  - c\r\n- d\r\n",
		},
		{
			// With a lone carriage return, the engine keeps each line
			// ending of the text it rewrites.
			name:    "a lone carriage return among line feeds",
			content: "# L\n\n- a\n  * b\r  * c\n- d\n",
			want:    "# L\n\n- a\n  - b\r  - c\n- d\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := groupRule("TEST001",
				groupMember("TEST001", 4, 1, edit(t, tt.content, "* b", "- b")),
				groupMember("TEST001", 5, 1, edit(t, tt.content, "* c", "- c")))

			result := lintMarkdown(t, tt.content, rule)

			require.Len(t, result.Diagnostics, 2)
			for _, diag := range result.Diagnostics {
				assert.True(t, diag.HasFix(), "line %d", diag.StartLine)
				assert.Equal(t, 1, diag.FixGroup, "line %d", diag.StartLine)
			}
			assert.Equal(t, 2, result.AcceptedFixes)
			fixed := string(fix.ApplyEdits([]byte(tt.content), result.Edits))
			if tt.want != "" {
				assert.Equal(t, tt.want, fixed)
			}
			assert.Equal(t, fixed, applyReported(t, tt.content, result), "the reported edits give what --fix writes")
		})
	}
}

// A group is checked for line endings as the one edit the engine applies,
// so members whose edits would join line endings only on their own are
// fixed together.
func TestEngine_ChecksTheLineEndingsOfAFixGroupTogether(t *testing.T) {
	t.Parallel()

	// Deleting x alone would put the carriage return before the line feed.
	const content = "a\rx\nz\n"
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, fix.TextEdit{StartOffset: 2, EndOffset: 3}),
		groupMember("TEST001", 2, 1, fix.TextEdit{StartOffset: 3, EndOffset: 3, NewText: "y"}))

	result := lintMarkdown(t, content, rule)

	require.Len(t, result.Diagnostics, 2)
	assert.Equal(t, 2, result.FixableCount())
	assert.Equal(t, 2, result.AcceptedFixes)
	assert.Equal(t, "a\ry\nz\n", string(fix.ApplyEdits([]byte(content), result.Edits)))
	assert.Equal(t, "a\ry\nz\n", applyReported(t, content, result))
}

// A rule's groups are fixed or withheld each on its own, and the fixes
// between them keep their places.
func TestEngine_DecidesEachFixGroupOfARuleOnItsOwn(t *testing.T) {
	t.Parallel()

	const content = "- a\n  * b <!-- markdownlint-disable-line TEST001 -->\n  * c\n\nplain\n\n- d\n  + e\n  + f\n"
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, edit(t, content, "* b", "- b")),
		groupMember("TEST001", 3, 1, edit(t, content, "* c", "- c")),
		fixDiag("TEST001", edit(t, content, "plain", "Plain")),
		groupMember("TEST001", 8, 2, edit(t, content, "+ e", "- e")),
		groupMember("TEST001", 9, 2, edit(t, content, "+ f", "- f")))

	result := lintMarkdown(t, content, rule)

	require.Len(t, result.Diagnostics, 4, "the finding on the disabled line is dropped")
	assert.False(t, result.Diagnostics[0].HasFix(), "group 1 lost a member")
	assert.Zero(t, result.Diagnostics[0].FixGroup, "a finding without a fix is in no group")
	assert.Equal(t, 2, result.Diagnostics[2].FixGroup)
	assert.Equal(t, 3, result.AcceptedFixes)
	assert.Equal(t, "- a\n  * b <!-- markdownlint-disable-line TEST001 -->\n  * c\n\nPlain\n\n- d\n  - e\n  - f\n",
		string(fix.ApplyEdits([]byte(content), result.Edits)))
}

// Members that carry the same edit fix their findings with it once, as
// findings that share a fix do.
func TestEngine_AppliesAnEditThatMembersShareOnce(t *testing.T) {
	t.Parallel()

	insertion := fix.TextEdit{StartOffset: 4, EndOffset: 4, NewText: "X"}
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, insertion),
		groupMember("TEST001", 3, 1, insertion))

	result := lintMarkdown(t, groupContent, rule)

	assert.Equal(t, 2, result.AcceptedFixes)
	assert.Equal(t, "- a\nX  * b\n  * c\n- d\n", string(fix.ApplyEdits([]byte(groupContent), result.Edits)))
}

// In a file with carriage returns, the group's edit writes the text between
// its members back as it is, and each member's own edit is written with the
// file's line endings.
func TestEngine_KeepsTheLineEndingsOfAFixGroup(t *testing.T) {
	t.Parallel()

	const content = "- a\r\n  * b\r\n\r\n  * c\r\n- d\r\n"
	rule := groupRule("TEST001",
		groupMember("TEST001", 2, 1, edit(t, content, "* b", "- b\n  ")),
		groupMember("TEST001", 4, 1, edit(t, content, "* c", "- c")))

	result := lintMarkdown(t, content, rule)

	require.Len(t, result.Diagnostics, 2)
	require.True(t, result.Diagnostics[0].HasFix())
	assert.Equal(t, "- b\r\n  ", result.Diagnostics[0].FixEdits[0].NewText)
	assert.Equal(t, 2, result.AcceptedFixes)
	assert.Equal(t, "- a\r\n  - b\r\n  \r\n\r\n  - c\r\n- d\r\n", string(fix.ApplyEdits([]byte(content), result.Edits)))
}
