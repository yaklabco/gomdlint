package runner_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestRunner_Run_OrdersDiagnosticsByPosition covers the order of a file's
// diagnostics, which every reporter prints: by line, as markdownlint-cli2
// sorts them, then by column, rule ID and message. Diagnostics equal in all
// of those keep the order their rule reported them in, which
// TestSortDiagnosticsIsStable covers for more diagnostics than a sort keeps
// stable by chance. The engine reports diagnostics rule by rule, so the
// rules here report out of order.
func TestRunner_Run_OrdersDiagnosticsByPosition(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Doc\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	at := func(ruleID string, line, column int, message, suggestion string) lint.Diagnostic {
		return lint.Diagnostic{RuleID: ruleID, StartLine: line, StartColumn: column, Message: message, Suggestion: suggestion}
	}
	registry := lint.NewRegistry()
	registry.Register(&diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "first-rule", "", nil, false),
		diags: []lint.Diagnostic{
			at("TEST001", 5, 1, "a", ""),
			at("TEST001", 9, 1, "same", "reported first"),
			at("TEST001", 2, 7, "b", ""),
			at("TEST001", 2, 3, "z", ""),
			at("TEST001", 9, 1, "same", "reported second"),
			at("TEST001", 2, 3, "a", ""),
		},
	})
	registry.Register(&diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST002", "second-rule", "", nil, false),
		diags: []lint.Diagnostic{
			at("TEST002", 2, 3, "a", ""),
			at("TEST002", 1, 1, "c", ""),
		},
	})

	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, registry)))
	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths: []string{"."}, WorkingDir: dir, Config: config.NewConfig(), Jobs: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Result == nil {
		t.Fatalf("want one linted file, got %+v", result.Files)
	}

	var got []string
	for _, diag := range result.Files[0].Result.Diagnostics {
		got = append(got, fmt.Sprintf("%d:%d %s %s %s", diag.StartLine, diag.StartColumn, diag.RuleID, diag.Message, diag.Suggestion))
	}
	want := []string{
		"1:1 TEST002 c ",
		"2:3 TEST001 a ",
		"2:3 TEST001 z ",
		"2:3 TEST002 a ",
		"2:7 TEST001 b ",
		"5:1 TEST001 a ",
		"9:1 TEST001 same reported first",
		"9:1 TEST001 same reported second",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("diagnostics in order:\n%q\nwant:\n%q", got, want)
	}
}
