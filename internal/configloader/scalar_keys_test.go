package configloader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestLoad_RejectsAKeyThatIsNoScalar checks config files with a mapping key
// that is a sequence or a mapping, or an alias of one. Loading fails with
// an error that names the file and the key's line, before anything decodes
// the file: yaml.v3 panicked on such a key in a mapping that merges another
// or is merged, and the command reported an internal error. The key is an
// error in a value that loading never reads too, which loaded: the value
// of an unknown key or option, a merged value that another replaces, and
// the value of a null key.
func TestLoad_RejectsAKeyThatIsNoScalar(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, content, line string }{
		{"beside a merge key", "rules:\n  <<: {MD001: false}\n  ? [MD013]\n  : false\n", "line 3"},
		{"an alias under a reused anchor name", "x: &a name\n*a : &a [x]\n*a : false\n<<: {}\n", "line 3"},
		{"without a merge key", "rules:\n  ? [MD013]\n  : false\n", "line 2"},
		{"in an unknown key", "x-shared:\n  ? [a]\n  : 1\nrules:\n  MD013: false\n", "line 2"},
		{"in an unknown option", "rules:\n  MD013:\n    bogus:\n      ? [a]\n      : 1\n", "line 4"},
		{"in a list item beside a merge key", "rules:\n  MD013:\n    options:\n      x:\n        - <<: {}\n          ? !!str [a]\n          : 1\n", "line 6"},
		{"in a merged mapping", "rules:\n  MD013:\n    line_length:\n      1: a\n      <<: {? [x] : 1}\n", "line 5"},
		{"in a merged value that a written key replaces", "rules:\n  <<: {MD013: {? [x] : 1}}\n  MD013: false\n", "line 2"},
		{"in a merged value that an earlier merged mapping replaces", "rules:\n  <<: [{MD013: false}, {MD013: {? [x] : 1}}]\n", "line 2"},
		{"in the value of a null key", "~:\n  ? [x]\n  : 1\nflavor: gfm\n", "line 2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var err error
			require.NotPanics(t, func() { _, err = loadProject(t, tt.content) })
			require.Error(t, err)
			assert.Equal(t, "parse YAML: yaml: unmarshal errors:\n  "+tt.line+": mapping key must be a scalar",
				afterConfigFile(t, err))
		})
	}
}

// TestLoadConfigFile_ReadsFilesWithoutAliasesAsBefore checks config files
// that write no alias and no key other than a scalar, across the settings,
// shorthands, flat options, merge keys, tags, styles, warnings and errors.
// Each loads exactly as it did on main before the change, measured there:
// the same settings and warnings, or the same error. $FILE stands for the
// file's path.
func TestLoadConfigFile_ReadsFilesWithoutAliasesAsBefore(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, content, want string }{
		{
			name:    `empty`,
			content: ``,
			want:    `OK {"rules":{}} warnings=`,
		},
		{
			name:    `comment`,
			content: "# only a comment\n",
			want:    `OK {"rules":{}} warnings=`,
		},
		{
			name: `settings`,
			content: "flavor: gfm\nseverity_default: warning\nignore: [vendor/**, \"*.tmp\"]\ngitignore: false\n" +
				"default_ignores: true\nbackups:\n  enabled: false\n  mode: sidecar\n",
			want: `OK {"backups":{"enabled":false,"mode":"sidecar"},"default_ignores":true,"flavor":"gfm",` +
				`"gitignore":false,"ignore":["vendor/**","*.tmp"],"rules":{},` +
				`"severity_default":"warning"} warnings=`,
		},
		{
			name: `rule forms`,
			content: "rules:\n  MD001: false\n  MD003: true\n  line-length: warning\n  no-trailing-spaces:\n" +
				"    enabled: false\n    severity: error\n    auto_fix: false\n    options:\n" +
				"      br_spaces: 3\n",
			want: `OK {"rules":{"MD001":{"enabled":false},"MD003":{"enabled":true},"MD009":{"auto_fix":false,` +
				`"enabled":false,"options":{"br_spaces":3},"severity":"error"},"MD013":{"enabled":true,` +
				`"severity":"warning"}}} warnings=`,
		},
		{
			name:    `flat options`,
			content: "rules:\n  MD013:\n    line_length: 100\n    tables: false\n    options:\n      headings: false\n",
			want: `OK {"rules":{"MD013":{"options":{"ignore_headings":true,"ignore_tables":true,` +
				`"max":100}}}} warnings=`,
		},
		{
			name:    `merge under a rule`,
			content: "rules:\n  MD013:\n    <<: {line_length: 100, tables: false}\n    tables: true\n",
			want:    `OK {"rules":{"MD013":{"options":{"ignore_tables":false,"max":100}}}} warnings=`,
		},
		{
			name:    `merge sequence under rules`,
			content: "rules:\n  <<: [{MD001: false}, {MD003: true, MD001: true}]\n  MD009: false\n",
			want: `OK {"rules":{"MD001":{"enabled":false},"MD003":{"enabled":true},` +
				`"MD009":{"enabled":false}}} warnings=`,
		},
		{
			name:    `tags and quotes`,
			content: "rules:\n  \"MD013\": !!str error\n  'MD009': !!bool false\n",
			want: `OK {"rules":{"MD009":{"enabled":false},"MD013":{"enabled":true,` +
				`"severity":"error"}}} warnings=`,
		},
		{
			name:    `flow`,
			content: "{rules: {MD013: {enabled: false}}, flavor: commonmark}\n",
			want:    `OK {"flavor":"commonmark","rules":{"MD013":{"enabled":false}}} warnings=`,
		},
		{
			name:    `unknown key with content`,
			content: "x-notes:\n  owner: docs\n  list: [1, {a: b}]\n  nested: {c: {d: e}}\nrules:\n  MD013: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}} warnings=$FILE:1: unknown key "x-notes"; it is ignored`,
		},
		{
			name:    `rule_defaults`,
			content: "rule_defaults:\n  MD013: {line_length: 120}\n  MD009: false\nrules:\n  MD013: error\n",
			want: `OK {"rule_defaults":{"MD009":{"enabled":false},"MD013":{"options":{"max":120}}},` +
				`"rules":{"MD013":{"enabled":true,"severity":"error"}}} warnings=`,
		},
		{
			name:    `nulls`,
			content: "rules:\n  MD013:\nflavor:\n",
			want:    `OK {"rules":{"MD013":{}}} warnings=`,
		},
		{
			name:    `anchors without aliases`,
			content: "x: &a {b: c}\nrules: &r\n  MD013: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}} warnings=$FILE:1: unknown key "x"; it is ignored`,
		},
		{
			name:    `non-string scalar keys`,
			content: "rules:\n  1: false\n  true: error\n",
			want: `OK {"rules":{"1":{"enabled":false},"true":{"enabled":true,` +
				`"severity":"error"}}} warnings=$FILE:2: unknown rule "1"; it is ignored | ` +
				`$FILE:3: unknown rule "true"; it is ignored`,
		},
		{
			name:    `two documents`,
			content: "---\nrules:\n  MD013: false\n---\nflavor: gfm\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}} warnings=`,
		},
		{
			name: `nested option values`,
			content: "rules:\n  MD013:\n    options:\n      nested: {a: [1, 2], b: {c: true}}\n" +
				"      when: 2026-10-19\n",
			want: `OK {"rules":{"MD013":{"options":{"nested":{"a":[1,2],"b":{"c":true}},` +
				`"when":"2026-10-19T00:00:00Z"}}}} warnings=$FILE:4: unknown option "nested" for line-length; it is ignored | ` +
				`$FILE:5: unknown option "when" for line-length; it is ignored`,
		},
		{
			name:    `rules as a list`,
			content: "rules: [a]\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into map[string]config.RuleConfig",
		},
		{
			name:    `a number as a rule value`,
			content: "rules:\n  MD013: 80\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 2: rule settings must be true, false, a severity (error," +
				` warning or info) or a mapping, not !!int "80"`,
		},
		{
			name:    `a bad boolean`,
			content: "rules:\n  MD013: {enabled: maybe}\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `maybe` into bool",
		},
		{
			name:    `an option written twice`,
			content: "rules:\n  MD013: {line_length: 1, line_length: 2}\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 2: mapping key \"line_length\" already defined at line 2",
		},
		{
			name:    `a bad flavor`,
			content: "flavor: [x]\nrules:\n  MD013: false\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into config.Flavor",
		},
		{
			name:    `a merge of a number`,
			content: "<<: 5\n",
			want: `ERROR $FILE: parse YAML: ` +
				`yaml: map merge requires map or sequence of maps as the value`,
		},
		{
			name:    `flat and options conflict`,
			content: "rules:\n  MD013:\n    line_length: 100\n    options:\n      line_length: 90\n",
			want:    `ERROR $FILE: rules: line 3: "line_length" and "options.line_length" both set line-length option "max"; keep one of them`,
		},
		{
			name:    `ignore as a string`,
			content: "ignore: vendor/**\n",
			want: `ERROR $FILE: parse YAML: ` +
				"yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `vendor/**` into []string",
		},
		{
			name:    `unknown option`,
			content: "rules:\n  MD013:\n    max: 100\n    bogus: 1\n",
			want:    `OK {"rules":{"MD013":{"options":{"max":100}}}} warnings=$FILE:4: unknown option "bogus" for line-length; it is ignored`,
		},
		{
			name:    `misspelled key`,
			content: "flavour: gfm\n",
			want:    `OK {"rules":{}} warnings=$FILE:1: unknown key "flavour" (did you mean "flavor"?); it is ignored`,
		},
		{
			name:    `JSON in YAML`,
			content: "{\"rules\": {\"MD013\": false}}\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}} warnings=`,
		},
		{
			name:    `anchored value`,
			content: "rules:\n  MD013: &v {line_length: 100}\n",
			want:    `OK {"rules":{"MD013":{"options":{"max":100}}}} warnings=`,
		},
		{
			name:    `binary key`,
			content: "rules:\n  !!binary TUQwMTM=: false\n",
			want:    `OK {"rules":{"MD013":{"enabled":false}}} warnings=`,
		},
		{
			name: `merges under a rule and its options`,
			content: "rules:\n  MD013:\n    <<: [{tables: true}, {headings: false}]\n" +
				"    options: {<<: {max: 5}, strict: true}\n",
			want: `OK {"rules":{"MD013":{"options":{"ignore_headings":true,"ignore_tables":false,"max":5,` +
				`"strict":true}}}} warnings=`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), ".gomdlint.yml")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))
			var (
				cfg      *config.Config
				warnings []string
				err      error
			)
			require.NotPanics(t, func() { cfg, warnings, err = loadConfigFile(path) })
			got := "OK " + loadedSettings(cfg) + " warnings=" + strings.Join(warnings, " | ")
			if err != nil {
				got = "ERROR " + err.Error()
			}
			assert.Equal(t, tt.want, strings.ReplaceAll(got, path, "$FILE"))
		})
	}
}

// loadedSettings renders what cfg sets, compactly and in key order: each
// setting that it gives a value, and the settings of each rule.
func loadedSettings(cfg *config.Config) string {
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
