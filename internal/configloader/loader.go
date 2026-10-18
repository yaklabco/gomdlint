// Package configloader provides configuration loading and resolution.
// It implements XDG-compliant configuration discovery, hierarchical merging,
// environment variable support, validation, and markdownlint migration.
package configloader

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// configFilePermissions is the file mode for configuration files (world-readable).
const configFilePermissions = 0644

// LoadOptions controls configuration loading behavior.
type LoadOptions struct {
	// WorkingDir is the directory to search from for project config.
	// Defaults to current working directory if empty.
	WorkingDir string

	// ExplicitPath is an explicit config file path (from --config flag).
	// If set, project config discovery is skipped.
	ExplicitPath string

	// IgnoreSystemConfig skips loading system-level configuration.
	IgnoreSystemConfig bool

	// IgnoreUserConfig skips loading user-level configuration.
	IgnoreUserConfig bool

	// IgnoreProjectConfig skips loading project-level configuration.
	IgnoreProjectConfig bool

	// IgnoreEnv skips loading environment variables.
	IgnoreEnv bool

	// IgnoreMarkdownlint skips markdownlint config detection and migration.
	IgnoreMarkdownlint bool

	// Verbose enables logging of configuration resolution steps.
	Verbose bool

	// NonInteractive disables interactive prompts (e.g., in CI).
	NonInteractive bool

	// Stdin and Stderr carry the interactive prompt that offers to save a
	// translated markdownlint config. The prompt only appears when both are
	// terminals. They default to os.Stdin and os.Stderr.
	Stdin  io.Reader
	Stderr io.Writer

	// CLIConfig contains configuration from CLI flags.
	// These take highest precedence.
	CLIConfig *config.Config

	// terminal decides whether Stdin and Stderr are terminals. Tests set it;
	// nil means interactiveTerminal.
	terminal func(stdin io.Reader, stderr io.Writer) bool
}

func (o LoadOptions) stdin() io.Reader {
	if o.Stdin == nil {
		return os.Stdin
	}
	return o.Stdin
}

func (o LoadOptions) stderr() io.Writer {
	if o.Stderr == nil {
		return os.Stderr
	}
	return o.Stderr
}

// interactive reports whether a prompt can be answered.
func (o LoadOptions) interactive() bool {
	if o.NonInteractive {
		return false
	}
	terminal := o.terminal
	if terminal == nil {
		terminal = interactiveTerminal
	}
	return terminal(o.stdin(), o.stderr())
}

// LoadResult contains the resolved configuration and metadata.
type LoadResult struct {
	// Config is the final merged configuration.
	Config *config.Config

	// Paths contains the discovered configuration file paths.
	Paths *ConfigPaths

	// LoadedFrom lists the files that were actually loaded (in order).
	LoadedFrom []string

	// Warnings contains non-fatal issues encountered during loading.
	Warnings []string

	// Notices are informational messages, such as a markdownlint config
	// being translated for the run.
	Notices []string

	// MigrationPerformed is true if a translated markdownlint config was
	// saved as a gomdlint config file.
	MigrationPerformed bool

	// IgnoreBaseDir is the absolute directory that relative patterns in
	// Config.Ignore are resolved against: the directory of the project or
	// explicit config file that set them, or the working directory when
	// they come from a user or system config, the environment, or flags.
	IgnoreBaseDir string
}

// Load resolves the final configuration by merging all sources.
// Precedence (highest to lowest):
//  1. CLI flags (opts.CLIConfig)
//  2. Environment variables (GOMDLINT_*)
//  3. Explicit config file (opts.ExplicitPath), which replaces the project config
//  4. Project config (.gomdlint.yml, .gomdlint.json or another name of
//     gomdlintConfigFiles, searched upward), or, without one, a
//     markdownlint config translated for the run
//  5. User config ($XDG_CONFIG_HOME/gomdlint/config.yaml, .yml or .json)
//  6. System config (/etc/gomdlint/config.yaml, .yml or .json)
//  7. Defaults
func Load(ctx context.Context, opts LoadOptions) (*LoadResult, error) {
	result := &LoadResult{
		Paths: &ConfigPaths{},
	}

	// Resolve working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	result.IgnoreBaseDir = workDir

	// Start with defaults
	cfg := config.NewConfig()

	// Discover config paths
	paths, err := DiscoverPaths(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("discover paths: %w", err)
	}
	result.Paths = paths

	// An explicit config file replaces project discovery.
	explicitPath := opts.ExplicitPath
	if explicitPath != "" {
		if explicitPath, err = filepath.Abs(explicitPath); err != nil {
			return nil, fmt.Errorf("resolve config path: %w", err)
		}
		paths.Explicit = explicitPath
		paths.Project = ""
		paths.Markdownlint = ""
	}
	if opts.IgnoreMarkdownlint {
		paths.Markdownlint = ""
	}

	// Load and merge in order (lowest to highest precedence)

	// loadFile merges one configuration file into cfg. ignoreBase is the
	// directory the file's ignore patterns are relative to.
	loadFile := func(kind, path, ignoreBase string) error {
		fileCfg, warnings, err := loadConfigFile(path)
		if err != nil {
			return fmt.Errorf("load %s config: %w", kind, err)
		}
		cfg = merge(cfg, fileCfg)
		if fileCfg.Ignore != nil {
			result.IgnoreBaseDir = ignoreBase
		}
		result.LoadedFrom = append(result.LoadedFrom, path)
		result.Warnings = append(result.Warnings, warnings...)
		return nil
	}

	// 1. System config. Machine-wide patterns follow the working directory.
	if !opts.IgnoreSystemConfig && paths.System != "" {
		if err := loadFile("system", paths.System, workDir); err != nil {
			return nil, err
		}
	}

	// 2. User config. Personal patterns follow the working directory.
	if !opts.IgnoreUserConfig && paths.User != "" {
		if err := loadFile("user", paths.User, workDir); err != nil {
			return nil, err
		}
	}

	// 3. Project config. Its patterns are relative to its own directory.
	// Without one, a markdownlint config stands in for it.
	switch {
	case opts.IgnoreProjectConfig:
	case paths.Project != "":
		if err := loadFile("project", paths.Project, filepath.Dir(paths.Project)); err != nil {
			return nil, err
		}
		if paths.Markdownlint != "" && filepath.Dir(paths.Markdownlint) == filepath.Dir(paths.Project) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("both %s and %s exist; using %s", paths.Project, paths.Markdownlint, paths.Project))
		}
	case paths.Markdownlint != "":
		translated, err := applyMarkdownlint(paths.Markdownlint, opts, result)
		if err != nil {
			return nil, err
		}
		cfg = merge(cfg, translated)
	}

	// 4. Explicit config (--config flag)
	if explicitPath != "" {
		if err := loadFile("explicit", explicitPath, filepath.Dir(explicitPath)); err != nil {
			return nil, err
		}
	}

	// 5. Environment variables
	if !opts.IgnoreEnv {
		if err := LoadFromEnv(cfg); err != nil {
			return nil, fmt.Errorf("load environment: %w", err)
		}
		if os.Getenv(envVarPrefix+"IGNORE") != "" {
			result.IgnoreBaseDir = workDir
		}
	}

	// 6. CLI config (highest precedence)
	if opts.CLIConfig != nil {
		// Normalize a copy so the caller's configuration stays untouched.
		cliCfg := *opts.CLIConfig
		if err := normalizeRuleKeys(&cliCfg, lint.DefaultRegistry); err != nil {
			return nil, fmt.Errorf("command-line configuration: %w", err)
		}
		cfg = merge(cfg, &cliCfg)
		if cliCfg.Ignore != nil {
			result.IgnoreBaseDir = workDir
		}
	}

	// Validate final configuration
	validation := Validate(cfg)
	if !validation.Valid() {
		// Return first error
		return nil, &validation.Errors[0]
	}

	// Add validation warnings to result
	for _, w := range validation.Warnings {
		result.Warnings = append(result.Warnings, w.Message)
	}

	result.Config = cfg
	return result, nil
}

// loadConfigFile loads a configuration from a file: JSON if its name ends
// in .json, and YAML otherwise. The warnings name keys in the file that
// gomdlint does not read.
func loadConfigFile(path string) (*config.Config, []string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	format, parse := "YAML", parseYAMLDocument
	if HoldsJSON(path) {
		format, parse = "JSON", jsonDocument
	}
	doc, err := parse(content)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: parse %s: %w", path, format, err)
	}

	cfg := &config.Config{}
	if doc.Kind != 0 { // An empty YAML file holds no node and sets nothing.
		if err := doc.Decode(cfg); err != nil {
			return nil, nil, fmt.Errorf("%s: parse %s: %w", path, format, err)
		}
	}

	// Ensure Rules map is initialized
	if cfg.Rules == nil {
		cfg.Rules = make(map[string]config.RuleConfig)
	}

	// Rule options may use markdownlint's spellings, and rule keys may be
	// IDs, names or aliases in any case. The options go first, so that an
	// error names the rule key as the file writes it.
	if err := normalizeRuleOptions(cfg, lint.DefaultRegistry); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := normalizeRuleKeys(cfg, lint.DefaultRegistry); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, unknownKeyWarnings(path, doc, lint.DefaultRegistry), nil
}

// parseYAMLDocument parses content into a YAML document node. Decoding the
// node gives the same result as decoding content directly. Empty content
// gives a node of kind zero.
func parseYAMLDocument(content []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err //nolint:wrapcheck // The caller names the file and the format.
	}
	return &doc, nil
}

// applyMarkdownlint translates the markdownlint config at path for this run,
// in place of a gomdlint project config, and returns the rule settings to
// merge. From an interactive terminal it also offers to save the
// translation as a gomdlint config file next to the markdownlint one.
func applyMarkdownlint(path string, opts LoadOptions, result *LoadResult) (*config.Config, error) {
	if !CanMigrate(path) {
		// Nothing to translate: an empty layer leaves the settings as they are.
		result.Warnings = append(result.Warnings, GetMigrationWarning(path))
		return &config.Config{}, nil
	}

	converted, err := ConvertMarkdownlintConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load markdownlint config %s: %w", path, err)
	}
	for _, warning := range converted.Warnings {
		result.Warnings = append(result.Warnings, path+": "+warning)
	}
	result.LoadedFrom = append(result.LoadedFrom, path)

	target := filepath.Join(filepath.Dir(path), gomdlintConfigFiles[0])
	if opts.interactive() && promptYes(opts.stdin(), opts.stderr(),
		fmt.Sprintf("Save the gomdlint translation of %s as %s? [Y/n] ", path, target)) {
		if err := saveMigratedConfig(converted, target); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		} else {
			result.MigrationPerformed = true
			result.Notices = append(result.Notices,
				fmt.Sprintf("saved the settings of %s as %s; you can delete the old file", path, target))
		}
	}
	if !result.MigrationPerformed {
		windows := runtime.GOOS == goosWindows
		result.Notices = append(result.Notices,
			fmt.Sprintf("using %s, translated to gomdlint settings for this run; "+
				"to save them, run: gomdlint migrate %s --output %s",
				path, shellQuote(path, windows), shellQuote(target, windows)))
	}

	// The translation holds rule settings only, so other settings keep the
	// values of the layers below.
	return converted.Config, nil
}

// saveMigratedConfig writes a translated markdownlint config to target,
// refusing to replace an existing file.
func saveMigratedConfig(converted *MigrationResult, target string) error {
	content, err := converted.FileContent(target)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, configFilePermissions)
	if err != nil {
		return fmt.Errorf("save %s: %w", target, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close() // The write error is the one to report.
		return fmt.Errorf("save %s: %w", target, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("save %s: %w", target, err)
	}
	return nil
}

// shellQuote quotes path for a command line the user may paste into a
// shell, leaving it bare when it holds only characters no shell treats
// specially. POSIX shells get single quotes; Windows gets double quotes,
// since cmd.exe does not treat single quotes as quotes.
func shellQuote(path string, windows bool) string {
	special := func(char rune) bool {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			return false
		case strings.ContainsRune("@%+=:,./_-", char):
			return false
		default:
			return !windows || char != '\\'
		}
	}
	if path != "" && !strings.ContainsFunc(path, special) {
		return path
	}
	if windows {
		return `"` + path + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// promptYes asks question on stderr and reads the answer from stdin. An
// empty answer means yes; a closed or unreadable stdin means no.
func promptYes(stdin io.Reader, stderr io.Writer, question string) bool {
	if _, err := io.WriteString(stderr, question); err != nil {
		return false
	}
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && answer == "" {
		_, _ = io.WriteString(stderr, "\n") //nolint:errcheck // Best effort to end the prompt line.
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y" || answer == "yes"
}

// interactiveTerminal reports whether a prompt on stderr can be answered on
// stdin: both must be terminals. Output redirected to a file, or input from
// a pipe, rules a prompt out.
func interactiveTerminal(stdin io.Reader, stderr io.Writer) bool {
	return isTerminal(stdin) && isTerminal(stderr)
}

// isTerminal reports whether stream is a file attached to a terminal.
func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	descriptor := file.Fd()
	if descriptor > math.MaxInt {
		return false
	}
	return term.IsTerminal(int(descriptor))
}

// normalizeRuleOptions rewrites the options of each rule in one
// configuration layer to the canonical names that the rule's catalog entry
// declares, such as max for markdownlint's line_length (see
// lint.Registry.NormalizeOptions), so that layers merge option by option
// whatever spelling each uses. A value of the wrong type is dropped, so the
// rule uses its default, and an unknown option is kept, unread; the file's
// warnings report both. Two spellings of one option in one rule's settings
// are an error: either value could win. Options of a key that names no
// rule stay as they are.
func normalizeRuleOptions(cfg *config.Config, registry *lint.Registry) error {
	for _, section := range []struct {
		name  string
		rules map[string]config.RuleConfig
	}{{"rules", cfg.Rules}, {"rule_defaults", cfg.RuleDefaults}} {
		// Sorted keys keep the error message stable.
		for _, key := range slices.Sorted(maps.Keys(section.rules)) {
			ruleCfg := section.rules[key]
			rule, found := registry.Lookup(key)
			if !found || len(ruleCfg.Options) == 0 {
				continue
			}
			normalized, issues, _ := registry.NormalizeOptions(rule.ID(), ruleCfg.Options)
			for _, issue := range issues {
				if issue.Kind == lint.OptionIssueConflict {
					return fmt.Errorf("%s: %s", section.name, issue.Text(rule.Name()))
				}
			}
			ruleCfg.Options = normalized
			section.rules[key] = ruleCfg
		}
	}
	return nil
}

// normalizeRuleKeys converts the rule names, aliases and differently cased
// IDs of one configuration layer to canonical IDs, so layers merge by rule
// whatever key each uses. A rule configured under two keys in one layer is
// an error: either key could win, depending on map order. Keys that name no
// rule stay as they are; the file's unknown key warnings report them.
func normalizeRuleKeys(cfg *config.Config, registry *lint.Registry) error {
	rules, err := normalizeRuleMap("rules", cfg.Rules, registry)
	if err != nil {
		return err
	}
	defaults, err := normalizeRuleMap("rule_defaults", cfg.RuleDefaults, registry)
	if err != nil {
		return err
	}
	cfg.Rules, cfg.RuleDefaults = rules, defaults
	return nil
}

func normalizeRuleMap(
	section string,
	rules map[string]config.RuleConfig,
	registry *lint.Registry,
) (map[string]config.RuleConfig, error) {
	if len(rules) == 0 {
		return rules, nil
	}

	normalized := make(map[string]config.RuleConfig, len(rules))
	seen := make(map[string]string, len(rules)) // canonical ID -> original key

	// Sorted keys keep the error message stable.
	for _, key := range slices.Sorted(maps.Keys(rules)) {
		rule, found := registry.Lookup(key)
		if !found {
			// Unknown rule: keep it as-is, so that it configures nothing.
			normalized[key] = rules[key]
			continue
		}
		canonicalID := rule.ID()

		if previous, duplicate := seen[canonicalID]; duplicate {
			return nil, fmt.Errorf("%s: %q and %q both configure %s; keep one of them",
				section, previous, key, lint.RuleLabel(canonicalID, rule.Name()))
		}

		seen[canonicalID] = key
		normalized[canonicalID] = rules[key]
	}

	return normalized, nil
}
