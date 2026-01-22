package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/config"
)

func TestRuleFormat_Values(t *testing.T) {
	assert.Equal(t, config.RuleFormatName, config.RuleFormat("name"))
	assert.Equal(t, config.RuleFormatID, config.RuleFormat("id"))
	assert.Equal(t, config.RuleFormatCombined, config.RuleFormat("combined"))
}

func TestRuleFormat_String(t *testing.T) {
	tests := []struct {
		format config.RuleFormat
		want   string
	}{
		{config.RuleFormatName, "name"},
		{config.RuleFormatID, "id"},
		{config.RuleFormatCombined, "combined"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, string(tt.format))
	}
}

func TestNewConfig_DefaultRuleFormat(t *testing.T) {
	cfg := config.NewConfig()
	assert.Equal(t, config.RuleFormatName, cfg.RuleFormat)
}
