package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// notAScalar ends the error for a mapping key that is no scalar.
const notAScalar = ": mapping key must be a scalar"

// TestFromYAML_RejectsAKeyThatIsNoScalar covers mapping keys that are a
// sequence or a mapping, or an alias of one. No configuration holds such a
// key, and yaml.v3 panics on one in a mapping that merges another, since it
// hashes the keys of such a mapping and a collection cannot be hashed. So
// such a key is an error that names its line, wherever it is written, and
// nothing decodes the document.
func TestFromYAML_RejectsAKeyThatIsNoScalar(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, content, line string }{
		// yaml.v3 panicked on each of these before, on main too.
		{"a sequence key beside a merge key", "<<: {}\n? [x]\n: 1\n", "line 2"},
		{"under rules", "rules:\n  <<: {MD001: false}\n  ? [MD013]\n  : false\n", "line 3"},
		{"under a rule", "rules:\n  MD013:\n    <<: {tables: true}\n    ? [x]\n    : 1\n", "line 4"},
		{"a mapping key under options", "rules:\n  MD013:\n    options:\n      <<: {a: 1}\n      ? {x: y}\n      : 1\n", "line 5"},
		{"an alias of a sequence", "x: &a name\n*a : &b [x]\n*b : false\n<<: {}\n", "line 3"},
		{"an alias of a mapping", "x: &a name\n*a : &b {k: v}\n*b : false\n<<: {}\n", "line 3"},
		// The check enters list items and the values of merge keys. yaml.v3
		// trusts the tag of the first key, so it reached the merge.
		{"in a list item beside a merge key", "rules:\n  MD013:\n    options:\n      x:\n        - <<: {}\n          ? !!str [a]\n          : 1\n", "line 6"},
		{"in a merged mapping", "rules:\n  MD013:\n    line_length:\n      1: a\n      <<: {? [x] : 1}\n", "line 5"},
		// The same alias keys under one anchor name, which two nodes take.
		// yaml.v3 compared the alias keys by that name, and once the alias
		// keys of scalars were written out, the merge met the collection.
		{"an alias of a sequence under a reused anchor name", "x: &a name\n*a : &a [x]\n*a : false\n<<: {}\n", "line 3"},
		{"an alias of a mapping under a reused anchor name", "x: &a name\n*a : &a {k: v}\n*a : false\n<<: {}\n", "line 3"},
		{"under rules under a reused anchor name", "x: &a MD013\nrules:\n  *a : &a [x]\n  *a : false\n  <<: {}\n", "line 4"},
		// Without a merge key, decoding rejected these with a type error,
		// which named the anchor's line for an alias.
		{"a sequence key under rules", "rules:\n  ? [MD013]\n  : false\n", "line 2"},
		{"a sequence key at the top", "? [a]\n: 1\nflavor: gfm\n", "line 1"},
		{"a flow sequence key", "{[a]: 1}\n", "line 1"},
		{"an alias of a sequence without a merge key", "x: &c [a]\nrules:\n  ? *c\n  : false\n", "line 3"},
		// Decoding rejected these with an error of its own: a type error,
		// beside the document's other errors, or, beside a merge key in an
		// option, an invalid map key. The first is the document on which an
		// earlier form of the alias key check made yaml.v3 panic.
		{"in a merged mapping that merges in turn", "c: &c {tables: true}\na: &a\n  <<: *c\n  ? [x]\n  : 1\nrules:\n  MD013:\n    <<: *a\n", "line 4"},
		{"under a rule beside a type error", "rules:\n  MD013:\n    ? [a]\n    : 1\n    enabled: maybe\n", "line 3"},
		{"at the top beside a type error", "? [a]\n: 1\nflavor: [x]\n", "line 1"},
		{"beside a merge key in an option", "rules:\n  MD013:\n    line_length: {<<: {}, [x]: 1}\n", "line 3"},
		// An error that ends decoding hid such a key.
		{"before an error that ends decoding", "flavor: !!int abc\nrules:\n  <<: {MD013: false}\n  ? [x]\n  : 1\n", "line 4"},
		// Decoding never reads these values. The loader loaded each of
		// them: with a warning about an unknown key or option, or with none
		// for a merged value that another replaces and for the value of a
		// null key. FromYAML reads every option under a rule, so it
		// rejected the third with an invalid map key.
		{"in an unknown key", "x-shared:\n  ? [a]\n  : 1\nrules:\n  MD013: false\n", "line 2"},
		{"beside a merge key in an unknown key", "x-shared:\n  <<: {}\n  ? [a]\n  : 1\n", "line 3"},
		{"in an option under the rule key", "rules:\n  MD013:\n    bogus:\n      ? [a]\n      : 1\n", "line 4"},
		{"in a merged value that a written key replaces", "rules:\n  <<: {MD013: {? [x] : 1}}\n  MD013: false\n", "line 2"},
		{"in a merged value that an earlier merged mapping replaces", "rules:\n  <<: [{MD013: false}, {MD013: {? [x] : 1}}]\n", "line 2"},
		{"in the value of a null key", "~:\n  ? [x]\n  : 1\nflavor: gfm\n", "line 2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var err error
			require.NotPanics(t, func() { _, err = config.FromYAML([]byte(tt.content)) })
			require.Error(t, err)
			assert.Equal(t, "parse yaml: "+yamlErrors+tt.line+notAScalar, err.Error())
		})
	}

	// Each such key is named, in the order of the document.
	var err error
	require.NotPanics(t, func() {
		_, err = config.FromYAML([]byte("? [a]\n: 1\nrules:\n  ? {b: c}\n  : 2\n  MD013:\n    ? [d]\n    : 3\n"))
	})
	require.Error(t, err)
	assert.Equal(t, "parse yaml: "+yamlErrors+"line 1"+notAScalar+"\n  line 4"+notAScalar+"\n  line 7"+notAScalar,
		err.Error())
}

// TestFromYAML_ReadsDocumentsWithoutAliasesAsBefore covers documents that
// write no alias and no key other than a scalar, across the settings,
// shorthands, flat options, merge keys, tags, styles and errors. Each reads
// exactly as it did on main before the change, measured there: the same
// settings, or the same error.
func TestFromYAML_ReadsDocumentsWithoutAliasesAsBefore(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, content, want string }{
		{
			name:    `empty`,
			content: ``,
			want:    `OK {"rules":{}}`,
		},
		{
			name:    `comment`,
			content: "# only a comment\n",
			want:    `OK {"rules":{}}`,
		},
		{
			name: `settings`,
			content: "flavor: gfm\nseverity_default: warning\nignore: [vendor/**, \"*.tmp\"]\ngitignore: false\n" +
				"default_ignores: true\nbackups:\n  enabled: false\n  mode: sidecar\n",
			want: `OK {"backups":{"enabled":false,"mode":"sidecar"},"default_ignores":true,"flavor":"gfm",` +
				`"gitignore":false,"ignore":["vendor/**","*.tmp"],"rules":{},"severity_default":"warning"}`,
		},
		{
			name: `rule forms`,
			content: "rules:\n  MD001: false\n  MD003: true\n  line-length: warning\n  no-trailing-spaces:\n" +
				"    enabled: false\n    severity: error\n    auto_fix: false\n    options:\n" +
				"      br_spaces: 3\n",
			want: `OK {"rules":{"MD001":{"enabled":false},"MD003":{"enabled":true},` +
				`"line-length":{"enabled":true,"severity":"warning"},` +
				`"no-trailing-spaces":{"auto_fix":false,"enabled":false,"options":{"br_spaces":3},` +
				`"severity":"error"}}}`,
		},
		{
			name:    `flat options`,
			content: "rules:\n  MD013:\n    line_length: 100\n    tables: false\n    options:\n      headings: false\n",
			want:    `OK {"rules":{"MD013":{"options":{"headings":false,"line_length":100,"tables":false}}}}`,
		},
		{
			name:    `merge under a rule`,
			content: "rules:\n  MD013:\n    <<: {line_length: 100, tables: false}\n    tables: true\n",
			want:    `OK {"rules":{"MD013":{"options":{"line_length":100,"tables":true}}}}`,
		},
		{
			name:    `merge sequence under rules`,
			content: "rules:\n  <<: [{MD001: false}, {MD003: true, MD001: true}]\n  MD009: false\n",
			want: `OK {"rules":{"MD001":{"enabled":false},"MD003":{"enabled":true},` +
				`"MD009":{"enabled":false}}}`,
		},
		{
			name:    `tags and quotes`,
			content: "rules:\n  \"MD013\": !!str error\n  'MD009': !!bool false\n",
			want:    `OK {"rules":{"MD009":{"enabled":false},"MD013":{"enabled":true,"severity":"error"}}}`,
		},
		{
			name:    `flow`,
			content: "{rules: {MD013: {enabled: false}}, flavor: commonmark}\n",
			want:    `OK {"flavor":"commonmark","rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name:    `unknown key with content`,
			content: "x-notes:\n  owner: docs\n  list: [1, {a: b}]\n  nested: {c: {d: e}}\nrules:\n  MD013: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name:    `rule_defaults`,
			content: "rule_defaults:\n  MD013: {line_length: 120}\n  MD009: false\nrules:\n  MD013: error\n",
			want: `OK {"rule_defaults":{"MD009":{"enabled":false},"MD013":{"options":{"line_length":120}}},` +
				`"rules":{"MD013":{"enabled":true,"severity":"error"}}}`,
		},
		{
			name:    `nulls`,
			content: "rules:\n  MD013:\nflavor:\n",
			want:    `OK {"rules":{"MD013":{}}}`,
		},
		{
			name:    `anchors without aliases`,
			content: "x: &a {b: c}\nrules: &r\n  MD013: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name:    `non-string scalar keys`,
			content: "rules:\n  1: false\n  true: error\n",
			want:    `OK {"rules":{"1":{"enabled":false},"true":{"enabled":true,"severity":"error"}}}`,
		},
		{
			name:    `two documents`,
			content: "---\nrules:\n  MD013: false\n---\nflavor: gfm\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name: `nested option values`,
			content: "rules:\n  MD013:\n    options:\n      nested: {a: [1, 2], b: {c: true}}\n" +
				"      when: 2026-10-19\n",
			want: `OK {"rules":{"MD013":{"options":{"nested":{"a":[1,2],"b":{"c":true}},` +
				`"when":"2026-10-19T00:00:00Z"}}}}`,
		},
		{
			name:    `rules as a list`,
			content: "rules: [a]\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into map[string]config.RuleConfig",
		},
		{
			name:    `a number as a rule value`,
			content: "rules:\n  MD013: 80\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 2: rule settings must be true, false, a severity (error," +
				` warning or info) or a mapping, not !!int "80"`,
		},
		{
			name:    `a bad boolean`,
			content: "rules:\n  MD013: {enabled: maybe}\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `maybe` into bool",
		},
		{
			name:    `an option written twice`,
			content: "rules:\n  MD013: {line_length: 1, line_length: 2}\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 2: mapping key \"line_length\" already defined at line 2",
		},
		{
			name:    `a bad flavor`,
			content: "flavor: [x]\nrules:\n  MD013: false\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into config.Flavor",
		},
		{
			name:    `a merge of a number`,
			content: "<<: 5\n",
			want: `ERROR parse yaml: ` +
				`yaml: map merge requires map or sequence of maps as the value`,
		},
		{
			name:    `flat and options conflict`,
			content: "rules:\n  MD013:\n    line_length: 100\n    options:\n      line_length: 90\n",
			want: `ERROR parse yaml: ` +
				`rules.MD013: line 3: "line_length" and "options.line_length" both set option "line_length"; keep one of them`,
		},
		{
			name:    `ignore as a string`,
			content: "ignore: vendor/**\n",
			want: `ERROR parse yaml: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `vendor/**` into []string",
		},
		{
			name:    `unknown option`,
			content: "rules:\n  MD013:\n    max: 100\n    bogus: 1\n",
			want:    `OK {"rules":{"MD013":{"options":{"bogus":1,"max":100}}}}`,
		},
		{
			name:    `misspelled key`,
			content: "flavour: gfm\n",
			want:    `OK {"rules":{}}`,
		},
		{
			name:    `JSON in YAML`,
			content: "{\"rules\": {\"MD013\": false}}\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name:    `anchored value`,
			content: "rules:\n  MD013: &v {line_length: 100}\n",
			want:    `OK {"rules":{"MD013":{"options":{"line_length":100}}}}`,
		},
		{
			name:    `binary key`,
			content: "rules:\n  !!binary TUQwMTM=: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}}`,
		},
		{
			name: `merges under a rule and its options`,
			content: "rules:\n  MD013:\n    <<: [{tables: true}, {headings: false}]\n" +
				"    options: {<<: {max: 5}, strict: true}\n",
			want: `OK {"rules":{"MD013":{"options":{"headings":false,"max":5,"strict":true,"tables":true}}}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				cfg *config.Config
				err error
			)
			require.NotPanics(t, func() { cfg, err = config.FromYAML([]byte(tt.content)) })
			got := "OK " + settingsOf(cfg)
			if err != nil {
				got = "ERROR " + err.Error()
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// settingsOf renders what cfg sets, compactly and in key order: each
// setting that it gives a value, and the settings of each rule.
func settingsOf(cfg *config.Config) string {
	if cfg == nil {
		return "nil"
	}
	rule := func(rc config.RuleConfig) map[string]any {
		out := map[string]any{}
		if rc.Enabled != nil {
			out["enabled"] = *rc.Enabled
		}
		if rc.Severity != nil {
			out["severity"] = *rc.Severity
		}
		if rc.AutoFix != nil {
			out["auto_fix"] = *rc.AutoFix
		}
		if rc.Options != nil {
			out["options"] = rc.Options
		}
		return out
	}
	rules := func(m map[string]config.RuleConfig) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			out[k] = rule(v)
		}
		return out
	}
	set := map[string]any{}
	if cfg.Flavor != "" {
		set["flavor"] = cfg.Flavor
	}
	if cfg.SeverityDefault != "" {
		set["severity_default"] = cfg.SeverityDefault
	}
	if cfg.Ignore != nil {
		set["ignore"] = cfg.Ignore
	}
	if cfg.Gitignore != nil {
		set["gitignore"] = *cfg.Gitignore
	}
	if cfg.DefaultIgnores != nil {
		set["default_ignores"] = *cfg.DefaultIgnores
	}
	backups := map[string]any{}
	if cfg.Backups.Enabled != nil {
		backups["enabled"] = *cfg.Backups.Enabled
	}
	if cfg.Backups.Mode != "" {
		backups["mode"] = cfg.Backups.Mode
	}
	if len(backups) > 0 {
		set["backups"] = backups
	}
	if cfg.Rules != nil {
		set["rules"] = rules(cfg.Rules)
	}
	if cfg.RuleDefaults != nil {
		set["rule_defaults"] = rules(cfg.RuleDefaults)
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		return fmt.Sprintf("%v", set)
	}
	return string(encoded)
}
