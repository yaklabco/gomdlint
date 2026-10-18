package rules

import (
	"context"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// optionSpelling is one alias of a rule option: markdownlint's spelling of
// an option that gomdlint names differently.
type optionSpelling struct {
	Option   string
	Alias    string
	Inverted bool
}

// TestCatalogOptionSpellings lists every option whose markdownlint spelling
// differs from gomdlint's canonical name, and checks that the catalog
// declares exactly these aliases and that each loads as its option.
func TestCatalogOptionSpellings(t *testing.T) {
	want := map[string][]optionSpelling{
		"MD009": {{Option: "ignore_code_blocks", Alias: "code_blocks", Inverted: true}},
		"MD012": {{Option: "max_consecutive", Alias: "maximum"}},
		"MD013": {
			{Option: "max", Alias: "line_length"},
			{Option: "heading_max", Alias: "heading_line_length"},
			{Option: "code_block_max", Alias: "code_block_line_length"},
			{Option: "ignore_code_blocks", Alias: "code_blocks", Inverted: true},
			{Option: "ignore_tables", Alias: "tables", Inverted: true},
			{Option: "ignore_headings", Alias: "headings", Inverted: true},
		},
		"MD059": {{Option: "patterns", Alias: "prohibited_texts"}},
	}

	got := make(map[string][]optionSpelling)
	for _, meta := range Catalog() {
		for _, spec := range meta.Options {
			for _, alias := range spec.Aliases {
				got[meta.ID] = append(got[meta.ID], optionSpelling{Option: spec.Name, Alias: alias.Name, Inverted: alias.Inverted})
			}
		}
	}
	assert.Equal(t, want, got)

	for id, spellings := range want {
		for _, spelling := range spellings {
			for _, value := range []any{true, false, 7} {
				normalized, issues, declared := lint.DefaultRegistry.NormalizeOptions(id, map[string]any{spelling.Alias: value})
				require.True(t, declared, id)
				expected := value
				if enabled, ok := value.(bool); ok && spelling.Inverted {
					expected = !enabled
				}
				spec := optionSpec(t, id, spelling.Option)
				if !spec.Type.Accepts(expected) {
					assert.Empty(t, normalized, "%s %s: %v is not a value of the option", id, spelling.Alias, value)
					assert.Len(t, issues, 1, "%s %s: %v", id, spelling.Alias, value)
					continue
				}
				assert.Equal(t, map[string]any{spelling.Option: expected}, normalized, "%s %s: %v", id, spelling.Alias, value)
				assert.Empty(t, issues, "%s %s: %v", id, spelling.Alias, value)
			}
		}
	}
}

// optionSpec returns the catalog's spec of the named option of a rule.
func optionSpec(t *testing.T, id, name string) lint.OptionSpec {
	t.Helper()
	meta, ok := lint.DefaultRegistry.Meta(id)
	require.True(t, ok, id)
	for _, spec := range meta.Options {
		if spec.Name == name {
			return spec
		}
	}
	require.Failf(t, "no such option", "%s has no option %q", id, name)
	return lint.OptionSpec{}
}

// TestCatalogOptionsFollowMarkdownlint checks the options of every rule
// that markdownlint has against markdownlint 0.41.1's parameters: each
// parameter is one option, under its own name or as an alias, with the
// same type, default and allowed values, inverted where the alias is.
// Options that markdownlint lacks are marked gomdlint-only, and no alias
// is a spelling that markdownlint does not know.
func TestCatalogOptionsFollowMarkdownlint(t *testing.T) {
	for _, meta := range Catalog() {
		parameters, fromMarkdownlint := markdownlintOptions[meta.ID]
		if !fromMarkdownlint {
			for _, spec := range meta.Options {
				assert.True(t, spec.GomdlintOnly, "%s %s: markdownlint lacks the rule", meta.ID, spec.Name)
				assert.Empty(t, spec.Aliases, "%s %s: markdownlint has no spelling for it", meta.ID, spec.Name)
			}
			continue
		}

		seen := make(map[string]bool)
		for _, spec := range meta.Options {
			if spec.GomdlintOnly {
				assert.NotContains(t, parameters, spec.Name, "%s %s: markdownlint has the option", meta.ID, spec.Name)
				assert.Empty(t, spec.Aliases, "%s %s: markdownlint has no spelling for it", meta.ID, spec.Name)
				continue
			}

			// markdownlint's spelling: the alias where gomdlint names the
			// option differently.
			name, inverted := spec.Name, false
			if assert.LessOrEqual(t, len(spec.Aliases), 1, "%s %s: one markdownlint spelling", meta.ID, spec.Name) &&
				len(spec.Aliases) == 1 {
				name, inverted = spec.Aliases[0].Name, spec.Aliases[0].Inverted
			}
			parameter, ok := parameters[name]
			if !assert.True(t, ok, "%s: markdownlint has no parameter %q", meta.ID, name) {
				continue
			}
			seen[name] = true

			gotType := string(spec.Type)
			if spec.Type == lint.OptionStringOrFalse {
				gotType = "string" // markdownlint documents a string, and false turns the pattern off there too.
			}
			assert.Equal(t, parameter.Type, gotType, "%s %s: type", meta.ID, name)
			assert.Equal(t, parameter.Values, spec.Values, "%s %s: allowed values", meta.ID, name)

			defaultValue := spec.Default
			if enabled, isBool := defaultValue.(bool); isBool && inverted {
				defaultValue = !enabled
			}
			switch {
			case spec.Name == "front_matter_title":
				assert.Equal(t, `^\s*title\s*[:=]`, parameter.Default, "%s %s: the schema's pattern", meta.ID, name)
				assert.Equal(t, config.MarkdownlintFrontMatterTitlePattern, defaultValue,
					"%s %s: the pattern markdownlint's code applies", meta.ID, name)
			default:
				assert.Equal(t, parameter.Default, defaultValue, "%s %s: default", meta.ID, name)
			}
		}
		assert.ElementsMatch(t, slices.Collect(maps.Keys(parameters)), slices.Collect(maps.Keys(seen)),
			"%s: every markdownlint parameter is an option", meta.ID)
	}
}

// optionNamePattern matches an option name: lower-case words of letters and
// digits, joined by underscores.
var optionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// TestCatalogOptionsAreWellFormed checks the declarations themselves:
// every rule declares its options, every default is a value of its type
// and allowed, a default taken from another option names one of the same
// type and default, inverted aliases are boolean, and descriptions are
// plain sentences.
func TestCatalogOptionsAreWellFormed(t *testing.T) {
	catalog := Catalog()
	ids := make([]string, 0, len(catalog))
	var unsetDefaults []string
	for _, meta := range catalog {
		ids = append(ids, meta.ID)
		assert.True(t, meta.OptionsDeclared, "%s declares its options", meta.ID)

		spellings := make(map[string]bool)
		byName := make(map[string]lint.OptionSpec)
		for _, spec := range meta.Options {
			byName[spec.Name] = spec
		}
		for _, spec := range meta.Options {
			label := meta.ID + " " + spec.Name
			assert.Regexp(t, optionNamePattern, spec.Name, label)
			assert.Contains(t, []lint.OptionType{
				lint.OptionBoolean, lint.OptionInteger, lint.OptionString,
				lint.OptionStringList, lint.OptionIntegerOrList, lint.OptionStringOrFalse,
			}, spec.Type, label)

			if spec.Default == nil {
				unsetDefaults = append(unsetDefaults, label)
			}
			assert.True(t, spec.Type.Accepts(spec.Default), "%s: the default %#v is a %s", label, spec.Default, spec.Type)
			if len(spec.Values) > 0 {
				assert.Equal(t, lint.OptionString, spec.Type, "%s: only strings list allowed values", label)
				assert.Contains(t, spec.Values, spec.Default, "%s: the default is allowed", label)
			}
			if spec.DefaultFrom != "" {
				source, ok := byName[spec.DefaultFrom]
				if assert.True(t, ok, "%s: takes its default from an option of the rule", label) {
					assert.Equal(t, source.Type, spec.Type, "%s: type of %s", label, spec.DefaultFrom)
					assert.Equal(t, source.Default, spec.Default, "%s: default of %s", label, spec.DefaultFrom)
				}
			}

			for _, spelling := range append([]string{spec.Name}, aliasNames(spec)...) {
				assert.False(t, spellings[spelling], "%s: %q spells two options", meta.ID, spelling)
				spellings[spelling] = true
				assert.Regexp(t, optionNamePattern, spelling, label)
			}
			for _, alias := range spec.Aliases {
				if alias.Inverted {
					assert.Equal(t, lint.OptionBoolean, spec.Type, "%s: only a boolean can be inverted", label)
				}
			}

			assert.NotEmpty(t, spec.Description, label)
			assert.True(t, strings.HasSuffix(spec.Description, "."), "%s: the description ends with a period", label)
			assert.NotContains(t, spec.Description, "`", "%s: the description is plain text", label)
			assert.NotContains(t, spec.Description, "—", "%s: no em dash", label)
		}
	}

	assert.ElementsMatch(t, ids, slices.Collect(maps.Keys(catalogOptions())), "options are declared for the catalog's rules")
	assert.Equal(t, []string{"MD047 max_trailing_blank_lines"}, unsetDefaults,
		"only an option that turns on a check of its own has no default")
}

// aliasNames returns the names of the aliases of spec.
func aliasNames(spec lint.OptionSpec) []string {
	names := make([]string, len(spec.Aliases))
	for idx, alias := range spec.Aliases {
		names[idx] = alias.Name
	}
	return names
}

// optionConfigs returns the option settings under which
// TestCatalogDeclaresEveryOptionRead runs a rule: none, the declared
// defaults, the defaults with every boolean flipped, which reaches options
// that only a boolean turns on, and the rule's options from the sidecar of
// a golden case, when it has one.
func optionConfigs(t *testing.T, specs []lint.OptionSpec, sidecar map[string]any) []map[string]any {
	t.Helper()

	defaults := declaredDefaults(t, specs, nil)
	flipped := maps.Clone(defaults)
	for _, spec := range specs {
		if enabled, ok := spec.Default.(bool); ok {
			flipped[spec.Name] = !enabled
		}
	}
	configs := []map[string]any{nil, defaults, flipped}
	if sidecar != nil {
		configs = append(configs, sidecar)
	}
	return configs
}

// declaredDefaults returns options that set every option of specs to its
// declared default, in the form a configuration file holds values, with
// set, canonical options, overriding them. An option without a default is
// left out, and so is one that takes its default from an option that set
// sets.
func declaredDefaults(t *testing.T, specs []lint.OptionSpec, set map[string]any) map[string]any {
	t.Helper()

	options := make(map[string]any, len(specs))
	for _, spec := range specs {
		if spec.Default == nil {
			continue
		}
		if _, sourceSet := set[spec.DefaultFrom]; spec.DefaultFrom != "" && sourceSet {
			continue
		}
		options[spec.Name] = configFileValue(t, spec.Default)
	}
	maps.Copy(options, set)
	return options
}

// configFileValue returns value as a YAML configuration file holds it,
// such as a []string as a []any.
func configFileValue(t *testing.T, value any) any {
	t.Helper()

	encoded, err := yaml.Marshal(value)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, yaml.Unmarshal(encoded, &decoded))
	return decoded
}

// TestCatalogDeclaresEveryOptionRead runs every rule on its golden inputs
// and on the real-world ones, under the settings of optionConfigs, and
// records the options it reads. Every option a rule reads must be declared
// under that canonical name, and every declared option must be read.
func TestCatalogDeclaresEveryOptionRead(t *testing.T) {
	baseDir := testdataDir(t)
	cases := discoverTestCases(t, baseDir)

	for _, meta := range Catalog() {
		rule, ok := lint.DefaultRegistry.GetByID(meta.ID)
		require.True(t, ok, meta.ID)

		declared := make(map[string]bool, len(meta.Options))
		for _, spec := range meta.Options {
			declared[spec.Name] = true
		}
		read := make(map[string]bool)

		for _, tc := range cases {
			if tc.RuleID != meta.ID && !tc.IsRealWorld {
				continue
			}
			cfg, err := loadGoldenConfig(tc.ConfigPath)
			require.NoError(t, err, tc.Name)
			content, err := os.ReadFile(tc.InputPath)
			require.NoError(t, err, tc.Name)
			snapshot, err := goldmark.New(string(cfg.Flavor)).Parse(context.Background(), tc.InputPath, content)
			require.NoError(t, err, tc.Name)

			for _, options := range optionConfigs(t, meta.Options, cfg.Rules[meta.ID].Options) {
				normalized, _, _ := lint.DefaultRegistry.NormalizeOptions(meta.ID, options)
				ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, &config.RuleConfig{Options: normalized})
				ruleCtx.Registry = lint.DefaultRegistry
				ruleCtx.RecordOptionReads(func(key string) { read[key] = true })
				_, err := rule.Apply(ruleCtx)
				require.NoError(t, err, "%s on %s", meta.ID, tc.Name)
			}
		}

		for _, key := range slices.Sorted(maps.Keys(read)) {
			assert.True(t, declared[key], "%s reads option %q, which its catalog entry does not declare", meta.ID, key)
		}
		for _, key := range slices.Sorted(maps.Keys(declared)) {
			assert.True(t, read[key], "%s declares option %q, which it never read", meta.ID, key)
		}
	}
}

// TestCatalogDefaultsMatchCode runs every golden case twice: as it is, and
// with every option of its rules set explicitly to its declared default,
// and requires the same findings and fixed text. A declared default that
// differs from the one the rule uses changes the result.
func TestCatalogDefaultsMatchCode(t *testing.T) {
	for _, tc := range discoverTestCases(t, testdataDir(t)) {
		t.Run(tc.Name, func(t *testing.T) {
			run := mustGoldenRun(t, tc)
			content, err := os.ReadFile(tc.InputPath)
			require.NoError(t, err)

			explicit := run
			explicit.cfg = run.cfg.Clone()
			for _, rule := range run.rules {
				meta, ok := lint.DefaultRegistry.Meta(rule.ID())
				require.True(t, ok, rule.ID())
				require.True(t, meta.OptionsDeclared, "%s declares its options", rule.ID())
				ruleCfg := explicit.cfg.Rules[rule.ID()]
				set, _, _ := lint.DefaultRegistry.NormalizeOptions(rule.ID(), ruleCfg.Options)
				ruleCfg.Options = declaredDefaults(t, meta.Options, set)
				explicit.cfg.Rules[rule.ID()] = ruleCfg
			}

			want := run.mustLintFile(t, tc.InputPath, content)
			got := explicit.mustLintFile(t, tc.InputPath, content)
			assert.Equal(t, want.Diagnostics, got.Diagnostics, "findings")

			wantFixed, _ := run.mustFixFile(t, tc.InputPath, content)
			gotFixed, _ := explicit.mustFixFile(t, tc.InputPath, content)
			assert.Equal(t, string(wantFixed), string(gotFixed), "fixed text")
		})
	}
}
