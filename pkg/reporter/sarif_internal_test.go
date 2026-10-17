package reporter

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolVersion(t *testing.T) {
	t.Parallel()

	release := &debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}
	devel := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}

	tests := []struct {
		name      string
		explicit  string
		buildInfo *debug.BuildInfo
		want      string
	}{
		{name: "explicit version wins", explicit: "0.3.0", buildInfo: release, want: "0.3.0"},
		{name: "dev placeholder falls back to module version", explicit: "dev", buildInfo: release, want: "v0.3.0"},
		{name: "empty falls back to module version", explicit: "", buildInfo: release, want: "v0.3.0"},
		{name: "no release anywhere is omitted", explicit: "dev", buildInfo: devel, want: ""},
		{name: "no build information is omitted", explicit: "", buildInfo: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, toolVersion(tt.explicit, tt.buildInfo))
		})
	}
}

func TestFileURIFromSlash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "unix path", path: "/home/me/docs/a.md", want: "file:///home/me/docs/a.md"},
		{name: "characters that need escaping", path: "/srv/a b/#1/é%.md", want: "file:///srv/a%20b/%231/%C3%A9%25.md"},
		{name: "windows drive path", path: "C:/Users/me/docs/a.md", want: "file:///C:/Users/me/docs/a.md"},
		{name: "windows drive root", path: "C:/", want: "file:///C:/"},
		{name: "windows unc path", path: "//server/share/docs/a.md", want: "file://server/share/docs/a.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, fileURIFromSlash(tt.path))
		})
	}
}

func TestRelativeURI(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "docs/a%20b.md", relativeURI("docs/a b.md"))
	assert.Equal(t, "docs/a.md", relativeURI("./docs/a.md"))
	assert.Equal(t, "./a:b.md", relativeURI("a:b.md"), "a colon in the first segment must not read as a scheme")
}
