package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jamesainslie/gomdlint/pkg/config"
)

func TestFormatRuleID(t *testing.T) {
	tests := []struct {
		name     string
		format   config.RuleFormat
		ruleID   string
		ruleName string
		want     string
	}{
		{"name format", config.RuleFormatName, "MD009", "no-trailing-spaces", "no-trailing-spaces"},
		{"id format", config.RuleFormatID, "MD009", "no-trailing-spaces", "MD009"},
		{"combined format", config.RuleFormatCombined, "MD009", "no-trailing-spaces", "MD009/no-trailing-spaces"},
		{"name format empty name", config.RuleFormatName, "MD009", "", "MD009"},
		{"default to name", config.RuleFormat(""), "MD009", "no-trailing-spaces", "no-trailing-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.FormatRuleID(tt.format, tt.ruleID, tt.ruleName)
			assert.Equal(t, tt.want, got)
		})
	}
}
