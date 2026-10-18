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
			name:          "go statements are not whole-block evidence",
			input:         "```\npackage main\n\nfunc main() {}\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "python statements are not whole-block evidence",
			input:         "```\ndef hello():\n    print('hello')\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "shebang names the language",
			input:         "```\n#!/usr/bin/env python3\nprint('hello')\n```",
			wantFix:       true,
			wantFixedLang: "python",
		},
		{
			name:          "complete diff",
			input:         "```\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new\n```",
			wantFix:       true,
			wantFixedLang: "diff",
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
			name:          "tilde fence json",
			input:         "~~~\n[1, 2]\n~~~",
			wantFix:       true,
			wantFixedLang: "json",
		},
		{
			name:          "empty code block",
			input:         "```\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "es module import gets no guess",
			input:         "```\nimport React from 'react';\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "javascript call inside a python string gets no guess",
			input:         "```\ndef example():\n    print(\"console.log(x)\")\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "javascript call in a python comment gets no guess",
			input:         "```\n# console.log(42)\nprint(42)\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "diff inside a python string gets no guess",
			input:         "```\ndef patch():\n    return \"\"\"\n--- old\n+++ new\n\"\"\"\n```",
			wantFix:       false,
			wantFixedLang: "",
		},
		{
			name:          "python with a javascript comment line gets no guess",
			input:         "```\nimport os\n# console.log(x)\n```",
			wantFix:       false,
			wantFixedLang: "",
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
			// markdownlint compares the language with each allowed name exactly.
			name:    "capitalized language with a lowercase name",
			input:   "```Go\ncode\n```",
			allowed: []any{"go"},
			wantN:   1,
		},
		{
			name:    "lowercase language with a capitalized name",
			input:   "```go\ncode\n```",
			allowed: []any{"Go"},
			wantN:   1,
		},
		{
			name:    "language written as allowed",
			input:   "```Go\ncode\n```",
			allowed: []any{"Go"},
			wantN:   0,
		},
		{
			name:    "empty list allows every language",
			input:   "```rust\ncode\n```",
			allowed: []any{},
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

// TestCodeBlockLanguageRule_FixKeepsToAllowedLanguages verifies that MD040
// inserts a detected language only when allowed_languages, matched exactly
// as markdownlint matches it, permits that language, so that the fix never
// writes a language the rule would report on the next run.
func TestCodeBlockLanguageRule_FixKeepsToAllowedLanguages(t *testing.T) {
	const bashBlock = "```\n#!/bin/bash\necho hi\n```\n"
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantFixed string // Output after the fixes; empty means unchanged.
	}{
		{name: "detected language not allowed", input: bashBlock,
			options: map[string]any{"allowed_languages": []any{"go", "ruby"}}},
		{name: "detected language allowed", input: bashBlock,
			options:   map[string]any{"allowed_languages": []any{"go", "bash"}},
			wantFixed: "```bash\n#!/bin/bash\necho hi\n```\n"},
		{name: "detected language allowed only in another case", input: bashBlock,
			options: map[string]any{"allowed_languages": []any{"Bash"}}},
		{name: "detected JSON allowed", input: "```\n{\"a\": 1}\n```\n",
			options:   map[string]any{"allowed_languages": []any{"json"}},
			wantFixed: "```json\n{\"a\": 1}\n```\n"},
		{name: "empty list allows the detected language", input: bashBlock,
			options:   map[string]any{"allowed_languages": []any{}},
			wantFixed: "```bash\n#!/bin/bash\necho hi\n```\n"},
		{name: "no list allows the detected language", input: bashBlock,
			wantFixed: "```bash\n#!/bin/bash\necho hi\n```\n"},
		{name: "list without names allows no language", input: bashBlock,
			options: map[string]any{"allowed_languages": []any{1}}},
		{name: "detected language allowed with language only", input: bashBlock,
			options:   map[string]any{"allowed_languages": []any{"bash"}, "language_only": true},
			wantFixed: "```bash\n#!/bin/bash\necho hi\n```\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewCodeBlockLanguageRule(), tt.input, tt.options)
			require.Len(t, diags, 1)
			assert.Equal(t, "Fenced code block has no language specified", diags[0].Message)
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			fixed := applyAllEdits(t, tt.input, diags)
			assert.Equal(t, want, fixed)
			// A fixed block passes; an unfixed one is reported again without a fix.
			again := applyRuleWithOptions(t, NewCodeBlockLanguageRule(), fixed, tt.options)
			if fixed != tt.input {
				assert.Empty(t, again)
			}
			for _, diag := range again {
				assert.False(t, diag.HasFix())
			}
		})
	}
}

// TestCodeBlockLanguageRule_AllowedLanguageMessages verifies that MD040 names
// a language that allowed_languages leaves out as it is written, and that,
// like markdownlint, it checks the allowed languages of a fence whose info
// string language_only also reports.
func TestCodeBlockLanguageRule_AllowedLanguageMessages(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		options      map[string]any
		wantMessages []string
	}{
		{name: "language in another case", input: "```Go\ncode\n```\n",
			options:      map[string]any{"allowed_languages": []any{"go"}},
			wantMessages: []string{"Language 'Go' is not in the allowed list"}},
		{name: "info string with more than a language", input: "```bash title=x\ncode\n```\n",
			options: map[string]any{"allowed_languages": []any{"go"}, "language_only": true},
			wantMessages: []string{
				"Code fence info must contain only a language identifier",
				"Language 'bash' is not in the allowed list",
			}},
		{name: "allowed language with more than a language", input: "```go title=x\ncode\n```\n",
			options:      map[string]any{"allowed_languages": []any{"go"}, "language_only": true},
			wantMessages: []string{"Code fence info must contain only a language identifier"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewCodeBlockLanguageRule(), tt.input, tt.options)
			messages := make([]string, 0, len(diags))
			for _, diag := range diags {
				messages = append(messages, diag.Message)
				assert.False(t, diag.HasFix())
			}
			assert.ElementsMatch(t, tt.wantMessages, messages)
		})
	}
}

// TestCodeBlockLanguageRule_InfoStringWords verifies that MD040 splits an
// info string into its language and the rest at spaces and tabs only, as GFM
// and markdownlint do, so that another space character such as U+00A0 is
// part of the language, and that its fix leaves a fence whose language is
// exactly the detected one.
func TestCodeBlockLanguageRule_InfoStringWords(t *testing.T) {
	const shebang = "#!/bin/bash\necho hi\n```\n"
	allowBash := map[string]any{"allowed_languages": []any{"bash"}, "language_only": true}
	tests := []struct {
		name         string
		input        string
		options      map[string]any
		wantMessages []string
		wantFixed    string // Output after the fixes; empty means unchanged.
	}{
		{name: "no-break space alone", input: "```\u00a0\n" + shebang, options: allowBash,
			wantMessages: []string{"Language '\u00a0' is not in the allowed list"}},
		{name: "no-break space alone without options", input: "```\u00a0\n" + shebang},
		{name: "em space alone", input: "```\u2003\n" + shebang, options: allowBash,
			wantMessages: []string{"Language '\u2003' is not in the allowed list"}},
		{name: "form feed alone", input: "```\f\n" + shebang, options: allowBash,
			wantMessages: []string{"Language '\f' is not in the allowed list"}},
		{name: "vertical tab alone without options", input: "```\v\n" + shebang},
		{name: "title after a tab", input: "```bash\ttitle=x\ncode\n```\n", options: allowBash,
			wantMessages: []string{"Code fence info must contain only a language identifier"}},
		{name: "title after a no-break space", input: "```bash\u00a0title=x\ncode\n```\n", options: allowBash,
			wantMessages: []string{"Language 'bash\u00a0title=x' is not in the allowed list"}},
		{name: "tab before the language", input: "```\tbash\ncode\n```\n", options: allowBash},
		{name: "tab after the fence", input: "```\t\n" + shebang, options: allowBash,
			wantMessages: []string{"Fenced code block has no language specified"},
			wantFixed:    "```bash\t\n" + shebang},
		{name: "spaces after the fence", input: "```  \n" + shebang, options: allowBash,
			wantMessages: []string{"Fenced code block has no language specified"},
			wantFixed:    "```bash  \n" + shebang},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewCodeBlockLanguageRule(), tt.input, tt.options)
			messages := make([]string, 0, len(diags))
			for _, diag := range diags {
				messages = append(messages, diag.Message)
			}
			assert.ElementsMatch(t, tt.wantMessages, messages)

			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			fixed := applyAllEdits(t, tt.input, diags)
			assert.Equal(t, want, fixed)
			if fixed != tt.input {
				// The inserted language is the fence's whole language, and allowed.
				assert.Empty(t, applyRuleWithOptions(t, NewCodeBlockLanguageRule(), fixed, tt.options))
			}
		})
	}
}

// TestFenceLanguageAfterInsert verifies that the language a fence would name
// after the MD040 fix inserts text after its fence characters is the first
// word of the line that follows, split at spaces and tabs only.
func TestFenceLanguageAfterInsert(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "nothing after the fence", content: "```\ncode\n```\n", want: "bash"},
		{name: "spaces after the fence", content: "```  \ncode\n```\n", want: "bash"},
		{name: "tab after the fence", content: "```\t\ncode\n```\n", want: "bash"},
		{name: "carriage return line ending", content: "```\r\ncode\r\n```\r\n", want: "bash"},
		{name: "fence on the last line", content: "```", want: "bash"},
		{name: "no-break space after the fence", content: "```\u00a0\ncode\n```\n", want: "bash\u00a0"},
		{name: "word after the fence", content: "```sh x\ncode\n```\n", want: "bashsh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(tt.content))
			require.NoError(t, err)
			assert.Equal(t, tt.want, fenceLanguageAfterInsert(file, len("```"), "bash"))
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

// TestCodeFenceStyleRule_SafeFix rewrites the whole opening and closing fence
// runs and refuses (reports without a fix) when the new fence would change
// where the code block ends or would not be a fence.
func TestCodeFenceStyleRule_SafeFix(t *testing.T) {
	options := map[string]any{"style": "backtick"}
	tests := []struct {
		name    string
		input   string
		wantFix bool
		fixed   string
	}{
		{"simple", "~~~\nx\n~~~\n", true, "```\nx\n```\n"},
		{"longer closing run", "~~~~~ js\ny\n~~~~~~~~\n", true, "````` js\ny\n````````\n"},
		{"inside list item", "- item\n\n  ~~~\n  in list\n  ~~~\n", true, "- item\n\n  ```\n  in list\n  ```\n"},
		{"inside blockquote", "> ~~~\n> quoted\n> ~~~\n", true, "> ```\n> quoted\n> ```\n"},
		{"unclosed fence", "~~~\nrest of file\n", true, "```\nrest of file\n"},
		{"empty block", "~~~\n~~~\n", true, "```\n```\n"},
		{"content holds a backtick fence", "~~~\n```\nnested\n```\n~~~\n", false, ""},
		{"backtick in info string", "~~~ a`b\nx\n~~~\n", false, ""},
		{"shorter backtick run in content", "~~~~\n```\nnested\n```\n~~~~\n", true, "````\n```\nnested\n```\n````\n"},
		{"backtick run with an info string in content", "~~~\n```js\nx\n~~~\n", true, "```\n```js\nx\n```\n"},
		{"tab-indented backtick run in content", "~~~\n\t```\n~~~\n", true, "```\n\t```\n```\n"},
		{"empty block with a longer closer", "~~~\n~~~~~\n\nText\n", true, "```\n`````\n\nText\n"},
		{"backtick in info string without a space", "~~~foo`bar\n~~~\n\nText\n", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), &config.RuleConfig{Options: options})
			diags, err := NewCodeFenceStyleRule().Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %+v", len(diags), diags)
			}
			if diags[0].HasFix() != tt.wantFix {
				t.Fatalf("HasFix = %v, want %v", diags[0].HasFix(), tt.wantFix)
			}
			if !tt.wantFix {
				return
			}
			prepared, err := fix.PrepareEdits(diags[0].FixEdits, len(tt.input))
			if err != nil {
				t.Fatalf("PrepareEdits error: %v", err)
			}
			if got := string(fix.ApplyEdits([]byte(tt.input), prepared)); got != tt.fixed {
				t.Errorf("fixed = %q, want %q", got, tt.fixed)
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
		{
			name:  "fenced block in blockquote",
			input: "> ```sh\n> $ ls\n> $ pwd\n> ```\n",
			want:  []position{{2, 3}, {3, 3}},
			fixed: "> ```sh\n> ls\n> pwd\n> ```\n",
		},
		{
			name:  "fence on the first line of a list item",
			input: "- ```sh\n  $ ls\n  ```\n",
			want:  []position{{2, 3}},
			fixed: "- ```sh\n  ls\n  ```\n",
		},
		{
			name:  "whitespace-only line is output",
			input: "```sh\n$ ls\n   \n$ pwd\n```\n",
		},
		{
			name:  "fence indentation is not content",
			input: " ```sh\n $ ls\n \n $ pwd\n ```\n",
			want:  []position{{2, 2}, {4, 2}},
			fixed: " ```sh\n ls\n \n pwd\n ```\n",
		},
		{
			name:  "indented code whitespace beyond the indent is output",
			input: "    $ ls\n      \n    $ pwd\n",
		},
		{
			name:  "indented code line of only its indent is blank",
			input: "    $ ls\n    \n    $ pwd\n",
			want:  []position{{1, 5}, {3, 5}},
			fixed: "    ls\n    \n    pwd\n",
		},
		{
			name:  "indented code short whitespace line is blank",
			input: "    $ ls\n  \n    $ pwd\n",
			want:  []position{{1, 5}, {3, 5}},
			fixed: "    ls\n  \n    pwd\n",
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

// TestCommandsShowOutputRule_PromptThatLooksLikeAFence covers a prompt line
// whose command is a fence: removing its prompt would close the code block
// there and turn the lines after it into other blocks. Both prompts are
// reported, as markdownlint does, but the fix check withholds that line's
// fix, so the block still ends where it does now.
func TestCommandsShowOutputRule_PromptThatLooksLikeAFence(t *testing.T) {
	tests := []struct {
		name, input, fixed string
	}{
		{"backtick fence", "```sh\n$ ```\n$ pwd\n```\n", "```sh\n$ ```\npwd\n```\n"},
		{"tilde fence", "~~~sh\n$ ~~~\n$ pwd\n~~~\n", "~~~sh\n$ ~~~\npwd\n~~~\n"},
		{"four-backtick fence", "````sh\n$ ````\n$ pwd\n````\n", "````sh\n$ ````\npwd\n````\n"},
	}
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, tt := range tests {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				diags, err := NewCommandsShowOutputRule().Apply(fixSafetyContext(t, flavor, tt.input))
				require.NoError(t, err)
				require.Len(t, diags, 2)
				assert.Equal(t, []int{2, 3}, []int{diags[0].StartLine, diags[1].StartLine})
				assert.False(t, diags[0].HasFix(), "removing the prompt would close the fence")
				assert.Equal(t, tt.fixed, applyAllFixEdits(t, tt.input, diags))
			})
		}
	}
}
