package lint_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// sharedKey is the key the test rules share a value under.
type sharedKey struct{ name string }

// sharingRule is a test rule that runs apply with its context.
type sharingRule struct {
	lint.BaseRule
	apply func(ctx *lint.RuleContext)
}

func (r *sharingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	r.apply(ctx)
	return nil, nil
}

// newSharingRule returns a test rule with the given ID that runs apply.
func newSharingRule(id string, apply func(ctx *lint.RuleContext)) *sharingRule {
	return &sharingRule{BaseRule: lint.NewBaseRule(id, "sharing-"+id, "", nil, false), apply: apply}
}

// lintWith lints a one-line document with an engine that runs rules, in ID
// order.
func lintWith(t *testing.T, rules ...lint.Rule) *lint.FileResult {
	t.Helper()
	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	result, err := lint.NewEngine(&mockParser{}, registry).
		LintFile(context.Background(), "test.md", []byte("# One\n"), config.NewConfig())
	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}
	return result
}

func TestSharedValue_BuiltOncePerLintOfAFile(t *testing.T) {
	t.Parallel()

	builds := 0
	var got []*int
	share := func(ctx *lint.RuleContext) {
		got = append(got, lint.SharedValue(ctx, sharedKey{"count"}, func() *int {
			builds++
			value := builds
			return &value
		}))
	}
	rules := []lint.Rule{newSharingRule("TEST001", share), newSharingRule("TEST002", share)}

	lintWith(t, rules...)
	if builds != 1 || len(got) != 2 || got[0] != got[1] {
		t.Fatalf("one lint: %d builds, values %v; want one value built once and shared", builds, got)
	}

	// Each lint of a file, as each pass of --fix, starts with no values.
	lintWith(t, rules...)
	if builds != 2 || len(got) != 4 || got[2] != got[3] || got[2] == got[0] || *got[2] != 2 {
		t.Fatalf("second lint: %d builds; want a value of its own, built once", builds)
	}
}

func TestSharedValue_KeysAreApart(t *testing.T) {
	t.Parallel()

	builds := make(map[string]int)
	share := func(ctx *lint.RuleContext, name string) string {
		return lint.SharedValue(ctx, sharedKey{name}, func() string {
			builds[name]++
			return "built for " + name
		})
	}
	rule := newSharingRule("TEST001", func(ctx *lint.RuleContext) {
		for _, name := range []string{"gfm", "commonmark", "gfm", "commonmark"} {
			if value := share(ctx, name); value != "built for "+name {
				t.Errorf("value of %s = %q", name, value)
			}
		}
		// The same key with a value of another type is built and not kept.
		for range 2 {
			if other := lint.SharedValue(ctx, sharedKey{"gfm"}, func() int { builds["gfm int"]++; return 7 }); other != 7 {
				t.Errorf("value of another type = %d, want 7", other)
			}
		}
		if value := share(ctx, "gfm"); value != "built for gfm" {
			t.Errorf("value of gfm after another type = %q", value)
		}
	})
	lintWith(t, rule)

	want := map[string]int{"gfm": 1, "commonmark": 1, "gfm int": 2}
	for name, count := range want {
		if builds[name] != count {
			t.Errorf("builds of %s = %d, want %d", name, builds[name], count)
		}
	}
}

func TestSharedValue_BuildMayShareAnotherValue(t *testing.T) {
	t.Parallel()

	builds := 0
	parse := func(ctx *lint.RuleContext) string {
		return lint.SharedValue(ctx, sharedKey{"parse"}, func() string { builds++; return "parse" })
	}
	outline := func(ctx *lint.RuleContext) string {
		return lint.SharedValue(ctx, sharedKey{"outline"}, func() string { return "outline of " + parse(ctx) })
	}
	first := newSharingRule("TEST001", func(ctx *lint.RuleContext) {
		if value := outline(ctx); value != "outline of parse" {
			t.Errorf("outline = %q", value)
		}
	})
	second := newSharingRule("TEST002", func(ctx *lint.RuleContext) {
		if value := parse(ctx); value != "parse" {
			t.Errorf("parse = %q", value)
		}
		if value := outline(ctx); value != "outline of parse" {
			t.Errorf("outline = %q", value)
		}
	})
	lintWith(t, first, second)

	if builds != 1 {
		t.Fatalf("builds of the parse = %d, want 1: a value built inside another build is kept", builds)
	}
}

func TestSharedValue_PanickingBuildKeepsNothing(t *testing.T) {
	t.Parallel()

	builds := 0
	var got string
	panics := newSharingRule("TEST001", func(ctx *lint.RuleContext) {
		lint.SharedValue(ctx, sharedKey{"value"}, func() string {
			builds++
			panic("half built")
		})
	})
	reads := newSharingRule("TEST002", func(ctx *lint.RuleContext) {
		got = lint.SharedValue(ctx, sharedKey{"value"}, func() string {
			builds++
			return "complete"
		})
	})
	result := lintWith(t, panics, reads)

	if !errors.Is(result.RuleErrors["TEST001"], lint.ErrRulePanic) {
		t.Fatalf("TEST001 error = %v, want a rule panic", result.RuleErrors["TEST001"])
	}
	if builds != 2 || got != "complete" {
		t.Fatalf("after a panicking build: %d builds, value %q; want the next rule to build its own", builds, got)
	}
}

func TestSharedValue_CancelledBuildIsNotKept(t *testing.T) {
	t.Parallel()

	builds := 0
	rule := newSharingRule("TEST001", func(ctx *lint.RuleContext) {
		cancelled, cancel := context.WithCancel(ctx.Ctx)
		copied := *ctx
		copied.Ctx = cancelled
		lint.SharedValue(&copied, sharedKey{"value"}, func() int {
			builds++
			cancel()
			return builds
		})
		if value := lint.SharedValue(ctx, sharedKey{"value"}, func() int { builds++; return builds }); value != 2 {
			t.Errorf("value after a cancelled build = %d, want 2", value)
		}
	})
	lintWith(t, rule)

	if builds != 2 {
		t.Fatalf("builds = %d, want 2: a value built while the lint is cancelled may be incomplete", builds)
	}
}

func TestSharedValue_OtherFileIsNotServed(t *testing.T) {
	t.Parallel()

	rule := newSharingRule("TEST001", func(ctx *lint.RuleContext) {
		if value := lint.SharedValue(ctx, sharedKey{"content"}, func() string { return string(ctx.File.Content) }); value != "# One\n" {
			t.Errorf("value = %q, want the file's content", value)
		}
		other := *ctx
		content := []byte("# Two\n")
		other.File = &mdast.FileSnapshot{Content: content, Lines: mdast.BuildLines(content), Root: mdast.NewNode(mdast.NodeDocument)}
		other.Root = other.File.Root
		for range 2 {
			if value := lint.SharedValue(&other, sharedKey{"content"}, func() string { return string(other.File.Content) }); value != "# Two\n" {
				t.Errorf("value for another file = %q, want that file's content", value)
			}
		}
		if value := lint.SharedValue(ctx, sharedKey{"content"}, func() string { return "rebuilt" }); value != "# One\n" {
			t.Errorf("value = %q, want the file's content, kept", value)
		}
	})
	lintWith(t, rule)
}

func TestSharedValue_WithoutEngineBuildsEachTime(t *testing.T) {
	t.Parallel()

	content := []byte("# One\n")
	file := &mdast.FileSnapshot{Content: content, Lines: mdast.BuildLines(content), Root: mdast.NewNode(mdast.NodeDocument)}
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)
	builds := 0
	for range 2 {
		lint.SharedValue(ctx, sharedKey{"value"}, func() int { builds++; return builds })
	}
	if builds != 2 {
		t.Fatalf("builds = %d, want 2: a context the engine did not make shares nothing", builds)
	}
}
