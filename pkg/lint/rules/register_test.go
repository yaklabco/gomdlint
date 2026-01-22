package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

func TestRegisterAll(t *testing.T) {
	registry := lint.NewRegistry()
	RegisterAll(registry)

	// Verify some key rules are registered
	rules := registry.Rules()
	assert.NotEmpty(t, rules, "should register rules")

	// Check a few specific rules by ID
	rule, ok := registry.GetByID("MD009")
	require.True(t, ok, "MD009 should be registered")
	assert.Equal(t, "no-trailing-spaces", rule.Name())

	rule, ok = registry.GetByID("MD041")
	require.True(t, ok, "MD041 should be registered")
	assert.Equal(t, "first-line-heading", rule.Name())

	rule, ok = registry.GetByID("MD025")
	require.True(t, ok, "MD025 should be registered")
	assert.Equal(t, "single-h1", rule.Name())
}

func TestRegisterLegacyAliases(t *testing.T) {
	registry := lint.NewRegistry()
	RegisterAll(registry)
	RegisterLegacyAliases(registry)

	tests := []struct {
		name         string
		alias        string
		expectID     string
		expectName   string
		expectExists bool
	}{
		{
			name:         "single-title resolves to MD025",
			alias:        "single-title",
			expectID:     "MD025",
			expectName:   "single-h1",
			expectExists: true,
		},
		{
			name:         "first-line-h1 resolves to MD041",
			alias:        "first-line-h1",
			expectID:     "MD041",
			expectName:   "first-line-heading",
			expectExists: true,
		},
		{
			name:         "canonical name single-h1 still works",
			alias:        "single-h1",
			expectID:     "MD025",
			expectName:   "single-h1",
			expectExists: true,
		},
		{
			name:         "canonical name first-line-heading still works",
			alias:        "first-line-heading",
			expectID:     "MD041",
			expectName:   "first-line-heading",
			expectExists: true,
		},
		{
			name:         "ID MD025 still works",
			alias:        "MD025",
			expectID:     "MD025",
			expectName:   "single-h1",
			expectExists: true,
		},
		{
			name:         "ID MD041 still works",
			alias:        "MD041",
			expectID:     "MD041",
			expectName:   "first-line-heading",
			expectExists: true,
		},
		{
			name:         "nonexistent alias returns not found",
			alias:        "nonexistent-alias",
			expectExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, rule, ok := registry.Resolve(tt.alias)

			if !tt.expectExists {
				assert.False(t, ok, "alias %q should not exist", tt.alias)
				return
			}

			require.True(t, ok, "alias %q should exist", tt.alias)
			assert.Equal(t, tt.expectID, id, "alias %q should resolve to ID %q", tt.alias, tt.expectID)
			assert.Equal(t, tt.expectName, rule.Name(), "alias %q should resolve to rule with name %q", tt.alias, tt.expectName)
		})
	}
}

func TestLegacyAliasesResolveCorrectly(t *testing.T) {
	// Test against the default registry to ensure aliases work in practice
	// This verifies the init() function registered aliases correctly

	// Verify single-title alias
	id, rule, ok := lint.DefaultRegistry.Resolve("single-title")
	require.True(t, ok, "single-title should resolve in DefaultRegistry")
	assert.Equal(t, "MD025", id)
	assert.Equal(t, "single-h1", rule.Name())

	// Verify first-line-h1 alias
	id, rule, ok = lint.DefaultRegistry.Resolve("first-line-h1")
	require.True(t, ok, "first-line-h1 should resolve in DefaultRegistry")
	assert.Equal(t, "MD041", id)
	assert.Equal(t, "first-line-heading", rule.Name())
}

func TestDefaultRegistryHasAllRules(t *testing.T) {
	// Verify the default registry was properly initialized
	rules := lint.DefaultRegistry.Rules()
	assert.NotEmpty(t, rules)

	// Check that we have a reasonable number of rules registered
	// This helps catch issues where init() might not have run
	assert.GreaterOrEqual(t, len(rules), 30, "should have at least 30 rules registered")
}
