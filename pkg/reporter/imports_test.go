package reporter_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	modulePath = "github.com/yaklabco/gomdlint"

	// repositoryRoot is the root of the module, relative to this package.
	repositoryRoot = "../.."
)

// runGo runs the go command with args in dir, with env added to the
// environment, and returns its standard output. It fails the test with
// the command's standard error when the command fails, and skips the
// test when there is no go command on PATH.
func runGo(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()

	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command on PATH")
	}
	cmd := exec.Command(goTool, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, "go %s: %s", strings.Join(args, " "), stderr.String())
	return string(out)
}

// TestReporterDoesNotImportTheBuiltinRules checks the import graph of
// pkg/reporter: it must not import the built-in rules, whose package
// registers them in lint.DefaultRegistry when it initializes, or
// internal/ruledocs, which imports them. A program that imports the
// reporter would otherwise have its own registrations under built-in IDs
// replaced. Rule help reaches the reporter through Options.RuleHelp.
func TestReporterDoesNotImportTheBuiltinRules(t *testing.T) {
	t.Parallel()

	deps := strings.Fields(runGo(t, ".", nil, "list", "-deps", "-f", "{{.ImportPath}}", modulePath+"/pkg/reporter"))

	require.True(t, slices.Contains(deps, modulePath+"/pkg/lint"), "the listing holds the reporter's dependencies")
	for _, forbidden := range []string{modulePath + "/pkg/lint/rules", modulePath + "/internal/ruledocs"} {
		assert.False(t, slices.Contains(deps, forbidden), "pkg/reporter imports %s", forbidden)
	}
}

// customRegistrationProgram is a program, without its go.mod, that
// registers a custom rule under MD009 in lint.DefaultRegistry while it
// initializes, imports the reporter, and prints the name of the rule that
// the default registry holds under MD009. Its custom package initializes
// before the reporter's dependencies, as example.com sorts before
// github.com.
var customRegistrationProgram = map[string]string{
	"custom/custom.go": `package custom

import "` + modulePath + `/pkg/lint"

func init() {
	rule := lint.NewBaseRule("MD009", "company-policy", "Company policy.", nil, false)
	lint.DefaultRegistry.Register(&rule)
}
`,
	"main.go": `package main

import (
	"fmt"

	_ "example.com/audit/custom"
	"` + modulePath + `/pkg/lint"
	_ "` + modulePath + `/pkg/reporter"
)

func main() {
	rule, _ := lint.DefaultRegistry.GetByID("MD009")
	fmt.Println(rule.Name())
}
`,
}

// TestReporterKeepsRulesRegisteredWhileInitializing builds a program that
// registers a custom rule under MD009 while it initializes and imports
// the reporter, in a workspace with this module, and checks that the
// custom rule stays registered.
func TestReporterKeepsRulesRegisteredWhileInitializing(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	t.Parallel()

	root, err := filepath.Abs(repositoryRoot)
	require.NoError(t, err)
	goVersion := strings.TrimSpace(runGo(t, root, []string{"GOWORK=off"}, "list", "-m", "-f", "{{.GoVersion}}"))
	require.NotEmpty(t, goVersion, "go.mod declares a Go version")

	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/audit\n\ngo " + goVersion + "\n",
		"go.work": "go " + goVersion + "\n\nuse (\n\t.\n\t" + filepath.ToSlash(root) + "\n)\n",
	}
	for name, content := range customRegistrationProgram {
		files[name] = content
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	env := []string{"GOWORK=" + filepath.Join(dir, "go.work"), "GOFLAGS=", "GOTOOLCHAIN=local"}
	out := runGo(t, dir, env, "run", "-mod=readonly", ".")

	assert.Equal(t, "company-policy\n", out)
}
