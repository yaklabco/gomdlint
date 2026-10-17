package configloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// projectConfigNames are the project config file names gomdlint reads, in
// order of precedence: in a directory that holds several, the first is used
// and the others are ignored. "gomdlint init" writes the first in YAML and
// the fifth with --format json. YAML names come first, so a project that
// already has a YAML config keeps using it.
var projectConfigNames = []string{
	".gomdlint.yml",
	".gomdlint.yaml",
	"gomdlint.yml",
	"gomdlint.yaml",
	".gomdlint.json",
	"gomdlint.json",
}

// TestFindProjectConfig_FindsEveryName covers each project config name on
// its own, found from the directory that holds it and from a subdirectory.
func TestFindProjectConfig_FindsEveryName(t *testing.T) {
	t.Parallel()

	for _, name := range projectConfigNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := t.TempDir()
			docs := filepath.Join(repo, "docs")
			if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.MkdirAll(docs, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			want := filepath.Join(repo, name)
			if err := os.WriteFile(want, []byte("{}\n"), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			for _, dir := range []string{repo, docs} {
				found, err := FindProjectConfig(context.Background(), dir)
				if err != nil {
					t.Fatalf("FindProjectConfig(%s) error = %v", dir, err)
				}
				if found != want {
					t.Errorf("FindProjectConfig(%s) = %q, want %q", dir, found, want)
				}
			}
		})
	}
}

// TestFindProjectConfig_Precedence covers directories that hold two project
// config files: the one earlier in projectConfigNames wins, whatever else
// is there.
func TestFindProjectConfig_Precedence(t *testing.T) {
	t.Parallel()

	for first := range projectConfigNames {
		for second := first + 1; second < len(projectConfigNames); second++ {
			preferred, ignored := projectConfigNames[first], projectConfigNames[second]
			t.Run(preferred+" over "+ignored, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				for _, name := range []string{preferred, ignored} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
						t.Fatalf("write config: %v", err)
					}
				}

				found, err := FindProjectConfig(context.Background(), dir)
				if err != nil {
					t.Fatalf("FindProjectConfig() error = %v", err)
				}
				if want := filepath.Join(dir, preferred); found != want {
					t.Errorf("FindProjectConfig() = %q, want %q", found, want)
				}
			})
		}
	}
}

// TestDiscoverPaths_UserConfigNames covers the user config, which may be
// config.yaml, config.yml or config.json, preferred in that order, like
// the YAML-first order of project configs.
func TestDiscoverPaths_UserConfigNames(t *testing.T) {
	names := []string{"config.yaml", "config.yml", "config.json"}
	for idx, name := range names {
		t.Run(name, func(t *testing.T) {
			xdg := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdg)
			dir := filepath.Join(xdg, "gomdlint")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			// The name and every name after it exist; the name wins.
			for _, present := range names[idx:] {
				if err := os.WriteFile(filepath.Join(dir, present), []byte("{}\n"), 0o644); err != nil {
					t.Fatalf("write config: %v", err)
				}
			}

			paths, err := DiscoverPaths(context.Background(), t.TempDir())
			if err != nil {
				t.Fatalf("DiscoverPaths() error = %v", err)
			}
			if want := filepath.Join(dir, name); paths.User != want {
				t.Errorf("user config = %q, want %q", paths.User, want)
			}
		})
	}
}

// TestFindProjectConfig_StopsAtWorktreeRoot covers Git worktrees and
// submodules, whose .git is a file. The search must not leave the working
// tree and pick up a config file from a parent directory.
func TestFindProjectConfig_StopsAtWorktreeRoot(t *testing.T) {
	t.Parallel()

	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	docs := filepath.Join(repo, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outer, ".gomdlint.yml"), []byte("flavor: gfm\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	found, err := FindProjectConfig(context.Background(), docs)
	if err != nil {
		t.Fatalf("FindProjectConfig() error = %v", err)
	}
	if found != "" {
		t.Errorf("found %s outside the working tree", found)
	}
}
