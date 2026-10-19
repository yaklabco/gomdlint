package rules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// goldenConfigSuffix is the file name suffix of a fixture's optional sidecar
// configuration, which sits next to the input:
// testdata/<RULE_ID>/<scenario>.config.yml.
const goldenConfigSuffix = ".config.yml"

var (
	// errEmptySidecar reports a sidecar that configures nothing.
	errEmptySidecar = errors.New("sidecar sets neither a flavor nor rule options")

	// errMultipleDocuments reports a sidecar with more than one YAML document.
	errMultipleDocuments = errors.New("sidecar holds more than one YAML document")
)

// goldenSidecar is the schema of a sidecar file. It is a strict subset of the
// .gomdlint.yml schema, so a sidecar is also a valid gomdlint config file.
type goldenSidecar struct {
	// Flavor selects the Markdown flavor used to parse the fixture and seen
	// by rules through the configuration. Empty keeps the product default.
	Flavor config.Flavor `yaml:"flavor"`

	// Rules holds rule options keyed by rule ID. They read as a config
	// file's do, so they may sit under options, directly under the rule ID
	// as markdownlint writes them, or both (see decodeGoldenSidecar).
	// loadGoldenConfig rejects the other rule settings.
	Rules map[string]config.RuleConfig `yaml:"rules"`

	// settings holds, for each rule key, the first rule setting other than
	// options that the sidecar writes for it (see writtenRuleSettings).
	settings map[string]string
}

// loadGoldenConfig builds the configuration for one fixture from its sidecar.
// Without a sidecar it returns the product defaults from config.NewConfig, so
// fixtures follow the product's default flavor. Unknown keys, unknown rules,
// options that the product would ignore and settings the harness cannot
// honour are errors, not silent no-ops.
func loadGoldenConfig(path string) (*config.Config, error) {
	cfg := config.NewConfig()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sidecar: %w", err)
	}

	sidecar, err := decodeGoldenSidecar(data)
	if err != nil {
		return nil, fmt.Errorf("sidecar %s: %w", path, err)
	}

	switch sidecar.Flavor {
	case "":
	case config.FlavorCommonMark, config.FlavorGFM:
		cfg.Flavor = sidecar.Flavor
	default:
		return nil, fmt.Errorf("sidecar %s: invalid flavor %q, want %q or %q",
			path, sidecar.Flavor, config.FlavorCommonMark, config.FlavorGFM)
	}

	for _, key := range slices.Sorted(maps.Keys(sidecar.Rules)) {
		rule, ok := lint.DefaultRegistry.GetByID(key)
		if !ok {
			if ruleID, _, found := lint.DefaultRegistry.Resolve(key); found {
				return nil, fmt.Errorf("sidecar %s: rules.%s: use the rule ID %s, not the name", path, key, ruleID)
			}
			return nil, fmt.Errorf("sidecar %s: rules.%s: unknown rule", path, key)
		}
		if setting := sidecar.settings[key]; setting != "" {
			return nil, fmt.Errorf("sidecar %s: rules.%s.%s: a sidecar sets rule options only", path, key, setting)
		}
		options := sidecar.Rules[key].Options
		if len(options) == 0 {
			return nil, fmt.Errorf("sidecar %s: rules.%s sets no options", path, key)
		}
		if err := checkSidecarOptions(rule, options); err != nil {
			return nil, fmt.Errorf("sidecar %s: rules.%s: %w", path, key, err)
		}
		cfg.Rules[key] = config.RuleConfig{Options: options}
	}

	return cfg, nil
}

// writtenRuleSettings returns, for each rule key of doc, the parsed
// sidecar, the first rule setting other than options that it writes for
// the rule, whatever its value: the harness runs a case's rules itself,
// with fixes on and their own severities, so it cannot honour one, and a
// setting that is null is written all the same. Keys count as decoding
// takes them, after merge keys. A bare true or false is markdownlint's
// shorthand for enabled, and a bare string its shorthand for enabled: true
// at that severity, which this reports as severity, the setting it names.
func writtenRuleSettings(doc *yaml.Node) (map[string]string, error) {
	var shape struct {
		Rules map[string]yaml.Node `yaml:"rules"`
	}
	if err := doc.Decode(&shape); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	settings := make(map[string]string)
	for key, node := range shape.Rules {
		target := &node
		for target.Kind == yaml.AliasNode {
			target = target.Alias
		}
		switch target.Kind {
		case yaml.ScalarNode:
			switch target.ShortTag() {
			case "!!bool":
				settings[key] = "enabled"
			case "!!str":
				settings[key] = "severity"
			}
		case yaml.MappingNode:
			var written map[string]yaml.Node
			if err := target.Decode(&written); err != nil {
				return nil, fmt.Errorf("parse: %w", err)
			}
			for _, setting := range []string{"enabled", "severity", "auto_fix"} {
				if _, ok := written[setting]; ok {
					settings[key] = setting
					break
				}
			}
		}
	}
	return settings, nil
}

// checkSidecarOptions checks options, the options of rule in a sidecar,
// against the options that the rule's catalog entry declares. It returns
// the first problem, in order of key, that would make the product ignore
// part of them or fail to load them: an option that the rule does not
// read, a value or a list item of the wrong type, or two spellings of one
// option. A value that the option does not allow is kept, because rules
// read such a value as markdownlint does, so a scenario may check one.
func checkSidecarOptions(rule lint.Rule, options map[string]any) error {
	_, issues, declared := lint.DefaultRegistry.NormalizeOptions(rule.ID(), options)
	if !declared {
		return fmt.Errorf("%s declares no options to check them against", rule.ID())
	}
	for _, issue := range issues {
		if issue.Kind != lint.OptionIssueNotAllowed {
			return errors.New(issue.Text(rule.Name()))
		}
	}
	return nil
}

// decodeGoldenSidecar decodes exactly one YAML document, rejecting unknown keys
// outside the rules. The settings of each rule read as a config file's do,
// and every key under a rule ID that is no rule setting is read as an
// option (see config.ReadFlatOptions), so that loadGoldenConfig rejects
// those the rule does not read. A key that a YAML alias repeats is an
// error, as in a config file (see config.ExpandAliasKeys). Option values
// decode into the same Go types as in the product's YAML loader.
//
// The keys of a sidecar with alias keys are checked before, and apart
// from, the decode that rejects unknown keys, since yaml.v3 offers that
// check to a decoder of bytes alone. So a sidecar that repeats a key
// through an alias fails without the errors for its unknown keys, which
// its literal twin also gets. That decode reads the sidecar as written, so
// it compares alias keys by anchor name and rejects two aliases of one
// anchor name that name two keys, which a config file accepts. A sidecar
// has no use for either.
func decodeGoldenSidecar(data []byte) (goldenSidecar, error) {
	// The keys are checked first, as decoding a config file checks them,
	// with the alias keys expanded, when there are any.
	var parsed yaml.Node
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return goldenSidecar{}, fmt.Errorf("parse: %w", err)
	}
	doc := config.ExpandAliasKeys(&parsed)
	if doc != &parsed {
		if err := doc.Decode(new(goldenSidecar)); err != nil {
			return goldenSidecar{}, fmt.Errorf("parse: %w", err)
		}
	}

	var sidecar goldenSidecar

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(&sidecar)
	if errors.Is(err, io.EOF) {
		return goldenSidecar{}, errEmptySidecar
	}
	if err != nil {
		return goldenSidecar{}, fmt.Errorf("parse: %w", err)
	}

	var extra yaml.Node
	switch err := decoder.Decode(&extra); {
	case err == nil:
		return goldenSidecar{}, errMultipleDocuments
	case !errors.Is(err, io.EOF):
		return goldenSidecar{}, fmt.Errorf("parse: %w", err)
	}

	if sidecar.Flavor == "" && len(sidecar.Rules) == 0 {
		return goldenSidecar{}, errEmptySidecar
	}

	// sidecar.Rules came from the bytes as written, and doc has its alias
	// keys expanded, but both take the same keys: the decode of doc above
	// rejected every key that an alias repeats, and the decode of the bytes
	// every alias that repeats an anchor name.
	rules := config.Config{Rules: sidecar.Rules}
	if err := config.ReadFlatOptions(doc, &rules, func(string, string) bool { return true }); err != nil {
		return goldenSidecar{}, fmt.Errorf("parse: %w", err)
	}
	if sidecar.settings, err = writtenRuleSettings(doc); err != nil {
		return goldenSidecar{}, err
	}

	return sidecar, nil
}

// goldenRun holds what one golden case needs to lint a document: the
// configuration from its sidecar, a parser for its flavor, the registry, and
// the rules the case runs.
type goldenRun struct {
	cfg      *config.Config
	parser   *goldmark.Parser
	registry *lint.Registry
	rules    []lint.Rule
}

// newGoldenRun prepares a case. A per-rule case runs its directory's rule,
// even when that rule is disabled by default, and a real-world case runs the
// default-enabled rules. No other rule can report, even when a directive
// enables it. A sidecar that configures a rule the case does not run is an
// error, because those options would have no effect.
func newGoldenRun(tc GoldenTestCase) (goldenRun, error) {
	var rules []lint.Rule
	switch {
	case tc.RuleID != "":
		rule, ok := lint.DefaultRegistry.GetByID(tc.RuleID)
		if !ok {
			return goldenRun{}, fmt.Errorf("rule %s not found in registry", tc.RuleID)
		}
		rules = []lint.Rule{rule}
	case tc.IsRealWorld:
		rules = defaultEnabledRules(lint.DefaultRegistry)
	default:
		return goldenRun{}, fmt.Errorf("case %s is neither a per-rule nor a real-world case", tc.Name)
	}

	cfg, err := loadGoldenConfig(tc.ConfigPath)
	if err != nil {
		return goldenRun{}, err
	}

	run := goldenRun{
		cfg:      cfg,
		parser:   goldmark.New(string(cfg.Flavor)),
		registry: isolatedRegistry(rules),
		rules:    rules,
	}
	if unused := run.unusedRuleConfigs(); len(unused) > 0 {
		return goldenRun{}, fmt.Errorf("sidecar %s configures rules the case does not run: %s",
			tc.ConfigPath, strings.Join(unused, ", "))
	}

	return run, nil
}

// lintFile lints content the way the product lints one file: through
// lint.Engine.LintFile, with fixes on and only the case's rules enabled. The
// engine applies inline directives, resolves severity, fills in rule names
// and prepares the fix edits. Rule errors fail the case, and so does a
// diagnostic that does not carry the ID and name of one of the case's rules.
func (run goldenRun) lintFile(name string, content []byte) (*lint.FileResult, error) {
	cfg := run.fixConfig()
	result, err := lint.NewEngine(run.parser, run.registry).LintFile(context.Background(), name, content, cfg)
	if err != nil {
		return nil, fmt.Errorf("lint %s: %w", name, err)
	}
	if len(result.RuleErrors) > 0 {
		return nil, fmt.Errorf("lint %s: rule errors: %v", name, result.RuleErrors)
	}
	if err := checkDiagIdentity(run.rules, result.Diagnostics); err != nil {
		return nil, fmt.Errorf("lint %s: %w", name, err)
	}

	return result, nil
}

// fixFile fixes content the way --fix does: through lint.Pipeline, which
// applies the engine's fixes pass by pass and lints the fixed content
// again after each pass, with the configuration and rules of lintFile. One
// engine pass need not apply every fix, since the engine leaves some fixes
// for a later pass. It returns the fixed content and the pipeline result,
// whose diagnostics describe the fixed content. Rule errors and a refused
// fix fail the case.
func (run goldenRun) fixFile(name string, content []byte) ([]byte, *lint.PipelineResult, error) {
	cfg := run.fixConfig()
	result, err := lint.NewPipeline(lint.NewEngine(run.parser, run.registry)).ProcessContent(
		context.Background(), name, content, cfg, lint.PipelineOptions{Fix: true})
	if err != nil {
		return nil, nil, fmt.Errorf("fix %s: %w", name, err)
	}
	if len(result.RuleErrors) > 0 {
		return nil, nil, fmt.Errorf("fix %s: rule errors: %v", name, result.RuleErrors)
	}
	if result.Skipped {
		return nil, nil, fmt.Errorf("fix %s: %s", name, result.SkipReason)
	}

	if !result.Modified {
		return content, result, nil
	}
	return result.ModifiedContent, result, nil
}

// fixConfig returns the configuration of the case with fixes on and only
// the case's rules enabled.
func (run goldenRun) fixConfig() *config.Config {
	enabled := make([]string, 0, len(run.rules))
	for _, rule := range run.rules {
		enabled = append(enabled, rule.ID())
	}
	var disabled []string
	for _, ruleID := range run.registry.IDs() {
		if !slices.Contains(enabled, ruleID) {
			disabled = append(disabled, ruleID)
		}
	}

	cfg := *run.cfg
	cfg.Fix = true
	cfg.EnableRules = enabled
	cfg.DisableRules = disabled
	return &cfg
}

// mustGoldenRun prepares a case, failing the test on error.
func mustGoldenRun(t *testing.T, tc GoldenTestCase) goldenRun {
	t.Helper()

	run, err := newGoldenRun(tc)
	require.NoError(t, err)

	return run
}

// mustLintFile lints content, failing the test on error.
func (run goldenRun) mustLintFile(t *testing.T, name string, content []byte) *lint.FileResult {
	t.Helper()

	result, err := run.lintFile(name, content)
	require.NoError(t, err)
	if len(result.SkippedEdits) > 0 {
		t.Logf("Note: %d edits skipped due to conflicts", len(result.SkippedEdits))
	}

	return result
}

// mustFixFile fixes content, failing the test on error.
func (run goldenRun) mustFixFile(t *testing.T, name string, content []byte) ([]byte, *lint.PipelineResult) {
	t.Helper()

	fixed, result, err := run.fixFile(name, content)
	require.NoError(t, err)

	return fixed, result
}

// checkDiagIdentity returns an error when a diagnostic does not carry the ID
// and name of one of the given rules.
func checkDiagIdentity(rules []lint.Rule, diags []lint.Diagnostic) error {
	for idx, diag := range diags {
		known := slices.ContainsFunc(rules, func(rule lint.Rule) bool {
			return rule.ID() == diag.RuleID && rule.Name() == diag.RuleName
		})
		if !known {
			return fmt.Errorf("diagnostic %d is labelled %s/%s, which is not a rule of this case",
				idx, diag.RuleID, diag.RuleName)
		}
	}

	return nil
}

// unusedRuleConfigs returns the sorted IDs of configured rules that the case
// does not run.
func (run goldenRun) unusedRuleConfigs() []string {
	var unused []string
	for ruleID := range run.cfg.Rules {
		runs := slices.ContainsFunc(run.rules, func(rule lint.Rule) bool {
			return rule.ID() == ruleID
		})
		if !runs {
			unused = append(unused, ruleID)
		}
	}
	slices.Sort(unused)

	return unused
}

// inertRule stands in for a rule outside the case. It keeps the rule's ID,
// name, tags and defaults, so directives resolve as in the product, but it
// never reports anything.
type inertRule struct {
	lint.Rule
}

// Apply reports nothing.
func (inertRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	return nil, nil
}

// isolatedRegistry returns a registry with the rules and catalog entries of
// lint.DefaultRegistry, in which every rule outside rules is an inert
// stand-in. Inline directives resolve exactly as in the product, but even a
// bare markdownlint-enable cannot make another rule report.
func isolatedRegistry(rules []lint.Rule) *lint.Registry {
	registry := lint.NewRegistry()
	for _, rule := range lint.DefaultRegistry.Rules() {
		inCase := slices.ContainsFunc(rules, func(caseRule lint.Rule) bool {
			return caseRule.ID() == rule.ID()
		})
		if inCase {
			registry.Register(rule)
		} else {
			registry.Register(inertRule{Rule: rule})
		}
		if meta, ok := lint.DefaultRegistry.Meta(rule.ID()); ok {
			registry.Describe(meta)
		}
	}

	return registry
}

// defaultEnabledRules returns the registry's rules that are enabled by default.
func defaultEnabledRules(registry *lint.Registry) []lint.Rule {
	var enabled []lint.Rule
	for _, rule := range registry.Rules() {
		if rule.DefaultEnabled() {
			enabled = append(enabled, rule)
		}
	}

	return enabled
}
