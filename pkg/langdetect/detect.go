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

// detectors run in order; the first confident answer wins. YAML runs last
// because many other formats also parse as YAML.
var detectors = []detector{
	detectMarkupProlog,
	detectDiff,
	detectJSON,
	detectGo,
	detectDockerfile,
	detectSQL,
	detectRust,
	detectCFamily,
	detectScript,
	detectPython,
	detectYAML,
}

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
	for _, detect := range detectors {
		if lang := detect(content, lines); lang != "" {
			return lang
		}
	}
	return langText
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

// sqlStatementPatterns match SQL statements written with uppercase keywords,
// which keeps sentences such as "Select the files from the list" out.
var sqlStatementPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)^SELECT\s.+?\sFROM\s+\S`),
	regexp.MustCompile(`^SELECT\s[^\n]*;\s*$`),
	regexp.MustCompile(`^INSERT\s+INTO\s+\S`),
	regexp.MustCompile(`^UPDATE\s+\S+\s+SET\s`),
	regexp.MustCompile(`^DELETE\s+FROM\s+\S`),
	regexp.MustCompile(`^(CREATE|ALTER|DROP)\s+(TABLE|INDEX|VIEW|DATABASE|SCHEMA)\s`),
	regexp.MustCompile(`^WITH\s+\w+\s+AS\s*\(`),
}

// detectSQL recognizes content whose first statement is an uppercase SQL statement.
func detectSQL(content []byte, lines []string) string {
	first := firstSignificantLine(lines, []string{"--"})
	start := strings.Index(string(content), first)
	if first == "" || start < 0 {
		return ""
	}
	statement := string(content[start:])
	for _, pattern := range sqlStatementPatterns {
		if pattern.MatchString(statement) {
			return langSQL
		}
	}
	return ""
}

// rustPatterns match syntax that only Rust uses.
var rustPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*(pub(\([^)]*\))?\s+)?(async\s+)?(unsafe\s+)?fn\s+\w+\s*(<[^>]*>)?\s*\(`),
	regexp.MustCompile(`\b(println|eprintln|format|vec|panic)!\s*[(\[]`),
	regexp.MustCompile(`(?m)^\s*let\s+mut\s+\w+`),
	regexp.MustCompile(`(?m)^\s*use\s+(std|crate|super|self)::`),
}

// detectRust recognizes Rust function items, macros, mutable bindings, and paths.
func detectRust(content []byte, _ []string) string {
	if matchesAny(content, rustPatterns) {
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
	if !cIncludePattern.Match(content) {
		return ""
	}
	if cppPattern.Match(content) {
		return langCPP
	}
	return langC
}

// javaScriptPatterns match ECMAScript module syntax, CommonJS, console calls,
// arrow functions bound with const or let, and function declarations.
var javaScriptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*import\s+[^'"\n]+\s+from\s+['"][^'"\n]+['"]\s*;?\s*$`),
	regexp.MustCompile(`(?m)^\s*import\s+['"][^'"\n]+['"]\s*;?\s*$`),
	regexp.MustCompile(`(?m)^\s*export\s+(default|const|let|function|class|async)\s`),
	regexp.MustCompile(`\brequire\(\s*['"][^'"\n]+['"]\s*\)`),
	regexp.MustCompile(`\bconsole\.(log|error|warn|info|debug)\([^)\n]*\)`),
	regexp.MustCompile(`(?m)^\s*(const|let)\s+\w+\s*=\s*(async\s*)?(\([^)]*\)|\w+)\s*=>`),
	regexp.MustCompile(`(?m)^\s*(async\s+)?function\*?\s+\w+\s*\([^)$]*\)\s*\{`),
}

// typeScriptPatterns match TypeScript-only declarations and annotations.
var typeScriptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*(export\s+)?(declare\s+)?(interface|enum)\s+\w+[^\n]*\{`),
	regexp.MustCompile(`(?m)^\s*(export\s+)?type\s+\w+(<[^>]*>)?\s*=`),
	regexp.MustCompile(`\b(const|let|var)\s+\w+\s*:\s*(string|number|boolean|any|unknown|void|never|object|bigint|symbol)\b`),
	regexp.MustCompile(`\)\s*:\s*(string|number|boolean|void|Promise<)`),
	regexp.MustCompile(`[({]\s*\w+\??\s*:\s*(string|number|boolean)\s*[;,)}]`),
}

// detectScript recognizes TypeScript by its type syntax, and JavaScript otherwise.
func detectScript(content []byte, _ []string) string {
	switch {
	case matchesAny(content, typeScriptPatterns):
		return langTypeScript
	case matchesAny(content, javaScriptPatterns):
		return langJavaScript
	}
	return ""
}

// pythonPatterns match Python definitions, imports, and the main guard.
var pythonPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*(async\s+)?def\s+\w+\s*\(.*\)\s*(->\s*[^:\n]+)?:\s*(#.*)?$`),
	regexp.MustCompile(`(?m)^\s*class\s+\w+\s*(\([^)\n]*\))?\s*:\s*(#.*)?$`),
	regexp.MustCompile(`(?m)^from\s+[\w.]+\s+import\s+[\w*, ()]+\s*$`),
	regexp.MustCompile(`(?m)^import\s+[\w.]+(\s+as\s+\w+)?(\s*,\s*[\w.]+(\s+as\s+\w+)?)*\s*$`),
	regexp.MustCompile(`(?m)^if\s+__name__\s*==\s*['"]__main__['"]\s*:`),
}

// detectPython recognizes Python statements that no other common language shares.
func detectPython(content []byte, _ []string) string {
	if matchesAny(content, pythonPatterns) {
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

func matchesAny(content []byte, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.Match(content) {
			return true
		}
	}
	return false
}

// normalize converts go-enry language names to fence tags.
func normalize(lang string) string {
	if lang == "Shell" {
		return langBash
	}
	return strings.ToLower(lang)
}
