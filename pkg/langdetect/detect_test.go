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

// TestDetect_RefusesWeakSignals covers content that weak substring heuristics
// misdetected. Detection must return "text" unless it is confident.
func TestDetect_RefusesWeakSignals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{"shell test expression is not json", "[ \"$HOME\" = /root ] && echo root"},
		{"english prose with let and const", "Let me explain: we let the constant drift, then const values settle."},
		{"prose with an arrow", "Click Save => the dialog closes."},
		{"c const declaration", "const int max = 10;\nint total = max * 2;"},
		{"directory tree", ".\n├── cmd/\n│   └── main.go\n└── README.md"},
		{"directory tree with comments", "project/\n- docs: documentation\n- src: source code"},
		{"flat key value prose", "Note: this matters.\nWarning: be careful."},
		{"prose that starts like sql", "Select the files from the list below."},
		{"python looking words", "We import data from the warehouse and define the import process."},
		{"single word", "hello"},
		{"cli usage", "Usage: gomdlint [flags] <paths>\n\nFlags:\n  --fix    apply fixes"},
		{"log output", "2024-01-01T00:00:00Z INFO server: started\n2024-01-01T00:00:01Z WARN db: slow"},
		{"shell commands", "npm install\nnpm run build"},
		{"http request", "GET /api/users HTTP/1.1\nHost: example.com\nAccept: application/json"},
		{"toml", "[package]\nname = \"demo\"\nversion = \"0.1.0\""},
		{"indented tree", "src/\n  main.go\n  util/\n    helpers.go"},
		{"unfinished console call in prose", "Open the console.log( file to see output"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := langdetect.Detect([]byte(tt.content)); got != "text" {
				t.Errorf("Detect() = %q, want %q", got, "text")
			}
		})
	}
}

// TestDetect_ConfidentLanguages covers strong, language-specific signals.
func TestDetect_ConfidentLanguages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"javascript es module import", "import React from 'react';\n\nexport default function App() {}", "javascript"},
		{"javascript require", "const fs = require('fs');", "javascript"},
		{"typescript interface", "export interface User {\n  name: string;\n}", "typescript"},
		{"typescript import with types", "import { Router } from 'express';\nconst port: number = 3000;", "typescript"},
		{"python import", "import os\nimport sys\n\nprint(os.getcwd())", "python"},
		{"python from import", "from pathlib import Path\n\nroot = Path('.')", "python"},
		{"python class", "class Greeter:\n    pass", "python"},
		{"go with leading comment", "// Package demo shows things.\npackage demo\n\nfunc F() {}", "go"},
		{"java package is not go", "package com.example;\n\npublic class App {}", "text"},
		{"rust function", "pub fn add(a: i32, b: i32) -> i32 {\n    a + b\n}", "rust"},
		{"c include", "#include <stdio.h>\n\nint main(void) { return 0; }", "c"},
		{"cpp include", "#include <iostream>\n\nint main() { std::cout << 1; }", "cpp"},
		{"php", "<?php\necho 'hi';", "php"},
		{"xml", "<?xml version=\"1.0\"?>\n<root/>", "xml"},
		{"unified diff", "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new", "diff"},
		{"nested yaml", "services:\n  web:\n    image: nginx\nversion: \"3\"", "yaml"},
		{"json array", "[\"a\", \"b\"]", "json"},
		{"uppercase sql insert", "INSERT INTO users (id) VALUES (1);", "sql"},
		{"dockerfile with arg", "ARG BASE=golang\nFROM ${BASE}\nRUN go build", "dockerfile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := langdetect.Detect([]byte(tt.content)); got != tt.want {
				t.Errorf("Detect() = %q, want %q", got, tt.want)
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
