package ruledocs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// level is a named string type, as a rule may use for an option's values.
type level string

// TestYAMLValueWritesEveryKindOnOneLine covers yamlValue for values beyond
// strings and lists of them: named string types, pointers, nil, maps with
// keys of any kind, and a merge key, each written on one line that YAML
// reads back as the value, after a key.
func TestYAMLValueWritesEveryKindOnOneLine(t *testing.T) {
	text := "a\nb"
	tests := []struct {
		name  string
		value any
		want  any
	}{
		{name: "a named string type", value: level("null"), want: "null"},
		{name: "a pointer to a string", value: &text, want: "a\nb"},
		{name: "a nil pointer", value: (*string)(nil), want: nil},
		{name: "nil", value: nil, want: nil},
		{name: "a map with keys of any kind", value: map[any]any{"k": "a\nb", 1: true}, want: map[any]any{"k": "a\nb", 1: true}},
		{name: "a merge key", value: map[string]any{"<<": "<<"}, want: map[string]any{"<<": "<<"}},
		{name: "a list of named strings", value: []level{"~", "x\u2028y"}, want: []any{"~", "x\u2028y"}},
	}
	for _, tt := range tests {
		line := yamlValue(tt.value)
		assert.NotContains(t, line, "\n", tt.name)

		var read map[string]any
		require.NoError(t, yaml.Unmarshal([]byte("key: "+line+"\n"), &read), "%s: %s", tt.name, line)
		assert.Equal(t, tt.want, read["key"], "%s: %s", tt.name, line)
	}
}
