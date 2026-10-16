package rules

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
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
		{
			name:          "es module import is javascript",
			input:         "```\nimport React from 'react';\n```",
			wantFix:       true,
			wantFixedLang: "javascript",
		},
		{
			name:          "shell test expression gets no guess",
			input:         "```\n[ \"$HOME\" = /root ] && echo root\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "prose gets no guess",
			input:         "```\nLet the constant settle before you continue.\n```",
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

// applyDefaultRule parses CommonMark input and applies a rule with no rule options.
func applyDefaultRule(t *testing.T, rule lint.Rule, input string) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	return diags
}

// TestCodeBlockStyleRule_DefaultConsistent checks the markdownlint default style "consistent".
func TestCodeBlockStyleRule_DefaultConsistent(t *testing.T) {
	if diags := applyDefaultRule(t, NewCodeBlockStyleRule(), "Text.\n\n    indented one\n\nText.\n\n    indented two\n"); len(diags) != 0 {
		t.Errorf("consistent indented blocks: got %d diagnostics, want 0", len(diags))
	}
	diags := applyDefaultRule(t, NewCodeBlockStyleRule(), "Text.\n\n    indented\n\nText.\n\n```\nfenced\n```\n")
	if len(diags) != 1 {
		t.Fatalf("indented then fenced: got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Message != "Code block style 'fenced' does not match expected 'indented'" {
		t.Errorf("unexpected message %q", diags[0].Message)
	}
}

// TestCodeFenceStyleRule_DefaultConsistent checks the markdownlint default style "consistent".
func TestCodeFenceStyleRule_DefaultConsistent(t *testing.T) {
	if diags := applyDefaultRule(t, NewCodeFenceStyleRule(), "~~~\none\n~~~\n\n~~~\ntwo\n~~~\n"); len(diags) != 0 {
		t.Errorf("consistent tilde fences: got %d diagnostics, want 0", len(diags))
	}
	diags := applyDefaultRule(t, NewCodeFenceStyleRule(), "~~~\none\n~~~\n\n```\ntwo\n```\n")
	if len(diags) != 1 {
		t.Fatalf("tilde then backtick: got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Message != "Code fence style 'backtick' does not match expected 'tilde'" {
		t.Errorf("unexpected message %q", diags[0].Message)
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

// TestCommandsShowOutputRule follows markdownlint MD014: every code block is
// checked regardless of its info string, the block is reported only when every
// non-blank line starts with "$" and whitespace, and each such line gets its
// own diagnostic and fix.
func TestCommandsShowOutputRule(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
		fixed string
	}{
		{
			name:  "all dollar signs no output",
			input: "```sh\n$ ls\n$ cat foo\n$ less bar\n```\n",
			want:  []position{{2, 1}, {3, 1}, {4, 1}},
			fixed: "```sh\nls\ncat foo\nless bar\n```\n",
		},
		{
			name:  "dollar signs with output",
			input: "```bash\n$ ls\nfoo bar\n$ cat foo\nHello world\n```\n",
		},
		{
			name:  "no dollar signs",
			input: "```sh\nls\ncat foo\n```\n",
		},
		{
			name:  "any info string is checked",
			input: "```go\n$ go test ./...\n```\n",
			want:  []position{{2, 1}},
			fixed: "```go\ngo test ./...\n```\n",
		},
		{
			name:  "info string with attributes",
			input: "```bash title=\"setup\"\n$ make\n```\n",
			want:  []position{{2, 1}},
			fixed: "```bash title=\"setup\"\nmake\n```\n",
		},
		{
			name:  "powershell block",
			input: "```powershell\n$ Get-ChildItem\n```\n",
			want:  []position{{2, 1}},
			fixed: "```powershell\nGet-ChildItem\n```\n",
		},
		{
			name:  "no info string",
			input: "```\n$ echo hello\n$ pwd\n```\n",
			want:  []position{{2, 1}, {3, 1}},
			fixed: "```\necho hello\npwd\n```\n",
		},
		{
			name:  "indented code block",
			input: "Text.\n\n    $ ls\n    $ pwd\n",
			want:  []position{{3, 5}, {4, 5}},
			fixed: "Text.\n\n    ls\n    pwd\n",
		},
		{
			name:  "leading whitespace and several spaces after the dollar",
			input: "```sh\n  $   ls\n```\n",
			want:  []position{{2, 3}},
			fixed: "```sh\n  ls\n```\n",
		},
		{
			name:  "dollar without following whitespace is output",
			input: "```sh\n$ echo $HOME\n$HOME\n```\n",
		},
		{
			name:  "bare dollar line is output",
			input: "```sh\n$ echo\n$\n```\n",
		},
		{
			name:  "empty code block",
			input: "```sh\n```\n",
		},
		{
			name:  "blank lines in code",
			input: "```sh\n$ echo 1\n\n$ echo 2\n```\n",
			want:  []position{{2, 1}, {4, 1}},
			fixed: "```sh\necho 1\n\necho 2\n```\n",
		},
		{
			name:  "fenced block in list item",
			input: "- item\n\n  ```sh\n  $ ls\n  ```\n",
			want:  []position{{4, 3}},
			fixed: "- item\n\n  ```sh\n  ls\n  ```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyDefaultRule(t, NewCommandsShowOutputRule(), tt.input)
			if len(diags) != len(tt.want) {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(diags), len(tt.want), diags)
			}
			var edits []fix.TextEdit
			for idx, want := range tt.want {
				if diags[idx].StartLine != want.line || diags[idx].StartColumn != want.column {
					t.Errorf("diagnostic %d at %d:%d, want %d:%d",
						idx, diags[idx].StartLine, diags[idx].StartColumn, want.line, want.column)
				}
				if len(diags[idx].FixEdits) == 0 {
					t.Errorf("diagnostic %d has no fix", idx)
				}
				edits = append(edits, diags[idx].FixEdits...)
			}
			if len(tt.want) == 0 {
				return
			}
			prepared, err := fix.PrepareEdits(edits, len(tt.input))
			if err != nil {
				t.Fatalf("PrepareEdits error: %v", err)
			}
			if got := string(fix.ApplyEdits([]byte(tt.input), prepared)); got != tt.fixed {
				t.Errorf("fixed = %q, want %q", got, tt.fixed)
			}
		})
	}
}
