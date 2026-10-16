package rules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// goldenConfigSuffix is the file name suffix of a fixture's optional sidecar
// configuration, which sits next to the input:
// testdata/<RULE_ID>/<scenario>.config.yml.
const goldenConfigSuffix = ".config.yml"

// goldenSidecar is the schema of a sidecar file. It is a strict subset of the
// .gomdlint.yml schema, so a sidecar is also a valid gomdlint config file.
type goldenSidecar struct {
	// Flavor selects the Markdown flavor used to parse the fixture and seen
	// by rules through the configuration. Empty keeps the product default.
	Flavor config.Flavor `yaml:"flavor"`

	// Rules holds rule options keyed by rule ID.
	Rules map[string]goldenSidecarRule `yaml:"rules"`
}

// goldenSidecarRule holds the options for one rule.
type goldenSidecarRule struct {
	Options map[string]any `yaml:"options"`
}

// loadGoldenConfig builds the configuration for one fixture from its sidecar.
// Without a sidecar it returns the product defaults from config.NewConfig, so
// fixtures follow the product's default flavor. Unknown keys, unknown rules
// and settings the harness cannot honour are errors, not silent no-ops.
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

	for key, rule := range sidecar.Rules {
		if _, ok := lint.DefaultRegistry.GetByID(key); !ok {
			if ruleID, _, found := lint.DefaultRegistry.Resolve(key); found {
				return nil, fmt.Errorf("sidecar %s: key rules by ID, use %s instead of %q", path, ruleID, key)
			}
			return nil, fmt.Errorf("sidecar %s: unknown rule %q", path, key)
		}
		if len(rule.Options) == 0 {
			return nil, fmt.Errorf("sidecar %s: rules.%s sets no options", path, key)
		}
		cfg.Rules[key] = config.RuleConfig{Options: rule.Options}
	}

	return cfg, nil
}

// decodeGoldenSidecar decodes exactly one YAML document, rejecting unknown keys.
// Option values decode into the same Go types as in the product's YAML loader.
func decodeGoldenSidecar(data []byte) (goldenSidecar, error) {
	var sidecar goldenSidecar

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err := decoder.Decode(&sidecar)
	if errors.Is(err, io.EOF) {
		return sidecar, errors.New("sidecar is empty, delete it or set flavor or rules")
	}
	if err != nil {
		return sidecar, fmt.Errorf("parse: %w", err)
	}

	var extra yaml.Node
	switch err := decoder.Decode(&extra); {
	case err == nil:
		return sidecar, errors.New("sidecar must hold a single YAML document")
	case !errors.Is(err, io.EOF):
		return sidecar, fmt.Errorf("parse: %w", err)
	}

	if sidecar.Flavor == "" && len(sidecar.Rules) == 0 {
		return sidecar, errors.New("sidecar is empty, delete it or set flavor or rules")
	}

	return sidecar, nil
}

// goldenEnv is the configuration and parser for one golden fixture.
type goldenEnv struct {
	cfg    *config.Config
	parser *goldmark.Parser
}

// newGoldenEnv loads the fixture's sidecar and builds a parser for its flavor.
func newGoldenEnv(t *testing.T, tc GoldenTestCase) goldenEnv {
	t.Helper()

	cfg, err := loadGoldenConfig(tc.ConfigPath)
	require.NoError(t, err)

	return goldenEnv{cfg: cfg, parser: goldmark.New(string(cfg.Flavor))}
}

// parse parses content with the fixture's flavor.
func (env goldenEnv) parse(t *testing.T, name string, content []byte) *mdast.FileSnapshot {
	t.Helper()

	snapshot, err := env.parser.Parse(context.Background(), name, content)
	require.NoError(t, err, "failed to parse %s", name)

	return snapshot
}

// applyRule runs one rule with the fixture's configuration and returns its
// diagnostics as the engine reports them. Like lint.Engine.LintFile, it passes
// the rule its configured options, fills in the rule name, and sets the
// resolved severity, which is the rule default because sidecars cannot
// configure severity. Every diagnostic must carry the rule's ID and name.
func (env goldenEnv) applyRule(t *testing.T, rule lint.Rule, snapshot *mdast.FileSnapshot) []lint.Diagnostic {
	t.Helper()

	var ruleCfg *config.RuleConfig
	if configured, ok := env.cfg.Rules[rule.ID()]; ok {
		ruleCfg = &configured
	}

	ruleCtx := lint.NewRuleContext(context.Background(), snapshot, env.cfg, ruleCfg)
	ruleCtx.Registry = lint.DefaultRegistry

	diags, err := rule.Apply(ruleCtx)
	require.NoError(t, err, "rule %s failed to apply", rule.ID())

	for idx := range diags {
		if diags[idx].RuleName == "" {
			diags[idx].RuleName = rule.Name()
		}
		diags[idx].Severity = rule.DefaultSeverity()
	}
	require.NoError(t, checkDiagIdentity(rule, diags))

	return diags
}

// checkDiagIdentity returns an error when a diagnostic does not carry the ID
// and name of the rule that produced it.
func checkDiagIdentity(rule lint.Rule, diags []lint.Diagnostic) error {
	for idx, diag := range diags {
		if diag.RuleID != rule.ID() || diag.RuleName != rule.Name() {
			return fmt.Errorf("diagnostic %d from %s/%s is labelled %s/%s",
				idx, rule.ID(), rule.Name(), diag.RuleID, diag.RuleName)
		}
	}

	return nil
}

// idleRuleConfigs returns the sorted IDs of configured rules that are not in
// rules. Their options would have no effect on the case.
func (env goldenEnv) idleRuleConfigs(rules []lint.Rule) []string {
	var idle []string
	for ruleID := range env.cfg.Rules {
		runs := slices.ContainsFunc(rules, func(rule lint.Rule) bool {
			return rule.ID() == ruleID
		})
		if !runs {
			idle = append(idle, ruleID)
		}
	}
	slices.Sort(idle)

	return idle
}

// requireRulesRun fails the test when the sidecar configures a rule that the
// case does not run.
func (env goldenEnv) requireRulesRun(t *testing.T, tc GoldenTestCase, rules []lint.Rule) {
	t.Helper()

	require.Empty(t, env.idleRuleConfigs(rules),
		"sidecar %s configures rules that this case does not run", tc.ConfigPath)
}
