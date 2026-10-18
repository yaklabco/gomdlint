package configloader

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// markdownlintOptions loads from dir with only a markdownlint config in
// play. The terminal check reports interactive, and stdin answers answer.
func markdownlintOptions(dir string, interactive bool, answer string, stderr io.Writer) LoadOptions {
	opts := isolatedOptions(dir)
	opts.IgnoreMarkdownlint = false
	opts.NonInteractive = false
	opts.Stdin = strings.NewReader(answer)
	opts.Stderr = stderr
	opts.terminal = func(io.Reader, io.Writer) bool { return interactive }
	return opts
}

func writeMarkdownlint(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestLoad_AppliesMarkdownlintConfig covers a project whose only
// configuration is a markdownlint file: its settings apply to the run.
func TestLoad_AppliesMarkdownlintConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := writeMarkdownlint(t, dir, ".markdownlint.json", `{"default": true, "MD013": false, "no-trailing-spaces": false}`)

	var stderr bytes.Buffer
	result, err := Load(context.Background(), markdownlintOptions(dir, false, "", &stderr))
	require.NoError(t, err)

	for _, id := range []string{"MD013", "MD009"} {
		rule, ok := result.Config.Rules[id]
		require.True(t, ok, "%s must be configured", id)
		require.NotNil(t, rule.Enabled)
		assert.False(t, *rule.Enabled, "%s must be disabled", id)
	}
	assert.Contains(t, result.LoadedFrom, source)
	require.Len(t, result.Notices, 1)
	assert.Contains(t, result.Notices[0], source)
	assert.Contains(t, result.Notices[0],
		"gomdlint migrate "+source+" --output "+filepath.Join(dir, ".gomdlint.yml"),
		"the notice gives a command that works from any directory")
	assert.False(t, result.MigrationPerformed)
	assert.Empty(t, stderr.String(), "no prompt without a terminal")
	assert.NoFileExists(t, filepath.Join(dir, ".gomdlint.yml"))
}

// TestLoad_MarkdownlintNoticeQuotesPaths covers a project directory whose
// name a shell would split or misread: the suggested command still passes
// each path as one argument.
func TestLoad_MarkdownlintNoticeQuotesPaths(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == goosWindows {
		t.Skip("the check runs the command through a POSIX shell")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no POSIX shell: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "it's a $HOME")
	source := writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD013": false}`)

	result, err := Load(context.Background(), markdownlintOptions(dir, false, "", io.Discard))
	require.NoError(t, err)
	require.Len(t, result.Notices, 1)

	_, command, found := strings.Cut(result.Notices[0], "run: ")
	require.True(t, found, "the notice must give a command: %s", result.Notices[0])
	out, err := exec.Command(shell, "-c", `set -- `+command+`; printf '%s\n' "$@"`).Output()
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"gomdlint", "migrate", source, "--output", filepath.Join(dir, ".gomdlint.yml")},
		strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"))
}

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path    string
		windows bool
		want    string
	}{
		{path: "/home/me/docs/.markdownlint.json", want: "/home/me/docs/.markdownlint.json"},
		{path: "/home/me/my docs/.markdownlint.json", want: "'/home/me/my docs/.markdownlint.json'"},
		{path: "/home/me/it's/x", want: `'/home/me/it'\''s/x'`},
		{path: `C:\docs\.markdownlint.json`, windows: true, want: `C:\docs\.markdownlint.json`},
		{path: `C:\my docs\.markdownlint.json`, windows: true, want: `"C:\my docs\.markdownlint.json"`},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, shellQuote(tt.path, tt.windows), tt.path)
	}
}

func TestLoad_AppliesMarkdownlintConfigFromParentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".markdownlint.yaml", "MD013: false\n")
	sub := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	result, err := Load(context.Background(), markdownlintOptions(sub, false, "", io.Discard))
	require.NoError(t, err)

	rule, ok := result.Config.Rules["MD013"]
	require.True(t, ok)
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
}

func TestLoad_MarkdownlintConfigKeepsOtherLayers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD013": false}`)

	opts := markdownlintOptions(dir, false, "", io.Discard)
	opts.CLIConfig = nil
	result, err := Load(context.Background(), opts)
	require.NoError(t, err)

	// The translation supplies rule settings only; it must not reset
	// settings such as backups that other layers own.
	assert.True(t, result.Config.Backups.IsEnabled())
	assert.Equal(t, "sidecar", result.Config.Backups.Mode)
}

func TestLoad_MalformedMarkdownlintConfigFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD013": `)

	_, err := Load(context.Background(), markdownlintOptions(dir, false, "", io.Discard))
	require.Error(t, err)
	assert.Contains(t, err.Error(), source)
}

func TestLoad_JavaScriptMarkdownlintConfigWarns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".markdownlint.cjs", "module.exports = {};\n")

	result, err := Load(context.Background(), markdownlintOptions(dir, true, "y\n", io.Discard))
	require.NoError(t, err)

	assert.Empty(t, result.Config.Rules)
	assert.Contains(t, strings.Join(result.Warnings, "\n"), "cannot be converted")
}

func TestLoad_GomdlintConfigWinsOverMarkdownlint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD013": false}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("flavor: gfm\n"), 0o644))

	var stderr bytes.Buffer
	result, err := Load(context.Background(), markdownlintOptions(dir, true, "y\n", &stderr))
	require.NoError(t, err)

	assert.NotContains(t, result.Config.Rules, "MD013")
	assert.Contains(t, strings.Join(result.Warnings, "\n"), "using")
	assert.Empty(t, stderr.String(), "no prompt when a gomdlint config exists")
}

// TestLoad_MarkdownlintPrompt covers the interactive offer to save the
// translation. The prompt goes to stderr, never stdout, so redirected
// output stays clean, and the settings apply whatever the answer.
func TestLoad_MarkdownlintPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		answer    string
		wantWrite bool
	}{
		{name: "yes", answer: "y\n", wantWrite: true},
		{name: "default is yes", answer: "\n", wantWrite: true},
		{name: "no", answer: "n\n", wantWrite: false},
		{name: "closed input", answer: "", wantWrite: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			sub := filepath.Join(dir, "docs")
			require.NoError(t, os.MkdirAll(sub, 0o755))
			writeMarkdownlint(t, dir, ".markdownlint.json", `{"MD013": false}`)

			var stderr bytes.Buffer
			result, err := Load(context.Background(), markdownlintOptions(sub, true, tt.answer, &stderr))
			require.NoError(t, err)

			assert.Contains(t, stderr.String(), "Save")
			rule, ok := result.Config.Rules["MD013"]
			require.True(t, ok, "the translation applies whatever the answer")
			require.NotNil(t, rule.Enabled)
			assert.False(t, *rule.Enabled)

			// The file goes next to the markdownlint config, not into the
			// working directory.
			written := filepath.Join(dir, ".gomdlint.yml")
			assert.Equal(t, tt.wantWrite, result.MigrationPerformed)
			if tt.wantWrite {
				content, err := os.ReadFile(written)
				require.NoError(t, err)
				assert.Contains(t, string(content), "MD013")
			} else {
				assert.NoFileExists(t, written)
			}
			assert.NoFileExists(t, filepath.Join(sub, ".gomdlint.yml"))
		})
	}
}

// TestLoad_SavedMigrationKeepsInheritedSettings covers a markdownlint config
// whose translation is saved, as the prompt offers, under a user config
// that sets what a markdownlint config cannot: ignore patterns, the flavor,
// the gitignore and default_ignores settings, backups and a rule's
// auto_fix. The saved file holds only rule settings, and leaves auto_fix
// unset, so the user's settings still apply, as they do while the
// translation applies for a run.
func TestLoad_SavedMigrationKeepsInheritedSettings(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "xdg", "gomdlint")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "config.yaml"), []byte(
		"ignore:\n  - \"drafts/**\"\n"+
			"flavor: commonmark\n"+
			"gitignore: false\n"+
			"default_ignores: false\n"+
			"backups:\n  enabled: false\n  mode: none\n"+
			"rules:\n  MD013:\n    auto_fix: false\n"), 0o644))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))

	project := filepath.Join(root, "project")
	source := writeMarkdownlint(t, project, ".markdownlint.json", `{"MD013": false}`)
	converted, err := ConvertMarkdownlintConfig(source)
	require.NoError(t, err)
	target := filepath.Join(project, ".gomdlint.yml")
	require.NoError(t, saveMigratedConfig(converted, target))
	require.NoError(t, os.Remove(source))

	opts := isolatedOptions(project)
	opts.IgnoreUserConfig = false
	result, err := Load(context.Background(), opts)
	require.NoError(t, err)

	require.Equal(t, []string{result.Paths.User, target}, result.LoadedFrom)
	assert.Equal(t, []string{"drafts/**"}, result.Config.Ignore)
	assert.Equal(t, project, result.IgnoreBaseDir, "the user's patterns follow the working directory")
	assert.Equal(t, config.FlavorCommonMark, result.Config.Flavor)
	assert.False(t, result.Config.GitignoreEnabled())
	assert.False(t, result.Config.DefaultIgnoresEnabled())
	assert.False(t, result.Config.Backups.IsEnabled())
	assert.Equal(t, "none", result.Config.Backups.Mode)
	rule, ok := result.Config.Rules["MD013"]
	require.True(t, ok, "the migrated rule settings apply")
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	require.NotNil(t, rule.AutoFix, "the user's auto_fix survives the migrated rule settings")
	assert.False(t, *rule.AutoFix)
}

func TestInteractiveTerminalNeedsTerminals(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, reader.Close())
		assert.NoError(t, writer.Close())
	})

	assert.False(t, interactiveTerminal(reader, writer), "pipes are not terminals")
	assert.False(t, interactiveTerminal(strings.NewReader(""), io.Discard), "non-file streams are not terminals")
}
