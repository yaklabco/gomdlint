package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/reporter"
)

func TestLint_SARIFRecordsBuildVersion(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Title\n\nTrailing   \n"), 0o644))
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0o644))

	cmd := cli.NewRootCommand(cli.BuildInfo{Version: "9.8.7", Commit: "abc1234", Date: "2026-10-16"})

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"lint", "--config", cfgFile, "--format", "sarif", "--color", "never", mdFile})

	_ = cmd.Execute() //nolint:errcheck // Findings make Execute fail; only the report matters here.

	var log reporter.SARIFOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &log), "stdout: %s\nstderr: %s", stdout.String(), stderr.String())
	require.Len(t, log.Runs, 1)
	assert.Equal(t, "9.8.7", log.Runs[0].Tool.Driver.Version, "SARIF records the version the binary was built as")
}
