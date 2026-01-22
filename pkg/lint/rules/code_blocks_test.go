package rules

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestCodeBlockLanguageRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantN int
	}{
		{
			name:  "with language",
			input: "```go\ncode\n```",
			wantN: 0,
		},
		{
			name:  "without language",
			input: "```\ncode\n```",
			wantN: 1,
		},
		{
			name:  "with language and options",
			input: "```javascript title=\"example\"\ncode\n```",
			wantN: 0,
		},
		{
			name:  "indented block",
			input: "    code",
			wantN: 0, // Indented blocks don't need language.
		},
		{
			name:  "multiple fenced blocks",
			input: "```go\ncode\n```\n\n```\ncode\n```",
			wantN: 1,
		},
		{
			name:  "tilde fence with language",
			input: "~~~python\ncode\n~~~",
			wantN: 0,
		},
		{
			name:  "tilde fence without language",
			input: "~~~\ncode\n~~~",
			wantN: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCodeBlockLanguageRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

func TestCodeBlockLanguageRule_Autofix(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantFix       bool
		wantFixedLang string // Expected language to be inserted
	}{
		{
			name:          "go code",
			input:         "```\npackage main\n\nfunc main() {}\n```",
			wantFix:       true,
			wantFixedLang: "go",
		},
		{
			name:          "python code",
			input:         "```\ndef hello():\n    print('hello')\n```",
			wantFix:       true,
			wantFixedLang: "python",
		},
		{
			name:          "json code",
			input:         "```\n{\"key\": \"value\"}\n```",
			wantFix:       true,
			wantFixedLang: "json",
		},
		{
			name:          "undetectable code returns no fix",
			input:         "```\nsome random text\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "tilde fence go code",
			input:         "~~~\npackage main\n~~~",
			wantFix:       true,
			wantFixedLang: "go",
		},
		{
			name:          "empty code block",
			input:         "```\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCodeBlockLanguageRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) == 0 {
				t.Fatal("expected at least one diagnostic")
			}

			hasFix := len(diags[0].FixEdits) > 0
			if hasFix != tt.wantFix {
				t.Errorf("hasFix = %v, want %v", hasFix, tt.wantFix)
			}

			if tt.wantFix && hasFix {
				// Verify the fix inserts the expected language
				edit := diags[0].FixEdits[0]
				if edit.NewText != tt.wantFixedLang {
					t.Errorf("fix text = %q, want %q", edit.NewText, tt.wantFixedLang)
				}
			}
		})
	}
}

func TestCodeBlockLanguageRule_AllowedLanguages(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		allowed []any
		wantN   int
	}{
		{
			name:    "allowed language",
			input:   "```go\ncode\n```",
			allowed: []any{"go", "python"},
			wantN:   0,
		},
		{
			name:    "not allowed language",
			input:   "```rust\ncode\n```",
			allowed: []any{"go", "python"},
			wantN:   1,
		},
		{
			name:    "case insensitive",
			input:   "```Go\ncode\n```",
			allowed: []any{"go"},
			wantN:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCodeBlockLanguageRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"allowed_languages": tt.allowed,
				},
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

func TestCodeBlockStyleRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		style string
		wantN int
	}{
		{
			name:  "all fenced style fenced",
			input: "```\ncode\n```\n\n```\ncode2\n```",
			style: "fenced",
			wantN: 0,
		},
		{
			name:  "mixed style fenced",
			input: "```\ncode\n```\n\n    indented",
			style: "fenced",
			wantN: 1,
		},
		{
			name:  "all indented style indented",
			input: "    code\n\n    code2",
			style: "indented",
			wantN: 0,
		},
		{
			name:  "consistent fenced first",
			input: "```\ncode\n```\n\n    indented",
			style: "consistent",
			wantN: 1,
		},
		{
			name:  "consistent indented first",
			input: "    indented\n\n```\nfenced\n```",
			style: "consistent",
			wantN: 1,
		},
		{
			name:  "single fenced",
			input: "```\ncode\n```",
			style: "consistent",
			wantN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCodeBlockStyleRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"style": tt.style,
				},
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

func TestCodeFenceStyleRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		style   string
		wantN   int
		wantFix bool
	}{
		{
			name:    "all backticks style backtick",
			input:   "```\ncode\n```\n\n```\ncode2\n```",
			style:   "backtick",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "all tildes style tilde",
			input:   "~~~\ncode\n~~~\n\n~~~\ncode2\n~~~",
			style:   "tilde",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "mixed style backtick",
			input:   "```\ncode\n```\n\n~~~\ncode2\n~~~",
			style:   "backtick",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "mixed style tilde",
			input:   "~~~\ncode\n~~~\n\n```\ncode2\n```",
			style:   "tilde",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "consistent backtick first",
			input:   "```\ncode\n```\n\n~~~\ncode2\n~~~",
			style:   "consistent",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "consistent tilde first",
			input:   "~~~\ncode\n~~~\n\n```\ncode2\n```",
			style:   "consistent",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "indented blocks ignored",
			input:   "```\ncode\n```\n\n    indented",
			style:   "backtick",
			wantN:   0,
			wantFix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCodeFenceStyleRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"style": tt.style,
				},
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}

func TestCommandsShowOutputRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantN   int
		wantFix bool
	}{
		{
			name:    "all dollar signs no output",
			input:   "```sh\n$ ls\n$ cat foo\n$ less bar\n```",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "dollar signs with output",
			input:   "```bash\n$ ls\nfoo bar\n$ cat foo\nHello world\n```",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "no dollar signs",
			input:   "```sh\nls\ncat foo\n```",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "mixed with output",
			input:   "```bash\n$ mkdir test\nmkdir: created directory 'test'\n$ ls test\n```",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "non-shell code block",
			input:   "```go\n$ not a command\n```",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "console block all dollar",
			input:   "```console\n$ echo hello\n$ pwd\n```",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "zsh block all dollar",
			input:   "```zsh\n$ cd /tmp\n$ ls\n```",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "terminal block all dollar",
			input:   "```terminal\n$ npm install\n$ npm test\n```",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "empty code block",
			input:   "```sh\n```",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "blank lines in code",
			input:   "```sh\n$ echo 1\n\n$ echo 2\n```",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "single dollar command",
			input:   "```sh\n$ pwd\n```",
			wantN:   1,
			wantFix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewCommandsShowOutputRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}
