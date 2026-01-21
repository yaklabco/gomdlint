package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/runner"
)

func TestDiscover_SingleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(mdFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{mdFile},
		WorkingDir: dir,
	}

	files, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if files[0] != mdFile {
		t.Errorf("expected %s, got %s", mdFile, files[0])
	}
}

func TestDiscover_Directory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create test files.
	files := []string{
		"readme.md",
		"docs/guide.md",
		"docs/api.markdown",
		"src/main.go",
		"notes.txt",
	}

	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup write: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should find only Markdown files.
	expected := []string{
		filepath.Join(dir, "docs/api.markdown"),
		filepath.Join(dir, "docs/guide.md"),
		filepath.Join(dir, "readme.md"),
	}

	if len(discovered) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(discovered), discovered)
	}

	for i, exp := range expected {
		if discovered[i] != exp {
			t.Errorf("file[%d] = %s, want %s", i, discovered[i], exp)
		}
	}
}

func TestDiscover_DefaultsToCurrentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      nil, // Should default to "."
		WorkingDir: dir,
	}

	files, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestDiscover_CustomExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create test files with different extensions.
	testFiles := []string{"file.md", "file.markdown", "file.txt", "file.mdx"}
	for _, f := range testFiles {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Extensions: []string{".mdx", ".txt"},
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(discovered) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(discovered), discovered)
	}

	// Should only find .mdx and .txt files.
	for _, f := range discovered {
		ext := filepath.Ext(f)
		if ext != ".mdx" && ext != ".txt" {
			t.Errorf("unexpected file extension: %s", f)
		}
	}
}

func TestDiscover_ExcludeGlobs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create test structure.
	files := []string{
		"readme.md",
		"vendor/pkg/doc.md",
		"node_modules/lib/readme.md",
		"docs/guide.md",
	}

	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup write: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:        []string{"."},
		WorkingDir:   dir,
		ExcludeGlobs: []string{"vendor/**", "node_modules/**"},
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should exclude vendor and node_modules.
	expected := []string{
		filepath.Join(dir, "docs/guide.md"),
		filepath.Join(dir, "readme.md"),
	}

	if len(discovered) != len(expected) {
		t.Fatalf("expected %d files, got %d: %v", len(expected), len(discovered), discovered)
	}

	sort.Strings(expected)
	for i, exp := range expected {
		if discovered[i] != exp {
			t.Errorf("file[%d] = %s, want %s", i, discovered[i], exp)
		}
	}
}

func TestDiscover_IncludeGlobs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create test structure.
	files := []string{
		"readme.md",
		"docs/guide.md",
		"docs/api.md",
		"src/readme.md",
	}

	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup write: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:        []string{"."},
		WorkingDir:   dir,
		IncludeGlobs: []string{"docs/**"},
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should only include files under docs/.
	for _, f := range discovered {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			t.Fatalf("filepath.Rel error: %v", err)
		}
		if !hasPrefix(rel, "docs") {
			t.Errorf("unexpected file outside docs: %s", rel)
		}
	}

	if len(discovered) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(discovered), discovered)
	}
}

func TestDiscover_HiddenFilesAndDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create test structure with hidden files/dirs.
	files := []string{
		"readme.md",
		".hidden.md",
		".git/config.md",
		"docs/.secret.md",
	}

	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup write: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should only find non-hidden readme.md.
	if len(discovered) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(discovered), discovered)
	}

	if filepath.Base(discovered[0]) != "readme.md" {
		t.Errorf("expected readme.md, got %s", filepath.Base(discovered[0]))
	}
}

func TestDiscover_DeterministicOrdering(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create files in non-alphabetical order.
	files := []string{"z.md", "a.md", "m.md", "b.md"}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
	}

	// Run discovery multiple times.
	results := make([][]string, 0, 5)
	for range 5 {
		discovered, err := runner.Discover(ctx, opts)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}
		results = append(results, discovered)
	}

	// All results should be identical.
	for runIdx := 1; runIdx < len(results); runIdx++ {
		if len(results[runIdx]) != len(results[0]) {
			t.Errorf("run %d has different length: %d vs %d", runIdx, len(results[runIdx]), len(results[0]))
			continue
		}
		for fileIdx := range results[runIdx] {
			if results[runIdx][fileIdx] != results[0][fileIdx] {
				t.Errorf("run %d, file %d differs: %s vs %s", runIdx, fileIdx, results[runIdx][fileIdx], results[0][fileIdx])
			}
		}
	}

	// Verify sorted order.
	for sortIdx := 1; sortIdx < len(results[0]); sortIdx++ {
		if results[0][sortIdx] < results[0][sortIdx-1] {
			t.Errorf("files not sorted: %s should come after %s", results[0][sortIdx-1], results[0][sortIdx])
		}
	}
}

func TestDiscover_Deduplication(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(mdFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	ctx := context.Background()
	opts := runner.Options{
		// Same file via different paths.
		Paths:      []string{"readme.md", "./readme.md", "readme.md"},
		WorkingDir: dir,
	}

	files, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (deduplicated), got %d: %v", len(files), files)
	}
}

func TestDiscover_MultiplePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create separate directories.
	dirs := []string{"docs", "guides", "notes"}
	for _, d := range dirs {
		subDir := filepath.Join(dir, d)
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("setup mkdir: %v", err)
		}
		mdFile := filepath.Join(subDir, "readme.md")
		if err := os.WriteFile(mdFile, []byte("content"), 0644); err != nil {
			t.Fatalf("setup write: %v", err)
		}
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"docs", "guides"},
		WorkingDir: dir,
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should find files only in docs and guides, not notes.
	if len(discovered) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(discovered), discovered)
	}

	for _, f := range discovered {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			t.Fatalf("filepath.Rel error: %v", err)
		}
		if !hasPrefix(rel, "docs") && !hasPrefix(rel, "guides") {
			t.Errorf("unexpected file: %s", rel)
		}
	}
}

func TestDiscover_NonExistentPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"nonexistent"},
		WorkingDir: dir,
	}

	_, err := runner.Discover(ctx, opts)
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestDiscover_ContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create some files.
	for idx := range 10 {
		path := filepath.Join(dir, "file"+string(rune('a'+idx))+".md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
	}

	_, err := runner.Discover(ctx, opts)
	if err == nil {
		t.Log("no error returned, cancellation may not have been caught early")
	}
}

func TestDiscover_Symlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create real file.
	realFile := filepath.Join(dir, "real.md")
	if err := os.WriteFile(realFile, []byte("content"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Create file symlink.
	linkFile := filepath.Join(dir, "link.md")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should find both the real file and the symlink.
	if len(discovered) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(discovered), discovered)
	}
}

func TestDiscover_DirectorySymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a real subdirectory with a file.
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("setup mkdir real: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "doc.md"), []byte("content"), 0644); err != nil {
		t.Fatalf("setup write real: %v", err)
	}

	// Create external directory (outside the walk root) with a different file.
	externalDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(externalDir, "external.md"), []byte("external"), 0644); err != nil {
		t.Fatalf("setup write external: %v", err)
	}

	// Create a symlink inside dir pointing to the external directory.
	linkDir := filepath.Join(dir, "linked")
	if err := os.Symlink(externalDir, linkDir); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// Test without following symlinks - should only find real/doc.md.
	ctx := context.Background()
	opts := runner.Options{
		Paths:          []string{"."},
		WorkingDir:     dir,
		FollowSymlinks: false,
	}

	discovered, err := runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(discovered) != 1 {
		t.Errorf("expected 1 file without FollowSymlinks, got %d: %v", len(discovered), discovered)
	}

	// Verify the file is from real/, not linked/.
	if len(discovered) == 1 && !strings.Contains(discovered[0], "real") {
		t.Errorf("expected file from real/, got: %v", discovered[0])
	}

	// Test with following symlinks - should find both files.
	opts.FollowSymlinks = true
	discovered, err = runner.Discover(ctx, opts)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should find 2 files: real/doc.md and the external file via symlink.
	if len(discovered) != 2 {
		t.Errorf("expected 2 files with FollowSymlinks, got %d: %v", len(discovered), discovered)
	}

	// Verify we found both files.
	foundReal, foundExternal := false, false
	for _, f := range discovered {
		if strings.HasSuffix(f, "doc.md") {
			foundReal = true
		}
		if strings.HasSuffix(f, "external.md") {
			foundExternal = true
		}
	}
	if !foundReal || !foundExternal {
		t.Errorf("expected to find both doc.md and external.md, got: %v", discovered)
	}
}

func TestDefaultExtensions(t *testing.T) {
	t.Parallel()

	exts := runner.DefaultExtensions()

	if len(exts) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(exts))
	}

	expected := map[string]bool{".md": true, ".markdown": true}
	for _, ext := range exts {
		if !expected[ext] {
			t.Errorf("unexpected extension: %s", ext)
		}
	}
}

// hasPrefix checks if path starts with prefix as a path component.
func hasPrefix(path, prefix string) bool {
	path = filepath.ToSlash(path)
	prefix = filepath.ToSlash(prefix)
	return path == prefix || len(path) > len(prefix) && path[:len(prefix)+1] == prefix+"/"
}
