package reporter_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/reporter"
)

func TestNew_SummaryFormat(t *testing.T) {
	var buf bytes.Buffer
	opts := reporter.Options{
		Writer: &buf,
		Format: reporter.FormatSummary,
		Color:  "never",
	}

	rep, err := reporter.New(opts)
	require.NoError(t, err)
	assert.NotNil(t, rep)
}
