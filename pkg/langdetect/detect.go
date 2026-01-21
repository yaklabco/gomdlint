// Package langdetect provides language detection for code content.
// It uses go-enry to detect programming languages from code snippets,
// primarily for auto-detecting language identifiers in code blocks.
package langdetect

import (
	"bytes"
	"strings"

	"github.com/go-enry/go-enry/v2"
)

// Language constants for common detected languages.
const (
	langGo         = "go"
	langPython     = "python"
	langJavaScript = "javascript"
	langJSON       = "json"
	langYAML       = "yaml"
	langHTML       = "html"
	langSQL        = "sql"
	langRust       = "rust"
	langDockerfile = "dockerfile"
	langText       = "text"
	langBash       = "bash"
)

// Detect returns the detected language for code content.
// Returns "text" if detection fails or confidence is low.
func Detect(content []byte) string {
	if len(content) == 0 {
		return langText
	}

	// Strategy 1: Check shebang first (most reliable).
	if lang, safe := enry.GetLanguageByShebang(content); safe {
		return normalize(lang)
	}

	// Strategy 2: Check for language-specific patterns before using classifier.
	if lang := detectByPattern(content); lang != "" {
		return lang
	}

	// Strategy 3: Use classifier with common language candidates.
	candidates := []string{
		"Go", "Python", "Shell", "JavaScript", "TypeScript",
		"Ruby", "Rust", "Java", "C", "C++", "SQL", "JSON",
		"YAML", "HTML", "CSS", "Markdown", "Dockerfile",
	}

	// Use classifier - it returns the most probable language.
	// Only use the result if confidence is high (safe == true).
	if lang, safe := enry.GetLanguageByClassifier(content, candidates); safe && lang != "" {
		return normalize(lang)
	}

	return langText
}

// detectByPattern checks for language-specific patterns that are highly indicative.
func detectByPattern(content []byte) string {
	contentStr := string(content)
	trimmed := bytes.TrimSpace(content)

	// Check patterns in order of specificity.
	if lang := detectGo(trimmed); lang != "" {
		return lang
	}
	if lang := detectPython(contentStr); lang != "" {
		return lang
	}
	if lang := detectHTML(trimmed); lang != "" {
		return lang
	}
	if lang := detectJSON(trimmed); lang != "" {
		return lang
	}
	if lang := detectDockerfile(content, trimmed); lang != "" {
		return lang
	}
	if lang := detectSQL(contentStr); lang != "" {
		return lang
	}
	if lang := detectRust(contentStr); lang != "" {
		return lang
	}
	if lang := detectJavaScript(contentStr); lang != "" {
		return lang
	}
	if lang := detectYAML(content); lang != "" {
		return lang
	}

	return ""
}

// detectGo checks for Go language patterns.
func detectGo(trimmed []byte) string {
	if bytes.HasPrefix(trimmed, []byte("package ")) {
		return langGo
	}
	return ""
}

// detectPython checks for Python language patterns.
func detectPython(contentStr string) string {
	// def/class definitions with colon.
	if strings.Contains(contentStr, "def ") && strings.Contains(contentStr, "):") {
		return langPython
	}
	// Python import statements (not Go which uses "import (").
	if strings.Contains(contentStr, "import ") && !strings.Contains(contentStr, "import (") {
		if strings.Contains(contentStr, "from ") || strings.HasPrefix(strings.TrimSpace(contentStr), "import ") {
			return langPython
		}
	}
	// Python dunder variables.
	if strings.Contains(contentStr, "__name__") || strings.Contains(contentStr, "__main__") {
		return langPython
	}
	return ""
}

// detectHTML checks for HTML language patterns.
func detectHTML(trimmed []byte) string {
	lowerTrimmed := bytes.ToLower(trimmed)
	if bytes.Contains(lowerTrimmed, []byte("<!doctype html")) ||
		bytes.Contains(lowerTrimmed, []byte("<html")) ||
		bytes.Contains(lowerTrimmed, []byte("<head>")) ||
		bytes.Contains(lowerTrimmed, []byte("<body>")) {
		return langHTML
	}
	return ""
}

// detectJSON checks for JSON patterns.
func detectJSON(trimmed []byte) string {
	if (bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))) &&
		bytes.Contains(trimmed, []byte(`"`)) {
		return langJSON
	}
	return ""
}

// detectDockerfile checks for Dockerfile patterns.
func detectDockerfile(content, trimmed []byte) string {
	if bytes.HasPrefix(trimmed, []byte("FROM ")) ||
		(bytes.Contains(content, []byte("\nFROM ")) && bytes.Contains(content, []byte("\nRUN "))) ||
		(bytes.Contains(content, []byte("WORKDIR ")) && bytes.Contains(content, []byte("COPY "))) {
		return langDockerfile
	}
	return ""
}

// detectSQL checks for SQL patterns.
func detectSQL(contentStr string) string {
	upper := strings.ToUpper(contentStr)
	trimmedUpper := strings.TrimSpace(upper)
	if strings.HasPrefix(trimmedUpper, "SELECT ") ||
		strings.HasPrefix(trimmedUpper, "INSERT ") ||
		strings.HasPrefix(trimmedUpper, "UPDATE ") ||
		strings.HasPrefix(trimmedUpper, "DELETE ") ||
		strings.HasPrefix(trimmedUpper, "CREATE ") {
		return langSQL
	}
	return ""
}

// detectRust checks for Rust language patterns.
func detectRust(contentStr string) string {
	if strings.Contains(contentStr, "fn main()") ||
		strings.Contains(contentStr, "println!") ||
		strings.Contains(contentStr, "let mut ") {
		return langRust
	}
	return ""
}

// detectJavaScript checks for JavaScript patterns.
func detectJavaScript(contentStr string) string {
	if strings.Contains(contentStr, "=>") ||
		strings.Contains(contentStr, "const ") ||
		strings.Contains(contentStr, "let ") ||
		strings.Contains(contentStr, "console.log") {
		return langJavaScript
	}
	return ""
}

// detectYAML checks for YAML patterns by counting key: value pairs.
func detectYAML(content []byte) string {
	lines := bytes.Split(content, []byte("\n"))
	yamlKeyCount := 0

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.HasPrefix(line, []byte("#")) {
			continue
		}
		// Simple key: value (identifier followed by colon and space).
		// Exclude lines that look like code (contain parentheses, brackets).
		if bytes.Contains(line, []byte(": ")) {
			if !bytes.Contains(line, []byte("(")) &&
				!bytes.Contains(line, []byte("{")) &&
				!bytes.HasPrefix(line, []byte(`"`)) {
				yamlKeyCount++
			}
		}
		// YAML list item at root level.
		if bytes.HasPrefix(line, []byte("- ")) {
			yamlKeyCount++
		}
	}

	if yamlKeyCount >= 2 {
		return langYAML
	}
	return ""
}

// normalize converts go-enry language names to fence tags.
func normalize(lang string) string {
	if lang == "Shell" {
		return langBash
	}
	return strings.ToLower(lang)
}
