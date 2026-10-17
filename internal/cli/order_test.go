package cli_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/reporter"
)

// wantOrder is the order of the findings in the fixture project: files by
// path, and each file's findings by line, then column and rule ID, as
// markdownlint-cli2 sorts them by line. The text and table goldens show the
// same order.
var wantOrder = []string{
	"README.md:3:22 MD009",
	"README.md:6:11 MD026",
	"README.md:8:19 MD009",
	"docs/guide.md:3:1 MD001",
	"docs/guide.md:5:18 MD009",
	"docs/guide.md:7:10 MD026",
	"docs/guide.md:11:9 MD026",
	"docs/guide.md:13:14 MD009",
	"docs/guide.md:14:12 MD009",
	"docs/guide.md:17:1 MD053",
}

// TestLintMachineOutputOrder covers JSON and SARIF output, which list the
// findings in the order text and table output print them.
func TestLintMachineOutputOrder(t *testing.T) {
	skipUnlessSlashPaths(t)
	project := readProject(t, testdataPath("project"))

	t.Run("json", func(t *testing.T) {
		inProject(t, project)

		got := execute(t, "lint", "--format", "json")

		var output reporter.JSONOutput
		require.NoError(t, json.Unmarshal([]byte(got.stdout), &output), got.stdout)
		var order []string
		for _, file := range output.Files {
			for _, diag := range file.Diagnostics {
				order = append(order, fmt.Sprintf("%s:%d:%d %s", file.Path, diag.StartLine, diag.StartColumn, diag.RuleID))
			}
		}
		assert.Equal(t, wantOrder, order)
	})

	t.Run("sarif", func(t *testing.T) {
		inProject(t, project)

		got := execute(t, "lint", "--format", "sarif")

		var output reporter.SARIFOutput
		require.NoError(t, json.Unmarshal([]byte(got.stdout), &output), got.stdout)
		require.Len(t, output.Runs, 1)
		var order []string
		for _, result := range output.Runs[0].Results {
			require.Len(t, result.Locations, 1)
			location := result.Locations[0].PhysicalLocation
			require.NotNil(t, location.Region)
			order = append(order, fmt.Sprintf("%s:%d:%d %s",
				location.ArtifactLocation.URI, location.Region.StartLine, location.Region.StartColumn, result.RuleID))
		}
		assert.Equal(t, wantOrder, order)
	})
}
