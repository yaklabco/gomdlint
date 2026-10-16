package configloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
