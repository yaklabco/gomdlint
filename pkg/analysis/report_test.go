package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jamesainslie/gomdlint/pkg/config"
)

func TestTotals_HasIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		totals Totals
		want   bool
	}{
		{
			name:   "no issues",
			totals: Totals{Issues: 0},
			want:   false,
		},
		{
			name:   "has issues",
			totals: Totals{Issues: 5},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.totals.HasIssues())
		})
	}
}

func TestTotals_HasErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		totals Totals
		want   bool
	}{
		{
			name:   "no errors",
			totals: Totals{Errors: 0, Warnings: 5},
			want:   false,
		},
		{
			name:   "has errors",
			totals: Totals{Errors: 3},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.totals.HasErrors())
		})
	}
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultOptions()

	assert.True(t, opts.IncludeDiagnostics)
	assert.True(t, opts.IncludeByFile)
	assert.True(t, opts.IncludeByRule)
	assert.Equal(t, SortByCount, opts.SortBy)
	assert.True(t, opts.SortDesc)
	assert.Equal(t, config.RuleFormatName, opts.RuleFormat)
}

func TestSortField_IsValid(t *testing.T) {
	t.Parallel()

	assert.True(t, SortByCount.IsValid())
	assert.True(t, SortByAlpha.IsValid())
	assert.True(t, SortBySeverity.IsValid())
	assert.False(t, SortField("invalid").IsValid())
}
