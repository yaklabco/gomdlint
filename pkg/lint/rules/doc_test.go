package rules

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// docRuleLinePattern matches rule list entries such as "//   - MD009: no-trailing-spaces".
var docRuleLinePattern = regexp.MustCompile(`(?m)^//\s+- (MDL?\d{3}): ([a-z0-9-]+)$`)

// TestPackageDocListsRegisteredRules keeps the package documentation in sync with the registry.
func TestPackageDocListsRegisteredRules(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "doc.go"))
	require.NoError(t, err)

	documented := make(map[string]string)
	for _, match := range docRuleLinePattern.FindAllStringSubmatch(string(source), -1) {
		documented[match[1]] = match[2]
	}

	registry := lint.NewRegistry()
	RegisterAll(registry)
	registered := make([]string, 0, len(registry.Rules()))
	for _, rule := range registry.Rules() {
		registered = append(registered, rule.ID())
		name, listed := documented[rule.ID()]
		if assert.True(t, listed, "doc.go does not list registered rule %s", rule.ID()) {
			assert.Equal(t, rule.Name(), name, "doc.go lists the wrong name for %s", rule.ID())
		}
	}

	documentedIDs := make([]string, 0, len(documented))
	for id := range documented {
		documentedIDs = append(documentedIDs, id)
	}
	sort.Strings(documentedIDs)
	sort.Strings(registered)
	assert.Equal(t, registered, documentedIDs, "doc.go lists rules that are not registered")
}
