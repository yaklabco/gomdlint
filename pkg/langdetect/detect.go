// Package langdetect detects the language of code content for MD040, which
// inserts the detected language into fenced code blocks that lack one.
//
// Detection is deliberately conservative: inserting a wrong language is worse
// than inserting none, so Detect only names a language when a strong,
// language-specific signal identifies it (a shebang, a parse as JSON or
// nested YAML, or a statement that only one language uses). Everything else
// is reported as "text", and MD040 then reports the block without a fix.
package langdetect

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/go-enry/go-enry/v2"
	"gopkg.in/yaml.v3"
)

// Language identifiers returned by Detect.
const (
	langBash       = "bash"
	langC          = "c"
	langCPP        = "cpp"
	langDiff       = "diff"
	langDockerfile = "dockerfile"
	langGo         = "go"
	langHTML       = "html"
	langJavaScript = "javascript"
	langJSON       = "json"
	langPHP        = "php"
	langPython     = "python"
	langRust       = "rust"
	langSQL        = "sql"
	langText       = "text"
	langTypeScript = "typescript"
	langXML        = "xml"
	langYAML       = "yaml"
)

// minYAMLKeys is the number of top-level keys a YAML mapping needs before it
// counts as YAML rather than a few "Label: value" lines of prose.
const minYAMLKeys = 2

// minDockerInstructions is the number of instruction lines a Dockerfile needs.
const minDockerInstructions = 2

// detector returns a language, or "" when its signals are absent.
type detector func(content []byte, lines []string) string

// Detect returns the detected language for code content, or "text" when no
// strong signal identifies one.
func Detect(content []byte) string {
	if len(bytes.TrimSpace(content)) == 0 {
		return langText
	}

	if lang, safe := enry.GetLanguageByShebang(content); safe {
		return normalize(lang)
	}

	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")

	// Structural signals name one language from the start of the content or
	// from the content as a whole, so the first one found wins.
	for _, detect := range []detector{detectMarkupProlog, detectDiff, detectJSON, detectGo, detectDockerfile, detectSQL} {
		if lang := detect(content, lines); lang != "" {
			return lang
		}
	}

	// Statements of one language can sit in another's strings or comments.
	// Each of these detectors reads its own language's code with strings and
	// comments blanked out, and they must agree: Python with a JavaScript call
	// in a comment is not confidently either.
	found := ""
	for _, detect := range []detector{detectRust, detectCFamily, detectScript, detectPython} {
		lang := detect(content, lines)
		if lang == "" {
			continue
		}
		if found != "" && found != lang {
			return langText
		}
		found = lang
	}
	if found != "" {
		return found
	}

	// YAML comes last because many other formats also parse as YAML.
	if lang := detectYAML(content, lines); lang != "" {
		return lang
	}
	return langText
}

// codeSyntax says where a language keeps string literals and comments.
type codeSyntax struct {
	quotes       string   // quote characters of strings that end with their line
	multiline    string   // quote characters of strings that may span lines
	triple       bool     // Python's triple-quoted strings
	lineComments []string // prefixes of comments that run to the end of the line
	blockComment bool     // C-style /* ... */ comments
}

// codeView returns content with comments replaced by spaces and the inside
// of string literals reduced to letters, digits, and underscores (every
// other character becomes 'x'), so statement patterns match only code while
// a pattern that needs a string, such as a module name or "__main__", still
// finds one. Line breaks stay where they are.
func codeView(content []byte, syntax codeSyntax) []byte {
	view := bytes.Clone(content)
	for idx := 0; idx < len(view); {
		rest := view[idx:]
		switch {
		case syntax.blockComment && bytes.HasPrefix(rest, []byte("/*")):
			end := len(view)
			if closing := bytes.Index(rest[2:], []byte("*/")); closing >= 0 {
				end = idx + 2 + closing + 2
			}
			blankComment(view, idx, end)
			idx = end
		case hasAnyPrefix(string(rest[:min(2, len(rest))]), syntax.lineComments):
			end := len(view)
			if newline := bytes.IndexByte(rest, '\n'); newline >= 0 {
				end = idx + newline
			}
			blankComment(view, idx, end)
			idx = end
		case syntax.triple && (bytes.HasPrefix(rest, []byte(`"""`)) || bytes.HasPrefix(rest, []byte(`'''`))):
			idx = blankString(view, idx, string(rest[:3]), true)
		case strings.IndexByte(syntax.quotes, rest[0]) >= 0:
			idx = blankString(view, idx, string(rest[:1]), false)
		case strings.IndexByte(syntax.multiline, rest[0]) >= 0:
			idx = blankString(view, idx, string(rest[:1]), true)
		default:
			idx++
		}
	}
	return view
}

// blankComment replaces view[from:to] with spaces, keeping line breaks.
func blankComment(view []byte, from, to int) {
	for idx := from; idx < to; idx++ {
		if view[idx] != '\n' && view[idx] != '\r' {
			view[idx] = ' '
		}
	}
}

// blankString reduces the inside of the string literal that opens with quote
// at view[open] and returns the offset after it. A backslash escapes the
// next character. A string that may not span lines ends at the line break.
func blankString(view []byte, open int, quote string, multiline bool) int {
	idx := open + len(quote)
	for idx < len(view) {
		switch {
		case bytes.HasPrefix(view[idx:], []byte(quote)):
			return idx + len(quote)
		case view[idx] == '\n' && !multiline:
			return idx
		case view[idx] == '\\' && idx+1 < len(view) && view[idx+1] != '\n':
			view[idx], view[idx+1] = 'x', 'x'
			idx += 2
			continue
		case view[idx] != '\n' && view[idx] != '\r' && view[idx] != '_' && !isAlphanumeric(view[idx]):
			view[idx] = 'x'
		}
		idx++
	}
	return idx
}

func isAlphanumeric(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

// detectMarkupProlog recognizes documents that open with a PHP tag, an XML
// declaration, or an HTML doctype or root element.
func detectMarkupProlog(content []byte, _ []string) string {
	lower := bytes.ToLower(bytes.TrimSpace(content))
	switch {
	case bytes.HasPrefix(lower, []byte("<?php")):
		return langPHP
	case bytes.HasPrefix(lower, []byte("<?xml")):
		return langXML
	case bytes.HasPrefix(lower, []byte("<!doctype html")), bytes.HasPrefix(lower, []byte("<html")):
		return langHTML
	}
	return ""
}

var diffHunkPattern = regexp.MustCompile(`^@@ -\d+(,\d+)? \+\d+(,\d+)? @@`)

// detectDiff recognizes unified diffs by their git header, file headers, or first hunk.
func detectDiff(_ []byte, lines []string) string {
	first := firstSignificantLine(lines, nil)
	switch {
	case strings.HasPrefix(first, "diff --git "), diffHunkPattern.MatchString(first):
		return langDiff
	}
	for idx := 0; idx+1 < len(lines); idx++ {
		if strings.HasPrefix(lines[idx], "--- ") && strings.HasPrefix(lines[idx+1], "+++ ") {
			return langDiff
		}
	}
	return ""
}

// detectJSON recognizes content that parses as a JSON object or array.
func detectJSON(content []byte, _ []string) string {
	trimmed := bytes.TrimSpace(content)
	if (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
		return langJSON
	}
	return ""
}

var goPackagePattern = regexp.MustCompile(`^package [A-Za-z_][A-Za-z0-9_]*\s*$`)

// detectGo recognizes a Go package clause as the first statement. Java package
// declarations use dotted names and a semicolon, so they do not match.
func detectGo(_ []byte, lines []string) string {
	if goPackagePattern.MatchString(firstSignificantLine(lines, []string{"//"})) {
		return langGo
	}
	return ""
}

var dockerInstructionPattern = regexp.MustCompile(
	`^(FROM|RUN|CMD|LABEL|MAINTAINER|EXPOSE|ENV|ADD|COPY|ENTRYPOINT|VOLUME|USER|WORKDIR|ARG|ONBUILD|STOPSIGNAL|HEALTHCHECK|SHELL)\s+\S`)

// detectDockerfile recognizes instruction lines that start with FROM (or ARG
// before FROM) and contain at least one more instruction.
func detectDockerfile(_ []byte, lines []string) string {
	first := firstSignificantLine(lines, []string{"#"})
	if !strings.HasPrefix(first, "FROM ") && !strings.HasPrefix(first, "ARG ") {
		return ""
	}
	instructions := 0
	hasFrom := false
	for _, line := range lines {
		if dockerInstructionPattern.MatchString(line) {
			instructions++
			hasFrom = hasFrom || strings.HasPrefix(line, "FROM ")
		}
	}
	if hasFrom && instructions >= minDockerInstructions {
		return langDockerfile
	}
	return ""
}

// sqlStatementPattern matches a SQL statement written with uppercase keywords
// at the start of the text, which keeps sentences such as "Select the files
// from the list" out.
var sqlStatementPattern = regexp.MustCompile(`^(?:` +
	`(?s:SELECT\s.+?\sFROM\s+\S)|SELECT\s[^\n]*;\s*$|INSERT\s+INTO\s+\S|UPDATE\s+\S+\s+SET\s|DELETE\s+FROM\s+\S|` +
	`(?:CREATE|ALTER|DROP)\s+(?:TABLE|INDEX|VIEW|DATABASE|SCHEMA)\s|WITH\s+\w+\s+AS\s*\()`)

// detectSQL recognizes content whose first statement is an uppercase SQL statement.
func detectSQL(content []byte, lines []string) string {
	first := firstSignificantLine(lines, []string{"--"})
	start := strings.Index(string(content), first)
	if first == "" || start < 0 {
		return ""
	}
	if sqlStatementPattern.Match(content[start:]) {
		return langSQL
	}
	return ""
}

// rustPattern matches syntax that only Rust uses: function items, macros,
// mutable bindings, and crate paths.
var rustPattern = regexp.MustCompile(`(?m)` +
	`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?fn\s+\w+\s*(?:<[^>]*>)?\s*\(|` +
	`\b(?:println|eprintln|format|vec|panic)!\s*[(\[]|` +
	`^\s*let\s+mut\s+\w+|` +
	`^\s*use\s+(?:std|crate|super|self)::`)

// detectRust recognizes Rust function items, macros, mutable bindings, and
// paths. Single quotes start char literals and lifetimes, not strings.
func detectRust(content []byte, _ []string) string {
	if rustPattern.Match(codeView(content, codeSyntax{quotes: `"`, lineComments: []string{"//"}, blockComment: true})) {
		return langRust
	}
	return ""
}

var (
	cIncludePattern = regexp.MustCompile(`(?m)^\s*#\s*include\s*[<"][^>"]+[>"]`)
	cppPattern      = regexp.MustCompile(
		`std::|#\s*include\s*<(iostream|vector|string|map|memory|algorithm)>|(?m)^\s*(class|namespace|template\s*<)\b`)
)

// detectCFamily recognizes C and C++ by their #include directives.
func detectCFamily(content []byte, _ []string) string {
	code := codeView(content, codeSyntax{quotes: `"'`, lineComments: []string{"//"}, blockComment: true})
	if !cIncludePattern.Match(code) {
		return ""
	}
	if cppPattern.Match(code) {
		return langCPP
	}
	return langC
}

// javaScriptPattern matches ECMAScript module syntax, CommonJS, console
// calls, arrow functions bound with const or let, and function declarations.
var javaScriptPattern = regexp.MustCompile(`(?m)` +
	`^\s*import\s+[^'"\n]+\s+from\s+['"][^'"\n]+['"]\s*;?\s*$|` +
	`^\s*import\s+['"][^'"\n]+['"]\s*;?\s*$|` +
	`^\s*export\s+(?:default|const|let|function|class|async)\s|` +
	`\brequire\(\s*['"][^'"\n]+['"]\s*\)|` +
	`\bconsole\.(?:log|error|warn|info|debug)\([^)\n]*\)|` +
	`^\s*(?:const|let)\s+\w+\s*=\s*(?:async\s*)?(?:\([^)]*\)|\w+)\s*=>|` +
	`^\s*(?:async\s+)?function\*?\s+\w+\s*\([^)$]*\)\s*\{`)

// typeScriptPattern matches TypeScript-only declarations and annotations.
var typeScriptPattern = regexp.MustCompile(`(?m)` +
	`^\s*(?:export\s+)?(?:declare\s+)?(?:interface|enum)\s+\w+[^\n]*\{|` +
	`^\s*(?:export\s+)?type\s+\w+(?:<[^>]*>)?\s*=|` +
	`\b(?:const|let|var)\s+\w+\s*:\s*(?:string|number|boolean|any|unknown|void|never|object|bigint|symbol)\b|` +
	`\)\s*:\s*(?:string|number|boolean|void|Promise<)|` +
	`[({]\s*\w+\??\s*:\s*(?:string|number|boolean)\s*[;,)}]`)

// detectScript recognizes TypeScript by its type syntax, and JavaScript otherwise.
func detectScript(content []byte, _ []string) string {
	code := codeView(content, codeSyntax{quotes: `"'`, multiline: "`", lineComments: []string{"//"}, blockComment: true})
	switch {
	case typeScriptPattern.Match(code):
		return langTypeScript
	case javaScriptPattern.Match(code):
		return langJavaScript
	}
	return ""
}

// pythonPattern matches Python definitions, imports, and the main guard.
var pythonPattern = regexp.MustCompile(`(?m)` +
	`^\s*(?:async\s+)?def\s+\w+\s*\(.*\)\s*(?:->\s*[^:\n]+)?:\s*(?:#.*)?$|` +
	`^\s*class\s+\w+\s*(?:\([^)\n]*\))?\s*:\s*(?:#.*)?$|` +
	`^from\s+[\w.]+\s+import\s+[\w*, ()]+\s*$|` +
	`^import\s+[\w.]+(?:\s+as\s+\w+)?(?:\s*,\s*[\w.]+(?:\s+as\s+\w+)?)*\s*$|` +
	`^if\s+__name__\s*==\s*['"]__main__['"]\s*:`)

// detectPython recognizes Python statements that no other common language shares.
func detectPython(content []byte, _ []string) string {
	if pythonPattern.Match(codeView(content, codeSyntax{quotes: `"'`, triple: true, lineComments: []string{"#"}})) {
		return langPython
	}
	return ""
}

var yamlKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// detectYAML recognizes a block-style YAML mapping with identifier keys and
// at least one nested mapping or sequence. Flat "Label: value" lines are left
// alone because prose often looks like that.
func detectYAML(content []byte, _ []string) string {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil || len(root.Content) == 0 {
		return ""
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode || mapping.Style&yaml.FlowStyle != 0 || len(mapping.Content) < 2*minYAMLKeys {
		return ""
	}
	nested := false
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		key, value := mapping.Content[idx], mapping.Content[idx+1]
		if key.Kind != yaml.ScalarNode || !yamlKeyPattern.MatchString(key.Value) {
			return ""
		}
		if value.Kind == yaml.MappingNode || value.Kind == yaml.SequenceNode {
			nested = true
		}
	}
	if nested {
		return langYAML
	}
	return ""
}

// firstSignificantLine returns the first line that is not blank and does not
// start with one of the comment prefixes.
func firstSignificantLine(lines, commentPrefixes []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || hasAnyPrefix(trimmed, commentPrefixes) {
			continue
		}
		return trimmed
	}
	return ""
}

func hasAnyPrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// normalize converts go-enry language names to fence tags. An info string
// ends its first word at a space, so a name with spaces, such as "Common
// Lisp", becomes its first alias without one, or the name with hyphens.
func normalize(lang string) string {
	if lang == "Shell" {
		return langBash
	}
	name := strings.ToLower(lang)
	if !strings.ContainsAny(name, " \t") {
		return name
	}
	if info, err := enry.GetLanguageInfo(lang); err == nil {
		for _, alias := range info.Aliases {
			if alias != "" && !strings.ContainsAny(alias, " \t") {
				return alias
			}
		}
	}
	return strings.Join(strings.Fields(name), "-")
}
