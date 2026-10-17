// Package langdetect names the language of a code block for MD040, which
// inserts it into a fenced code block that has none.
//
// A wrong name is worse than none, so Detect only names a language when the
// whole block says which one it is, in a way that strings and comments
// cannot fake: a shebang on the first line, content that parses completely
// as JSON or as an XML document, a PHP or HTML document opening, a complete
// unified diff, or a Dockerfile whose first instruction is FROM. Statements
// of a language never count, because they can appear inside another
// language's strings and comments. Everything else is "text", and MD040
// then reports the block without a fix.
package langdetect

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-enry/go-enry/v2"
)

// Language identifiers returned by Detect.
const (
	langBash       = "bash"
	langDiff       = "diff"
	langDockerfile = "dockerfile"
	langHTML       = "html"
	langJSON       = "json"
	langPHP        = "php"
	langText       = "text"
	langXML        = "xml"
)

// minDockerInstructions is the number of instructions a Dockerfile needs, so
// a single line that starts with FROM is not taken for one.
const minDockerInstructions = 2

// Detect returns the language of a code block, or "text" when the whole
// block does not name one beyond doubt.
func Detect(content []byte) string {
	if len(bytes.TrimSpace(content)) == 0 {
		return langText
	}
	if bytes.HasPrefix(content, []byte("#!")) {
		if lang, safe := enry.GetLanguageByShebang(content); safe {
			return normalize(lang)
		}
		return langText
	}
	trimmed := bytes.TrimSpace(content)
	lower := bytes.ToLower(trimmed)
	lines := blockLines(content)
	switch {
	case (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed):
		return langJSON
	case bytes.HasPrefix(trimmed, []byte("<?xml")) && isXMLDocument(trimmed):
		return langXML
	case bytes.HasPrefix(lower, []byte("<?php")):
		return langPHP
	case bytes.HasPrefix(lower, []byte("<!doctype html")):
		return langHTML
	case isUnifiedDiff(lines):
		return langDiff
	case isDockerfile(lines):
		return langDockerfile
	}
	return langText
}

// blockLines splits content into lines without line endings, dropping the
// empty lines at the end.
func blockLines(content []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// isXMLDocument reports whether content is one well-formed XML document: a
// single root element with nothing but markup around it.
func isXMLDocument(content []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	depth, roots := 0, 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return roots == 1 && depth == 0
		}
		if err != nil {
			return false
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(token)) > 0 {
				return false
			}
		}
	}
}

var (
	// diffHunkHeader matches "@@ -start[,count] +start[,count] @@".
	diffHunkHeader = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@`)
	// gitHeaderLine matches the extended header lines git writes before a
	// file's "---" and "+++" lines.
	gitHeaderLine = regexp.MustCompile(`^(?:diff --git |index |(?:new|deleted) file mode |(?:old|new) mode |` +
		`similarity index |dissimilarity index |rename (?:from|to) |copy (?:from|to) )`)
)

// isUnifiedDiff reports whether lines are complete unified diffs and nothing
// else: one or more file sections, each with optional git header lines, a
// "---" line, a "+++" line, and hunks whose lines match the counts in their
// headers.
func isUnifiedDiff(lines []string) bool {
	idx, sections := 0, 0
	for idx < len(lines) {
		for idx < len(lines) && gitHeaderLine.MatchString(lines[idx]) {
			idx++
		}
		if idx+1 >= len(lines) || !strings.HasPrefix(lines[idx], "--- ") || !strings.HasPrefix(lines[idx+1], "+++ ") {
			return false
		}
		idx += 2
		hunks := 0
		for idx < len(lines) {
			header := diffHunkHeader.FindStringSubmatch(lines[idx])
			if header == nil {
				break
			}
			next, ok := hunkEnd(lines, idx+1, hunkCount(header[1]), hunkCount(header[2]))
			if !ok {
				return false
			}
			idx = next
			hunks++
		}
		if hunks == 0 {
			return false
		}
		sections++
	}
	return sections > 0
}

// hunkCount reads the line count of a hunk header range, which is 1 when
// the header leaves it out.
func hunkCount(text string) int {
	if text == "" {
		return 1
	}
	count, err := strconv.Atoi(text)
	if err != nil {
		return -1
	}
	return count
}

// hunkEnd consumes the lines of a hunk starting at lines[idx] until the old
// and new line counts are used up, and returns the index after the hunk. A
// context line (a space, or nothing left of it) counts for both sides, a
// "-" line for the old side, a "+" line for the new side, and a "\" line
// ("\ No newline at end of file") for neither.
func hunkEnd(lines []string, idx, oldCount, newCount int) (int, bool) {
	if oldCount < 0 || newCount < 0 {
		return idx, false
	}
	for oldCount > 0 || newCount > 0 {
		if idx >= len(lines) {
			return idx, false
		}
		line := lines[idx]
		switch {
		case line == "" || line[0] == ' ':
			oldCount--
			newCount--
		case line[0] == '-':
			oldCount--
		case line[0] == '+':
			newCount--
		case line[0] == '\\':
		default:
			return idx, false
		}
		if oldCount < 0 || newCount < 0 {
			return idx, false
		}
		idx++
	}
	for idx < len(lines) && strings.HasPrefix(lines[idx], `\`) {
		idx++
	}
	return idx, true
}

var (
	// dockerFrom matches a FROM instruction: an optional platform, an image
	// reference, and an optional stage name after AS, in either case.
	dockerFrom = regexp.MustCompile(`^FROM\s+(?:--platform=\S+\s+)?[\w.\-/:@${}]+(?:\s+(?i:as)\s+[\w.\-]+)?\s*$`)
	// dockerInstruction matches the start of any other instruction.
	dockerInstruction = regexp.MustCompile(
		`^(?:FROM|RUN|CMD|LABEL|MAINTAINER|EXPOSE|ENV|ADD|COPY|ENTRYPOINT|VOLUME|USER|WORKDIR|ARG|ONBUILD|STOPSIGNAL|HEALTHCHECK|SHELL)(?:\s|$)`)
)

// isDockerfile reports whether every line is a Dockerfile instruction, a
// continuation of the one before, a comment, or blank, the first
// instruction is FROM, and there are at least two instructions.
func isDockerfile(lines []string) bool {
	instructions := 0
	continued := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if continued {
			continued = strings.HasSuffix(trimmed, `\`)
			continue
		}
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			continue
		case instructions == 0:
			if !dockerFrom.MatchString(trimmed) {
				return false
			}
		case !dockerInstruction.MatchString(trimmed):
			return false
		}
		instructions++
		continued = strings.HasSuffix(trimmed, `\`)
	}
	return instructions >= minDockerInstructions
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
