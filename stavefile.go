//go:build stave

package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Default target runs build.
var Default = Build

// Aliases for common targets.
var Aliases = map[string]interface{}{
	"b": Build,
	"t": Test,
	"l": Lint,
	"c": Check,
	"i": Install,
}

// ldflags returns the linker flags for version injection.
func ldflags() string {
	version, err := shOutput(context.Background(), "git", "describe", "--tags", "--always", "--dirty")
	if err != nil || version == "" {
		version = "dev"
	}
	commit, err := shOutput(context.Background(), "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		commit = "none"
	}
	date := time.Now().UTC().Format(time.RFC3339)

	return fmt.Sprintf(
		"-X main.version=%s -X main.commit=%s -X main.date=%s",
		strings.TrimSpace(version),
		strings.TrimSpace(commit),
		date,
	)
}

// Build compiles the gomdlint binary with version info.
func Build(ctx context.Context) error {
	fmt.Println("Building gomdlint...")
	return sh(ctx, "go", "build", "-ldflags", ldflags(), "-o", "bin/gomdlint", "./cmd/gomdlint")
}

// Test runs all tests using gotestsum with race detection and coverage.
func Test(ctx context.Context) error {
	fmt.Println("Running tests...")
	nCores := cmp.Or(os.Getenv("STAVE_NUM_PROCESSORS"), "4")
	args := []string{
		"tool", "gotestsum",
		"-f", "pkgname-and-test-fails",
		"--",
		"-v", "-race",
		"-p", nCores,
		"-parallel", nCores,
		"./...",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
	}
	return sh(ctx, "go", args...)
}

// TestV runs all tests with verbose output.
func TestV(ctx context.Context) error {
	fmt.Println("Running tests (verbose)...")
	nCores := cmp.Or(os.Getenv("STAVE_NUM_PROCESSORS"), "4")
	args := []string{
		"tool", "gotestsum",
		"-f", "standard-verbose",
		"--",
		"-v", "-race",
		"-p", nCores,
		"-parallel", nCores,
		"./...",
		"-coverprofile=coverage.out",
		"-covermode=atomic",
	}
	return sh(ctx, "go", args...)
}

// Lint runs golangci-lint with auto-fix.
func Lint(ctx context.Context) error {
	fmt.Println("Running linters...")
	return sh(ctx, "golangci-lint", "run", "--fix", "./...")
}

// LintCI runs golangci-lint without auto-fix (for CI).
func LintCI(ctx context.Context) error {
	fmt.Println("Running linters (CI mode)...")
	return sh(ctx, "golangci-lint", "run", "./...")
}

// Fmt formats all Go code.
func Fmt(ctx context.Context) error {
	fmt.Println("Formatting code...")
	return sh(ctx, "gofmt", "-w", ".")
}

// Format is an alias for Fmt.
func Format(ctx context.Context) error {
	return Fmt(ctx)
}

// Check runs format, lint, and test.
func Check(ctx context.Context) error {
	fmt.Println("Running checks...")
	if err := Fmt(ctx); err != nil {
		return err
	}
	if err := Lint(ctx); err != nil {
		return err
	}
	return Test(ctx)
}

// Vet runs go vet.
func Vet(ctx context.Context) error {
	fmt.Println("Running go vet...")
	return sh(ctx, "go", "vet", "./...")
}

// CIGate runs all CI checks in idiomatic Go order.
func CIGate(ctx context.Context) error {
	fmt.Println("Running CI gate checks...")

	// 1. Check formatting
	fmt.Println("\n1. Checking code formatting...")
	out, err := shOutput(ctx, "gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("gofmt check failed: %w", err)
	}
	if out != "" {
		return fmt.Errorf("the following files are not formatted:\n%s\nRun 'gofmt -w .' or 'stave fmt' to fix", out)
	}
	fmt.Println("✓ Code formatting OK")

	// 2. Run go vet
	fmt.Println("\n2. Running go vet...")
	if err := sh(ctx, "go", "vet", "./..."); err != nil {
		return fmt.Errorf("go vet failed: %w", err)
	}
	fmt.Println("✓ go vet passed")

	// 3. Run golangci-lint
	fmt.Println("\n3. Running golangci-lint...")
	if err := sh(ctx, "golangci-lint", "run", "./..."); err != nil {
		return fmt.Errorf("golangci-lint failed: %w", err)
	}
	fmt.Println("✓ golangci-lint passed")

	// 4. Build the project
	fmt.Println("\n4. Building project...")
	if err := Build(ctx); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	fmt.Println("✓ Build successful")

	// 5. Run tests
	fmt.Println("\n5. Running tests...")
	if err := Test(ctx); err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}
	fmt.Println("✓ Tests passed")

	// 6. Check go.mod/go.sum are tidy
	fmt.Println("\n6. Checking go.mod/go.sum...")
	if err := ModTidy(ctx); err != nil {
		return fmt.Errorf("mod tidy check failed: %w", err)
	}

	// 7. Cross-compile for all platforms
	fmt.Println("\n7. Cross-compiling for release platforms...")
	if err := CrossCompile(ctx); err != nil {
		return fmt.Errorf("cross-compile failed: %w", err)
	}

	fmt.Println("\n✓ All CI gate checks passed!")
	return nil
}

// Clean removes build artifacts.
func Clean(ctx context.Context) error {
	fmt.Println("Cleaning build artifacts...")
	if err := os.RemoveAll("bin"); err != nil {
		return err
	}
	_ = os.Remove("coverage.out")
	_ = os.Remove("coverage.html")
	return nil
}

// Install installs gomdlint to $GOBIN or $GOPATH/bin.
func Install(ctx context.Context) error {
	fmt.Println("Installing gomdlint...")
	return sh(ctx, "go", "install", "-ldflags", ldflags(), "./cmd/gomdlint")
}

// Uninstall removes gomdlint from $GOBIN or $GOPATH/bin.
func Uninstall(ctx context.Context) error {
	fmt.Println("Uninstalling gomdlint...")

	binPath, err := findInstalledBinary("gomdlint")
	if err != nil {
		return err
	}

	if err := os.Remove(binPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("gomdlint is not installed")
			return nil
		}
		return fmt.Errorf("remove binary: %w", err)
	}

	fmt.Printf("Removed %s\n", binPath)
	return nil
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

// Deps ensures all dependencies are downloaded.
func Deps(ctx context.Context) error {
	fmt.Println("Downloading dependencies...")
	if err := sh(ctx, "go", "mod", "download"); err != nil {
		return err
	}
	return sh(ctx, "go", "mod", "tidy")
}

// Bench runs benchmarks.
func Bench(ctx context.Context) error {
	fmt.Println("Running benchmarks...")
	return sh(ctx, "go", "test", "-bench=.", "-benchmem", "./...")
}

// ModTidy checks that go.mod and go.sum are tidy.
func ModTidy(ctx context.Context) error {
	fmt.Println("Checking go.mod/go.sum are tidy...")

	// Get current state of go.mod and go.sum
	modBefore, _ := os.ReadFile("go.mod")
	sumBefore, _ := os.ReadFile("go.sum")

	// Run go mod tidy
	if err := sh(ctx, "go", "mod", "tidy"); err != nil {
		return err
	}

	// Check if files changed
	modAfter, _ := os.ReadFile("go.mod")
	sumAfter, _ := os.ReadFile("go.sum")

	if string(modBefore) != string(modAfter) || string(sumBefore) != string(sumAfter) {
		return fmt.Errorf("go.mod or go.sum changed after 'go mod tidy' - please commit the changes")
	}

	fmt.Println("✓ go.mod/go.sum are tidy")
	return nil
}

// CrossCompile builds for all release platforms to catch platform-specific issues.
func CrossCompile(ctx context.Context) error {
	fmt.Println("Cross-compiling for all release platforms...")

	platforms := []struct {
		goos   string
		goarch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
	}

	for _, p := range platforms {
		fmt.Printf("  Building %s/%s...\n", p.goos, p.goarch)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", "/dev/null", "./cmd/gomdlint")
		cmd.Env = append(os.Environ(), "GOOS="+p.goos, "GOARCH="+p.goarch, "CGO_ENABLED=0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build failed for %s/%s: %w", p.goos, p.goarch, err)
		}
	}

	fmt.Println("✓ All platforms build successfully")
	return nil
}

// Coverage generates test coverage report.
func Coverage(ctx context.Context) error {
	fmt.Println("Generating coverage report...")
	if err := Test(ctx); err != nil {
		return err
	}
	if err := sh(ctx, "go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html"); err != nil {
		return err
	}
	return sh(ctx, "open", "coverage.html")
}

// sh executes a shell command with proper output handling.
func sh(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Printf("→ %s %s\n", name, strings.Join(args, " "))

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s exited with code %d", name, exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// shOutput executes a command and returns its output.
func shOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
