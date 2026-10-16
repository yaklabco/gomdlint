package configloader

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Contains(t, result.Notices[0], "gomdlint migrate")
	assert.False(t, result.MigrationPerformed)
	assert.Empty(t, stderr.String(), "no prompt without a terminal")
	assert.NoFileExists(t, filepath.Join(dir, ".gomdlint.yml"))
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
