package ruledocs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// namedRule is a custom rule with a name of a test's choosing.
type namedRule struct {
	lint.BaseRule
}

func (*namedRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil }

// TestImplicitKeyLimit covers names at YAML's limit of 1,024 characters for
// an implicit key, counted as YAML writes the key, quotes included, and in
// characters rather than bytes: a rule is keyed by its name up to the
// limit and by its ID beyond it, and an option is written as an implicit
// entry up to the limit and as an explicit one beyond it. Each line loads.
func TestImplicitKeyLimit(t *testing.T) {
	quoted := func(length int) string { return "a: " + strings.Repeat("y", length-len(`"a: "`)) }
	tests := []struct {
		name     string
		key      string
		implicit bool
	}{
		{name: "1,023 characters", key: strings.Repeat("x", 1023), implicit: true},
		{name: "1,024 characters", key: strings.Repeat("x", 1024), implicit: true},
		{name: "1,025 characters", key: strings.Repeat("x", 1025)},
		{name: "1,024 characters with quotes", key: quoted(1024), implicit: true},
		{name: "1,025 characters with quotes", key: quoted(1025)},
		{name: "1,024 two-byte characters", key: strings.Repeat("é", 1024), implicit: true},
		{name: "1,025 two-byte characters", key: strings.Repeat("é", 1025)},
	}
	for _, tt := range tests {
		registry := lint.NewRegistry()
		registry.Register(&namedRule{lint.NewBaseRule("XX001", tt.key, "", nil, false)})

		want := "XX001"
		if tt.implicit {
			want = tt.key
		}
		key := ConfigKey(registry, "XX001")
		assert.Equal(t, want, key, tt.name)
		var rule map[string]string
		require.NoError(t, yaml.Unmarshal([]byte(yamlKey(key)+": on\n"), &rule), tt.name)
		assert.Equal(t, map[string]string{key: "on"}, rule, tt.name)

		lines := templateOption(lint.OptionSpec{Name: tt.key, Type: lint.OptionString, Default: "v"})
		if tt.implicit {
			assert.Len(t, lines, 1, "%s: an implicit entry", tt.name)
		} else {
			assert.Len(t, lines, 2, "%s: an explicit entry", tt.name)
		}
		var option map[string]string
		require.NoError(t, yaml.Unmarshal([]byte(strings.Join(lines, "\n")+"\n"), &option), tt.name)
		assert.Equal(t, map[string]string{tt.key: "v"}, option, tt.name)
	}
}

// TestYAMLNodeRefusesValuesWithoutYAML covers values that YAML has no form
// for, on which the encoder panics: YAMLNode reports an error instead, and
// yamlValue writes them as quoted text on one line.
func TestYAMLNodeRefusesValuesWithoutYAML(t *testing.T) {
	for _, value := range []any{make(chan int), func() {}, complex(1, 2)} {
		_, err := YAMLNode(value, true)
		require.ErrorIs(t, err, errNoYAML)
		assert.NotContains(t, yamlValue(value), "\n")
	}
}
