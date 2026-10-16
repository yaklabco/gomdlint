package reporter_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sarifSchemaPath is the OASIS SARIF 2.1.0 (errata 01) JSON schema, vendored
// so validation runs offline. Source:
// https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json
// (sha256 c3b4bb2d6093897483348925aaa73af03b3e3f4bd4ca38cef26dcb4212a2682e).
const sarifSchemaPath = "testdata/sarif-schema-2.1.0.json"

// schemaKeywords lists the JSON Schema draft-04 keywords the validator
// understands. Annotation keywords are accepted and ignored; any other keyword
// is reported, so a constraint can never be skipped silently.
var schemaKeywords = map[string]bool{
	"$schema": true, "id": true, "title": true, "description": true, "default": true, "definitions": true,
	"$ref": true, "type": true, "enum": true, "anyOf": true, "oneOf": true,
	"properties": true, "additionalProperties": true, "required": true,
	"items": true, "minItems": true, "uniqueItems": true,
	"minimum": true, "maximum": true, "pattern": true, "format": true,
}

// schemaValidator checks decoded JSON documents against a draft-04 schema,
// implementing exactly the keywords the SARIF schema uses.
type schemaValidator struct {
	root     map[string]any
	patterns map[string]*regexp.Regexp
}

// loadSARIFValidator returns a validator for the vendored SARIF schema.
func loadSARIFValidator(t *testing.T) *schemaValidator {
	t.Helper()

	data, err := os.ReadFile(sarifSchemaPath)
	require.NoError(t, err)

	root, ok := decodeJSON(t, data).(map[string]any)
	require.True(t, ok, "schema root must be an object")

	validator := &schemaValidator{root: root, patterns: make(map[string]*regexp.Regexp)}
	collectPatterns(t, root, validator.patterns)

	return validator
}

// collectPatterns compiles every pattern in the schema up front, so a pattern
// Go cannot compile fails loudly instead of being skipped.
func collectPatterns(t *testing.T, node any, patterns map[string]*regexp.Regexp) {
	t.Helper()

	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if pattern, ok := value.(string); ok && key == "pattern" {
				compiled, err := regexp.Compile(pattern)
				require.NoError(t, err, "schema pattern %q", pattern)
				patterns[pattern] = compiled
				continue
			}
			collectPatterns(t, value, patterns)
		}
	case []any:
		for _, value := range typed {
			collectPatterns(t, value, patterns)
		}
	}
}

// decodeJSON decodes data keeping numbers as json.Number, so integers and
// numbers can be told apart.
func decodeJSON(t *testing.T, data []byte) any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	require.NoError(t, decoder.Decode(&value))

	return value
}

// validate returns one message per violation, each starting with the JSON
// pointer of the offending value.
func (v *schemaValidator) validate(doc any) []string {
	var violations []string
	v.check(v.root, doc, "", &violations)
	return violations
}

func (v *schemaValidator) check(schema map[string]any, value any, pointer string, violations *[]string) {
	for keyword := range schema {
		if !schemaKeywords[keyword] {
			*violations = append(*violations, fmt.Sprintf("%s: unsupported schema keyword %q", pointer, keyword))
		}
	}

	// In draft-04, $ref replaces the schema it appears in.
	if ref, ok := schema["$ref"]; ok {
		target, err := v.resolve(ref)
		if err != nil {
			*violations = append(*violations, fmt.Sprintf("%s: %v", pointer, err))
			return
		}
		v.check(target, value, pointer, violations)
		return
	}

	v.checkType(schema, value, pointer, violations)
	v.checkEnum(schema, value, pointer, violations)
	v.checkCombinators(schema, value, pointer, violations)

	switch typed := value.(type) {
	case map[string]any:
		v.checkObject(schema, typed, pointer, violations)
	case []any:
		v.checkArray(schema, typed, pointer, violations)
	case string:
		v.checkString(schema, typed, pointer, violations)
	case json.Number:
		v.checkNumber(schema, typed, pointer, violations)
	}
}

func (v *schemaValidator) resolve(ref any) (map[string]any, error) {
	refString, ok := ref.(string)
	if !ok || !strings.HasPrefix(refString, "#/definitions/") {
		return nil, fmt.Errorf("unsupported $ref %v", ref)
	}

	definitions, ok := v.root["definitions"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema has no definitions for %s", refString)
	}

	target, ok := definitions[strings.TrimPrefix(refString, "#/definitions/")].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved $ref %s", refString)
	}

	return target, nil
}

func (v *schemaValidator) checkType(schema map[string]any, value any, pointer string, violations *[]string) {
	declared, ok := schema["type"]
	if !ok {
		return
	}

	var names []string
	switch typed := declared.(type) {
	case string:
		names = []string{typed}
	case []any:
		for _, entry := range typed {
			name, isString := entry.(string)
			if !isString {
				*violations = append(*violations, fmt.Sprintf("%s: malformed type %v", pointer, declared))
				return
			}
			names = append(names, name)
		}
	default:
		*violations = append(*violations, fmt.Sprintf("%s: malformed type %v", pointer, declared))
		return
	}

	for _, name := range names {
		if typeMatches(name, value) {
			return
		}
	}
	*violations = append(*violations, fmt.Sprintf("%s: %v is not of type %v", pointer, value, names))
}

func typeMatches(name string, value any) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseInt(number.String(), 10, 64)
		return err == nil
	default:
		return false
	}
}

func (v *schemaValidator) checkEnum(schema map[string]any, value any, pointer string, violations *[]string) {
	allowed, ok := schema["enum"].([]any)
	if !ok {
		return
	}

	for _, candidate := range allowed {
		if reflect.DeepEqual(candidate, value) {
			return
		}
	}
	*violations = append(*violations, fmt.Sprintf("%s: %v is not one of %v", pointer, value, allowed))
}

func (v *schemaValidator) checkCombinators(schema map[string]any, value any, pointer string, violations *[]string) {
	if options, ok := schema["anyOf"].([]any); ok {
		if v.countMatches(options, value, pointer) == 0 {
			*violations = append(*violations, pointer+": matches none of anyOf")
		}
	}

	if options, ok := schema["oneOf"].([]any); ok {
		if matches := v.countMatches(options, value, pointer); matches != 1 {
			*violations = append(*violations, fmt.Sprintf("%s: matches %d of oneOf, want exactly 1", pointer, matches))
		}
	}
}

func (v *schemaValidator) countMatches(options []any, value any, pointer string) int {
	matches := 0
	for _, option := range options {
		subschema, ok := option.(map[string]any)
		if !ok {
			continue
		}
		var optionViolations []string
		v.check(subschema, value, pointer, &optionViolations)
		if len(optionViolations) == 0 {
			matches++
		}
	}
	return matches
}

func (v *schemaValidator) checkObject(schema map[string]any, object map[string]any, pointer string, violations *[]string) {
	var properties map[string]any
	if declared, ok := schema["properties"].(map[string]any); ok {
		properties = declared
	}

	if required, ok := schema["required"].([]any); ok {
		for _, name := range required {
			key, isString := name.(string)
			if !isString {
				continue
			}
			if _, present := object[key]; !present {
				*violations = append(*violations, fmt.Sprintf("%s: missing required property %q", pointer, key))
			}
		}
	}

	for key, member := range object {
		memberPointer := pointer + "/" + key
		if subschema, ok := properties[key].(map[string]any); ok {
			v.check(subschema, member, memberPointer, violations)
			continue
		}

		switch additional := schema["additionalProperties"].(type) {
		case bool:
			if !additional {
				*violations = append(*violations, fmt.Sprintf("%s: property %q is not allowed", pointer, key))
			}
		case map[string]any:
			v.check(additional, member, memberPointer, violations)
		}
	}
}

func (v *schemaValidator) checkArray(schema map[string]any, array []any, pointer string, violations *[]string) {
	if minItems, ok := schema["minItems"].(json.Number); ok {
		if limit, err := minItems.Int64(); err == nil && int64(len(array)) < limit {
			*violations = append(*violations, fmt.Sprintf("%s: %d items, want at least %d", pointer, len(array), limit))
		}
	}

	if unique, ok := schema["uniqueItems"].(bool); ok && unique {
		for first := range array {
			for second := first + 1; second < len(array); second++ {
				if reflect.DeepEqual(array[first], array[second]) {
					*violations = append(*violations, fmt.Sprintf("%s: items %d and %d are equal", pointer, first, second))
				}
			}
		}
	}

	if items, ok := schema["items"].(map[string]any); ok {
		for idx, item := range array {
			v.check(items, item, fmt.Sprintf("%s/%d", pointer, idx), violations)
		}
	}
}

func (v *schemaValidator) checkString(schema map[string]any, text string, pointer string, violations *[]string) {
	if pattern, ok := schema["pattern"].(string); ok {
		if compiled := v.patterns[pattern]; compiled == nil || !compiled.MatchString(text) {
			*violations = append(*violations, fmt.Sprintf("%s: %q does not match pattern %q", pointer, text, pattern))
		}
	}

	format, ok := schema["format"].(string)
	if !ok {
		return
	}

	var valid bool
	switch format {
	case "uri":
		valid = isAbsoluteURI(text)
	case "uri-reference":
		valid = isURIReference(text)
	case "date-time":
		_, err := time.Parse(time.RFC3339Nano, text)
		valid = err == nil
	default:
		*violations = append(*violations, fmt.Sprintf("%s: unsupported format %q", pointer, format))
		return
	}

	if !valid {
		*violations = append(*violations, fmt.Sprintf("%s: %q is not a valid %s", pointer, text, format))
	}
}

func (v *schemaValidator) checkNumber(schema map[string]any, number json.Number, pointer string, violations *[]string) {
	value, err := number.Float64()
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("%s: malformed number %s", pointer, number))
		return
	}

	if minimum, ok := schema["minimum"].(json.Number); ok {
		if limit, limitErr := minimum.Float64(); limitErr == nil && value < limit {
			*violations = append(*violations, fmt.Sprintf("%s: %s is below minimum %s", pointer, number, minimum))
		}
	}

	if maximum, ok := schema["maximum"].(json.Number); ok {
		if limit, limitErr := maximum.Float64(); limitErr == nil && value > limit {
			*violations = append(*violations, fmt.Sprintf("%s: %s is above maximum %s", pointer, number, maximum))
		}
	}
}

// isURIReference reports whether text is an RFC 3986 URI reference: it uses
// only characters a URI may contain, percent-encodings are well formed, and
// it parses.
func isURIReference(text string) bool {
	if !hasOnlyURICharacters(text) {
		return false
	}
	_, err := url.Parse(text)
	return err == nil
}

// isAbsoluteURI reports whether text is an RFC 3986 URI with a scheme.
func isAbsoluteURI(text string) bool {
	if !hasOnlyURICharacters(text) {
		return false
	}
	parsed, err := url.Parse(text)
	return err == nil && parsed.Scheme != ""
}

func hasOnlyURICharacters(text string) bool {
	const allowed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~:/?#[]@!$&'()*+,;="

	for idx := 0; idx < len(text); idx++ {
		char := text[idx]
		if char == '%' {
			if idx+2 >= len(text) || !isHexDigit(text[idx+1]) || !isHexDigit(text[idx+2]) {
				return false
			}
			idx += 2
			continue
		}
		if !strings.ContainsRune(allowed, rune(char)) {
			return false
		}
	}

	return true
}

func isHexDigit(char byte) bool {
	return strings.ContainsRune("0123456789abcdefABCDEF", rune(char))
}

// minimalSARIF is a small valid SARIF log that the validator self-tests break
// in one place at a time.
const minimalSARIF = `{
  "version": "2.1.0",
  "runs": [{
    "tool": {"driver": {"name": "gomdlint"}},
    "columnKind": "utf16CodeUnits",
    "results": [{
      "ruleId": "MD009",
      "level": "warning",
      "message": {"text": "Trailing whitespace"},
      "locations": [{"physicalLocation": {
        "artifactLocation": {"uri": "docs/a.md", "uriBaseId": "%SRCROOT%"},
        "region": {"startLine": 3, "startColumn": 10, "endLine": 3, "endColumn": 13}
      }}],
      "fixes": [{"artifactChanges": [{
        "artifactLocation": {"uri": "docs/a.md", "uriBaseId": "%SRCROOT%"},
        "replacements": [{"deletedRegion": {"startLine": 3, "startColumn": 10, "endLine": 3, "endColumn": 13}}]
      }]}]
    }]
  }]
}`

func TestSchemaValidator_AcceptsValidSARIF(t *testing.T) {
	t.Parallel()

	validator := loadSARIFValidator(t)
	assert.Empty(t, validator.validate(decodeJSON(t, []byte(minimalSARIF))))
}

func TestSchemaValidator_RejectsInvalidSARIF(t *testing.T) {
	t.Parallel()

	validator := loadSARIFValidator(t)
	region := `"region": {"startLine": 3, "startColumn": 10, "endLine": 3, "endColumn": 13}`
	change := `{
        "artifactLocation": {"uri": "docs/a.md", "uriBaseId": "%SRCROOT%"},
        "replacements": [{"deletedRegion": {"startLine": 3, "startColumn": 10, "endLine": 3, "endColumn": 13}}]
      }`

	tests := []struct {
		name    string
		old     string
		new     string
		pointer string
	}{
		{
			name:    "line number below one",
			old:     region,
			new:     `"region": {"startLine": 0}`,
			pointer: "/runs/0/results/0/locations/0/physicalLocation/region/startLine",
		},
		{
			name:    "region without a start",
			old:     region,
			new:     `"region": {"endColumn": 4}`,
			pointer: "/runs/0/results/0/locations/0/physicalLocation/region",
		},
		{
			name:    "windows path is not a uri reference",
			old:     `"artifactLocation": {"uri": "docs/a.md", "uriBaseId": "%SRCROOT%"},` + "\n        " + region,
			new:     `"artifactLocation": {"uri": "C:\\docs\\a.md"},` + "\n        " + region,
			pointer: "/runs/0/results/0/locations/0/physicalLocation/artifactLocation/uri",
		},
		{
			name:    "duplicate artifact changes in one fix",
			old:     `"artifactChanges": [` + change + `]`,
			new:     `"artifactChanges": [` + change + `, ` + change + `]`,
			pointer: "/runs/0/results/0/fixes/0/artifactChanges",
		},
		{
			name:    "level outside its enum",
			old:     `"level": "warning"`,
			new:     `"level": "info"`,
			pointer: "/runs/0/results/0/level",
		},
		{
			name:    "column kind outside its enum",
			old:     `"columnKind": "utf16CodeUnits"`,
			new:     `"columnKind": "bytes"`,
			pointer: "/runs/0/columnKind",
		},
		{
			name:    "property the schema does not define",
			old:     `"ruleId": "MD009",`,
			new:     `"ruleId": "MD009", "column": 3,`,
			pointer: "/runs/0/results/0",
		},
		{
			name:    "missing required property",
			old:     `"message": {"text": "Trailing whitespace"},`,
			new:     ``,
			pointer: "/runs/0/results/0",
		},
		{
			name:    "integer given as string",
			old:     `{"deletedRegion": {"startLine": 3,`,
			new:     `{"deletedRegion": {"startLine": "3",`,
			pointer: "/runs/0/results/0/fixes/0/artifactChanges/0/replacements/0/deletedRegion/startLine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Contains(t, minimalSARIF, tt.old, "fixture edit must apply")
			broken := strings.Replace(minimalSARIF, tt.old, tt.new, 1)

			violations := validator.validate(decodeJSON(t, []byte(broken)))
			require.NotEmpty(t, violations)
			assert.True(t, hasViolationAt(violations, tt.pointer), "want a violation at %s, got %v", tt.pointer, violations)
		})
	}
}

// hasViolationAt reports whether any violation names exactly pointer.
func hasViolationAt(violations []string, pointer string) bool {
	for _, violation := range violations {
		if strings.HasPrefix(violation, pointer+": ") {
			return true
		}
	}
	return false
}
