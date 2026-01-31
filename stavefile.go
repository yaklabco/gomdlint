//go:build stave

package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklabco/stave/pkg/sh"
	"github.com/yaklabco/stave/pkg/st"
	"github.com/yaklabco/stave/pkg/target"
)

// Default target runs build.
var Default = Build

// Aliases for common targets.
var Aliases = map[string]any{
	"b":    Build,
	"t":    Test.Default,
	"l":    Lint.Default,
	"c":    Check,
	"i":    Install,
	"fmt":  Lint.Fmt,
	"cmp":  Bench.Compare,
	"cmpf": Bench.Fast,
}

// Namespace types group related targets.
type (
	Test  st.Namespace
	Lint  st.Namespace
	CI    st.Namespace
	Bench st.Namespace
)

// ---------------------------------------------------------------------------
// Top-level targets
// ---------------------------------------------------------------------------

// Build compiles the gomdlint binary with version info.
// Skips recompilation when source files have not changed.
func Build() error {
	rebuild, err := target.Dir("bin/gomdlint", "cmd/", "pkg/", "internal/", "go.mod", "go.sum")
	if err != nil {
		return err
	}
	if !rebuild {
		fmt.Println("bin/gomdlint is up to date")
		return nil
	}
	fmt.Println("Building gomdlint...")
	return sh.RunV("go", "build", "-ldflags", ldflags(), "-o", "bin/gomdlint", "./cmd/gomdlint")
}

// Check runs format, lint, and test sequentially.
func Check() {
	st.SerialDeps(Lint.Fmt, Lint.Default, Test.Default)
}

// Clean removes build artifacts.
func Clean() error {
	fmt.Println("Cleaning build artifacts...")
	if err := sh.Rm("bin"); err != nil {
		return err
	}
	if err := sh.Rm("coverage.out"); err != nil {
		return err
	}
	return sh.Rm("coverage.html")
}

// Install installs gomdlint to $GOBIN or $GOPATH/bin.
func Install() error {
	fmt.Println("Installing gomdlint...")
	return sh.RunV("go", "install", "-ldflags", ldflags(), "./cmd/gomdlint")
}

// Uninstall removes gomdlint from $GOBIN or $GOPATH/bin.
func Uninstall() error {
	fmt.Println("Uninstalling gomdlint...")
	binPath, err := findInstalledBinary("gomdlint")
	if err != nil {
		return err
	}
	if err := os.Remove(binPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Println("gomdlint is not installed")
			return nil
		}
		return fmt.Errorf("remove binary: %w", err)
	}
	fmt.Printf("Removed %s\n", binPath)
	return nil
}

// Deps ensures all dependencies are downloaded.
func Deps() error {
	fmt.Println("Downloading dependencies...")
	if err := sh.RunV("go", "mod", "download"); err != nil {
		return err
	}
	return sh.RunV("go", "mod", "tidy")
}

// Coverage generates a test coverage report and opens it.
func Coverage() error {
	st.Deps(Test.Default)
	fmt.Println("Generating coverage report...")
	if err := sh.RunV("go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html"); err != nil {
		return err
	}
	return sh.RunV("open", "coverage.html")
}

// ---------------------------------------------------------------------------
// Test namespace
// ---------------------------------------------------------------------------

// Default runs all tests using gotestsum with race detection and coverage.
func (Test) Default() error {
	fmt.Println("Running tests...")
	nCores := cmp.Or(os.Getenv("STAVE_NUM_PROCESSORS"), "4")
	return sh.RunV("go",
		"tool", "gotestsum",
		"-f", "pkgname-and-test-fails",
		"--",
		"-v", "-race",
		"-p", nCores,
		"-parallel", nCores,
		"./...",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
	)
}

// Verbose runs all tests with standard-verbose output.
func (Test) Verbose() error {
	fmt.Println("Running tests (verbose)...")
	nCores := cmp.Or(os.Getenv("STAVE_NUM_PROCESSORS"), "4")
	return sh.RunV("go",
		"tool", "gotestsum",
		"-f", "standard-verbose",
		"--",
		"-v", "-race",
		"-p", nCores,
		"-parallel", nCores,
		"./...",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
	)
}

// ---------------------------------------------------------------------------
// Lint namespace
// ---------------------------------------------------------------------------

// Default runs golangci-lint with auto-fix.
func (Lint) Default() error {
	fmt.Println("Running linters...")
	return sh.RunV("golangci-lint", "run", "--fix", "./...")
}

// CI runs golangci-lint without auto-fix (for CI pipelines).
func (Lint) CI() error {
	fmt.Println("Running linters (CI mode)...")
	return sh.RunV("golangci-lint", "run", "./...")
}

// Fmt formats all Go code.
func (Lint) Fmt() error {
	fmt.Println("Formatting code...")
	return sh.RunV("gofmt", "-w", ".")
}

// FmtCheck verifies code formatting without modifying files.
func (Lint) FmtCheck() error {
	out, err := sh.Output("gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("gofmt check failed: %w", err)
	}
	if out != "" {
		return fmt.Errorf("unformatted files:\n%s\nRun 'stave lint:fmt' to fix", out)
	}
	fmt.Println("✓ Code formatting OK")
	return nil
}

// Vet runs go vet.
func (Lint) Vet() error {
	fmt.Println("Running go vet...")
	return sh.RunV("go", "vet", "./...")
}

// ---------------------------------------------------------------------------
// CI namespace
// ---------------------------------------------------------------------------

// Gate runs all CI checks in idiomatic Go order.
func (CI) Gate() error {
	fmt.Println("Running CI gate checks...")
	st.SerialDeps(
		Lint.FmtCheck,
		Lint.Vet,
		Lint.CI,
		Build,
		Test.Default,
		CI.ModTidy,
		CI.Cross,
	)
	fmt.Println("\n✓ All CI gate checks passed!")
	return nil
}

// ModTidy checks that go.mod and go.sum are tidy.
func (CI) ModTidy() error {
	fmt.Println("Checking go.mod/go.sum are tidy...")
	modBefore, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	sumBefore, err := os.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("read go.sum: %w", err)
	}

	if err := sh.RunV("go", "mod", "tidy"); err != nil {
		return err
	}

	modAfter, err := os.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("read go.mod after tidy: %w", err)
	}
	sumAfter, err := os.ReadFile("go.sum")
	if err != nil {
		return fmt.Errorf("read go.sum after tidy: %w", err)
	}

	if string(modBefore) != string(modAfter) || string(sumBefore) != string(sumAfter) {
		return errors.New("go.mod or go.sum changed after 'go mod tidy' - please commit the changes")
	}
	fmt.Println("✓ go.mod/go.sum are tidy")
	return nil
}

// Cross builds for all release platforms to catch platform-specific issues.
func (CI) Cross() error {
	fmt.Println("Cross-compiling for all release platforms...")
	platforms := []struct{ goos, goarch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"freebsd", "amd64"},
		{"freebsd", "arm64"},
		{"openbsd", "amd64"},
		{"netbsd", "amd64"},
	}
	for _, p := range platforms {
		fmt.Printf("  Building %s/%s...\n", p.goos, p.goarch)
		env := map[string]string{
			"GOOS":        p.goos,
			"GOARCH":      p.goarch,
			"CGO_ENABLED": "0",
		}
		if err := sh.RunWith(env, "go", "build", "-o", "/dev/null", "./cmd/gomdlint"); err != nil {
			return fmt.Errorf("build failed for %s/%s: %w", p.goos, p.goarch, err)
		}
	}
	fmt.Println("✓ All platforms build successfully")
	return nil
}

// ---------------------------------------------------------------------------
// Bench namespace
// ---------------------------------------------------------------------------

// Default runs Go benchmarks.
func (Bench) Default() error {
	fmt.Println("Running benchmarks...")
	return sh.RunV("go",
		"tool", "gotestsum",
		"-f", "pkgname-and-test-fails",
		"--",
		"-bench=.", "-benchmem",
		"./...",
	)
}

// Compare benchmarks gomdlint against markdownlint.
func (Bench) Compare() error {
	fmt.Println("Running gomdlint vs markdownlint comparison...")
	st.Deps(Build)
	if err := checkCompareDepends(); err != nil {
		return err
	}
	if err := sh.RunV("bench/scripts/clone-repos.sh"); err != nil {
		return fmt.Errorf("clone repos: %w", err)
	}
	if err := sh.RunV("bench/scripts/run-bench.sh"); err != nil {
		return fmt.Errorf("run benchmarks: %w", err)
	}
	if err := sh.RunV("bench/scripts/generate-plots.sh"); err != nil {
		return fmt.Errorf("generate charts: %w", err)
	}
	return nil
}

// Fast runs a quick comparison on smallest repos only.
func (Bench) Fast() error {
	fmt.Println("Running quick gomdlint vs markdownlint comparison...")
	st.Deps(Build)
	if err := checkCompareDepends(); err != nil {
		return err
	}
	if err := sh.RunV("bench/scripts/clone-repos.sh"); err != nil {
		return fmt.Errorf("clone repos: %w", err)
	}
	return sh.RunWithV(map[string]string{"BENCH_RUNS": "1"}, "bench/scripts/run-bench.sh")
}

// ---------------------------------------------------------------------------
// Helpers (unexported — not targets)
// ---------------------------------------------------------------------------

// gitOutput runs a git command and returns trimmed stdout, or empty on error.
func gitOutput(args ...string) string {
	out, err := sh.Output("git", args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// ldflags returns the linker flags for version injection.
func ldflags() string {
	version := cmp.Or(gitOutput("describe", "--tags", "--always", "--dirty"), "dev")
	commit := cmp.Or(gitOutput("rev-parse", "--short", "HEAD"), "none")
	date := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf(
		"-X main.version=%s -X main.commit=%s -X main.date=%s",
		version, commit, date,
	)
}

// findInstalledBinary returns the path where go install would place the binary.
func findInstalledBinary(name string) (string, error) {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return filepath.Join(gobin, name), nil
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		gopath = filepath.Join(home, "go")
	}
	return filepath.Join(gopath, "bin", name), nil
}

// checkCompareDepends verifies required tools are installed for benchmarking.
func checkCompareDepends() error {
	tools := []struct {
		name    string
		cmd     string
		args    []string
		install string
	}{
		{"markdownlint", "markdownlint", []string{"--version"}, "npm install -g markdownlint-cli"},
		{"jq", "jq", []string{"--version"}, "brew install jq"},
		{"gnuplot", "gnuplot", []string{"--version"}, "brew install gnuplot"},
	}

	hasGtime := exec.Command("which", "gtime").Run() == nil //nolint:gosec // args are constant
	hasGnuTime := false
	if !hasGtime {
		// --version may exit non-zero on some implementations; we only inspect output content.
		out, _ := exec.Command("/usr/bin/time", "--version").CombinedOutput() //nolint:gosec // args are constant
		hasGnuTime = strings.Contains(string(out), "GNU")
	}
	if !hasGtime && !hasGnuTime {
		return errors.New("GNU time required; install with: brew install gnu-time")
	}

	for _, tool := range tools {
		if err := exec.Command(tool.cmd, tool.args...).Run(); err != nil { //nolint:gosec // args are constant
			return fmt.Errorf("%s not found; install with: %s", tool.name, tool.install)
		}
	}
	return nil
}
