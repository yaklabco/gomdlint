package runner

import (
	"strconv"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestSortDiagnosticsIsStable covers many diagnostics equal in line, column,
// rule ID and message, spread over the slice: each group keeps the order in
// which its diagnostics arrived. Go sorts up to 12 elements by insertion,
// which is stable anyway, so the test sorts more.
func TestSortDiagnosticsIsStable(t *testing.T) {
	t.Parallel()

	const count = 200
	diagnostics := make([]lint.Diagnostic, 0, count)
	for idx := range count {
		diagnostics = append(diagnostics, lint.Diagnostic{
			RuleID: "MD009", Message: "Trailing whitespace", StartLine: 1 + (idx*7)%5, StartColumn: 1,
			Suggestion: strconv.Itoa(idx), // Records the arrival order.
		})
	}

	sortDiagnostics(diagnostics)

	lastArrival := make(map[int]int)
	for idx, diag := range diagnostics {
		if idx > 0 && diag.StartLine < diagnostics[idx-1].StartLine {
			t.Fatalf("line %d follows line %d", diag.StartLine, diagnostics[idx-1].StartLine)
		}
		arrival, err := strconv.Atoi(diag.Suggestion)
		if err != nil {
			t.Fatal(err)
		}
		if previous, seen := lastArrival[diag.StartLine]; seen && arrival < previous {
			t.Errorf("line %d: diagnostic %d follows diagnostic %d", diag.StartLine, arrival, previous)
		}
		lastArrival[diag.StartLine] = arrival
	}
}

// TestSortDiagnosticsOrdersPositionsInsideACharacter covers two findings at
// the byte column of one emoji: one at its start and one a UTF-16 code unit
// into it, as markdownlint's line length rule can report. The finding
// inside the emoji has the later reported column, so it comes second, before
// the rule ID decides.
func TestSortDiagnosticsOrdersPositionsInsideACharacter(t *testing.T) {
	t.Parallel()

	diagnostics := []lint.Diagnostic{
		{RuleID: "MD001", StartLine: 1, StartColumn: 80, StartColumnUTF16Offset: 1},
		{RuleID: "MD013", StartLine: 1, StartColumn: 80},
	}

	sortDiagnostics(diagnostics)

	if diagnostics[0].RuleID != "MD013" || diagnostics[1].RuleID != "MD001" {
		t.Errorf("got %s then %s, want the finding at the start of the emoji first",
			diagnostics[0].RuleID, diagnostics[1].RuleID)
	}
}
