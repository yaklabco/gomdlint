package langdetect_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/langdetect"
)

// TestDetect_WholeBlockEvidence covers the only evidence Detect accepts:
// the whole block says which language it is, in a way that strings and
// comments cannot fake.
func TestDetect_WholeBlockEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"shebang bash", "#!/bin/bash\necho hello", "bash"},
		{"shebang sh normalizes to bash", "#!/bin/sh\necho hello", "bash"},
		{"shebang env python3", "#!/usr/bin/env python3\nprint('hello')", "python"},
		{"shebang wins over the content", "#!/bin/bash\ndef foo():\n    pass", "bash"},
		{"multi-word language name uses an alias", "#!/usr/bin/env sbcl\n(print 1)", "lisp"},
		{"json object", `{"key": "value", "number": 123}`, "json"},
		{"json array over lines", "[\n  \"a\",\n  \"b\"\n]\n", "json"},
		{"xml document", "<?xml version=\"1.0\"?>\n<root>\n  <item a=\"1\"/>\n</root>\n", "xml"},
		{"php opening tag", "<?php\necho 'hi';", "php"},
		{"html doctype", "<!DOCTYPE html>\n<html>\n<body></body>\n</html>", "html"},
		{"git diff", "diff --git a/x b/x\nindex 83db48f..bf269f4 100644\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n", "diff"},
		{"unified diff with two hunks", "--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n-b\n+c\n@@ -10 +10,2 @@\n x\n+y\n", "diff"},
		{"diff without a final newline marker", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n", "diff"},
		{"two file sections", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n--- a/g\n+++ b/g\n@@ -1 +1 @@\n-c\n+d\n", "diff"},
		{"dockerfile", "FROM golang:1.21\nWORKDIR /app\nCOPY . .\nRUN go build", "dockerfile"},
		{"dockerfile with a lowercase stage keyword", "FROM golang:1.21 as build\nRUN go build\n", "dockerfile"},
		{"dockerfile with comments and continuations", "# build stage\nFROM golang:1.21 AS build\nRUN apt-get update && \\\n    apt-get install -y git\nCMD [\"app\"]\n", "dockerfile"},
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

// TestDetect_NoWholeBlockEvidence covers content that names no language
// beyond doubt: statements of one language can sit in another's strings or
// comments, so they never name a language, and structures that are only
// partly there do not count.
func TestDetect_NoWholeBlockEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"go code", "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}"},
		{"python code", "def foo():\n    pass\n\nif __name__ == '__main__':\n    foo()"},
		{"javascript code", "const x = () => { return 42; };\nconsole.log(x());"},
		{"typescript code", "export interface User {\n  name: string;\n}"},
		{"yaml content", "key: value\nother: 123\nlist:\n  - item1\n  - item2"},
		{"rust code", "fn main() {\n    println!(\"Hello, world!\");\n}"},
		{"sql query", "SELECT * FROM users WHERE id = 1;"},
		{"c include", "#include <stdio.h>\n\nint main(void) { return 0; }"},
		{"python with a javascript call in a comment", "# console.log(42)\nprint(42)"},
		{"python with a diff in a string", "def patch():\n    return \"\"\"\n--- old\n+++ new\n@@ -1 +1 @@\n-a\n+b\n\"\"\"\n"},
		{"diff headers without hunks", "--- old\n+++ new\n"},
		{"diff with prose after it", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\nThat is the change.\n"},
		{"diff with prose before it", "Apply this:\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n"},
		{"diff whose hunk is short", "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n-a\n+b\n"},
		{"json followed by prose", "{\"a\": 1}\nand more"},
		{"xml that is not well formed", "<?xml version=\"1.0\"?>\n<root><item></root>\n"},
		{"xml with text after the root", "<?xml version=\"1.0\"?>\n<root/>\ntrailing words\n"},
		{"dockerfile that starts with ARG", "ARG BASE=golang\nFROM ${BASE}\nRUN go build"},
		{"single FROM line", "FROM golang:1.21"},
		{"prose that starts with FROM", "FROM the start, read every line.\nRUN the tests after."},
		{"dockerfile with a stray line", "FROM alpine\nRUN apk add git\nthen run it\n"},
		{"shell test expression", "[ \"$HOME\" = /root ] && echo root"},
		{"english prose", "Let me explain: we let the constant drift, then const values settle."},
		{"shell commands", "npm install\nnpm run build"},
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
