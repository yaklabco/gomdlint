package langdetect_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/langdetect"
)

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "shebang bash",
			content:  "#!/bin/bash\necho hello",
			expected: "bash",
		},
		{
			name:     "shebang sh",
			content:  "#!/bin/sh\necho hello",
			expected: "bash",
		},
		{
			name:     "shebang python",
			content:  "#!/usr/bin/env python3\nprint('hello')",
			expected: "python",
		},
		{
			name:     "go code",
			content:  "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}",
			expected: "go",
		},
		{
			name:     "python code",
			content:  "def foo():\n    pass\n\nif __name__ == '__main__':\n    foo()",
			expected: "python",
		},
		{
			name:     "javascript code",
			content:  "const x = () => { return 42; };\nconsole.log(x());",
			expected: "javascript",
		},
		{
			name:     "json object",
			content:  `{"key": "value", "number": 123}`,
			expected: "json",
		},
		{
			name:     "yaml content",
			content:  "key: value\nother: 123\nlist:\n  - item1\n  - item2",
			expected: "yaml",
		},
		{
			name:     "rust code",
			content:  "fn main() {\n    println!(\"Hello, world!\");\n}",
			expected: "rust",
		},
		{
			name:     "plain text fallback",
			content:  "just some text without any code patterns",
			expected: "text",
		},
		{
			name:     "empty content fallback",
			content:  "",
			expected: "text",
		},
		{
			name:     "sql query",
			content:  "SELECT * FROM users WHERE id = 1;",
			expected: "sql",
		},
		{
			name:     "html content",
			content:  "<!DOCTYPE html>\n<html>\n<head><title>Test</title></head>\n<body></body>\n</html>",
			expected: "html",
		},
		{
			name:     "dockerfile",
			content:  "FROM golang:1.21\nWORKDIR /app\nCOPY . .\nRUN go build",
			expected: "dockerfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := langdetect.Detect([]byte(tt.content))

			if result != tt.expected {
				t.Errorf("Detect() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetect_ShebangTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Content looks like Python but has bash shebang
	content := []byte("#!/bin/bash\ndef foo():\n    pass")
	result := langdetect.Detect(content)

	if result != "bash" {
		t.Errorf("Detect() = %q, want %q (shebang should take precedence)", result, "bash")
	}
}

func TestDetect_NormalizesLanguageNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "shell normalizes to bash",
			content: "#!/bin/sh\necho test",
			want:    "bash",
		},
		{
			name:    "languages are lowercase",
			content: "package main\n\nfunc main() {}",
			want:    "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := langdetect.Detect([]byte(tt.content))

			if result != tt.want {
				t.Errorf("Detect() = %q, want %q", result, tt.want)
			}
		})
	}
}
