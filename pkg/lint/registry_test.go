package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklabco/gomdlint/pkg/config"
)

// mockRule for testing.
type mockRule struct {
	id   string
	name string
}

func (m *mockRule) ID() string                               { return m.id }
func (m *mockRule) Name() string                             { return m.name }
func (m *mockRule) Description() string                      { return "mock" }
func (m *mockRule) DefaultEnabled() bool                     { return true }
func (m *mockRule) DefaultSeverity() config.Severity         { return config.SeverityWarning }
func (m *mockRule) Tags() []string                           { return nil }
func (m *mockRule) CanFix() bool                             { return false }
func (m *mockRule) Apply(*RuleContext) ([]Diagnostic, error) { return nil, nil }

func TestRegistry_GetByName(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	got, ok := reg.GetByName("no-trailing-spaces")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())
}

func TestRegistry_GetByName_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.GetByName("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_Get_ByNameFallback(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	// Get should find by name when ID doesn't match
	got, ok := reg.Get("no-trailing-spaces")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())
}

func TestRegistry_Resolve(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	tests := []struct {
		key    string
		wantID string
		wantOK bool
	}{
		{"MD009", "MD009", true},
		{"no-trailing-spaces", "MD009", true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		id, _, ok := reg.Resolve(tt.key)
		assert.Equal(t, tt.wantOK, ok, "key: %s", tt.key)
		if tt.wantOK {
			assert.Equal(t, tt.wantID, id, "key: %s", tt.key)
		}
	}
}

func TestRegistry_GetByID(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	got, ok := reg.GetByID("MD009")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())

	_, ok = reg.GetByID("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_Register_And_Get(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD001", name: "heading-increment"}
	reg.Register(rule)

	// Should be retrievable by ID
	got, ok := reg.Get("MD001")
	assert.True(t, ok)
	assert.Equal(t, "MD001", got.ID())
	assert.Equal(t, "heading-increment", got.Name())
}

func TestRegistry_Rules(t *testing.T) {
	reg := NewRegistry()
	rule1 := &mockRule{id: "MD001", name: "heading-increment"}
	rule2 := &mockRule{id: "MD002", name: "first-heading-h1"}
	reg.Register(rule1)
	reg.Register(rule2)

	rules := reg.Rules()
	assert.Len(t, rules, 2)
	// Should be sorted by ID
	assert.Equal(t, "MD001", rules[0].ID())
	assert.Equal(t, "MD002", rules[1].ID())
}

func TestRegistry_IDs(t *testing.T) {
	reg := NewRegistry()
	rule1 := &mockRule{id: "MD002", name: "first-heading-h1"}
	rule2 := &mockRule{id: "MD001", name: "heading-increment"}
	reg.Register(rule1)
	reg.Register(rule2)

	ids := reg.IDs()
	assert.Equal(t, []string{"MD001", "MD002"}, ids)
}

func TestRegistry_RegisterAlias(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD041", name: "first-line-heading"}
	reg.Register(rule)
	reg.RegisterAlias("single-h1", "MD041")
	reg.RegisterAlias("first-line-h1", "MD041")

	// Should resolve alias to rule
	id, r, ok := reg.Resolve("single-h1")
	assert.True(t, ok)
	assert.Equal(t, "MD041", id)
	assert.Equal(t, "first-line-heading", r.Name())

	// Should resolve other alias too
	id2, _, ok2 := reg.Resolve("first-line-h1")
	assert.True(t, ok2)
	assert.Equal(t, "MD041", id2)
}

func TestRegistry_RegisterAlias_UnknownRule(t *testing.T) {
	reg := NewRegistry()
	// Registering alias for unknown rule should not panic
	reg.RegisterAlias("some-alias", "UNKNOWN")

	_, _, ok := reg.Resolve("some-alias")
	assert.False(t, ok)
}
