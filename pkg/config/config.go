// Package config defines core configuration types for gomdlint.
// These types are pure data structures with no external dependencies on Viper or other config loaders.
package config

import "slices"

// Severity represents the severity level of a lint diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// RuleConfig holds the settings of one rule: whether it runs, its severity,
// whether --fix applies its fixes, and its options.
//
// A configuration file may give a rule markdownlint's shorthand instead of
// a mapping: true or false, which sets enabled, or a string, which sets
// the severity (see UnmarshalYAML).
//
// A configuration file may write the options under the options key, the
// shape that gomdlint writes, or directly under the rule key, the flat
// shape that markdownlint uses, or both. Only the options that a rule
// declares tell which keys directly under the rule key are options, so
// decoding (see UnmarshalYAML) reads the options under options only, and
// ReadFlatOptions reads the others from the parsed document, as FromYAML
// and the configuration loader do before the configuration is used.
type RuleConfig struct {
	Enabled  *bool          `mapstructure:"enabled" yaml:"enabled"`
	Severity *string        `mapstructure:"severity" yaml:"severity"`
	AutoFix  *bool          `mapstructure:"auto_fix" yaml:"auto_fix"`
	Options  map[string]any `mapstructure:"options" yaml:"options"`
}

// ruleSettingKeys are the keys that RuleConfig's fields read; see
// RuleSettingKeys.
//
//nolint:gochecknoglobals // Read-only lookup table.
var ruleSettingKeys = [...]string{"enabled", "severity", "auto_fix", "options"}

// RuleSettingKeys returns the keys of a rule's settings that RuleConfig's
// fields read, in their order: enabled, severity, auto_fix and options.
// Every other key under a rule is one of its options. The caller may keep
// the list.
func RuleSettingKeys() []string {
	return slices.Clone(ruleSettingKeys[:])
}

// isRuleSettingKey reports whether key is one of RuleSettingKeys.
func isRuleSettingKey(key string) bool {
	return slices.Contains(ruleSettingKeys[:], key)
}

// BackupsConfig controls backup behavior when fixing files.
type BackupsConfig struct {
	// Enabled turns backups on. Nil means the setting was not given, so a
	// configuration layer can turn backups off without the zero value of a
	// plain bool being mistaken for "not set". Use IsEnabled to read it.
	Enabled *bool  `mapstructure:"enabled" yaml:"enabled,omitempty"`
	Mode    string `mapstructure:"mode" yaml:"mode"` // "sidecar", "xdg", etc.
}

// IsEnabled reports whether backups are enabled. An unset value counts as
// disabled; NewConfig enables backups explicitly.
func (b BackupsConfig) IsEnabled() bool {
	return b.Enabled != nil && *b.Enabled
}

// Bool returns a pointer to value, for optional boolean settings.
func Bool(value bool) *bool {
	return &value
}

// OutputFormat specifies the output format for diagnostics.
type OutputFormat string

const (
	FormatText    OutputFormat = "text"
	FormatTable   OutputFormat = "table"
	FormatJSON    OutputFormat = "json"
	FormatSARIF   OutputFormat = "sarif"
	FormatDiff    OutputFormat = "diff"
	FormatSummary OutputFormat = "summary"
)

// RuleFormat controls how rule identifiers appear in output.
type RuleFormat string

const (
	RuleFormatName     RuleFormat = "name"     // "no-trailing-spaces"
	RuleFormatID       RuleFormat = "id"       // "MD009"
	RuleFormatCombined RuleFormat = "combined" // "MD009/no-trailing-spaces"
)

// IsValid returns true if the rule format is valid.
func (f RuleFormat) IsValid() bool {
	switch f {
	case RuleFormatName, RuleFormatID, RuleFormatCombined:
		return true
	default:
		return false
	}
}

// SummaryOrder controls the order of tables in summary output.
type SummaryOrder string

const (
	// SummaryOrderRules shows rules table first (default).
	SummaryOrderRules SummaryOrder = "rules"
	// SummaryOrderFiles shows files table first.
	SummaryOrderFiles SummaryOrder = "files"
)

// IsValid returns true if the summary order is valid.
func (s SummaryOrder) IsValid() bool {
	switch s {
	case SummaryOrderRules, SummaryOrderFiles:
		return true
	default:
		return false
	}
}

// Flavor specifies the Markdown flavor to use for parsing.
type Flavor string

const (
	FlavorCommonMark Flavor = "commonmark"
	FlavorGFM        Flavor = "gfm"
)

// Config is the root configuration structure for mdlint.
type Config struct {
	// Flavor specifies the Markdown flavor ("commonmark" or "gfm").
	Flavor Flavor `mapstructure:"flavor" yaml:"flavor"`

	// SeverityDefault is the severity for rules that don't specify one. It
	// replaces each rule's built-in default severity; empty keeps them.
	SeverityDefault string `mapstructure:"severity_default" yaml:"severity_default,omitempty"`

	// Rules contains per-rule configuration keyed by rule ID.
	Rules map[string]RuleConfig `mapstructure:"rules" yaml:"rules"`

	// RuleDefaults defines an explicit baseline before per-rule overrides.
	// Migration uses it to preserve markdownlint defaults and file directives.
	RuleDefaults map[string]RuleConfig `mapstructure:"rule_defaults" yaml:"rule_defaults,omitempty"`

	// Ignore contains glob patterns for files to ignore. Patterns from a
	// project or explicit config file are relative to that file's directory.
	Ignore []string `mapstructure:"ignore" yaml:"ignore"`

	// Gitignore skips files ignored by .gitignore files when walking
	// directories. Nil means unset; GitignoreEnabled treats it as disabled
	// and NewConfig enables it.
	Gitignore *bool `mapstructure:"gitignore" yaml:"gitignore,omitempty"`

	// DefaultIgnores skips node_modules and vendor directories when walking
	// directories. Nil means unset; DefaultIgnoresEnabled treats it as
	// disabled and NewConfig enables it.
	DefaultIgnores *bool `mapstructure:"default_ignores" yaml:"default_ignores,omitempty"`

	// Backups configures backup behavior when fixing.
	Backups BackupsConfig `mapstructure:"backups" yaml:"backups"`

	// CLI-level options (not persisted to config files).

	// Fix enables auto-fixing of issues.
	Fix bool `mapstructure:"-" yaml:"-"`

	// DryRun shows what would be fixed without making changes.
	DryRun bool `mapstructure:"-" yaml:"-"`

	// Format specifies the output format.
	Format OutputFormat `mapstructure:"-" yaml:"-"`

	// RuleFormat controls how human-readable output names rules. It comes
	// from --rule-format or GOMDLINT_RULE_FORMAT; config files cannot set it.
	RuleFormat RuleFormat `mapstructure:"-" yaml:"-"`

	// Jobs specifies the number of parallel workers.
	Jobs int `mapstructure:"-" yaml:"-"`

	// EnableRules contains rule IDs to explicitly enable.
	EnableRules []string `mapstructure:"-" yaml:"-"`

	// DisableRules contains rule IDs to explicitly disable.
	DisableRules []string `mapstructure:"-" yaml:"-"`

	// FixRules limits auto-fixing to specific rule IDs.
	FixRules []string `mapstructure:"-" yaml:"-"`

	// NoBackups disables backup creation when fixing.
	NoBackups bool `mapstructure:"-" yaml:"-"`
}

// NewConfig returns a Config with sensible defaults.
func NewConfig() *Config {
	return &Config{
		Flavor: FlavorGFM,
		// No SeverityDefault: rules keep their built-in severities.
		Rules:          make(map[string]RuleConfig),
		Ignore:         nil,
		Gitignore:      Bool(true),
		DefaultIgnores: Bool(true),
		Backups: BackupsConfig{
			Enabled: Bool(true),
			Mode:    "sidecar",
		},
		Format:     FormatText,
		RuleFormat: RuleFormatName,
		Jobs:       0, // 0 means use GOMAXPROCS
	}
}

// GitignoreEnabled reports whether directory walks skip files that
// .gitignore files ignore.
func (c *Config) GitignoreEnabled() bool {
	return c.Gitignore != nil && *c.Gitignore
}

// DefaultIgnoresEnabled reports whether directory walks skip node_modules
// and vendor directories.
func (c *Config) DefaultIgnoresEnabled() bool {
	return c.DefaultIgnores != nil && *c.DefaultIgnores
}
