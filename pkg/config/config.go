// Package config defines core configuration types for gomdlint.
// These types are pure data structures with no external dependencies on Viper or other config loaders.
package config

// Severity represents the severity level of a lint diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// RuleConfig holds per-rule configuration options.
type RuleConfig struct {
	Enabled  *bool          `mapstructure:"enabled" yaml:"enabled"`
	Severity *string        `mapstructure:"severity" yaml:"severity"`
	AutoFix  *bool          `mapstructure:"auto_fix" yaml:"auto_fix"`
	Options  map[string]any `mapstructure:"options" yaml:"options"`
}

// BackupsConfig controls backup behavior when fixing files.
type BackupsConfig struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Mode    string `mapstructure:"mode" yaml:"mode"` // "sidecar", "xdg", etc.
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

	// SeverityDefault is the default severity for rules that don't specify one.
	SeverityDefault string `mapstructure:"severity_default" yaml:"severity_default"`

	// Rules contains per-rule configuration keyed by rule ID.
	Rules map[string]RuleConfig `mapstructure:"rules" yaml:"rules"`

	// Ignore contains glob patterns for files to ignore.
	Ignore []string `mapstructure:"ignore" yaml:"ignore"`

	// Backups configures backup behavior when fixing.
	Backups BackupsConfig `mapstructure:"backups" yaml:"backups"`

	// CLI-level options (not persisted to config files).

	// Fix enables auto-fixing of issues.
	Fix bool `mapstructure:"-" yaml:"-"`

	// DryRun shows what would be fixed without making changes.
	DryRun bool `mapstructure:"-" yaml:"-"`

	// Format specifies the output format.
	Format OutputFormat `mapstructure:"-" yaml:"-"`

	// RuleFormat controls how rule identifiers appear in output.
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
		Flavor:          FlavorCommonMark,
		SeverityDefault: string(SeverityWarning),
		Rules:           make(map[string]RuleConfig),
		Ignore:          nil,
		Backups: BackupsConfig{
			Enabled: true,
			Mode:    "sidecar",
		},
		Format:     FormatText,
		RuleFormat: RuleFormatName,
		Jobs:       0, // 0 means use GOMAXPROCS
	}
}
