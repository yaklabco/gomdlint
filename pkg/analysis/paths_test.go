package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayPath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	otherDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		workDir string
		want    string
	}{
		{
			name:    "file directly inside working directory",
			path:    filepath.Join(workDir, "README.md"),
			workDir: workDir,
			want:    "README.md",
		},
		{
			name:    "file in subdirectory keeps its directories",
			path:    filepath.Join(workDir, "docs", "guide", "intro.md"),
			workDir: workDir,
			want:    filepath.Join("docs", "guide", "intro.md"),
		},
		{
			name:    "directory whose name starts with two dots is inside",
			path:    filepath.Join(workDir, "..notes", "a.md"),
			workDir: workDir,
			want:    filepath.Join("..notes", "a.md"),
		},
		{
			name:    "file outside working directory stays absolute",
			path:    filepath.Join(otherDir, "sub", "a.md"),
			workDir: workDir,
			want:    filepath.Join(otherDir, "sub", "a.md"),
		},
		{
			name:    "sibling directory sharing a name prefix is outside",
			path:    filepath.Join(workDir+"-sibling", "a.md"),
			workDir: workDir,
			want:    filepath.Join(workDir+"-sibling", "a.md"),
		},
		{
			name:    "relative path is already relative",
			path:    filepath.Join("docs", "a.md"),
			workDir: workDir,
			want:    filepath.Join("docs", "a.md"),
		},
		{
			name:    "empty working directory keeps path",
			path:    filepath.Join(workDir, "a.md"),
			workDir: "",
			want:    filepath.Join(workDir, "a.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, DisplayPath(tt.path, tt.workDir))
		})
	}
}

func TestDisplayPath_ResolvesSymlinkedDirectories(t *testing.T) {
	t.Parallel()

	realDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(realDir, "docs"), 0o755))

	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	// Working directory reached through the link, file path through the real directory.
	assert.Equal(t, filepath.Join("docs", "a.md"),
		DisplayPath(filepath.Join(realDir, "docs", "a.md"), linkDir))

	// Working directory is the real directory, file path goes through the link.
	assert.Equal(t, filepath.Join("docs", "a.md"),
		DisplayPath(filepath.Join(linkDir, "docs", "a.md"), realDir))
}

func TestDisplayPath_DoesNotFollowFileSymlink(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.md")
	require.NoError(t, os.WriteFile(target, []byte("# Target\n"), 0o644))

	link := filepath.Join(workDir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	// The file is shown under the name it was linted by, not its target.
	assert.Equal(t, "link.md", DisplayPath(link, workDir))
}
