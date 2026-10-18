package configloader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// loadConfigContent loads content as the config file named name, given
// with --config, with nothing else: no user, system, markdownlint or
// environment settings. The file must load without warnings.
func loadConfigContent(t *testing.T, name string, content []byte) *config.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, content, 0o600))
	result, err := Load(context.Background(), LoadOptions{
		WorkingDir: filepath.Dir(path), ExplicitPath: path, IgnoreEnv: true,
		IgnoreUserConfig: true, IgnoreSystemConfig: true, IgnoreMarkdownlint: true, NonInteractive: true,
	})
	require.NoError(t, err, string(content))
	assert.Empty(t, result.Warnings, string(content))
	return result.Config
}

// loadMarkdownlintForRun loads the markdownlint config at path the way
// lint applies a project's markdownlint config for a run, translated, with
// no user, system or environment settings.
func loadMarkdownlintForRun(t *testing.T, path string) *config.Config {
	t.Helper()

	result, err := Load(context.Background(), LoadOptions{
		WorkingDir: filepath.Dir(path), IgnoreEnv: true, IgnoreUserConfig: true, IgnoreSystemConfig: true,
		NonInteractive: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{path}, result.LoadedFrom, "the markdownlint config is the only layer")
	return result.Config
}

// configShapes returns the ways of writing the settings of the markdownlint
// config at path for gomdlint, each loaded: the markdownlint config itself,
// applied for a run, the gomdlint config that migrate writes from it, with
// each rule's options under options, and that config in the flat shape, as
// YAML with gomdlint's names and as JSON with markdownlint's names for the
// rules and their options.
func configShapes(t *testing.T, path string) map[string]*config.Config {
	t.Helper()

	converted, err := ConvertMarkdownlintConfig(path)
	require.NoError(t, err)
	native, err := converted.FileContent(gomdlintConfigFiles[0])
	require.NoError(t, err)

	return map[string]*config.Config{
		"the markdownlint config applied for a run": loadMarkdownlintForRun(t, path),
		"the migrated config":                       loadConfigContent(t, ".gomdlint.yml", native),
		"the migrated config in the flat shape":     loadConfigContent(t, ".gomdlint.yml", flatShape(t, native, false, false)),
		"the migrated config in the flat shape, in markdownlint's words, as JSON": loadConfigContent(
			t, ".gomdlint.json", flatShape(t, native, true, true)),
	}
}

// flatShape rewrites content, a gomdlint config file in YAML that writes
// each rule's options under options, as migrate does, into the flat shape,
// with each option directly under its rule key. With markdownlintWords, the
// rule keys become markdownlint's names for the rules, and options take
// markdownlint's spellings, an inverted spelling with the opposite value,
// so that the file writes rules as markdownlint's documentation does. It
// writes JSON when asJSON is set, and YAML otherwise.
func flatShape(t *testing.T, content []byte, markdownlintWords, asJSON bool) []byte {
	t.Helper()

	var file map[string]map[string]map[string]any
	require.NoError(t, yaml.Unmarshal(content, &file), string(content))
	flat := make(map[string]map[string]map[string]any, len(file))
	for section, settings := range file {
		flat[section] = make(map[string]map[string]any, len(settings))
		for key, setting := range settings {
			options, isMapping := setting["options"].(map[string]any)
			require.True(t, isMapping || setting["options"] == nil, "%s: options must be a mapping", key)
			delete(setting, "options")
			for name, value := range options {
				if markdownlintWords {
					name, value = markdownlintSpelling(t, key, name, value)
				}
				require.NotContains(t, setting, name, "%s option %s is spelled like a rule setting", key, name)
				setting[name] = value
			}
			if markdownlintWords {
				key = markdownlintRuleName(t, key)
			}
			flat[section][key] = setting
		}
	}

	if asJSON {
		encoded, err := json.MarshalIndent(flat, "", "  ")
		require.NoError(t, err)
		return encoded
	}
	encoded, err := yaml.Marshal(flat)
	require.NoError(t, err)
	return encoded
}

// markdownlintSpelling returns markdownlint's spelling of the option name of
// the rule with the given ID, where it has one, with value negated for an
// inverted spelling, and otherwise name and value as they are.
func markdownlintSpelling(t *testing.T, ruleID, name string, value any) (string, any) {
	t.Helper()

	meta, ok := lint.DefaultRegistry.Meta(ruleID)
	require.True(t, ok, ruleID)
	for _, spec := range meta.Options {
		if spec.Name != name || len(spec.Aliases) == 0 {
			continue
		}
		alias := spec.Aliases[0]
		if enabled, isBool := value.(bool); isBool && alias.Inverted {
			value = !enabled
		}
		return alias.Name, value
	}
	return name, value
}

// markdownlintRuleName returns the name that markdownlint lists first for
// the rule with the given ID, or gomdlint's name for a rule that
// markdownlint lacks.
func markdownlintRuleName(t *testing.T, ruleID string) string {
	t.Helper()

	meta, ok := lint.DefaultRegistry.Meta(ruleID)
	require.True(t, ok, ruleID)
	if meta.MarkdownlintName != "" {
		return meta.MarkdownlintName
	}
	return meta.Name
}

// sampleDocument breaks many rules at once, so that linting it shows the
// rules that a configuration turns on and how their options set them.
const sampleDocument = "---\ntitle: Sample\n---\n" +
	"# Sample\n\n### Skipped level #\n\nTrailing spaces   \nand a hard\ttab, a line that is far longer than " +
	"twenty characters and longer than eighty characters too, by a long way.\n\n\n" +
	"* one\n+ two\n- three\n\n1. one\n1. two\n3. three\n\n" +
	"```\ncode without a language, long enough to pass any limit on the length of a line\n```\n\n" +
	"    indented code\n\n" +
	"<br>Some <sup>HTML</sup>.\n\n_emphasis_ and *emphasis*, __strong__ and **strong**.\n\n" +
	"Visit https://example.com or [here](https://example.com).\n\n" +
	"| a | b |\n|:-|-:|\n| 1 | 2 |\n\n" +
	">  Quoted with two spaces.\n\n***\n\n---\n\n## Sample\n\n## Ends with a colon:\n" +
	"Text right after a heading.\n"

// lintSample lints sampleDocument under cfg with the built-in rules.
func lintSample(t *testing.T, cfg *config.Config) []lint.Diagnostic {
	t.Helper()

	return lintWithConfig(t, cfg, sampleDocument)
}

// lintWithConfig lints input under cfg with the built-in rules.
func lintWithConfig(t *testing.T, cfg *config.Config, input string) []lint.Diagnostic {
	t.Helper()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	return result.Diagnostics
}

// TestMarkdownlintFixturesInEveryShape checks that each markdownlint config
// of #43's equivalence fixtures, which include the #19 regression
// fixtures, configures gomdlint the same way in every shape: applied for a
// run, migrated to options mappings, and migrated and then flattened, with
// gomdlint's names or with markdownlint's. Every rule runs with the same
// severity, fixes and option values, and a document that breaks many rules
// lints the same.
func TestMarkdownlintFixturesInEveryShape(t *testing.T) {
	t.Parallel()

	for _, fixture := range markdownlintFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			source := filepath.Join(t.TempDir(), fixture.file)
			require.NoError(t, os.WriteFile(source, []byte(fixture.content), 0o600))

			shapes := configShapes(t, source)
			reference := shapes["the markdownlint config applied for a run"]
			want := effectiveOptions(t, reference)
			wantDiagnostics := lintSample(t, reference)
			for name, cfg := range shapes {
				assert.Equal(t, want, effectiveOptions(t, cfg), name)
				assert.Equal(t, wantDiagnostics, lintSample(t, cfg), name)
			}
		})
	}
}
